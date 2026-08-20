package services

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
	"github.com/santialemarino/coti/apps/api/internal/secrets"
)

type fakeChannelStore struct {
	channels  []domain.Channel
	current   *domain.Channel
	err       error
	accountID uuid.UUID
	branchID  uuid.UUID
	listedAll bool
	created   *domain.NewChannel
	updated   *domain.ChannelUpdate
	updatedID uuid.UUID
	closedID  uuid.UUID
}

func (f *fakeChannelStore) ListActiveByBranch(
	_ context.Context, _ repository.Querier, accountID, branchID uuid.UUID,
) ([]domain.Channel, error) {
	f.accountID, f.branchID = accountID, branchID
	return f.channels, f.err
}

func (f *fakeChannelStore) ListAllByBranch(
	_ context.Context, _ repository.Querier, accountID, branchID uuid.UUID,
) ([]domain.Channel, error) {
	f.accountID, f.branchID, f.listedAll = accountID, branchID, true
	return f.channels, f.err
}

func (f *fakeChannelStore) GetByID(
	_ context.Context, _ repository.Querier, accountID, branchID, _ uuid.UUID,
) (*domain.Channel, error) {
	f.accountID, f.branchID = accountID, branchID
	if f.current == nil {
		return nil, domain.ErrNotFound
	}
	return f.current, f.err
}

func (f *fakeChannelStore) Create(
	_ context.Context, _ repository.Querier, accountID, branchID uuid.UUID, in domain.NewChannel,
) (*domain.Channel, error) {
	f.accountID, f.branchID, f.created = accountID, branchID, &in
	if f.err != nil {
		return nil, f.err
	}
	return &domain.Channel{ID: testChannelID, AccountID: accountID, BranchID: branchID,
		Type: in.Type, Identifier: in.Identifier, IsConfigured: in.Config != nil}, nil
}

func (f *fakeChannelStore) Update(
	_ context.Context, _ repository.Querier, accountID, branchID, channelID uuid.UUID,
	in domain.ChannelUpdate,
) (*domain.Channel, error) {
	f.accountID, f.branchID, f.updatedID, f.updated = accountID, branchID, channelID, &in
	if f.err != nil {
		return nil, f.err
	}
	return &domain.Channel{ID: channelID, AccountID: accountID, BranchID: branchID,
		Identifier: in.Identifier, IsConfigured: in.Config != nil}, nil
}

func (f *fakeChannelStore) Deactivate(
	_ context.Context, _ repository.Querier, accountID, branchID, channelID uuid.UUID,
) error {
	f.accountID, f.branchID, f.closedID = accountID, branchID, channelID
	return f.err
}

func testSealer(t *testing.T, withKey bool) *secrets.AESGCM {
	t.Helper()
	var key []byte
	if withKey {
		key = make([]byte, secrets.KeyLength)
		for i := range key {
			key[i] = byte(i + 1)
		}
	}
	sealer, err := secrets.NewAESGCM(key)
	if err != nil {
		t.Fatalf("NewAESGCM() = %v, want no error", err)
	}
	return sealer
}

const whatsAppConfigJSON = `{"phone_number_id":"1234567890","access_token":"EAAG-token"}`

func TestChannelService_ListChannels_ScopesToSelectedBranch(t *testing.T) {
	db := &fakeDB{}
	want := []domain.Channel{{
		ID: testChannelID, AccountID: testAccountID, BranchID: testBranchID,
		Type: domain.ChannelTypeWhatsApp, IsActive: true,
	}}
	store := &fakeChannelStore{channels: want}
	service := NewChannelService(db, store, testSealer(t, true))

	got, err := service.ListChannels(context.Background(), branchTenant())
	if err != nil {
		t.Fatalf("ListChannels() = %v, want no error", err)
	}
	if len(got) != 1 || got[0].ID != testChannelID {
		t.Fatalf("ListChannels() = %#v, want %#v", got, want)
	}
	if store.accountID != testAccountID || store.branchID != testBranchID {
		t.Errorf("repository scope = %v/%v, want %v/%v", store.accountID, store.branchID,
			testAccountID, testBranchID)
	}
	if store.listedAll {
		t.Error("ListChannels() read the administrative list, want the active one")
	}
	if len(db.scopes) != 1 || db.scopes[0] != testAccountID {
		t.Errorf("tenant scopes = %v, want [%v]", db.scopes, testAccountID)
	}
}

func TestChannelService_ListChannels_RequiresSelectedBranch(t *testing.T) {
	db := &fakeDB{}
	store := &fakeChannelStore{}
	service := NewChannelService(db, store, testSealer(t, true))
	tenant := domain.Tenant{
		AccountID: testAccountID, UserID: testUserID, Role: domain.UserRoleAdmin,
	}

	_, err := service.ListChannels(context.Background(), tenant)
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("ListChannels() = %v, want %v", err, domain.ErrInvalidInput)
	}
	if len(db.scopes) != 0 {
		t.Errorf("tenant scopes = %v, want none without a selected branch", db.scopes)
	}
}

