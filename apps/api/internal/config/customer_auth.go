package config

import (
	"fmt"
	"net/url"
	"os"
	"time"
)

// CustomerAuthConfig bounds customer sessions and credential attempts independently of staff auth.
type CustomerAuthConfig struct {
	GoogleClientID              string
	GoogleClientSecret          string
	GoogleLocalOrigin           string
	GoogleTimeout               time.Duration
	SignupTTL                   time.Duration
	RecoveryTTL                 time.Duration
	VerificationTTL             time.Duration
	OrderPageSize               int
	SessionTTL                  time.Duration
	MaxFailedAttempts           int
	LockoutDuration             time.Duration
	PasswordMinLength           int
	ContactVerificationTTL      time.Duration
	ContactVerificationCooldown time.Duration
	ContactVerificationAttempts int
	// ContactDestinationMax bounds the proofs one supplier sends a destination per window.
	ContactDestinationMax    int
	ContactDestinationWindow time.Duration
	// ContactFailureMax wrong proofs against a destination at one supplier lock it there for ContactLockout.
	ContactFailureMax int
	ContactLockout    time.Duration
	// ContactSpamMax bounds the proofs a destination receives per window across every supplier.
	ContactSpamMax      int
	ContactSpamWindow   time.Duration
	ProfileContactsMax  int
	ContactCodeTemplate string
	ContactCodeLanguage string
}

