//go:build integration

package repository

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

func seedOnboarding(t *testing.T, db *DB, accountID uuid.UUID) {
	t.Helper()
	if _, err := db.CrossAccount().Exec(context.Background(),
		`INSERT INTO account_onboarding (account_id, status) VALUES ($1, 'DISMISSED')`,
		accountID); err != nil {
		t.Fatalf("seed onboarding: %v", err)
	}
	t.Cleanup(func() {
		mustCleanup(t, db.CrossAccount(),
			`DELETE FROM onboarding_step_progress WHERE account_id = $1`, accountID)
		mustCleanup(t, db.CrossAccount(),
			`DELETE FROM account_onboarding WHERE account_id = $1`, accountID)
	})
}

func evidenceOf(t *testing.T, db *DB, accountID uuid.UUID) domain.OnboardingEvidence {
	t.Helper()
	var evidence domain.OnboardingEvidence
	if err := db.InTenantTx(context.Background(), domain.Tenant{AccountID: accountID},
		func(q Querier) error {
			var err error
			evidence, err = NewOnboardingRepository().GetEvidence(context.Background(), q, accountID)
			return err
		}); err != nil {
		t.Fatalf("GetEvidence: %v", err)
	}
	return evidence
}

// Each proof reads only its own account: another account's brand, catalog and team prove nothing.
func TestOnboardingRepository_GetEvidenceReadsTheAccountsOwnData(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	bare := seedAccount(t, db, "Corralón sin configurar")
	set := seedAccount(t, db, "Corralón configurado")
	seedUser(t, db, set, "ADMIN")
	seedUser(t, db, set, "SELLER")
	seedProduct(t, db, set, "Cemento")
	if _, err := db.CrossAccount().Exec(ctx,
		`UPDATE account SET brand_color = '#1d4ed8' WHERE id = $1`, set); err != nil {
		t.Fatalf("set brand: %v", err)
	}
	seedUser(t, db, bare, "ADMIN")

	for step, want := range map[domain.OnboardingStepKey]bool{
		domain.OnboardingStepBrand:         false,
		domain.OnboardingStepCatalogUpload: false,
		domain.OnboardingStepTeam:          false,
	} {
		if got := evidenceOf(t, db, bare)[step]; got != want {
			t.Errorf("bare account %s = %v, want %v", step, got, want)
		}
	}
	for _, step := range domain.OnboardingChecklistSteps {
		if !evidenceOf(t, db, set)[step] {
			t.Errorf("configured account %s = false, want true", step)
		}
	}
}

// A second user who is inactive is not a team, and an inactive product is not a catalog.
func TestOnboardingRepository_GetEvidenceIgnoresInactiveRows(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	accountID := seedAccount(t, db, "Corralón inactivo")
	seedUser(t, db, accountID, "ADMIN")
	seller := seedUser(t, db, accountID, "SELLER")
	product := seedProduct(t, db, accountID, "Arena")
	if _, err := db.CrossAccount().Exec(ctx,
		`UPDATE app_user SET is_active = FALSE WHERE id = $1`, seller); err != nil {
		t.Fatalf("deactivate seller: %v", err)
	}
	if _, err := db.CrossAccount().Exec(ctx,
		`UPDATE product SET is_active = FALSE WHERE id = $1`, product); err != nil {
		t.Fatalf("deactivate product: %v", err)
	}

	evidence := evidenceOf(t, db, accountID)
	if evidence[domain.OnboardingStepTeam] || evidence[domain.OnboardingStepCatalogUpload] {
		t.Fatalf("inactive rows counted as evidence: %v", evidence)
	}
}

func TestOnboardingRepository_UpdateChecklistHiddenSetsAndClearsTheStamp(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	repo := NewOnboardingRepository()

	accountID := seedAccount(t, db, "Corralón oculto")
	seedOnboarding(t, db, accountID)
	tenant := domain.Tenant{AccountID: accountID}

	read := func() *domain.Onboarding {
		t.Helper()
		var onboarding *domain.Onboarding
		if err := db.InTenantTx(ctx, tenant, func(q Querier) error {
			var err error
			onboarding, err = repo.GetByAccountID(ctx, q, accountID)
			return err
		}); err != nil {
			t.Fatalf("GetByAccountID: %v", err)
		}
		return onboarding
	}
	set := func(hidden bool) {
		t.Helper()
		if err := db.InTenantTx(ctx, tenant, func(q Querier) error {
			return repo.UpdateChecklistHidden(ctx, q, accountID, hidden)
		}); err != nil {
			t.Fatalf("UpdateChecklistHidden(%v): %v", hidden, err)
		}
	}

	if read().ChecklistHiddenAt != nil {
		t.Fatal("a new onboarding starts hidden")
	}
	set(true)
	hiddenAt := read().ChecklistHiddenAt
	if hiddenAt == nil {
		t.Fatal("hiding did not stamp checklist_hidden_at")
	}
	set(true)
	if again := read().ChecklistHiddenAt; again == nil || !again.Equal(*hiddenAt) {
		t.Fatalf("hiding twice moved the stamp from %v to %v", hiddenAt, again)
	}
	set(false)
	if read().ChecklistHiddenAt != nil {
		t.Fatal("showing did not clear checklist_hidden_at")
	}
}
