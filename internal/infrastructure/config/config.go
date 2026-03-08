package config

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"
)

type Config struct {
	AppEnv           string
	HTTPPort         string
	PostgresDSN      string
	JWTIssuer        string
	JWTSecret        string
	AccessTokenTTL   time.Duration
	RefreshTokenTTL  time.Duration
	LocalStoragePath string
}

func Load() (Config, error) {
	cfg := Config{
		AppEnv:           getEnv("APP_ENV", "development"),
		HTTPPort:         getEnv("HTTP_PORT", "8080"),
		PostgresDSN:      os.Getenv("POSTGRES_DSN"),
		JWTIssuer:        getEnv("JWT_ISSUER", "note-app"),
		JWTSecret:        os.Getenv("JWT_SECRET"),
		LocalStoragePath: getEnv("LOCAL_STORAGE_PATH", "./tmp/storage"),
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

	if strings.TrimSpace(cfg.PostgresDSN) == "" {
		return Config{}, fmt.Errorf("POSTGRES_DSN is required")
	}
	if strings.TrimSpace(cfg.JWTSecret) == "" {
		return Config{}, fmt.Errorf("JWT_SECRET is required")
	}
	if len(cfg.JWTSecret) < 16 {
		return Config{}, fmt.Errorf("JWT_SECRET must be at least 16 characters")
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
