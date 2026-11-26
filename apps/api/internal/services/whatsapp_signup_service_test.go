package services

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

type signupTestDB struct{ fakeDB }

func (d *signupTestDB) WithAdvisoryQuerier(_ context.Context, _ string, fn func(repository.Querier) error) error {
	return fn(nil)
}

type signupTestStore struct {
	fakeChannelStore
	owner uuid.UUID
}

func (s *signupTestStore) GetActiveByTypeAndIdentifiersCrossAccount(_ context.Context, _ repository.Querier, _ domain.ChannelType, ids []string) (map[string]domain.Channel, error) {
	if s.owner == uuid.Nil {
		return nil, nil
	}
	return map[string]domain.Channel{ids[0]: {ID: s.owner}}, nil
}
func (s *signupTestStore) HasWhatsAppReceipt(context.Context, repository.Querier, uuid.UUID, uuid.UUID, uuid.UUID) (bool, error) {
	return true, nil
}

type signupTestProvider struct {
	registrationError error
	calls             int
	pin               string
}

func (p *signupTestProvider) Authorize(context.Context, string, string, string) (domain.WhatsAppSignupGrant, error) {
	p.calls++
	return domain.WhatsAppSignupGrant{Identifier: "+5491155550101", AccessToken: "test-business-token", NeedsRegistration: true}, nil
}
func (p *signupTestProvider) Subscribe(context.Context, string, string) error { return nil }
func (p *signupTestProvider) Register(_ context.Context, _, _, pin string) error {
	p.pin = pin
	return p.registrationError
}

func TestWhatsAppSignupWritesOnlyAfterRegistration(t *testing.T) {
	for _, tc := range []struct {
		name                         string
		owner                        uuid.UUID
		registrationError, errorWant error
	}{
		{name: "authorized"},
		{name: "already routed", owner: uuid.New(), errorWant: domain.ErrConflict},
		{name: "registration failed", registrationError: errors.New("Meta unavailable"), errorWant: domain.ErrDeliveryUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := &signupTestDB{}
			store := &signupTestStore{fakeChannelStore: fakeChannelStore{current: &domain.Channel{ID: testChannelID, Type: domain.ChannelTypeWhatsApp}}, owner: tc.owner}
			sealer := testSealer(t, true)
			provider := &signupTestProvider{registrationError: tc.registrationError}
			signup := NewWhatsAppSignupService(db, store, provider, NewChannelService(db, store, sealer), "test-app-secret")
			err := signup.Connect(t.Context(), branchTenant(), testChannelID, "one-use", "456", "123")
			if !errors.Is(err, tc.errorWant) {
				t.Fatalf("Connect() = %v, want %v", err, tc.errorWant)
			}
			if tc.errorWant != nil {
				if store.updated != nil || (tc.owner != uuid.Nil && provider.calls != 0) {
					t.Fatal("failed signup changed the stored channel or called Meta for a claimed phone")
				}
				return
			}
			if len(provider.pin) != 6 || store.updated == nil || store.accountID != testAccountID || store.branchID != testBranchID || store.updatedID != testChannelID {
				t.Fatal("signup did not register and update the scoped channel")
			}
			var config domain.WhatsAppChannelConfig
			if err := json.Unmarshal(store.updated.Config, &config); err != nil {
				t.Fatal(err)
			}
			token, err := sealer.Open(config.AccessToken)
			if err != nil || token != "test-business-token" || config.AccessToken == token || config.BusinessAccountID != "456" || config.PhoneNumberID != "123" {
				t.Fatal("unexpected or unencrypted saved credentials")
			}
		})
	}
}
