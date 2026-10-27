package services

import (
	"context"

	"github.com/google/uuid"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

type contactOnboardingStore interface {
	CompleteNameRequest(context.Context, repository.Querier, uuid.UUID, uuid.UUID, domain.ChannelType, string, string) error
	QueueNameRequest(context.Context, repository.Querier, domain.InboundChannelRoute, domain.ChannelType, string) error
	AttachContactObservations(context.Context, repository.Querier, uuid.UUID, uuid.UUID, domain.ChannelType, string) error
	AssociateObservedRequests(context.Context, repository.Querier, uuid.UUID, domain.ChannelType, string) error
	ClaimNameRequest(context.Context, repository.Querier, domain.InboundChannelRoute, domain.ChannelType, string) (*uuid.UUID, error)
	FinishNameRequest(context.Context, repository.Querier, domain.InboundChannelRoute, uuid.UUID, string, bool) error
}

type inboundContactCompleter interface {
	CompleteInbound(context.Context, domain.InboundChannelRoute, domain.ChannelType, string) error
}

// WithMessaging enables fixed name prompts after messages and requests have been retained.
func (s *ClientContactService) WithMessaging(messenger domain.ContactMessenger, origins ...string) *ClientContactService {
	s.messenger = messenger
	if len(origins) > 0 {
		s.localOrigin = origins[0]
	}
	return s
}

// CompleteInbound associates retained requests and dispatches an unknown sender's pending name prompt.
func (s *ClientContactService) CompleteInbound(ctx context.Context, route domain.InboundChannelRoute, kind domain.ChannelType, value string) error {
	store, ok := s.contacts.(contactOnboardingStore)
	if !ok {
		return nil
	}
	value, err := domain.NormalizeClientContact(kind, value)
	if err != nil {
		return nil
	}
	var id *uuid.UUID
	err = s.db.InTenantTx(ctx, route.Tenant(), func(q repository.Querier) error {
		if err := store.AssociateObservedRequests(ctx, q, route.AccountID, kind, value); err != nil {
			return err
		}
		if s.messenger == nil {
			return nil
		}
		var err error
		id, err = store.ClaimNameRequest(ctx, q, route, kind, value)
		return err
	})
	if err != nil || id == nil {
		return err
	}
	message := "Para completar tus datos, necesitamos tu nombre y apellido. Respondé en una sola línea: Nombre y apellido: Juan Pérez"
	reference, deliveryErr := s.messenger.SendContact(ctx, domain.ContactMessage{AccountID: route.AccountID, BranchID: route.BranchID, ChannelID: route.ChannelID, Kind: kind, To: value, Body: message})
	return s.db.InTenantTx(ctx, route.Tenant(), func(q repository.Querier) error {
		return store.FinishNameRequest(ctx, q, route, *id, reference, deliveryErr == nil)
	})
}

type inboundNameReplyResolver interface {
	PendingNameRFQ(context.Context, repository.Querier, domain.InboundChannelRoute, domain.ChannelType, string) (*uuid.UUID, error)
}

// PendingNameRFQ identifies the request completed by a name-only channel reply.
func (s *ClientContactService) PendingNameRFQ(ctx context.Context, q repository.Querier, route domain.InboundChannelRoute, kind domain.ChannelType, raw string) (*uuid.UUID, error) {
	store, ok := s.contacts.(inboundNameReplyResolver)
	if !ok {
		return nil, nil
	}
	value, err := domain.NormalizeClientContact(kind, raw)
	if err != nil {
		return nil, err
	}
	return store.PendingNameRFQ(ctx, q, route, kind, value)
}
