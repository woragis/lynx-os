package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	application "github.com/lynx/lynx-os/backend/internal/application/health"
	domain "github.com/lynx/lynx-os/backend/internal/domain/health"
	"github.com/lynx/lynx-os/backend/internal/infrastructure/postgres"
	redisinfra "github.com/lynx/lynx-os/backend/internal/infrastructure/redis"
	healthhttp "github.com/lynx/lynx-os/backend/internal/interfaces/http/health"
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
