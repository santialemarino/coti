package config

import (
	"strings"
	"testing"
	"time"
)

const validSecret = "0123456789abcdef0123456789abcdef" // 32 chars.

// setEnv applies the given variables for the test and clears everything else Load
// reads, so a stray value in the developer's shell cannot change the outcome.
func setEnv(t *testing.T, vars map[string]string) {
	t.Helper()
	known := []string{
		"ENV", "LOG_LEVEL", "API_PORT",
		"SERVER_READ_TIMEOUT_SECONDS", "SERVER_WRITE_TIMEOUT_SECONDS", "SERVER_SHUTDOWN_TIMEOUT_SECONDS",
		"DATABASE_URL", "DATABASE_ADMIN_URL", "DB_MAX_CONNS", "DB_MIN_CONNS",
		"DB_MAX_CONN_LIFETIME_MINUTES", "DB_MAX_CONN_IDLE_MINUTES", "DB_CONNECT_TIMEOUT_SECONDS",
		"AUTH_JWT_SECRET", "AUTH_ACCESS_TTL_MINUTES", "AUTH_REFRESH_TTL_HOURS",
		"AUTH_REFRESH_REMEMBER_DAYS", "AUTH_REFRESH_REUSE_GRACE_SECONDS",
		"AUTH_MAX_FAILED_ATTEMPTS", "AUTH_LOCKOUT_MINUTES", "AUTH_PASSWORD_MIN_LENGTH",
		"AUTH_PASSWORD_RESET_TTL_MINUTES", "AUTH_EMAIL_VERIFICATION_TTL_HOURS",
		"AUTH_REQUIRE_VERIFIED_EMAIL",
		"RATE_LIMIT_ENABLED", "RATE_LIMIT_WINDOW_SECONDS", "RATE_LIMIT_GLOBAL_MAX",
		"RATE_LIMIT_CREDENTIALS_MAX", "RATE_LIMIT_SIGNUP_MAX", "RATE_LIMIT_MAIL_MAX",
		"RATE_LIMIT_TRUSTED_PROXY_HOPS",
		"MAIL_PROVIDER", "MAIL_FROM_ADDRESS", "MAIL_FROM_NAME",
		"MAIL_SMTP_HOST", "MAIL_SMTP_PORT", "MAIL_SMTP_USERNAME", "MAIL_SMTP_PASSWORD",
		"WEB_BACKOFFICE_URL",
		"PRICE_IMPORT_MAX_BYTES",
	}
	for _, k := range known {
		t.Setenv(k, "")
	}
	for k, v := range vars {
		t.Setenv(k, v)
	}
}

func minimalEnv() map[string]string {
	return map[string]string{
		"DATABASE_URL":       "postgres://app@localhost:5432/coti",
		"DATABASE_ADMIN_URL": "postgres://owner@localhost:5432/coti",
		"AUTH_JWT_SECRET":    validSecret,
	}
}

