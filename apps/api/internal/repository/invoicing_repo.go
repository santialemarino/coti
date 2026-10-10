package repository

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// invoiceActiveQuoteIndex is the partial unique index behind "one live invoice per sale".
const invoiceActiveQuoteIndex = "uq_invoice_active_quote"

// branchPointOfSaleIndex is the constraint keeping two branches of an account off one point of sale.
const branchPointOfSaleIndex = "uq_branch_point_of_sale"

const invoiceColumns = `id, account_id, branch_id, quote_id, quote_version_id, status, invoice_type,
	point_of_sale, number, claimed_number, issued_on, cae, cae_expires_on, issuer_cuit, receiver_name,
	receiver_doc_type, receiver_doc_number, receiver_iva_condition, net_amount, exempt_amount,
	vat_amount, total, currency, vat_breakdown, issues, created_at, updated_at, snapshot, issuer_iva_condition, issuer_profile`

// vatBreakdownRow is how one rate of an invoice is stored in vat_breakdown.
type vatBreakdownRow struct {
	Rate   domain.VATRate `json:"rate"`
	Base   string         `json:"base"`
	Amount string         `json:"amount"`
}

// InvoicingRepository owns the fiscal data invoices need and the invoices themselves.
type InvoicingRepository struct{}

// NewInvoicingRepository builds an InvoicingRepository.
func NewInvoicingRepository() *InvoicingRepository {
	return &InvoicingRepository{}
}

// --- Fiscal data ---

// GetAccountFiscal loads the account's fiscal identity and how its prices treat IVA.
func (r *InvoicingRepository) GetAccountFiscal(
	ctx context.Context, q Querier, accountID uuid.UUID,
) (*domain.AccountFiscal, error) {
	var a domain.AccountFiscal
	var profile []byte
	err := q.QueryRow(ctx,
		`SELECT a.name, a.legal_name, COALESCE(s.tax_id, a.tax_id), a.iva_condition, a.prices_include_vat, a.invoice_profile
		 FROM account a LEFT JOIN arca_setup s ON s.account_id = a.id WHERE a.id = $1`, accountID,
	).Scan(&a.Name, &a.LegalName, &a.TaxID, &a.IVACondition, &a.PricesIncludeVAT, &profile)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(profile, &a.Profile); err != nil {
		return nil, err
	}
	return &a, nil
}

