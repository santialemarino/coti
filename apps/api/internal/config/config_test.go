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
		"AUTH_MAX_FAILED_ATTEMPTS", "AUTH_LOCKOUT_MINUTES",
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
