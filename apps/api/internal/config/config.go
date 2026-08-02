// Package config loads every runtime setting from the environment and is the one
// place operational thresholds live. Business logic reads them from here, never
// from a literal.
package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// minJWTSecretLength is the floor for AUTH_JWT_SECRET. HMAC-SHA256 keys shorter
// than the digest add no security.
const minJWTSecretLength = 32
const defaultPriceImportMaxBytes = 5 * 1024 * 1024

// Environment is the deployment environment the process runs in.
type Environment string

const (
	EnvironmentDevelopment Environment = "development"
	EnvironmentProduction  Environment = "production"
)

// MailProvider selects the transport behind the domain.Mailer port.
type MailProvider string

const (
	MailProviderConsole MailProvider = "console"
	MailProviderSMTP    MailProvider = "smtp"
)

// Config is the fully resolved runtime configuration.
type Config struct {
	Environment Environment
	LogLevel    string
	Server      ServerConfig
	Database    DatabaseConfig
	Auth        AuthConfig
	Mail        MailConfig
	Web         WebConfig
	PriceImport PriceImportConfig
	Catalog     CatalogConfig
	Branch      BranchConfig
}

// ServerConfig holds the HTTP listener settings.
type ServerConfig struct {
	Port            string
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	ShutdownTimeout time.Duration
}

// DatabaseConfig holds both connection strings and the pool sizing shared by them. URL is
// the restricted, RLS-subject role; AdminURL is the owner role, for migrations, the
// follow-up cron, and the pre-auth lookups that cannot know the account yet.
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
	PasswordMinLength  int
	PasswordResetTTL   time.Duration
}

// MailConfig holds the outbound-mail transport settings. The SMTP fields are the contract an
// SMTP adapter will read; until one exists, selecting that provider is a startup error.
type MailConfig struct {
	Provider     MailProvider
	FromAddress  string
	FromName     string
	SMTPHost     string
	SMTPPort     int
	SMTPUsername string
	SMTPPassword string
}

// WebConfig holds the frontend base URLs an emailed link points at. The API never serves
// those routes, so it cannot derive them from its own address.
type WebConfig struct {
	BackofficeURL string
}

// PriceImportConfig holds operational limits for spreadsheet imports.
type PriceImportConfig struct {
	MaxBytes int64
}

// BranchConfig holds the defaults a newly opened branch starts with.
type BranchConfig struct {
	DefaultExpiryDays int
}

// CatalogConfig holds the catalog listing limits. The cap is what stops a client from
// asking for the whole catalog in one response.
type CatalogConfig struct {
	DefaultPageSize int
	MaxPageSize     int
}

