package services

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

type arcaSetupStore interface {
	Get(context.Context, repository.Querier, uuid.UUID) (*domain.ARCASetup, error)
	Exists(context.Context, repository.Querier, uuid.UUID) (bool, error)
	HasPendingInvoice(context.Context, repository.Querier, uuid.UUID) (bool, error)
	Create(context.Context, repository.Querier, uuid.UUID, domain.ARCASetup) error
	SaveCertificate(context.Context, repository.Querier, uuid.UUID, uuid.UUID, string, time.Time) error
	Delete(context.Context, repository.Querier, uuid.UUID) error
}

type arcaAccountLocker interface {
	GetByIDForUpdate(ctx context.Context, q repository.Querier, accountID uuid.UUID) (*domain.Account, error)
}

type arcaSetupCipher interface {
	Enabled() bool
	Seal(string) (string, error)
	Open(string) (string, error)
}

// ARCASetupService owns the credential an account connects to ARCA with.
type ARCASetupService struct {
	db        tenantTxRunner
	store     arcaSetupStore
	accounts  arcaAccountLocker
	cipher    arcaSetupCipher
	connector domain.ARCAConnector
	locks     sync.Map
}

// NewARCASetupService builds the setup service.
func NewARCASetupService(db tenantTxRunner, store arcaSetupStore, accounts arcaAccountLocker, cipher arcaSetupCipher, connector domain.ARCAConnector) *ARCASetupService {
	return &ARCASetupService{db: db, store: store, accounts: accounts, cipher: cipher, connector: connector}
}

// Get returns the account's setup without its private key; nil when it has none. The flag says
// whether this server can store credentials at all.
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
	return setup, s.cipher.Enabled(), nil
}

// Create generates the account's key for the account's own CUIT and returns its public certificate
// request. The account row stays locked until the setup exists, so the CUIT cannot change in between.
func (s *ARCASetupService) Create(ctx context.Context, tenant domain.Tenant) (*domain.ARCASetup, error) {
	if !tenant.IsAdmin() {
		return nil, domain.ErrForbidden
	}
	if !s.cipher.Enabled() {
		return nil, domain.ErrNotConfigured
	}
	unlock := s.lock(tenant.AccountID)
	defer unlock()
	var setup domain.ARCASetup
	err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		account, err := s.accounts.GetByIDForUpdate(ctx, q, tenant.AccountID)
		if err != nil {
			return err
		}
		if account.TaxID == nil || strings.TrimSpace(*account.TaxID) == "" {
			return domain.WithCode(domain.CodeAccountTaxIDRequired,
				fmt.Errorf("%w: the account has no CUIT", domain.ErrInvalidInput))
		}
		taxID := domain.NormalizeTaxID(*account.TaxID)
		if !validARCATaxID(taxID) {
			return domain.WithCode(domain.CodeInvalidTaxID,
				fmt.Errorf("%w: the account CUIT is not valid", domain.ErrInvalidInput))
		}
		key, csr, err := s.connector.Generate(taxID)
		if err != nil {
			return err
		}
		sealed, err := s.cipher.Seal(tenant.AccountID.String() + ":" + key)
		if err != nil {
			return err
		}
		setup = domain.ARCASetup{ID: uuid.New(), TaxID: taxID, CSR: csr, SealedKey: sealed}
		return s.store.Create(ctx, q, tenant.AccountID, setup)
	})
	if err != nil {
		return nil, err
	}
	setup.SealedKey = ""
	return &setup, nil
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

// Delete removes the account's credential, whether the connection is finished or in progress, unless
// an invoice issued with it is still waiting for ARCA. The account row is locked first, as a CUIT
// change does.
func (s *ARCASetupService) Delete(ctx context.Context, tenant domain.Tenant) error {
	if !tenant.IsAdmin() {
		return domain.ErrForbidden
	}
	unlock := s.lock(tenant.AccountID)
	defer unlock()
	return s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		if _, err := s.accounts.GetByIDForUpdate(ctx, q, tenant.AccountID); err != nil {
			return err
		}
		return disconnectARCA(ctx, q, s.store, tenant.AccountID)
	})
}

func (s *ARCASetupService) load(ctx context.Context, tenant domain.Tenant) (*domain.ARCASetup, error) {
	var setup *domain.ARCASetup
	err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		var err error
		setup, err = s.store.Get(ctx, q, tenant.AccountID)
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
