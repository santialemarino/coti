package services

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

var declaredTaxPattern = regexp.MustCompile(`(?i)\b(?:CUIT|CUIL)\s*:?\s*([0-9]{2}[ .-]?[0-9]{8}[ .-]?[0-9])\b`)
var declaredNamePattern = regexp.MustCompile(`(?i)^nombre y apellido\s*:\s*([\p{L}][\p{L}\p{M}\x{27}\- ]*)[.!]?$`)

func channelClientData(body string, label *string) domain.ClientContactHints {
	hints := declaredClientData(body)
	hints.DisplayName = label
	return hints
}

type inboundContactRecorder interface {
	RecordInbound(context.Context, repository.Querier, domain.InboundChannelRoute, uuid.UUID, domain.ChannelType, string, string, domain.ClientContactHints) error
}

func declaredClientData(body string) domain.ClientContactHints {
	var hints domain.ClientContactHints
	matches := declaredTaxPattern.FindAllStringSubmatch(body, -1)
	if len(matches) == 1 {
		taxID := domain.NormalizeTaxID(matches[0][1])
		if domain.ValidCUIT(taxID) {
			hints.TaxID = &taxID
		}
	}
	first, _, _ := strings.Cut(strings.TrimSpace(body), "\n")
	if match := declaredNamePattern.FindStringSubmatch(strings.TrimSpace(first)); len(match) == 2 {
		name := strings.TrimSpace(match[1])
		hints.Name = &name
	}
	return hints
}

// WithContacts adds deterministic contact detection to WhatsApp's durable ingress reservation.
func (s *WhatsAppInboundService) WithContacts(contacts inboundContactRecorder) *WhatsAppInboundService {
	s.contacts = contacts
	return s
}

// WithContacts adds deterministic contact detection to retained email intake.
func (s *EmailInboundService) WithContacts(contacts inboundContactRecorder) *EmailInboundService {
	s.contacts = contacts
	return s
}

type customerContactRecorder interface {
	RecordCustomer(context.Context, domain.CustomerPrincipal, domain.InboundChannelRoute, uuid.UUID, domain.PublicRFQContact) error
}

// WithContacts records authenticated request declarations as commercial customer data.
func (s *PublicRFQService) WithContacts(contacts customerContactRecorder) *PublicRFQService {
	s.contacts = contacts
	return s
}

// RecordCustomer enriches the customer's own profile once after durable request reception.
func (s *ClientContactService) RecordCustomer(ctx context.Context, p domain.CustomerPrincipal, route domain.InboundChannelRoute, messageID uuid.UUID, in domain.PublicRFQContact) error {
	if p.AccountID != route.AccountID || p.TaxID != in.TaxID || p.CustomerID == uuid.Nil {
		return domain.ErrForbidden
	}
	return s.db.InTenantTx(ctx, route.Tenant(), func(q repository.Querier) error {
		incoming := []domain.ClientContact{{Kind: domain.ChannelTypeWhatsApp, Value: in.Phone}, {Kind: domain.ChannelTypeEmail, Value: in.Email}}
		if err := s.lockContactValues(ctx, q, p.AccountID, incoming); err != nil {
			return err
		}
		if err := s.lockContactProfiles(ctx, q, p.AccountID, p.TaxID, nil); err != nil {
			return err
		}
		seen, err := s.contacts.HasObservation(ctx, q, p.AccountID, messageID)
		if err != nil || seen {
			return err
		}
		profiles, err := s.profiles.FindByTaxID(ctx, q, p.AccountID, p.TaxID)
		if err != nil {
			return err
		}
		var client *domain.Client
		if len(profiles) == 0 {
			origin := domain.ClientOriginWebApp
			client, err = s.profiles.Create(ctx, q, p.AccountID, domain.NewClient{TaxID: &p.TaxID, Name: &in.Name, OriginChannel: &origin})
			if err != nil {
				return err
			}
		} else if len(profiles) == 1 {
			client = &profiles[0]
		} else {
			return domain.ErrConflict
		}
		access, err := s.contacts.GetProfileAccess(ctx, q, p, client.ID)
		if err != nil {
			return err
		}
		current, err := s.contacts.List(ctx, q, p.AccountID, []uuid.UUID{client.ID})
		if err != nil {
			return err
		}
		contacts := current[client.ID]
		declared := make([]domain.ClientContact, 0, len(incoming))
		for _, contact := range incoming {
			if contact.Value == "" {
				continue
			}
			value, err := domain.NormalizeClientContact(contact.Kind, contact.Value)
			if err != nil {
				continue
			}
			contact.Value = value
			declared = append(declared, contact)
			found, hasKind := false, false
			for _, c := range contacts {
				if c.Kind == contact.Kind {
					hasKind = true
					normalized, _ := domain.NormalizeClientContact(c.Kind, c.Value)
					if normalized == value {
						found = true
					}
				}
			}
			if !found {
				// An unproved customer never sets the earlier profile's primary, which feeds client.phone/email.
				contact.Primary = !hasKind && access.Proved
				contacts = append(contacts, contact)
			}
		}
		if err := s.contacts.Save(ctx, q, p.AccountID, client.ID, contacts, "CUSTOMER"); err != nil {
			return err
		}
		if access.Proved {
			if err := s.contacts.ProjectPrimaries(ctx, q, p.AccountID, client.ID); err != nil {
				return err
			}
		}
		if err := s.contacts.Declare(ctx, q, p, client.ID, declared); err != nil {
			return err
		}
		original, err := json.Marshal(in)
		if err != nil {
			return err
		}
		return s.contacts.CreateObservation(ctx, q, p.AccountID, messageID, domain.ContactObservation{BranchID: route.BranchID, SourceClientID: &client.ID, TaxID: &p.TaxID, Kind: domain.ChannelTypeEmail, Value: in.Email, Name: &in.Name, Original: string(original), Status: "RECORDED"})
	})
}
