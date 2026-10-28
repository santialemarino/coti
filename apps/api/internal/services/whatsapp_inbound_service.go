package services

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

const maxInboundProviderIdentifierLength = 255

type whatsAppInboundDB interface {
	tenantTxRunner
	WithAdvisoryLock(ctx context.Context, key string, fn func() error) error
	CrossAccount() repository.Querier
}

type whatsAppInboundRouter interface {
	ResolveChannels(ctx context.Context, channelType domain.ChannelType,
		identifiers []string) (map[string]domain.InboundChannelRoute, error)
}

type whatsAppInboundMessageStore interface {
	Reserve(ctx context.Context, q repository.Querier, accountID, branchID, channelID uuid.UUID,
		in domain.NewInboundChannelMessage) (*domain.InboundChannelMessage, error)
}

type whatsAppInboundConfigReader interface {
	ListActiveWhatsAppConfigurationsCrossAccount(ctx context.Context,
		q repository.Querier) ([]domain.ChannelConfiguration, error)
}

type whatsAppInboundConversations interface {
	Route(ctx context.Context, q repository.Querier, route domain.InboundChannelRoute,
		senderID string) (*domain.ConversationRoute, error)
	AppendMessage(ctx context.Context, q repository.Querier, tenant domain.Tenant,
		in domain.NewQuoteMessage) (*domain.QuoteMessage, error)
}

// errInboundAlreadyReserved rolls back a retried delivery's writes; its first outcome stands.
var errInboundAlreadyReserved = errors.New("inbound message already reserved")

type whatsAppInboundRFQCreator interface {
	CreateInboundTextDraftForMessage(ctx context.Context, route domain.InboundChannelRoute,
		in domain.InboundTextRFQDraftInput, rfqID uuid.UUID) (*domain.TextRFQDraft, error)
}

// WhatsAppInboundService turns verified WhatsApp messages into idempotent inbound RFQ drafts, or
// into client messages on the live quote conversation their sender continues.
type WhatsAppInboundService struct {
	contacts      inboundContactRecorder
	db            whatsAppInboundDB
	routes        whatsAppInboundRouter
	messages      whatsAppInboundMessageStore
	configs       whatsAppInboundConfigReader
	rfqs          whatsAppInboundRFQCreator
	conversations whatsAppInboundConversations
	opener        channelConfigOpener
	now           func() time.Time
	log           *slog.Logger
}

// NewWhatsAppInboundService builds a WhatsAppInboundService.
func NewWhatsAppInboundService(db whatsAppInboundDB, routes whatsAppInboundRouter,
	messages whatsAppInboundMessageStore, configs whatsAppInboundConfigReader,
	rfqs whatsAppInboundRFQCreator, opener channelConfigOpener, log *slog.Logger,
) *WhatsAppInboundService {
	if log == nil {
		log = slog.Default()
	}
	return &WhatsAppInboundService{db: db, routes: routes, messages: messages, configs: configs,
		rfqs: rfqs, opener: opener, now: time.Now, log: log}
}

// WithConversations routes messages from a sender with a live sent quote into its conversation.
func (s *WhatsAppInboundService) WithConversations(
	conversations whatsAppInboundConversations,
) *WhatsAppInboundService {
	s.conversations = conversations
	return s
}

// Verify reports whether a configured active WhatsApp channel accepts Meta's callback token.
func (s *WhatsAppInboundService) Verify(ctx context.Context, verifyToken string) (bool, error) {
	if s.db == nil || s.configs == nil || s.opener == nil {
		return false, fmt.Errorf("%w: WhatsApp webhook verification is not fully wired",
			domain.ErrNotConfigured)
	}
	verifyToken = strings.TrimSpace(verifyToken)
	if verifyToken == "" {
		return false, nil
	}
	configs, err := s.configs.ListActiveWhatsAppConfigurationsCrossAccount(ctx, s.db.CrossAccount())
	if err != nil {
		return false, err
	}
	for _, stored := range configs {
		parsed, parseErr := domain.ParseChannelConfig(domain.ChannelTypeWhatsApp, stored.Config)
		if parseErr != nil || parsed == nil {
			return false, fmt.Errorf("%w: stored WhatsApp channel configuration is invalid",
				domain.ErrNotConfigured)
		}
		if openErr := parsed.MapSecrets(s.opener.Open); openErr != nil {
			return false, fmt.Errorf("%w: stored WhatsApp channel credentials cannot be opened",
				domain.ErrNotConfigured)
		}
		config, ok := parsed.(*domain.WhatsAppChannelConfig)
		if !ok {
			return false, fmt.Errorf("%w: stored channel is not WhatsApp", domain.ErrNotConfigured)
		}
		if config.WebhookVerifyToken != "" &&
			subtle.ConstantTimeCompare([]byte(config.WebhookVerifyToken), []byte(verifyToken)) == 1 {
			return true, nil
		}
	}
	return false, nil
}

