package services

import (
	"context"
	"fmt"
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
	SetMissingTaxID(context.Context, repository.Querier, uuid.UUID, uuid.UUID, string) error
}

type contactStore interface {
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
	Lock(context.Context, repository.Querier, uuid.UUID, string) error
}

// ClientContactService manages customer-owned data and seller-reviewed inbound associations.
type ClientContactService struct {
	db       tenantTxRunner
	profiles contactProfiles
	contacts contactStore
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
		result.ClientID = &c.ID
		if c.Name != nil {
			result.Name = *c.Name
		}
		contacts, err := s.contacts.List(ctx, q, p.AccountID, []uuid.UUID{c.ID})
		if err != nil {
			return err
		}
		if contacts[c.ID] != nil {
			result.Contacts = contacts[c.ID]
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
	if utf8.RuneCountInString(in.Name) > maxClientNameLength || strings.ContainsFunc(in.Name, unicode.IsControl) {
		return domain.ErrInvalidInput
	}
	contacts, err := domain.NormalizeClientContacts(in.Contacts)
	if err != nil {
		return err
	}
	return s.db.InTenantTx(ctx, domain.Tenant{AccountID: p.AccountID}, func(q repository.Querier) error {
		if err := s.contacts.Lock(ctx, q, p.AccountID, "commercial-contacts"); err != nil {
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
		return s.contacts.Save(ctx, q, p.AccountID, client.ID, contacts, "CUSTOMER")
	})
}

// ResolveSuggestion applies an explicit seller decision without granting customer access.
func (s *ClientContactService) ResolveSuggestion(ctx context.Context, tenant domain.Tenant, id uuid.UUID, action string) error {
	if action != "ASSOCIATE" && action != "SEPARATE" && action != "MERGE" {
		return domain.ErrInvalidInput
	}
	return s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		if err := s.contacts.Lock(ctx, q, tenant.AccountID, "commercial-contacts"); err != nil {
			return err
		}
		o, err := s.contacts.GetObservation(ctx, q, tenant, id)
		if err != nil {
			return err
		}
		status := domain.ContactReviewSeparate
		if action == "ASSOCIATE" {
			status = "ASSOCIATED"
		}
		if action == "MERGE" {
			status = "MERGED"
		}
		if o.Status == status {
			return nil
		}
		if o.Status != "PENDING" {
			return domain.ErrConflict
		}
		if action == "SEPARATE" && o.SourceClientID == nil {
			origin := domain.ClientOrigin(o.Kind)
			client, err := s.profiles.Create(ctx, q, tenant.AccountID, domain.NewClient{Name: o.Name, OriginChannel: &origin})
			if err != nil {
				return err
			}
			if err := s.contacts.Save(ctx, q, tenant.AccountID, client.ID, []domain.ClientContact{{Kind: o.Kind, Value: o.Value, Primary: true}}, "SELLER"); err != nil {
				return err
			}
			if err := s.contacts.AttachSource(ctx, q, tenant.AccountID, id, client.ID); err != nil {
				return err
			}
		}
		if action == "ASSOCIATE" || action == "MERGE" {
			if o.CandidateClientID == nil {
				return fmt.Errorf("%w: no unambiguous candidate", domain.ErrConflict)
			}
			current, err := s.contacts.List(ctx, q, tenant.AccountID, []uuid.UUID{*o.CandidateClientID})
			if err != nil {
				return err
			}
			contacts := current[*o.CandidateClientID]
			if action == "MERGE" {
				if o.SourceClientID == nil || *o.SourceClientID == *o.CandidateClientID {
					return domain.ErrConflict
				}
				source, err := s.contacts.List(ctx, q, tenant.AccountID, []uuid.UUID{*o.SourceClientID})
				if err != nil {
					return err
				}
				for _, incoming := range source[*o.SourceClientID] {
					found, hasKind := false, false
					for _, existing := range contacts {
						if existing.Kind == incoming.Kind {
							hasKind = true
							if existing.Value == incoming.Value {
								found = true
							}
						}
					}
					if !found {
						incoming.Primary = !hasKind
						contacts = append(contacts, incoming)
					}
				}
				if err := s.contacts.Merge(ctx, q, tenant, *o.SourceClientID, *o.CandidateClientID); err != nil {
					return err
				}
			}
			found := false
			hasKind := false
			for _, c := range contacts {
				if c.Kind == o.Kind {
					hasKind = true
					if c.Value == o.Value {
						found = true
					}
				}
			}
			if !found {
				contacts = append(contacts, domain.ClientContact{Kind: o.Kind, Value: o.Value, Primary: !hasKind})
			}
			if err := s.contacts.Save(ctx, q, tenant.AccountID, *o.CandidateClientID, contacts, "SELLER"); err != nil {
				return err
			}
		}
		return s.contacts.Decide(ctx, q, tenant, id, string(status))
	})
}

