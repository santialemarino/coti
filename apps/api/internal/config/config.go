// Package config loads every runtime setting from the environment and is the one
// place operational thresholds live. Business logic reads them from here, never
// from a literal.
package config

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// minJWTSecretLength is the floor for AUTH_JWT_SECRET. HMAC-SHA256 keys shorter
// than the digest add no security.
const minJWTSecretLength = 32
const defaultCatalogImportMaxBytes = 5 * 1024 * 1024
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
	Environment   Environment
	LogLevel      string
	Server        ServerConfig
	Database      DatabaseConfig
	Auth          AuthConfig
	Mail          MailConfig
	Web           WebConfig
	Catalog       CatalogConfig
	RateLimit     RateLimitConfig
	Branch        BranchConfig
	CatalogImport SpreadsheetImportConfig
	PriceImport   SpreadsheetImportConfig
}

// RateLimitConfig holds the request allowances, all of them settings rather than literals.
type RateLimitConfig struct {
	Enabled bool
	Window  time.Duration
	// Global is the allowance for all of /v1. The rest are tighter, per surface.
	Global      int
	Credentials int
	Signup      int
	Mail        int
	// MailPerAddress is counted by target address instead of by caller, so it bounds what one
	// mailbox receives however many callers ask for it.
	MailPerAddress int
	// TrustedProxyHops is how many intermediaries sit in front of the API.
	TrustedProxyHops int
	// TrustedProxies are the peers whose forwarding header is believed at all.
	TrustedProxies []*net.IPNet
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
	JWTSecret            string
	AccessTTL            time.Duration
	RefreshTTL           time.Duration
	RefreshRememberTTL   time.Duration
	RefreshReuseGrace    time.Duration
	MaxFailedAttempts    int
	LockoutDuration      time.Duration
	PasswordMinLength    int
	PasswordResetTTL     time.Duration
	RequireVerifiedEmail bool
	VerificationTTL      time.Duration
}

// MailConfig holds the outbound-mail transport settings. One account per environment: the SMTP
// credentials are the whole installation's, not an individual corralón's.
type MailConfig struct {
	Provider     MailProvider
	FromAddress  string
	FromName     string
	SMTPHost     string
	SMTPPort     int
	SMTPUsername string
	SMTPPassword string
	// SMTPStartTLS is declared rather than negotiated, so a server that stops advertising
	// STARTTLS fails the send instead of quietly downgrading it to plaintext.
	SMTPStartTLS bool
	SMTPTimeout  time.Duration
}

// WebConfig holds the frontend base URLs an emailed link points at. The API never serves
// those routes, so it cannot derive them from its own address.
type WebConfig struct {
	BackofficeURL string
}

