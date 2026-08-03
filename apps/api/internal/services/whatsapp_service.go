package services

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

// WhatsAppInboundTextMessage is one text message received from Meta's webhook.
type WhatsAppInboundTextMessage struct {
	PhoneNumberID     string
	ExternalMessageID string
	SenderID          string
	SenderName        string
	Body              string
	ReceivedAt        time.Time
	RawPayload        json.RawMessage
}

// WhatsAppIngestResult reports how much of a webhook delivery became work.
type WhatsAppIngestResult struct {
	Received    int
	CreatedRFQs int
	Duplicates  int
	Ignored     int
}

type whatsappDB interface {
	tenantTxRunner
	CrossAccount() repository.Querier
}

type whatsappChannelReader interface {
	GetActiveByTypeAndIdentifiersCrossAccount(
		ctx context.Context, q repository.Querier, typ domain.ChannelType, identifiers []string,
	) (map[string]domain.Channel, error)
}

type whatsappRFQWriter interface {
	Create(ctx context.Context, q repository.Querier, in domain.NewRFQ) (*domain.RFQ, error)
	CreateInboundMessage(
		ctx context.Context, q repository.Querier, in domain.NewInboundChannelMessage,
	) (*domain.InboundChannelMessage, bool, error)
	AttachInboundMessageToRFQ(ctx context.Context, q repository.Querier, accountID, messageID, rfqID uuid.UUID) error
}

// WhatsAppService ingests WhatsApp Cloud API webhook messages into the RFQ inbox.
type WhatsAppService struct {
	db       whatsappDB
	channels whatsappChannelReader
	rfqs     whatsappRFQWriter
	now      func() time.Time
}

// NewWhatsAppService builds a WhatsAppService.
func NewWhatsAppService(
	db whatsappDB, channels whatsappChannelReader, rfqs whatsappRFQWriter, now func() time.Time,
) *WhatsAppService {
	if now == nil {
		now = time.Now
	}
	return &WhatsAppService{db: db, channels: channels, rfqs: rfqs, now: now}
}

// IngestTextMessages persists each new WhatsApp text message as an RFQ source.
func (s *WhatsAppService) IngestTextMessages(
	ctx context.Context, messages []WhatsAppInboundTextMessage,
) (WhatsAppIngestResult, error) {
	result := WhatsAppIngestResult{Received: len(messages)}
	candidates := make([]WhatsAppInboundTextMessage, 0, len(messages))
	identifiers := make([]string, 0, len(messages))
	seenIdentifiers := map[string]struct{}{}

	for _, msg := range messages {
		msg.PhoneNumberID = strings.TrimSpace(msg.PhoneNumberID)
		msg.ExternalMessageID = strings.TrimSpace(msg.ExternalMessageID)
		msg.SenderID = strings.TrimSpace(msg.SenderID)
		msg.Body = strings.TrimSpace(msg.Body)
		if msg.PhoneNumberID == "" || msg.ExternalMessageID == "" || msg.SenderID == "" || msg.Body == "" {
			result.Ignored++
			continue
		}
		if msg.ReceivedAt.IsZero() {
			msg.ReceivedAt = s.now()
		}
		if len(msg.RawPayload) == 0 {
			msg.RawPayload = json.RawMessage(`{}`)
		}
		candidates = append(candidates, msg)
		if _, ok := seenIdentifiers[msg.PhoneNumberID]; !ok {
			seenIdentifiers[msg.PhoneNumberID] = struct{}{}
			identifiers = append(identifiers, msg.PhoneNumberID)
		}
	}

	channels, err := s.channels.GetActiveByTypeAndIdentifiersCrossAccount(
		ctx, s.db.CrossAccount(), domain.ChannelTypeWhatsApp, identifiers)
	if err != nil {
		return result, err
	}

	for _, msg := range candidates {
		channel, ok := channels[msg.PhoneNumberID]
		if !ok {
			result.Ignored++
			continue
		}
		outcome, err := s.ingestTextMessage(ctx, channel, msg)
		if err != nil {
			return result, err
		}
		switch outcome {
		case whatsappIngestCreated:
			result.CreatedRFQs++
		case whatsappIngestDuplicate:
			result.Duplicates++
		}
	}
	return result, nil
}

type whatsappIngestOutcome string

const (
	whatsappIngestCreated   whatsappIngestOutcome = "created"
	whatsappIngestDuplicate whatsappIngestOutcome = "duplicate"
)

func (s *WhatsAppService) ingestTextMessage(
	ctx context.Context, channel domain.Channel, msg WhatsAppInboundTextMessage,
) (whatsappIngestOutcome, error) {
	var outcome whatsappIngestOutcome
	tenant := domain.Tenant{AccountID: channel.AccountID}

	err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		stored, created, err := s.rfqs.CreateInboundMessage(ctx, q, domain.NewInboundChannelMessage{
			AccountID:         channel.AccountID,
			BranchID:          channel.BranchID,
			ChannelID:         channel.ID,
			ExternalMessageID: msg.ExternalMessageID,
			ExternalSenderID:  msg.SenderID,
			Body:              msg.Body,
			RawPayload:        msg.RawPayload,
			ReceivedAt:        msg.ReceivedAt,
		})
		if err != nil {
			return err
		}
		if !created {
			outcome = whatsappIngestDuplicate
			return nil
		}

		label := whatsappClientLabel(msg)
		rfq, err := s.rfqs.Create(ctx, q, domain.NewRFQ{
			AccountID:   channel.AccountID,
			BranchID:    channel.BranchID,
			ChannelID:   channel.ID,
			RawText:     msg.Body,
			ReceivedAt:  msg.ReceivedAt,
			ClientLabel: &label,
		})
		if err != nil {
			return err
		}
		if err := s.rfqs.AttachInboundMessageToRFQ(ctx, q, channel.AccountID, stored.ID, rfq.ID); err != nil {
			return err
		}
		outcome = whatsappIngestCreated
		return nil
	})
	if err != nil {
		return "", err
	}
	if outcome == "" {
		return "", fmt.Errorf("%w: whatsapp ingest produced no outcome", domain.ErrInvalidInput)
	}
	return outcome, nil
}

func whatsappClientLabel(msg WhatsAppInboundTextMessage) string {
	if strings.TrimSpace(msg.SenderName) != "" {
		return msg.SenderName + " (WhatsApp " + msg.SenderID + ")"
	}
	return "WhatsApp " + msg.SenderID
}
