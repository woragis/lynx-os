package config

import (
	"strings"
	"testing"
)

func validProductionConfig() Config {
	return Config{
		Environment:   "production",
		JWTSigningKey: strings.Repeat("j", 32),
		EncryptionKey: strings.Repeat("a", 64),
		PublicURL:     "https://lynx.example",
		ReadTimeout:   10,
		WriteTimeout:  30,
	}
}

func TestProductionConfigRequiresSecurityKeysAndHTTPS(t *testing.T) {
	tests := []struct {
		name   string
		change func(*Config)
	}{
		{name: "jwt signing key", change: func(c *Config) { c.JWTSigningKey = "" }},
		{name: "encryption key", change: func(c *Config) { c.EncryptionKey = "invalid" }},
		{name: "https public url", change: func(c *Config) { c.PublicURL = "http://lynx.example" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := validProductionConfig()
			test.change(&config)
			if err := config.Validate(); err == nil {
				t.Fatal("expected unsafe production configuration to fail")
			}
		})
	}
	if err := validProductionConfig().Validate(); err != nil {
		t.Fatalf("valid production configuration rejected: %v", err)
	}
}

func TestDevelopmentCanOmitProductionSecrets(t *testing.T) {
	config := Config{Environment: "development", ReadTimeout: 10, WriteTimeout: 30}
	if err := config.Validate(); err != nil {
		t.Fatalf("development configuration rejected: %v", err)
	}
}
