package services

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
	"github.com/santialemarino/coti/apps/api/internal/secrets"
)

type setupTestStore struct {
	setup   *domain.ARCASetup
	account uuid.UUID
	pending bool
}

func (f *setupTestStore) Exists(context.Context, repository.Querier, uuid.UUID) (bool, error) {
	return f.setup != nil, nil
}
func (f *setupTestStore) HasPendingInvoice(context.Context, repository.Querier, uuid.UUID) (bool, error) {
	return f.pending, nil
}

func (f *setupTestStore) Get(_ context.Context, _ repository.Querier, account uuid.UUID) (*domain.ARCASetup, error) {
	f.account = account
	if f.setup == nil {
		return nil, domain.ErrNotFound
	}
	copy := *f.setup
	return &copy, nil
}
func (f *setupTestStore) Create(_ context.Context, _ repository.Querier, account uuid.UUID, setup domain.ARCASetup) error {
	f.account = account
	f.setup = &setup
	return nil
}
func (f *setupTestStore) SaveCertificate(_ context.Context, _ repository.Querier, _, _ uuid.UUID, cert string, expires time.Time) error {
	f.setup.Certificate = cert
	f.setup.CertificateExpiresAt = &expires
	return nil
}
func (f *setupTestStore) Delete(_ context.Context, _ repository.Querier, _ uuid.UUID) error {
	f.setup = nil
	return nil
}

// setupTestAccounts records whether the account row was locked before the setup was written.
type setupTestAccounts struct {
	taxID       *string
	store       *setupTestStore
	lockedFirst bool
}

func (f *setupTestAccounts) GetByIDForUpdate(_ context.Context, _ repository.Querier, id uuid.UUID) (*domain.Account, error) {
	f.lockedFirst = f.store == nil || f.store.setup == nil
	return &domain.Account{ID: id, TaxID: f.taxID}, nil
}

type setupTestConnector struct {
	generatedFor string
	validations  int
}

func (f *setupTestConnector) Generate(taxID string) (string, string, error) {
	f.generatedFor = taxID
	return "private-key", "public-csr", nil
}
func (f *setupTestConnector) ValidateCertificate(string, string, string) (time.Time, error) {
	f.validations++
	return time.Now().Add(time.Hour), nil
}

func setupTestService(t *testing.T) (*ARCASetupService, *setupTestStore, *setupTestConnector, *secrets.AESGCM, domain.Tenant) {
	t.Helper()
	cuit := "20-32964233-0"
	s, store, connector, cipher, tenant, _ := setupTestServiceFor(t, &cuit)
	return s, store, connector, cipher, tenant
}

func setupTestServiceFor(t *testing.T, taxID *string) (*ARCASetupService, *setupTestStore, *setupTestConnector, *secrets.AESGCM, domain.Tenant, *setupTestAccounts) {
	t.Helper()
	cipher, err := secrets.NewAESGCM([]byte(strings.Repeat("x", 32)))
	if err != nil {
		t.Fatal(err)
	}
	store := &setupTestStore{}
	accounts := &setupTestAccounts{taxID: taxID, store: store}
	connector := &setupTestConnector{}
	service := NewARCASetupService(&fakeDB{}, store, accounts, cipher, connector)
	tenant := domain.Tenant{AccountID: uuid.New(), BranchID: uuid.New(), Role: domain.UserRoleAdmin}
	return service, store, connector, cipher, tenant, accounts
}

func TestARCASetupService_CredentialsAreEncryptedAndNeverReturned(t *testing.T) {
	t.Parallel()
	s, store, _, cipher, tenant := setupTestService(t)
	public, err := s.Create(context.Background(), tenant)
	if err != nil {
		t.Fatal(err)
	}
	if public.SealedKey != "" || public.CSR != "public-csr" {
		t.Fatalf("unsafe public setup %+v", public)
	}
	if strings.Contains(store.setup.SealedKey, "private-key") {
		t.Fatal("private key was stored in plaintext")
	}
	plain, err := cipher.Open(store.setup.SealedKey)
	if err != nil || plain != tenant.AccountID.String()+":private-key" {
		t.Fatal("key is not bound to its tenant")
	}
	read, _, err := s.Get(context.Background(), tenant)
	if err != nil || read.SealedKey != "" {
		t.Fatal("public read exposed credentials")
	}
}

