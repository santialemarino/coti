package services

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

type arcaSetupStore interface {
	Get(context.Context, repository.Querier, uuid.UUID, uuid.UUID) (*domain.ARCASetup, error)
	Create(context.Context, repository.Querier, uuid.UUID, domain.ARCASetup) error
	SaveCertificate(context.Context, repository.Querier, uuid.UUID, uuid.UUID, string, time.Time) error
	SaveTicket(context.Context, repository.Querier, uuid.UUID, uuid.UUID, string) error
	SaveBranch(context.Context, repository.Querier, uuid.UUID, uuid.UUID, uuid.UUID, int) error
	Delete(context.Context, repository.Querier, uuid.UUID) error
}
type arcaSetupCipher interface {
	Enabled() bool
	Seal(string) (string, error)
	Open(string) (string, error)
}

// ARCASetupService owns account credentials and branch homologation checks.
type ARCASetupService struct {
	db        tenantTxRunner
	store     arcaSetupStore
	cipher    arcaSetupCipher
	connector domain.ARCAConnector
	locks     sync.Map
}

// NewARCASetupService builds the setup service.
func NewARCASetupService(db tenantTxRunner, store arcaSetupStore, cipher arcaSetupCipher, connector domain.ARCAConnector) *ARCASetupService {
	return &ARCASetupService{db: db, store: store, cipher: cipher, connector: connector}
}

// Get returns public setup metadata without credentials.
func (s *ARCASetupService) Get(ctx context.Context, tenant domain.Tenant) (*domain.ARCASetup, bool, error) {
	if !tenant.IsAdmin() {
		return nil, false, domain.ErrForbidden
	}
	setup, err := s.load(ctx, tenant)
	if errors.Is(err, domain.ErrNotFound) {
		return nil, s.cipher.Enabled(), nil
	}
	if err != nil {
		return nil, s.cipher.Enabled(), err
	}
	setup.SealedKey = ""
	setup.SealedTicket = ""
	return setup, s.cipher.Enabled(), nil
}

// Create generates the account's first key and returns its public certificate request.
func (s *ARCASetupService) Create(ctx context.Context, tenant domain.Tenant, taxID string) (*domain.ARCASetup, error) {
	if !tenant.IsAdmin() {
		return nil, domain.ErrForbidden
	}
	if !s.cipher.Enabled() {
		return nil, domain.ErrNotConfigured
	}
	taxID = strings.ReplaceAll(strings.TrimSpace(taxID), "-", "")
	if !validARCATaxID(taxID) {
		return nil, domain.ErrInvalidInput
	}
	unlock := s.lock(tenant.AccountID)
	defer unlock()
	key, csr, err := s.connector.Generate(taxID)
	if err != nil {
		return nil, err
	}
	sealed, err := s.cipher.Seal(tenant.AccountID.String() + ":" + key)
	if err != nil {
		return nil, err
	}
	setup := domain.ARCASetup{ID: uuid.New(), TaxID: taxID, CSR: csr, SealedKey: sealed}
	err = s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error { return s.store.Create(ctx, q, tenant.AccountID, setup) })
	setup.SealedKey = ""
	return &setup, err
}

// Upload installs the ARCA-issued certificate matching the account's private key.
func (s *ARCASetupService) Upload(ctx context.Context, tenant domain.Tenant, certificate string) error {
	if !tenant.IsAdmin() {
		return domain.ErrForbidden
	}
	if !s.cipher.Enabled() {
		return domain.ErrNotConfigured
	}
	unlock := s.lock(tenant.AccountID)
	defer unlock()
	setup, err := s.load(ctx, tenant)
	if err != nil {
		return err
	}
	key, err := s.open(tenant.AccountID, setup.SealedKey)
	if err != nil {
		return err
	}
	expires, err := s.connector.ValidateCertificate(key, certificate, setup.TaxID)
	if err != nil {
		return err
	}
	return s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		return s.store.SaveCertificate(ctx, q, tenant.AccountID, setup.ID, certificate, expires)
	})
}

// Delete disconnects every branch and removes the account's fiscal credentials.
func (s *ARCASetupService) Delete(ctx context.Context, tenant domain.Tenant) error {
	if !tenant.IsAdmin() {
		return domain.ErrForbidden
	}
	unlock := s.lock(tenant.AccountID)
	defer unlock()
	return s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error { return s.store.Delete(ctx, q, tenant.AccountID) })
}

