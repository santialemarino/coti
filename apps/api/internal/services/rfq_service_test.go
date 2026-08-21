package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

// Manual RFQ entry is service-level business logic (born GENERATED, quote born DRAFT,
// branch- and account-scoped); the SQL that materializes it is exercised against a real
// database in internal/repository.

var manualChannelID = uuid.MustParse("66666666-6666-4666-8666-666666666666")
var knownProduct = uuid.MustParse("77777777-7777-4777-8777-777777777777")
var foreignProduct = uuid.MustParse("88888888-8888-4888-8888-888888888888")

type fakeRfqRepo struct {
	channelID  uuid.UUID
	channelErr error
	owned      int
	creation   *domain.RfqCreation
	createErr  error
	receivedAt time.Time
	created    []domain.NewRfq
	listItems  []domain.RfqListItem
}

func (f *fakeRfqRepo) ListByTenant(
	_ context.Context, _ repository.Querier, _ domain.Tenant,
) ([]domain.RfqListItem, error) {
	return f.listItems, nil
}

func (f *fakeRfqRepo) GetManualEntryChannelID(
	_ context.Context, _ repository.Querier, _, _ uuid.UUID,
) (uuid.UUID, error) {
	if f.channelErr != nil {
		return uuid.Nil, f.channelErr
	}
	return f.channelID, nil
}

func (f *fakeRfqRepo) CountProductsInAccount(
	_ context.Context, _ repository.Querier, _ uuid.UUID, productIDs []uuid.UUID,
) (int, error) {
	if len(productIDs) == 0 {
		return 0, nil
	}
	return f.owned, nil
}

func (f *fakeRfqRepo) CreateManualEntry(
	_ context.Context, _ repository.Querier, tenant domain.Tenant, channelID uuid.UUID,
	in domain.NewRfq, now time.Time,
) (*domain.RfqCreation, error) {
	f.created = append(f.created, in)
	f.receivedAt = now
	if f.createErr != nil {
		return nil, f.createErr
	}
	if f.creation != nil {
		return f.creation, nil
	}
	return &domain.RfqCreation{
		Rfq: domain.Rfq{
			ID: uuid.New(), BranchID: tenant.BranchID, ChannelID: channelID,
			RawText: in.RawText, WorkType: in.WorkType, ClientLabel: in.ClientLabel,
			Status: domain.RFQStatusGenerated, ReceivedAt: now,
		},
		Quote: domain.Quote{
			ID: uuid.New(), BranchID: tenant.BranchID, SellerID: &tenant.UserID,
			CurrentStatus: domain.QuoteStatusDraft,
		},
	}, nil
}

func rfqHarness(repo *fakeRfqRepo) (*RfqService, *fakeDB) {
	db := &fakeDB{}
	svc := NewRfqService(db, repo, func() time.Time { return fixedNow })
	return svc, db
}

func manualItems() []domain.NewRfqItem {
	return []domain.NewRfqItem{{
		ProductID:            &knownProduct,
		RequestedDescription: "Cemento Loma Negra x50",
		Quantity:             decimal.RequireFromString("2.5"),
		Unit:                 strPtr("bolsa"),
	}}
}

func strPtr(s string) *string { return &s }

// The heart of the ticket: a manual order skips the AI entirely and is GENERATED at
// once, its quote born DRAFT and scoped to the active branch.
func TestRfqService_CreateManual_BornGeneratedAndDraft(t *testing.T) {
	raw := strPtr("  pedido de hoy  ")
	repo := &fakeRfqRepo{channelID: manualChannelID, owned: 1}
	svc, db := rfqHarness(repo)

	creation, err := svc.CreateManual(context.Background(), branchTenant(), domain.NewRfq{
		RawText:     raw,
		ClientLabel: strPtr("  Pérez  "),
		Items:       manualItems(),
	})
	if err != nil {
		t.Fatalf("CreateManual returned an unexpected error: %v", err)
	}
	if creation.Rfq.Status != domain.RFQStatusGenerated {
		t.Errorf("RFQ status = %s, want GENERATED", creation.Rfq.Status)
	}
	if creation.Quote.CurrentStatus != domain.QuoteStatusDraft {
		t.Errorf("quote status = %s, want DRAFT", creation.Quote.CurrentStatus)
	}
	if !creation.Rfq.ReceivedAt.Equal(fixedNow) {
		t.Errorf("received_at = %v, want the injected clock %v", creation.Rfq.ReceivedAt, fixedNow)
	}
	if creation.Quote.SellerID == nil || *creation.Quote.SellerID != testUserID {
		t.Errorf("quote seller_id = %v, want the caller %v", creation.Quote.SellerID, testUserID)
	}
	if creation.Rfq.ChannelID != manualChannelID {
		t.Errorf("rfq channel = %v, want the manual-entry channel %v", creation.Rfq.ChannelID, manualChannelID)
	}

	if got := *creation.Rfq.ClientLabel; got != "Pérez" {
		t.Errorf("client label was not trimmed: %q", got)
	}
	if got := *creation.Rfq.RawText; got != "pedido de hoy" {
		t.Errorf("raw_text was not trimmed: %q", got)
	}
	if len(repo.created) != 1 {
		t.Fatalf("CreateManualEntry called %d times, want 1", len(repo.created))
	}
	item := repo.created[0].Items[0]
	if item.Quantity.String() != "2.5" {
		t.Errorf("quantity = %s, want 2.5", item.Quantity)
	}
	if item.Unit == nil || *item.Unit != "bolsa" {
		t.Errorf("unit = %v, want bolsa", item.Unit)
	}
	if item.ProductID == nil || *item.ProductID != knownProduct {
		t.Errorf("product_id = %v, want the known product", item.ProductID)
	}

	if len(db.scopes) != 1 || db.scopes[0] != testAccountID {
		t.Errorf("transaction scoped to %v, want [%v]", db.scopes, testAccountID)
	}
}

