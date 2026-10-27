package services

import (
	"context"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

type contactProfiles interface {
	FindByTaxID(context.Context, repository.Querier, uuid.UUID, string) ([]domain.Client, error)
	GetByID(context.Context, repository.Querier, uuid.UUID, uuid.UUID) (*domain.Client, error)
	Create(context.Context, repository.Querier, uuid.UUID, domain.NewClient) (*domain.Client, error)
}

type contactStore interface {
	ListForCustomer(context.Context, repository.Querier, domain.CustomerPrincipal, uuid.UUID) ([]domain.ClientContact, error)
	AttachSource(context.Context, repository.Querier, uuid.UUID, uuid.UUID, uuid.UUID) error
	PriorSeparation(context.Context, repository.Querier, uuid.UUID, string, domain.ChannelType, string) (bool, error)
	HasAssociations(context.Context, repository.Querier, uuid.UUID, uuid.UUID) (bool, error)
	HasPendingForTaxID(context.Context, repository.Querier, uuid.UUID, string) (bool, error)
	Merge(context.Context, repository.Querier, domain.Tenant, uuid.UUID, uuid.UUID) error
	List(context.Context, repository.Querier, uuid.UUID, []uuid.UUID) (map[uuid.UUID][]domain.ClientContact, error)
	HasObservation(context.Context, repository.Querier, uuid.UUID, uuid.UUID) (bool, error)
	FindProfiles(context.Context, repository.Querier, uuid.UUID, string, domain.ChannelType, string) ([]domain.Client, error)
	ListObservations(context.Context, repository.Querier, domain.Tenant) ([]domain.ContactObservation, error)
	GetObservation(context.Context, repository.Querier, domain.Tenant, uuid.UUID) (*domain.ContactObservation, error)
	CreateObservation(context.Context, repository.Querier, uuid.UUID, uuid.UUID, domain.ContactObservation) error
	Save(context.Context, repository.Querier, uuid.UUID, uuid.UUID, []domain.ClientContact, string) error
	UpdateName(context.Context, repository.Querier, uuid.UUID, uuid.UUID, string) error
	Decide(context.Context, repository.Querier, domain.Tenant, uuid.UUID, string) error
	Lock(context.Context, repository.Querier, uuid.UUID, ...string) error
	GetFiscal(context.Context, repository.Querier, uuid.UUID, uuid.UUID) (*domain.ClientFiscal, error)
	SaveFiscal(context.Context, repository.Querier, uuid.UUID, uuid.UUID, domain.ClientContactUpdate) error
}

// ClientContactService manages customer-owned profiles and verified channel associations.
type ClientContactService struct {
	db          tenantTxRunner
	profiles    contactProfiles
	contacts    contactStore
	messenger   domain.ContactMessenger
	localOrigin string
}

// NewClientContactService builds the commercial contact use cases.
func NewClientContactService(db tenantTxRunner, profiles contactProfiles, contacts contactStore) *ClientContactService {
	return &ClientContactService{db: db, profiles: profiles, contacts: contacts}
}

// ListSuggestions returns durable pending cases within the seller's branch reach.
func (s *ClientContactService) ListSuggestions(ctx context.Context, tenant domain.Tenant) ([]domain.ContactObservation, error) {
	var result []domain.ContactObservation
	err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		var err error
		result, err = s.contacts.ListObservations(ctx, q, tenant)
		return err
	})
	return result, err
}

// GetProfile reads only the authenticated supplier's unique CUIT/CUIL profile.
func (s *ClientContactService) GetProfile(ctx context.Context, p domain.CustomerPrincipal) (*domain.ClientContactProfile, error) {
	if p.CustomerID == uuid.Nil || !domain.ValidCUIT(p.TaxID) {
		return nil, domain.ErrUnauthenticated
	}
	result := &domain.ClientContactProfile{TaxID: p.TaxID, Contacts: []domain.ClientContact{}}
	err := s.db.InTenantTx(ctx, domain.Tenant{AccountID: p.AccountID}, func(q repository.Querier) error {
		pending, err := s.contacts.HasPendingForTaxID(ctx, q, p.AccountID, p.TaxID)
		if err != nil {
			return err
		}
		result.ReviewPending = pending
		profiles, err := s.profiles.FindByTaxID(ctx, q, p.AccountID, p.TaxID)
		if err != nil {
			return err
		}
		if len(profiles) == 0 {
			return nil
		}
		if len(profiles) != 1 {
			return domain.ErrConflict
		}
		c := profiles[0]
		fiscal, err := s.contacts.GetFiscal(ctx, q, p.AccountID, c.ID)
		if err != nil {
			return err
		}
		result.LegalName, result.IVACondition, result.Address = fiscal.LegalName, fiscal.IVACondition, fiscal.Address
		result.ClientID = &c.ID
		if c.Name != nil {
			result.Name = *c.Name
		}
		contacts, err := s.contacts.ListForCustomer(ctx, q, p, c.ID)
		if err != nil {
			return err
		}
		if contacts != nil {
			result.Contacts = contacts
		}
		return nil
	})
	return result, err
}

