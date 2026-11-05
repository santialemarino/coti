package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// DataExportRepository reads the account's records for a download to another system.
type DataExportRepository struct{}

// NewDataExportRepository builds a DataExportRepository.
func NewDataExportRepository() *DataExportRepository {
	return &DataExportRepository{}
}

// ListQuotes returns the quotes created in the filter's period, each with its current version's lines.
func (r *DataExportRepository) ListQuotes(
	ctx context.Context, q Querier, filter domain.DataExportFilter,
) ([]domain.DataExportQuote, error) {
	rows, err := q.Query(ctx,
		`SELECT q.id, q.number, b.name, q.current_status, q.client_id, c.name, u.name,
		        q.created_at, q.expires_at, q.archived_at, q.paid_at, q.needs_followup,
		        v.version_number, v.currency, items.subtotal::text, v.total::text,
		        qi.id, qi.requested_description, p.code, p.canonical_name, qi.quantity::text,
		        qi.unit, qi.unit_price_snapshot::text, qi.subtotal::text, qi.match_status
		 FROM quote q
		 JOIN branch b ON b.account_id = q.account_id AND b.id = q.branch_id
		 LEFT JOIN client c ON c.account_id = q.account_id AND c.id = q.client_id
		 LEFT JOIN app_user u ON u.account_id = q.account_id AND u.id = q.seller_id
		 LEFT JOIN quote_version v ON v.account_id = q.account_id AND v.id = q.current_version_id
		 LEFT JOIN LATERAL (
		   SELECT sum(subtotal) AS subtotal FROM quote_item
		   WHERE account_id = q.account_id AND version_id = v.id
		 ) items ON TRUE
		 LEFT JOIN quote_item qi ON qi.account_id = q.account_id AND qi.version_id = v.id
		 LEFT JOIN product p ON p.account_id = q.account_id AND p.id = qi.product_id
		 WHERE q.account_id = $1
		   AND ($2::uuid IS NULL OR q.branch_id = $2)
		   AND ($3::timestamptz IS NULL OR q.created_at >= $3)
		   AND ($4::timestamptz IS NULL OR q.created_at < $4)
		 ORDER BY q.number, qi.created_at, qi.id`,
		filter.AccountID, filter.BranchID, filter.StartAt, filter.EndAt)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var quotes []domain.DataExportQuote
	for rows.Next() {
		var quote domain.DataExportQuote
		var itemID *uuid.UUID
		var item domain.DataExportQuoteItem
		var description, quantity *string
		var matchStatus *domain.ItemMatchStatus
		if err := rows.Scan(&quote.ID, &quote.Number, &quote.Branch, &quote.Status, &quote.ClientID,
			&quote.Client, &quote.Seller, &quote.CreatedAt, &quote.ExpiresAt, &quote.ArchivedAt,
			&quote.PaidAt, &quote.NeedsFollowUp, &quote.VersionNumber, &quote.Currency,
			&quote.ItemsSubtotal, &quote.Total, &itemID, &description, &item.ProductCode,
			&item.ProductName, &quantity, &item.Unit, &item.UnitPrice, &item.Subtotal,
			&matchStatus); err != nil {
			return nil, err
		}
		if len(quotes) == 0 || quotes[len(quotes)-1].ID != quote.ID {
			quotes = append(quotes, quote)
		}
		if itemID != nil {
			item.RequestedDescription, item.Quantity, item.MatchStatus = *description, *quantity, *matchStatus
			last := &quotes[len(quotes)-1]
			last.Items = append(last.Items, item)
		}
	}
	return quotes, rows.Err()
}

