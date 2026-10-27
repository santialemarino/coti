package repository

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// The partial unique indexes behind "one live invoice per sale" and "one credit note per invoice".
const (
	invoiceActiveQuoteIndex = "uq_invoice_active_quote"
	invoiceCreditNoteIndex  = "uq_invoice_credit_note"
)

// invoiceCredited holds for an invoice row aliased i that an issued credit note cancels.
const invoiceCredited = `EXISTS (SELECT 1 FROM invoice nc WHERE nc.account_id = i.account_id AND nc.credited_invoice_id = i.id
	AND nc.kind = 'CREDIT_NOTE' AND nc.status = 'ISSUED')`

const invoiceColumns = `id, account_id, branch_id, quote_id, quote_version_id, status, invoice_type,
	point_of_sale, number, claimed_number, issued_on, cae, cae_expires_on, issuer_cuit, receiver_name,
	receiver_doc_type, receiver_doc_number, receiver_iva_condition, net_amount, exempt_amount,
	vat_amount, total, currency, vat_breakdown, issues, created_at, updated_at, snapshot, issuer_iva_condition, issuer_profile,
	environment, kind, credited_invoice_id, replaces_invoice_id`

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
		`SELECT a.name, a.legal_name, a.tax_id, a.iva_condition, a.prices_include_vat, a.invoice_profile
		 FROM account a WHERE a.id = $1`, accountID,
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

// GetInvoiceOnPayment reads whether marking a sale paid offers its invoice ticked.
func (r *InvoicingRepository) GetInvoiceOnPayment(ctx context.Context, q Querier, accountID uuid.UUID) (bool, error) {
	var on bool
	err := q.QueryRow(ctx, `SELECT invoice_on_payment FROM account WHERE id = $1`, accountID).Scan(&on)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, domain.ErrNotFound
	}
	return on, err
}

// UpdateInvoiceOnPayment sets whether marking a sale paid offers its invoice ticked.
func (r *InvoicingRepository) UpdateInvoiceOnPayment(ctx context.Context, q Querier, accountID uuid.UUID, on bool) error {
	tag, err := q.Exec(ctx, `UPDATE account SET invoice_on_payment = $2 WHERE id = $1`, accountID, on)
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
		`SELECT b.id, b.name, b.is_active, b.point_of_sale
		 FROM branch b
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
		`SELECT b.point_of_sale FROM branch b WHERE b.account_id = $1 AND b.id = $2`,
		accountID, branchID).Scan(&pos)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return intPtr(pos), nil
}

// UpdateBranchPointsOfSale sets the given branches' points of sale; nil clears one. A branch outside
// the account is ErrNotFound and a number another branch holds is ErrConflict.
func (r *InvoicingRepository) UpdateBranchPointsOfSale(
	ctx context.Context, q Querier, accountID uuid.UUID, points map[uuid.UUID]*int,
) error {
	if len(points) == 0 {
		return nil
	}
	ids := make([]uuid.UUID, 0, len(points))
	values := make([]*int32, 0, len(points))
	for id, point := range points {
		ids = append(ids, id)
		if point == nil {
			values = append(values, nil)
			continue
		}
		value := int32(*point)
		values = append(values, &value)
	}
	// Cleared first: the unique index checks each row as it is written, so two branches swapping
	// numbers in one statement would collide halfway.
	tag, err := q.Exec(ctx,
		`UPDATE branch SET point_of_sale = NULL WHERE account_id = $1 AND id = ANY($2)`, accountID, ids)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != int64(len(ids)) {
		return domain.ErrNotFound
	}
	_, err = q.Exec(ctx,
		`UPDATE branch b SET point_of_sale = u.point_of_sale
		 FROM unnest($2::uuid[], $3::integer[]) AS u(id, point_of_sale)
		 WHERE b.account_id = $1 AND b.id = u.id AND u.point_of_sale IS NOT NULL`,
		accountID, ids, values)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return domain.ErrConflict
	}
	return err
}

// GetClientFiscal loads a client's fiscal identity.
func (r *InvoicingRepository) GetClientFiscal(
	ctx context.Context, q Querier, accountID, clientID uuid.UUID,
) (*domain.ClientFiscal, error) {
	var c domain.ClientFiscal
	err := q.QueryRow(ctx,
		`SELECT id, name, legal_name, tax_id, dni, iva_condition, fiscal_address
		 FROM client WHERE account_id = $1 AND id = $2`, accountID, clientID,
	).Scan(&c.ID, &c.Name, &c.LegalName, &c.TaxID, &c.DNI, &c.IVACondition, &c.Address)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// UpdateClientFiscal replaces a client's legal name, IVA condition, address and DNI and returns the
// fiscal identity. It never writes tax_id: a CUIT comes only from the customer's account.
func (r *InvoicingRepository) UpdateClientFiscal(
	ctx context.Context, q Querier, accountID, clientID uuid.UUID, in domain.ClientFiscalUpdate,
) (*domain.ClientFiscal, error) {
	var c domain.ClientFiscal
	err := q.QueryRow(ctx,
		`UPDATE client SET legal_name = $3, dni = $4, iva_condition = $5, fiscal_address = $6
		 WHERE account_id = $1 AND id = $2
		 RETURNING id, name, legal_name, tax_id, dni, iva_condition, fiscal_address`,
		accountID, clientID, in.LegalName, in.DNI, in.IVACondition, in.Address,
	).Scan(&c.ID, &c.Name, &c.LegalName, &c.TaxID, &c.DNI, &c.IVACondition, &c.Address)
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
		`SELECT environment, tax_id, '', certificate_expires_at, certificate_expires_at FROM arca_setup WHERE account_id = $1 AND certificate <> ''`,
		accountID).Scan(&s.Environment, &s.CUIT, &s.Subject, &s.ExpiresAt, &s.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// GetSealedCredential returns the account's certificate, its still-sealed key and the environment
// they were issued for.
func (r *InvoicingRepository) GetSealedCredential(
	ctx context.Context, q Querier, accountID uuid.UUID,
) (certificatePEM, sealedKey string, env domain.ARCAEnvironment, err error) {
	err = q.QueryRow(ctx,
		`SELECT certificate, sealed_key, environment FROM arca_setup WHERE account_id = $1 AND certificate <> ''`,
		accountID).Scan(&certificatePEM, &sealedKey, &env)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", "", domain.ErrNotFound
	}
	return certificatePEM, sealedKey, env, err
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

// HasPendingInvoice reports whether ARCA still owes the answer for one of the account's invoices.
func (r *InvoicingRepository) HasPendingInvoice(ctx context.Context, q Querier, accountID uuid.UUID) (bool, error) {
	var pending bool
	err := q.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM invoice WHERE account_id = $1 AND status = 'PENDING')`, accountID).Scan(&pending)
	return pending, err
}