// Verify checks homologation access and saves a branch only after a successful read.
func (s *ARCASetupService) Verify(ctx context.Context, tenant domain.Tenant, pointOfSale int) (domain.ARCAConnectionResult, error) {
	if !tenant.IsAdmin() {
		return domain.ARCAConnectionResult{}, domain.ErrForbidden
	}
	if err := requireBranch(tenant, "ARCA connection"); err != nil {
		return domain.ARCAConnectionResult{}, err
	}
	if pointOfSale < 1 || pointOfSale > 99998 {
		return domain.ARCAConnectionResult{}, domain.ErrInvalidInput
	}
	if !s.cipher.Enabled() {
		return domain.ARCAConnectionResult{}, domain.ErrNotConfigured
	}
	unlock := s.lock(tenant.AccountID)
	defer unlock()
	setup, err := s.load(ctx, tenant)
	if err != nil {
		return domain.ARCAConnectionResult{}, err
	}
	key, err := s.open(tenant.AccountID, setup.SealedKey)
	if err != nil {
		return domain.ARCAConnectionResult{}, err
	}
	var ticket domain.ARCATicket
	if setup.SealedTicket != "" {
		raw, openErr := s.open(tenant.AccountID, setup.SealedTicket)
		if openErr != nil {
			return domain.ARCAConnectionResult{}, openErr
		}
		if err = json.Unmarshal([]byte(raw), &ticket); err != nil {
			return domain.ARCAConnectionResult{}, err
		}
	}
	result, err := s.connector.Verify(ctx, key, setup.Certificate, setup.TaxID, pointOfSale, ticket)
	if err != nil {
		return domain.ARCAConnectionResult{}, err
	}
	sealed := ""
	if result.Ticket.Token != "" {
		raw, marshalErr := json.Marshal(result.Ticket)
		if marshalErr != nil {
			return domain.ARCAConnectionResult{}, marshalErr
		}
		sealed, err = s.cipher.Seal(tenant.AccountID.String() + ":" + string(raw))
		if err != nil {
			return domain.ARCAConnectionResult{}, err
		}
	}
	// A branch conflict must not roll back a live WSAA ticket that cannot be reissued yet.
	if sealed != "" {
		err = s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
			return s.store.SaveTicket(ctx, q, tenant.AccountID, setup.ID, sealed)
		})
		if err != nil {
			return domain.ARCAConnectionResult{}, err
		}
	}
	if result.Failure == "" {
		err = s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
			return s.store.SaveBranch(ctx, q, tenant.AccountID, tenant.BranchID, setup.ID, pointOfSale)
		})
	}
	result.Ticket = domain.ARCATicket{}
	return result, err
}

func (s *ARCASetupService) load(ctx context.Context, tenant domain.Tenant) (*domain.ARCASetup, error) {
	var setup *domain.ARCASetup
	err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		var err error
		setup, err = s.store.Get(ctx, q, tenant.AccountID, tenant.BranchID)
		return err
	})
	return setup, err
}

func (s *ARCASetupService) open(accountID uuid.UUID, sealed string) (string, error) {
	plain, err := s.cipher.Open(sealed)
	if err != nil {
		return "", domain.ErrNotConfigured
	}
	value, ok := strings.CutPrefix(plain, accountID.String()+":")
	if !ok {
		return "", domain.ErrNotConfigured
	}
	return value, nil
}

func (s *ARCASetupService) lock(accountID uuid.UUID) func() {
	value, _ := s.locks.LoadOrStore(accountID, &sync.Mutex{})
	lock := value.(*sync.Mutex)
	lock.Lock()
	return lock.Unlock
}

func validARCATaxID(value string) bool {
	if len(value) != 11 {
		return false
	}
	switch value[:2] {
	case "20", "23", "24", "27", "30", "33", "34":
	default:
		return false
	}
	weights := [...]int{5, 4, 3, 2, 7, 6, 5, 4, 3, 2}
	sum := 0
	for i := range 11 {
		if value[i] < '0' || value[i] > '9' {
			return false
		}
		if i < 10 {
			sum += int(value[i]-'0') * weights[i]
		}
	}
	check := 11 - sum%11
	if check == 11 {
		check = 0
	}
	if check == 10 {
		check = 9
	}
	return check == int(value[10]-'0')
}