// Load resolves the configuration from the environment, applying defaults for everything
// optional. It reports every validation problem at once, not the first.
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
			PasswordMinLength:  getInt("AUTH_PASSWORD_MIN_LENGTH", 8, &problems),
			PasswordResetTTL:   getDuration("AUTH_PASSWORD_RESET_TTL_MINUTES", 60*time.Minute, &problems),
		},
		Mail: MailConfig{
			Provider:     MailProvider(getString("MAIL_PROVIDER", string(MailProviderConsole))),
			FromAddress:  getString("MAIL_FROM_ADDRESS", ""),
			FromName:     getString("MAIL_FROM_NAME", "Coti"),
			SMTPHost:     getString("MAIL_SMTP_HOST", ""),
			SMTPPort:     getInt("MAIL_SMTP_PORT", 587, &problems),
			SMTPUsername: getString("MAIL_SMTP_USERNAME", ""),
			SMTPPassword: getString("MAIL_SMTP_PASSWORD", ""),
		},
		Web: WebConfig{
			BackofficeURL: getString("WEB_BACKOFFICE_URL", "http://localhost:3000"),
		},
		Catalog: CatalogConfig{
			DefaultPageSize: getInt("CATALOG_DEFAULT_PAGE_SIZE", 50, &problems),
			MaxPageSize:     getInt("CATALOG_MAX_PAGE_SIZE", 200, &problems),
		},
		Branch: BranchConfig{
			DefaultExpiryDays: getInt("BRANCH_DEFAULT_EXPIRY_DAYS", 7, &problems),
		},
		PriceImport: PriceImportConfig{
			MaxBytes: int64(getInt("PRICE_IMPORT_MAX_BYTES", defaultPriceImportMaxBytes, &problems)),
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
	if cfg.Branch.DefaultExpiryDays <= 0 {
		problems = append(problems, "BRANCH_DEFAULT_EXPIRY_DAYS must be greater than zero")
	}
	if cfg.PriceImport.MaxBytes <= 0 {
		problems = append(problems, "PRICE_IMPORT_MAX_BYTES must be greater than zero")
	}

	if cfg.Auth.PasswordMinLength < 8 {
		problems = append(problems, fmt.Sprintf("AUTH_PASSWORD_MIN_LENGTH must be at least 8, got %d",
			cfg.Auth.PasswordMinLength))
	}
	if cfg.Auth.PasswordResetTTL <= 0 {
		problems = append(problems, "AUTH_PASSWORD_RESET_TTL_MINUTES must be greater than zero")
	}

	// The console transport reaches nothing, so it needs no sender of its own; a real one
	// cannot start without the address and credentials it authenticates with.
	switch cfg.Mail.Provider {
	case MailProviderConsole:
		if cfg.Mail.FromAddress == "" {
			cfg.Mail.FromAddress = "no-reply@coti.local"
		}
	case MailProviderSMTP:
		// Reported here rather than at the composition root so an operator sees it alongside
		// whatever else is missing, instead of one restart per problem.
		problems = append(problems, "MAIL_PROVIDER is "+string(MailProviderSMTP)+
			", which has no adapter wired yet: the only working transport is "+
			string(MailProviderConsole))
		required := []struct{ key, value string }{
			{"MAIL_FROM_ADDRESS", cfg.Mail.FromAddress},
			{"MAIL_SMTP_HOST", cfg.Mail.SMTPHost},
			{"MAIL_SMTP_USERNAME", cfg.Mail.SMTPUsername},
			{"MAIL_SMTP_PASSWORD", cfg.Mail.SMTPPassword},
		}
		for _, r := range required {
			if r.value == "" {
				problems = append(problems, r.key+" is required when MAIL_PROVIDER is "+
					string(MailProviderSMTP))
			}
		}
		if cfg.Mail.SMTPPort <= 0 {
			problems = append(problems, fmt.Sprintf("MAIL_SMTP_PORT must be greater than zero, got %d",
				cfg.Mail.SMTPPort))
		}
	default:
		problems = append(problems, fmt.Sprintf("MAIL_PROVIDER must be %q or %q, got %q",
			MailProviderConsole, MailProviderSMTP, cfg.Mail.Provider))
	}

	// A base URL missing its scheme or host yields recovery links that go nowhere, and the
	// only symptom is a user reporting that the mail does not work.
	if u, err := url.Parse(cfg.Web.BackofficeURL); err != nil || u.Scheme == "" || u.Host == "" {
		problems = append(problems, fmt.Sprintf(
			"WEB_BACKOFFICE_URL must be an absolute URL with a scheme and host, got %q",
			cfg.Web.BackofficeURL))
	}

	if cfg.Catalog.DefaultPageSize < 1 {
		problems = append(problems, fmt.Sprintf("CATALOG_DEFAULT_PAGE_SIZE must be at least 1, got %d",
			cfg.Catalog.DefaultPageSize))
	}
	if cfg.Catalog.DefaultPageSize > cfg.Catalog.MaxPageSize {
		problems = append(problems, fmt.Sprintf("CATALOG_DEFAULT_PAGE_SIZE (%d) exceeds CATALOG_MAX_PAGE_SIZE (%d)",
			cfg.Catalog.DefaultPageSize, cfg.Catalog.MaxPageSize))
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
