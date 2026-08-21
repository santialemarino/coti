package services

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

// rfqRepository is the persistence surface a manual RFQ entry needs.
type rfqRepository interface {
	ListByTenant(ctx context.Context, q repository.Querier, tenant domain.Tenant) ([]domain.RfqListItem, error)
	GetManualEntryChannelID(ctx context.Context, q repository.Querier, accountID, branchID uuid.UUID) (uuid.UUID, error)
	CountProductsInAccount(ctx context.Context, q repository.Querier, accountID uuid.UUID, productIDs []uuid.UUID) (int, error)
	CreateManualEntry(ctx context.Context, q repository.Querier, tenant domain.Tenant, channelID uuid.UUID, in domain.NewRfq, now time.Time) (*domain.RfqCreation, error)
}

// RfqService owns how requests become quotes. Manual entry needs no AI: the seller
// types the order and it is GENERATED at once.
type RfqService struct {
	db   tenantTxRunner
	rfqs rfqRepository
	now  func() time.Time
}

// NewRfqService builds an RfqService. now is injectable so the received_at a manual
// entry writes is deterministic in tests.
func NewRfqService(db tenantTxRunner, rfqs rfqRepository, now func() time.Time) *RfqService {
	if now == nil {
		now = time.Now
	}
	return &RfqService{db: db, rfqs: rfqs, now: now}
}

// List returns the RFQ list for the caller's tenant scope.
func (s *RfqService) List(ctx context.Context, tenant domain.Tenant) ([]domain.RfqListItem, error) {
	var items []domain.RfqListItem
	if err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		var err error
		items, err = s.rfqs.ListByTenant(ctx, q, tenant)
		return err
	}); err != nil {
		return nil, err
	}
	return items, nil
}

// CreateManual records a counter, phone or otherwise unintegrated order: the RFQ is
// born GENERATED and its quote DRAFT in one transaction, with the typed lines.
func (s *RfqService) CreateManual(
	ctx context.Context, tenant domain.Tenant, in domain.NewRfq,
) (*domain.RfqCreation, error) {
	if err := requireBranch(tenant, "a manual RFQ"); err != nil {
		return nil, err
	}
	in, err := normalizeManualEntry(in)
	if err != nil {
		return nil, err
	}

	var creation *domain.RfqCreation
	if err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		channelID, getErr := s.rfqs.GetManualEntryChannelID(ctx, q, tenant.AccountID, tenant.BranchID)
		if getErr != nil {
			return getErr
		}
		if assertErr := s.assertProductsInAccount(ctx, q, tenant.AccountID, in.Items); assertErr != nil {
			return assertErr
		}
		var createErr error
		creation, createErr = s.rfqs.CreateManualEntry(ctx, q, tenant, channelID, in, s.now())
		return createErr
	}); err != nil {
		return nil, err
	}
	return creation, nil
}

// normalizeManualEntry trims what the seller typed and refuses an order with neither
// text nor lines.
func normalizeManualEntry(in domain.NewRfq) (domain.NewRfq, error) {
	if in.RawText != nil {
		trimmed := strings.TrimSpace(*in.RawText)
		in.RawText = &trimmed
	}
	if in.WorkType != nil {
		trimmed := strings.TrimSpace(*in.WorkType)
		in.WorkType = &trimmed
	}
	if in.ClientLabel != nil {
		trimmed := strings.TrimSpace(*in.ClientLabel)
		in.ClientLabel = &trimmed
	}
	if len(in.Items) == 0 && (in.RawText == nil || *in.RawText == "") {
		return domain.NewRfq{}, fmt.Errorf("%w: a manual entry needs raw_text or at least one item",
			domain.ErrInvalidInput)
	}
	for i := range in.Items {
		if err := normalizeManualItem(&in.Items[i], i+1); err != nil {
			return domain.NewRfq{}, err
		}
	}
	return in, nil
}

func normalizeManualItem(it *domain.NewRfqItem, index int) error {
	it.RequestedDescription = strings.TrimSpace(it.RequestedDescription)
	if it.RequestedDescription == "" {
		return fmt.Errorf("%w: item %d needs a requested_description", domain.ErrInvalidInput, index)
	}
	if it.Unit != nil {
		trimmed := strings.TrimSpace(*it.Unit)
		if trimmed == "" {
			it.Unit = nil
		} else {
			it.Unit = &trimmed
		}
	}
	if it.Quantity.LessThanOrEqual(decimal.Zero) {
		return fmt.Errorf("%w: item %d quantity must be greater than zero", domain.ErrInvalidInput, index)
	}
	return validateAmount(it.Quantity, fmt.Sprintf("item %d quantity", index))
}

// assertProductsInAccount fails closed on a line that names a product outside the
// account, batch-checked so a long list costs one query.
func (s *RfqService) assertProductsInAccount(
	ctx context.Context, q repository.Querier, accountID uuid.UUID, items []domain.NewRfqItem,
) error {
	ids := make([]uuid.UUID, 0, len(items))
	for _, it := range items {
		if it.ProductID != nil {
			ids = append(ids, *it.ProductID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	owned, err := s.rfqs.CountProductsInAccount(ctx, q, accountID, ids)
	if err != nil {
		return err
	}
	if owned != len(ids) {
		return domain.ErrNotFound
	}
	return nil
}
