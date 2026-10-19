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
