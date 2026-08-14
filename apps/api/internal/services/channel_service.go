package services

import (
	"context"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

// channelReader is the channel listing surface the service needs.
type channelReader interface {
	ListActiveByBranch(ctx context.Context, q repository.Querier, accountID, branchID uuid.UUID) ([]domain.Channel, error)
}

// ChannelService owns branch-scoped intake channel discovery.
type ChannelService struct {
	db       tenantTxRunner
	channels channelReader
}

// NewChannelService builds a ChannelService.
func NewChannelService(db tenantTxRunner, channels channelReader) *ChannelService {
	return &ChannelService{db: db, channels: channels}
}

// ListChannels returns the active intake channels of the selected branch.
func (s *ChannelService) ListChannels(
	ctx context.Context, tenant domain.Tenant,
) ([]domain.Channel, error) {
	if err := requireBranch(tenant, "channel list"); err != nil {
		return nil, err
	}

	var channels []domain.Channel
	err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		var listErr error
		channels, listErr = s.channels.ListActiveByBranch(ctx, q, tenant.AccountID, tenant.BranchID)
		return listErr
	})
	if err != nil {
		return nil, err
	}
	return channels, nil
}