// InvoicingEnabled reports whether the account invoices with Coti.
func (r *InvoicingRepository) InvoicingEnabled(ctx context.Context, q Querier, accountID uuid.UUID) (bool, error) {
	var enabled bool
	err := q.QueryRow(ctx, `SELECT invoicing_enabled FROM account WHERE id = $1`, accountID).Scan(&enabled)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, domain.ErrNotFound
	}
	return enabled, err
}

// LockAccountForShare holds the account row until the transaction ends, so a CUIT change, an ARCA
// disconnect or switching invoicing off waits for an invoice being held. It returns whether the
// account invoices with Coti, read under that lock.
func (r *InvoicingRepository) LockAccountForShare(ctx context.Context, q Querier, accountID uuid.UUID) (bool, error) {
	var enabled bool
	err := q.QueryRow(ctx, `SELECT invoicing_enabled FROM account WHERE id = $1 FOR SHARE`, accountID).Scan(&enabled)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, domain.ErrNotFound
	}
	return enabled, err
}

// GetLatestByQuote returns the quote's newest invoice made in env, still pending anywhere, or issued
// in production, nil when it has none. A test invoice never holds the sale in production, and one an
// issued credit note cancelled holds it nowhere.
func (r *InvoicingRepository) GetLatestByQuote(
	ctx context.Context, q Querier, accountID, branchID, quoteID uuid.UUID, env domain.ARCAEnvironment,
) (*domain.Invoice, error) {
	inv, err := scanInvoice(q.QueryRow(ctx,
		`SELECT `+invoiceColumns+` FROM invoice i
		 WHERE account_id = $1 AND quote_id = $2 AND branch_id = $3 AND kind = 'INVOICE'
		   AND (environment = $4 OR status = 'PENDING' OR (status = 'ISSUED' AND environment = 'PRODUCTION'))
		   AND NOT `+invoiceCredited+`
		 ORDER BY created_at DESC, id DESC LIMIT 1`, accountID, quoteID, branchID, env))
	if errors.Is(err, domain.ErrNotFound) {
		return nil, nil
	}
	return inv, err
}

// GetLatestIssuedOutside returns the quote's newest invoice issued in an environment other than env,
// nil when it has none.
func (r *InvoicingRepository) GetLatestIssuedOutside(
	ctx context.Context, q Querier, accountID, branchID, quoteID uuid.UUID, env domain.ARCAEnvironment,
) (*domain.Invoice, error) {
	inv, err := scanInvoice(q.QueryRow(ctx,
		`SELECT `+invoiceColumns+` FROM invoice
		 WHERE account_id = $1 AND quote_id = $2 AND branch_id = $3 AND environment <> $4 AND status = 'ISSUED'
		   AND kind = 'INVOICE'
		 ORDER BY created_at DESC, id DESC LIMIT 1`, accountID, quoteID, branchID, env))
	if errors.Is(err, domain.ErrNotFound) {
		return nil, nil
	}
	return inv, err
}