// RecordInbound retains deterministic declarations inside the caller's intake transaction.
func (s *ClientContactService) RecordInbound(ctx context.Context, q repository.Querier, route domain.InboundChannelRoute, messageID uuid.UUID, kind domain.ChannelType, value, original string, declared domain.ClientContactHints) error {
	value, err := domain.NormalizeClientContact(kind, value)
	if err != nil {
		return nil
	}
	if err := s.contacts.Lock(ctx, q, route.AccountID, "commercial-contacts"); err != nil {
		return err
	}
	seen, err := s.contacts.HasObservation(ctx, q, route.AccountID, messageID)
	if err != nil || seen {
		return err
	}
	taxID := ""
	if declared.TaxID != nil {
		taxID = domain.NormalizeTaxID(*declared.TaxID)
		if !domain.ValidCUIT(taxID) {
			taxID = ""
		}
	}
	profiles, err := s.contacts.FindProfiles(ctx, q, route.AccountID, taxID, kind, value)
	if err != nil {
		return err
	}
	ids := make([]uuid.UUID, 0, len(profiles))
	for _, c := range profiles {
		ids = append(ids, c.ID)
	}
	contacts, err := s.contacts.List(ctx, q, route.AccountID, ids)
	if err != nil {
		return err
	}
	var source, candidate *domain.Client
	ambiguous := false
	for i := range profiles {
		c := &profiles[i]
		if taxID != "" && c.TaxID != nil && *c.TaxID == taxID {
			candidate = c
		}
		for _, contact := range contacts[c.ID] {
			normalized, nerr := domain.NormalizeClientContact(contact.Kind, contact.Value)
			if nerr == nil && contact.Kind == kind && normalized == value {
				if source != nil && source.ID != c.ID {
					ambiguous = true
				}
				source = c
			}
		}
	}
	o := domain.ContactObservation{BranchID: route.BranchID, Kind: kind, Value: value, Original: original, Status: "RECORDED"}
	if taxID != "" {
		o.TaxID = &taxID
	}
	if declared.Name != nil {
		n := strings.TrimSpace(*declared.Name)
		if n != "" && utf8.RuneCountInString(n) <= maxClientNameLength && !strings.ContainsFunc(n, unicode.IsControl) {
			o.Name = &n
		}
	}
	conflict := ambiguous || (source != nil && source.TaxID != nil && taxID != "" && *source.TaxID != taxID) || (source != nil && candidate != nil && source.ID != candidate.ID)
	if source != nil && source.TaxID == nil && taxID != "" && !conflict {
		associated, err := s.contacts.HasAssociations(ctx, q, route.AccountID, source.ID)
		if err != nil {
			return err
		}
		conflict = associated
	}
	if !conflict && source != nil && o.Name != nil && source.Name != nil && *source.Name != *o.Name {
		conflict = true
	}
	if candidate != nil {
		o.CandidateClientID = &candidate.ID
	}
	if conflict || (candidate != nil && source == nil) {
		o.Status = "PENDING"
		separated, err := s.contacts.PriorSeparation(ctx, q, route.AccountID, taxID, kind, value)
		if err != nil {
			return err
		}
		if separated {
			o.Status = "RECORDED"
		}
	} else {
		if source == nil {
			in := domain.NewClient{Name: o.Name}
			origin := domain.ClientOrigin(kind)
			in.OriginChannel = &origin
			if taxID != "" {
				in.TaxID = &taxID
			}
			source, err = s.profiles.Create(ctx, q, route.AccountID, in)
			if err != nil {
				return err
			}
			if err := s.contacts.Save(ctx, q, route.AccountID, source.ID, []domain.ClientContact{{Kind: kind, Value: value, Primary: true}}, "CHANNEL"); err != nil {
				return err
			}
		} else {
			if source.TaxID == nil && taxID != "" {
				if err := s.profiles.SetMissingTaxID(ctx, q, route.AccountID, source.ID, taxID); err != nil {
					return err
				}
			}
			if source.Name == nil && o.Name != nil {
				if err := s.contacts.UpdateName(ctx, q, route.AccountID, source.ID, *o.Name); err != nil {
					return err
				}
			}
		}
	}
	if source != nil && !ambiguous {
		o.SourceClientID = &source.ID
	}
	return s.contacts.CreateObservation(ctx, q, route.AccountID, messageID, o)
}
