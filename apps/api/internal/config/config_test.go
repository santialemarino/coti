package config

import (
	"fmt"
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
		"RATE_LIMIT_AI_MAX",
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
		"AI_MAX_ATTEMPTS", "AI_RETRY_BACKOFF_SECONDS", "AI_MAX_BACKOFF_SECONDS",
		"AI_EMBEDDINGS_BATCH_SIZE",
		"WEB_BACKOFFICE_URL",
		"CATALOG_DEFAULT_PAGE_SIZE", "CATALOG_MAX_PAGE_SIZE",
		"CATALOG_SEARCH_TOP_K", "CATALOG_SEARCH_OVER_FETCH_FACTOR",
		"CATALOG_SEARCH_MAX_FETCH", "CATALOG_SEARCH_IVFFLAT_PROBES", "CATALOG_SEARCH_RRF_K",
		"CATALOG_EMBEDDING_BATCH_SIZE",
		"CATALOG_MATCH_MIN_CONFIDENCE_PERCENT", "CATALOG_MATCH_AMBIGUITY_MARGIN_PERCENT",
		"CATALOG_MATCH_LEXICAL_CONFIDENCE_PERCENT",
		"CATALOG_IMPORT_MAX_BYTES",
		"PRICE_IMPORT_MAX_BYTES",
		"JOB_TIMEOUT_MINUTES",
		"RFQ_MAX_TEXT_CHARACTERS", "RFQ_MAX_ITEMS", "RFQ_PIPELINE_TIMEOUT_SECONDS",
		"QUOTE_CORRECTION_SIMILARITY_PERCENT", "QUOTE_CORRECTION_MAX_PATTERNS_PER_ACCOUNT",
		"QUOTE_CORRECTION_MAX_INTERPRETATION_EXAMPLES", "QUOTE_CORRECTION_PROCESSING_BATCH_SIZE",
		"STORAGE_PROVIDER", "STORAGE_LOCAL_DIR", "STORAGE_LOCAL_API_BASE_URL",
		"STORAGE_LOCAL_SIGNING_SECRET", "STORAGE_ENDPOINT", "STORAGE_REGION", "STORAGE_BUCKET",
		"STORAGE_ACCESS_KEY", "STORAGE_SECRET_KEY",
		"STORAGE_MAX_FILE_SIZE_BYTES", "STORAGE_SIGNED_URL_EXPIRY_MINUTES",
		"CHANNEL_CONFIG_ENCRYPTION_KEY",
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
		// The default storage provider signs its own links, so its secret is as required as
		// the token one.
		"STORAGE_LOCAL_SIGNING_SECRET": validSecret,
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
	// Pinned exactly rather than as a relationship: .env.example and docs/technical quote these
	// three numbers, and a drifting default would leave them describing a different product.
	if cfg.RFQ.MaxTextCharacters != 20000 {
		t.Errorf("RFQ.MaxTextCharacters = %d, want 20000", cfg.RFQ.MaxTextCharacters)
	}
	if cfg.RFQ.MaxItems != 200 {
		t.Errorf("RFQ.MaxItems = %d, want 200", cfg.RFQ.MaxItems)
	}
	if cfg.RFQ.PipelineTimeout != 25*time.Second {
		t.Errorf("RFQ.PipelineTimeout = %v, want 25s", cfg.RFQ.PipelineTimeout)
	}
	if cfg.QuoteCorrection.SimilarityPercent != 80 ||
		cfg.QuoteCorrection.MaxPatternsPerAccount != 1000 ||
		cfg.QuoteCorrection.MaxInterpretationExamples != 3 ||
		cfg.QuoteCorrection.ProcessingBatchSize != 100 {
		t.Errorf("QuoteCorrection defaults = %+v, want 80/1000/3/100", cfg.QuoteCorrection)
	}
	// The pipeline has to answer inside the response budget, or its reply is cut off mid-write
	// and the client reads a broken connection instead of a model that ran out of time.
	if cfg.RFQ.PipelineTimeout >= cfg.Server.WriteTimeout {
		t.Errorf("RFQ.PipelineTimeout = %v, want it below Server.WriteTimeout %v",
			cfg.RFQ.PipelineTimeout, cfg.Server.WriteTimeout)
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
	if cfg.RateLimit.AI != 10 {
		t.Errorf("RateLimit.AI = %d, want 10", cfg.RateLimit.AI)
	}
	if !cfg.RateLimit.Enabled {
		t.Error("RateLimit.Enabled = false, want true by default")
	}
	if cfg.RateLimit.TrustedProxyHops != 0 {
		t.Errorf("RateLimit.TrustedProxyHops = %d, want 0: nothing is in front by default",
			cfg.RateLimit.TrustedProxyHops)
	}
	// An over-fetch factor of one would ask for exactly what the caller wants, which the
	// branch filter then cuts into, and one probe recalls too little to make up for it.
	if cfg.Catalog.SearchOverFetchFactor < 2 {
		t.Errorf("Catalog.SearchOverFetchFactor = %d, want more than 1 by default",
			cfg.Catalog.SearchOverFetchFactor)
	}
	if cfg.Catalog.SearchProbes < 2 {
		t.Errorf("Catalog.SearchProbes = %d, want more than the database's own 1",
			cfg.Catalog.SearchProbes)
	}
	if cfg.Catalog.SearchTopK != 10 {
		t.Errorf("Catalog.SearchTopK = %d, want 10", cfg.Catalog.SearchTopK)
	}
	// A ceiling below the first fetch would make the widening shrink instead of grow.
	if cfg.Catalog.SearchMaxFetch < cfg.Catalog.SearchTopK*cfg.Catalog.SearchOverFetchFactor {
		t.Errorf("Catalog.SearchMaxFetch = %d, want at least the first fetch width",
			cfg.Catalog.SearchMaxFetch)
	}
	if cfg.Catalog.SearchRRFK != 60 {
		t.Errorf("Catalog.SearchRRFK = %d, want 60", cfg.Catalog.SearchRRFK)
	}
	if cfg.Catalog.EmbeddingBatchSize != 200 {
		t.Errorf("Catalog.EmbeddingBatchSize = %d, want 200", cfg.Catalog.EmbeddingBatchSize)
	}
	// Pinned exactly, because .env.example and the catalog documentation both quote these three
	// and nothing else would notice them drifting apart.
	for _, tc := range []struct {
		name string
		got  int
		want int
	}{
		{"Catalog.MatchMinConfidencePercent", cfg.Catalog.MatchMinConfidencePercent, 60},
		{"Catalog.MatchAmbiguityMarginPercent", cfg.Catalog.MatchAmbiguityMarginPercent, 5},
		{"Catalog.MatchLexicalConfidencePercent", cfg.Catalog.MatchLexicalConfidencePercent, 75},
	} {
		if tc.got != tc.want {
			t.Errorf("%s = %d, want %d", tc.name, tc.got, tc.want)
		}
	}
	// A trade term loaded as a synonym has to resolve to its product out of the box, and the
	// lexical half is the only half that reaches an unembedded row.
	if cfg.Catalog.MatchLexicalConfidencePercent < cfg.Catalog.MatchMinConfidencePercent {
		t.Errorf("Catalog.MatchLexicalConfidencePercent = %d, want at least the floor of %d",
			cfg.Catalog.MatchLexicalConfidencePercent, cfg.Catalog.MatchMinConfidencePercent)
	}
	// At zero every leading candidate is decided, whatever sits behind it.
	if cfg.Catalog.MatchAmbiguityMarginPercent < 1 {
		t.Errorf("Catalog.MatchAmbiguityMarginPercent = %d, want a margin by default",
			cfg.Catalog.MatchAmbiguityMarginPercent)
	}
	if cfg.Job.Timeout != 30*time.Minute {
		t.Errorf("Job.Timeout = %v, want 30m", cfg.Job.Timeout)
	}
	if cfg.IsProduction() {
		t.Error("IsProduction() = true, want false")
	}
}

// The suffix on a duration key carries its unit, so the env file holds plain numbers.
// TestLoad_RFQKeysLandOnTheirOwnFields reads three distinct values, because three sibling keys
// mapped onto three sibling fields is the copy-paste the compiler cannot see: swapping two of them
// builds, vets clean, and every default pin still passes.
func TestLoad_RFQKeysLandOnTheirOwnFields(t *testing.T) {
	env := minimalEnv()
	env["RFQ_MAX_TEXT_CHARACTERS"] = "1234"
	env["RFQ_MAX_ITEMS"] = "77"
	env["RFQ_PIPELINE_TIMEOUT_SECONDS"] = "9"
	setEnv(t, env)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() = %v, want no error", err)
	}
	if cfg.RFQ.MaxTextCharacters != 1234 {
		t.Errorf("RFQ.MaxTextCharacters = %d, want 1234", cfg.RFQ.MaxTextCharacters)
	}
	if cfg.RFQ.MaxItems != 77 {
		t.Errorf("RFQ.MaxItems = %d, want 77", cfg.RFQ.MaxItems)
	}
	if cfg.RFQ.PipelineTimeout != 9*time.Second {
		t.Errorf("RFQ.PipelineTimeout = %v, want 9s", cfg.RFQ.PipelineTimeout)
	}
}

