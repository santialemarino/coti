//go:build integration

package repository

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// seedTextRFQ stores one order and removes it, plus whatever quote chain hangs off it, when the
// test ends.
func seedTextRFQ(t *testing.T, db *DB, accountID, branchID, channelID uuid.UUID) *domain.RFQ {
	t.Helper()
	ctx := context.Background()
	repo := NewRFQRepository()
	raw := "Necesito cemento"

	var rfq *domain.RFQ
	err := db.InTenantTx(ctx, domain.Tenant{AccountID: accountID, BranchID: branchID},
		func(q Querier) error {
			var createErr error
			rfq, createErr = repo.Create(ctx, q, accountID, domain.NewRFQ{
				BranchID: branchID, ChannelID: channelID, RawText: &raw,
			})
			return createErr
		})
	if err != nil {
		t.Fatalf("seed rfq: %v", err)
	}
	t.Cleanup(func() {
		owner := db.CrossAccount()
		mustCleanup(t, owner,
			`DELETE FROM quote_item WHERE version_id IN (
			   SELECT v.id FROM quote_version v JOIN quote c ON c.id = v.quote_id
			   WHERE c.rfq_id = $1)`, rfq.ID)
		mustCleanup(t, owner, `UPDATE quote SET current_version_id = NULL WHERE rfq_id = $1`, rfq.ID)
		mustCleanup(t, owner,
			`DELETE FROM quote_version WHERE quote_id IN (SELECT id FROM quote WHERE rfq_id = $1)`,
			rfq.ID)
		mustCleanup(t, owner,
			`DELETE FROM quote_status_change WHERE quote_id IN (
			   SELECT id FROM quote WHERE rfq_id = $1)`, rfq.ID)
		mustCleanup(t, owner, `DELETE FROM quote WHERE rfq_id = $1`, rfq.ID)
		mustCleanup(t, owner, `DELETE FROM rfq_status_change WHERE rfq_id = $1`, rfq.ID)
		mustCleanup(t, owner, `DELETE FROM rfq WHERE id = $1`, rfq.ID)
	})
	return rfq
}

func TestRFQRepository_TransitionsStayInsideTheAccount(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	accountA := seedAccount(t, db, "RFQ transitions account A")
	accountB := seedAccount(t, db, "RFQ transitions account B")
	branchA := branchOf(t, db, accountA)
	channelA := seedChannel(t, db, accountA, branchA, domain.ChannelTypeWhatsApp, true)
	rfq := seedTextRFQ(t, db, accountA, branchA, channelA)
	repo := NewRFQRepository()

	if rfq.Status != domain.RFQStatusReceived {
		t.Fatalf("Create() status = %q, want RECEIVED", rfq.Status)
	}

	var updated *domain.RFQ
	if err := db.InTenantTx(ctx, domain.Tenant{AccountID: accountA, BranchID: branchA},
		func(q Querier) error {
			if _, appendErr := repo.AppendStatusChange(ctx, q, accountA, rfq.ID,
				&rfq.Status, domain.RFQStatusGenerated, nil); appendErr != nil {
				return appendErr
			}
			var updateErr error
			updated, updateErr = repo.UpdateStatus(ctx, q, accountA, rfq.ID,
				domain.RFQStatusGenerated)
			return updateErr
		}); err != nil {
		t.Fatalf("transition = %v, want no error", err)
	}
	if updated.Status != domain.RFQStatusGenerated {
		t.Errorf("UpdateStatus() status = %q, want GENERATED", updated.Status)
	}

	// Another account cannot move it: row level security is the second net, and the explicit
	// predicate is the first.
	err := db.InTenantTx(ctx, domain.Tenant{AccountID: accountB}, func(q Querier) error {
		_, updateErr := repo.UpdateStatus(ctx, q, accountB, rfq.ID, domain.RFQStatusReceived)
		return updateErr
	})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("UpdateStatus() from another account = %v, want %v", err, domain.ErrNotFound)
	}
	var status string
	if err := db.CrossAccount().QueryRow(ctx, `SELECT status FROM rfq WHERE id = $1`,
		rfq.ID).Scan(&status); err != nil {
		t.Fatalf("read status back: %v", err)
	}
	if status != string(domain.RFQStatusGenerated) {
		t.Errorf("stored status = %q, want it untouched at GENERATED", status)
	}
}

