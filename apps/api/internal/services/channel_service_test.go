package services

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

type fakeChannelReader struct {
	channels  []domain.Channel
	err       error
	accountID uuid.UUID
	branchID  uuid.UUID
}

func (f *fakeChannelReader) ListActiveByBranch(
	_ context.Context, _ repository.Querier, accountID, branchID uuid.UUID,
) ([]domain.Channel, error) {
	f.accountID = accountID
	f.branchID = branchID
	return f.channels, f.err
}

func TestChannelService_ListChannels_ScopesToSelectedBranch(t *testing.T) {
	db := &fakeDB{}
	want := []domain.Channel{{
		ID: testChannelID, AccountID: testAccountID, BranchID: testBranchID,
		Type: domain.ChannelTypeWhatsApp, IsActive: true,
	}}
	reader := &fakeChannelReader{channels: want}
	service := NewChannelService(db, reader)

	got, err := service.ListChannels(context.Background(), branchTenant())
	if err != nil {
		t.Fatalf("ListChannels() = %v, want no error", err)
	}
	if len(got) != 1 || got[0].ID != testChannelID {
		t.Fatalf("ListChannels() = %#v, want %#v", got, want)
	}
	if reader.accountID != testAccountID || reader.branchID != testBranchID {
		t.Errorf("repository scope = %v/%v, want %v/%v", reader.accountID, reader.branchID,
			testAccountID, testBranchID)
	}
	if len(db.scopes) != 1 || db.scopes[0] != testAccountID {
		t.Errorf("tenant scopes = %v, want [%v]", db.scopes, testAccountID)
	}
}

func TestChannelService_ListChannels_RequiresSelectedBranch(t *testing.T) {
	db := &fakeDB{}
	reader := &fakeChannelReader{}
	service := NewChannelService(db, reader)
	tenant := domain.Tenant{
		AccountID: testAccountID, UserID: testUserID, Role: domain.UserRoleAdmin,
	}

	_, err := service.ListChannels(context.Background(), tenant)
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("ListChannels() = %v, want %v", err, domain.ErrInvalidInput)
	}
	if len(db.scopes) != 0 {
		t.Errorf("tenant scopes = %v, want none without a selected branch", db.scopes)
	}
}
