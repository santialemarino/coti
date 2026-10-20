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
		if client.TaxID == nil || !domain.ValidCUIT(*client.TaxID) {
			return fmt.Errorf("%w: the client needs a CUIT/CUIL", domain.ErrInvalidInput)
		}
		return nil
	}
	if !domain.ValidCUIT(in.TaxID) || (in.ClientID == nil) == (in.New == nil) {
		return domain.ErrInvalidInput
	}
	matches, err := s.clients.FindByTaxID(ctx, q, tenant.AccountID, in.TaxID)
	if err != nil {
		return err
	}
	if len(matches) > 0 && (in.ClientID == nil || matches[0].ID != *in.ClientID) {
		return fmt.Errorf("%w: select the existing client for this CUIT/CUIL", domain.ErrConflict)
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
		if client.TaxID != nil && *client.TaxID != in.TaxID {
			return fmt.Errorf("%w: the selected client's CUIT/CUIL differs", domain.ErrInvalidInput)
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
		if err := s.clients.SetMissingTaxID(ctx, q, tenant.AccountID, clientID, in.TaxID); err != nil {
			return err
		}
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
	if !domain.ValidCUIT(in.TaxID) || (in.ClientID == nil) == (in.New == nil) {
		return fmt.Errorf("%w: select one client and a valid CUIT/CUIL", domain.ErrInvalidInput)
	}
	if in.New != nil {
		in.New.TaxID = &in.TaxID
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
