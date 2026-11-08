package http

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/delivery/http/handler"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/ratelimit"
)

const recoveryRouteLimit = 2

// acceptingRecovery answers every recovery step, so only the limiter can refuse.
type acceptingRecovery struct{ handler.CustomerAuthService }

func (acceptingRecovery) RecoveryOptions(context.Context, string, string) ([]domain.RecoveryContact, error) {
	return nil, nil
}
func (acceptingRecovery) RequestRecoveryCode(context.Context, string, string, uuid.UUID) (*domain.ContactChallenge, error) {
	return &domain.ContactChallenge{ID: uuid.New()}, nil
}
func (acceptingRecovery) ResetWithRecoveryCode(context.Context, string, uuid.UUID, string, string) (*domain.CustomerLogin, error) {
	return &domain.CustomerLogin{}, nil
}

func recoveryLimitRouter() http.Handler {
	cfg := &config.Config{RateLimit: config.RateLimitConfig{Enabled: true, Window: time.Minute, Global: 100,
		Credentials: recoveryRouteLimit, Mail: recoveryRouteLimit}}
	return NewRouter(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)),
		Handlers{CustomerAuth: handler.NewCustomerAuthHandler(acceptingRecovery{})},
		Auth{Customer: tokenCustomers{}}, RateLimit{Limiter: ratelimit.NewMemory(nil)})
}

func TestRecoveryRoutesShareOneBucketAcrossKeyCaseAndValueSpelling(t *testing.T) {
	t.Parallel()
	id := uuid.New()
	contact := uuid.NewString()
	routes := []struct {
		path   string
		bodies []string
	}{
		{"/auth/recovery/options", []string{`{"tax_id":"20123456786"}`, `{"TAX_ID":"20-12345678-6"}`, `{"Tax_Id":"20 12345678 6"}`}},
		{"/auth/recovery/code", []string{
			`{"tax_id":"20123456786","contact_id":"` + contact + `"}`,
			`{"TAX_ID":"20-12345678-6","contact_id":"` + contact + `"}`,
			`{"Tax_Id":"20.12345678.6","CONTACT_ID":"` + contact + `"}`,
		}},
		{"/auth/recovery/reset", []string{
			`{"id":"` + id.String() + `","code":"123456","password":"x"}`,
			`{"ID":"{` + id.String() + `}","code":"123456","password":"x"}`,
			`{"Id":"urn:uuid:` + id.String() + `","code":"123456","password":"x"}`,
		}},
	}
	for _, route := range routes {
		router := recoveryLimitRouter()
		for i, body := range route.bodies {
			// A new address each time, so only the bucket keyed by what the body names can refuse.
			response := callContactRoute(router, http.MethodPost, route.path, body, "", fmt.Sprintf("10.0.1.%d:1111", i+1))
			if i < recoveryRouteLimit && response.Code == http.StatusTooManyRequests {
				t.Fatalf("%s request %d refused inside the limit", route.path, i+1)
			}
			if i == recoveryRouteLimit && response.Code != http.StatusTooManyRequests {
				t.Fatalf("%s spelled %s = %d, want 429 from the shared bucket", route.path, body, response.Code)
			}
		}
	}
}

// countingRecovery counts the recovery steps that reached the service.
type countingRecovery struct {
	acceptingRecovery
	calls *atomic.Int32
}

func (r countingRecovery) RecoveryOptions(ctx context.Context, host, taxID string) ([]domain.RecoveryContact, error) {
	r.calls.Add(1)
	return r.acceptingRecovery.RecoveryOptions(ctx, host, taxID)
}
func (r countingRecovery) RequestRecoveryCode(ctx context.Context, host, taxID string, contact uuid.UUID) (*domain.ContactChallenge, error) {
	r.calls.Add(1)
	return r.acceptingRecovery.RequestRecoveryCode(ctx, host, taxID, contact)
}
func (r countingRecovery) ResetWithRecoveryCode(ctx context.Context, host string, id uuid.UUID, code, password string) (*domain.CustomerLogin, error) {
	r.calls.Add(1)
	return r.acceptingRecovery.ResetWithRecoveryCode(ctx, host, id, code, password)
}

// The limiter refuses a body whose key it cannot read with its own envelope, before the handler.
// The options and code bodies are ones the handler would accept; a reset body the handler would
// accept always names an id, so there the envelope and the untouched service are the proof.
func TestRecoveryRoutesRefuseABodyWithoutAReadableKey(t *testing.T) {
	t.Parallel()
	for path, body := range map[string]string{
		"/auth/recovery/options": `{"tax_id":"not a cuit"}`,
		"/auth/recovery/code":    `{"tax_id":"not a cuit","contact_id":"` + uuid.NewString() + `"}`,
		"/auth/recovery/reset":   `{"code":"123456","password":"x"}`,
	} {
		calls := &atomic.Int32{}
		cfg := &config.Config{RateLimit: config.RateLimitConfig{Enabled: true, Window: time.Minute, Global: 100,
			Credentials: recoveryRouteLimit, Mail: recoveryRouteLimit}}
		router := NewRouter(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)),
			Handlers{CustomerAuth: handler.NewCustomerAuthHandler(countingRecovery{calls: calls})},
			Auth{Customer: tokenCustomers{}}, RateLimit{Limiter: ratelimit.NewMemory(nil)})
		response := callContactRoute(router, http.MethodPost, path, body, "", "10.0.2.1:1111")
		if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), `"code":"`+string(domain.CodeInvalidInput)+`"`) || calls.Load() != 0 {
			t.Fatalf("%s with %s = %d %s, service calls %d; want the limiter's 400 %s and no call", path, body, response.Code, response.Body, calls.Load(), domain.CodeInvalidInput)
		}
	}
}