// Ingest stores and routes every verified WhatsApp text message in a provider delivery.
func (s *WhatsAppInboundService) Ingest(ctx context.Context, messages []domain.InboundTextMessage) error {
	if s.db == nil || s.routes == nil || s.messages == nil || s.rfqs == nil {
		return fmt.Errorf("%w: WhatsApp inbound intake is not fully wired", domain.ErrNotConfigured)
	}
	if len(messages) == 0 {
		return nil
	}

	normalized := make([]domain.InboundTextMessage, 0, len(messages))
	destinations := make([]string, 0, len(messages))
	for _, message := range messages {
		message, err := normalizeInboundTextMessage(message, s.now)
		if err != nil {
			return err
		}
		normalized = append(normalized, message)
		destinations = append(destinations, message.DestinationIdentifier)
	}

	routes, routingErr := s.routes.ResolveChannels(ctx, domain.ChannelTypeWhatsApp, destinations)
	if routingErr != nil && !errors.Is(routingErr, domain.ErrInboundDestinationInactive) {
		return routingErr
	}
	for _, message := range normalized {
		route, found := routes[message.DestinationIdentifier]
		if !found {
			if routingErr == nil {
				s.log.InfoContext(ctx, "whatsapp message ignored for an unconfigured destination")
			}
			continue
		}
		if err := s.ingestOne(ctx, route, message); err != nil {
			return err
		}
		if completer, ok := s.contacts.(inboundContactCompleter); ok {
			if err := completer.CompleteInbound(ctx, route, domain.ChannelTypeWhatsApp, "+"+strings.TrimPrefix(message.SenderID, "+"), domain.ContactReplyContext{}); err != nil {
				return err
			}
		}
	}
	return routingErr
}

func (s *WhatsAppInboundService) ingestOne(
	ctx context.Context, route domain.InboundChannelRoute, message domain.InboundTextMessage,
) error {
	lockKey := "whatsapp-inbound:" + route.ChannelID.String() + ":" + message.ExternalMessageID
	return s.db.WithAdvisoryLock(ctx, lockKey, func() error {
		var reserved *domain.InboundChannelMessage
		err := s.db.InTenantTx(ctx, route.Tenant(), func(q repository.Querier) error {
			in := domain.NewInboundChannelMessage{
				ID:                 uuid.New(),
				ExternalMessageID:  message.ExternalMessageID,
				SenderID:           message.SenderID,
				SenderLabel:        message.SenderLabel,
				Body:               message.Body,
				Payload:            message.Payload,
				ProviderReceivedAt: message.ProviderReceivedAt,
			}
			hints := channelClientData(message.Body, message.SenderLabel)
			// A live quote conversation claims the message first: «Confirmo» answers the quote.
			joined, err := s.joinConversation(ctx, q, route, message)
			if err != nil {
				return err
			}
			var pending *uuid.UUID
			var name string
			if resolver, ok := s.contacts.(inboundNameReplyResolver); ok && joined == nil {
				pending, name, err = resolver.NameReply(ctx, q, route, domain.ChannelTypeWhatsApp, "+"+strings.TrimPrefix(message.SenderID, "+"), message.Body)
				if err != nil {
					return err
				}
			}
			switch {
			case joined != nil:
				in.QuoteMessageID = &joined.ID
			case pending != nil:
				hints.Name = &name
				in.RFQID = pending
				var payload map[string]any
				if err := json.Unmarshal(in.Payload, &payload); err != nil {
					return err
				}
				payload["contact_name_reply"] = true
				in.Payload, _ = json.Marshal(payload)
			default:
				rfqID := uuid.New()
				in.RFQID = &rfqID
			}
			reserved, err = s.messages.Reserve(ctx, q, route.AccountID, route.BranchID,
				route.ChannelID, in)
			if err != nil {
				return err
			}
			// A retry keeps whatever the first delivery became; rolling back discards this attempt's join.
			if reserved.ID != in.ID {
				return errInboundAlreadyReserved
			}
			if s.contacts != nil {
				return s.contacts.RecordInbound(ctx, q, route, reserved.ID, domain.ChannelTypeWhatsApp, "+"+strings.TrimPrefix(message.SenderID, "+"), message.Body, hints)
			}
			return nil
		})
		if err != nil && !errors.Is(err, errInboundAlreadyReserved) {
			return err
		}
		var payload struct {
			NameReply bool `json:"contact_name_reply"`
		}
		_ = json.Unmarshal(reserved.Payload, &payload)
		if reserved.RFQID == nil || payload.NameReply {
			return nil
		}
		_, err = s.rfqs.CreateInboundTextDraftForMessage(ctx, route,
			domain.InboundTextRFQDraftInput{ClientLabel: message.SenderLabel, RawText: message.Body},
			*reserved.RFQID)
		return err
	})
}

