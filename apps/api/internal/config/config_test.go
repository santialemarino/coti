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
		"RATE_LIMIT_MAIL_PER_ADDRESS_MAX", "RATE_LIMIT_TRUSTED_PROXY_HOPS",
		"RATE_LIMIT_TRUSTED_PROXY_CIDRS",
		"MAIL_PROVIDER", "MAIL_FROM_ADDRESS", "MAIL_FROM_NAME",
		"MAIL_SMTP_HOST", "MAIL_SMTP_PORT", "MAIL_SMTP_USERNAME", "MAIL_SMTP_PASSWORD",
		"MAIL_SMTP_STARTTLS", "MAIL_SMTP_TIMEOUT_SECONDS",
		"AI_ANTHROPIC_API_KEY", "AI_ANTHROPIC_BASE_URL", "AI_OPENAI_API_KEY", "AI_OPENAI_BASE_URL",
		"AI_LLM_PROVIDER", "AI_LLM_MODEL", "AI_LLM_EFFORT", "AI_LLM_MAX_TOKENS",
		"AI_LLM_TIMEOUT_SECONDS",
		"AI_EMBEDDINGS_PROVIDER", "AI_EMBEDDINGS_MODEL", "AI_EMBEDDINGS_TIMEOUT_SECONDS",
		"AI_TRANSCRIPTION_PROVIDER", "AI_TRANSCRIPTION_MODEL", "AI_TRANSCRIPTION_TIMEOUT_SECONDS",
		"AI_MAX_ATTEMPTS", "AI_RETRY_BACKOFF_SECONDS",
		"WEB_BACKOFFICE_URL",
		"CATALOG_IMPORT_MAX_BYTES",
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
	if cfg.CatalogImport.MaxBytes != defaultCatalogImportMaxBytes {
		t.Errorf("CatalogImport.MaxBytes = %d, want %d", cfg.CatalogImport.MaxBytes, defaultCatalogImportMaxBytes)
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
			// It is counted across callers, so unlike the per-route allowances it is not
			// compared against the global one — only refused when it cannot bite at all.
			name:    "per-address mail allowance of zero",
			mutate:  func(e map[string]string) { e["RATE_LIMIT_MAIL_PER_ADDRESS_MAX"] = "0" },
			wantSub: "RATE_LIMIT_MAIL_PER_ADDRESS_MAX must be at least 1",
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
		{
			name:    "catalog import size of zero",
			mutate:  func(e map[string]string) { e["CATALOG_IMPORT_MAX_BYTES"] = "0" },
			wantSub: "CATALOG_IMPORT_MAX_BYTES must be greater than zero",
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

// The per-address allowance is counted across callers, so unlike a per-route one it is not
// unreachable above the global limit and must not be rejected for exceeding it.
func TestLoad_PerAddressMailAllowanceMayExceedTheGlobalOne(t *testing.T) {
	env := minimalEnv()
	env["RATE_LIMIT_GLOBAL_MAX"] = "10"
	env["RATE_LIMIT_MAIL_PER_ADDRESS_MAX"] = "50"
	setEnv(t, env)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() = %v, want no error", err)
	}
	if cfg.RateLimit.MailPerAddress != 50 {
		t.Errorf("RateLimit.MailPerAddress = %d, want 50", cfg.RateLimit.MailPerAddress)
	}
}

func smtpEnv() map[string]string {
	env := minimalEnv()
	env["MAIL_PROVIDER"] = "smtp"
	env["MAIL_FROM_ADDRESS"] = "no-reply@coti.test"
	env["MAIL_SMTP_HOST"] = "smtp.coti.test"
	env["MAIL_SMTP_USERNAME"] = "coti"
	env["MAIL_SMTP_PASSWORD"] = "s3cret"
	return env
}

func TestLoad_SMTPProviderLoadsWithEveryCredentialPresent(t *testing.T) {
	setEnv(t, smtpEnv())

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() = %v, want no error", err)
	}
	if cfg.Mail.Provider != MailProviderSMTP {
		t.Errorf("Mail.Provider = %q, want %q", cfg.Mail.Provider, MailProviderSMTP)
	}
	if cfg.Mail.SMTPPort != 587 {
		t.Errorf("Mail.SMTPPort = %d, want 587", cfg.Mail.SMTPPort)
	}
	// Encryption defaults on: an operator who says nothing gets the safe transport, and
	// reaching a sandbox that speaks no TLS is the case that has to be asked for.
	if !cfg.Mail.SMTPStartTLS {
		t.Error("Mail.SMTPStartTLS = false by default, want true")
	}
	if cfg.Mail.SMTPTimeout != 10*time.Second {
		t.Errorf("Mail.SMTPTimeout = %v, want 10s", cfg.Mail.SMTPTimeout)
	}
}