// UpdateProfile saves authenticated declarations without changing recovery credentials.
func (s *ClientContactService) UpdateProfile(ctx context.Context, p domain.CustomerPrincipal, in domain.ClientContactUpdate) error {
	if p.CustomerID == uuid.Nil || !domain.ValidCUIT(p.TaxID) {
		return domain.ErrUnauthenticated
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.IVACondition != nil && !in.IVACondition.Valid() {
		return domain.ErrInvalidInput
	}
	in.LegalName, in.Address = trimmedOrNil(in.LegalName), trimmedOrNil(in.Address)
	if in.Name == "" || utf8.RuneCountInString(in.Name) > maxClientNameLength || strings.ContainsFunc(in.Name, unicode.IsControl) {
		return domain.ErrInvalidInput
	}
	contacts, err := domain.NormalizeClientContacts(in.Contacts)
	if err != nil {
		return err
	}
	if len(contacts) == 0 {
		return domain.ErrInvalidInput
	}
	return s.db.InTenantTx(ctx, domain.Tenant{AccountID: p.AccountID}, func(q repository.Querier) error {
		if err := s.lockContactValues(ctx, q, p.AccountID, contacts); err != nil {
			return err
		}
		if err := s.lockContactProfiles(ctx, q, p.AccountID, p.TaxID, nil); err != nil {
			return err
		}
		profiles, err := s.profiles.FindByTaxID(ctx, q, p.AccountID, p.TaxID)
		if err != nil {
			return err
		}
		var client *domain.Client
		if len(profiles) == 0 {
			client, err = s.profiles.Create(ctx, q, p.AccountID, domain.NewClient{TaxID: &p.TaxID})
			if err != nil {
				return err
			}
		} else if len(profiles) == 1 {
			client = &profiles[0]
		} else {
			return domain.ErrConflict
		}
		if err := s.contacts.UpdateName(ctx, q, p.AccountID, client.ID, in.Name); err != nil {
			return err
		}
		if err := s.contacts.SaveFiscal(ctx, q, p.AccountID, client.ID, in); err != nil {
			return err
		}
		return s.contacts.Save(ctx, q, p.AccountID, client.ID, contacts, "CUSTOMER")
	})
}

// ResolveSuggestion refuses staff association decisions that would bypass customer contact proofs.
func (s *ClientContactService) ResolveSuggestion(context.Context, domain.Tenant, uuid.UUID, string) error {
	return domain.ErrForbidden
}

// RecordInbound identifies senders by verified contact without accepting a declared CUIT as identity.
func (s *ClientContactService) RecordInbound(ctx context.Context, q repository.Querier, route domain.InboundChannelRoute, messageID uuid.UUID, kind domain.ChannelType, value, original string, declared domain.ClientContactHints) error {
	value, err := domain.NormalizeClientContact(kind, value)
	if err != nil {
		return nil
	}
	if err := s.lockContactValues(ctx, q, route.AccountID, []domain.ClientContact{{Kind: kind, Value: value}}); err != nil {
		return err
	}
	seen, err := s.contacts.HasObservation(ctx, q, route.AccountID, messageID)
	if err != nil || seen {
		return err
	}
	profiles, err := s.contacts.FindProfiles(ctx, q, route.AccountID, "", kind, value)
	if err != nil {
		return err
	}
	if err := s.lockContactProfiles(ctx, q, route.AccountID, "", profiles); err != nil {
		return err
	}
	profiles, err = s.contacts.FindProfiles(ctx, q, route.AccountID, "", kind, value)
	if err != nil {
		return err
	}
	observation := domain.ContactObservation{BranchID: route.BranchID, Kind: kind, Value: value, Original: original, TaxID: declared.TaxID, Status: domain.ContactReviewRecorded}
	if declared.Name != nil {
		name := strings.TrimSpace(*declared.Name)
		if name != "" && utf8.RuneCountInString(name) <= maxClientNameLength && !strings.ContainsFunc(name, unicode.IsControl) {
			observation.Name = &name
		}
	}
	if len(profiles) > 1 {
		return domain.ErrConflict
	}
	if len(profiles) == 1 {
		observation.SourceClientID = &profiles[0].ID
	} else {
		name := value
		if declared.DisplayName != nil {
			label := strings.TrimSpace(*declared.DisplayName)
			if label != "" && utf8.RuneCountInString(label) <= maxClientNameLength && !strings.ContainsFunc(label, unicode.IsControl) {
				name = label
			}
		}
		if observation.Name != nil {
			name = *observation.Name
		}
		origin := domain.ClientOrigin(kind)
		profile, err := s.profiles.Create(ctx, q, route.AccountID, domain.NewClient{Name: &name, OriginChannel: &origin})
		if err != nil {
			return err
		}
		if err := s.contacts.Save(ctx, q, route.AccountID, profile.ID, []domain.ClientContact{{Kind: kind, Value: value, Primary: true}}, "CHANNEL"); err != nil {
			return err
		}
		observation.SourceClientID = &profile.ID
	}
	if err := s.contacts.CreateObservation(ctx, q, route.AccountID, messageID, observation); err != nil {
		return err
	}
	if store, ok := s.contacts.(contactOnboardingStore); ok {
		if len(profiles) == 0 && observation.Name == nil {
			if err := store.QueueNameRequest(ctx, q, route, kind, value); err != nil {
				return err
			}
		}
		if observation.Name != nil {
			if err := store.CompleteNameRequest(ctx, q, route.AccountID, *observation.SourceClientID, kind, value, *observation.Name); err != nil {
				return err
			}
		}
		return store.AttachContactObservations(ctx, q, route.AccountID, *observation.SourceClientID, kind, value)
	}
	return nil
}