func TestLoad_QuoteCorrectionKeysLandOnTheirOwnFields(t *testing.T) {
	env := minimalEnv()
	env["QUOTE_CORRECTION_SIMILARITY_PERCENT"] = "81"
	env["QUOTE_CORRECTION_MAX_PATTERNS_PER_ACCOUNT"] = "901"
	env["QUOTE_CORRECTION_MAX_INTERPRETATION_EXAMPLES"] = "4"
	env["QUOTE_CORRECTION_PROCESSING_BATCH_SIZE"] = "73"
	setEnv(t, env)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() = %v, want no error", err)
	}
	if cfg.QuoteCorrection.SimilarityPercent != 81 ||
		cfg.QuoteCorrection.MaxPatternsPerAccount != 901 ||
		cfg.QuoteCorrection.MaxInterpretationExamples != 4 ||
		cfg.QuoteCorrection.ProcessingBatchSize != 73 {
		t.Errorf("QuoteCorrection = %+v, want 81/901/4/73", cfg.QuoteCorrection)
	}
}

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
			// A run bounded at nothing holds its lock until the process is killed, and every
			// later firing then does nothing while it waits.
			name:    "a scheduled run bounded at nothing",
			mutate:  func(e map[string]string) { e["JOB_TIMEOUT_MINUTES"] = "0" },
			wantSub: "JOB_TIMEOUT_MINUTES must be greater than zero",
		},
		{
			// An allowance above the global one can never bite, so it reads as a cost bound that
			// is not there.
			name:    "an AI allowance wider than the global one",
			mutate:  func(e map[string]string) { e["RATE_LIMIT_AI_MAX"] = "301" },
			wantSub: "RATE_LIMIT_AI_MAX (301) exceeds RATE_LIMIT_GLOBAL_MAX",
		},
		{
			name:    "an order bounded at nothing",
			mutate:  func(e map[string]string) { e["RFQ_MAX_TEXT_CHARACTERS"] = "0" },
			wantSub: "RFQ_MAX_TEXT_CHARACTERS must be greater than zero",
		},
		{
			name:    "no line allowed per order",
			mutate:  func(e map[string]string) { e["RFQ_MAX_ITEMS"] = "0" },
			wantSub: "RFQ_MAX_ITEMS must be greater than zero",
		},
		{
			name:    "a pipeline bounded at nothing",
			mutate:  func(e map[string]string) { e["RFQ_PIPELINE_TIMEOUT_SECONDS"] = "0" },
			wantSub: "RFQ_PIPELINE_TIMEOUT_SECONDS must be greater than zero",
		},
		{
			// Allowed to outlast the response budget, the pipeline's answer is cut off while it
			// is being written, which reaches the client as a broken connection.
			name: "a pipeline allowed to outlast the response budget",
			mutate: func(e map[string]string) {
				e["RFQ_PIPELINE_TIMEOUT_SECONDS"] = "30"
				e["SERVER_WRITE_TIMEOUT_SECONDS"] = "30"
			},
			wantSub: "must be below SERVER_WRITE_TIMEOUT_SECONDS",
		},
		{
			name:    "catalog import size of zero",
			mutate:  func(e map[string]string) { e["CATALOG_IMPORT_MAX_BYTES"] = "0" },
			wantSub: "CATALOG_IMPORT_MAX_BYTES must be greater than zero",
		},
		{
			name:    "no search results asked for",
			mutate:  func(e map[string]string) { e["CATALOG_SEARCH_TOP_K"] = "0" },
			wantSub: "CATALOG_SEARCH_TOP_K must be at least 2",
		},
		{
			// One candidate never has a runner-up, so matching could never call a line
			// ambiguous and every line above the floor would read as decided.
			name:    "a single search result, leaving matching no runner-up",
			mutate:  func(e map[string]string) { e["CATALOG_SEARCH_TOP_K"] = "1" },
			wantSub: "CATALOG_SEARCH_TOP_K must be at least 2",
		},
		{
			// Below one the search would ask the database for fewer rows than the caller wants.
			name:    "an over-fetch factor that shrinks the fetch",
			mutate:  func(e map[string]string) { e["CATALOG_SEARCH_OVER_FETCH_FACTOR"] = "0" },
			wantSub: "CATALOG_SEARCH_OVER_FETCH_FACTOR must be at least 1",
		},
		{
			name:    "a fetch ceiling of nothing",
			mutate:  func(e map[string]string) { e["CATALOG_SEARCH_MAX_FETCH"] = "0" },
			wantSub: "CATALOG_SEARCH_MAX_FETCH must be at least 1",
		},
		{
			name:    "no index partitions visited",
			mutate:  func(e map[string]string) { e["CATALOG_SEARCH_IVFFLAT_PROBES"] = "0" },
			wantSub: "CATALOG_SEARCH_IVFFLAT_PROBES must be at least 1",
		},
		{
			name:    "an embedding batch of nothing",
			mutate:  func(e map[string]string) { e["CATALOG_EMBEDDING_BATCH_SIZE"] = "0" },
			wantSub: "CATALOG_EMBEDDING_BATCH_SIZE must be at least 1",
		},
		{
			// The similarity these are compared against never exceeds 1, so a threshold past
			// 100 would flag every line NO_MATCH with nothing in the logs to say why.
			name:    "a confidence floor no candidate could reach",
			mutate:  func(e map[string]string) { e["CATALOG_MATCH_MIN_CONFIDENCE_PERCENT"] = "101" },
			wantSub: "CATALOG_MATCH_MIN_CONFIDENCE_PERCENT must be between 0 and 100",
		},
		{
			name:    "a negative ambiguity margin",
			mutate:  func(e map[string]string) { e["CATALOG_MATCH_AMBIGUITY_MARGIN_PERCENT"] = "-1" },
			wantSub: "CATALOG_MATCH_AMBIGUITY_MARGIN_PERCENT must be between 0 and 100",
		},
		{
			name: "a lexical confidence off the scale",
			mutate: func(e map[string]string) {
				e["CATALOG_MATCH_LEXICAL_CONFIDENCE_PERCENT"] = "140"
			},
			wantSub: "CATALOG_MATCH_LEXICAL_CONFIDENCE_PERCENT must be between 0 and 100",
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
	// Without a ceiling the doubling is unbounded, so raising the attempt count alone would
	// produce waits measured in minutes.
	if cfg.AI.Retry.MaxBackoff != 8*time.Second {
		t.Errorf("AI.Retry.MaxBackoff = %v, want 8s", cfg.AI.Retry.MaxBackoff)
	}
	if cfg.AI.EmbeddingsBatchSize != 100 {
		t.Errorf("AI.EmbeddingsBatchSize = %d, want 100", cfg.AI.EmbeddingsBatchSize)
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
		{
			name: "no ceiling on the wait",
			env:  map[string]string{"AI_MAX_BACKOFF_SECONDS": "0"},
			want: "AI_MAX_BACKOFF_SECONDS must be greater than zero",
		},
		{
			// A ceiling below the first wait would make the ladder shrink instead of grow.
			name: "ceiling below the first wait",
			env: map[string]string{
				"AI_RETRY_BACKOFF_SECONDS": "10",
				"AI_MAX_BACKOFF_SECONDS":   "5",
			},
			want: "AI_MAX_BACKOFF_SECONDS (5s) is below AI_RETRY_BACKOFF_SECONDS (10s)",
		},
		{
			name: "no batch size",
			env: map[string]string{
				"AI_EMBEDDINGS_PROVIDER":   "openai",
				"AI_OPENAI_API_KEY":        "sk-test",
				"AI_EMBEDDINGS_BATCH_SIZE": "0",
			},
			want: "AI_EMBEDDINGS_BATCH_SIZE must be at least 1",
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

// Each adapter is handed only its own settings, so no package holds a key it must not use — and a
// stray %+v of one cannot print another provider's credential.
func TestAIConfig_NarrowsWhatEachAdapterSees(t *testing.T) {
	t.Parallel()

	cfg := AIConfig{
		AnthropicAPIKey:      "sk-ant-secret",
		AnthropicBaseURL:     "https://gateway.internal",
		OpenAIAPIKey:         "sk-openai-secret",
		OpenAIBaseURL:        "https://api.openai.com/v1",
		LLMModel:             "claude-opus-5",
		LLMEffort:            "low",
		LLMMaxTokens:         16000,
		LLMTimeout:           time.Minute,
		EmbeddingsModel:      "text-embedding-3-small",
		EmbeddingsBatchSize:  100,
		EmbeddingsTimeout:    30 * time.Second,
		TranscriptionModel:   "whisper-1",
		TranscriptionTimeout: 2 * time.Minute,
		Retry:                AIRetryPolicy{MaxAttempts: 3, Backoff: time.Second, MaxBackoff: 8 * time.Second},
	}

	llm := cfg.Anthropic()
	if llm.APIKey != "sk-ant-secret" || llm.Model != "claude-opus-5" || llm.MaxTokens != 16000 {
		t.Fatalf("Anthropic() = %+v, want the language-model settings", llm)
	}
	if strings.Contains(fmt.Sprintf("%+v", llm), "sk-openai-secret") {
		t.Fatal("Anthropic() carries the OpenAI key, which that adapter must not hold")
	}

	embeddings := cfg.Embeddings()
	if embeddings.APIKey != "sk-openai-secret" || embeddings.BatchSize != 100 {
		t.Fatalf("Embeddings() = %+v, want the embedding settings", embeddings)
	}
	if strings.Contains(fmt.Sprintf("%+v", embeddings), "sk-ant-secret") {
		t.Fatal("Embeddings() carries the Anthropic key, which that adapter must not hold")
	}

	transcription := cfg.Transcription()
	if transcription.APIKey != "sk-openai-secret" || transcription.Model != "whisper-1" {
		t.Fatalf("Transcription() = %+v, want the transcription settings", transcription)
	}
	if strings.Contains(fmt.Sprintf("%+v", transcription), "sk-ant-secret") {
		t.Fatal("Transcription() carries the Anthropic key, which that adapter must not hold")
	}

	// The retry policy is shared on purpose: it is one operational decision, not three.
	for name, got := range map[string]AIRetryPolicy{
		"Anthropic":     llm.Retry,
		"Embeddings":    embeddings.Retry,
		"Transcription": transcription.Retry,
	} {
		if got != cfg.Retry {
			t.Errorf("%s().Retry = %+v, want the shared policy %+v", name, got, cfg.Retry)
		}
	}
}

func spacesEnv() map[string]string {
	env := minimalEnv()
	env["STORAGE_PROVIDER"] = "spaces"
	env["STORAGE_ENDPOINT"] = "https://nyc3.digitaloceanspaces.com"
	env["STORAGE_REGION"] = "us-east-1"
	env["STORAGE_BUCKET"] = "coti-attachments"
	env["STORAGE_ACCESS_KEY"] = "spaces-access-key"
	env["STORAGE_SECRET_KEY"] = "spaces-secret-key"
	return env
}

// The default provider keeps files on the filesystem, so a checkout with no bucket — which is
// every checkout today — has to boot.
func TestLoad_StorageArrivesLocalAndNeedsNoBucket(t *testing.T) {
	setEnv(t, minimalEnv())

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() = %v, want no error", err)
	}
	if cfg.Storage.Provider != StorageProviderLocal {
		t.Errorf("Storage.Provider = %q, want %q", cfg.Storage.Provider, StorageProviderLocal)
	}
	if cfg.Storage.Bucket != "" || cfg.Storage.AccessKey != "" || cfg.Storage.SecretKey != "" {
		t.Errorf("Storage = %+v, want no bucket credentials", cfg.Storage)
	}
	// Pinned exactly rather than as a relationship: .env.example and docs/technical quote these.
	if cfg.Storage.MaxFileSize != 10*1024*1024 {
		t.Errorf("Storage.MaxFileSize = %d, want %d", cfg.Storage.MaxFileSize, 10*1024*1024)
	}
	if cfg.Storage.SignedURLExpiry != 15*time.Minute {
		t.Errorf("Storage.SignedURLExpiry = %v, want 15m", cfg.Storage.SignedURLExpiry)
	}
	if cfg.Storage.Dir != "./.storage" {
		t.Errorf("Storage.Dir = %q, want %q", cfg.Storage.Dir, "./.storage")
	}
	if cfg.Storage.APIBaseURL != "http://localhost:8000" {
		t.Errorf("Storage.APIBaseURL = %q, want %q", cfg.Storage.APIBaseURL, "http://localhost:8000")
	}
}

// Every key set to a value only it could produce: with the defaults in place both halves of a
// swapped pair still read correctly, so a defaults test cannot see the mistake at all.
func TestLoad_StorageKeysLandOnTheirOwnFields(t *testing.T) {
	env := spacesEnv()
	env["STORAGE_LOCAL_DIR"] = "/var/lib/coti-objects"
	env["STORAGE_LOCAL_API_BASE_URL"] = "https://api.example.test"
	env["STORAGE_LOCAL_SIGNING_SECRET"] = "0123456789abcdef0123456789abcdefX"
	env["STORAGE_MAX_FILE_SIZE_BYTES"] = "4242"
	env["STORAGE_SIGNED_URL_EXPIRY_MINUTES"] = "7"
	setEnv(t, env)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() = %v, want no error", err)
	}
	cases := []struct {
		name string
		got  string
		want string
	}{
		{"Provider", string(cfg.Storage.Provider), "spaces"},
		{"Dir", cfg.Storage.Dir, "/var/lib/coti-objects"},
		{"APIBaseURL", cfg.Storage.APIBaseURL, "https://api.example.test"},
		{"SigningSecret", cfg.Storage.SigningSecret, "0123456789abcdef0123456789abcdefX"},
		{"Endpoint", cfg.Storage.Endpoint, "https://nyc3.digitaloceanspaces.com"},
		{"Region", cfg.Storage.Region, "us-east-1"},
		{"Bucket", cfg.Storage.Bucket, "coti-attachments"},
		{"AccessKey", cfg.Storage.AccessKey, "spaces-access-key"},
		{"SecretKey", cfg.Storage.SecretKey, "spaces-secret-key"},
	}
	for _, tc := range cases {
		if tc.got != tc.want {
			t.Errorf("Storage.%s = %q, want %q", tc.name, tc.got, tc.want)
		}
	}
	if cfg.Storage.MaxFileSize != 4242 {
		t.Errorf("Storage.MaxFileSize = %d, want 4242", cfg.Storage.MaxFileSize)
	}
	if cfg.Storage.SignedURLExpiry != 7*time.Minute {
		t.Errorf("Storage.SignedURLExpiry = %v, want 7m", cfg.Storage.SignedURLExpiry)
	}
}

func TestLoad_SpacesProviderLoadsWithEveryCredentialPresent(t *testing.T) {
	setEnv(t, spacesEnv())

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() = %v, want no error", err)
	}
	if cfg.Storage.Provider != StorageProviderSpaces {
		t.Errorf("Storage.Provider = %q, want %q", cfg.Storage.Provider, StorageProviderSpaces)
	}
}