func TestChannelService_ListAllChannels_IsAdminOnly(t *testing.T) {
	db := &fakeDB{}
	store := &fakeChannelStore{channels: []domain.Channel{{ID: testChannelID}}}
	service := NewChannelService(db, store, testSealer(t, true))

	// A seller WITH a branch selected, so the refusal can only be the role and not the branch.
	seller := domain.Tenant{AccountID: testAccountID, UserID: testUserID,
		Role: domain.UserRoleSeller, BranchID: testBranchID}

	if _, err := service.ListAllChannels(context.Background(), seller); !errors.Is(
		err, domain.ErrForbidden) {
		t.Fatalf("ListAllChannels() as a seller = %v, want %v", err, domain.ErrForbidden)
	}
	if store.listedAll {
		t.Fatal("ListAllChannels() reached the repository for a seller")
	}
	if len(db.scopes) != 0 {
		t.Errorf("tenant scopes = %v, want none for a refused read", db.scopes)
	}

	if _, err := service.ListAllChannels(context.Background(), branchTenant()); err != nil {
		t.Fatalf("ListAllChannels() as an admin = %v, want no error", err)
	}
	if !store.listedAll {
		t.Error("ListAllChannels() did not read the administrative list")
	}
}

func TestChannelService_CreateChannel_SealsEveryCredential(t *testing.T) {
	db := &fakeDB{}
	store := &fakeChannelStore{}
	sealer := testSealer(t, true)
	service := NewChannelService(db, store, sealer)

	channel, err := service.CreateChannel(context.Background(), branchTenant(), domain.NewChannel{
		Type:   domain.ChannelTypeWhatsApp,
		Config: []byte(whatsAppConfigJSON),
	})
	if err != nil {
		t.Fatalf("CreateChannel() = %v, want no error", err)
	}
	if !channel.IsConfigured {
		t.Error("created channel reports no configuration, want one")
	}
	if store.created == nil {
		t.Fatal("CreateChannel() never reached the repository")
	}

	var stored map[string]string
	if err := json.Unmarshal(store.created.Config, &stored); err != nil {
		t.Fatalf("stored config is not an object of strings: %v", err)
	}
	if stored["phone_number_id"] != "1234567890" {
		t.Errorf("stored phone_number_id = %q, want it in the clear", stored["phone_number_id"])
	}
	if strings.Contains(string(store.created.Config), "EAAG-token") {
		t.Fatalf("stored config carries the access token in the clear: %s", store.created.Config)
	}
	opened, err := sealer.Open(stored["access_token"])
	if err != nil {
		t.Fatalf("Open(stored access_token) = %v, want no error", err)
	}
	if opened != "EAAG-token" {
		t.Errorf("Open(stored access_token) = %q, want %q", opened, "EAAG-token")
	}
}

func TestChannelService_CreateChannel_RefusesCredentialsWithNoKey(t *testing.T) {
	db := &fakeDB{}
	store := &fakeChannelStore{}
	service := NewChannelService(db, store, testSealer(t, false))

	_, err := service.CreateChannel(context.Background(), branchTenant(), domain.NewChannel{
		Type:   domain.ChannelTypeWhatsApp,
		Config: []byte(whatsAppConfigJSON),
	})
	if !errors.Is(err, domain.ErrNotConfigured) {
		t.Fatalf("CreateChannel() = %v, want %v", err, domain.ErrNotConfigured)
	}
	if domain.CodeOf(err) != domain.CodeNotConfigured {
		t.Errorf("CodeOf() = %v, want %v", domain.CodeOf(err), domain.CodeNotConfigured)
	}
	if store.created != nil {
		t.Error("CreateChannel() wrote a channel with no key to seal its credential")
	}
	if len(db.scopes) != 0 {
		t.Errorf("tenant scopes = %v, want none: nothing should have opened a transaction",
			db.scopes)
	}

	// A channel with no configuration needs no key, so the same deployment can still open one.
	if _, err := service.CreateChannel(context.Background(), branchTenant(), domain.NewChannel{
		Type: domain.ChannelTypeWebApp,
	}); err != nil {
		t.Fatalf("CreateChannel(WEBAPP) = %v, want no error without a key", err)
	}
}

