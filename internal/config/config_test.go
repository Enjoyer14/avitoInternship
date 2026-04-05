package config

import (
	"os"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	_ = os.Unsetenv("APP_PORT")
	_ = os.Unsetenv("DATABASE_URL")
	_ = os.Unsetenv("JWT_SECRET")

	cfg := Load()
	if cfg.Port != "8080" {
		t.Fatalf("unexpected port: %s", cfg.Port)
	}
	if cfg.DatabaseURL == "" {
		t.Fatal("database url should not be empty")
	}
	if cfg.JWTSecret == "" {
		t.Fatal("jwt secret should not be empty")
	}
}

func TestLoadFromEnv(t *testing.T) {
	t.Setenv("APP_PORT", "9000")
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("JWT_SECRET", "abc")

	cfg := Load()
	if cfg.Port != "9000" || cfg.DatabaseURL != "postgres://x" || cfg.JWTSecret != "abc" {
		t.Fatalf("unexpected config: %#v", cfg)
	}
}