// GetLatestCreditedByQuote returns the quote's newest invoice in env that an issued credit note
// cancelled, nil when it has none.
func (r *InvoicingRepository) GetLatestCreditedByQuote(
	ctx context.Context, q Querier, accountID, branchID, quoteID uuid.UUID, env domain.ARCAEnvironment,
) (*domain.Invoice, error) {
	inv, err := scanInvoice(q.QueryRow(ctx,
		`SELECT `+invoiceColumns+` FROM invoice i
		 WHERE account_id = $1 AND quote_id = $2 AND branch_id = $3 AND environment = $4 AND kind = 'INVOICE'
		   AND status = 'ISSUED' AND `+invoiceCredited+`
		 ORDER BY created_at DESC, id DESC LIMIT 1`, accountID, quoteID, branchID, env))
	if errors.Is(err, domain.ErrNotFound) {
		return nil, nil
	}
	return inv, err
}

// GetByIDs loads the account's vouchers by id, keyed by id.
func (r *InvoicingRepository) GetByIDs(
	ctx context.Context, q Querier, accountID uuid.UUID, ids []uuid.UUID,
) (map[uuid.UUID]domain.Invoice, error) {
	return collectInvoices(q.Query(ctx,
		`SELECT `+invoiceColumns+` FROM invoice WHERE account_id = $1 AND id = ANY($2)`, accountID, ids))
}

// GetCreditNotesByInvoiceIDs returns the newest credit note of each named invoice, keyed by the
// invoice's id; an invoice with none is absent.
func (r *InvoicingRepository) GetCreditNotesByInvoiceIDs(
	ctx context.Context, q Querier, accountID uuid.UUID, invoiceIDs []uuid.UUID,
) (map[uuid.UUID]domain.Invoice, error) {
	notes, err := collectInvoices(q.Query(ctx,
		`SELECT DISTINCT ON (credited_invoice_id) `+invoiceColumns+` FROM invoice
		 WHERE account_id = $1 AND kind = 'CREDIT_NOTE' AND credited_invoice_id = ANY($2)
		 ORDER BY credited_invoice_id, created_at DESC, id DESC`, accountID, invoiceIDs))
	if err != nil {
		return nil, err
	}
	byInvoice := make(map[uuid.UUID]domain.Invoice, len(notes))
	for _, note := range notes {
		byInvoice[*note.CreditedInvoiceID] = note
	}
	return byInvoice, nil
}

// CreatePending records a voucher about to be asked for: an invoice holds its quote in its
// environment, a credit note holds the invoice it cancels. Returns domain.ErrConflict when the quote
// already has a live invoice there, or the invoice a pending or issued credit note.
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
		   receiver_iva_condition, net_amount, exempt_amount, vat_amount, total, currency, vat_breakdown, snapshot, issuer_iva_condition, issuer_profile,
		   environment, kind, credited_invoice_id, replaces_invoice_id)
		 VALUES ($1, $2, $3, $4, 'PENDING', $5, $6, $7, $8, $9, $10, NULLIF($11, ''), $12, $13, $14,
		   $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25)
		 RETURNING `+invoiceColumns,
		in.AccountID, in.BranchID, in.QuoteID, in.QuoteVersionID, in.Type, in.PointOfSale, in.IssuedOn,
		in.IssuerCUIT, in.Receiver.Name, in.Receiver.DocType, in.Receiver.DocNumber,
		in.Receiver.IVACondition, in.Amounts.Net, in.Amounts.Exempt, in.Amounts.VAT, in.Amounts.Total,
		in.Currency, breakdown, mustInvoiceSnapshot(in.Snapshot), in.IssuerCondition, mustProfile(in.IssuerProfile),
		in.Environment, in.Kind, in.CreditedInvoiceID, in.ReplacesInvoiceID))
	if isUniqueViolation(err, invoiceActiveQuoteIndex) || isUniqueViolation(err, invoiceCreditNoteIndex) ||
		isUniqueViolation(err, "uq_invoice_pending_series") {
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
		&inv.Amounts.Total, &inv.Currency, &breakdown, &issues, &inv.CreatedAt, &inv.UpdatedAt, &snapshot, &inv.IssuerCondition, &profile,
		&inv.Environment, &inv.Kind, &inv.CreditedInvoiceID, &inv.ReplacesInvoiceID)
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

// collectInvoices scans every row of a voucher query, keyed by id.
func collectInvoices(rows pgx.Rows, err error) (map[uuid.UUID]domain.Invoice, error) {
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[uuid.UUID]domain.Invoice)
	for rows.Next() {
		inv, err := scanInvoice(rows)
		if err != nil {
			return nil, err
		}
		result[inv.ID] = *inv
	}
	return result, rows.Err()
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
