package services

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

// pagedCustomerOrders holds a customer's whole history and serves the window asked for.
type pagedCustomerOrders struct {
	orders []domain.CustomerOrder
	env    domain.ARCAEnvironment
}

func (f *pagedCustomerOrders) List(
	_ context.Context, _ repository.Querier, _, _ uuid.UUID, limit, offset int, env domain.ARCAEnvironment,
) ([]domain.CustomerOrder, error) {
	f.env = env
	if offset >= len(f.orders) {
		return []domain.CustomerOrder{}, nil
	}
	return f.orders[offset:min(offset+limit, len(f.orders))], nil
}

func (f *pagedCustomerOrders) Get(
	_ context.Context, _ repository.Querier, _, _, id uuid.UUID, env domain.ARCAEnvironment,
) (*domain.CustomerOrder, error) {
	f.env = env
	for _, order := range f.orders {
		if order.ID == id {
			return &order, nil
		}
	}
	return nil, domain.ErrNotFound
}

func (f *pagedCustomerOrders) GetByToken(
	context.Context, repository.Querier, uuid.UUID, uuid.UUID, string, domain.ARCAEnvironment,
) (*domain.CustomerOrder, error) {
	return nil, domain.ErrNotFound
}

// A customer sees an accepted sale as finished by the same rule that closes it in the backoffice:
// paid and invoiced, waived or not invoicing at all. Anything short of that stays accepted.
func TestCustomerOrderService_ReportsACompleteSaleAsCompleted(t *testing.T) {
	principal := domain.CustomerPrincipal{AccountID: uuid.New(), CustomerID: uuid.New()}
	paid := time.Now()
	orders := []domain.CustomerOrder{
		{ID: uuid.New(), Status: "ACCEPTED", Closure: domain.SaleClosure{PaidAt: &paid, Invoiced: true, InvoicingEnabled: true}},
		{ID: uuid.New(), Status: "ACCEPTED", Closure: domain.SaleClosure{PaidAt: &paid, InvoicingEnabled: false}},
		{ID: uuid.New(), Status: "ACCEPTED", Closure: domain.SaleClosure{PaidAt: &paid, InvoicingEnabled: true}},
		{ID: uuid.New(), Status: "ACCEPTED", Closure: domain.SaleClosure{Invoiced: true, InvoicingEnabled: true}},
		{ID: uuid.New(), Status: "ACCEPTED", Closure: domain.SaleClosure{PaidAt: &paid, Invoiced: true, InvoicePending: true, InvoicingEnabled: true}},
		{ID: uuid.New(), Status: "SENT", Closure: domain.SaleClosure{PaidAt: &paid, InvoicingEnabled: false}},
	}
	want := []string{"COMPLETED", "COMPLETED", "ACCEPTED", "ACCEPTED", "ACCEPTED", "SENT"}
	store := &pagedCustomerOrders{orders: orders}
	service := NewCustomerOrderService(&fakeQuoteDB{}, store, 10).WithInvoicingEnvironment(domain.ARCAEnvironment("HOMOLOGATION"))

	listed, _, err := service.List(context.Background(), principal, 1)
	if err != nil {
		t.Fatalf("List() = %v", err)
	}
	for i, order := range listed {
		if order.Status != want[i] {
			t.Errorf("order %d listed as %q, want %q", i, order.Status, want[i])
		}
	}
	if store.env != "HOMOLOGATION" {
		t.Errorf("the store read closure facts for environment %q, want HOMOLOGATION", store.env)
	}
	got, err := service.Get(context.Background(), principal, orders[0].ID)
	if err != nil || got.Status != "COMPLETED" {
		t.Fatalf("Get() = %+v, %v; want COMPLETED", got, err)
	}
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
