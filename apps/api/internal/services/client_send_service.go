package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/google/uuid"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

type quoteClientConfirmer interface {
	ValidateForSend(context.Context, repository.Querier, domain.Tenant, domain.Quote, *domain.AssociateQuoteClientInput) error
	ConfirmForSend(context.Context, repository.Querier, domain.Tenant, domain.Quote, *domain.AssociateQuoteClientInput) error
}

// WithClientAssociations confirms the selected client in the publication transaction.
func (s *QuoteDeliveryService) WithClientAssociations(clients quoteClientConfirmer) *QuoteDeliveryService {
	s.clients = clients
	return s
}

// ValidateForSend checks the seller's selection before any delivery provider is contacted.
func (s *ClientService) ValidateForSend(ctx context.Context, q repository.Querier, tenant domain.Tenant, quote domain.Quote, in *domain.AssociateQuoteClientInput) error {
	if err := normalizeSendClient(in); err != nil {
		return err
	}
	if err := requireUnarchived(quote); err != nil {
		return err
	}
	if quote.CurrentStatus != domain.QuoteStatusQuoted && quote.CurrentStatus != domain.QuoteStatusSent {
		return domain.ErrConflict
	}
	if in == nil {
		if quote.ClientID == nil {
			return fmt.Errorf("%w: select a client before sending", domain.ErrInvalidInput)
		}
		client, err := s.clients.GetByID(ctx, q, tenant.AccountID, *quote.ClientID)
		if err != nil {
			return err
		}
		if client.Name == nil || (client.Phone == nil && client.Email == nil) {
			return fmt.Errorf("%w: the client needs a full name and a contact", domain.ErrInvalidInput)
		}
		return nil
	}
	if (in.TaxID != "" && !domain.ValidCUIT(in.TaxID)) || (in.ClientID == nil) == (in.New == nil) {
		return domain.ErrInvalidInput
	}
	if quote.ClientID != nil && (in.ClientID == nil || *quote.ClientID != *in.ClientID) {
		if quote.CurrentStatus == domain.QuoteStatusSent {
			return domain.ErrConflict
		}
		sends, err := s.sends.ListByQuote(ctx, q, tenant.AccountID, tenant.BranchID, quote.ID)
		if err != nil {
			return err
		}
		if hasSuccessfulSend(sends) {
			return domain.ErrConflict
		}
	}
	if in.ClientID != nil {
		client, err := s.clients.GetByID(ctx, q, tenant.AccountID, *in.ClientID)
		if err != nil {
			return err
		}
		if in.TaxID != "" && (client.TaxID == nil || *client.TaxID != in.TaxID) {
			return fmt.Errorf("%w: the selected client's CUIT/CUIL differs", domain.ErrInvalidInput)
		}
	} else if in.New != nil {
		matches, err := s.clients.FindByContacts(ctx, q, tenant.AccountID, in.New.Phone, in.New.Email)
		if err != nil {
			return err
		}
		if len(matches) > 0 {
			return fmt.Errorf("%w: select the existing contact profile", domain.ErrConflict)
		}
	}
	tags, err := s.tags.GetByIDs(ctx, q, tenant.AccountID, in.TagIDs)
	if err != nil {
		return err
	}
	if len(tags) != len(in.TagIDs) {
		return domain.ErrInvalidInput
	}
	return nil
}

// ConfirmForSend links both records only inside the successful publication transaction.
func (s *ClientService) ConfirmForSend(ctx context.Context, q repository.Querier, tenant domain.Tenant, quote domain.Quote, in *domain.AssociateQuoteClientInput) error {
	if err := normalizeSendClient(in); err != nil {
		return err
	}
	if err := s.ValidateForSend(ctx, q, tenant, quote, in); err != nil {
		return err
	}
	if in == nil {
		return nil
	}
	var clientID uuid.UUID
	if in.ClientID != nil {
		clientID = *in.ClientID
	} else {
		client, err := s.clients.Create(ctx, q, tenant.AccountID, *in.New)
		if err != nil {
			return err
		}
		clientID = client.ID
	}
	if err := s.tags.ReplaceClientTags(ctx, q, tenant.AccountID, clientID, in.TagIDs); err != nil {
		return err
	}
	if err := s.quotes.SetClient(ctx, q, tenant.AccountID, tenant.BranchID, quote.ID, clientID); err != nil {
		return err
	}
	return s.rfqs.SetClient(ctx, q, tenant.AccountID, tenant.BranchID, quote.RFQID, clientID)
}

func normalizeSendClient(in *domain.AssociateQuoteClientInput) error {
	if in == nil {
		return nil
	}
	in.TaxID = domain.NormalizeTaxID(in.TaxID)
	if (in.TaxID != "" && !domain.ValidCUIT(in.TaxID)) || (in.ClientID == nil) == (in.New == nil) {
		return fmt.Errorf("%w: select one client", domain.ErrInvalidInput)
	}
	if in.New != nil {
		if in.TaxID != "" || in.New.TaxID != nil {
			return domain.ErrInvalidInput
		}
		if err := normalizeNewClient(in.New); err != nil {
			return err
		}
		if in.New.Name == nil {
			return fmt.Errorf("%w: the client name is required", domain.ErrInvalidInput)
		}
	}
	in.TagIDs = uniqueUUIDs(in.TagIDs)
	sort.Slice(in.TagIDs, func(i, j int) bool { return in.TagIDs[i].String() < in.TagIDs[j].String() })
	return nil
}

func sendClientFingerprint(in *domain.AssociateQuoteClientInput) string {
	if in == nil {
		return ""
	}
	data, _ := json.Marshal(in)
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}
