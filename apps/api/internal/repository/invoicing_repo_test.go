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
		mustCleanup(t, db.CrossAccount(), `DELETE FROM arca_credential WHERE account_id = $1`, accountID)
	})
	return db, domain.Tenant{AccountID: accountID, BranchID: branchID, Role: domain.UserRoleAdmin},
		quoteID, versionID, productID
}

func pendingInvoice(tenant domain.Tenant, quoteID, versionID uuid.UUID) domain.Invoice {
	return domain.Invoice{
		AccountID: tenant.AccountID, BranchID: tenant.BranchID, QuoteID: quoteID, QuoteVersionID: versionID,
		Type: domain.InvoiceTypeA, PointOfSale: 3, IssuedOn: time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC),
		IssuerCUIT: "30712345678", Currency: "ARS",
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
		return repo.MarkRejected(ctx, q, tenant.AccountID, first.ID,
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
		if issued, err = repo.MarkIssued(ctx, q, tenant.AccountID, pending.ID, domain.InvoiceAuthorization{
			Number: 23, CAE: "74123456789012", CAEExpiresOn: expires, Observations: []string{"10217: ok"},
			RawRequest: []byte("<req/>"), RawResponse: []byte("<res/>"),
		}); err != nil {
			return err
		}
		latest, err = repo.GetLatestByQuote(ctx, q, tenant.AccountID, quoteID)
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
	pos := 7

	if err := db.InTenantTx(ctx, tenant, func(q Querier) error {
		if err := repo.UpdateAccountFiscal(ctx, q, tenant.AccountID, &registered, true); err != nil {
			return err
		}
		return repo.UpdateBranchPointsOfSale(ctx, q, tenant.AccountID, map[uuid.UUID]*int{tenant.BranchID: &pos})
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
		got, err := repo.GetBranchPointOfSale(ctx, q, tenant.AccountID, tenant.BranchID)
		if err != nil {
			return err
		}
		if got == nil || *got != pos {
			t.Errorf("point of sale = %v, want %d", got, pos)
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
		return repo.UpdateBranchPointsOfSale(ctx, q, tenant.AccountID, map[uuid.UUID]*int{uuid.New(): &pos})
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
		if err := repo.ClaimNumber(ctx, q, tenant.AccountID, pending.ID, 42); err != nil {
			return err
		}
		latest, err = repo.GetLatestByQuote(ctx, q, tenant.AccountID, quoteID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if latest.ClaimedNumber == nil || *latest.ClaimedNumber != 42 || latest.Number != nil {
		t.Fatalf("latest = %+v, want 42 claimed and nothing authorized", latest)
	}
}

// The WSAA ticket lives beside the certificate, and a new certificate never inherits the old one's.
func TestInvoicingRepository_TicketIsStoredAndClearedWithANewCertificate(t *testing.T) {
	db, tenant, _, _, _ := invoiceFixture(t)
	ctx := context.Background()
	repo := NewInvoicingRepository()
	status := domain.ARCACredentialStatus{CUIT: "30712345678", Subject: "CN=coti", ExpiresAt: time.Now().AddDate(1, 0, 0)}
	expires := time.Date(2026, 10, 6, 2, 50, 0, 0, time.UTC)

	var stored, afterUpload *domain.SealedARCATicket
	if err := db.InTenantTx(ctx, tenant, func(q Querier) error {
		if _, err := repo.UpsertCredential(ctx, q, tenant.AccountID, "CERT", "KEY", status); err != nil {
			return err
		}
		if err := repo.SaveSealedTicket(ctx, q, tenant.AccountID,
			&domain.SealedARCATicket{Token: "T", Sign: "S", ExpiresAt: expires}); err != nil {
			return err
		}
		var err error
		if stored, err = repo.GetSealedTicket(ctx, q, tenant.AccountID); err != nil {
			return err
		}
		if _, err := repo.UpsertCredential(ctx, q, tenant.AccountID, "CERT2", "KEY2", status); err != nil {
			return err
		}
		afterUpload, err = repo.GetSealedTicket(ctx, q, tenant.AccountID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if stored == nil || stored.Token != "T" || stored.Sign != "S" || !stored.ExpiresAt.Equal(expires) {
		t.Fatalf("stored = %+v, want the ticket back", stored)
	}
	if afterUpload != nil {
		t.Fatalf("after a new certificate = %+v, want no ticket", afterUpload)
	}
}

func TestInvoicingRepository_TwoBranchesCannotShareAPointOfSale(t *testing.T) {
	db, tenant, _, _, _ := invoiceFixture(t)
	ctx := context.Background()
	repo := NewInvoicingRepository()
	other := seedExtraBranch(t, db, tenant.AccountID, "Sucursal Norte")
	pos := 9

	err := db.InTenantTx(ctx, tenant, func(q Querier) error {
		return repo.UpdateBranchPointsOfSale(ctx, q, tenant.AccountID,
			map[uuid.UUID]*int{tenant.BranchID: &pos, other: &pos})
	})
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("err = %v, want ErrConflict", err)
	}
}

// A pending or issued invoice makes the sale invoiced; a refused one does not.
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
		if whilePending, err = quotes.HasLiveInvoice(ctx, q, tenant.AccountID, quoteID); err != nil {
			return err
		}
		if err := invoices.MarkRejected(ctx, q, tenant.AccountID, pending.ID,
			domain.InvoiceRejectedError{Issues: []string{"10013: DocTipo"}}); err != nil {
			return err
		}
		afterRefusal, err = quotes.HasLiveInvoice(ctx, q, tenant.AccountID, quoteID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if !whilePending || afterRefusal {
		t.Fatalf("pending = %v, refused = %v, want true then false", whilePending, afterRefusal)
	}
}

// The check runs at the end of the statement, so one save may swap two branches' numbers.
func TestInvoicingRepository_TwoBranchesCanSwapPointsOfSale(t *testing.T) {
	db, tenant, _, _, _ := invoiceFixture(t)
	ctx := context.Background()
	repo := NewInvoicingRepository()
	other := seedExtraBranch(t, db, tenant.AccountID, "Sucursal Sur")
	three, four := 3, 4

	err := db.InTenantTx(ctx, tenant, func(q Querier) error {
		if err := repo.UpdateBranchPointsOfSale(ctx, q, tenant.AccountID,
			map[uuid.UUID]*int{tenant.BranchID: &three, other: &four}); err != nil {
			return err
		}
		return repo.UpdateBranchPointsOfSale(ctx, q, tenant.AccountID,
			map[uuid.UUID]*int{tenant.BranchID: &four, other: &three})
	})
	if err != nil {
		t.Fatalf("swap = %v, want it accepted", err)
	}
}