// The address a verification link is demanded of has to be reachable, which is exactly what the
// console transport cannot do — and what landing this adapter is for.
func TestLoad_SMTPProviderMayRequireVerifiedEmail(t *testing.T) {
	env := smtpEnv()
	env["AUTH_REQUIRE_VERIFIED_EMAIL"] = "true"
	setEnv(t, env)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() = %v, want no error", err)
	}
	if !cfg.Auth.RequireVerifiedEmail {
		t.Error("Auth.RequireVerifiedEmail = false, want true")
	}
}

// One pass names every missing credential, so an operator fixes the environment once instead of
// discovering the next blank key on the next restart.
func TestLoad_SMTPProviderReportsEveryMissingKeyTogether(t *testing.T) {
	env := smtpEnv()
	for _, key := range []string{"MAIL_FROM_ADDRESS", "MAIL_SMTP_HOST", "MAIL_SMTP_USERNAME", "MAIL_SMTP_PASSWORD"} {
		delete(env, key)
	}
	setEnv(t, env)

	_, err := Load()
	if err == nil {
		t.Fatal("Load() = nil error, want an error")
	}
	for _, want := range []string{"MAIL_FROM_ADDRESS", "MAIL_SMTP_HOST", "MAIL_SMTP_USERNAME", "MAIL_SMTP_PASSWORD"} {
		if !strings.Contains(err.Error(), want+" is required when MAIL_PROVIDER is smtp") {
			t.Errorf("Load() error is missing %q; got:\n%s", want, err.Error())
		}
	}
}

func TestLoad_SMTPProviderRejectsAnUnusableConnection(t *testing.T) {
	env := smtpEnv()
	env["MAIL_SMTP_PORT"] = "0"
	env["MAIL_SMTP_TIMEOUT_SECONDS"] = "0"
	setEnv(t, env)

	_, err := Load()
	if err == nil {
		t.Fatal("Load() = nil error, want an error")
	}
	for _, want := range []string{"MAIL_SMTP_PORT", "MAIL_SMTP_TIMEOUT_SECONDS"} {
		if !strings.Contains(err.Error(), want+" must be greater than zero") {
			t.Errorf("Load() error is missing %q; got:\n%s", want, err.Error())
		}
	}
}

// No AI provider needs to exist for the API to boot: a fresh checkout has no keys, and the engine
// refuses the calls that needed a model rather than the process refusing to start.
func TestLoad_AIProvidersArriveDisabled(t *testing.T) {
	setEnv(t, minimalEnv())

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() = %v, want no error", err)
	}
	for _, provider := range []struct {
		name  string
		value AIProvider
	}{
		{"AI.LLMProvider", cfg.AI.LLMProvider},
		{"AI.EmbeddingsProvider", cfg.AI.EmbeddingsProvider},
		{"AI.TranscriptionProvider", cfg.AI.TranscriptionProvider},
	} {
		if provider.value != AIProviderDisabled {
			t.Errorf("%s = %q, want %q", provider.name, provider.value, AIProviderDisabled)
		}
	}
	// Mapping work rather than open-ended writing, so the reasoning default sits at the low end.
	if cfg.AI.LLMEffort != "low" {
		t.Errorf("AI.LLMEffort = %q, want low", cfg.AI.LLMEffort)
	}
	if cfg.AI.Retry.MaxAttempts != 3 || cfg.AI.Retry.Backoff != time.Second {
		t.Errorf("AI.Retry = %+v, want 3 attempts from 1s", cfg.AI.Retry)
	}
}

// A capability left disabled must not demand the credentials it would never use.
func TestLoad_DisabledAICapabilitiesNeedNoCredentials(t *testing.T) {
	env := minimalEnv()
	env["AI_LLM_PROVIDER"] = "anthropic"
	env["AI_ANTHROPIC_API_KEY"] = "sk-ant-test"
	setEnv(t, env)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() = %v, want no error: embeddings and transcription are off", err)
	}
	if cfg.AI.LLMProvider != AIProviderAnthropic {
		t.Errorf("AI.LLMProvider = %q, want %q", cfg.AI.LLMProvider, AIProviderAnthropic)
	}
}

