package config

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

type Config struct {
	AppEnv             string
	HTTPPort           string
	PostgresDSN        string
	JWTIssuer          string
	JWTSecret          string
	AccessTokenTTL     time.Duration
	RefreshTokenTTL    time.Duration
	LocalStoragePath   string
	CORSAllowedOrigins []string
	TrustProxyHeaders  bool
	TrustedProxyCIDRs  []string
}

func Load() (Config, error) {
	return LoadFromEnvFile(".env")
}

func LoadFromEnvFile(path string) (Config, error) {
	envPath := strings.TrimSpace(path)
	if envPath == "" {
		envPath = ".env"
	}
	if err := loadDotEnv(envPath); err != nil {
		return Config{}, fmt.Errorf("load %s: %w", envPath, err)
	}

	cfg := Config{
		AppEnv:             getEnv("APP_ENV", "development"),
		HTTPPort:           getEnv("PORT", getEnv("HTTP_PORT", "8080")),
		PostgresDSN:        getEnv("POSTGRES_DSN", getEnv("DATABASE_URL", "")),
		JWTIssuer:          getEnv("JWT_ISSUER", "note-app"),
		JWTSecret:          os.Getenv("JWT_SECRET"),
		LocalStoragePath:   getEnv("LOCAL_STORAGE_PATH", "./tmp/storage"),
		CORSAllowedOrigins: splitCSVEnv("CORS_ALLOWED_ORIGINS"),
		TrustedProxyCIDRs:  splitCSVEnv("TRUSTED_PROXY_CIDRS"),
	}

	var err error
	cfg.AccessTokenTTL, err = time.ParseDuration(getEnv("ACCESS_TOKEN_TTL", "15m"))
	if err != nil {
		return Config{}, fmt.Errorf("parse ACCESS_TOKEN_TTL: %w", err)
	}

	cfg.RefreshTokenTTL, err = time.ParseDuration(getEnv("REFRESH_TOKEN_TTL", "168h"))
	if err != nil {
		return Config{}, fmt.Errorf("parse REFRESH_TOKEN_TTL: %w", err)
	}

	cfg.TrustProxyHeaders, err = parseEnvBool("TRUST_PROXY_HEADERS", false)
	if err != nil {
		return Config{}, fmt.Errorf("parse TRUST_PROXY_HEADERS: %w", err)
	}

	if strings.TrimSpace(cfg.PostgresDSN) == "" {
		return Config{}, fmt.Errorf("POSTGRES_DSN or DATABASE_URL is required")
	}
	if strings.TrimSpace(cfg.JWTSecret) == "" {
		return Config{}, fmt.Errorf("JWT_SECRET is required")
	}
	if len(cfg.JWTSecret) < 32 {
		return Config{}, fmt.Errorf("JWT_SECRET must be at least 32 characters")
	}
	if cfg.TrustProxyHeaders && len(cfg.TrustedProxyCIDRs) == 0 {
		return Config{}, fmt.Errorf("TRUSTED_PROXY_CIDRS is required when TRUST_PROXY_HEADERS is enabled")
	}
	for _, cidr := range cfg.TrustedProxyCIDRs {
		if _, _, err := net.ParseCIDR(cidr); err != nil {
			return Config{}, fmt.Errorf("invalid TRUSTED_PROXY_CIDRS entry %q", cidr)
		}
	}
	if err := validatePostgresDSNSecurity(cfg.AppEnv, cfg.PostgresDSN); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func (c Config) LogLevel() slog.Level {
	if strings.EqualFold(c.AppEnv, "development") {
		return slog.LevelDebug
	}
	return slog.LevelInfo
}

func getEnv(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func parseEnvBool(key string, fallback bool) (bool, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		return false, err
	}
	return value, nil
}

func splitCSVEnv(key string) []string {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		values = append(values, part)
	}
	return values
}

func validatePostgresDSNSecurity(appEnv, dsn string) error {
	if !strings.EqualFold(strings.TrimSpace(appEnv), "production") {
		return nil
	}

	cfg, err := pgconn.ParseConfig(dsn)
	if err != nil {
		return fmt.Errorf("parse POSTGRES_DSN: %w", err)
	}
	if cfg.TLSConfig == nil {
		return fmt.Errorf("production POSTGRES_DSN must enable TLS")
	}
	for _, fallback := range cfg.Fallbacks {
		if fallback != nil && fallback.TLSConfig == nil {
			return fmt.Errorf("production POSTGRES_DSN must not allow plaintext fallback")
		}
	}
	if isSupabaseTransactionPoolConfig(cfg) {
		return fmt.Errorf("production Supabase DATABASE_URL must not use transaction pool mode (:6543); use direct mode or Supavisor session mode (:5432) for persistent API traffic")
	}
	return nil
}

func isSupabaseTransactionPoolConfig(cfg *pgconn.Config) bool {
	if cfg == nil {
		return false
	}
	host := strings.ToLower(strings.TrimSpace(cfg.Host))
	return strings.HasSuffix(host, ".pooler.supabase.com") && uint16(cfg.Port) == 6543
}

func loadDotEnv(path string) error {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) || errors.Is(err, fs.ErrNotExist) || errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if strings.HasPrefix(line, "export ") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		if key == "" {
			continue
		}
		if _, exists := os.LookupEnv(key); exists {
			continue
		}

		value := strings.TrimSpace(parts[1])
		if len(value) >= 2 {
			if (value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'') {
				value = value[1 : len(value)-1]
			}
		}

		if err := os.Setenv(key, value); err != nil {
			return err
		}
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	return nil
}
