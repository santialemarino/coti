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
