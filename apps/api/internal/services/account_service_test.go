package services

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

type accountTestDB struct{ fakeDB }

func (*accountTestDB) AdminTx(context.Context) (pgx.Tx, error) { return nil, errors.New("unused") }

type accountTestRepo struct {
	account *domain.Account
	updated *domain.AccountUpdate
	locked  bool
}

func (r *accountTestRepo) GetByID(context.Context, repository.Querier, uuid.UUID) (*domain.Account, error) {
	copy := *r.account
	return &copy, nil
}

func (r *accountTestRepo) GetByIDForUpdate(ctx context.Context, q repository.Querier, id uuid.UUID) (*domain.Account, error) {
	r.locked = true
	return r.GetByID(ctx, q, id)
}

func (r *accountTestRepo) Create(context.Context, repository.Querier, string, *string, *string) (*domain.Account, error) {
	return nil, errors.New("unused")
}

func (r *accountTestRepo) Update(_ context.Context, _ repository.Querier, _ uuid.UUID, in domain.AccountUpdate) (*domain.Account, error) {
	r.updated = &in
	return &domain.Account{Name: in.Name, TaxID: in.TaxID}, nil
}

type accountTestARCASetups struct {
	connected bool
	pending   bool
	deleted   bool
}

func (f *accountTestARCASetups) Exists(context.Context, repository.Querier, uuid.UUID) (bool, error) {
	return f.connected, nil
}

func (f *accountTestARCASetups) HasPendingInvoice(context.Context, repository.Querier, uuid.UUID) (bool, error) {
	return f.pending, nil
}

func (f *accountTestARCASetups) Delete(context.Context, repository.Querier, uuid.UUID) error {
	f.deleted = true
	return nil
}

func TestAccountService_UpdateLocksTheCUITWhileARCAIsConnected(t *testing.T) {
	t.Parallel()
	stored, other, formatted := "30-71234567-9", "20-32964233-0", "30712345679"
	cases := []struct {
		name      string
		connected bool
		taxID     *string
		refused   bool
	}{
		{"another CUIT while connected", true, &other, true},
		{"no CUIT while connected", true, nil, true},
		{"the same CUIT written differently while connected", true, &formatted, false},
		{"another CUIT while not connected", false, &other, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			repo := &accountTestRepo{account: &domain.Account{Name: "Corralón", TaxID: &stored}}
			service := &AccountService{db: &accountTestDB{}, accounts: repo,
				arcaSetups: &accountTestARCASetups{connected: tc.connected}, invoices: &accountTestARCASetups{}}

			_, err := service.Update(context.Background(), domain.Tenant{AccountID: uuid.New()},
				domain.AccountUpdate{Name: "Corralón", TaxID: tc.taxID})

			if tc.refused {
				if !errors.Is(err, domain.ErrConflict) || domain.CodeOf(err) != domain.CodeARCAConnected || repo.updated != nil {
					t.Fatalf("err = %v, updated = %v, want ARCA_CONNECTED and nothing written", err, repo.updated)
				}
				return
			}
			if err != nil || repo.updated == nil {
				t.Fatalf("err = %v, want the account updated", err)
			}
			if !repo.locked {
				t.Fatal("the CUIT was compared without locking the account row")
			}
		})
	}
}

func TestAccountService_UpdateDisconnectsARCAOnlyWhenConfirmed(t *testing.T) {
	t.Parallel()
	stored, other := "30-71234567-9", "20-32964233-0"
	cases := []struct {
		name     string
		confirm  bool
		pending  bool
		wantCode domain.ErrorCode
		deleted  bool
	}{
		{"unconfirmed", false, false, domain.CodeARCAConnected, false},
		{"confirmed", true, false, "", true},
		{"confirmed with an invoice waiting for ARCA", true, true, domain.CodeInvoicePending, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			repo := &accountTestRepo{account: &domain.Account{Name: "Corralón", TaxID: &stored}}
			setups := &accountTestARCASetups{connected: true, pending: tc.pending}
			service := &AccountService{db: &accountTestDB{}, accounts: repo, arcaSetups: setups, invoices: setups}

			_, err := service.Update(context.Background(), domain.Tenant{AccountID: uuid.New()},
				domain.AccountUpdate{Name: "Corralón", TaxID: &other, DisconnectARCA: tc.confirm})

			if tc.wantCode != "" {
				if domain.CodeOf(err) != tc.wantCode || repo.updated != nil || setups.deleted {
					t.Fatalf("err = %v, updated = %v, deleted = %v, want %s and nothing written", err, repo.updated, setups.deleted, tc.wantCode)
				}
				return
			}
			if err != nil || repo.updated == nil || !setups.deleted {
				t.Fatalf("err = %v, updated = %v, deleted = %v, want both written", err, repo.updated, setups.deleted)
			}
		})
	}
}
