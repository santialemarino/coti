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
	"slices"
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

// AIProvider selects the adapter behind one of the domain AI ports.
type AIProvider string

const (
	AIProviderDisabled  AIProvider = "disabled"
	AIProviderAnthropic AIProvider = "anthropic"
	AIProviderOpenAI    AIProvider = "openai"
)

// aiEfforts are the reasoning-depth levels the language model accepts.
var aiEfforts = []string{"low", "medium", "high", "xhigh", "max"}

// Config is the fully resolved runtime configuration.
type Config struct {
	Environment   Environment
	LogLevel      string
	Server        ServerConfig
	Database      DatabaseConfig
	Auth          AuthConfig
	Mail          MailConfig
	AI            AIConfig
	Web           WebConfig
	Catalog       CatalogConfig
	RFQ           RFQConfig
	RateLimit     RateLimitConfig
	Branch        BranchConfig
	Job           JobConfig
	CatalogImport SpreadsheetImportConfig
	PriceImport   SpreadsheetImportConfig
	Storage       StorageConfig
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
	// AI bounds the routes billed per call by a provider, the way Mail bounds the ones whose
	// effect is a message. The global allowance is far too wide for a route that costs money.
	AI int
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

// AIConfig holds the provider selection, credentials and limits for the three external AI
// capabilities. Each is selected on its own because no single provider covers all three, and
// because an environment may want the language model without the transcriber.
type AIConfig struct {
	AnthropicAPIKey string
	// AnthropicBaseURL is empty in normal use: the SDK carries the provider's own address, and
	// duplicating it here would be a second copy to keep current. Set it to reach a gateway.
	AnthropicBaseURL string
	OpenAIAPIKey     string
	OpenAIBaseURL    string

	LLMProvider AIProvider
	LLMModel    string
	// LLMEffort is how much reasoning the model spends. Extraction and classification are
	// mapping work rather than open-ended writing, so the default sits at the low end.
	LLMEffort string
	// LLMMaxTokens caps one answer, the model's own reasoning included.
	LLMMaxTokens int
	LLMTimeout   time.Duration

	EmbeddingsProvider AIProvider
	EmbeddingsModel    string
	// EmbeddingsBatchSize caps how many texts go in one request, so a catalog-sized list is
	// chunked instead of rejected wholesale for exceeding the provider's per-request limits.
	EmbeddingsBatchSize int
	EmbeddingsTimeout   time.Duration

	TranscriptionProvider AIProvider
	TranscriptionModel    string
	TranscriptionTimeout  time.Duration

	Retry AIRetryPolicy
}

// AIRetryPolicy is how many times one provider call is attempted, and how long the wait between
// attempts starts at. The wait doubles from there, up to MaxBackoff.
type AIRetryPolicy struct {
	MaxAttempts int
	Backoff     time.Duration
	// MaxBackoff is the ceiling on one wait. Without it the doubling is unbounded, and it is also
	// the longest window a provider may ask us to sit out before we give up instead.
	MaxBackoff time.Duration
}

// AnthropicConfig is the slice of the AI settings the language-model adapter reads.
type AnthropicConfig struct {
	APIKey    string
	BaseURL   string
	Model     string
	Effort    string
	MaxTokens int
	Timeout   time.Duration
	Retry     AIRetryPolicy
}

// EmbeddingsConfig is the slice of the AI settings the embedding adapter reads.
type EmbeddingsConfig struct {
	APIKey    string
	BaseURL   string
	Model     string
	BatchSize int
	Timeout   time.Duration
	Retry     AIRetryPolicy
}

// TranscriptionConfig is the slice of the AI settings the transcription adapter reads.
type TranscriptionConfig struct {
	APIKey  string
	BaseURL string
	Model   string
	Timeout time.Duration
	Retry   AIRetryPolicy
}

// Anthropic returns what the language-model adapter needs and nothing else, so no adapter is
// handed another provider's key.
func (a AIConfig) Anthropic() AnthropicConfig {
	return AnthropicConfig{
		APIKey:    a.AnthropicAPIKey,
		BaseURL:   a.AnthropicBaseURL,
		Model:     a.LLMModel,
		Effort:    a.LLMEffort,
		MaxTokens: a.LLMMaxTokens,
		Timeout:   a.LLMTimeout,
		Retry:     a.Retry,
	}
}

// Embeddings returns what the embedding adapter needs and nothing else.
func (a AIConfig) Embeddings() EmbeddingsConfig {
	return EmbeddingsConfig{
		APIKey:    a.OpenAIAPIKey,
		BaseURL:   a.OpenAIBaseURL,
		Model:     a.EmbeddingsModel,
		BatchSize: a.EmbeddingsBatchSize,
		Timeout:   a.EmbeddingsTimeout,
		Retry:     a.Retry,
	}
}

// Transcription returns what the transcription adapter needs and nothing else.
func (a AIConfig) Transcription() TranscriptionConfig {
	return TranscriptionConfig{
		APIKey:  a.OpenAIAPIKey,
		BaseURL: a.OpenAIBaseURL,
		Model:   a.TranscriptionModel,
		Timeout: a.TranscriptionTimeout,
		Retry:   a.Retry,
	}
}

// problems reports everything wrong with the AI settings, naming the key at fault. A capability
// left disabled needs no credentials, so the requirements are collected per capability.
func (a AIConfig) problems() []string {
	var problems []string
	needsAnthropic, needsOpenAI := false, false

	switch a.LLMProvider {
	case AIProviderDisabled:
	case AIProviderAnthropic:
		needsAnthropic = true
		if !slices.Contains(aiEfforts, a.LLMEffort) {
			problems = append(problems, fmt.Sprintf("AI_LLM_EFFORT must be one of %s, got %q",
				strings.Join(aiEfforts, ", "), a.LLMEffort))
		}
		if a.LLMMaxTokens <= 0 {
			problems = append(problems, "AI_LLM_MAX_TOKENS must be greater than zero")
		}
		if a.LLMTimeout <= 0 {
			problems = append(problems, "AI_LLM_TIMEOUT_SECONDS must be greater than zero")
		}
	default:
		problems = append(problems, fmt.Sprintf("AI_LLM_PROVIDER must be %q or %q, got %q",
			AIProviderDisabled, AIProviderAnthropic, a.LLMProvider))
	}

	switch a.EmbeddingsProvider {
	case AIProviderDisabled:
	case AIProviderOpenAI:
		needsOpenAI = true
		if a.EmbeddingsTimeout <= 0 {
			problems = append(problems, "AI_EMBEDDINGS_TIMEOUT_SECONDS must be greater than zero")
		}
		if a.EmbeddingsBatchSize < 1 {
			problems = append(problems, "AI_EMBEDDINGS_BATCH_SIZE must be at least 1")
		}
	default:
		problems = append(problems, fmt.Sprintf("AI_EMBEDDINGS_PROVIDER must be %q or %q, got %q",
			AIProviderDisabled, AIProviderOpenAI, a.EmbeddingsProvider))
	}

	switch a.TranscriptionProvider {
	case AIProviderDisabled:
	case AIProviderOpenAI:
		needsOpenAI = true
		if a.TranscriptionTimeout <= 0 {
			problems = append(problems, "AI_TRANSCRIPTION_TIMEOUT_SECONDS must be greater than zero")
		}
	default:
		problems = append(problems, fmt.Sprintf("AI_TRANSCRIPTION_PROVIDER must be %q or %q, got %q",
			AIProviderDisabled, AIProviderOpenAI, a.TranscriptionProvider))
	}

	// One key per provider, reported once however many capabilities it backs.
	if needsAnthropic && a.AnthropicAPIKey == "" {
		problems = append(problems, "AI_ANTHROPIC_API_KEY is required when AI_LLM_PROVIDER is "+
			string(AIProviderAnthropic))
	}
	if needsOpenAI && a.OpenAIAPIKey == "" {
		problems = append(problems, "AI_OPENAI_API_KEY is required when AI_EMBEDDINGS_PROVIDER "+
			"or AI_TRANSCRIPTION_PROVIDER is "+string(AIProviderOpenAI))
	}
	if needsOpenAI {
		if u, err := url.Parse(a.OpenAIBaseURL); err != nil || u.Scheme == "" || u.Host == "" {
			problems = append(problems, fmt.Sprintf("AI_OPENAI_BASE_URL must be an absolute URL "+
				"with a scheme and host, got %q", a.OpenAIBaseURL))
		}
	}
	if a.AnthropicBaseURL != "" {
		if u, err := url.Parse(a.AnthropicBaseURL); err != nil || u.Scheme == "" || u.Host == "" {
			problems = append(problems, fmt.Sprintf("AI_ANTHROPIC_BASE_URL must be an absolute URL "+
				"with a scheme and host, got %q", a.AnthropicBaseURL))
		}
	}

	if a.Retry.MaxAttempts < 1 {
		problems = append(problems, "AI_MAX_ATTEMPTS must be at least 1")
	}
	if a.Retry.Backoff <= 0 {
		problems = append(problems, "AI_RETRY_BACKOFF_SECONDS must be greater than zero")
	}
	if a.Retry.MaxBackoff <= 0 {
		problems = append(problems, "AI_MAX_BACKOFF_SECONDS must be greater than zero")
	}
	// A ceiling below the first wait would make the ladder shrink instead of grow.
	if a.Retry.MaxBackoff > 0 && a.Retry.MaxBackoff < a.Retry.Backoff {
		problems = append(problems, fmt.Sprintf("AI_MAX_BACKOFF_SECONDS (%s) is below "+
			"AI_RETRY_BACKOFF_SECONDS (%s)", a.Retry.MaxBackoff, a.Retry.Backoff))
	}
	return problems
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

// JobConfig holds the settings for the scheduled-job runner. How often a job runs is not here: it
// is the deployment platform's schedule, one component per job.
type JobConfig struct {
	// Timeout bounds one run. Without it a job wedged on a slow query holds its lock until the
	// process is killed, and every later firing does nothing while it waits.
	Timeout time.Duration
}

// RFQConfig bounds one run of the RFQ text pipeline.
type RFQConfig struct {
	// MaxTextCharacters caps the order the extractor is asked to read. rfq.raw_text is unbounded,
	// so without this a pasted document would be sent to the model whole.
	MaxTextCharacters int
	// MaxItems caps the lines one order may produce. Matching runs a query per line, so a
	// spreadsheet pasted as text would turn one request into hundreds of them.
	MaxItems int
	// PipelineTimeout bounds the whole extract-and-match pass. The AI timeouts are per attempt,
	// so a retrying chain outruns the response budget; this makes the route answer 503 instead
	// of having its response cut off mid-write.
	PipelineTimeout time.Duration
}

// CatalogConfig holds the catalog listing limits and the knobs behind the hybrid search. The
// listing cap is what stops a client from asking for the whole catalog in one response.
type CatalogConfig struct {
	DefaultPageSize int
	MaxPageSize     int
	// SearchTopK is how many candidates a search returns when the caller names no limit.
	SearchTopK int
	// SearchOverFetchFactor multiplies the rows each half of the search asks the database for.
	// An approximate vector scan filters by branch after ordering, so asking for exactly K
	// leaves fewer than K once what the branch does not carry is dropped.
	SearchOverFetchFactor int
	// SearchMaxFetch is the widest one round may ask the database for. Without it a branch that
	// yields one more row per round drives the doubling up to the size of the catalog.
	SearchMaxFetch int
	// SearchProbes is how many index partitions an approximate scan visits. One is the
	// database's default and recalls too little to survive the branch filter.
	SearchProbes int
	// SearchRRFK is the constant in the reciprocal rank fusion that merges the lexical and
	// semantic halves. Higher flattens the ranking, so the tail counts for more.
	SearchRRFK int
	// EmbeddingBatchSize is how many products the backfill loads and writes back per round.
	EmbeddingBatchSize int
	// MatchMinConfidencePercent is the similarity a line's leading candidate clears to be a
	// match at all. Below it the line is flagged NO_MATCH and keeps no product.
	MatchMinConfidencePercent int
	// MatchAmbiguityMarginPercent is how far the leading candidate sits above the runner-up
	// before the line counts as decided. Two cements a point apart are a choice, not a match.
	MatchAmbiguityMarginPercent int
	// MatchLexicalConfidencePercent is what a candidate only the lexical half scored is worth.
	// A synonym hit carries no cosine similarity to read, and it is evidence rather than none.
	MatchLexicalConfidencePercent int
}

// StorageConfig holds the object storage settings and file limits.
type StorageConfig struct {
	Endpoint        string
	Region          string
	Bucket          string
	AccessKey       string
	SecretKey       string
	MaxFileSize     int64
	SignedURLExpiry time.Duration
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
		AI: AIConfig{
			AnthropicAPIKey:  getString("AI_ANTHROPIC_API_KEY", ""),
			AnthropicBaseURL: getString("AI_ANTHROPIC_BASE_URL", ""),
			OpenAIAPIKey:     getString("AI_OPENAI_API_KEY", ""),
			OpenAIBaseURL:    getString("AI_OPENAI_BASE_URL", "https://api.openai.com/v1"),

			LLMProvider:  AIProvider(getString("AI_LLM_PROVIDER", string(AIProviderDisabled))),
			LLMModel:     getString("AI_LLM_MODEL", "claude-opus-5"),
			LLMEffort:    getString("AI_LLM_EFFORT", "low"),
			LLMMaxTokens: getInt("AI_LLM_MAX_TOKENS", 16000, &problems),
			LLMTimeout:   getDuration("AI_LLM_TIMEOUT_SECONDS", 60*time.Second, &problems),

			EmbeddingsProvider: AIProvider(getString("AI_EMBEDDINGS_PROVIDER",
				string(AIProviderDisabled))),
			EmbeddingsModel:     getString("AI_EMBEDDINGS_MODEL", "text-embedding-3-small"),
			EmbeddingsBatchSize: getInt("AI_EMBEDDINGS_BATCH_SIZE", 100, &problems),
			EmbeddingsTimeout:   getDuration("AI_EMBEDDINGS_TIMEOUT_SECONDS", 30*time.Second, &problems),

			TranscriptionProvider: AIProvider(getString("AI_TRANSCRIPTION_PROVIDER",
				string(AIProviderDisabled))),
			TranscriptionModel: getString("AI_TRANSCRIPTION_MODEL", "whisper-1"),
			TranscriptionTimeout: getDuration("AI_TRANSCRIPTION_TIMEOUT_SECONDS",
				120*time.Second, &problems),

			Retry: AIRetryPolicy{
				MaxAttempts: getInt("AI_MAX_ATTEMPTS", 3, &problems),
				Backoff:     getDuration("AI_RETRY_BACKOFF_SECONDS", time.Second, &problems),
				MaxBackoff:  getDuration("AI_MAX_BACKOFF_SECONDS", 8*time.Second, &problems),
			},
		},
		Web: WebConfig{
			BackofficeURL: getString("WEB_BACKOFFICE_URL", "http://localhost:3000"),
		},
		Catalog: CatalogConfig{
			DefaultPageSize:               getInt("CATALOG_DEFAULT_PAGE_SIZE", 50, &problems),
			MaxPageSize:                   getInt("CATALOG_MAX_PAGE_SIZE", 200, &problems),
			SearchTopK:                    getInt("CATALOG_SEARCH_TOP_K", 10, &problems),
			SearchOverFetchFactor:         getInt("CATALOG_SEARCH_OVER_FETCH_FACTOR", 4, &problems),
			SearchMaxFetch:                getInt("CATALOG_SEARCH_MAX_FETCH", 2000, &problems),
			SearchProbes:                  getInt("CATALOG_SEARCH_IVFFLAT_PROBES", 10, &problems),
			SearchRRFK:                    getInt("CATALOG_SEARCH_RRF_K", 60, &problems),
			EmbeddingBatchSize:            getInt("CATALOG_EMBEDDING_BATCH_SIZE", 200, &problems),
			MatchMinConfidencePercent:     getInt("CATALOG_MATCH_MIN_CONFIDENCE_PERCENT", 60, &problems),
			MatchAmbiguityMarginPercent:   getInt("CATALOG_MATCH_AMBIGUITY_MARGIN_PERCENT", 5, &problems),
			MatchLexicalConfidencePercent: getInt("CATALOG_MATCH_LEXICAL_CONFIDENCE_PERCENT", 75, &problems),
		},
		Storage: StorageConfig{
			Endpoint:        getString("STORAGE_ENDPOINT", ""),
			Region:          getString("STORAGE_REGION", ""),
			Bucket:          getString("STORAGE_BUCKET", ""),
			AccessKey:       getString("STORAGE_ACCESS_KEY", ""),
			SecretKey:       getString("STORAGE_SECRET_KEY", ""),
			MaxFileSize:     int64(getInt("STORAGE_MAX_FILE_SIZE_BYTES", 10*1024*1024, &problems)),
			SignedURLExpiry: getDuration("STORAGE_SIGNED_URL_EXPIRY_MINUTES", 15*time.Minute, &problems),
		},
		RateLimit: RateLimitConfig{
			Enabled:          getBool("RATE_LIMIT_ENABLED", true, &problems),
			Window:           getDuration("RATE_LIMIT_WINDOW_SECONDS", time.Minute, &problems),
			Global:           getInt("RATE_LIMIT_GLOBAL_MAX", 300, &problems),
			Credentials:      getInt("RATE_LIMIT_CREDENTIALS_MAX", 10, &problems),
			Signup:           getInt("RATE_LIMIT_SIGNUP_MAX", 5, &problems),
			Mail:             getInt("RATE_LIMIT_MAIL_MAX", 5, &problems),
			AI:               getInt("RATE_LIMIT_AI_MAX", 10, &problems),
			MailPerAddress:   getInt("RATE_LIMIT_MAIL_PER_ADDRESS_MAX", 3, &problems),
			TrustedProxyHops: getInt("RATE_LIMIT_TRUSTED_PROXY_HOPS", 0, &problems),
			TrustedProxies:   getCIDRs("RATE_LIMIT_TRUSTED_PROXY_CIDRS", &problems),
		},
		RFQ: RFQConfig{
			MaxTextCharacters: getInt("RFQ_MAX_TEXT_CHARACTERS", 20000, &problems),
			MaxItems:          getInt("RFQ_MAX_ITEMS", 200, &problems),
			PipelineTimeout:   getDuration("RFQ_PIPELINE_TIMEOUT_SECONDS", 25*time.Second, &problems),
		},
		Job: JobConfig{
			Timeout: getDuration("JOB_TIMEOUT_MINUTES", 30*time.Minute, &problems),
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
	if cfg.Storage.MaxFileSize <= 0 {
		problems = append(problems, "STORAGE_MAX_FILE_SIZE_BYTES must be greater than zero")
	}

	if cfg.Storage.SignedURLExpiry <= 0 {
		problems = append(problems, "STORAGE_SIGNED_URL_EXPIRY_MINUTES must be greater than zero")
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

	problems = append(problems, cfg.AI.problems()...)

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
	catalogFloors := []struct {
		key   string
		value int
		floor int
	}{
		// Two, not one: a single candidate leaves matching with no runner-up, so every line
		// above the confidence floor would read as decided and AMBIGUOUS could never happen.
		{"CATALOG_SEARCH_TOP_K", cfg.Catalog.SearchTopK, 2},
		// Below 1 the search would ask for fewer rows than the caller wants, which is the
		// shortfall the factor exists to prevent.
		{"CATALOG_SEARCH_OVER_FETCH_FACTOR", cfg.Catalog.SearchOverFetchFactor, 1},
		{"CATALOG_SEARCH_MAX_FETCH", cfg.Catalog.SearchMaxFetch, 1},
		{"CATALOG_SEARCH_IVFFLAT_PROBES", cfg.Catalog.SearchProbes, 1},
		{"CATALOG_SEARCH_RRF_K", cfg.Catalog.SearchRRFK, 1},
		{"CATALOG_EMBEDDING_BATCH_SIZE", cfg.Catalog.EmbeddingBatchSize, 1},
	}
	for _, f := range catalogFloors {
		if f.value < f.floor {
			problems = append(problems, fmt.Sprintf("%s must be at least %d, got %d",
				f.key, f.floor, f.value))
		}
	}
	// A run bounded at nothing would hold its lock until the process is killed, and every later
	// firing would do nothing while it waited.
	if cfg.Job.Timeout <= 0 {
		problems = append(problems, "JOB_TIMEOUT_MINUTES must be greater than zero")
	}
	if cfg.RFQ.MaxTextCharacters <= 0 {
		problems = append(problems, "RFQ_MAX_TEXT_CHARACTERS must be greater than zero")
	}
	if cfg.RFQ.MaxItems <= 0 {
		problems = append(problems, "RFQ_MAX_ITEMS must be greater than zero")
	}
	if cfg.RFQ.PipelineTimeout <= 0 {
		problems = append(problems, "RFQ_PIPELINE_TIMEOUT_SECONDS must be greater than zero")
	} else if cfg.RFQ.PipelineTimeout >= cfg.Server.WriteTimeout {
		// A pipeline allowed to outlast the response budget has its answer cut off mid-write,
		// which the client reads as a broken connection rather than as a model that timed out.
		problems = append(problems, fmt.Sprintf(
			"RFQ_PIPELINE_TIMEOUT_SECONDS (%s) must be below SERVER_WRITE_TIMEOUT_SECONDS (%s)",
			cfg.RFQ.PipelineTimeout, cfg.Server.WriteTimeout))
	}

	catalogPercents := []struct {
		key   string
		value int
	}{
		{"CATALOG_MATCH_MIN_CONFIDENCE_PERCENT", cfg.Catalog.MatchMinConfidencePercent},
		{"CATALOG_MATCH_AMBIGUITY_MARGIN_PERCENT", cfg.Catalog.MatchAmbiguityMarginPercent},
		{"CATALOG_MATCH_LEXICAL_CONFIDENCE_PERCENT", cfg.Catalog.MatchLexicalConfidencePercent},
	}
	for _, p := range catalogPercents {
		if p.value < 0 || p.value > 100 {
			problems = append(problems, fmt.Sprintf("%s must be between 0 and 100, got %d",
				p.key, p.value))
		}
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
			{"RATE_LIMIT_AI_MAX", cfg.RateLimit.AI},
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
