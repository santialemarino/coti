package services

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

type fakeQuoteDeliveryWhatsAppChannels struct {
	configs map[uuid.UUID][]byte
	seen    []uuid.UUID
}

func (f *fakeQuoteDeliveryWhatsAppChannels) ListActiveByBranch(
	context.Context, repository.Querier, uuid.UUID, uuid.UUID,
) ([]domain.Channel, error) {
	return nil, nil
}

func (f *fakeQuoteDeliveryWhatsAppChannels) GetActiveConfigurationsByIDs(
	_ context.Context, _ repository.Querier, _ uuid.UUID, _ uuid.UUID, channelIDs []uuid.UUID,
) (map[uuid.UUID][]byte, error) {
	f.seen = append(f.seen, channelIDs...)
	return f.configs, nil
}

type fakeQuoteDeliveryChannelOpener struct{}

func (fakeQuoteDeliveryChannelOpener) Open(value string) (string, error) {
	return strings.TrimPrefix(value, "sealed:"), nil
}

func TestQuoteDeliveryService_WhatsAppCredentialsOpenOnlyForDispatch(t *testing.T) {
	t.Parallel()
	channelID := uuid.New()
	channels := &fakeQuoteDeliveryWhatsAppChannels{configs: map[uuid.UUID][]byte{channelID: []byte(
		`{"phone_number_id":"phone-id","access_token":"sealed:access-token"}`)}}
	service := NewQuoteDeliveryService(&fakePublicQuoteDB{}, nil, nil, channels, nil, nil, nil,
		nil, nil, "", nil, nil).WithWhatsAppCredentials(fakeQuoteDeliveryChannelOpener{})
	tenant := domain.Tenant{AccountID: uuid.New(), BranchID: uuid.New()}

	credentials, err := service.whatsAppCredentials(context.Background(), tenant, []domain.QuoteSend{{
		ChannelID: channelID, ChannelType: domain.ChannelTypeWhatsApp,
	}})
	if err != nil {
		t.Fatalf("whatsAppCredentials() = %v, want no error", err)
	}
	credential, found := credentials[channelID]
	if !found || credential.PhoneNumberID != "phone-id" || credential.AccessToken != "access-token" {
		t.Errorf("credentials = %#v, want opened WhatsApp credentials", credentials)
	}
	if len(channels.seen) != 1 || channels.seen[0] != channelID {
		t.Errorf("configuration read IDs = %v, want [%s]", channels.seen, channelID)
	}
}
