package services

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

type followupQuoteReader interface {
	GetByID(ctx context.Context, q repository.Querier, accountID, branchID, id uuid.UUID) (*domain.Quote, error)
	GetCurrentVersion(ctx context.Context, q repository.Querier, accountID, branchID,
		quoteID uuid.UUID) (*domain.QuoteVersion, error)
	ListItemsWithProduct(ctx context.Context, q repository.Querier, accountID,
		versionID uuid.UUID) ([]domain.QuoteItem, error)
}

type followupAccountReader interface {
	GetByID(ctx context.Context, q repository.Querier, accountID uuid.UUID) (*domain.Account, error)
}

type followupClientReader interface {
	GetByID(ctx context.Context, q repository.Querier, accountID, clientID uuid.UUID) (*domain.Client, error)
}

type followupUserReader interface {
	GetByID(ctx context.Context, q repository.Querier, accountID, id uuid.UUID) (*domain.AppUser, error)
}

// QuoteFollowupService drafts, on the seller's request, the message that chases a sent quote the
// client has not answered. It writes nothing: the seller sends the message, and only a later write
// to the quote clears its follow-up flag.
type QuoteFollowupService struct {
	db       tenantTxRunner
	quotes   followupQuoteReader
	accounts followupAccountReader
	clients  followupClientReader
	users    followupUserReader
	drafter  domain.FollowupDrafter
	now      func() time.Time
}

// NewQuoteFollowupService builds the service; a nil drafter answers every draft as AI_UNAVAILABLE.
func NewQuoteFollowupService(
	db tenantTxRunner, quotes followupQuoteReader, accounts followupAccountReader,
	clients followupClientReader, users followupUserReader, drafter domain.FollowupDrafter,
) *QuoteFollowupService {
	return &QuoteFollowupService{db: db, quotes: quotes, accounts: accounts, clients: clients,
		users: users, drafter: drafter, now: time.Now}
}

// Draft writes a fresh follow-up message from the quote as it stands now. Only a sent quote has a
// client to chase; one still waiting on the seller is chased by sending it.
func (s *QuoteFollowupService) Draft(
	ctx context.Context, tenant domain.Tenant, quoteID uuid.UUID,
) (string, error) {
	if s.drafter == nil {
		return "", fmt.Errorf("%w: no language model is bound", domain.ErrAIUnavailable)
	}
	var in domain.FollowupDraftInput
	err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		quote, err := s.quotes.GetByID(ctx, q, tenant.AccountID, tenant.BranchID, quoteID)
		if err != nil {
			return err
		}
		if quote.ArchivedAt != nil {
			return domain.WithCode(domain.CodeQuoteArchived, domain.ErrConflict)
		}
		if quote.CurrentStatus != domain.QuoteStatusSent {
			return domain.WithCode(domain.CodeQuoteNotSent,
				fmt.Errorf("%w: a %s quote has no client answer to chase", domain.ErrConflict,
					quote.CurrentStatus))
		}
		account, err := s.accounts.GetByID(ctx, q, tenant.AccountID)
		if err != nil {
			return err
		}
		version, err := s.quotes.GetCurrentVersion(ctx, q, tenant.AccountID, tenant.BranchID, quoteID)
		if err != nil {
			return err
		}
		items, err := s.quotes.ListItemsWithProduct(ctx, q, tenant.AccountID, version.ID)
		if err != nil {
			return err
		}
		in = domain.FollowupDraftInput{SupplierName: account.Name, QuoteNumber: quote.Number,
			QuietSince: quote.UpdatedAt,
			Lapsed:     quote.ExpiresAt != nil && quote.ExpiresAt.Before(s.now()),
			Items:      followupItems(items)}
		// A client merged away or a seller removed only leaves the greeting or signature out.
		if quote.ClientID != nil {
			client, err := s.clients.GetByID(ctx, q, tenant.AccountID, *quote.ClientID)
			switch {
			case err == nil && client.Name != nil:
				in.ClientName = *client.Name
			case err != nil && !errors.Is(err, domain.ErrNotFound):
				return err
			}
		}
		if quote.SellerID != nil {
			seller, err := s.users.GetByID(ctx, q, tenant.AccountID, *quote.SellerID)
			switch {
			case err == nil:
				in.SellerName = seller.Name
			case !errors.Is(err, domain.ErrNotFound):
				return err
			}
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	return s.drafter.Draft(domain.WithAIAccount(ctx, tenant), in)
}

// followupItems words each line as the client would read it: a matched product with its quantity,
// an unmatched one in the client's own words, which already carry theirs.
func followupItems(items []domain.QuoteItem) []string {
	lines := make([]string, 0, len(items))
	for _, item := range items {
		if item.ProductName == nil || *item.ProductName == "" {
			lines = append(lines, item.RequestedDescription)
			continue
		}
		quantity := item.Quantity.String()
		if item.ProductUnit != nil && *item.ProductUnit != "" {
			quantity += " " + *item.ProductUnit
		}
		lines = append(lines, quantity+" "+*item.ProductName)
	}
	return lines
}
