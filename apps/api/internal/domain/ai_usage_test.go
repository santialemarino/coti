package domain

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func TestWithAIAccount_KeepsTheOrderOnlyWithinItsAccount(t *testing.T) {
	account, rfqID := uuid.New(), uuid.New()
	ctx := WithAIRFQ(WithAIAccount(context.Background(), Tenant{AccountID: account}), rfqID)

	same := AIUsageScopeFrom(WithAIAccount(ctx, Tenant{AccountID: account, BranchID: uuid.New()}))
	if same.RFQID == nil || *same.RFQID != rfqID || same.BranchID == nil {
		t.Errorf("scope = %+v, want the order kept and the branch set", same)
	}
	other := AIUsageScopeFrom(WithAIAccount(ctx, Tenant{AccountID: uuid.New()}))
	if other.RFQID != nil || other.BranchID != nil {
		t.Errorf("scope = %+v, want another account's order and branch dropped", other)
	}
}

// The text pipeline names the order before the account; naming the account first is not a change.
func TestWithAIAccount_KeepsAnOrderNamedBeforeAnyAccount(t *testing.T) {
	account, rfqID := uuid.New(), uuid.New()
	ctx := WithAIAccount(WithAIRFQ(context.Background(), rfqID), Tenant{AccountID: account})

	scope := AIUsageScopeFrom(ctx)
	if scope.AccountID != account || scope.RFQID == nil || *scope.RFQID != rfqID {
		t.Errorf("scope = %+v, want the account set and the order kept", scope)
	}
}
