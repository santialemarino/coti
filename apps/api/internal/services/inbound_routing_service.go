package services

import (
	"context"
	"fmt"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

type inboundRoutingDB interface {
	CrossAccount() repository.Querier
}

type inboundRoutingChannelReader interface {
	GetActiveByTypeAndIdentifiersCrossAccount(ctx context.Context, q repository.Querier,
		channelType domain.ChannelType, identifiers []string) (map[string]domain.Channel, error)
}

// InboundRoutingService resolves external inbox identifiers before the account is known.
type InboundRoutingService struct {
	db       inboundRoutingDB
	channels inboundRoutingChannelReader
}

// NewInboundRoutingService builds an InboundRoutingService.
func NewInboundRoutingService(
	db inboundRoutingDB, channels inboundRoutingChannelReader,
) *InboundRoutingService {
	return &InboundRoutingService{db: db, channels: channels}
}

// ResolveChannels maps external inbox identifiers to their account, branch, and concrete channel.
func (s *InboundRoutingService) ResolveChannels(
	ctx context.Context, channelType domain.ChannelType, identifiers []string,
) (map[string]domain.InboundChannelRoute, error) {
	if s.db == nil || s.channels == nil {
		return nil, fmt.Errorf("%w: inbound routing is not fully wired", domain.ErrInvalidInput)
	}

	normalized, err := normalizeInboundChannelIdentifiers(channelType, identifiers)
	if err != nil {
		return nil, err
	}
	if len(normalized) == 0 {
		return map[string]domain.InboundChannelRoute{}, nil
	}

	channels, err := s.channels.GetActiveByTypeAndIdentifiersCrossAccount(ctx, s.db.CrossAccount(),
		channelType, normalized)
	if err != nil {
		return nil, err
	}
	routes := make(map[string]domain.InboundChannelRoute, len(channels))
	for identifier, channel := range channels {
		route := domain.InboundChannelRoute{
			AccountID: channel.AccountID, BranchID: channel.BranchID, ChannelID: channel.ID,
			ChannelType: channel.Type,
		}
		if err := route.Validate(); err != nil {
			return nil, err
		}
		routes[identifier] = route
	}
	return routes, nil
}

func normalizeInboundChannelIdentifiers(
	channelType domain.ChannelType, identifiers []string,
) ([]string, error) {
	normalized := make([]string, 0, len(identifiers))
	seen := make(map[string]struct{}, len(identifiers))
	for _, identifier := range identifiers {
		value, err := domain.NormalizeInboundChannelIdentifier(channelType, identifier)
		if err != nil {
			return nil, err
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		normalized = append(normalized, value)
	}
	return normalized, nil
}