func TestQuoteRepository_CreateItemsKeepsOrderAndScale(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	accountID := seedAccount(t, db, "Quote items scale")
	branchID := branchOf(t, db, accountID)
	channelID := seedChannel(t, db, accountID, branchID, domain.ChannelTypeWhatsApp, true)
	rfq := seedTextRFQ(t, db, accountID, branchID, channelID)
	repo := NewQuoteRepository()
	tenant := domain.Tenant{AccountID: accountID, BranchID: branchID}

	// Five lines, so a reordering shows up rather than being a coin flip on two.
	lines := make([]domain.NewQuoteItem, 5)
	for i := range lines {
		lines[i] = domain.NewQuoteItem{
			RequestedDescription: fmt.Sprintf("material %d", i),
			Quantity:             decimal.RequireFromString(fmt.Sprintf("%d.25", i+1)),
			MatchStatus:          domain.ItemMatchStatusNoMatch,
		}
	}
	// One scored line, at the column's own four decimals.
	scored := decimal.RequireFromString("0.9137")
	lines[2].ConfidenceScore = decimal.NewNullDecimal(scored)
	lines[2].MatchStatus = domain.ItemMatchStatusAmbiguous

	var created []domain.QuoteItem
	if err := db.InTenantTx(ctx, tenant, func(q Querier) error {
		quote, createErr := repo.Create(ctx, q, accountID, domain.NewQuote{
			BranchID: branchID, RFQID: rfq.ID, CurrentStatus: domain.QuoteStatusDraft,
		})
		if createErr != nil {
			return createErr
		}
		version, versionErr := repo.CreateVersion(ctx, q, accountID, domain.NewQuoteVersion{
			QuoteID: quote.ID, VersionNumber: 1, Total: decimal.Zero,
		})
		if versionErr != nil {
			return versionErr
		}
		created, createErr = repo.CreateItems(ctx, q, accountID, version.ID, lines)
		return createErr
	}); err != nil {
		t.Fatalf("create draft = %v, want no error", err)
	}

	if len(created) != len(lines) {
		t.Fatalf("CreateItems() returned %d rows, want %d", len(created), len(lines))
	}
	for i, item := range created {
		want := fmt.Sprintf("material %d", i)
		if item.RequestedDescription != want {
			t.Errorf("row %d description = %q, want %q: the client's order is the line order",
				i, item.RequestedDescription, want)
		}
		wantQuantity := decimal.RequireFromString(fmt.Sprintf("%d.25", i+1))
		if !item.Quantity.Equal(wantQuantity) {
			t.Errorf("row %d quantity = %s, want %s", i, item.Quantity, wantQuantity)
		}
		if item.ProductID != nil {
			t.Errorf("row %d carries product %v, want none: no line named one", i, item.ProductID)
		}
	}
	// Four decimals survive the round trip: rounding here would store a score no decision was
	// taken on.
	if !created[2].ConfidenceScore.Valid || !created[2].ConfidenceScore.Decimal.Equal(scored) {
		t.Errorf("scored row confidence = %v, want %s", created[2].ConfidenceScore, scored)
	}
	if created[0].ConfidenceScore.Valid {
		t.Errorf("unscored row confidence = %v, want null", created[0].ConfidenceScore)
	}
	if created[2].MatchStatus != domain.ItemMatchStatusAmbiguous {
		t.Errorf("scored row status = %q, want AMBIGUOUS", created[2].MatchStatus)
	}
}

func TestQuoteRepository_CreateItemsRefusesAnotherAccountsVersion(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	accountA := seedAccount(t, db, "Quote items account A")
	accountB := seedAccount(t, db, "Quote items account B")
	branchA := branchOf(t, db, accountA)
	branchB := branchOf(t, db, accountB)
	channelA := seedChannel(t, db, accountA, branchA, domain.ChannelTypeWhatsApp, true)
	rfqA := seedTextRFQ(t, db, accountA, branchA, channelA)
	repo := NewQuoteRepository()

	var versionA *domain.QuoteVersion
	if err := db.InTenantTx(ctx, domain.Tenant{AccountID: accountA, BranchID: branchA},
		func(q Querier) error {
			quote, createErr := repo.Create(ctx, q, accountA, domain.NewQuote{
				BranchID: branchA, RFQID: rfqA.ID, CurrentStatus: domain.QuoteStatusDraft,
			})
			if createErr != nil {
				return createErr
			}
			var versionErr error
			versionA, versionErr = repo.CreateVersion(ctx, q, accountA, domain.NewQuoteVersion{
				QuoteID: quote.ID, VersionNumber: 1, Total: decimal.Zero,
			})
			return versionErr
		}); err != nil {
		t.Fatalf("seed account A draft = %v, want no error", err)
	}

	// Account B's own account_id passes row level security, and the foreign key resolves, so
	// nothing but the join stops the line landing on account A's version.
	err := db.InTenantTx(ctx, domain.Tenant{AccountID: accountB, BranchID: branchB},
		func(q Querier) error {
			_, createErr := repo.CreateItems(ctx, q, accountB, versionA.ID,
				[]domain.NewQuoteItem{{
					RequestedDescription: "cemento",
					Quantity:             decimal.RequireFromString("1"),
					MatchStatus:          domain.ItemMatchStatusNoMatch,
				}})
			return createErr
		})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("CreateItems() onto another account's version = %v, want %v", err,
			domain.ErrNotFound)
	}

	var rows int
	if err := db.CrossAccount().QueryRow(ctx,
		`SELECT count(*) FROM quote_item WHERE version_id = $1`, versionA.ID).Scan(&rows); err != nil {
		t.Fatalf("count items: %v", err)
	}
	if rows != 0 {
		t.Errorf("account A's version carries %d foreign lines, want 0", rows)
	}
}