// UpdateAccountFiscal replaces the account's IVA condition and how its prices treat IVA.
func (r *InvoicingRepository) UpdateAccountFiscal(
	ctx context.Context, q Querier, accountID uuid.UUID, condition *domain.IVACondition, pricesIncludeVAT bool,
) error {
	tag, err := q.Exec(ctx,
		`UPDATE account SET iva_condition = $2, prices_include_vat = $3 WHERE id = $1`,
		accountID, condition, pricesIncludeVAT)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// ListBranchPointsOfSale lists every branch of the account with its point of sale, by name.
func (r *InvoicingRepository) ListBranchPointsOfSale(
	ctx context.Context, q Querier, accountID uuid.UUID,
) ([]domain.BranchPointOfSale, error) {
	rows, err := q.Query(ctx,
		`SELECT b.id, b.name, b.is_active, s.point_of_sale
		 FROM branch b LEFT JOIN branch_arca_setup s ON s.account_id = b.account_id AND s.branch_id = b.id
		 WHERE b.account_id = $1
		 ORDER BY b.is_active DESC, b.name, b.id`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	branches := make([]domain.BranchPointOfSale, 0)
	for rows.Next() {
		var b domain.BranchPointOfSale
		var pos *int32
		if err := rows.Scan(&b.BranchID, &b.BranchName, &b.IsActive, &pos); err != nil {
			return nil, err
		}
		b.PointOfSale = intPtr(pos)
		branches = append(branches, b)
	}
	return branches, rows.Err()
}

// GetBranchPointOfSale returns the branch's point of sale, nil when none is set.
func (r *InvoicingRepository) GetBranchPointOfSale(
	ctx context.Context, q Querier, accountID, branchID uuid.UUID,
) (*int, error) {
	var pos *int32
	err := q.QueryRow(ctx,
		`SELECT s.point_of_sale FROM branch b LEFT JOIN branch_arca_setup s ON s.account_id = b.account_id AND s.branch_id = b.id WHERE b.account_id = $1 AND b.id = $2`,
		accountID, branchID).Scan(&pos)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return intPtr(pos), nil
}

// UpdateBranchPointsOfSale refuses writes outside the verified ARCA setup flow.
func (r *InvoicingRepository) UpdateBranchPointsOfSale(
	ctx context.Context, q Querier, accountID uuid.UUID, points map[uuid.UUID]*int,
) error {
	if len(points) != 0 {
		return domain.ErrInvalidInput
	}
	return nil
}

// GetClientFiscal loads a client's fiscal identity.
func (r *InvoicingRepository) GetClientFiscal(
	ctx context.Context, q Querier, accountID, clientID uuid.UUID,
) (*domain.ClientFiscal, error) {
	var c domain.ClientFiscal
	err := q.QueryRow(ctx,
		`SELECT id, name, legal_name, tax_id, iva_condition, fiscal_address
		 FROM client WHERE account_id = $1 AND id = $2`, accountID, clientID,
	).Scan(&c.ID, &c.Name, &c.LegalName, &c.TaxID, &c.IVACondition, &c.Address)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// UpdateClientFiscal replaces a client's fiscal identity and returns it.
func (r *InvoicingRepository) UpdateClientFiscal(
	ctx context.Context, q Querier, accountID, clientID uuid.UUID, in domain.ClientFiscalUpdate,
) (*domain.ClientFiscal, error) {
	var c domain.ClientFiscal
	err := q.QueryRow(ctx,
		`UPDATE client SET legal_name = $3, tax_id = $4, iva_condition = $5, fiscal_address = $6
		 WHERE account_id = $1 AND id = $2
		 RETURNING id, name, legal_name, tax_id, iva_condition, fiscal_address`,
		accountID, clientID, in.LegalName, in.TaxID, in.IVACondition, in.Address,
	).Scan(&c.ID, &c.Name, &c.LegalName, &c.TaxID, &c.IVACondition, &c.Address)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// VATRatesByProductIDs returns the IVA rate of each named product.
func (r *InvoicingRepository) VATRatesByProductIDs(
	ctx context.Context, q Querier, accountID uuid.UUID, ids []uuid.UUID,
) (map[uuid.UUID]domain.VATRate, error) {
	rates := make(map[uuid.UUID]domain.VATRate, len(ids))
	if len(ids) == 0 {
		return rates, nil
	}
	rows, err := q.Query(ctx,
		`SELECT id, vat_rate FROM product WHERE account_id = $1 AND id = ANY($2)`, accountID, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id uuid.UUID
		var rate domain.VATRate
		if err := rows.Scan(&id, &rate); err != nil {
			return nil, err
		}
		rates[id] = rate
	}
	return rates, rows.Err()
}

// --- ARCA credentials ---

// GetCredentialStatus describes the account's certificate, nil when none was uploaded.
func (r *InvoicingRepository) GetCredentialStatus(
	ctx context.Context, q Querier, accountID uuid.UUID,
) (*domain.ARCACredentialStatus, error) {
	var s domain.ARCACredentialStatus
	err := q.QueryRow(ctx,
		`SELECT tax_id, '', certificate_expires_at, certificate_expires_at FROM arca_setup WHERE account_id = $1 AND certificate <> ''`,
		accountID).Scan(&s.CUIT, &s.Subject, &s.ExpiresAt, &s.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// GetSealedCredential returns the account's certificate and its still-sealed key.
func (r *InvoicingRepository) GetSealedCredential(
	ctx context.Context, q Querier, accountID uuid.UUID,
) (certificatePEM, sealedKey string, err error) {
	err = q.QueryRow(ctx,
		`SELECT certificate, sealed_key FROM arca_setup WHERE account_id = $1 AND certificate <> ''`,
		accountID).Scan(&certificatePEM, &sealedKey)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", domain.ErrNotFound
	}
	return certificatePEM, sealedKey, err
}

// GetSealedTicket returns the account's stored WSAA ticket, still sealed; nil when it has none.
func (r *InvoicingRepository) GetSealedTicket(
	ctx context.Context, q Querier, accountID uuid.UUID,
) (*domain.SealedARCATicket, error) {
	var sealed string
	err := q.QueryRow(ctx, `SELECT sealed_ticket FROM arca_setup WHERE account_id = $1`, accountID).Scan(&sealed)
	if errors.Is(err, pgx.ErrNoRows) || sealed == "" {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &domain.SealedARCATicket{Token: sealed}, nil
}

// SaveSealedTicket stores the account's WSAA ticket; nil clears it.
func (r *InvoicingRepository) SaveSealedTicket(
	ctx context.Context, q Querier, accountID uuid.UUID, ticket *domain.SealedARCATicket,
) error {
	sealed := ""
	if ticket != nil {
		sealed = ticket.Token
	}
	_, err := q.Exec(ctx, `UPDATE arca_setup SET sealed_ticket = $2 WHERE account_id = $1`, accountID, sealed)
	return err
}

// --- Invoices ---

// GetLatestByQuote returns the quote's newest invoice in any status, nil when it has none.
func (r *InvoicingRepository) GetLatestByQuote(
	ctx context.Context, q Querier, accountID, branchID, quoteID uuid.UUID,
) (*domain.Invoice, error) {
	inv, err := scanInvoice(q.QueryRow(ctx,
		`SELECT `+invoiceColumns+` FROM invoice
		 WHERE account_id = $1 AND quote_id = $2 AND branch_id = $3
		 ORDER BY created_at DESC, id DESC LIMIT 1`, accountID, quoteID, branchID))
	if errors.Is(err, domain.ErrNotFound) {
		return nil, nil
	}
	return inv, err
}

// CreatePending records an invoice about to be asked for, holding its quote version. Returns
// domain.ErrConflict when the version already has a pending or issued invoice.
func (r *InvoicingRepository) CreatePending(
	ctx context.Context, q Querier, in domain.Invoice,
) (*domain.Invoice, error) {
	breakdown, err := json.Marshal(toBreakdownRows(in.Amounts.ByRate))
	if err != nil {
		return nil, err
	}
	inv, err := scanInvoice(q.QueryRow(ctx,
		`INSERT INTO invoice (account_id, branch_id, quote_id, quote_version_id, status, invoice_type,
		   point_of_sale, issued_on, issuer_cuit, receiver_name, receiver_doc_type, receiver_doc_number,
		   receiver_iva_condition, net_amount, exempt_amount, vat_amount, total, currency, vat_breakdown, snapshot, issuer_iva_condition, issuer_profile)
		 VALUES ($1, $2, $3, $4, 'PENDING', $5, $6, $7, $8, $9, $10, NULLIF($11, ''), $12, $13, $14,
		   $15, $16, $17, $18, $19, $20, $21)
		 RETURNING `+invoiceColumns,
		in.AccountID, in.BranchID, in.QuoteID, in.QuoteVersionID, in.Type, in.PointOfSale, in.IssuedOn,
		in.IssuerCUIT, in.Receiver.Name, in.Receiver.DocType, in.Receiver.DocNumber,
		in.Receiver.IVACondition, in.Amounts.Net, in.Amounts.Exempt, in.Amounts.VAT, in.Amounts.Total,
		in.Currency, breakdown, mustInvoiceSnapshot(in.Snapshot), in.IssuerCondition, mustProfile(in.IssuerProfile)))
	if isUniqueViolation(err, invoiceActiveQuoteIndex) || isUniqueViolation(err, "uq_invoice_pending_series") {
		return nil, domain.ErrConflict
	}
	return inv, err
}

// ClaimNumber records the number a pending invoice is about to be requested under.
func (r *InvoicingRepository) ClaimNumber(ctx context.Context, q Querier, accountID, branchID, id uuid.UUID, number int64) error {
	tag, err := q.Exec(ctx,
		`UPDATE invoice SET claimed_number = $3 WHERE account_id = $1 AND id = $2 AND branch_id = $4 AND status = 'PENDING'`,
		accountID, id, number, branchID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return domain.ErrNotFound
	}
	return nil
}

// MarkIssued records ARCA's authorization on a pending invoice.
func (r *InvoicingRepository) MarkIssued(
	ctx context.Context, q Querier, accountID, branchID, id uuid.UUID, auth domain.InvoiceAuthorization,
) (*domain.Invoice, error) {
	issues, err := json.Marshal(nonNilStrings(auth.Observations))
	if err != nil {
		return nil, err
	}
	return scanInvoice(q.QueryRow(ctx,
		`UPDATE invoice
		 SET status = 'ISSUED', number = $3, cae = $4, cae_expires_on = $5, issues = $6,
		     arca_request = $7, arca_response = $8
		 WHERE account_id = $1 AND id = $2 AND branch_id = $9 AND status = 'PENDING'
		 RETURNING `+invoiceColumns,
		accountID, id, auth.Number, auth.CAE, auth.CAEExpiresOn, issues,
		nullText(auth.RawRequest), nullText(auth.RawResponse), branchID))
}

// MarkRejected closes a pending invoice ARCA refused, which frees its quote version for another try.
func (r *InvoicingRepository) MarkRejected(
	ctx context.Context, q Querier, accountID, branchID, id uuid.UUID, rejection domain.InvoiceRejectedError,
) error {
	issues, err := json.Marshal(nonNilStrings(rejection.Issues))
	if err != nil {
		return err
	}
	_, err = q.Exec(ctx,
		`UPDATE invoice SET status = 'REJECTED', issues = $3, arca_request = $4, arca_response = $5
		 WHERE account_id = $1 AND id = $2 AND branch_id = $6 AND status = 'PENDING'`,
		accountID, id, issues, nullText(rejection.RawRequest), nullText(rejection.RawResponse), branchID)
	return err
}

// ReleasePending drops the hold of a pending invoice whose request never reached a verdict, so
// the seller can try again. The row stays, as REJECTED, with the reason.
func (r *InvoicingRepository) ReleasePending(
	ctx context.Context, q Querier, accountID, branchID, id uuid.UUID, reason string,
) error {
	issues, err := json.Marshal([]string{reason})
	if err != nil {
		return err
	}
	_, err = q.Exec(ctx,
		`UPDATE invoice SET status = 'REJECTED', issues = $3
		 WHERE account_id = $1 AND id = $2 AND branch_id = $4 AND status = 'PENDING'`, accountID, id, issues, branchID)
	return err
}

func scanInvoice(row pgx.Row) (*domain.Invoice, error) {
	var inv domain.Invoice
	var number, claimed *int64
	var docNumber *string
	var breakdown, issues, snapshot, profile []byte
	var issuedOn time.Time
	err := row.Scan(&inv.ID, &inv.AccountID, &inv.BranchID, &inv.QuoteID, &inv.QuoteVersionID,
		&inv.Status, &inv.Type, &inv.PointOfSale, &number, &claimed, &issuedOn, &inv.CAE, &inv.CAEExpiresOn,
		&inv.IssuerCUIT, &inv.Receiver.Name, &inv.Receiver.DocType, &docNumber,
		&inv.Receiver.IVACondition, &inv.Amounts.Net, &inv.Amounts.Exempt, &inv.Amounts.VAT,
		&inv.Amounts.Total, &inv.Currency, &breakdown, &issues, &inv.CreatedAt, &inv.UpdatedAt, &snapshot, &inv.IssuerCondition, &profile)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(snapshot, &inv.Snapshot); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(profile, &inv.IssuerProfile); err != nil {
		return nil, err
	}
	inv.Receiver.Address = firstInvoiceAddress(inv.Snapshot.Customer.Address)
	inv.Number = number
	inv.ClaimedNumber = claimed
	inv.IssuedOn = issuedOn
	if docNumber != nil {
		inv.Receiver.DocNumber = *docNumber
	}
	var rows []vatBreakdownRow
	if err := json.Unmarshal(breakdown, &rows); err != nil {
		return nil, err
	}
	for _, row := range rows {
		base, err := decimal.NewFromString(row.Base)
		if err != nil {
			return nil, err
		}
		amount, err := decimal.NewFromString(row.Amount)
		if err != nil {
			return nil, err
		}
		inv.Amounts.ByRate = append(inv.Amounts.ByRate, domain.VATAmount{Rate: row.Rate, Base: base, Amount: amount})
	}
	if err := json.Unmarshal(issues, &inv.Issues); err != nil {
		return nil, err
	}
	return &inv, nil
}

func toBreakdownRows(amounts []domain.VATAmount) []vatBreakdownRow {
	rows := make([]vatBreakdownRow, 0, len(amounts))
	for _, a := range amounts {
		rows = append(rows, vatBreakdownRow{Rate: a.Rate, Base: a.Base.StringFixed(2), Amount: a.Amount.StringFixed(2)})
	}
	return rows
}

func intPtr(v *int32) *int {
	if v == nil {
		return nil
	}
	i := int(*v)
	return &i
}

func nullText(b []byte) *string {
	if len(b) == 0 {
		return nil
	}
	s := string(b)
	return &s
}

func nonNilStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

// SaveProfile stores the account fiscal identity used by future invoices.
func (r *InvoicingRepository) SaveProfile(ctx context.Context, q Querier, accountID uuid.UUID, profile domain.InvoiceIssuerProfile) error {
	_, err := q.Exec(ctx, `UPDATE account SET invoice_profile = $2 WHERE id = $1`, accountID, mustProfile(profile))
	return err
}
func mustProfile(profile domain.InvoiceIssuerProfile) []byte {
	raw, _ := json.Marshal(profile)
	return raw
}

func firstInvoiceAddress(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
