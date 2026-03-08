package config

import (
	"os"
	"testing"
)

func TestLoad(t *testing.T) {
	t.Setenv("POSTGRES_DSN", "postgres://test:test@localhost:5432/test?sslmode=disable")
	t.Setenv("JWT_SECRET", "super-secret-token")
	t.Setenv("ACCESS_TOKEN_TTL", "10m")
	t.Setenv("REFRESH_TOKEN_TTL", "24h")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.HTTPPort != "8080" {
		t.Fatalf("expected default port 8080, got %s", cfg.HTTPPort)
	}
}

func TestLoadRequiresSecrets(t *testing.T) {
	t.Setenv("POSTGRES_DSN", "")
	t.Setenv("JWT_SECRET", "")
	os.Unsetenv("POSTGRES_DSN")
	os.Unsetenv("JWT_SECRET")

	if _, err := Load(); err == nil {
		t.Fatal("expected error when required env vars are missing")
	}
}
