package config

import (
	"strings"
	"testing"
	"time"
)

func TestLoadCustomerAuth(t *testing.T) {
	setEnv(t, minimalEnv())
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.CustomerAuth.SessionTTL != 30*24*time.Hour || cfg.CustomerAuth.LockoutDuration != 15*time.Minute || cfg.CustomerAuth.MaxFailedAttempts != 5 || cfg.CustomerAuth.PasswordMinLength != 12 {
		t.Fatalf("unexpected customer auth defaults: %+v", cfg.CustomerAuth)
	}

	t.Setenv("CUSTOMER_SESSION_TTL_DAYS", "7")
	cfg, err = Load()
	if err != nil || cfg.CustomerAuth.SessionTTL != 7*24*time.Hour {
		t.Fatalf("session duration override: %+v, %v", cfg.CustomerAuth, err)
	}
}

func TestLoadCustomerAuthReportsEveryInvalidSetting(t *testing.T) {
	vars := minimalEnv()
	keys := []string{"CUSTOMER_SESSION_TTL_DAYS", "CUSTOMER_LOCKOUT_MINUTES", "CUSTOMER_MAX_FAILED_ATTEMPTS", "CUSTOMER_PASSWORD_MIN_LENGTH"}
	for _, key := range keys {
		vars[key] = "0"
	}
	setEnv(t, vars)
	_, err := Load()
	if err == nil {
		t.Fatal("expected invalid configuration")
	}
	for _, key := range keys {
		if !strings.Contains(err.Error(), key) {
			t.Errorf("missing %s in %v", key, err)
		}
	}
}

func TestLoadCustomerContactVerificationSettings(t *testing.T) {
	vars := minimalEnv()
	vars["CUSTOMER_CONTACT_VERIFICATION_TTL_MINUTES"] = "13"
	vars["CUSTOMER_CONTACT_VERIFICATION_COOLDOWN_SECONDS"] = "71"
	vars["CUSTOMER_CONTACT_VERIFICATION_MAX_ATTEMPTS"] = "7"
	vars["WHATSAPP_CONTACT_CODE_TEMPLATE"] = "contact_code"
	vars["WHATSAPP_CONTACT_CODE_LANGUAGE"] = "es_AR"
	setEnv(t, vars)
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	c := cfg.CustomerAuth
	if c.ContactVerificationTTL != 13*time.Minute || c.ContactVerificationCooldown != 71*time.Second || c.ContactVerificationAttempts != 7 || c.ContactCodeTemplate != "contact_code" || c.ContactCodeLanguage != "es_AR" {
		t.Fatalf("verification configuration wiring: %+v", c)
	}
}

func TestLoadCustomerContactVerificationDefaultsAndInvalidSettings(t *testing.T) {
	vars := minimalEnv()
	keys := []string{"CUSTOMER_CONTACT_VERIFICATION_TTL_MINUTES", "CUSTOMER_CONTACT_VERIFICATION_COOLDOWN_SECONDS", "CUSTOMER_CONTACT_VERIFICATION_MAX_ATTEMPTS"}
	for _, key := range keys {
		vars[key] = ""
	}
	setEnv(t, vars)
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.CustomerAuth.ContactVerificationTTL != 10*time.Minute || cfg.CustomerAuth.ContactVerificationCooldown != time.Minute || cfg.CustomerAuth.ContactVerificationAttempts != 5 {
		t.Fatalf("verification defaults: %+v", cfg.CustomerAuth)
	}
	for _, key := range keys {
		t.Setenv(key, "0")
	}
	_, err = Load()
	if err == nil {
		t.Fatal("invalid verification settings accepted")
	}
	for _, key := range keys {
		if !strings.Contains(err.Error(), key) {
			t.Errorf("missing %s in %v", key, err)
		}
	}
}
