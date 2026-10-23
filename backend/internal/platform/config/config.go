package config

import (
	"encoding/hex"
	"errors"
	"net/url"
	"strings"

	"github.com/caarlos0/env/v11"
)

type Config struct {
	HTTPAddr              string `env:"LYNX_HTTP_ADDR" envDefault:":8082"`
	DatabaseURL           string `env:"DATABASE_URL,required"`
	RedisURL              string `env:"REDIS_URL,required"`
	Environment           string `env:"LYNX_ENV" envDefault:"development"`
	ReadTimeout           int    `env:"LYNX_HTTP_READ_TIMEOUT_SECONDS" envDefault:"10"`
	WriteTimeout          int    `env:"LYNX_HTTP_WRITE_TIMEOUT_SECONDS" envDefault:"30"`
	JWTSigningKey         string `env:"LYNX_JWT_SIGNING_KEY"`
	EncryptionKey         string `env:"LYNX_ENCRYPTION_KEY"`
	MailgunAPIKey         string `env:"MAILGUN_API_KEY"`
	MailgunDomain         string `env:"MAILGUN_DOMAIN"`
	MailgunFrom           string `env:"MAILGUN_FROM"`
	PublicURL             string `env:"LYNX_PUBLIC_URL" envDefault:"http://localhost:3000"`
	SMTPHost              string `env:"SMTP_HOST" envDefault:"127.0.0.1"`
	SMTPPort              int    `env:"SMTP_PORT" envDefault:"1025"`
	SMTPFrom              string `env:"SMTP_FROM" envDefault:"Lynx OS <no-reply@lynx.local>"`
	TelegramInternalToken string `env:"LYNX_TELEGRAM_INTERNAL_TOKEN"`
	TelegramBotToken      string `env:"TELEGRAM_BOT_TOKEN"`
	OpenAIAPIKey          string `env:"OPENAI_API_KEY"`
	OpenAIModel           string `env:"OPENAI_MODEL" envDefault:"gpt-6-luna"`
}

func Load() (Config, error) {
	var cfg Config
	if err := env.Parse(&cfg); err != nil {
		return Config{}, err
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c Config) Validate() error {
	if c.ReadTimeout < 1 || c.WriteTimeout < 1 {
		return errors.New("HTTP read/write timeouts must be positive")
	}
	if !strings.EqualFold(c.Environment, "production") {
		return nil
	}
	if len(c.JWTSigningKey) < 32 {
		return errors.New("LYNX_JWT_SIGNING_KEY must contain at least 32 characters in production")
	}
	key, err := hex.DecodeString(c.EncryptionKey)
	if err != nil || len(key) != 32 {
		return errors.New("LYNX_ENCRYPTION_KEY must be 64 hexadecimal characters in production")
	}
	publicURL, err := url.ParseRequestURI(c.PublicURL)
	if err != nil || publicURL.Scheme != "https" || publicURL.Host == "" {
		return errors.New("LYNX_PUBLIC_URL must be an HTTPS URL in production")
	}
	return nil
}