// The mirror of the local case: a bucket signs its own links, so the key that signs the API's
// own must not be demanded from a deployment that never uses it.
func TestLoad_SpacesProviderNeedsNoLinkSigningSecret(t *testing.T) {
	env := spacesEnv()
	delete(env, "STORAGE_LOCAL_SIGNING_SECRET")
	setEnv(t, env)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() = %v, want no error", err)
	}
	if cfg.Storage.SigningSecret != "" {
		t.Errorf("Storage.SigningSecret = %q, want empty", cfg.Storage.SigningSecret)
	}
}

func TestLoad_SpacesProviderReportsEveryMissingKeyTogether(t *testing.T) {
	missing := []string{"STORAGE_ENDPOINT", "STORAGE_REGION", "STORAGE_BUCKET",
		"STORAGE_ACCESS_KEY", "STORAGE_SECRET_KEY"}
	env := spacesEnv()
	for _, key := range missing {
		delete(env, key)
	}
	setEnv(t, env)

	_, err := Load()
	if err == nil {
		t.Fatal("Load() = nil error, want an error")
	}
	for _, want := range missing {
		if !strings.Contains(err.Error(), want+" is required when STORAGE_PROVIDER is spaces") {
			t.Errorf("Load() error is missing %q; got:\n%s", want, err.Error())
		}
	}
}

