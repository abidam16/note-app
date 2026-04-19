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
	t.Setenv("JWT_SECRET", "0123456789abcdef0123456789abcdef")
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
	t.Setenv("DATABASE_URL", "")
	t.Setenv("JWT_SECRET", "")
	os.Unsetenv("POSTGRES_DSN")
	os.Unsetenv("DATABASE_URL")
	os.Unsetenv("JWT_SECRET")

	if _, err := Load(); err == nil || err.Error() != "POSTGRES_DSN or DATABASE_URL is required" {
		t.Fatalf("expected missing postgres env error, got %v", err)
	}
}

func TestLoadRequiresJWTSecret(t *testing.T) {
	chdirTemp(t)
	t.Setenv("POSTGRES_DSN", "postgres://test:test@localhost:5432/test?sslmode=disable")
	os.Unsetenv("DATABASE_URL")
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

	t.Setenv("JWT_SECRET", strings.Repeat("a", 31))
	if _, err := Load(); err == nil {
		t.Fatal("expected JWT secret shorter than 32 chars to fail")
	}

	t.Setenv("JWT_SECRET", strings.Repeat("a", 32))
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
	t.Setenv("TRUST_PROXY_HEADERS", "not-bool")
	if _, err := Load(); err == nil {
		t.Fatal("expected invalid trust proxy bool to fail")
	}

	t.Setenv("TRUST_PROXY_HEADERS", "true")
	os.Unsetenv("TRUSTED_PROXY_CIDRS")
	if _, err := Load(); err == nil {
		t.Fatal("expected missing trusted proxy cidrs to fail when proxy trust is enabled")
	}

	t.Setenv("TRUSTED_PROXY_CIDRS", "bad-cidr")
	if _, err := Load(); err == nil {
		t.Fatal("expected invalid trusted proxy cidr to fail")
	}

	t.Setenv("REFRESH_TOKEN_TTL", "24h")
	t.Setenv("TRUST_PROXY_HEADERS", "false")
	t.Setenv("TRUSTED_PROXY_CIDRS", "10.0.0.0/8, 192.168.0.0/16")
	t.Setenv("APP_ENV", "development")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("load valid config: %v", err)
	}
	if cfg.LogLevel() != slog.LevelDebug {
		t.Fatalf("expected debug level in development, got %v", cfg.LogLevel())
	}
	if cfg.TrustProxyHeaders {
		t.Fatalf("expected trust proxy headers false, got %+v", cfg)
	}
	if len(cfg.TrustedProxyCIDRs) != 2 {
		t.Fatalf("expected trusted proxy cidrs to be parsed, got %+v", cfg.TrustedProxyCIDRs)
	}
	if cfg.CORSAllowedOrigins != nil {
		t.Fatalf("expected nil cors allowed origins by default, got %+v", cfg.CORSAllowedOrigins)
	}

	cfg.AppEnv = "production"
	if cfg.LogLevel() != slog.LevelInfo {
		t.Fatalf("expected info level in production, got %v", cfg.LogLevel())
	}
}

func TestLoadParsesCORSAllowedOrigins(t *testing.T) {
	chdirTemp(t)
	t.Setenv("POSTGRES_DSN", "postgres://test:test@localhost:5432/test?sslmode=disable")
	t.Setenv("JWT_SECRET", strings.Repeat("a", 32))
	t.Setenv("CORS_ALLOWED_ORIGINS", " http://localhost:5173 , https://frontend.example.com ")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if len(cfg.CORSAllowedOrigins) != 2 {
		t.Fatalf("expected two cors origins, got %+v", cfg.CORSAllowedOrigins)
	}
	if cfg.CORSAllowedOrigins[0] != "http://localhost:5173" || cfg.CORSAllowedOrigins[1] != "https://frontend.example.com" {
		t.Fatalf("unexpected cors origins: %+v", cfg.CORSAllowedOrigins)
	}
}

func TestLoadRejectsInsecureProductionPostgresDSN(t *testing.T) {
	chdirTemp(t)
	t.Setenv("JWT_SECRET", strings.Repeat("a", 32))
	t.Setenv("ACCESS_TOKEN_TTL", "10m")
	t.Setenv("REFRESH_TOKEN_TTL", "24h")
	t.Setenv("APP_ENV", "production")

	t.Setenv("POSTGRES_DSN", "postgres://test:test@db.internal:5432/test?sslmode=disable")
	if _, err := Load(); err == nil || err.Error() != "production POSTGRES_DSN must enable TLS" {
		t.Fatalf("expected production tls requirement, got %v", err)
	}

	t.Setenv("POSTGRES_DSN", "postgres://test:test@db.internal:5432/test?sslmode=prefer")
	if _, err := Load(); err == nil || err.Error() != "production POSTGRES_DSN must not allow plaintext fallback" {
		t.Fatalf("expected production fallback rejection, got %v", err)
	}

	t.Setenv("POSTGRES_DSN", "postgres://test:test@db.internal:5432/test?sslmode=allow")
	if _, err := Load(); err == nil || err.Error() != "production POSTGRES_DSN must enable TLS" {
		t.Fatalf("expected production allow rejection, got %v", err)
	}

	t.Setenv("POSTGRES_DSN", "postgres://test:test@db.internal:5432/test?sslmode=require")
	if _, err := Load(); err != nil {
		t.Fatalf("expected production require tls dsn to pass, got %v", err)
	}
}

