//go:build integration

package repository

import (
	"context"
	"errors"
	"testing"

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