func TestLoad_Defaults(t *testing.T) {
	setEnv(t, minimalEnv())

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() = %v, want no error", err)
	}

	if cfg.Environment != EnvironmentDevelopment {
		t.Errorf("Environment = %q, want %q", cfg.Environment, EnvironmentDevelopment)
	}
	if cfg.Server.Port != "8000" {
		t.Errorf("Server.Port = %q, want %q", cfg.Server.Port, "8000")
	}
	if cfg.Database.MaxConns != 10 {
		t.Errorf("Database.MaxConns = %d, want 10", cfg.Database.MaxConns)
	}
	if cfg.Auth.AccessTTL != 15*time.Minute {
		t.Errorf("Auth.AccessTTL = %v, want 15m", cfg.Auth.AccessTTL)
	}
	if cfg.Auth.RefreshRememberTTL != 30*24*time.Hour {
		t.Errorf("Auth.RefreshRememberTTL = %v, want 720h", cfg.Auth.RefreshRememberTTL)
	}
	if cfg.PriceImport.MaxBytes != defaultPriceImportMaxBytes {
		t.Errorf("PriceImport.MaxBytes = %d, want %d", cfg.PriceImport.MaxBytes, defaultPriceImportMaxBytes)
	}
	if cfg.Mail.Provider != MailProviderConsole {
		t.Errorf("Mail.Provider = %q, want %q", cfg.Mail.Provider, MailProviderConsole)
	}
	// A transport that reaches nobody still needs an address to sign messages with, so the
	// console default must not leave one missing.
	if cfg.Mail.FromAddress == "" {
		t.Error("Mail.FromAddress is empty under the console provider, want a default")
	}
	if cfg.Auth.PasswordResetTTL != time.Hour {
		t.Errorf("Auth.PasswordResetTTL = %v, want 1h", cfg.Auth.PasswordResetTTL)
	}
	// The requirement has to arrive off, or a fresh environment locks everyone out.
	if cfg.Auth.RequireVerifiedEmail {
		t.Error("Auth.RequireVerifiedEmail = true, want false by default")
	}
	if !cfg.RateLimit.Enabled {
		t.Error("RateLimit.Enabled = false, want true by default")
	}
	if cfg.RateLimit.TrustedProxyHops != 0 {
		t.Errorf("RateLimit.TrustedProxyHops = %d, want 0: nothing is in front by default",
			cfg.RateLimit.TrustedProxyHops)
	}
	if cfg.IsProduction() {
		t.Error("IsProduction() = true, want false")
	}
}

// The suffix on a duration key carries its unit, so the env file holds plain numbers.
func TestLoad_DurationUnitsComeFromKeySuffix(t *testing.T) {
	env := minimalEnv()
	env["AUTH_ACCESS_TTL_MINUTES"] = "5"
	env["AUTH_REFRESH_TTL_HOURS"] = "2"
	env["AUTH_REFRESH_REMEMBER_DAYS"] = "7"
	env["DB_CONNECT_TIMEOUT_SECONDS"] = "3"
	setEnv(t, env)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() = %v, want no error", err)
	}

	cases := []struct {
		name string
		got  time.Duration
		want time.Duration
	}{
		{"AccessTTL", cfg.Auth.AccessTTL, 5 * time.Minute},
		{"RefreshTTL", cfg.Auth.RefreshTTL, 2 * time.Hour},
		{"RefreshRememberTTL", cfg.Auth.RefreshRememberTTL, 7 * 24 * time.Hour},
		{"ConnectTimeout", cfg.Database.ConnectTimeout, 3 * time.Second},
	}
	for _, tc := range cases {
		if tc.got != tc.want {
			t.Errorf("%s = %v, want %v", tc.name, tc.got, tc.want)
		}
	}
}

