package services

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

// maxListedNonOrders caps the «Mensajes sin pedido» list; older ones stay retained, unlisted.
const maxListedNonOrders = 100

// nonOrderInbox is the seller-facing surface over messages that opened no order.
type nonOrderInbox interface {
	ListOpenNonOrders(ctx context.Context, q repository.Querier, accountID, branchID uuid.UUID,
		limit int) ([]domain.InboundChannelMessage, error)
	ClaimNonOrder(ctx context.Context, q repository.Querier, accountID, branchID, id,
		rfqID uuid.UUID) (*domain.InboundChannelMessage, error)
	Dismiss(ctx context.Context, q repository.Querier, accountID, branchID, id uuid.UUID) error
}

// ListNonOrders lists the branch's open messages that are not orders, newest first.
func (s *RFQService) ListNonOrders(
	ctx context.Context, tenant domain.Tenant,
) ([]domain.InboundChannelMessage, error) {
	inbox, err := s.nonOrderInbox(tenant, "the messages that are not orders")
	if err != nil {
		return nil, err
	}
	var messages []domain.InboundChannelMessage
	err = s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		var err error
		messages, err = inbox.ListOpenNonOrders(ctx, q, tenant.AccountID, tenant.BranchID,
			maxListedNonOrders)
		return err
	})
	return messages, err
}

// DismissNonOrder puts away one open message that is not an order.
func (s *RFQService) DismissNonOrder(ctx context.Context, tenant domain.Tenant, id uuid.UUID) error {
	inbox, err := s.nonOrderInbox(tenant, "dismissing a message")
	if err != nil {
		return err
	}
	return s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		return inbox.Dismiss(ctx, q, tenant.AccountID, tenant.BranchID, id)
	})
}

// CreateDraftFromNonOrder opens the order a seller says a non-order message is, reading its
// text through the same pipeline; one that still names no material becomes a failed order.
func (s *RFQService) CreateDraftFromNonOrder(
	ctx context.Context, tenant domain.Tenant, id uuid.UUID,
) (*domain.TextRFQDraft, error) {
	inbox, err := s.nonOrderInbox(tenant, "an order from a message")
	if err != nil {
		return nil, err
	}
	rfqID := uuid.New()
	var message *domain.InboundChannelMessage
	if err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		var err error
		message, err = inbox.ClaimNonOrder(ctx, q, tenant.AccountID, tenant.BranchID, id, rfqID)
		return err
	}); err != nil {
		return nil, err
	}
	raw, err := s.requiredRFQText(message.Body)
	if err != nil {
		return nil, err
	}
	sellerID := tenant.UserID
	return s.createTextDraft(ctx, tenant, domain.TextRFQDraftInput{
		ChannelID: message.ChannelID, ClientLabel: message.SenderLabel, RawText: raw,
	}, &sellerID, &rfqID)
}

func (s *RFQService) nonOrderInbox(tenant domain.Tenant, what string) (nonOrderInbox, error) {
	if err := requireBranch(tenant, what); err != nil {
		return nil, err
	}
	inbox, ok := s.inboundMessages.(nonOrderInbox)
	if !ok {
		return nil, fmt.Errorf("%w: the inbound message store is not wired", domain.ErrNotConfigured)
	}
	return inbox, nil
}
