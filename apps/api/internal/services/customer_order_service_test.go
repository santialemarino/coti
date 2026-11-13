package services

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

// pagedCustomerOrders holds a customer's whole history and serves the window asked for.
type pagedCustomerOrders struct{ orders []domain.CustomerOrder }

func (f *pagedCustomerOrders) List(
	_ context.Context, _ repository.Querier, _, _ uuid.UUID, limit, offset int,
) ([]domain.CustomerOrder, error) {
	if offset >= len(f.orders) {
		return []domain.CustomerOrder{}, nil
	}
	return f.orders[offset:min(offset+limit, len(f.orders))], nil
}

func (f *pagedCustomerOrders) Get(
	context.Context, repository.Querier, uuid.UUID, uuid.UUID, uuid.UUID,
) (*domain.CustomerOrder, error) {
	return nil, domain.ErrNotFound
}

func (f *pagedCustomerOrders) GetByToken(
	context.Context, repository.Querier, uuid.UUID, uuid.UUID, string,
) (*domain.CustomerOrder, error) {
	return nil, domain.ErrNotFound
}

func TestCustomerOrderService_List_SaysWhetherALaterPageExists(t *testing.T) {
	principal := domain.CustomerPrincipal{AccountID: uuid.New(), CustomerID: uuid.New()}
	for _, tc := range []struct {
		name        string
		total, page int
		wantLen     int
		wantMore    bool
	}{
		{"a full page with more after it", 5, 1, 2, true},
		{"a short last page", 5, 3, 1, false},
		{"a last page that is exactly full", 4, 2, 2, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &pagedCustomerOrders{orders: make([]domain.CustomerOrder, tc.total)}
			orders, more, err := NewCustomerOrderService(&fakeQuoteDB{}, store, 2).
				List(context.Background(), principal, tc.page)
			if err != nil || len(orders) != tc.wantLen || more != tc.wantMore {
				t.Fatalf("List() = %d orders, more %v, %v; want %d, %v", len(orders), more, err, tc.wantLen, tc.wantMore)
			}
		})
	}
}
