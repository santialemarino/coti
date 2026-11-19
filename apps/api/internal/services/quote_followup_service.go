package services

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

type followupQuoteReader interface {
	GetByID(ctx context.Context, q repository.Querier, accountID, branchID, id uuid.UUID) (*domain.Quote, error)
}

type followupAccountReader interface {
	GetByID(ctx context.Context, q repository.Querier, accountID uuid.UUID) (*domain.Account, error)
}

type followupClientReader interface {
	GetByID(ctx context.Context, q repository.Querier, accountID, clientID uuid.UUID) (*domain.Client, error)
}

// QuoteFollowupService renders seller-configured reminders without contacting a customer.
type QuoteFollowupService struct {
	db       tenantTxRunner
	quotes   followupQuoteReader
	accounts followupAccountReader
	clients  followupClientReader
}

// NewQuoteFollowupService builds the template renderer.
func NewQuoteFollowupService(
	db tenantTxRunner, quotes followupQuoteReader, accounts followupAccountReader,
	clients followupClientReader,
) *QuoteFollowupService {
	return &QuoteFollowupService{db: db, quotes: quotes, accounts: accounts, clients: clients}
}

// Draft renders the reminder without storing or sending it.
func (s *QuoteFollowupService) Draft(ctx context.Context, tenant domain.Tenant, quoteID uuid.UUID) (string, error) {
	result, err := s.Template(ctx, tenant, quoteID)
	if err != nil {
		return "", err
	}
	return result.Message, nil
}

// Template renders the account's reminder after checking the quote's tenant and state.
func (s *QuoteFollowupService) Template(ctx context.Context, tenant domain.Tenant, quoteID uuid.UUID) (*domain.QuoteFollowupTemplate, error) {
	var result domain.QuoteFollowupTemplate
	err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		quote, err := s.quotes.GetByID(ctx, q, tenant.AccountID, tenant.BranchID, quoteID)
		if err != nil {
			return err
		}
		if err := requireUnarchived(*quote); err != nil {
			return err
		}
		if quote.CurrentStatus != domain.QuoteStatusSent {
			return domain.WithCode(domain.CodeQuoteNotSent, domain.ErrConflict)
		}
		account, err := s.accounts.GetByID(ctx, q, tenant.AccountID)
		if err != nil {
			return err
		}
		template := domain.DefaultFollowupTemplate
		if account.FollowupTemplate != nil {
			template = *account.FollowupTemplate
			result.TemplateConfigured = true
		}
		name := ""
		if quote.ClientID != nil {
			client, err := s.clients.GetByID(ctx, q, tenant.AccountID, *quote.ClientID)
			if err != nil && !errors.Is(err, domain.ErrNotFound) {
				return err
			}
			if client != nil && client.Name != nil {
				name = *client.Name
			}
		}
		result.Message = domain.RenderFollowupTemplate(template, name, quote.Number)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &result, nil
}
