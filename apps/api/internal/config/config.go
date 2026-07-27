// Package config loads every runtime setting from the environment and is the one
// place operational thresholds live. Business logic reads them from here, never
// from a literal.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// minJWTSecretLength is the floor for AUTH_JWT_SECRET. HMAC-SHA256 keys shorter
// than the digest add no security.
const minJWTSecretLength = 32

// Environment is the deployment environment the process runs in.
type Environment string

const (
	EnvironmentDevelopment Environment = "development"
	EnvironmentProduction  Environment = "production"
)

// Config is the fully resolved runtime configuration.
type Config struct {
	Environment Environment
	LogLevel    string
	Server      ServerConfig
	Database    DatabaseConfig
	Auth        AuthConfig
}

// ServerConfig holds the HTTP listener settings.
type ServerConfig struct {
	Port            string
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	ShutdownTimeout time.Duration
}

// DatabaseConfig holds both connection strings and the pool sizing shared by them.
//
// URL is the restricted, RLS-subject role used for every request-scoped query.
// AdminURL is the owner role, and only three things may use it: migrations, the
// follow-up cron, and the pre-auth lookups that cannot know the account yet
// (login by email, resolving a public quote token).
type DatabaseConfig struct {
	URL             string
	AdminURL        string
	MaxConns        int32
	MinConns        int32
	MaxConnLifetime time.Duration
	MaxConnIdleTime time.Duration
	ConnectTimeout  time.Duration
}

// AuthConfig holds the token settings. The access token is short-lived; the
// refresh token is single-use and rotates within a family.
type AuthConfig struct {
	JWTSecret          string
	AccessTTL          time.Duration
	RefreshTTL         time.Duration
	RefreshRememberTTL time.Duration
	RefreshReuseGrace  time.Duration
	MaxFailedAttempts  int
	LockoutDuration    time.Duration
}

// Load resolves the configuration from the environment, applying defaults for
// everything optional. Returns every validation problem at once rather than the
// first, so a misconfigured deploy is diagnosed in one pass.
func Load() (*Config, error) {
	var problems []string

	env := Environment(getString("ENV", string(EnvironmentDevelopment)))
	if env != EnvironmentDevelopment && env != EnvironmentProduction {
		problems = append(problems, fmt.Sprintf("ENV must be %q or %q, got %q",
			EnvironmentDevelopment, EnvironmentProduction, env))
	}

	cfg := &Config{
		Environment: env,
		LogLevel:    getString("LOG_LEVEL", "info"),
		Server: ServerConfig{
			Port:            getString("API_PORT", "8000"),
			ReadTimeout:     getDuration("SERVER_READ_TIMEOUT_SECONDS", 15*time.Second, &problems),
			WriteTimeout:    getDuration("SERVER_WRITE_TIMEOUT_SECONDS", 30*time.Second, &problems),
			ShutdownTimeout: getDuration("SERVER_SHUTDOWN_TIMEOUT_SECONDS", 10*time.Second, &problems),
		},
		Database: DatabaseConfig{
			URL:             os.Getenv("DATABASE_URL"),
			AdminURL:        os.Getenv("DATABASE_ADMIN_URL"),
			MaxConns:        int32(getInt("DB_MAX_CONNS", 10, &problems)),
			MinConns:        int32(getInt("DB_MIN_CONNS", 2, &problems)),
			MaxConnLifetime: getDuration("DB_MAX_CONN_LIFETIME_MINUTES", 60*time.Minute, &problems),
			MaxConnIdleTime: getDuration("DB_MAX_CONN_IDLE_MINUTES", 30*time.Minute, &problems),
			ConnectTimeout:  getDuration("DB_CONNECT_TIMEOUT_SECONDS", 10*time.Second, &problems),
		},
		Auth: AuthConfig{
			JWTSecret:          os.Getenv("AUTH_JWT_SECRET"),
			AccessTTL:          getDuration("AUTH_ACCESS_TTL_MINUTES", 15*time.Minute, &problems),
			RefreshTTL:         getDuration("AUTH_REFRESH_TTL_HOURS", 12*time.Hour, &problems),
			RefreshRememberTTL: getDuration("AUTH_REFRESH_REMEMBER_DAYS", 30*24*time.Hour, &problems),
			RefreshReuseGrace:  getDuration("AUTH_REFRESH_REUSE_GRACE_SECONDS", 30*time.Second, &problems),
			MaxFailedAttempts:  getInt("AUTH_MAX_FAILED_ATTEMPTS", 5, &problems),
			LockoutDuration:    getDuration("AUTH_LOCKOUT_MINUTES", 15*time.Minute, &problems),
		},
	}

	if cfg.Database.URL == "" {
		problems = append(problems, "DATABASE_URL is required")
	}
	if cfg.Database.AdminURL == "" {
		problems = append(problems, "DATABASE_ADMIN_URL is required")
	}
	if cfg.Database.MinConns > cfg.Database.MaxConns {
		problems = append(problems, fmt.Sprintf("DB_MIN_CONNS (%d) exceeds DB_MAX_CONNS (%d)",
			cfg.Database.MinConns, cfg.Database.MaxConns))
	}
	if len(cfg.Auth.JWTSecret) < minJWTSecretLength {
		problems = append(problems, fmt.Sprintf("AUTH_JWT_SECRET must be at least %d characters, got %d",
			minJWTSecretLength, len(cfg.Auth.JWTSecret)))
	}

	// A production deploy pointing the request pool at the owner role would silently
	// bypass every row level security policy.
	if cfg.Environment == EnvironmentProduction && cfg.Database.URL == cfg.Database.AdminURL {
		problems = append(problems, "DATABASE_URL must differ from DATABASE_ADMIN_URL in production: "+
			"the request pool has to use the restricted role or RLS is bypassed")
	}

	if len(problems) > 0 {
		return nil, fmt.Errorf("invalid configuration:\n  - %s", strings.Join(problems, "\n  - "))
	}
	return cfg, nil
}

// IsProduction reports whether the process runs with production settings.
func (c *Config) IsProduction() bool {
	return c.Environment == EnvironmentProduction
}

func getString(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func getInt(key string, fallback int, problems *[]string) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		*problems = append(*problems, fmt.Sprintf("%s must be an integer, got %q", key, raw))
		return fallback
	}
	return v
}

// getDuration reads a plain number whose unit comes from the key's suffix, so the
// env file reads as SECONDS/MINUTES/HOURS/DAYS instead of a Go duration string.
func getDuration(key string, fallback time.Duration, problems *[]string) time.Duration {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		*problems = append(*problems, fmt.Sprintf("%s must be an integer, got %q", key, raw))
		return fallback
	}
	unit, err := unitFor(key)
	if err != nil {
		*problems = append(*problems, err.Error())
		return fallback
	}
	return time.Duration(n) * unit
}

func unitFor(key string) (time.Duration, error) {
	switch {
	case strings.HasSuffix(key, "_SECONDS"):
		return time.Second, nil
	case strings.HasSuffix(key, "_MINUTES"):
		return time.Minute, nil
	case strings.HasSuffix(key, "_HOURS"):
		return time.Hour, nil
	case strings.HasSuffix(key, "_DAYS"):
		return 24 * time.Hour, nil
	default:
		return 0, errors.New(key + " needs a _SECONDS, _MINUTES, _HOURS or _DAYS suffix to carry its unit")
	}
}