func TestLoad_LocalProviderDemandsASigningSecretLongEnoughToSign(t *testing.T) {
	env := minimalEnv()
	env["STORAGE_LOCAL_SIGNING_SECRET"] = "short"
	setEnv(t, env)

	_, err := Load()
	if err == nil {
		t.Fatal("Load() = nil error, want an error")
	}
	if !strings.Contains(err.Error(), "STORAGE_LOCAL_SIGNING_SECRET must be at least 32 characters") {
		t.Errorf("Load() error does not name STORAGE_LOCAL_SIGNING_SECRET; got:\n%s", err.Error())
	}
}

func TestLoad_StorageRejectsUnusableSettings(t *testing.T) {
	cases := []struct {
		name string
		env  func() map[string]string
		want string
	}{
		{"unknown provider", func() map[string]string {
			env := minimalEnv()
			env["STORAGE_PROVIDER"] = "gcs"
			return env
		}, `STORAGE_PROVIDER must be "local" or "spaces", got "gcs"`},
		{"local base url with no host", func() map[string]string {
			env := minimalEnv()
			env["STORAGE_LOCAL_API_BASE_URL"] = "localhost:8000"
			return env
		}, "STORAGE_LOCAL_API_BASE_URL must be an absolute URL"},
		{"spaces endpoint with no scheme", func() map[string]string {
			env := spacesEnv()
			env["STORAGE_ENDPOINT"] = "nyc3.digitaloceanspaces.com"
			return env
		}, "STORAGE_ENDPOINT must be an absolute URL"},
		{"no maximum file size", func() map[string]string {
			env := minimalEnv()
			env["STORAGE_MAX_FILE_SIZE_BYTES"] = "0"
			return env
		}, "STORAGE_MAX_FILE_SIZE_BYTES must be greater than zero"},
		{"no link lifetime", func() map[string]string {
			env := minimalEnv()
			env["STORAGE_SIGNED_URL_EXPIRY_MINUTES"] = "0"
			return env
		}, "STORAGE_SIGNED_URL_EXPIRY_MINUTES must be greater than zero"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setEnv(t, tc.env())

			_, err := Load()
			if err == nil {
				t.Fatal("Load() = nil error, want an error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("Load() error is missing %q; got:\n%s", tc.want, err.Error())
			}
		})
	}
}