// ListCatalog returns every product with each branch that carries it, narrowed to the filter's
// branch when one is given; a product no branch carries still appears, without branches.
func (r *DataExportRepository) ListCatalog(
	ctx context.Context, q Querier, filter domain.DataExportFilter,
) ([]domain.DataExportProduct, error) {
	rows, err := q.Query(ctx,
		`SELECT p.id, p.code, p.canonical_name, p.description, p.unit, f.name, s.name, p.vat_rate,
		        p.is_active, b.name, bp.is_active, bp.stock::text, pp.price::text,
		        pp.min_price::text, pp.currency
		 FROM product p
		 LEFT JOIN product_family f ON f.id = p.family_id
		 LEFT JOIN product_subgroup s ON s.id = p.subgroup_id
		 LEFT JOIN branch_product bp ON bp.account_id = p.account_id AND bp.product_id = p.id
		   AND ($2::uuid IS NULL OR bp.branch_id = $2)
		 LEFT JOIN branch b ON b.account_id = p.account_id AND b.id = bp.branch_id
		 LEFT JOIN LATERAL (
		   SELECT price, min_price, currency
		   FROM product_price
		   WHERE account_id = p.account_id AND branch_id = bp.branch_id AND product_id = p.id
		     AND valid_from <= now() AND (valid_to IS NULL OR valid_to > now())
		   ORDER BY valid_from DESC
		   LIMIT 1
		 ) pp ON TRUE
		 WHERE p.account_id = $1
		 ORDER BY p.code NULLS LAST, p.canonical_name, p.id, b.name, b.id`,
		filter.AccountID, filter.BranchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var products []domain.DataExportProduct
	for rows.Next() {
		var product domain.DataExportProduct
		var branch domain.DataExportProductBranch
		var branchName *string
		var branchActive *bool
		if err := rows.Scan(&product.ID, &product.Code, &product.Name, &product.Description,
			&product.Unit, &product.Family, &product.Subgroup, &product.VATRate, &product.IsActive,
			&branchName, &branchActive, &branch.Stock, &branch.Price, &branch.MinPrice,
			&branch.Currency); err != nil {
			return nil, err
		}
		if len(products) == 0 || products[len(products)-1].ID != product.ID {
			products = append(products, product)
		}
		if branchName != nil {
			branch.Branch, branch.IsActive = *branchName, *branchActive
			last := &products[len(products)-1]
			last.Branches = append(last.Branches, branch)
		}
	}
	return products, rows.Err()
}

// ListClients returns every client not merged into another, with its tags and live contacts.
func (r *DataExportRepository) ListClients(
	ctx context.Context, q Querier, filter domain.DataExportFilter,
) ([]domain.DataExportClient, error) {
	rows, err := q.Query(ctx,
		`SELECT c.id, c.name, c.legal_name, c.tax_id, c.dni, c.iva_condition, c.fiscal_address,
		        c.origin_channel, c.notes, c.created_at,
		        coalesce((SELECT array_agg(t.name ORDER BY t.name)
		                  FROM client_tag ct
		                  JOIN tag t ON t.account_id = ct.account_id AND t.id = ct.tag_id
		                  WHERE ct.account_id = c.account_id AND ct.client_id = c.id), '{}'),
		        cc.kind, cc.value, cc.is_main, cc.verified_at IS NOT NULL
		 FROM client c
		 LEFT JOIN client_contact cc ON cc.account_id = c.account_id AND cc.client_id = c.id
		   AND cc.retired_at IS NULL
		 WHERE c.account_id = $1 AND c.merged_into IS NULL
		 ORDER BY c.created_at, c.id, cc.is_main DESC, cc.created_at, cc.id`,
		filter.AccountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var clients []domain.DataExportClient
	for rows.Next() {
		var client domain.DataExportClient
		var kind *domain.ChannelType
		var value *string
		var isMain, verified *bool
		if err := rows.Scan(&client.ID, &client.Name, &client.LegalName, &client.TaxID,
			&client.DNI, &client.IVACondition, &client.FiscalAddress, &client.Origin, &client.Notes,
			&client.CreatedAt, &client.Tags, &kind, &value, &isMain, &verified); err != nil {
			return nil, err
		}
		if len(clients) == 0 || clients[len(clients)-1].ID != client.ID {
			clients = append(clients, client)
		}
		if kind != nil {
			last := &clients[len(clients)-1]
			last.Contacts = append(last.Contacts, domain.DataExportClientContact{
				Kind: *kind, Value: *value, IsMain: *isMain, Verified: *verified,
			})
		}
	}
	return clients, rows.Err()
}

// ListInvoices returns the invoices and credit notes issued on a date in the filter's period.
func (r *DataExportRepository) ListInvoices(
	ctx context.Context, q Querier, filter domain.DataExportFilter,
) ([]domain.DataExportInvoice, error) {
	rows, err := q.Query(ctx,
		`SELECT i.id, i.kind, i.invoice_type, i.point_of_sale, i.number, i.issued_on::text,
		        i.status, i.cae, i.cae_expires_on::text, i.environment, b.name, q.number,
		        i.credited_invoice_id, i.receiver_name, i.receiver_doc_type,
		        i.receiver_doc_number, i.receiver_iva_condition, i.net_amount::text,
		        i.exempt_amount::text, i.vat_amount::text, i.total::text, i.currency
		 FROM invoice i
		 JOIN branch b ON b.account_id = i.account_id AND b.id = i.branch_id
		 JOIN quote q ON q.account_id = i.account_id AND q.id = i.quote_id
		 WHERE i.account_id = $1
		   AND ($2::uuid IS NULL OR i.branch_id = $2)
		   AND ($3::date IS NULL OR i.issued_on >= $3::date)
		   AND ($4::date IS NULL OR i.issued_on <= $4::date)
		 ORDER BY i.issued_on, i.point_of_sale, i.number NULLS LAST, i.created_at, i.id`,
		filter.AccountID, filter.BranchID, filter.FromDate, filter.ToDate)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var invoices []domain.DataExportInvoice
	for rows.Next() {
		var invoice domain.DataExportInvoice
		if err := rows.Scan(&invoice.ID, &invoice.Kind, &invoice.Type, &invoice.PointOfSale,
			&invoice.Number, &invoice.IssuedOn, &invoice.Status, &invoice.CAE, &invoice.CAEExpiresOn,
			&invoice.Environment, &invoice.Branch, &invoice.QuoteNumber, &invoice.CreditedInvoiceID,
			&invoice.ReceiverName, &invoice.ReceiverDocType, &invoice.ReceiverDocNumber,
			&invoice.ReceiverIVACondition, &invoice.NetAmount, &invoice.ExemptAmount,
			&invoice.VATAmount, &invoice.Total, &invoice.Currency); err != nil {
			return nil, err
		}
		invoices = append(invoices, invoice)
	}
	return invoices, rows.Err()
}

// GetBranchName returns the name of one of the account's branches, active or not.
func (r *DataExportRepository) GetBranchName(
	ctx context.Context, q Querier, accountID, branchID uuid.UUID,
) (string, error) {
	var name string
	err := q.QueryRow(ctx, `SELECT name FROM branch WHERE account_id = $1 AND id = $2`,
		accountID, branchID).Scan(&name)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", domain.ErrNotFound
	}
	return name, err
}
