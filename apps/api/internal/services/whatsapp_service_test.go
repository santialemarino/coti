package services

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

type fakeWhatsAppDB struct{}

func (fakeWhatsAppDB) InTenantTx(
	ctx context.Context, _ domain.Tenant, fn func(repository.Querier) error,
) error {
	return fn(nil)
}

func (fakeWhatsAppDB) CrossAccount() repository.Querier {
	return nil
}

type fakeWhatsAppChannels struct {
	channels map[string]domain.Channel
}

func (f fakeWhatsAppChannels) GetActiveByTypeAndIdentifiersCrossAccount(
	_ context.Context, _ repository.Querier, _ domain.ChannelType, identifiers []string,
) (map[string]domain.Channel, error) {
	out := map[string]domain.Channel{}
	for _, identifier := range identifiers {
		if channel, ok := f.channels[identifier]; ok {
			out[identifier] = channel
		}
	}
	return out, nil
}

type fakeWhatsAppRFQs struct {
	duplicateIDs map[string]bool
	messages     []domain.NewInboundChannelMessage
	rfqs         []domain.NewRFQ
	attachments  []uuid.UUID
}

func (f *fakeWhatsAppRFQs) Create(
	_ context.Context, _ repository.Querier, in domain.NewRFQ,
) (*domain.RFQ, error) {
	f.rfqs = append(f.rfqs, in)
	id := uuid.New()
	return &domain.RFQ{ID: id, AccountID: in.AccountID, BranchID: in.BranchID}, nil
}

func (f *fakeWhatsAppRFQs) CreateInboundMessage(
	_ context.Context, _ repository.Querier, in domain.NewInboundChannelMessage,
) (*domain.InboundChannelMessage, bool, error) {
	if f.duplicateIDs[in.ExternalMessageID] {
		return nil, false, nil
	}
	f.messages = append(f.messages, in)
	id := uuid.New()
	return &domain.InboundChannelMessage{ID: id}, true, nil
}

func (f *fakeWhatsAppRFQs) AttachInboundMessageToRFQ(
	_ context.Context, _ repository.Querier, _ uuid.UUID, messageID, _ uuid.UUID,
) error {
	f.attachments = append(f.attachments, messageID)
	return nil
}

func TestWhatsAppService_IngestTextMessagesCreatesRFQs(t *testing.T) {
	accountID := uuid.New()
	branchID := uuid.New()
	channelID := uuid.New()
	phoneNumberID := "123456789"
	repo := &fakeWhatsAppRFQs{duplicateIDs: map[string]bool{}}
	service := NewWhatsAppService(fakeWhatsAppDB{}, fakeWhatsAppChannels{
		channels: map[string]domain.Channel{
			phoneNumberID: {
				ID:         channelID,
				AccountID:  accountID,
				BranchID:   branchID,
				Type:       domain.ChannelTypeWhatsApp,
				Identifier: &phoneNumberID,
				IsActive:   true,
			},
		},
	}, repo, func() time.Time { return time.Unix(1785614500, 0).UTC() })

	result, err := service.IngestTextMessages(context.Background(), []WhatsAppInboundTextMessage{{
		PhoneNumberID:     phoneNumberID,
		ExternalMessageID: "wamid.test",
		SenderID:          "5491112345678",
		SenderName:        "Test Client",
		Body:              "I need 20 cement bags",
		RawPayload:        json.RawMessage(`{"ok":true}`),
	}})
	if err != nil {
		t.Fatalf("IngestTextMessages() = %v", err)
	}

	if result.CreatedRFQs != 1 || result.Duplicates != 0 || result.Ignored != 0 {
		t.Fatalf("result = %#v, want one created RFQ", result)
	}
	if len(repo.messages) != 1 || len(repo.rfqs) != 1 || len(repo.attachments) != 1 {
		t.Fatalf("messages=%d rfqs=%d attachments=%d", len(repo.messages), len(repo.rfqs), len(repo.attachments))
	}
	if got := repo.rfqs[0].RawText; got != "I need 20 cement bags" {
		t.Errorf("rfq raw text = %q", got)
	}
	if repo.rfqs[0].ClientLabel == nil || *repo.rfqs[0].ClientLabel != "Test Client (WhatsApp 5491112345678)" {
		t.Errorf("client label = %v", repo.rfqs[0].ClientLabel)
	}
}

func TestWhatsAppService_IngestTextMessagesCountsDuplicatesAndUnknownChannels(t *testing.T) {
	repo := &fakeWhatsAppRFQs{duplicateIDs: map[string]bool{"wamid.duplicate": true}}
	service := NewWhatsAppService(fakeWhatsAppDB{}, fakeWhatsAppChannels{
		channels: map[string]domain.Channel{
			"known": {
				ID:         uuid.New(),
				AccountID:  uuid.New(),
				BranchID:   uuid.New(),
				Type:       domain.ChannelTypeWhatsApp,
				Identifier: stringPtr("known"),
				IsActive:   true,
			},
		},
	}, repo, nil)

	result, err := service.IngestTextMessages(context.Background(), []WhatsAppInboundTextMessage{
		{PhoneNumberID: "known", ExternalMessageID: "wamid.duplicate", SenderID: "5491112345678", Body: "hello"},
		{PhoneNumberID: "missing", ExternalMessageID: "wamid.missing", SenderID: "5491112345678", Body: "hello"},
	})
	if err != nil {
		t.Fatalf("IngestTextMessages() = %v", err)
	}
	if result.Duplicates != 1 || result.Ignored != 1 || result.CreatedRFQs != 0 {
		t.Fatalf("result = %#v, want duplicate and ignored", result)
	}
	if len(repo.rfqs) != 0 {
		t.Fatalf("created RFQs = %d, want 0", len(repo.rfqs))
	}
}

func stringPtr(v string) *string {
	return &v
}