func TestStorageConfig_NarrowsWhatEachAdapterSees(t *testing.T) {
	t.Parallel()

	cfg := StorageConfig{
		Provider:      StorageProviderSpaces,
		Dir:           "/var/lib/coti-objects",
		APIBaseURL:    "https://api.example.test",
		SigningSecret: "storage-signing-secret",
		Endpoint:      "https://nyc3.digitaloceanspaces.com",
		Region:        "us-east-1",
		Bucket:        "coti-attachments",
		AccessKey:     "spaces-access-key",
		SecretKey:     "spaces-secret-key",
	}

	local := cfg.Local()
	if local.Dir != cfg.Dir || local.APIBaseURL != cfg.APIBaseURL || local.SigningSecret != cfg.SigningSecret {
		t.Fatalf("Local() = %+v, want the filesystem settings", local)
	}
	if strings.Contains(fmt.Sprintf("%+v", local), "spaces-secret-key") {
		t.Fatal("Local() carries the bucket credentials, which that adapter must not hold")
	}

	spaces := cfg.Spaces()
	if spaces.Bucket != cfg.Bucket || spaces.Region != cfg.Region || spaces.SecretKey != cfg.SecretKey {
		t.Fatalf("Spaces() = %+v, want the bucket settings", spaces)
	}
	if strings.Contains(fmt.Sprintf("%+v", spaces), "storage-signing-secret") {
		t.Fatal("Spaces() carries the link signing secret, which that adapter must not hold")
	}
}

