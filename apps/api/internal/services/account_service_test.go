package services

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

type accountTestDB struct{ fakeDB }

func (*accountTestDB) AdminTx(context.Context) (pgx.Tx, error) { return nil, errors.New("unused") }

type accountTestRepo struct {
	account   *domain.Account
	updated   *domain.AccountUpdate
	locked    bool
	switchSet *bool
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

func (r *accountTestRepo) SetInvoicingEnabled(_ context.Context, _ repository.Querier, _ uuid.UUID, enabled bool) (*domain.Account, error) {
	r.switchSet = &enabled
	return &domain.Account{Name: r.account.Name, InvoicingEnabled: enabled}, nil
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
	stored, other, formatted := "30-71234567-1", "20-32964233-0", "30712345671"
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

// The account's CUIT is what ARCA invoices carry, so the API holds the rule the form shows.
func TestAccountService_UpdateKeepsOnlyAValidCUIT(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		taxID string
		want  *string
	}{
		{"formatted", "20-32964233-0", ptr("20329642330")},
		{"blank means none", "   ", nil},
		{"wrong check digit", "20-32964233-1", nil},
		{"too short", "20-3296423", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			repo := &accountTestRepo{account: &domain.Account{Name: "Corralón"}}
			service := &AccountService{db: &accountTestDB{}, accounts: repo,
				arcaSetups: &accountTestARCASetups{}, invoices: &accountTestARCASetups{}}
			taxID := tc.taxID

			_, err := service.Update(context.Background(), domain.Tenant{AccountID: uuid.New()},
				domain.AccountUpdate{Name: "Corralón", TaxID: &taxID})

			wantRefused := tc.want == nil && strings.TrimSpace(tc.taxID) != ""
			if wantRefused {
				if domain.CodeOf(err) != domain.CodeInvalidTaxID {
					t.Fatalf("err = %v, want INVALID_TAX_ID", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			got := repo.updated.TaxID
			if (got == nil) != (tc.want == nil) || (got != nil && *got != *tc.want) {
				t.Fatalf("stored tax id = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestAccountService_UpdateDisconnectsARCAOnlyWhenConfirmed(t *testing.T) {
	t.Parallel()
	stored, other := "30-71234567-1", "20-32964233-0"
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

// Only a lower-price strategy may be the account's first choice; none keeps the stored one.
func TestAccountService_UpdateAcceptsOnlyALowerPriceStrategy(t *testing.T) {
	t.Parallel()
	adapt, quality := domain.HandlerStrategyAdaptToReachDiscount, domain.HandlerStrategyImproveQuality
	cases := []struct {
		name     string
		strategy *domain.HandlerStrategy
		refused  bool
	}{
		{"adapt", &adapt, false},
		{"kept", nil, false},
		{"not a lower-price strategy", &quality, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			repo := &accountTestRepo{account: &domain.Account{Name: "Corralón"}}
			service := &AccountService{db: &accountTestDB{}, accounts: repo,
				arcaSetups: &accountTestARCASetups{}, invoices: &accountTestARCASetups{}}

			_, err := service.Update(context.Background(), domain.Tenant{AccountID: uuid.New()},
				domain.AccountUpdate{Name: "Corralón", PreferredLowerPriceStrategy: tc.strategy})

			if tc.refused {
				if !errors.Is(err, domain.ErrInvalidInput) || repo.updated != nil {
					t.Fatalf("err = %v, updated = %v, want INVALID_INPUT and nothing written", err, repo.updated)
				}
				return
			}
			if err != nil || repo.updated == nil || repo.updated.PreferredLowerPriceStrategy != tc.strategy {
				t.Fatalf("err = %v, updated = %+v, want the strategy passed through", err, repo.updated)
			}
		})
	}
}

// Switching invoicing off waits for ARCA's pending answers, under the account row lock; on is never refused.
func TestAccountService_SetInvoicingEnabled(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		enabled  bool
		pending  bool
		wantCode domain.ErrorCode
	}{
		{"off", false, false, ""},
		{"off with an invoice waiting for ARCA", false, true, domain.CodeInvoicePending},
		{"on with an invoice waiting for ARCA", true, true, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			repo := &accountTestRepo{account: &domain.Account{Name: "Corralón", InvoicingEnabled: !tc.enabled}}
			invoices := &accountTestARCASetups{pending: tc.pending}
			service := &AccountService{db: &accountTestDB{}, accounts: repo,
				arcaSetups: &accountTestARCASetups{}, invoices: invoices}

			account, err := service.SetInvoicingEnabled(context.Background(), domain.Tenant{AccountID: uuid.New()}, tc.enabled)

			if !repo.locked {
				t.Fatal("the switch was written without locking the account row")
			}
			if tc.wantCode != "" {
				if !errors.Is(err, domain.ErrConflict) || domain.CodeOf(err) != tc.wantCode || repo.switchSet != nil {
					t.Fatalf("err = %v, written = %v, want %s and nothing written", err, repo.switchSet, tc.wantCode)
				}
				return
			}
			if err != nil || repo.switchSet == nil || *repo.switchSet != tc.enabled || account.InvoicingEnabled != tc.enabled {
				t.Fatalf("err = %v, written = %v, want the switch set to %v", err, repo.switchSet, tc.enabled)
			}
		})
	}
}