func TestChannelService_CreateChannel_RefusesAShapeThatDoesNotMatchTheType(t *testing.T) {
	for _, test := range []struct {
		name        string
		in          domain.NewChannel
		wantCode    domain.ErrorCode
		wantMessage string
	}{
		{
			name:        "unknown type",
			in:          domain.NewChannel{Type: domain.ChannelType("TELEGRAM")},
			wantCode:    domain.CodeInvalidInput,
			wantMessage: `unknown channel type "TELEGRAM"`,
		},
		{
			name: "config of the wrong type",
			in: domain.NewChannel{Type: domain.ChannelTypeEmail,
				Config: []byte(whatsAppConfigJSON)},
			wantCode:    domain.CodeChannelConfigShape,
			wantMessage: `unknown field "phone_number_id"`,
		},
		{
			name: "config on a type that takes none",
			in: domain.NewChannel{Type: domain.ChannelTypeWebApp,
				Config: []byte(whatsAppConfigJSON)},
			wantCode:    domain.CodeChannelConfigShape,
			wantMessage: "a WEBAPP channel takes no configuration",
		},
		{
			name: "identifier on a type that carries none",
			in: domain.NewChannel{Type: domain.ChannelTypeManualEntry,
				Identifier: ptr("mostrador")},
			wantCode:    domain.CodeChannelIdentifier,
			wantMessage: "carries no identifier",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			db := &fakeDB{}
			store := &fakeChannelStore{}
			service := NewChannelService(db, store, testSealer(t, true))

			_, err := service.CreateChannel(context.Background(), branchTenant(), test.in)
			if !errors.Is(err, domain.ErrInvalidInput) {
				t.Fatalf("CreateChannel() = %v, want %v", err, domain.ErrInvalidInput)
			}
			if domain.CodeOf(err) != test.wantCode {
				t.Errorf("CodeOf() = %v, want %v", domain.CodeOf(err), test.wantCode)
			}
			if !strings.Contains(err.Error(), test.wantMessage) {
				t.Errorf("CreateChannel() = %q, want it to mention %q", err, test.wantMessage)
			}
			if store.created != nil {
				t.Error("CreateChannel() wrote a channel the validation refused")
			}
		})
	}
}

func TestChannelService_CreateChannel_NormalizesABlankIdentifier(t *testing.T) {
	db := &fakeDB{}
	store := &fakeChannelStore{}
	service := NewChannelService(db, store, testSealer(t, true))

	if _, err := service.CreateChannel(context.Background(), branchTenant(), domain.NewChannel{
		Type: domain.ChannelTypeWhatsApp, Identifier: ptr("  +5491100000000  "),
	}); err != nil {
		t.Fatalf("CreateChannel() = %v, want no error", err)
	}
	if store.created == nil || store.created.Identifier == nil {
		t.Fatal("CreateChannel() dropped the identifier")
	}
	if *store.created.Identifier != "+5491100000000" {
		t.Errorf("stored identifier = %q, want it trimmed", *store.created.Identifier)
	}

	store.created = nil
	if _, err := service.CreateChannel(context.Background(), branchTenant(), domain.NewChannel{
		Type: domain.ChannelTypeWebApp, Identifier: ptr("   "),
	}); err != nil {
		t.Fatalf("CreateChannel() = %v, want a blank identifier to read as absent", err)
	}
	if store.created == nil || store.created.Identifier != nil {
		t.Errorf("stored identifier = %v, want nil: an empty string is not NULL, so it would "+
			"slip past the partial unique index", store.created.Identifier)
	}
}

func TestChannelService_UpdateChannel_ConfigAbsentKeepsItAndNullClearsIt(t *testing.T) {
	for _, test := range []struct {
		name        string
		config      []byte
		wantClear   bool
		wantWritten bool
	}{
		{name: "absent leaves it alone", config: nil},
		{name: "null removes it", config: []byte("null"), wantClear: true},
		{name: "empty object removes it", config: []byte("{}"), wantClear: true},
		{name: "a shape replaces it", config: []byte(whatsAppConfigJSON), wantWritten: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			db := &fakeDB{}
			store := &fakeChannelStore{current: &domain.Channel{
				ID: testChannelID, Type: domain.ChannelTypeWhatsApp, IsConfigured: true,
			}}
			service := NewChannelService(db, store, testSealer(t, true))

			if _, err := service.UpdateChannel(context.Background(), branchTenant(), testChannelID,
				domain.ChannelUpdate{Config: test.config}); err != nil {
				t.Fatalf("UpdateChannel() = %v, want no error", err)
			}
			if store.updated == nil {
				t.Fatal("UpdateChannel() never reached the repository")
			}
			if store.updatedID != testChannelID {
				t.Errorf("updated channel = %v, want %v", store.updatedID, testChannelID)
			}
			if store.updated.ClearConfig != test.wantClear {
				t.Errorf("ClearConfig = %v, want %v", store.updated.ClearConfig, test.wantClear)
			}
			if (store.updated.Config != nil) != test.wantWritten {
				t.Errorf("Config = %s, want written = %v", store.updated.Config, test.wantWritten)
			}
			if test.wantWritten && strings.Contains(string(store.updated.Config), "EAAG-token") {
				t.Errorf("stored config carries the token in the clear: %s", store.updated.Config)
			}
		})
	}
}