// SpreadsheetImportConfig holds operational limits for spreadsheet imports.
type SpreadsheetImportConfig struct {
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
			JWTSecret:            os.Getenv("AUTH_JWT_SECRET"),
			AccessTTL:            getDuration("AUTH_ACCESS_TTL_MINUTES", 15*time.Minute, &problems),
			RefreshTTL:           getDuration("AUTH_REFRESH_TTL_HOURS", 12*time.Hour, &problems),
			RefreshRememberTTL:   getDuration("AUTH_REFRESH_REMEMBER_DAYS", 30*24*time.Hour, &problems),
			RefreshReuseGrace:    getDuration("AUTH_REFRESH_REUSE_GRACE_SECONDS", 30*time.Second, &problems),
			MaxFailedAttempts:    getInt("AUTH_MAX_FAILED_ATTEMPTS", 5, &problems),
			LockoutDuration:      getDuration("AUTH_LOCKOUT_MINUTES", 15*time.Minute, &problems),
			PasswordMinLength:    getInt("AUTH_PASSWORD_MIN_LENGTH", 12, &problems),
			PasswordResetTTL:     getDuration("AUTH_PASSWORD_RESET_TTL_MINUTES", 60*time.Minute, &problems),
			RequireVerifiedEmail: getBool("AUTH_REQUIRE_VERIFIED_EMAIL", false, &problems),
			VerificationTTL:      getDuration("AUTH_EMAIL_VERIFICATION_TTL_HOURS", 48*time.Hour, &problems),
		},
		Mail: MailConfig{
			Provider:     MailProvider(getString("MAIL_PROVIDER", string(MailProviderConsole))),
			FromAddress:  getString("MAIL_FROM_ADDRESS", ""),
			FromName:     getString("MAIL_FROM_NAME", "Coti"),
			SMTPHost:     getString("MAIL_SMTP_HOST", ""),
			SMTPPort:     getInt("MAIL_SMTP_PORT", 587, &problems),
			SMTPUsername: getString("MAIL_SMTP_USERNAME", ""),
			SMTPPassword: getString("MAIL_SMTP_PASSWORD", ""),
			SMTPStartTLS: getBool("MAIL_SMTP_STARTTLS", true, &problems),
			SMTPTimeout:  getDuration("MAIL_SMTP_TIMEOUT_SECONDS", 10*time.Second, &problems),
		},
		Web: WebConfig{
			BackofficeURL: getString("WEB_BACKOFFICE_URL", "http://localhost:3000"),
		},
		Catalog: CatalogConfig{
			DefaultPageSize: getInt("CATALOG_DEFAULT_PAGE_SIZE", 50, &problems),
			MaxPageSize:     getInt("CATALOG_MAX_PAGE_SIZE", 200, &problems),
		},
		RateLimit: RateLimitConfig{
			Enabled:          getBool("RATE_LIMIT_ENABLED", true, &problems),
			Window:           getDuration("RATE_LIMIT_WINDOW_SECONDS", time.Minute, &problems),
			Global:           getInt("RATE_LIMIT_GLOBAL_MAX", 300, &problems),
			Credentials:      getInt("RATE_LIMIT_CREDENTIALS_MAX", 10, &problems),
			Signup:           getInt("RATE_LIMIT_SIGNUP_MAX", 5, &problems),
			Mail:             getInt("RATE_LIMIT_MAIL_MAX", 5, &problems),
			MailPerAddress:   getInt("RATE_LIMIT_MAIL_PER_ADDRESS_MAX", 3, &problems),
			TrustedProxyHops: getInt("RATE_LIMIT_TRUSTED_PROXY_HOPS", 0, &problems),
			TrustedProxies:   getCIDRs("RATE_LIMIT_TRUSTED_PROXY_CIDRS", &problems),
		},
		Branch: BranchConfig{
			DefaultExpiryDays: getInt("BRANCH_DEFAULT_EXPIRY_DAYS", 7, &problems),
		},
		CatalogImport: SpreadsheetImportConfig{
			MaxBytes: int64(getInt("CATALOG_IMPORT_MAX_BYTES", defaultCatalogImportMaxBytes, &problems)),
		},
		PriceImport: SpreadsheetImportConfig{
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
	if cfg.CatalogImport.MaxBytes <= 0 {
		problems = append(problems, "CATALOG_IMPORT_MAX_BYTES must be greater than zero")
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
	if cfg.Auth.VerificationTTL <= 0 {
		problems = append(problems, "AUTH_EMAIL_VERIFICATION_TTL_HOURS must be greater than zero")
	}
	// Demanding a confirmed address while the only transport writes to a log would lock
	// every user out of an environment nobody can receive mail in.
	if cfg.Auth.RequireVerifiedEmail && cfg.Mail.Provider == MailProviderConsole {
		problems = append(problems, "AUTH_REQUIRE_VERIFIED_EMAIL needs a mail provider that "+
			"delivers: the console transport only writes to the log")
	}

	// The console transport reaches nothing, so it needs no sender of its own; a real one
	// cannot start without the address and credentials it authenticates with.
	switch cfg.Mail.Provider {
	case MailProviderConsole:
		if cfg.Mail.FromAddress == "" {
			cfg.Mail.FromAddress = "no-reply@coti.local"
		}
	case MailProviderSMTP:
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
		if cfg.Mail.SMTPTimeout <= 0 {
			problems = append(problems, "MAIL_SMTP_TIMEOUT_SECONDS must be greater than zero")
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

	if cfg.RateLimit.Enabled {
		if cfg.RateLimit.Window <= 0 {
			problems = append(problems, "RATE_LIMIT_WINDOW_SECONDS must be greater than zero")
		}
		tighter := []struct {
			key   string
			value int
		}{
			{"RATE_LIMIT_CREDENTIALS_MAX", cfg.RateLimit.Credentials},
			{"RATE_LIMIT_SIGNUP_MAX", cfg.RateLimit.Signup},
			{"RATE_LIMIT_MAIL_MAX", cfg.RateLimit.Mail},
		}
		if cfg.RateLimit.Global < 1 {
			problems = append(problems, "RATE_LIMIT_GLOBAL_MAX must be at least 1")
		}
		for _, t := range tighter {
			if t.value < 1 {
				problems = append(problems, t.key+" must be at least 1")
				continue
			}
			// A per-route allowance above the global one can never bite, so it reads as
			// protection that is not there.
			if t.value > cfg.RateLimit.Global {
				problems = append(problems, fmt.Sprintf("%s (%d) exceeds RATE_LIMIT_GLOBAL_MAX (%d), "+
					"so it can never be reached", t.key, t.value, cfg.RateLimit.Global))
			}
		}
		// Left out of the comparison above on purpose: this one is counted across callers, so
		// the per-caller global says nothing about whether it can be reached.
		if cfg.RateLimit.MailPerAddress < 1 {
			problems = append(problems, "RATE_LIMIT_MAIL_PER_ADDRESS_MAX must be at least 1")
		}
		if cfg.RateLimit.TrustedProxyHops < 0 {
			problems = append(problems, "RATE_LIMIT_TRUSTED_PROXY_HOPS cannot be negative")
		}
		// Counting hops in a header any caller could have written lets them pick their own
		// bucket, so the two settings only mean something together.
		if cfg.RateLimit.TrustedProxyHops > 0 && len(cfg.RateLimit.TrustedProxies) == 0 {
			problems = append(problems, "RATE_LIMIT_TRUSTED_PROXY_HOPS is set without "+
				"RATE_LIMIT_TRUSTED_PROXY_CIDRS: a forwarding header is only believed from a "+
				"declared proxy")
		}
		if len(cfg.RateLimit.TrustedProxies) > 0 && cfg.RateLimit.TrustedProxyHops == 0 {
			problems = append(problems, "RATE_LIMIT_TRUSTED_PROXY_CIDRS is set with "+
				"RATE_LIMIT_TRUSTED_PROXY_HOPS at 0, so the header is never read")
		}
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

// getCIDRs reads a comma-separated list of networks. Single addresses are accepted and
// widened to a /32 or /128, because "the proxy is at 10.0.0.4" is how an operator thinks.
func getCIDRs(key string, problems *[]string) []*net.IPNet {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return nil
	}
	var networks []*net.IPNet
	for _, entry := range strings.Split(raw, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if !strings.Contains(entry, "/") {
			if address := net.ParseIP(entry); address != nil {
				bits := 32
				if address.To4() == nil {
					bits = 128
				}
				entry = fmt.Sprintf("%s/%d", entry, bits)
			}
		}
		_, network, err := net.ParseCIDR(entry)
		if err != nil {
			*problems = append(*problems, fmt.Sprintf("%s has an entry that is not an address "+
				"or a network: %q", key, entry))
			continue
		}
		networks = append(networks, network)
	}
	return networks
}

func getBool(key string, fallback bool, problems *[]string) bool {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	v, err := strconv.ParseBool(raw)
	if err != nil {
		*problems = append(*problems, fmt.Sprintf("%s must be true or false, got %q", key, raw))
		return fallback
	}
	return v
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