func TestLoad_Invalid(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(map[string]string)
		wantSub string
	}{
		{
			name:    "missing app url",
			mutate:  func(e map[string]string) { delete(e, "DATABASE_URL") },
			wantSub: "DATABASE_URL is required",
		},
		{
			name:    "missing admin url",
			mutate:  func(e map[string]string) { delete(e, "DATABASE_ADMIN_URL") },
			wantSub: "DATABASE_ADMIN_URL is required",
		},
		{
			name:    "short jwt secret",
			mutate:  func(e map[string]string) { e["AUTH_JWT_SECRET"] = "too-short" },
			wantSub: "AUTH_JWT_SECRET must be at least 32 characters",
		},
		{
			name:    "unknown environment",
			mutate:  func(e map[string]string) { e["ENV"] = "staging" },
			wantSub: "ENV must be",
		},
		{
			name:    "non-numeric int",
			mutate:  func(e map[string]string) { e["DB_MAX_CONNS"] = "ten" },
			wantSub: "DB_MAX_CONNS must be an integer",
		},
		{
			name:    "min conns above max",
			mutate:  func(e map[string]string) { e["DB_MIN_CONNS"] = "50" },
			wantSub: "DB_MIN_CONNS (50) exceeds DB_MAX_CONNS (10)",
		},
		{
			// The request pool must not run as the owner in production: it would bypass
			// every row level security policy without any visible symptom.
			name: "production reuses the owner url for requests",
			mutate: func(e map[string]string) {
				e["ENV"] = "production"
				e["DATABASE_URL"] = e["DATABASE_ADMIN_URL"]
			},
			wantSub: "DATABASE_URL must differ from DATABASE_ADMIN_URL in production",
		},
		{
			name:    "unknown mail provider",
			mutate:  func(e map[string]string) { e["MAIL_PROVIDER"] = "carrier-pigeon" },
			wantSub: `MAIL_PROVIDER must be "console" or "smtp"`,
		},
		{
			// A real transport with no sender and no credentials has to fail at startup, not
			// on the first message nobody receives.
			name:    "real provider without credentials",
			mutate:  func(e map[string]string) { e["MAIL_PROVIDER"] = "smtp" },
			wantSub: "MAIL_SMTP_PASSWORD is required when MAIL_PROVIDER is smtp",
		},
		{
			name: "real provider without a sender",
			mutate: func(e map[string]string) {
				e["MAIL_PROVIDER"] = "smtp"
				e["MAIL_SMTP_HOST"] = "smtp.example"
				e["MAIL_SMTP_USERNAME"] = "coti"
				e["MAIL_SMTP_PASSWORD"] = "secret"
			},
			wantSub: "MAIL_FROM_ADDRESS is required when MAIL_PROVIDER is smtp",
		},
		{
			name:    "backoffice url with no scheme",
			mutate:  func(e map[string]string) { e["WEB_BACKOFFICE_URL"] = "backoffice.example" },
			wantSub: "WEB_BACKOFFICE_URL must be an absolute URL",
		},
		{
			// Demanding a confirmed address while the only transport writes to a log would
			// lock every user out of an environment nobody can receive mail in.
			name:    "verification required with the console transport",
			mutate:  func(e map[string]string) { e["AUTH_REQUIRE_VERIFIED_EMAIL"] = "true" },
			wantSub: "AUTH_REQUIRE_VERIFIED_EMAIL needs a mail provider that delivers",
		},
		{
			// A per-route allowance above the global one can never bite, so it reads as
			// protection that is not there.
			name:    "route allowance above the global one",
			mutate:  func(e map[string]string) { e["RATE_LIMIT_CREDENTIALS_MAX"] = "9999" },
			wantSub: "RATE_LIMIT_CREDENTIALS_MAX (9999) exceeds RATE_LIMIT_GLOBAL_MAX",
		},
		{
			name:    "negative proxy hops",
			mutate:  func(e map[string]string) { e["RATE_LIMIT_TRUSTED_PROXY_HOPS"] = "-1" },
			wantSub: "RATE_LIMIT_TRUSTED_PROXY_HOPS cannot be negative",
		},
		{
			name:    "non-boolean flag",
			mutate:  func(e map[string]string) { e["AUTH_REQUIRE_VERIFIED_EMAIL"] = "yes-please" },
			wantSub: `AUTH_REQUIRE_VERIFIED_EMAIL must be true or false, got "yes-please"`,
		},
		{
			name:    "password reset ttl of zero",
			mutate:  func(e map[string]string) { e["AUTH_PASSWORD_RESET_TTL_MINUTES"] = "0" },
			wantSub: "AUTH_PASSWORD_RESET_TTL_MINUTES must be greater than zero",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := minimalEnv()
			tc.mutate(env)
			setEnv(t, env)

			_, err := Load()
			if err == nil {
				t.Fatal("Load() = nil error, want an error")
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Errorf("Load() error = %q, want it to contain %q", err.Error(), tc.wantSub)
			}
		})
	}
}

// Every problem is reported in one pass so a bad deploy is diagnosed once.
func TestLoad_ReportsEveryProblemAtOnce(t *testing.T) {
	setEnv(t, map[string]string{"AUTH_JWT_SECRET": "short"})

	_, err := Load()
	if err == nil {
		t.Fatal("Load() = nil error, want an error")
	}
	for _, want := range []string{"DATABASE_URL is required", "DATABASE_ADMIN_URL is required", "AUTH_JWT_SECRET"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("Load() error is missing %q; got:\n%s", want, err.Error())
		}
	}
}
