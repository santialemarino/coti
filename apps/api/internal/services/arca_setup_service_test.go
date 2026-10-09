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
	setup           *domain.ARCASetup
	account, branch uuid.UUID
	branchErr       error
	branchWrites    int
	ticketWrites    int
}

func (f *setupTestStore) Get(_ context.Context, _ repository.Querier, account, branch uuid.UUID) (*domain.ARCASetup, error) {
	f.account, f.branch = account, branch
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
func (f *setupTestStore) SaveTicket(_ context.Context, _ repository.Querier, _, _ uuid.UUID, ticket string) error {
	f.setup.SealedTicket = ticket
	f.ticketWrites++
	return nil
}
func (f *setupTestStore) SaveBranch(_ context.Context, _ repository.Querier, account, branch, _ uuid.UUID, _ int) error {
	f.account, f.branch = account, branch
	f.branchWrites++
	return f.branchErr
}
func (f *setupTestStore) Delete(_ context.Context, _ repository.Querier, _ uuid.UUID) error {
	f.setup = nil
	return nil
}

type setupTestConnector struct {
	result     domain.ARCAConnectionResult
	seenTicket domain.ARCATicket
	calls      int
}

func (f *setupTestConnector) Generate(string) (string, string, error) {
	return "private-key", "public-csr", nil
}
func (f *setupTestConnector) ValidateCertificate(string, string, string) (time.Time, error) {
	return time.Now().Add(time.Hour), nil
}
func (f *setupTestConnector) Verify(_ context.Context, _, _, _ string, _ int, ticket domain.ARCATicket) (domain.ARCAConnectionResult, error) {
	f.calls++
	f.seenTicket = ticket
	return f.result, nil
}

func setupTestService(t *testing.T) (*ARCASetupService, *setupTestStore, *setupTestConnector, *secrets.AESGCM, domain.Tenant) {
	t.Helper()
	cipher, err := secrets.NewAESGCM([]byte(strings.Repeat("x", 32)))
	if err != nil {
		t.Fatal(err)
	}
	store := &setupTestStore{}
	connector := &setupTestConnector{}
	service := NewARCASetupService(&fakeDB{}, store, cipher, connector)
	tenant := domain.Tenant{AccountID: uuid.New(), BranchID: uuid.New(), Role: domain.UserRoleAdmin}
	return service, store, connector, cipher, tenant
}

func TestARCASetupService_CredentialsAreEncryptedAndNeverReturned(t *testing.T) {
	t.Parallel()
	s, store, _, cipher, tenant := setupTestService(t)
	public, err := s.Create(context.Background(), tenant, "20-32964233-0")
	if err != nil {
		t.Fatal(err)
	}
	if public.SealedKey != "" || public.SealedTicket != "" || public.CSR != "public-csr" {
		t.Fatalf("unsafe public setup %+v", public)
	}
	if strings.Contains(store.setup.SealedKey, "private-key") {
		t.Fatal("private key was stored in plaintext")
	}
	plain, err := cipher.Open(store.setup.SealedKey)
	if err != nil || plain != tenant.AccountID.String()+":private-key" {
		t.Fatal("key is not bound to its tenant")
	}
	store.setup.SealedTicket = "secret-ticket"
	read, _, err := s.Get(context.Background(), tenant)
	if err != nil || read.SealedKey != "" || read.SealedTicket != "" {
		t.Fatal("public read exposed credentials")
	}
}

func TestARCASetupService_RefusesSellersAndCrossAccountCredentials(t *testing.T) {
	t.Parallel()
	s, store, connector, _, tenant := setupTestService(t)
	seller := tenant
	seller.Role = domain.UserRoleSeller
	if _, err := s.Create(context.Background(), seller, "20329642330"); !errors.Is(err, domain.ErrForbidden) {
		t.Fatal("seller generated credentials")
	}
	if _, err := s.Create(context.Background(), tenant, "00000000000"); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatal("accepted fabricated identity")
	}
	if _, err := s.Create(context.Background(), tenant, "20329642330"); err != nil {
		t.Fatal(err)
	}
	other := tenant
	other.AccountID = uuid.New()
	if _, err := s.Verify(context.Background(), other, 1); !errors.Is(err, domain.ErrNotConfigured) {
		t.Fatalf("accepted another account's key: %v", err)
	}
	if connector.calls != 0 || store.branchWrites != 0 {
		t.Fatal("foreign credentials reached ARCA")
	}
}

func TestARCASetupService_PreservesTicketWhenBranchVerificationFails(t *testing.T) {
	t.Parallel()
	s, store, connector, _, tenant := setupTestService(t)
	if _, err := s.Create(context.Background(), tenant, "20329642330"); err != nil {
		t.Fatal(err)
	}
	connector.result = domain.ARCAConnectionResult{Ticket: domain.ARCATicket{Token: "token", Sign: "sign", ExpiresAt: time.Now().Add(time.Hour)}, Failure: "POINT_OF_SALE_REJECTED"}
	result, err := s.Verify(context.Background(), tenant, 2)
	if err != nil || result.Ticket.Token != "" || store.branchWrites != 0 || store.ticketWrites != 1 {
		t.Fatal("failed check leaked or discarded credentials")
	}
	connector.result.Failure = ""
	store.branchErr = domain.ErrConflict
	if _, err = s.Verify(context.Background(), tenant, 2); !errors.Is(err, domain.ErrConflict) {
		t.Fatal("branch conflict was ignored")
	}
	if connector.seenTicket.Token != "token" || store.ticketWrites != 2 || store.branch != tenant.BranchID {
		t.Fatal("ticket or branch scope was lost")
	}
}