// A missing key fails the boot with a message naming it, rather than at the first call. Both are
// reported in the same pass, so enabling two capabilities at once is diagnosed once.
func TestLoad_AIProviderReportsEveryMissingKeyByName(t *testing.T) {
	env := minimalEnv()
	env["AI_LLM_PROVIDER"] = "anthropic"
	env["AI_EMBEDDINGS_PROVIDER"] = "openai"
	setEnv(t, env)

	_, err := Load()
	if err == nil {
		t.Fatal("Load() = nil error, want an error")
	}
	for _, want := range []string{
		"AI_ANTHROPIC_API_KEY is required",
		"AI_OPENAI_API_KEY is required",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("Load() error is missing %q; got:\n%s", want, err.Error())
		}
	}
}

// One key backs two capabilities, so it is named once and not once per capability.
func TestLoad_TheOpenAIKeyIsReportedOnce(t *testing.T) {
	env := minimalEnv()
	env["AI_EMBEDDINGS_PROVIDER"] = "openai"
	env["AI_TRANSCRIPTION_PROVIDER"] = "openai"
	setEnv(t, env)

	_, err := Load()
	if err == nil {
		t.Fatal("Load() = nil error, want an error")
	}
	if got := strings.Count(err.Error(), "AI_OPENAI_API_KEY is required"); got != 1 {
		t.Errorf("AI_OPENAI_API_KEY named %d times, want 1:\n%s", got, err.Error())
	}
}

func TestLoad_AIRejectsUnusableSettings(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
		want string
	}{
		{
			name: "unknown language model provider",
			env:  map[string]string{"AI_LLM_PROVIDER": "openai"},
			want: "AI_LLM_PROVIDER must be",
		},
		{
			name: "unknown embeddings provider",
			env:  map[string]string{"AI_EMBEDDINGS_PROVIDER": "anthropic"},
			want: "AI_EMBEDDINGS_PROVIDER must be",
		},
		{
			name: "unknown transcription provider",
			env:  map[string]string{"AI_TRANSCRIPTION_PROVIDER": "anthropic"},
			want: "AI_TRANSCRIPTION_PROVIDER must be",
		},
		{
			name: "effort outside the accepted levels",
			env: map[string]string{
				"AI_LLM_PROVIDER":      "anthropic",
				"AI_ANTHROPIC_API_KEY": "sk-ant-test",
				"AI_LLM_EFFORT":        "medium-high",
			},
			want: "AI_LLM_EFFORT must be one of",
		},
		{
			name: "no token budget",
			env: map[string]string{
				"AI_LLM_PROVIDER":      "anthropic",
				"AI_ANTHROPIC_API_KEY": "sk-ant-test",
				"AI_LLM_MAX_TOKENS":    "0",
			},
			want: "AI_LLM_MAX_TOKENS must be greater than zero",
		},
		{
			name: "base URL without a scheme",
			env: map[string]string{
				"AI_EMBEDDINGS_PROVIDER": "openai",
				"AI_OPENAI_API_KEY":      "sk-test",
				"AI_OPENAI_BASE_URL":     "api.openai.com/v1",
			},
			want: "AI_OPENAI_BASE_URL must be an absolute URL",
		},
		{
			name: "gateway address without a scheme",
			env: map[string]string{
				"AI_LLM_PROVIDER":       "anthropic",
				"AI_ANTHROPIC_API_KEY":  "sk-ant-test",
				"AI_ANTHROPIC_BASE_URL": "gateway.internal",
			},
			want: "AI_ANTHROPIC_BASE_URL must be an absolute URL",
		},
		{
			name: "no attempts",
			env:  map[string]string{"AI_MAX_ATTEMPTS": "0"},
			want: "AI_MAX_ATTEMPTS must be at least 1",
		},
		{
			name: "no wait between attempts",
			env:  map[string]string{"AI_RETRY_BACKOFF_SECONDS": "0"},
			want: "AI_RETRY_BACKOFF_SECONDS must be greater than zero",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := minimalEnv()
			for k, v := range tc.env {
				env[k] = v
			}
			setEnv(t, env)

			_, err := Load()
			if err == nil {
				t.Fatalf("Load() = nil error, want %q", tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("Load() error is missing %q; got:\n%s", tc.want, err.Error())
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
