//go:build integration

package repository

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

func seedChannel(
	t *testing.T, db *DB, accountID, branchID uuid.UUID, channelType domain.ChannelType,
	isActive bool,
) uuid.UUID {
	t.Helper()
	id := uuid.New()
	identifier := uuid.NewString()
	if _, err := db.CrossAccount().Exec(context.Background(),
		`INSERT INTO channel (id, account_id, branch_id, type, identifier, is_active)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		id, accountID, branchID, channelType, identifier, isActive); err != nil {
		t.Fatalf("seed channel: %v", err)
	}
	t.Cleanup(func() {
		mustCleanup(t, db.CrossAccount(), `DELETE FROM channel WHERE id = $1`, id)
	})
	return id
}

// seedChannelWithIdentifier is seedChannel for the tests that need to choose the identifier,
// including choosing none.
func seedChannelWithIdentifier(
	t *testing.T, db *DB, accountID, branchID uuid.UUID, channelType domain.ChannelType,
	identifier *string,
) uuid.UUID {
	t.Helper()
	id := uuid.New()
	if _, err := db.CrossAccount().Exec(context.Background(),
		`INSERT INTO channel (id, account_id, branch_id, type, identifier)
		 VALUES ($1, $2, $3, $4, $5)`,
		id, accountID, branchID, channelType, identifier); err != nil {
		t.Fatalf("seed channel: %v", err)
	}
	t.Cleanup(func() {
		mustCleanup(t, db.CrossAccount(), `DELETE FROM channel WHERE id = $1`, id)
	})
	return id
}

func TestChannelRepository_ActiveReadsStayInsideAccountAndBranch(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	accountA := seedAccount(t, db, "Channel account A")
	accountB := seedAccount(t, db, "Channel account B")
	branchA := branchOf(t, db, accountA)
	branchB := branchOf(t, db, accountB)
	branchASecond := seedExtraBranch(t, db, accountA, "Channel account A second")
	activeA := seedChannel(t, db, accountA, branchA, domain.ChannelTypeWhatsApp, true)
	inactiveA := seedChannel(t, db, accountA, branchA, domain.ChannelTypeEmail, false)
	activeASecond := seedChannel(t, db, accountA, branchASecond, domain.ChannelTypeWhatsApp, true)
	activeB := seedChannel(t, db, accountB, branchB, domain.ChannelTypeWhatsApp, true)
	repo := NewChannelRepository()

	var channels []domain.Channel
	if err := db.InTenantTx(ctx, domain.Tenant{AccountID: accountA, BranchID: branchA},
		func(q Querier) error {
			var listErr error
			channels, listErr = repo.ListActiveByBranch(ctx, q, accountA, branchA)
			return listErr
		}); err != nil {
		t.Fatalf("ListActiveByBranch() = %v, want no error", err)
	}
	if len(channels) != 1 || channels[0].ID != activeA {
		t.Fatalf("ListActiveByBranch() = %#v, want only active channel %v", channels, activeA)
	}

	for _, test := range []struct {
		name      string
		accountID uuid.UUID
		branchID  uuid.UUID
		channelID uuid.UUID
	}{
		{name: "inactive", accountID: accountA, branchID: branchA, channelID: inactiveA},
		{name: "other account", accountID: accountA, branchID: branchA, channelID: activeB},
		{name: "other branch", accountID: accountA, branchID: branchA, channelID: activeASecond},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := db.InTenantTx(ctx, domain.Tenant{AccountID: accountA, BranchID: branchA},
				func(q Querier) error {
					_, getErr := repo.GetActiveByID(ctx, q, test.accountID, test.branchID,
						test.channelID)
					return getErr
				})
			if !errors.Is(err, domain.ErrNotFound) {
				t.Errorf("GetActiveByID() = %v, want %v", err, domain.ErrNotFound)
			}
		})
	}
}

func TestChannelRepository_ListActiveByTypeFiltersTheSelectedBranch(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	accountID := seedAccount(t, db, "Typed channels")
	branchID := branchOf(t, db, accountID)
	whatsAppID := seedChannel(t, db, accountID, branchID, domain.ChannelTypeWhatsApp, true)
	seedChannel(t, db, accountID, branchID, domain.ChannelTypeEmail, true)
	repo := NewChannelRepository()

	var channels []domain.Channel
	err := db.InTenantTx(ctx, domain.Tenant{AccountID: accountID, BranchID: branchID},
		func(q Querier) error {
			var listErr error
			channels, listErr = repo.ListActiveByType(ctx, q, accountID, branchID,
				domain.ChannelTypeWhatsApp)
			return listErr
		})
	if err != nil {
		t.Fatalf("ListActiveByType() = %v, want no error", err)
	}
	if len(channels) != 1 || channels[0].ID != whatsAppID {
		t.Fatalf("ListActiveByType() = %#v, want WhatsApp channel %v", channels, whatsAppID)
	}
}

// cleanupChannel removes a channel a repository method created, rather than the seed. One row, one
// owner: a second teardown for the same row is what puts a delete ahead of a foreign key.
func cleanupChannel(t *testing.T, db *DB, id uuid.UUID) {
	t.Helper()
	t.Cleanup(func() {
		mustCleanup(t, db.CrossAccount(), `DELETE FROM channel WHERE id = $1`, id)
	})
}

// readStoredConfig reads channel.config straight out of the table. Nothing in the product reads it
// back — the repository selects only whether it exists — so the round trip is asserted here.
func readStoredConfig(t *testing.T, db *DB, id uuid.UUID) map[string]any {
	t.Helper()
	var raw []byte
	if err := db.CrossAccount().QueryRow(context.Background(),
		`SELECT config FROM channel WHERE id = $1`, id).Scan(&raw); err != nil {
		t.Fatalf("read stored config: %v", err)
	}
	if raw == nil {
		return nil
	}
	var stored map[string]any
	if err := json.Unmarshal(raw, &stored); err != nil {
		t.Fatalf("stored config is not an object: %v", err)
	}
	return stored
}

func TestChannelRepository_CreateRoundTripsTheConfigVerbatim(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	accountID := seedAccount(t, db, "Channel config round trip")
	branchID := branchOf(t, db, accountID)
	repo := NewChannelRepository()
	identifier := "+5491100000000"
	// Every JSON scalar the shapes use, plus the characters an escaping bug would mangle.
	config := []byte(`{"phone_number_id":"1234567890","business_account_id":"9876",` +
		`"access_token":"v1.ábç\"quote\\slash/","webhook_verify_token":"","smtp_port":587,` +
		`"smtp_starttls":true}`)

	var created *domain.Channel
	if err := db.InTenantTx(ctx, domain.Tenant{AccountID: accountID, BranchID: branchID},
		func(q Querier) error {
			var createErr error
			created, createErr = repo.Create(ctx, q, accountID, branchID, domain.NewChannel{
				Type: domain.ChannelTypeWhatsApp, Identifier: &identifier, Config: config,
			})
			return createErr
		}); err != nil {
		t.Fatalf("Create() = %v, want no error", err)
	}
	cleanupChannel(t, db, created.ID)

	if created.Type != domain.ChannelTypeWhatsApp || created.Identifier == nil ||
		*created.Identifier != identifier {
		t.Errorf("Create() = %+v, want a WHATSAPP channel on %q", created, identifier)
	}
	if !created.IsConfigured || !created.IsActive {
		t.Errorf("Create() = configured %v active %v, want both true", created.IsConfigured,
			created.IsActive)
	}

	var want map[string]any
	if err := json.Unmarshal(config, &want); err != nil {
		t.Fatalf("test fixture is not valid JSON: %v", err)
	}
	stored := readStoredConfig(t, db, created.ID)
	if len(stored) != len(want) {
		t.Fatalf("stored config = %#v, want %d fields", stored, len(want))
	}
	for field, value := range want {
		if stored[field] != value {
			t.Errorf("stored %s = %#v, want %#v", field, stored[field], value)
		}
	}
}

func TestChannelRepository_WritesStayInsideAccountAndBranch(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	accountA := seedAccount(t, db, "Channel writes A")
	accountB := seedAccount(t, db, "Channel writes B")
	branchA := branchOf(t, db, accountA)
	branchB := branchOf(t, db, accountB)
	branchASecond := seedExtraBranch(t, db, accountA, "Channel writes A second")
	victim := seedChannel(t, db, accountB, branchB, domain.ChannelTypeWhatsApp, true)
	sibling := seedChannel(t, db, accountA, branchASecond, domain.ChannelTypeWhatsApp, true)
	repo := NewChannelRepository()

	// On the owner pool, which is RLS-exempt: inside InTenantTx the policy refuses another
	// account's row first, so the application predicates cannot be told apart from it.
	tx, err := db.AdminTx(ctx)
	if err != nil {
		t.Fatalf("AdminTx() = %v, want no error", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	for _, test := range []struct {
		name      string
		channelID uuid.UUID
	}{
		{name: "another account", channelID: victim},
		{name: "another branch of the same account", channelID: sibling},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, getErr := repo.GetByID(ctx, tx, accountA, branchA, test.channelID); !errors.Is(
				getErr, domain.ErrNotFound) {
				t.Errorf("GetByID() = %v, want %v", getErr, domain.ErrNotFound)
			}
			if _, updateErr := repo.Update(ctx, tx, accountA, branchA, test.channelID,
				domain.ChannelUpdate{}); !errors.Is(updateErr, domain.ErrNotFound) {
				t.Errorf("Update() = %v, want %v", updateErr, domain.ErrNotFound)
			}
			if closeErr := repo.Deactivate(ctx, tx, accountA, branchA,
				test.channelID); !errors.Is(closeErr, domain.ErrNotFound) {
				t.Errorf("Deactivate() = %v, want %v", closeErr, domain.ErrNotFound)
			}
		})
	}
}

// Another account's row is in another branch too, so the branch predicate refuses it first and
// either guard alone keeps the suite green. Only a row carrying THIS branch with ANOTHER account
// isolates the account predicate, and only the owner pool gets past RLS to reach it.
func TestChannelRepository_AccountPredicateRefusesAMismatchedRow(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	accountA := seedAccount(t, db, "Channel mismatch A")
	accountB := seedAccount(t, db, "Channel mismatch B")
	branchA := branchOf(t, db, accountA)
	mismatched := seedChannel(t, db, accountB, branchA, domain.ChannelTypeWhatsApp, true)
	repo := NewChannelRepository()

	tx, err := db.AdminTx(ctx)
	if err != nil {
		t.Fatalf("AdminTx() = %v, want no error", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, getErr := repo.GetByID(ctx, tx, accountA, branchA, mismatched); !errors.Is(
		getErr, domain.ErrNotFound) {
		t.Errorf("GetByID() = %v, want %v", getErr, domain.ErrNotFound)
	}
	if _, getErr := repo.GetActiveByID(ctx, tx, accountA, branchA, mismatched); !errors.Is(
		getErr, domain.ErrNotFound) {
		t.Errorf("GetActiveByID() = %v, want %v", getErr, domain.ErrNotFound)
	}
	if _, updateErr := repo.Update(ctx, tx, accountA, branchA, mismatched,
		domain.ChannelUpdate{}); !errors.Is(updateErr, domain.ErrNotFound) {
		t.Errorf("Update() = %v, want %v", updateErr, domain.ErrNotFound)
	}
	if closeErr := repo.Deactivate(ctx, tx, accountA, branchA, mismatched); !errors.Is(
		closeErr, domain.ErrNotFound) {
		t.Errorf("Deactivate() = %v, want %v", closeErr, domain.ErrNotFound)
	}

	var channels []domain.Channel
	channels, err = repo.ListAllByBranch(ctx, tx, accountA, branchA)
	if err != nil {
		t.Fatalf("ListAllByBranch() = %v, want no error", err)
	}
	for _, channel := range channels {
		if channel.ID == mismatched {
			t.Errorf("ListAllByBranch() returned the mismatched channel %v", mismatched)
		}
	}
}

func TestChannelRepository_UpdateKeepsOrClearsTheConfig(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	accountID := seedAccount(t, db, "Channel config update")
	branchID := branchOf(t, db, accountID)
	repo := NewChannelRepository()
	tenant := domain.Tenant{AccountID: accountID, BranchID: branchID}

	for _, test := range []struct {
		name           string
		update         domain.ChannelUpdate
		wantConfigured bool
		wantToken      string
	}{
		{name: "nothing sent keeps it", update: domain.ChannelUpdate{},
			wantConfigured: true, wantToken: "first"},
		{name: "a config replaces it",
			update:         domain.ChannelUpdate{Config: []byte(`{"access_token":"second"}`)},
			wantConfigured: true, wantToken: "second"},
		{name: "clearing removes it",
			update:         domain.ChannelUpdate{ClearConfig: true},
			wantConfigured: false},
		{name: "clearing wins over a config sent alongside it",
			update: domain.ChannelUpdate{ClearConfig: true,
				Config: []byte(`{"access_token":"third"}`)},
			wantConfigured: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			var channelID uuid.UUID
			if err := db.InTenantTx(ctx, tenant, func(q Querier) error {
				created, createErr := repo.Create(ctx, q, accountID, branchID, domain.NewChannel{
					Type:       domain.ChannelTypeWhatsApp,
					Identifier: ptrString(uuid.NewString()),
					Config:     []byte(`{"access_token":"first"}`),
				})
				if createErr != nil {
					return createErr
				}
				channelID = created.ID
				return nil
			}); err != nil {
				t.Fatalf("Create() = %v, want no error", err)
			}
			cleanupChannel(t, db, channelID)

			var updated *domain.Channel
			if err := db.InTenantTx(ctx, tenant, func(q Querier) error {
				var updateErr error
				updated, updateErr = repo.Update(ctx, q, accountID, branchID, channelID,
					test.update)
				return updateErr
			}); err != nil {
				t.Fatalf("Update() = %v, want no error", err)
			}

			if updated.IsConfigured != test.wantConfigured {
				t.Errorf("is_configured = %v, want %v", updated.IsConfigured, test.wantConfigured)
			}
			stored := readStoredConfig(t, db, channelID)
			if !test.wantConfigured {
				if stored != nil {
					t.Errorf("stored config = %#v, want NULL", stored)
				}
				return
			}
			if stored["access_token"] != test.wantToken {
				t.Errorf("stored access_token = %#v, want %q", stored["access_token"],
					test.wantToken)
			}
		})
	}
}

func TestChannelRepository_UpdateReplacesTheIdentifierAndTheFlag(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	accountID := seedAccount(t, db, "Channel identifier update")
	branchID := branchOf(t, db, accountID)
	repo := NewChannelRepository()
	tenant := domain.Tenant{AccountID: accountID, BranchID: branchID}
	channelID := seedChannel(t, db, accountID, branchID, domain.ChannelTypeWhatsApp, true)
	replacement := "+5491199999999"
	inactive := false

	var updated *domain.Channel
	if err := db.InTenantTx(ctx, tenant, func(q Querier) error {
		var updateErr error
		updated, updateErr = repo.Update(ctx, q, accountID, branchID, channelID,
			domain.ChannelUpdate{Identifier: &replacement, IsActive: &inactive})
		return updateErr
	}); err != nil {
		t.Fatalf("Update() = %v, want no error", err)
	}
	if updated.Identifier == nil || *updated.Identifier != replacement {
		t.Errorf("identifier = %v, want %q", updated.Identifier, replacement)
	}
	if updated.IsActive {
		t.Error("is_active = true, want the flag turned off")
	}

	// Omitting the identifier clears it, the way every other PUT in the API replaces a record;
	// omitting the flag leaves it where the previous write put it.
	if err := db.InTenantTx(ctx, tenant, func(q Querier) error {
		var updateErr error
		updated, updateErr = repo.Update(ctx, q, accountID, branchID, channelID,
			domain.ChannelUpdate{})
		return updateErr
	}); err != nil {
		t.Fatalf("Update() = %v, want no error", err)
	}
	if updated.Identifier != nil {
		t.Errorf("identifier = %q, want it cleared", *updated.Identifier)
	}
	if updated.IsActive {
		t.Error("is_active = true, want it left off")
	}
}

func TestChannelRepository_CreateReportsBothUniquenessRules(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	accountID := seedAccount(t, db, "Channel uniqueness")
	branchID := branchOf(t, db, accountID)
	repo := NewChannelRepository()
	tenant := domain.Tenant{AccountID: accountID, BranchID: branchID}
	identifier := "+5491100000001"

	create := func(t *testing.T, in domain.NewChannel) error {
		t.Helper()
		return db.InTenantTx(ctx, tenant, func(q Querier) error {
			created, err := repo.Create(ctx, q, accountID, branchID, in)
			if created != nil {
				cleanupChannel(t, db, created.ID)
			}
			return err
		})
	}

	withIdentifier := domain.NewChannel{Type: domain.ChannelTypeEmail, Identifier: &identifier}
	if err := create(t, withIdentifier); err != nil {
		t.Fatalf("Create() = %v, want no error", err)
	}
	if err := create(t, withIdentifier); !errors.Is(err, domain.ErrConflict) {
		t.Errorf("Create() with a duplicate identifier = %v, want %v", err, domain.ErrConflict)
	}

	withoutIdentifier := domain.NewChannel{Type: domain.ChannelTypeWebApp}
	if err := create(t, withoutIdentifier); err != nil {
		t.Fatalf("Create() = %v, want no error", err)
	}
	if err := create(t, withoutIdentifier); !errors.Is(err, domain.ErrConflict) {
		t.Errorf("Create() with a second identifier-less channel = %v, want %v", err,
			domain.ErrConflict)
	}
}

// Update answers the same two uniqueness rules Create does, which is easy to miss: taking a
// sibling's identifier trips the composite constraint, and clearing one when the branch already
// holds an identifier-less channel of that type trips the partial index.
func TestChannelRepository_UpdateReportsBothUniquenessRules(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	accountID := seedAccount(t, db, "Channel update uniqueness")
	branchID := branchOf(t, db, accountID)
	repo := NewChannelRepository()
	tenant := domain.Tenant{AccountID: accountID, BranchID: branchID}
	taken := "+5491100000002"
	moving := seedChannel(t, db, accountID, branchID, domain.ChannelTypeWhatsApp, true)
	seedChannelWithIdentifier(t, db, accountID, branchID, domain.ChannelTypeWhatsApp, &taken)
	seedChannelWithIdentifier(t, db, accountID, branchID, domain.ChannelTypeWhatsApp, nil)

	for _, test := range []struct {
		name       string
		identifier *string
	}{
		{name: "a sibling's identifier", identifier: &taken},
		{name: "no identifier when one already has none", identifier: nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := db.InTenantTx(ctx, tenant, func(q Querier) error {
				_, updateErr := repo.Update(ctx, q, accountID, branchID, moving,
					domain.ChannelUpdate{Identifier: test.identifier})
				return updateErr
			})
			if !errors.Is(err, domain.ErrConflict) {
				t.Errorf("Update() = %v, want %v", err, domain.ErrConflict)
			}
		})
	}
}

func TestChannelRepository_ListAllByBranchIncludesTheClosedOnes(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	accountID := seedAccount(t, db, "Channel administrative list")
	otherAccount := seedAccount(t, db, "Channel administrative list other")
	branchID := branchOf(t, db, accountID)
	otherBranch := seedExtraBranch(t, db, accountID, "Channel administrative list second")
	active := seedChannel(t, db, accountID, branchID, domain.ChannelTypeWhatsApp, true)
	closed := seedChannel(t, db, accountID, branchID, domain.ChannelTypeEmail, false)
	seedChannel(t, db, accountID, otherBranch, domain.ChannelTypeWhatsApp, true)
	seedChannel(t, db, otherAccount, branchOf(t, db, otherAccount), domain.ChannelTypeWhatsApp, true)
	repo := NewChannelRepository()

	var channels []domain.Channel
	if err := db.InTenantTx(ctx, domain.Tenant{AccountID: accountID, BranchID: branchID},
		func(q Querier) error {
			var listErr error
			channels, listErr = repo.ListAllByBranch(ctx, q, accountID, branchID)
			return listErr
		}); err != nil {
		t.Fatalf("ListAllByBranch() = %v, want no error", err)
	}

	got := make(map[uuid.UUID]bool, len(channels))
	for _, channel := range channels {
		got[channel.ID] = channel.IsActive
	}
	if len(got) != 2 {
		t.Fatalf("ListAllByBranch() = %#v, want exactly this branch's two channels", channels)
	}
	if !got[active] {
		t.Errorf("channel %v missing or inactive, want it active", active)
	}
	if _, present := got[closed]; !present || got[closed] {
		t.Errorf("closed channel %v = %v, want it listed as inactive", closed, got[closed])
	}
}

func ptrString(s string) *string { return &s }
