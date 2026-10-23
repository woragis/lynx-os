package main

import (
	"context"
	"encoding/hex"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	application "github.com/lynx/lynx-os/backend/internal/application/health"
	identityapp "github.com/lynx/lynx-os/backend/internal/application/identity"
	integrationapp "github.com/lynx/lynx-os/backend/internal/application/integration"
	telegramapp "github.com/lynx/lynx-os/backend/internal/application/telegram"
	domain "github.com/lynx/lynx-os/backend/internal/domain/health"
	identitydomain "github.com/lynx/lynx-os/backend/internal/domain/identity"
	"github.com/lynx/lynx-os/backend/internal/infrastructure/cryptox"
	"github.com/lynx/lynx-os/backend/internal/infrastructure/mailgun"
	openaiinfra "github.com/lynx/lynx-os/backend/internal/infrastructure/openai"
	"github.com/lynx/lynx-os/backend/internal/infrastructure/postgres"
	redisinfra "github.com/lynx/lynx-os/backend/internal/infrastructure/redis"
	"github.com/lynx/lynx-os/backend/internal/infrastructure/saasclient"
	smtpinfra "github.com/lynx/lynx-os/backend/internal/infrastructure/smtp"
	telegrambot "github.com/lynx/lynx-os/backend/internal/infrastructure/telegrambot"
	healthhttp "github.com/lynx/lynx-os/backend/internal/interfaces/http/health"
	identityhttp "github.com/lynx/lynx-os/backend/internal/interfaces/http/identity"
	integrationhttp "github.com/lynx/lynx-os/backend/internal/interfaces/http/integration"
	"github.com/lynx/lynx-os/backend/internal/platform/authn"
	"github.com/lynx/lynx-os/backend/internal/platform/config"
	"github.com/lynx/lynx-os/backend/internal/platform/httpx"
	redisclient "github.com/redis/go-redis/v9"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)
	cfg, err := config.Load()
	if err != nil {
		logger.Error("configuration is invalid")
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	db, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("could not create PostgreSQL pool")
		os.Exit(1)
	}
	defer db.Close()

	redisOptions, err := redisclient.ParseURL(cfg.RedisURL)
	if err != nil {
		logger.Error("REDIS_URL is invalid")
		os.Exit(1)
	}
	redis := redisclient.NewClient(redisOptions)
	defer redis.Close()

	checks := application.NewService(
		domain.Dependency{Name: "postgres", Checker: postgres.HealthChecker{Pool: db}},
		domain.Dependency{Name: "redis", Checker: redisinfra.HealthChecker{Client: redis}},
	)
	router := httpx.NewRouter()
	router.Mount("/health", healthhttp.NewHandler(checks).Routes())
	var identityHandler *identityhttp.Handler
	var identityService *identityapp.Service
	if cfg.JWTSigningKey != "" {
		jwtManager, jwtErr := authn.NewJWTManager([]byte(cfg.JWTSigningKey))
		if jwtErr != nil {
			logger.Error("LYNX_JWT_SIGNING_KEY must contain at least 32 characters")
			os.Exit(1)
		}
		var resetMailer identitydomain.PasswordResetMailer
		if strings.EqualFold(cfg.Environment, "dev") || strings.EqualFold(cfg.Environment, "development") {
			resetMailer, err = smtpinfra.NewPasswordResetMailer(cfg.SMTPHost, cfg.SMTPPort, cfg.SMTPFrom, cfg.PublicURL)
			if err != nil {
				logger.Error("local SMTP password recovery configuration is invalid")
				os.Exit(1)
			}
		} else {
			resetMailer, err = mailgun.NewPasswordResetMailer(cfg.MailgunAPIKey, cfg.MailgunDomain, cfg.MailgunFrom, cfg.PublicURL)
			if err != nil {
				logger.Error("Mailgun password recovery configuration is required outside development")
				os.Exit(1)
			}
		}
		var serviceErr error
		identityService, serviceErr = identityapp.NewService(postgres.NewIdentityRepository(db), redisinfra.NewLoginRateLimiter(redis), jwtManager, resetMailer)
		if serviceErr != nil {
			logger.Error("could not initialize administrator authentication")
			os.Exit(1)
		}
		identityHandler = identityhttp.NewHandlerWithTelegram(identityService, cfg.Environment == "production", cfg.TelegramInternalToken)
		router.Mount("/api/v1/auth", identityHandler.Routes())
	}
	var integrationService *integrationapp.Service
	var cipher *cryptox.AESGCM
	if cfg.EncryptionKey != "" {
		if identityHandler == nil {
			logger.Error("LYNX_JWT_SIGNING_KEY is required when enabling integration management")
			os.Exit(1)
		}
		key, decodeErr := hex.DecodeString(cfg.EncryptionKey)
		if decodeErr != nil || len(key) != 32 {
			logger.Error("LYNX_ENCRYPTION_KEY must be 64 hexadecimal characters when enabling integration management")
			os.Exit(1)
		}
		var cipherErr error
		cipher, cipherErr = cryptox.NewAESGCM(key)
		if cipherErr != nil {
			logger.Error("could not initialize integration secret encryption")
			os.Exit(1)
		}
		contractClient := saasclient.NewContractV1Client(nil)
		contractService := integrationapp.NewContractService(contractClient)
		integrationRepository := postgres.NewIntegrationRepository(db)
		integrationService = integrationapp.NewService(integrationRepository, integrationRepository, cipher, contractService)
		router.Mount("/api/v1/integrations", identityHandler.ProtectedMiddleware(integrationhttp.NewHandler(integrationService).Routes()))
	}
	if cfg.TelegramBotToken != "" {
		if identityService == nil || integrationService == nil || cipher == nil || cfg.OpenAIAPIKey == "" {
			logger.Error("Telegram bot requires administrator auth, integration encryption, and OPENAI_API_KEY")
			os.Exit(1)
		}
		telegramRepository := postgres.NewTelegramRepository(db)
		telegramService := telegramapp.NewService(telegramRepository, postgres.NewIdentityRepository(db), identityService, integrationService, openaiinfra.NewDraftExtractor(cfg.OpenAIAPIKey, cfg.OpenAIModel), cipher)
		worker := telegramapp.NewWorker(telegrambot.NewClient(cfg.TelegramBotToken), telegramService, telegramRepository, logger)
		go worker.Run(ctx)
		logger.Info("Telegram bot worker enabled in Go API process")
	}

	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       time.Duration(cfg.ReadTimeout) * time.Second,
		WriteTimeout:      time.Duration(cfg.WriteTimeout) * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			logger.Error("HTTP shutdown failed", "error", err)
		}
	}()

	logger.Info("Lynx OS Go API starting", "address", cfg.HTTPAddr, "environment", cfg.Environment)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("HTTP server stopped", "error", err)
		os.Exit(1)
	}
}
