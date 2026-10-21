package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/lynx/lynx-os/telegram-bot/internal/bot"
)

func main() {
	config := bot.Config{
		BotToken:      os.Getenv("TELEGRAM_BOT_TOKEN"),
		LynxOSURL:     os.Getenv("LYNX_OS_INTERNAL_URL"),
		InternalToken: os.Getenv("LYNX_TELEGRAM_INTERNAL_TOKEN"),
	}
	if err := config.Validate(); err != nil {
		slog.Error("invalid Telegram bot configuration", "error", err)
		os.Exit(1)
	}

	port := strings.TrimSpace(os.Getenv("PORT"))
	if port == "" {
		port = "8081"
	}
	handler := bot.NewHandler(config, nil)
	server := &http.Server{
		Addr:              ":" + port,
		Handler:           handler.Routes(),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
		WriteTimeout:      55 * time.Second,
	}

	stopped, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-stopped.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	}()
	go func() {
		if err := handler.RunPolling(stopped); err != nil {
			slog.Error("Telegram polling stopped", "error", err)
			stop()
		}
	}()

	slog.Info("Lynx Telegram service listening", "address", server.Addr)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("Telegram service stopped", "error", err)
		os.Exit(1)
	}
}