func TestParseEnvBoolAndSplitCSVEnv(t *testing.T) {
	t.Setenv("BOOL_TEST", "true")
	value, err := parseEnvBool("BOOL_TEST", false)
	if err != nil || !value {
		t.Fatalf("expected parseEnvBool true, got value=%t err=%v", value, err)
	}

	t.Setenv("BOOL_TEST", "broken")
	if _, err := parseEnvBool("BOOL_TEST", false); err == nil {
		t.Fatal("expected parseEnvBool invalid value to fail")
	}

	os.Unsetenv("CSV_TEST")
	if got := splitCSVEnv("CSV_TEST"); got != nil {
		t.Fatalf("expected nil for empty csv env, got %+v", got)
	}

	t.Setenv("CSV_TEST", "10.0.0.0/8, ,192.168.0.0/16")
	got := splitCSVEnv("CSV_TEST")
	if len(got) != 2 || got[0] != "10.0.0.0/8" || got[1] != "192.168.0.0/16" {
		t.Fatalf("unexpected split csv env result: %+v", got)
	}
}

func TestValidatePostgresDSNSecurity(t *testing.T) {
	if err := validatePostgresDSNSecurity("development", "postgres://test:test@localhost:5432/test?sslmode=disable"); err != nil {
		t.Fatalf("expected non-production dsn to skip strict validation, got %v", err)
	}

	if err := validatePostgresDSNSecurity("production", "::bad-dsn"); err == nil || !strings.Contains(err.Error(), "parse POSTGRES_DSN") {
		t.Fatalf("expected parse error for invalid production dsn, got %v", err)
	}

	if err := validatePostgresDSNSecurity("production", "postgres://postgres.projectref:pass@aws-0-us-east-1.pooler.supabase.com:6543/postgres?sslmode=require"); err == nil || err.Error() != "production Supabase DATABASE_URL must not use transaction pool mode (:6543); use direct mode or Supavisor session mode (:5432) for persistent API traffic" {
		t.Fatalf("expected supabase transaction pool rejection, got %v", err)
	}

	if err := validatePostgresDSNSecurity("production", "postgres://postgres.projectref:pass@aws-0-us-east-1.pooler.supabase.com:5432/postgres?sslmode=require"); err != nil {
		t.Fatalf("expected supabase session pool mode to pass, got %v", err)
	}

	if err := validatePostgresDSNSecurity("production", "postgres://postgres:pass@db.projectref.supabase.co:5432/postgres?sslmode=require"); err != nil {
		t.Fatalf("expected supabase direct connection to pass, got %v", err)
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

	dotenv := "POSTGRES_DSN=postgres://from-file:pass@localhost:5432/filedb?sslmode=disable\nJWT_SECRET=0123456789abcdef0123456789abcdef\n"
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
	if cfg.JWTSecret != "0123456789abcdef0123456789abcdef" {
		t.Fatalf("unexpected jwt secret from .env: %s", cfg.JWTSecret)
	}
}

func TestLoadFromEnvFileReadsExplicitPath(t *testing.T) {
	tmp := chdirTemp(t)
	os.Unsetenv("POSTGRES_DSN")
	os.Unsetenv("DATABASE_URL")
	os.Unsetenv("JWT_SECRET")

	dotenv := "POSTGRES_DSN=postgres://from-local-file:pass@localhost:5432/localdb?sslmode=disable\nJWT_SECRET=fedcba9876543210fedcba9876543210\n"
	envPath := filepath.Join(tmp, ".env.local")
	if err := os.WriteFile(envPath, []byte(dotenv), 0o600); err != nil {
		t.Fatalf("write .env.local: %v", err)
	}

	cfg, err := LoadFromEnvFile(".env.local")
	if err != nil {
		t.Fatalf("LoadFromEnvFile() error = %v", err)
	}
	if cfg.PostgresDSN != "postgres://from-local-file:pass@localhost:5432/localdb?sslmode=disable" {
		t.Fatalf("unexpected dsn from explicit env file: %s", cfg.PostgresDSN)
	}
	if cfg.JWTSecret != "fedcba9876543210fedcba9876543210" {
		t.Fatalf("unexpected jwt secret from explicit env file: %s", cfg.JWTSecret)
	}
}

func TestLoadUsesDatabaseURLFallback(t *testing.T) {
	chdirTemp(t)
	os.Unsetenv("POSTGRES_DSN")
	t.Setenv("DATABASE_URL", "postgres://from-db-url:pass@localhost:5432/dburl?sslmode=disable")
	t.Setenv("JWT_SECRET", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.PostgresDSN != "postgres://from-db-url:pass@localhost:5432/dburl?sslmode=disable" {
		t.Fatalf("expected DATABASE_URL fallback, got %s", cfg.PostgresDSN)
	}
}

func TestLoadDotEnvDoesNotOverrideExistingEnv(t *testing.T) {
	tmp := chdirTemp(t)
	t.Setenv("POSTGRES_DSN", "postgres://from-env:pass@localhost:5432/envdb?sslmode=disable")
	t.Setenv("JWT_SECRET", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")

	dotenv := "POSTGRES_DSN=postgres://from-file:pass@localhost:5432/filedb?sslmode=disable\nJWT_SECRET=0123456789abcdef0123456789abcdef\n"
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
	if cfg.JWTSecret != "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" {
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
