package services

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

func TestChannelService_UpdateWhatsAppChannel_PreservesOmittedCredentials(t *testing.T) {
	t.Parallel()
	sealer := testSealer(t, true)
	oldToken, _ := sealer.Seal("old-access-token")
	verifier, _ := sealer.Seal("existing-webhook-verifier")
	stored := domain.WhatsAppChannelConfig{PhoneNumberID: "1234567890", BusinessAccountID: "9876543210",
		AccessToken: oldToken, WebhookVerifyToken: verifier}
	raw, _ := json.Marshal(stored)
	store := &fakeChannelStore{configuration: raw, current: &domain.Channel{
		ID: testChannelID, Type: domain.ChannelTypeWhatsApp, Identifier: ptr("+5491155550101"), IsConfigured: true}}
	db := &fakeDB{}
	service := NewChannelService(db, store, sealer)
	_, err := service.UpdateWhatsAppChannel(context.Background(), branchTenant(), testChannelID,
		domain.WhatsAppChannelUpdate{AccessToken: ptr("new-access-token")})
	if err != nil {
		t.Fatalf("UpdateWhatsAppChannel() = %v", err)
	}
	if store.accountID != testAccountID || store.branchID != testBranchID || store.updatedID != testChannelID {
		t.Fatal("update did not retain the selected account, branch and channel")
	}
	var got domain.WhatsAppChannelConfig
	if err := json.Unmarshal(store.updated.Config, &got); err != nil {
		t.Fatal(err)
	}
	if got.PhoneNumberID != stored.PhoneNumberID || got.BusinessAccountID != stored.BusinessAccountID ||
		got.WebhookVerifyToken != verifier || *store.updated.Identifier != "+5491155550101" {
		t.Fatal("token rotation changed an omitted setting")
	}
	opened, err := sealer.Open(got.AccessToken)
	if err != nil || opened != "new-access-token" {
		t.Fatalf("new access token was not encrypted correctly: %v", err)
	}
	if strings.Contains(string(store.updated.Config), "new-access-token") || store.updated.ClearConfig {
		t.Fatal("token rotation exposed plaintext or cleared the configuration")
	}
}

func TestChannelService_UpdateWhatsAppChannel_RequiresCompleteNumberChanges(t *testing.T) {
	t.Parallel()
	for _, input := range []domain.WhatsAppChannelUpdate{
		{Identifier: ptr("+541143210000")},
		{PhoneNumberID: ptr("2222222222")},
		{BusinessAccountID: ptr("3333333333")},
	} {
		store := &fakeChannelStore{configuration: []byte(`{"phone_number_id":"1234567890","business_account_id":"9876543210","access_token":"sealed-access","webhook_verify_token":"sealed-verifier"}`),
			current: &domain.Channel{Type: domain.ChannelTypeWhatsApp, Identifier: ptr("+5491155550101")}}
		service := NewChannelService(&fakeDB{}, store, testSealer(t, true))
		_, err := service.UpdateWhatsAppChannel(context.Background(), branchTenant(), testChannelID, input)
		if domain.CodeOf(err) != domain.CodeChannelConfigShape || store.updated != nil {
			t.Fatalf("incomplete number change = %v, wrote = %v", err, store.updated != nil)
		}
	}
}

func TestChannelService_UpdateWhatsAppChannel_AcceptsRealNumbersAndInitialSetup(t *testing.T) {
	t.Parallel()
	for _, identifier := range []string{"+541143210000", "+5491155550101", "+15556530731"} {
		store := &fakeChannelStore{current: &domain.Channel{Type: domain.ChannelTypeWhatsApp}}
		service := NewChannelService(&fakeDB{}, store, testSealer(t, true))
		_, err := service.UpdateWhatsAppChannel(context.Background(), branchTenant(), testChannelID,
			domain.WhatsAppChannelUpdate{Identifier: &identifier, PhoneNumberID: ptr("1234567890"),
				BusinessAccountID: ptr("9876543210"), AccessToken: ptr("access-token"), WebhookVerifyToken: ptr("verifier")})
		if err != nil || store.updated == nil || *store.updated.Identifier != identifier {
			t.Fatalf("setup %s = %v, want the number unchanged", identifier, err)
		}
	}
}

func TestChannelService_UpdateWhatsAppChannel_RejectsInvalidUpdatesWithoutWriting(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name  string
		input domain.WhatsAppChannelUpdate
	}{
		{"blank access token", domain.WhatsAppChannelUpdate{AccessToken: ptr("")}},
		{"blank verifier", domain.WhatsAppChannelUpdate{WebhookVerifyToken: ptr(" ")}},
		{"oversized unicode token", domain.WhatsAppChannelUpdate{AccessToken: ptr(strings.Repeat("ñ", 2049))}},
		{"nonnumeric provider id", domain.WhatsAppChannelUpdate{PhoneNumberID: ptr("invalid")}},
		{"incomplete initial setup", domain.WhatsAppChannelUpdate{AccessToken: ptr("access")}},
		{"missing initial verifier", domain.WhatsAppChannelUpdate{Identifier: ptr("+541143210000"),
			PhoneNumberID: ptr("123"), BusinessAccountID: ptr("456"), AccessToken: ptr("access")}},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := &fakeChannelStore{current: &domain.Channel{Type: domain.ChannelTypeWhatsApp}}
			service := NewChannelService(&fakeDB{}, store, testSealer(t, true))
			_, err := service.UpdateWhatsAppChannel(context.Background(), branchTenant(), testChannelID, test.input)
			if domain.CodeOf(err) != domain.CodeChannelConfigShape || store.updated != nil {
				t.Fatalf("invalid update = %v, wrote = %v", err, store.updated != nil)
			}
		})
	}
}

func TestChannelService_UpdateWhatsAppChannel_RefusesOtherScopesAndMissingKey(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name    string
		tenant  domain.Tenant
		channel *domain.Channel
		key     bool
		want    error
	}{
		{"seller", domain.Tenant{AccountID: testAccountID, BranchID: testBranchID, Role: domain.UserRoleSeller}, nil, true, domain.ErrForbidden},
		{"missing channel", branchTenant(), nil, true, domain.ErrNotFound},
		{"wrong type", branchTenant(), &domain.Channel{Type: domain.ChannelTypeEmail}, true, domain.ErrNotFound},
		{"missing key", branchTenant(), &domain.Channel{Type: domain.ChannelTypeWhatsApp, Identifier: ptr("+541143210000")}, false, domain.ErrNotConfigured},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := &fakeChannelStore{current: test.channel, configuration: []byte(`{"phone_number_id":"123","access_token":"sealed-access","webhook_verify_token":"sealed-verifier"}`)}
			service := NewChannelService(&fakeDB{}, store, testSealer(t, test.key))
			_, err := service.UpdateWhatsAppChannel(context.Background(), test.tenant, testChannelID,
				domain.WhatsAppChannelUpdate{AccessToken: ptr("access")})
			if !errors.Is(err, test.want) || store.updated != nil {
				t.Fatalf("update = %v, want %v without writes", err, test.want)
			}
		})
	}
}
