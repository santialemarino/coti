//go:build integration

package integration

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
	"github.com/santialemarino/coti/apps/api/internal/services"
)

// uniquePortalBase is a base host no other test shares, so labels collide only on purpose.
func uniquePortalBase() string {
	return "portal-" + strings.ReplaceAll(uuid.NewString(), "-", "")[:12] + ".test"
}

func (e *env) setPublicHostname(t *testing.T, accountID uuid.UUID, hostname string) {
	t.Helper()
	if _, err := e.db.CrossAccount().Exec(context.Background(),
		`UPDATE account SET public_hostname = $2 WHERE id = $1`, accountID, hostname); err != nil {
		t.Fatalf("set hostname: %v", err)
	}
}

func (e *env) publicHostname(t *testing.T, accountID uuid.UUID) string {
	t.Helper()
	var hostname string
	if err := e.db.CrossAccount().QueryRow(context.Background(),
		`SELECT COALESCE(public_hostname, '') FROM account WHERE id = $1`, accountID).Scan(&hostname); err != nil {
		t.Fatalf("read hostname: %v", err)
	}
	return hostname
}

// A new account's portal is its name as a label, the next free suffix when that is taken.
func TestDefaultPortal_RegistrationTakesTheNextFreeLabel(t *testing.T) {
	base := uniquePortalBase()
	e := newEnv(t, func(cfg *config.Config) { cfg.CustomerPortal.BaseURL = "https://" + base })
	holder, _ := e.seedAccount(t, "Holder")
	e.setPublicHostname(t, holder, "corralon-nandu."+base)

	suffix := uuid.NewString()
	e.dropAccountByAdminEmail(t, suffix+"@test.local")
	body := signupBody(suffix)
	body["account_name"] = "Corralón Ñandú"
	res := e.do(t, request{method: http.MethodPost, path: "/v1/public/accounts", body: body})
	if res.Code != http.StatusCreated {
		t.Fatalf("signup = %d, want 201 (body %s)", res.Code, res.Body.String())
	}
	var accountID uuid.UUID
	if err := e.db.CrossAccount().QueryRow(context.Background(),
		`SELECT account_id FROM app_user WHERE lower(email) = lower($1)`, suffix+"@test.local").Scan(&accountID); err != nil {
		t.Fatal(err)
	}
	if got := e.publicHostname(t, accountID); got != "corralon-nandu-2."+base {
		t.Fatalf("registered hostname = %q, want corralon-nandu-2.%s", got, base)
	}
}

// The backfill plans a portal for an account with none and for one holding the bare shared host,
// leaves every account with a label alone, and writes nothing on a dry run.
func TestDefaultPortal_BackfillPlansEveryAccountWithoutItsOwnPortal(t *testing.T) {
	base := uniquePortalBase()
	// Other tests leave accounts without a portal too, so every name here is this run's own.
	tag := strings.ReplaceAll(uuid.NewString(), "-", "")[:8]
	e := newEnv(t)
	none, _ := e.seedAccount(t, "Materiales Ñandú "+tag)
	bare, _ := e.seedAccount(t, "Marko "+tag)
	e.setPublicHostname(t, bare, base)
	labelled, _ := e.seedAccount(t, "Labelled")
	e.setPublicHostname(t, labelled, "marko-"+tag+"."+base)
	portals := services.NewDefaultPortalService(e.db, repository.NewCustomerPortalRepository(),
		config.CustomerPortalConfig{BaseURL: "https://" + base})

	plan, err := portals.Backfill(context.Background(), false)
	if err != nil {
		t.Fatalf("Backfill(dry run) = %v", err)
	}
	planned := map[uuid.UUID]domain.PortalAssignment{}
	for _, a := range plan {
		planned[a.AccountID] = a
	}
	wantNone := "materiales-nandu-" + tag + "." + base
	wantBare := "marko-" + tag + "-2." + base
	if got := planned[none]; got.Hostname != wantNone || got.Previous != "" {
		t.Errorf("account without a portal planned %+v, want %s", got, wantNone)
	}
	if got := planned[bare]; got.Hostname != wantBare || got.Previous != base {
		t.Errorf("account on the bare host planned %+v, want %s over %s", got, wantBare, base)
	}
	if _, ok := planned[labelled]; ok {
		t.Error("an account with its own label was planned again")
	}
	if e.publicHostname(t, none) != "" || e.publicHostname(t, bare) != base {
		t.Fatal("a dry run wrote")
	}

	// Writing only this test's rows: the shared database holds other tests' accounts too.
	repo := repository.NewCustomerPortalRepository()
	ctx := context.Background()
	tx, err := e.db.AdminTx(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	stale := planned[bare]
	stale.Previous = "elsewhere." + base
	if err := repo.AssignHostnamesCrossAccount(ctx, tx, []domain.PortalAssignment{planned[none], stale}); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("an assignment over a hostname that moved = %v, want ErrConflict", err)
	}
	_ = tx.Rollback(ctx)
	tx, err = e.db.AdminTx(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.AssignHostnamesCrossAccount(ctx, tx, []domain.PortalAssignment{planned[none], planned[bare]}); err != nil {
		t.Fatalf("assign = %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if e.publicHostname(t, none) != wantNone || e.publicHostname(t, bare) != wantBare {
		t.Fatal("the assignments were not written")
	}
}
