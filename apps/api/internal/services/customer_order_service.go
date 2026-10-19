package services

import (
	"context"
	"math"

	"github.com/google/uuid"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

type customerOrderStore interface {
	List(context.Context, repository.Querier, uuid.UUID, string, int, int) ([]domain.CustomerOrder, error)
	Get(context.Context, repository.Querier, uuid.UUID, string, uuid.UUID) (*domain.CustomerOrder, error)
	GetByToken(context.Context, repository.Querier, uuid.UUID, string, string) (*domain.CustomerOrder, error)
}

// CustomerOrderService authorizes history by authenticated CUIT/CUIL within the resolved supplier.
type CustomerOrderService struct {
	db       tenantTxRunner
	store    customerOrderStore
	pageSize int
}

// NewCustomerOrderService builds the customer history service.
func NewCustomerOrderService(db tenantTxRunner, store customerOrderStore, pageSize int) *CustomerOrderService {
	return &CustomerOrderService{db: db, store: store, pageSize: pageSize}
}

// List reads one page of requests for the authenticated customer.
func (s *CustomerOrderService) List(ctx context.Context, p domain.CustomerPrincipal, page int) ([]domain.CustomerOrder, error) {
	if page < 1 || s.pageSize < 1 || page > math.MaxInt/s.pageSize {
		return nil, domain.ErrInvalidInput
	}
	var result []domain.CustomerOrder
	err := s.db.InTenantTx(ctx, domain.Tenant{AccountID: p.AccountID}, func(q repository.Querier) error {
		var err error
		result, err = s.store.List(ctx, q, p.AccountID, p.TaxID, s.pageSize, (page-1)*s.pageSize)
		return err
	})
	return result, err
}

// Get authorizes a request before exposing its original content or publication reference.
func (s *CustomerOrderService) Get(ctx context.Context, p domain.CustomerPrincipal, id uuid.UUID) (*domain.CustomerOrder, error) {
	var result *domain.CustomerOrder
	err := s.db.InTenantTx(ctx, domain.Tenant{AccountID: p.AccountID}, func(q repository.Querier) error {
		var err error
		result, err = s.store.Get(ctx, q, p.AccountID, p.TaxID, id)
		return err
	})
	return result, err
}

// GetByToken checks the customer before the delivery resolver reads its immutable representation.
func (s *CustomerOrderService) GetByToken(ctx context.Context, p domain.CustomerPrincipal, token string) (*domain.CustomerOrder, error) {
	var result *domain.CustomerOrder
	err := s.db.InTenantTx(ctx, domain.Tenant{AccountID: p.AccountID}, func(q repository.Querier) error {
		var err error
		result, err = s.store.GetByToken(ctx, q, p.AccountID, p.TaxID, token)
		return err
	})
	return result, err
}