func TestQuoteRepository_CreateRefusesASecondQuoteForOneRFQ(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	accountID := seedAccount(t, db, "One quote per rfq")
	branchID := branchOf(t, db, accountID)
	channelID := seedChannel(t, db, accountID, branchID, domain.ChannelTypeWhatsApp, true)
	rfq := seedTextRFQ(t, db, accountID, branchID, channelID)
	repo := NewQuoteRepository()
	tenant := domain.Tenant{AccountID: accountID, BranchID: branchID}
	newQuote := domain.NewQuote{
		BranchID: branchID, RFQID: rfq.ID, CurrentStatus: domain.QuoteStatusDraft,
	}

	if err := db.InTenantTx(ctx, tenant, func(q Querier) error {
		_, createErr := repo.Create(ctx, q, accountID, newQuote)
		return createErr
	}); err != nil {
		t.Fatalf("Create() = %v, want no error", err)
	}

	// One RFQ, one quote: reopening reactivates the same one, and a duplicate would split the
	// order's history in two.
	err := db.InTenantTx(ctx, tenant, func(q Querier) error {
		_, createErr := repo.Create(ctx, q, accountID, newQuote)
		return createErr
	})
	if !errors.Is(err, domain.ErrConflict) {
		t.Errorf("second Create() = %v, want %v", err, domain.ErrConflict)
	}
}

// seedClient creates a client for one account and removes it with the test.
func seedClient(t *testing.T, db *DB, accountID uuid.UUID) uuid.UUID {
	t.Helper()
	id := uuid.New()
	if _, err := db.CrossAccount().Exec(context.Background(),
		`INSERT INTO client (id, account_id, name) VALUES ($1, $2, $3)`,
		id, accountID, "Cliente "+id.String()); err != nil {
		t.Fatalf("seed client: %v", err)
	}
	t.Cleanup(func() { mustCleanup(t, db.CrossAccount(), `DELETE FROM client WHERE id = $1`, id) })
	return id
}

func TestRFQRepository_CreateRefusesAnotherAccountsClient(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	accountA := seedAccount(t, db, "RFQ client account A")
	accountB := seedAccount(t, db, "RFQ client account B")
	branchA := branchOf(t, db, accountA)
	channelA := seedChannel(t, db, accountA, branchA, domain.ChannelTypeWhatsApp, true)
	foreignClient := seedClient(t, db, accountB)
	ownClient := seedClient(t, db, accountA)
	repo := NewRFQRepository()
	tenantA := domain.Tenant{AccountID: accountA, BranchID: branchA}
	raw := "Necesito cemento"

	// The client is the one reference that arrives from the body. Its foreign key resolves across
	// accounts, and account A's own row-level scope says nothing about who the client belongs to.
	err := db.InTenantTx(ctx, tenantA, func(q Querier) error {
		_, createErr := repo.Create(ctx, q, accountA, domain.NewRFQ{
			BranchID: branchA, ChannelID: channelA, ClientID: &foreignClient, RawText: &raw,
		})
		return createErr
	})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("Create() with another account's client = %v, want %v", err, domain.ErrNotFound)
	}
	var planted int
	if err := db.CrossAccount().QueryRow(ctx,
		`SELECT count(*) FROM rfq WHERE client_id = $1`, foreignClient).Scan(&planted); err != nil {
		t.Fatalf("count rfqs: %v", err)
	}
	if planted != 0 {
		t.Errorf("%d rfqs point at account B's client, want 0", planted)
	}

	// Its own client still works, and so does no client at all — counter sales have none.
	for name, clientID := range map[string]*uuid.UUID{"own client": &ownClient, "no client": nil} {
		t.Run(name, func(t *testing.T) {
			var rfq *domain.RFQ
			if err := db.InTenantTx(ctx, tenantA, func(q Querier) error {
				var createErr error
				rfq, createErr = repo.Create(ctx, q, accountA, domain.NewRFQ{
					BranchID: branchA, ChannelID: channelA, ClientID: clientID, RawText: &raw,
				})
				return createErr
			}); err != nil {
				t.Fatalf("Create() = %v, want no error", err)
			}
			t.Cleanup(func() {
				mustCleanup(t, db.CrossAccount(), `DELETE FROM rfq WHERE id = $1`, rfq.ID)
			})
			if clientID == nil && rfq.ClientID != nil {
				t.Errorf("stored client = %v, want none", rfq.ClientID)
			}
			if clientID != nil && (rfq.ClientID == nil || *rfq.ClientID != *clientID) {
				t.Errorf("stored client = %v, want %v", rfq.ClientID, *clientID)
			}
		})
	}
}
