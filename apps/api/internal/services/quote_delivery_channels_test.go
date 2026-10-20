package services

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

type deliveryChannelsFixture struct {
	channels []domain.Channel
	account  uuid.UUID
	branch   uuid.UUID
}

func (f *deliveryChannelsFixture) ListActiveByBranch(_ context.Context, _ repository.Querier,
	accountID, branchID uuid.UUID,
) ([]domain.Channel, error) {
	f.account, f.branch = accountID, branchID
	return f.channels, nil
}

func TestQuoteDeliveryService_SelectedChannels(t *testing.T) {
	t.Parallel()
	whatsappID, otherWhatsAppID := uuid.New(), uuid.New()
	emailID, otherEmailID, webID := uuid.New(), uuid.New(), uuid.New()
	email := "buyer@example.test"
	whatsapp := domain.Channel{ID: whatsappID, Type: domain.ChannelTypeWhatsApp}
	otherWhatsApp := domain.Channel{ID: otherWhatsAppID, Type: domain.ChannelTypeWhatsApp}
	emailChannel := domain.Channel{ID: emailID, Type: domain.ChannelTypeEmail}
	otherEmail := domain.Channel{ID: otherEmailID, Type: domain.ChannelTypeEmail}
	web := domain.Channel{ID: webID, Type: domain.ChannelTypeWebApp}
	tests := []struct {
		name     string
		channels []domain.Channel
		origin   uuid.UUID
		input    domain.QuoteDeliveryInput
		want     []uuid.UUID
		wantErr  bool
	}{
		{"original WhatsApp among two", []domain.Channel{otherWhatsApp, whatsapp, web}, whatsappID,
			domain.QuoteDeliveryInput{Phone: "+5491155550101"}, []uuid.UUID{whatsappID, webID}, false},
		{"original email among two", []domain.Channel{otherEmail, web, emailChannel}, emailID,
			domain.QuoteDeliveryInput{Email: &email}, []uuid.UUID{emailID, webID}, false},
		{"another medium is ambiguous", []domain.Channel{whatsapp, otherWhatsApp, emailChannel, web}, emailID,
			domain.QuoteDeliveryInput{Phone: "+5491155550101"}, nil, true},
		{"inactive origin is not selected", []domain.Channel{otherWhatsApp, web}, whatsappID,
			domain.QuoteDeliveryInput{Phone: "+5491155550101"}, []uuid.UUID{otherWhatsAppID, webID}, false},
		{"inactive origin does not resolve other channels", []domain.Channel{otherWhatsApp, {ID: uuid.New(), Type: domain.ChannelTypeWhatsApp}, web}, whatsappID,
			domain.QuoteDeliveryInput{Phone: "+5491155550101"}, nil, true},
		{"missing channel is rejected", []domain.Channel{web}, whatsappID,
			domain.QuoteDeliveryInput{Phone: "+5491155550101"}, nil, true},
		{"one channel per selected medium", []domain.Channel{whatsapp, emailChannel, web}, emailID,
			domain.QuoteDeliveryInput{Phone: "+5491155550101", Email: &email}, []uuid.UUID{whatsappID, emailID, webID}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fixture := &deliveryChannelsFixture{channels: tt.channels}
			service := &QuoteDeliveryService{channels: fixture}
			tenant := domain.Tenant{AccountID: uuid.New(), BranchID: uuid.New()}
			got, err := service.selectedChannels(context.Background(), nil, tenant, tt.input, tt.origin)
			if tt.wantErr {
				if domain.CodeOf(err) != domain.CodeDeliveryChannel {
					t.Fatalf("error = %v, want delivery channel error", err)
				}
				return
			}
			if err != nil || len(got) != len(tt.want) {
				t.Fatalf("channels = %v, error = %v, want %v", got, err, tt.want)
			}
			for i, id := range tt.want {
				if got[i].ID != id {
					t.Errorf("channel %d = %s, want %s", i, got[i].ID, id)
				}
			}
			if fixture.account != tenant.AccountID || fixture.branch != tenant.BranchID {
				t.Errorf("scope = %s/%s, want %s/%s", fixture.account, fixture.branch, tenant.AccountID, tenant.BranchID)
			}
		})
	}
}
