//go:build integration

package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

func invoiceFixture(t *testing.T) (*DB, domain.Tenant, uuid.UUID, uuid.UUID, uuid.UUID) {
	t.Helper()
	db := testDB(t)
	accountID := seedAccount(t, db, "Invoicing")
	branchID := branchOf(t, db, accountID)
	productID := seedProduct(t, db, accountID, "Cemento Portland 50kg")
	priceCleanup(t, db, productID)
	quoteID, versionID, _ := seedQuoteChain(t, db, accountID, branchID, productID)
	t.Cleanup(func() {
		mustCleanup(t, db.CrossAccount(), `DELETE FROM invoice WHERE account_id = $1`, accountID)
		mustCleanup(t, db.CrossAccount(), `DELETE FROM arca_setup WHERE account_id = $1`, accountID)
	})
	return db, domain.Tenant{AccountID: accountID, BranchID: branchID, Role: domain.UserRoleAdmin},
		quoteID, versionID, productID
}

func pendingInvoice(tenant domain.Tenant, quoteID, versionID uuid.UUID) domain.Invoice {
	return domain.Invoice{
		AccountID: tenant.AccountID, BranchID: tenant.BranchID, QuoteID: quoteID, QuoteVersionID: versionID,
		Type: domain.InvoiceTypeA, PointOfSale: 3, IssuedOn: time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC),
		IssuerCondition: domain.IVAConditionRegistered, IssuerCUIT: "30712345678", Currency: "ARS",
		Receiver: domain.InvoiceReceiver{Name: "Obra Norte", DocType: domain.ReceiverDocCUIT,
			DocNumber: "20123456786", IVACondition: domain.IVAConditionRegistered},
		Amounts: domain.InvoiceAmounts{Net: decimal.RequireFromString("1000"), Exempt: decimal.Zero,
			VAT: decimal.RequireFromString("210"), Total: decimal.RequireFromString("1210"),
			ByRate: []domain.VATAmount{{Rate: domain.VATRateTwentyOne,
				Base: decimal.RequireFromString("1000"), Amount: decimal.RequireFromString("210")}}},
	}
}

// One sale holds one live invoice: a second pending one conflicts, and a refusal frees it.
func TestInvoicingRepository_OneLiveInvoicePerSale(t *testing.T) {
	db, tenant, quoteID, versionID, _ := invoiceFixture(t)
	ctx := context.Background()
	repo := NewInvoicingRepository()

	var first *domain.Invoice
	if err := db.InTenantTx(ctx, tenant, func(q Querier) error {
		var err error
		first, err = repo.CreatePending(ctx, q, pendingInvoice(tenant, quoteID, versionID))
		return err
	}); err != nil {
		t.Fatalf("CreatePending() = %v", err)
	}

	err := db.InTenantTx(ctx, tenant, func(q Querier) error {
		_, err := repo.CreatePending(ctx, q, pendingInvoice(tenant, quoteID, versionID))
		return err
	})
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("second CreatePending() = %v, want ErrConflict", err)
	}

	if err := db.InTenantTx(ctx, tenant, func(q Querier) error {
		return repo.MarkRejected(ctx, q, tenant.AccountID, tenant.BranchID, first.ID,
			domain.InvoiceRejectedError{Issues: []string{"10013: DocTipo"}})
	}); err != nil {
		t.Fatalf("MarkRejected() = %v", err)
	}
	if err := db.InTenantTx(ctx, tenant, func(q Querier) error {
		_, err := repo.CreatePending(ctx, q, pendingInvoice(tenant, quoteID, versionID))
		return err
	}); err != nil {
		t.Fatalf("CreatePending() after a rejection = %v, want the version free again", err)
	}
}

func TestInvoicingRepository_IssuedInvoiceRoundTrips(t *testing.T) {
	db, tenant, quoteID, versionID, _ := invoiceFixture(t)
	ctx := context.Background()
	repo := NewInvoicingRepository()

	var issued, latest *domain.Invoice
	expires := time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC)
	if err := db.InTenantTx(ctx, tenant, func(q Querier) error {
		pending, err := repo.CreatePending(ctx, q, pendingInvoice(tenant, quoteID, versionID))
		if err != nil {
			return err
		}
		if issued, err = repo.MarkIssued(ctx, q, tenant.AccountID, tenant.BranchID, pending.ID, domain.InvoiceAuthorization{
			Number: 23, CAE: "74123456789012", CAEExpiresOn: expires, Observations: []string{"10217: ok"},
			RawRequest: []byte("<req/>"), RawResponse: []byte("<res/>"),
		}); err != nil {
			return err
		}
		latest, err = repo.GetLatestByQuote(ctx, q, tenant.AccountID, tenant.BranchID, quoteID)
		return err
	}); err != nil {
		t.Fatalf("issue round trip = %v", err)
	}

	if latest.Status != domain.InvoiceStatusIssued || *latest.Number != 23 || *latest.CAE != "74123456789012" ||
		!latest.CAEExpiresOn.Equal(expires) {
		t.Fatalf("latest = %+v, want the issued invoice", latest)
	}
	if latest.Receiver.DocNumber != "20123456786" || len(latest.Amounts.ByRate) != 1 ||
		!latest.Amounts.ByRate[0].Amount.Equal(decimal.RequireFromString("210")) ||
		len(latest.Issues) != 1 || issued.ID != latest.ID {
		t.Fatalf("latest = %+v, want receiver, breakdown and observations stored", latest)
	}
}

