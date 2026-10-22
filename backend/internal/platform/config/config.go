package config

import "github.com/caarlos0/env/v11"

type Config struct {
	HTTPAddr      string `env:"LYNX_HTTP_ADDR" envDefault:":8082"`
	DatabaseURL   string `env:"DATABASE_URL,required"`
	RedisURL      string `env:"REDIS_URL,required"`
	Environment   string `env:"LYNX_ENV" envDefault:"development"`
	ReadTimeout   int    `env:"LYNX_HTTP_READ_TIMEOUT_SECONDS" envDefault:"10"`
	WriteTimeout  int    `env:"LYNX_HTTP_WRITE_TIMEOUT_SECONDS" envDefault:"30"`
	AdminAPIKey   string `env:"LYNX_ADMIN_API_KEY"`
	EncryptionKey string `env:"LYNX_ENCRYPTION_KEY"`
}

func Load() (Config, error) {
	var cfg Config
	if err := env.Parse(&cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}
