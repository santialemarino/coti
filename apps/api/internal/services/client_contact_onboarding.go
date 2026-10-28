package services

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

type contactOnboardingStore interface {
	CompleteNameRequest(context.Context, repository.Querier, uuid.UUID, uuid.UUID, domain.ChannelType, string, string) error
	QueueNameRequest(context.Context, repository.Querier, domain.InboundChannelRoute, domain.ChannelType, string) error
	AttachContactObservations(context.Context, repository.Querier, uuid.UUID, uuid.UUID, domain.ChannelType, string) error
	AssociateObservedRequests(context.Context, repository.Querier, uuid.UUID, domain.ChannelType, string) error
	ClaimNameRequest(context.Context, repository.Querier, domain.InboundChannelRoute, domain.ChannelType, string, time.Duration) (*uuid.UUID, error)
	FinishNameRequest(context.Context, repository.Querier, domain.InboundChannelRoute, uuid.UUID, string, bool) error
}

type inboundContactCompleter interface {
	CompleteInbound(context.Context, domain.InboundChannelRoute, domain.ChannelType, string, domain.ContactReplyContext) error
}

// namePromptSettings holds the delivery tuning of the name prompt outbox.
type namePromptSettings struct {
	// lease is how long a claimed prompt may stay in delivery before another claim takes it over.
	lease time.Duration
}

// WithMessaging enables fixed name prompts after messages and requests have been retained.
func (s *ClientContactService) WithMessaging(messenger domain.ContactMessenger, origins ...string) *ClientContactService {
	s.messenger = messenger
	if len(origins) > 0 {
		s.localOrigin = origins[0]
	}
	return s
}

// WithPromptLease lets a prompt stranded in delivery by a crash be claimed again after lease.
func (s *ClientContactService) WithPromptLease(lease time.Duration) *ClientContactService {
	s.prompts.lease = lease
	return s
}

// CompleteInbound associates retained requests and dispatches an unknown sender's pending name prompt.
func (s *ClientContactService) CompleteInbound(ctx context.Context, route domain.InboundChannelRoute, kind domain.ChannelType, value string, reply domain.ContactReplyContext) error {
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
		if s.messenger == nil || reply.Automated {
			return nil
		}
		var err error
		id, err = store.ClaimNameRequest(ctx, q, route, kind, value, s.prompts.lease)
		return err
	})
	if err != nil || id == nil {
		return err
	}
	reference, deliveryErr := s.messenger.SendContact(ctx, domain.ContactMessage{AccountID: route.AccountID, BranchID: route.BranchID, ChannelID: route.ChannelID, Purpose: domain.ContactPurposeNameRequest, Kind: kind, To: value, Body: contactNamePrompt, InReplyTo: reply.MessageID})
	return s.db.InTenantTx(ctx, route.Tenant(), func(q repository.Querier) error {
		return store.FinishNameRequest(ctx, q, route, *id, reference, deliveryErr == nil)
	})
}

type inboundNameReplyResolver interface {
	PendingNameRFQ(context.Context, repository.Querier, domain.InboundChannelRoute, domain.ChannelType, string) (*uuid.UUID, error)
}

type pendingNameStore interface {
	PendingNameRFQ(context.Context, repository.Querier, domain.InboundChannelRoute, domain.ChannelType, string, time.Duration) (*uuid.UUID, error)
}

// PendingNameRFQ identifies the request a name reply completes while its prompt is still answerable.
func (s *ClientContactService) PendingNameRFQ(ctx context.Context, q repository.Querier, route domain.InboundChannelRoute, kind domain.ChannelType, raw string) (*uuid.UUID, error) {
	store, ok := s.contacts.(pendingNameStore)
	if !ok {
		return nil, nil
	}
	value, err := domain.NormalizeClientContact(kind, raw)
	if err != nil {
		return nil, err
	}
	return store.PendingNameRFQ(ctx, q, route, kind, value, nameReplyWindow)
}