func TestLoad_ChannelEncryptionKeyIsOptionalAndDecoded(t *testing.T) {
	setEnv(t, minimalEnv())

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() = %v, want no error", err)
	}
	if cfg.Channel.EncryptionKey != nil {
		t.Errorf("Channel.EncryptionKey = %v, want nil: a checkout with no key has to boot",
			cfg.Channel.EncryptionKey)
	}

	env := minimalEnv()
	// 32 bytes, base64 of "0123456789abcdef0123456789abcdef".
	env["CHANNEL_CONFIG_ENCRYPTION_KEY"] = "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY="
	setEnv(t, env)

	cfg, err = Load()
	if err != nil {
		t.Fatalf("Load() = %v, want no error", err)
	}
	if string(cfg.Channel.EncryptionKey) != validSecret {
		t.Errorf("Channel.EncryptionKey = %q, want %q decoded", cfg.Channel.EncryptionKey,
			validSecret)
	}
	if len(cfg.Channel.EncryptionKey) != channelKeyLength {
		t.Errorf("Channel.EncryptionKey is %d bytes, want %d", len(cfg.Channel.EncryptionKey),
			channelKeyLength)
	}
}

// A malformed key is a startup problem rather than a silent fallback to no encryption, and the
// message never quotes the value: it is the key.
func TestLoad_ChannelEncryptionKeyRejectsAnUnusableValue(t *testing.T) {
	for _, test := range []struct {
		name  string
		value string
		want  string
	}{
		{name: "not base64", value: "not base64 at all!",
			want: "CHANNEL_CONFIG_ENCRYPTION_KEY must be base64-encoded"},
		{name: "too short", value: "c2hvcnQ=",
			want: "CHANNEL_CONFIG_ENCRYPTION_KEY must decode to 32 bytes, got 5"},
		{name: "too long", value: "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWZmZg==",
			want: "CHANNEL_CONFIG_ENCRYPTION_KEY must decode to 32 bytes, got 34"},
	} {
		t.Run(test.name, func(t *testing.T) {
			env := minimalEnv()
			env["CHANNEL_CONFIG_ENCRYPTION_KEY"] = test.value
			setEnv(t, env)

			_, err := Load()
			if err == nil {
				t.Fatal("Load() = nil, want an error")
			}
			if !strings.Contains(err.Error(), test.want) {
				t.Errorf("Load() = %q, want it to mention %q", err, test.want)
			}
			if strings.Contains(err.Error(), test.value) {
				t.Errorf("Load() = %q, want the key value left out of the message", err)
			}
		})
	}
}
