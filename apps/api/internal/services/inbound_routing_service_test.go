package services

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

type fakeInboundRoutingDB struct{}

func (fakeInboundRoutingDB) CrossAccount() repository.Querier { return nil }

type fakeInboundRoutingChannels struct {
	channels    map[string]domain.Channel
	err         error
	identifiers []string
	channelType domain.ChannelType
}

func (f *fakeInboundRoutingChannels) GetActiveByTypeAndIdentifiersCrossAccount(
	_ context.Context, _ repository.Querier, channelType domain.ChannelType, identifiers []string,
) (map[string]domain.Channel, error) {
	f.channelType = channelType
	f.identifiers = append([]string(nil), identifiers...)
	if f.err != nil {
		return nil, f.err
	}
	return f.channels, nil
}

func TestInboundRoutingService_ResolveChannels_NormalizesAndReturnsConcreteRoutes(t *testing.T) {
	t.Parallel()
	accountID := uuid.New()
	branchID := uuid.New()
	channelID := uuid.New()
	channels := &fakeInboundRoutingChannels{channels: map[string]domain.Channel{
		"pedidos@corralon.test": {
			ID: channelID, AccountID: accountID, BranchID: branchID, Type: domain.ChannelTypeEmail,
		},
	}}
	service := NewInboundRoutingService(fakeInboundRoutingDB{}, channels)

	routes, err := service.ResolveChannels(context.Background(), domain.ChannelTypeEmail,
		[]string{" Pedidos@Corralon.test ", "pedidos@corralon.test"})
	if err != nil {
		t.Fatalf("ResolveChannels() = %v, want no error", err)
	}
	if channels.channelType != domain.ChannelTypeEmail {
		t.Errorf("channel type = %s, want EMAIL", channels.channelType)
	}
	if len(channels.identifiers) != 1 || channels.identifiers[0] != "pedidos@corralon.test" {
		t.Errorf("identifiers = %v, want one normalized mailbox", channels.identifiers)
	}
	route, ok := routes["pedidos@corralon.test"]
	if !ok {
		t.Fatalf("routes = %v, want the normalized mailbox", routes)
	}
	if route.AccountID != accountID || route.BranchID != branchID || route.ChannelID != channelID {
		t.Errorf("route = %+v, want account %s branch %s channel %s", route, accountID, branchID,
			channelID)
	}
	if tenant := route.Tenant(); tenant.AccountID != accountID || tenant.BranchID != branchID ||
		tenant.UserID != uuid.Nil {
		t.Errorf("route tenant = %+v, want only the external route scope", tenant)
	}
}

func TestInboundRoutingService_ResolveChannels_RefusesInvalidInputBeforeTheLookup(t *testing.T) {
	t.Parallel()
	channels := &fakeInboundRoutingChannels{}
	service := NewInboundRoutingService(fakeInboundRoutingDB{}, channels)

	for _, test := range []struct {
		name        string
		channelType domain.ChannelType
		identifiers []string
	}{
		{name: "public link", channelType: domain.ChannelTypeWebApp, identifiers: []string{"token"}},
		{name: "empty inbox", channelType: domain.ChannelTypeWhatsApp, identifiers: []string{" "}},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := service.ResolveChannels(context.Background(), test.channelType, test.identifiers)
			if !errors.Is(err, domain.ErrInvalidInput) {
				t.Fatalf("ResolveChannels() = %v, want %v", err, domain.ErrInvalidInput)
			}
		})
	}
	if len(channels.identifiers) != 0 {
		t.Errorf("repository received %v after rejected input", channels.identifiers)
	}
}