func loadCustomerAuth(problems *[]string) CustomerAuthConfig {
	cfg := CustomerAuthConfig{
		GoogleClientID:              os.Getenv("CUSTOMER_GOOGLE_CLIENT_ID"),
		ContactVerificationTTL:      getDuration("CUSTOMER_CONTACT_VERIFICATION_TTL_MINUTES", 10*time.Minute, problems),
		ContactVerificationCooldown: getDuration("CUSTOMER_CONTACT_VERIFICATION_COOLDOWN_SECONDS", time.Minute, problems),
		ContactVerificationAttempts: getInt("CUSTOMER_CONTACT_VERIFICATION_MAX_ATTEMPTS", 5, problems),
		ContactDestinationMax:       getInt("CUSTOMER_CONTACT_VERIFICATION_DESTINATION_MAX", 5, problems),
		ContactDestinationWindow:    getDuration("CUSTOMER_CONTACT_VERIFICATION_DESTINATION_WINDOW_HOURS", 24*time.Hour, problems),
		ContactFailureMax:           getInt("CUSTOMER_CONTACT_VERIFICATION_FAILURE_MAX", 10, problems),
		ContactLockout:              getDuration("CUSTOMER_CONTACT_VERIFICATION_LOCKOUT_MINUTES", time.Hour, problems),
		ContactSpamMax:              getInt("CUSTOMER_CONTACT_VERIFICATION_SPAM_MAX", 20, problems),
		ContactSpamWindow:           getDuration("CUSTOMER_CONTACT_VERIFICATION_SPAM_WINDOW_HOURS", 24*time.Hour, problems),
		ProfileContactsMax:          getInt("CUSTOMER_PROFILE_CONTACTS_MAX", 10, problems),
		ContactCodeTemplate:         os.Getenv("WHATSAPP_CONTACT_CODE_TEMPLATE"),
		ContactCodeLanguage:         getString("WHATSAPP_CONTACT_CODE_LANGUAGE", "es_AR"),
		GoogleClientSecret:          os.Getenv("CUSTOMER_GOOGLE_CLIENT_SECRET"),
		GoogleLocalOrigin:           os.Getenv("CUSTOMER_GOOGLE_LOCAL_ORIGIN"),
		GoogleTimeout:               getDuration("CUSTOMER_GOOGLE_TIMEOUT_SECONDS", 10*time.Second, problems),
		SignupTTL:                   getDuration("CUSTOMER_GOOGLE_SIGNUP_TTL_MINUTES", 10*time.Minute, problems),
		RecoveryTTL:                 getDuration("CUSTOMER_RECOVERY_TTL_MINUTES", time.Hour, problems),
		VerificationTTL:             getDuration("CUSTOMER_EMAIL_VERIFICATION_TTL_HOURS", 24*time.Hour, problems),
		OrderPageSize:               getInt("CUSTOMER_ORDER_PAGE_SIZE", 20, problems),
		SessionTTL:                  getDuration("CUSTOMER_SESSION_TTL_DAYS", 30*24*time.Hour, problems),
		MaxFailedAttempts:           getInt("CUSTOMER_MAX_FAILED_ATTEMPTS", 5, problems),
		LockoutDuration:             getDuration("CUSTOMER_LOCKOUT_MINUTES", 15*time.Minute, problems),
		PasswordMinLength:           getInt("CUSTOMER_PASSWORD_MIN_LENGTH", 12, problems),
	}
	if (cfg.GoogleClientID == "") != (cfg.GoogleClientSecret == "") {
		*problems = append(*problems, "CUSTOMER_GOOGLE_CLIENT_ID and CUSTOMER_GOOGLE_CLIENT_SECRET must be configured together")
	}
	if cfg.ContactVerificationTTL <= 0 {
		*problems = append(*problems, "CUSTOMER_CONTACT_VERIFICATION_TTL_MINUTES must be positive")
	}
	if cfg.ContactVerificationCooldown <= 0 {
		*problems = append(*problems, "CUSTOMER_CONTACT_VERIFICATION_COOLDOWN_SECONDS must be positive")
	}
	if cfg.ContactVerificationAttempts < 1 {
		*problems = append(*problems, "CUSTOMER_CONTACT_VERIFICATION_MAX_ATTEMPTS must be positive")
	}
	if cfg.ContactDestinationMax < 1 || cfg.ContactFailureMax < 1 || cfg.ContactSpamMax < 1 || cfg.ProfileContactsMax < 1 {
		*problems = append(*problems, "CUSTOMER_CONTACT_VERIFICATION_DESTINATION_MAX, CUSTOMER_CONTACT_VERIFICATION_FAILURE_MAX, CUSTOMER_CONTACT_VERIFICATION_SPAM_MAX and CUSTOMER_PROFILE_CONTACTS_MAX must be positive")
	}
	if cfg.ContactDestinationWindow <= 0 || cfg.ContactLockout <= 0 || cfg.ContactSpamWindow <= 0 {
		*problems = append(*problems, "CUSTOMER_CONTACT_VERIFICATION_DESTINATION_WINDOW_HOURS, CUSTOMER_CONTACT_VERIFICATION_LOCKOUT_MINUTES and CUSTOMER_CONTACT_VERIFICATION_SPAM_WINDOW_HOURS must be positive")
	}
	if cfg.GoogleTimeout <= 0 || cfg.SignupTTL <= 0 || cfg.RecoveryTTL <= 0 || cfg.VerificationTTL <= 0 {
		*problems = append(*problems, "CUSTOMER_GOOGLE_TIMEOUT_SECONDS, CUSTOMER_GOOGLE_SIGNUP_TTL_MINUTES, CUSTOMER_RECOVERY_TTL_MINUTES and CUSTOMER_EMAIL_VERIFICATION_TTL_HOURS must be positive")
	}
	if cfg.GoogleLocalOrigin != "" {
		u, err := url.Parse(cfg.GoogleLocalOrigin)
		if os.Getenv("ENV") == "production" || err != nil || u.Scheme != "http" || (u.Hostname() != "localhost" && u.Hostname() != "127.0.0.1") || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.User != nil {
			*problems = append(*problems, "CUSTOMER_GOOGLE_LOCAL_ORIGIN is restricted to a development localhost HTTP origin")
		}
	}
	if cfg.OrderPageSize < 1 || cfg.OrderPageSize > 100 {
		*problems = append(*problems, "CUSTOMER_ORDER_PAGE_SIZE must be between 1 and 100")
	}
	if cfg.SessionTTL <= 0 {
		*problems = append(*problems, "CUSTOMER_SESSION_TTL_DAYS must be greater than zero")
	}
	if cfg.MaxFailedAttempts <= 0 {
		*problems = append(*problems, "CUSTOMER_MAX_FAILED_ATTEMPTS must be greater than zero")
	}
	if cfg.LockoutDuration <= 0 {
		*problems = append(*problems, "CUSTOMER_LOCKOUT_MINUTES must be greater than zero")
	}
	if cfg.PasswordMinLength < 8 {
		*problems = append(*problems, fmt.Sprintf("CUSTOMER_PASSWORD_MIN_LENGTH must be at least 8, got %d", cfg.PasswordMinLength))
	}
	return cfg
}