func TestChannelService_UpdateChannel_ValidatesAgainstTheStoredType(t *testing.T) {
	db := &fakeDB{}
	store := &fakeChannelStore{current: &domain.Channel{
		ID: testChannelID, Type: domain.ChannelTypeEmail,
	}}
	service := NewChannelService(db, store, testSealer(t, true))

	_, err := service.UpdateChannel(context.Background(), branchTenant(), testChannelID,
		domain.ChannelUpdate{Config: []byte(whatsAppConfigJSON)})
	if domain.CodeOf(err) != domain.CodeChannelConfigShape {
		t.Fatalf("UpdateChannel() = %v (%v), want %v", err, domain.CodeOf(err),
			domain.CodeChannelConfigShape)
	}
	if store.updated != nil {
		t.Error("UpdateChannel() wrote a config the stored type does not accept")
	}
}

func TestChannelService_ClosingTheManualEntryChannelIsRefused(t *testing.T) {
	for _, test := range []struct {
		name        string
		channelType domain.ChannelType
		wantRefused bool
	}{
		{name: "manual entry", channelType: domain.ChannelTypeManualEntry, wantRefused: true},
		{name: "whatsapp", channelType: domain.ChannelTypeWhatsApp},
	} {
		t.Run(test.name, func(t *testing.T) {
			db := &fakeDB{}
			store := &fakeChannelStore{current: &domain.Channel{
				ID: testChannelID, Type: test.channelType, IsActive: true,
			}}
			service := NewChannelService(db, store, testSealer(t, true))
			closed := false

			err := service.DeactivateChannel(context.Background(), branchTenant(), testChannelID)
			if store.closedID == testChannelID {
				closed = true
			}
			if !test.wantRefused {
				if err != nil {
					t.Fatalf("DeactivateChannel() = %v, want no error", err)
				}
				if !closed {
					t.Error("DeactivateChannel() did not close the channel")
				}
				return
			}
			if domain.CodeOf(err) != domain.CodeManualEntryChannel {
				t.Fatalf("DeactivateChannel() = %v (%v), want %v", err, domain.CodeOf(err),
					domain.CodeManualEntryChannel)
			}
			if closed {
				t.Error("DeactivateChannel() closed the branch's manual-entry channel")
			}

			// The same guard has to hold on the flag, or PUT is a way around DELETE.
			isActive := false
			_, err = service.UpdateChannel(context.Background(), branchTenant(), testChannelID,
				domain.ChannelUpdate{IsActive: &isActive})
			if domain.CodeOf(err) != domain.CodeManualEntryChannel {
				t.Fatalf("UpdateChannel(is_active=false) = %v (%v), want %v", err,
					domain.CodeOf(err), domain.CodeManualEntryChannel)
			}
			if store.updated != nil {
				t.Error("UpdateChannel() closed the branch's manual-entry channel")
			}
		})
	}
}

func TestChannelService_WritesRequireSelectedBranch(t *testing.T) {
	tenant := domain.Tenant{
		AccountID: testAccountID, UserID: testUserID, Role: domain.UserRoleAdmin,
	}

	for _, test := range []struct {
		name string
		call func(*ChannelService) error
	}{
		{name: "create", call: func(s *ChannelService) error {
			_, err := s.CreateChannel(context.Background(), tenant,
				domain.NewChannel{Type: domain.ChannelTypeWebApp})
			return err
		}},
		{name: "update", call: func(s *ChannelService) error {
			_, err := s.UpdateChannel(context.Background(), tenant, testChannelID,
				domain.ChannelUpdate{})
			return err
		}},
		{name: "deactivate", call: func(s *ChannelService) error {
			return s.DeactivateChannel(context.Background(), tenant, testChannelID)
		}},
		{name: "list all", call: func(s *ChannelService) error {
			_, err := s.ListAllChannels(context.Background(), tenant)
			return err
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			db := &fakeDB{}
			store := &fakeChannelStore{current: &domain.Channel{ID: testChannelID}}
			service := NewChannelService(db, store, testSealer(t, true))

			if err := test.call(service); !errors.Is(err, domain.ErrInvalidInput) {
				t.Fatalf("%s = %v, want %v", test.name, err, domain.ErrInvalidInput)
			}
			if len(db.scopes) != 0 {
				t.Errorf("tenant scopes = %v, want none without a selected branch", db.scopes)
			}
		})
	}
}