// joinConversation appends the message to the live quote conversation its sender continues, or
// returns nil when the message starts a new order.
func (s *WhatsAppInboundService) joinConversation(ctx context.Context, q repository.Querier,
	route domain.InboundChannelRoute, message domain.InboundTextMessage,
) (*domain.QuoteMessage, error) {
	if s.conversations == nil {
		return nil, nil
	}
	match, err := s.conversations.Route(ctx, q, route, message.SenderID)
	if err != nil || match == nil {
		return nil, err
	}
	return s.conversations.AppendMessage(ctx, q, route.Tenant(), domain.NewQuoteMessage{
		ID: uuid.New(), QuoteID: match.QuoteID, ChannelID: &route.ChannelID,
		AuthorType: domain.MessageAuthorClient, Body: message.Body,
		ReceivedAt: message.ProviderReceivedAt, OtherLiveQuotes: match.OtherLiveQuotes,
	})
}

func normalizeInboundTextMessage(
	message domain.InboundTextMessage, now func() time.Time,
) (domain.InboundTextMessage, error) {
	destination, err := domain.NormalizeInboundChannelIdentifier(
		domain.ChannelTypeWhatsApp, message.DestinationIdentifier)
	if err != nil {
		return message, err
	}
	message.DestinationIdentifier = destination
	message.ExternalMessageID = strings.TrimSpace(message.ExternalMessageID)
	message.SenderID = strings.TrimSpace(message.SenderID)
	message.Body = strings.TrimSpace(message.Body)
	if message.ExternalMessageID == "" || message.SenderID == "" || message.Body == "" {
		return message, fmt.Errorf("%w: inbound WhatsApp message is incomplete", domain.ErrInvalidInput)
	}
	if len(message.ExternalMessageID) > maxInboundProviderIdentifierLength ||
		len(message.SenderID) > maxInboundProviderIdentifierLength {
		return message, fmt.Errorf("%w: inbound WhatsApp provider identifiers are too long",
			domain.ErrInvalidInput)
	}
	if message.SenderLabel != nil {
		label := strings.TrimSpace(*message.SenderLabel)
		if label == "" {
			message.SenderLabel = nil
		} else if utf8.RuneCountInString(label) > 255 {
			return message, fmt.Errorf("%w: inbound WhatsApp sender label is too long", domain.ErrInvalidInput)
		} else {
			message.SenderLabel = &label
		}
	}
	if message.ProviderReceivedAt.IsZero() {
		message.ProviderReceivedAt = now().UTC()
	}
	if len(message.Payload) == 0 {
		return message, fmt.Errorf("%w: inbound WhatsApp payload is required", domain.ErrInvalidInput)
	}
	return message, nil
}
