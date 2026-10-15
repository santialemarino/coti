//go:build integration

package repository

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

func TestHandlerDecisionRepository_Create_StoresTheUnreviewedProposal(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	accountID := seedAccount(t, db, "Handler decision")
	branchID := branchOf(t, db, accountID)
	productID := seedProduct(t, db, accountID, "Cemento Portland 50kg")
	priceCleanup(t, db, productID)
	_, versionID, _ := seedQuoteChain(t, db, accountID, branchID, productID)
	t.Cleanup(func() {
		mustCleanup(t, db.CrossAccount(), `DELETE FROM handler_decision WHERE quote_version_id = $1`, versionID)
	})

	input := "Agregar 5 bolsas de cal hidratada 25 kg"
	interpretation := "El cliente pidió agregar cinco bolsas de cal hidratada de 25 kg."
	proposal := `{"interpretation":"El cliente pidió agregar cinco bolsas de cal hidratada de 25 kg.","operations":[{"type":"ADD_ITEM"}]}`
	err := db.InTenantTx(ctx, domain.Tenant{AccountID: accountID, Role: domain.UserRoleAdmin},
		func(q Querier) error {
			return NewHandlerDecisionRepository().Create(ctx, q, accountID, domain.NewHandlerDecision{
				QuoteVersionID: versionID, ClientInput: input, AIInterpretation: interpretation,
				AIProposal: proposal, StateAtDecision: domain.QuoteStatusSent,
			})
		})
	if err != nil {
		t.Fatalf("Create handler decision: %v", err)
	}

	var gotInput, gotInterpretation, gotProposal string
	var gotState domain.QuoteStatus
	if err := db.CrossAccount().QueryRow(ctx,
		`SELECT client_input, ai_interpretation, ai_proposal, state_at_decision
		 FROM handler_decision WHERE account_id = $1 AND quote_version_id = $2`,
		accountID, versionID).Scan(&gotInput, &gotInterpretation, &gotProposal, &gotState); err != nil {
		t.Fatalf("read handler decision: %v", err)
	}
	if gotInput != input || gotInterpretation != interpretation || gotProposal != proposal {
		t.Errorf("stored proposal = (%q, %q, %q), want (%q, %q, %q)",
			gotInput, gotInterpretation, gotProposal, input, interpretation, proposal)
	}
	if gotState != domain.QuoteStatusSent {
		t.Errorf("state at decision = %q, want SENT", gotState)
	}
}

func TestHandlerDecisionRepository_Create_RefusesAnotherAccountsVersion(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	victimAccountID := seedAccount(t, db, "Victim handler decision")
	victimBranchID := branchOf(t, db, victimAccountID)
	victimProductID := seedProduct(t, db, victimAccountID, "Cemento Portland 50kg")
	priceCleanup(t, db, victimProductID)
	_, victimVersionID, _ := seedQuoteChain(t, db, victimAccountID, victimBranchID, victimProductID)
	attackerAccountID := seedAccount(t, db, "Attacker handler decision")

	err := db.InTenantTx(ctx, domain.Tenant{AccountID: attackerAccountID, Role: domain.UserRoleAdmin},
		func(q Querier) error {
			return NewHandlerDecisionRepository().Create(ctx, q, attackerAccountID, domain.NewHandlerDecision{
				QuoteVersionID: victimVersionID, ClientInput: "change", AIInterpretation: "change",
				AIProposal: "{}", StateAtDecision: domain.QuoteStatusSent,
			})
		})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("cross-account handler decision = %v, want ErrNotFound", err)
	}

	var written int
	if err := db.CrossAccount().QueryRow(ctx,
		`SELECT count(*) FROM handler_decision WHERE quote_version_id = $1`, victimVersionID).Scan(&written); err != nil {
		t.Fatalf("count victim handler decisions: %v", err)
	}
	if written != 0 {
		t.Errorf("cross-account attempt wrote %d decisions, want 0", written)
	}
}

// A decision names the batch that triggered it, and that batch must be the version's own quote's;
// the latest one on the version is what the seller reads, with its no-match reason.
func TestHandlerDecisionRepository_TiesTheBatchAndReadsTheLatest(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	accountID := seedAccount(t, db, "Handler decision batch")
	branchID := branchOf(t, db, accountID)
	productID := seedProduct(t, db, accountID, "Cemento Portland 50kg")
	priceCleanup(t, db, productID)
	quoteID, versionID, _ := seedQuoteChain(t, db, accountID, branchID, productID)
	otherQuoteID, _, _ := seedQuoteChain(t, db, accountID, branchID, productID)
	ownBatch, otherBatch := uuid.New(), uuid.New()
	for _, batch := range []struct{ id, quote uuid.UUID }{{ownBatch, quoteID}, {otherBatch, otherQuoteID}} {
		if _, err := db.CrossAccount().Exec(ctx,
			`INSERT INTO message_batch (id, account_id, quote_id, status, closes_at)
			 VALUES ($1, $2, $3, 'PROCESSING', now())`, batch.id, accountID, batch.quote); err != nil {
			t.Fatalf("seed batch: %v", err)
		}
	}
	t.Cleanup(func() {
		mustCleanup(t, db.CrossAccount(), `DELETE FROM handler_decision WHERE account_id = $1`, accountID)
		mustCleanup(t, db.CrossAccount(), `DELETE FROM message_batch WHERE account_id = $1`, accountID)
	})
	tenant := domain.Tenant{AccountID: accountID, Role: domain.UserRoleAdmin}
	repo := NewHandlerDecisionRepository()
	create := func(batchID uuid.UUID, proposal string) error {
		return db.InTenantTx(ctx, tenant, func(q Querier) error {
			return repo.Create(ctx, q, accountID, domain.NewHandlerDecision{QuoteVersionID: versionID,
				MessageBatchID: &batchID, ClientInput: "dale", AIInterpretation: "Acepta.",
				AIProposal: proposal, StateAtDecision: domain.QuoteStatusChangeRequested})
		})
	}

	if err := create(otherBatch, `{"outcome":"APPLIED"}`); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("a batch of another quote = %v, want ErrNotFound", err)
	}
	if err := create(ownBatch, `{"outcome":"APPLIED"}`); err != nil {
		t.Fatalf("Create() = %v", err)
	}
	if err := create(ownBatch, `{"outcome":"NO_MATCH","no_match_reason":"REQUIRES_SELLER"}`); err != nil {
		t.Fatalf("Create() = %v", err)
	}
	var note *domain.HandlerDecisionNote
	if err := db.InTenantTx(ctx, tenant, func(q Querier) error {
		var err error
		note, err = repo.GetLatestByVersionID(ctx, q, accountID, versionID)
		return err
	}); err != nil {
		t.Fatalf("GetLatestByVersionID() = %v", err)
	}
	if note.Outcome != domain.HandlerOutcomeNoMatch || note.NoMatchReason == nil ||
		*note.NoMatchReason != domain.HandlerNoMatchRequiresSeller || note.ClientInput != "dale" {
		t.Errorf("latest = %+v, want the REQUIRES_SELLER no-match", note)
	}
	var stored uuid.UUID
	if err := db.CrossAccount().QueryRow(ctx,
		`SELECT message_batch_id FROM handler_decision WHERE account_id = $1 LIMIT 1`, accountID).
		Scan(&stored); err != nil || stored != ownBatch {
		t.Errorf("stored batch = %v (%v), want %s", stored, err, ownBatch)
	}
}