func TestARCASetupService_RefusesSellersAndCrossAccountCredentials(t *testing.T) {
	t.Parallel()
	s, _, connector, _, tenant := setupTestService(t)
	seller := tenant
	seller.Role = domain.UserRoleSeller
	if _, err := s.Create(context.Background(), seller); !errors.Is(err, domain.ErrForbidden) {
		t.Fatal("seller generated credentials")
	}
	if _, err := s.Create(context.Background(), tenant); err != nil {
		t.Fatal(err)
	}
	other := tenant
	other.AccountID = uuid.New()
	if err := s.Upload(context.Background(), other, "certificate"); !errors.Is(err, domain.ErrNotConfigured) {
		t.Fatalf("accepted another account's key: %v", err)
	}
	if connector.validations != 0 {
		t.Fatal("a foreign key reached certificate validation")
	}
}

func TestARCASetupService_CreateUsesTheLockedAccountCUIT(t *testing.T) {
	t.Parallel()
	cuit := "20-32964233-0"
	s, store, connector, _, tenant, accounts := setupTestServiceFor(t, &cuit)
	setup, err := s.Create(context.Background(), tenant)
	if err != nil {
		t.Fatal(err)
	}
	if connector.generatedFor != "20329642330" || setup.TaxID != "20329642330" || store.setup.TaxID != "20329642330" {
		t.Fatalf("request for %q, stored %q, want the account CUIT as digits", connector.generatedFor, store.setup.TaxID)
	}
	if !accounts.lockedFirst {
		t.Fatal("the setup was written without locking the account first")
	}
	if len(s.db.(*fakeDB).scopes) != 1 {
		t.Fatalf("ran %d transactions, want the read and the write in one", len(s.db.(*fakeDB).scopes))
	}
}

func TestARCASetupService_CreateRefusesAnAccountWithoutAValidCUIT(t *testing.T) {
	t.Parallel()
	blank, fabricated := " ", "00-00000000-0"
	cases := []struct {
		name  string
		taxID *string
		want  domain.ErrorCode
	}{
		{"no CUIT", nil, domain.CodeAccountTaxIDRequired},
		{"blank CUIT", &blank, domain.CodeAccountTaxIDRequired},
		{"invalid CUIT", &fabricated, domain.CodeInvalidTaxID},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s, store, connector, _, tenant, _ := setupTestServiceFor(t, tc.taxID)
			_, err := s.Create(context.Background(), tenant)
			if !errors.Is(err, domain.ErrInvalidInput) || domain.CodeOf(err) != tc.want {
				t.Fatalf("err = %v (%s), want %s", err, domain.CodeOf(err), tc.want)
			}
			if store.setup != nil || connector.generatedFor != "" {
				t.Fatal("a request was generated without the account CUIT")
			}
		})
	}
}

func TestARCASetupService_DeleteWaitsForPendingInvoices(t *testing.T) {
	t.Parallel()
	s, store, _, _, tenant := setupTestService(t)
	if _, err := s.Create(context.Background(), tenant); err != nil {
		t.Fatal(err)
	}
	store.pending = true
	if err := s.Delete(context.Background(), tenant); domain.CodeOf(err) != domain.CodeInvoicePending {
		t.Fatalf("err = %v, want INVOICE_PENDING", err)
	}
	if store.setup == nil {
		t.Fatal("the credential went while an invoice waited for ARCA")
	}
	store.pending = false
	if err := s.Delete(context.Background(), tenant); err != nil || store.setup != nil {
		t.Fatalf("err = %v, setup = %v, want it removed", err, store.setup)
	}
}
