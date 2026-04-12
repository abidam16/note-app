package testenv

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolvePostgresDSN(t *testing.T) {
	tempDir := t.TempDir()
	envPath := filepath.Join(tempDir, ".env")
	if err := os.WriteFile(envPath, []byte("POSTGRES_DSN=postgres://from-dot-env\nTEST_POSTGRES_DSN=postgres://from-test-dot-env\n"), 0o600); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	t.Run("prefers process env", func(t *testing.T) {
		t.Setenv("POSTGRES_DSN", "postgres://from-process")
		dsn, err := ResolvePostgresDSN(tempDir)
		if err != nil {
			t.Fatalf("ResolvePostgresDSN() error = %v", err)
		}
		if dsn != "postgres://from-process" {
			t.Fatalf("expected process env dsn, got %q", dsn)
		}
	})

	t.Run("prefers explicit test env", func(t *testing.T) {
		t.Setenv("POSTGRES_DSN", "postgres://from-process")
		t.Setenv("TEST_POSTGRES_DSN", "postgres://from-test-process")
		dsn, err := ResolvePostgresDSN(tempDir)
		if err != nil {
			t.Fatalf("ResolvePostgresDSN() error = %v", err)
		}
		if dsn != "postgres://from-test-process" {
			t.Fatalf("expected test process env dsn, got %q", dsn)
		}
	})

	t.Run("uses dot env when process env missing", func(t *testing.T) {
		dsn, err := ResolvePostgresDSN(tempDir)
		if err != nil {
			t.Fatalf("ResolvePostgresDSN() error = %v", err)
		}
		if dsn != "postgres://from-test-dot-env" {
			t.Fatalf("expected test dot env dsn, got %q", dsn)
		}
	})

	t.Run("prefers .env.test over .env", func(t *testing.T) {
		envTestPath := filepath.Join(tempDir, ".env.test")
		if err := os.WriteFile(envTestPath, []byte("TEST_POSTGRES_DSN=postgres://from-dot-env-test\n"), 0o600); err != nil {
			t.Fatalf("write .env.test: %v", err)
		}

		dsn, err := ResolvePostgresDSN(tempDir)
		if err != nil {
			t.Fatalf("ResolvePostgresDSN() error = %v", err)
		}
		if dsn != "postgres://from-dot-env-test" {
			t.Fatalf("expected .env.test dsn, got %q", dsn)
		}
	})

	t.Run("uses TEST_ENV_FILE override", func(t *testing.T) {
		overridePath := filepath.Join(tempDir, "custom.env")
		if err := os.WriteFile(overridePath, []byte("TEST_POSTGRES_DSN=postgres://from-custom-env-file\n"), 0o600); err != nil {
			t.Fatalf("write custom env file: %v", err)
		}
		t.Setenv("TEST_ENV_FILE", "custom.env")

		dsn, err := ResolvePostgresDSN(tempDir)
		if err != nil {
			t.Fatalf("ResolvePostgresDSN() error = %v", err)
		}
		if dsn != "postgres://from-custom-env-file" {
			t.Fatalf("expected custom env file dsn, got %q", dsn)
		}
	})

	t.Run("falls back to local default", func(t *testing.T) {
		emptyDir := t.TempDir()
		dsn, err := ResolvePostgresDSN(emptyDir)
		if err != nil {
			t.Fatalf("ResolvePostgresDSN() error = %v", err)
		}
		if dsn != DefaultLocalPostgresDSN {
			t.Fatalf("expected default local dsn, got %q", dsn)
		}
	})
}
