package config

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func chdirTemp(t *testing.T) string {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get cwd: %v", err)
	}
	tmp := t.TempDir()
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(cwd)
	})
	return tmp
}

func TestLoad(t *testing.T) {
	chdirTemp(t)
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
	chdirTemp(t)
	t.Setenv("POSTGRES_DSN", "")
	t.Setenv("JWT_SECRET", "")
	os.Unsetenv("POSTGRES_DSN")
	os.Unsetenv("JWT_SECRET")

	if _, err := Load(); err == nil {
		t.Fatal("expected error when required env vars are missing")
	}
}

func TestLoadRequiresJWTSecret(t *testing.T) {
	chdirTemp(t)
	t.Setenv("POSTGRES_DSN", "postgres://test:test@localhost:5432/test?sslmode=disable")
	os.Unsetenv("JWT_SECRET")

	if _, err := Load(); err == nil || err.Error() != "JWT_SECRET is required" {
		t.Fatalf("expected JWT_SECRET required error, got %v", err)
	}
}

func TestLoadValidationAndLogLevel(t *testing.T) {
	chdirTemp(t)
	t.Setenv("POSTGRES_DSN", "postgres://test:test@localhost:5432/test?sslmode=disable")
	t.Setenv("JWT_SECRET", "short")
	if _, err := Load(); err == nil {
		t.Fatal("expected short JWT secret to fail")
	}

	t.Setenv("JWT_SECRET", "super-secret-token")
	t.Setenv("ACCESS_TOKEN_TTL", "bad")
	if _, err := Load(); err == nil {
		t.Fatal("expected invalid access token ttl to fail")
	}

	t.Setenv("ACCESS_TOKEN_TTL", "10m")
	t.Setenv("REFRESH_TOKEN_TTL", "bad")
	if _, err := Load(); err == nil {
		t.Fatal("expected invalid refresh token ttl to fail")
	}

	t.Setenv("REFRESH_TOKEN_TTL", "24h")
	t.Setenv("APP_ENV", "development")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("load valid config: %v", err)
	}
	if cfg.LogLevel() != slog.LevelDebug {
		t.Fatalf("expected debug level in development, got %v", cfg.LogLevel())
	}

	cfg.AppEnv = "production"
	if cfg.LogLevel() != slog.LevelInfo {
		t.Fatalf("expected info level in production, got %v", cfg.LogLevel())
	}
}

func TestGetEnv(t *testing.T) {
	t.Setenv("X_ENV_TEST", " value ")
	if got := getEnv("X_ENV_TEST", "fallback"); got != "value" {
		t.Fatalf("expected trimmed value, got %q", got)
	}

	t.Setenv("X_ENV_TEST", "")
	if got := getEnv("X_ENV_TEST", "fallback"); got != "fallback" {
		t.Fatalf("expected fallback, got %q", got)
	}
}

func TestLoadReadsDotEnvFile(t *testing.T) {
	tmp := chdirTemp(t)
	os.Unsetenv("POSTGRES_DSN")
	os.Unsetenv("JWT_SECRET")

	dotenv := "POSTGRES_DSN=postgres://from-file:pass@localhost:5432/filedb?sslmode=disable\nJWT_SECRET=from-file-secret-123\n"
	if err := os.WriteFile(filepath.Join(tmp, ".env"), []byte(dotenv), 0o600); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.PostgresDSN != "postgres://from-file:pass@localhost:5432/filedb?sslmode=disable" {
		t.Fatalf("unexpected dsn from .env: %s", cfg.PostgresDSN)
	}
	if cfg.JWTSecret != "from-file-secret-123" {
		t.Fatalf("unexpected jwt secret from .env: %s", cfg.JWTSecret)
	}
}

func TestLoadDotEnvDoesNotOverrideExistingEnv(t *testing.T) {
	tmp := chdirTemp(t)
	t.Setenv("POSTGRES_DSN", "postgres://from-env:pass@localhost:5432/envdb?sslmode=disable")
	t.Setenv("JWT_SECRET", "from-env-secret-123")

	dotenv := "POSTGRES_DSN=postgres://from-file:pass@localhost:5432/filedb?sslmode=disable\nJWT_SECRET=from-file-secret-123\n"
	if err := os.WriteFile(filepath.Join(tmp, ".env"), []byte(dotenv), 0o600); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.PostgresDSN != "postgres://from-env:pass@localhost:5432/envdb?sslmode=disable" {
		t.Fatalf("expected env dsn to win, got %s", cfg.PostgresDSN)
	}
	if cfg.JWTSecret != "from-env-secret-123" {
		t.Fatalf("expected env secret to win, got %s", cfg.JWTSecret)
	}
}

func TestLoadPropagatesDotEnvReadError(t *testing.T) {
	chdirTemp(t)
	if err := os.Mkdir(".env", 0o755); err != nil {
		t.Fatalf("create .env dir: %v", err)
	}

	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "load .env") {
		t.Fatalf("expected load .env error, got %v", err)
	}
}

func TestLoadDotEnvParserBranches(t *testing.T) {
	tmp := chdirTemp(t)
	t.Setenv("EXISTING", "from-env")
	os.Unsetenv("EXPORTED_KEY")
	os.Unsetenv("QUOTED")
	os.Unsetenv("SINGLE")
	os.Unsetenv("MISMATCH")

	dotenv := "# comment\n\nexport EXPORTED_KEY=exported\nNO_EQUALS\n=missing\nQUOTED=\"quoted value\"\nSINGLE='single value'\nMISMATCH=\"abc\nEXISTING=from-file\n"
	if err := os.WriteFile(filepath.Join(tmp, ".env"), []byte(dotenv), 0o600); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	if err := loadDotEnv(".env"); err != nil {
		t.Fatalf("loadDotEnv() error = %v", err)
	}

	if got := os.Getenv("EXPORTED_KEY"); got != "exported" {
		t.Fatalf("unexpected EXPORTED_KEY: %q", got)
	}
	if got := os.Getenv("QUOTED"); got != "quoted value" {
		t.Fatalf("unexpected QUOTED: %q", got)
	}
	if got := os.Getenv("SINGLE"); got != "single value" {
		t.Fatalf("unexpected SINGLE: %q", got)
	}
	if got := os.Getenv("MISMATCH"); got != "\"abc" {
		t.Fatalf("unexpected MISMATCH: %q", got)
	}
	if got := os.Getenv("EXISTING"); got != "from-env" {
		t.Fatalf("expected EXISTING from env, got %q", got)
	}
}

func TestLoadDotEnvOpenError(t *testing.T) {
	if err := loadDotEnv("\x00"); err == nil {
		t.Fatal("expected loadDotEnv open error")
	}
}