func TestInvoicingRepository_FiscalDataRoundTrips(t *testing.T) {
	db, tenant, _, _, productID := invoiceFixture(t)
	ctx := context.Background()
	repo := NewInvoicingRepository()
	registered := domain.IVAConditionRegistered

	if err := db.InTenantTx(ctx, tenant, func(q Querier) error {
		if err := repo.UpdateAccountFiscal(ctx, q, tenant.AccountID, &registered, true); err != nil {
			return err
		}
		return nil
	}); err != nil {
		t.Fatalf("update fiscal data = %v", err)
	}

	if err := db.InTenantTx(ctx, tenant, func(q Querier) error {
		account, err := repo.GetAccountFiscal(ctx, q, tenant.AccountID)
		if err != nil {
			return err
		}
		if account.IVACondition == nil || *account.IVACondition != registered || !account.PricesIncludeVAT {
			t.Errorf("account = %+v, want registered with prices including IVA", account)
		}
		rates, err := repo.VATRatesByProductIDs(ctx, q, tenant.AccountID, []uuid.UUID{productID})
		if err != nil {
			return err
		}
		if rates[productID] != domain.DefaultVATRate {
			t.Errorf("rate = %q, want the 21%% default", rates[productID])
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	// A branch of another account is not the caller's to number.
	err := db.InTenantTx(ctx, tenant, func(q Querier) error {
		return repo.UpdateBranchPointsOfSale(ctx, q, tenant.AccountID, map[uuid.UUID]*int{uuid.New(): nil})
	})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("foreign branch = %v, want ErrNotFound", err)
	}
}

func TestInvoicingRepository_ClaimedNumberRoundTrips(t *testing.T) {
	db, tenant, quoteID, versionID, _ := invoiceFixture(t)
	ctx := context.Background()
	repo := NewInvoicingRepository()

	var latest *domain.Invoice
	if err := db.InTenantTx(ctx, tenant, func(q Querier) error {
		pending, err := repo.CreatePending(ctx, q, pendingInvoice(tenant, quoteID, versionID))
		if err != nil {
			return err
		}
		if err := repo.ClaimNumber(ctx, q, tenant.AccountID, tenant.BranchID, pending.ID, 42); err != nil {
			return err
		}
		latest, err = repo.GetLatestByQuote(ctx, q, tenant.AccountID, tenant.BranchID, quoteID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if latest.ClaimedNumber == nil || *latest.ClaimedNumber != 42 || latest.Number != nil {
		t.Fatalf("latest = %+v, want 42 claimed and nothing authorized", latest)
	}
}

// The WSAA ticket lives beside the certificate, and a new certificate never inherits the old one's.
func TestQuoteRepository_HasLiveInvoice(t *testing.T) {
	db, tenant, quoteID, versionID, _ := invoiceFixture(t)
	ctx := context.Background()
	invoices, quotes := NewInvoicingRepository(), NewQuoteRepository()

	var whilePending, afterRefusal bool
	if err := db.InTenantTx(ctx, tenant, func(q Querier) error {
		pending, err := invoices.CreatePending(ctx, q, pendingInvoice(tenant, quoteID, versionID))
		if err != nil {
			return err
		}
		if whilePending, err = quotes.HasLiveInvoice(ctx, q, tenant.AccountID, tenant.BranchID, quoteID); err != nil {
			return err
		}
		if err := invoices.MarkRejected(ctx, q, tenant.AccountID, tenant.BranchID, pending.ID,
			domain.InvoiceRejectedError{Issues: []string{"10013: DocTipo"}}); err != nil {
			return err
		}
		afterRefusal, err = quotes.HasLiveInvoice(ctx, q, tenant.AccountID, tenant.BranchID, quoteID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if !whilePending || afterRefusal {
		t.Fatalf("pending = %v, refused = %v, want true then false", whilePending, afterRefusal)
	}
}

// The check runs at the end of the statement, so one save may swap two branches' numbers.

func TestInvoicingRepository_SeriesHoldAndBranchIsolation(t *testing.T) {
	db, tenant, quoteID, versionID, productID := invoiceFixture(t)
	ctx := context.Background()
	repo := NewInvoicingRepository()
	var pending *domain.Invoice
	if err := db.InTenantTx(ctx, tenant, func(q Querier) error {
		var err error
		pending, err = repo.CreatePending(ctx, q, pendingInvoice(tenant, quoteID, versionID))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	anotherQuote, anotherVersion, _ := seedQuoteChain(t, db, tenant.AccountID, tenant.BranchID, productID)
	err := db.InTenantTx(ctx, tenant, func(q Querier) error {
		_, err := repo.CreatePending(ctx, q, pendingInvoice(tenant, anotherQuote, anotherVersion))
		return err
	})
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("series hold = %v", err)
	}
	err = db.InTenantTx(ctx, tenant, func(q Querier) error {
		_, err := repo.GetByID(ctx, q, tenant.AccountID, uuid.New(), pending.ID)
		return err
	})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("foreign branch = %v", err)
	}
	var cutoff time.Time
	if err := db.CrossAccount().QueryRow(ctx, "SELECT clock_timestamp()").Scan(&cutoff); err != nil {
		t.Fatal(err)
	}
	for index, want := range []bool{true, false} {
		var claimed bool
		err := db.InTenantTx(ctx, tenant, func(q Querier) error {
			var err error
			claimed, err = repo.ClaimRecovery(ctx, q, tenant.AccountID, tenant.BranchID, pending.ID, cutoff)
			return err
		})
		if err != nil || claimed != want {
			t.Fatalf("claim %d = %v, %v", index, claimed, err)
		}
	}
}

func TestInvoicingRepository_SnapshotSourceDoesNotRequireRepresentation(t *testing.T) {
	db, tenant, quoteID, versionID, _ := invoiceFixture(t)
	ctx := context.Background()
	repo := NewInvoicingRepository()
	if err := db.InTenantTx(ctx, tenant, func(q Querier) error {
		if _, err := q.Exec(ctx, `UPDATE quote SET current_status='ACCEPTED' WHERE account_id=$1 AND branch_id=$2 AND id=$3`, tenant.AccountID, tenant.BranchID, quoteID); err != nil {
			return err
		}
		if _, err := repo.GetSnapshot(ctx, q, tenant.AccountID, tenant.BranchID, quoteID, versionID); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("representation = %v, want missing", err)
		}
		source, err := repo.GetSnapshotSource(ctx, q, tenant.AccountID, tenant.BranchID, quoteID, versionID)
		if err != nil {
			return err
		}
		if source.Version.ID != versionID || source.Quote.CurrentStatus != domain.QuoteStatusAccepted || len(source.Items) != 1 || source.Items[0].ProductName == nil {
			t.Fatal("accepted version source is incomplete")
		}
		for _, scope := range []domain.Tenant{{AccountID: uuid.New(), BranchID: tenant.BranchID}, {AccountID: tenant.AccountID, BranchID: uuid.New()}} {
			if _, err := repo.GetSnapshotSource(ctx, q, scope.AccountID, scope.BranchID, quoteID, versionID); !errors.Is(err, domain.ErrNotFound) {
				t.Fatalf("foreign scope = %v, want not found", err)
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

// Points of sale are written per branch inside the account: two branches may swap numbers in one
// write, a number another branch keeps is refused, and another account's branch is not found.
func TestInvoicingRepository_UpdateBranchPointsOfSale(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	repo := NewInvoicingRepository()
	accountID := seedAccount(t, db, "Points of sale A")
	other := seedAccount(t, db, "Points of sale B")
	first := branchOf(t, db, accountID)
	foreign := branchOf(t, db, other)
	second := seedExtraBranch(t, db, accountID, "Villa Bosch")
	tenant := domain.Tenant{AccountID: accountID, Role: domain.UserRoleAdmin}
	write := func(points map[uuid.UUID]*int) error {
		return db.InTenantTx(ctx, tenant, func(q Querier) error {
			return repo.UpdateBranchPointsOfSale(ctx, q, accountID, points)
		})
	}
	read := func() map[uuid.UUID]*int {
		result := map[uuid.UUID]*int{}
		if err := db.InTenantTx(ctx, tenant, func(q Querier) error {
			branches, err := repo.ListBranchPointsOfSale(ctx, q, accountID)
			for _, b := range branches {
				result[b.BranchID] = b.PointOfSale
			}
			return err
		}); err != nil {
			t.Fatal(err)
		}
		return result
	}
	one, two := 1, 2

	if err := write(map[uuid.UUID]*int{first: &one, second: &two}); err != nil {
		t.Fatal(err)
	}
	if err := write(map[uuid.UUID]*int{first: &two, second: &one}); err != nil {
		t.Fatalf("swap = %v, want the numbers exchanged", err)
	}
	if got := read(); *got[first] != 2 || *got[second] != 1 {
		t.Fatalf("after swap first = %d, second = %d", *got[first], *got[second])
	}
	if err := write(map[uuid.UUID]*int{first: &one}); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("taken number = %v, want ErrConflict", err)
	}
	if err := write(map[uuid.UUID]*int{first: nil}); err != nil {
		t.Fatal(err)
	}
	if got := read(); got[first] != nil || *got[second] != 1 {
		t.Fatal("clearing one branch touched the other")
	}
	if err := write(map[uuid.UUID]*int{foreign: &two}); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("foreign branch = %v, want ErrNotFound", err)
	}
	var foreignPoint *int32
	if err := db.CrossAccount().QueryRow(ctx, `SELECT point_of_sale FROM branch WHERE id = $1`, foreign).Scan(&foreignPoint); err != nil || foreignPoint != nil {
		t.Fatalf("foreign branch point of sale = %v, %v, want untouched", foreignPoint, err)
	}
}

// The history lists one branch, a set of branches, or the whole account when no set is given.
func TestInvoicingRepository_ListNarrowsToTheBranchSet(t *testing.T) {
	db, tenant, quoteID, versionID, productID := invoiceFixture(t)
	ctx := context.Background()
	repo := NewInvoicingRepository()
	other := seedExtraBranch(t, db, tenant.AccountID, "Villa Bosch")
	otherQuote, otherVersion, _ := seedQuoteChain(t, db, tenant.AccountID, other, productID)
	second := pendingInvoice(domain.Tenant{AccountID: tenant.AccountID, BranchID: other}, otherQuote, otherVersion)
	second.PointOfSale = 4
	// Registered after the quote chains, so it runs before their teardown.
	t.Cleanup(func() {
		mustCleanup(t, db.CrossAccount(), `DELETE FROM invoice WHERE account_id = $1`, tenant.AccountID)
	})
	if err := db.InTenantTx(ctx, tenant, func(q Querier) error {
		if _, err := repo.CreatePending(ctx, q, pendingInvoice(tenant, quoteID, versionID)); err != nil {
			return err
		}
		_, err := repo.CreatePending(ctx, q, second)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name     string
		branches []uuid.UUID
		want     int
	}{
		{"every branch", nil, 2},
		{"one branch", []uuid.UUID{other}, 1},
		{"no branch reachable", []uuid.UUID{}, 0},
	}
	for _, tc := range cases {
		var invoices []domain.Invoice
		var total int
		if err := db.InTenantTx(ctx, tenant, func(q Querier) error {
			var err error
			invoices, total, err = repo.List(ctx, q, tenant.AccountID, tc.branches, 25, 0)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		if total != tc.want || len(invoices) != tc.want {
			t.Fatalf("%s: total %d, listed %d, want %d", tc.name, total, len(invoices), tc.want)
		}
		if tc.name == "one branch" && invoices[0].BranchID != other {
			t.Fatalf("listed branch %s, want %s", invoices[0].BranchID, other)
		}
	}
}

// A pending invoice is what keeps the ARCA connection from going.
func TestInvoicingRepository_HasPendingInvoice(t *testing.T) {
	db, tenant, quoteID, versionID, _ := invoiceFixture(t)
	ctx := context.Background()
	invoices := NewInvoicingRepository()
	pending := func() bool {
		var has bool
		if err := db.InTenantTx(ctx, tenant, func(q Querier) error {
			var err error
			has, err = invoices.HasPendingInvoice(ctx, q, tenant.AccountID)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		return has
	}
	if pending() {
		t.Fatal("an account with no invoices reads as pending")
	}
	var invoice *domain.Invoice
	if err := db.InTenantTx(ctx, tenant, func(q Querier) error {
		var err error
		invoice, err = NewInvoicingRepository().CreatePending(ctx, q, pendingInvoice(tenant, quoteID, versionID))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if !pending() {
		t.Fatal("a pending invoice was not seen")
	}
	if err := db.InTenantTx(ctx, tenant, func(q Querier) error {
		return NewInvoicingRepository().MarkRejected(ctx, q, tenant.AccountID, tenant.BranchID, invoice.ID,
			domain.InvoiceRejectedError{Issues: []string{"rechazada"}})
	}); err != nil {
		t.Fatal(err)
	}
	if pending() {
		t.Fatal("a rejected invoice still reads as pending")
	}
}