func TestRfqService_CreateManual_NeedsAnActiveBranch(t *testing.T) {
	repo := &fakeRfqRepo{channelID: manualChannelID}
	svc, _ := rfqHarness(repo)
	tenant := domain.Tenant{AccountID: testAccountID, UserID: testUserID}

	_, err := svc.CreateManual(context.Background(), tenant, domain.NewRfq{Items: manualItems()})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("err = %v, want ErrInvalidInput", err)
	}
}

func TestRfqService_CreateManual_NeedsTextOrItems(t *testing.T) {
	repo := &fakeRfqRepo{channelID: manualChannelID}
	svc, _ := rfqHarness(repo)

	_, err := svc.CreateManual(context.Background(), branchTenant(), domain.NewRfq{})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("err = %v, want ErrInvalidInput", err)
	}
	if got := len(repo.created); got != 0 {
		t.Fatalf("CreateManualEntry called %d times, want 0", got)
	}
}

func TestRfqService_CreateManual_RejectsBadItems(t *testing.T) {
	cases := []struct {
		name string
		item domain.NewRfqItem
	}{
		{"empty description", domain.NewRfqItem{
			RequestedDescription: "   ", Quantity: decimal.NewFromInt(1),
		}},
		{"zero quantity", domain.NewRfqItem{
			RequestedDescription: "Cemento", Quantity: decimal.Zero,
		}},
		{"negative quantity", domain.NewRfqItem{
			RequestedDescription: "Cemento", Quantity: decimal.NewFromInt(-3),
		}},
		{"too many decimals", domain.NewRfqItem{
			RequestedDescription: "Cemento", Quantity: decimal.RequireFromString("2.500"),
		}},
		{"over moneyMax", domain.NewRfqItem{
			RequestedDescription: "Cemento", Quantity: decimal.RequireFromString("999999999999.99").Add(decimal.NewFromInt(1)),
		}},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeRfqRepo{channelID: manualChannelID}
			svc, _ := rfqHarness(repo)

			_, err := svc.CreateManual(context.Background(), branchTenant(),
				domain.NewRfq{Items: []domain.NewRfqItem{tc.item}})
			if !errors.Is(err, domain.ErrInvalidInput) {
				t.Fatalf("err = %v, want ErrInvalidInput", err)
			}
		})
	}
}

func TestRfqService_CreateManual_ProductOutsideAccountFailsClosed(t *testing.T) {
	repo := &fakeRfqRepo{channelID: manualChannelID, owned: 1}
	svc, _ := rfqHarness(repo)

	items := append(manualItems(), domain.NewRfqItem{
		ProductID: &foreignProduct, RequestedDescription: "Ajeno", Quantity: decimal.NewFromInt(1),
	})
	_, err := svc.CreateManual(context.Background(), branchTenant(), domain.NewRfq{Items: items})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
	if got := len(repo.created); got != 0 {
		t.Fatalf("CreateManualEntry called %d times, want 0", got)
	}
}

func TestRfqService_CreateManual_MissingChannelIsPropagated(t *testing.T) {
	repo := &fakeRfqRepo{channelErr: domain.ErrNotFound}
	svc, _ := rfqHarness(repo)

	_, err := svc.CreateManual(context.Background(), branchTenant(), domain.NewRfq{Items: manualItems()})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestRfqService_CreateManual_RepositoryErrorRollsBack(t *testing.T) {
	repo := &fakeRfqRepo{channelID: manualChannelID, owned: 1, createErr: domain.ErrConflict}
	svc, _ := rfqHarness(repo)

	_, err := svc.CreateManual(context.Background(), branchTenant(), domain.NewRfq{Items: manualItems()})
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("err = %v, want ErrConflict", err)
	}
}
