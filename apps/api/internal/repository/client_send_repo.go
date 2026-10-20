package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// FindByTaxID returns exact fiscal-identifier candidates without merging profiles.
func (r *ClientRepository) FindByTaxID(ctx context.Context, q Querier, accountID uuid.UUID, taxID string) ([]domain.Client, error) {
	rows, err := q.Query(ctx, `SELECT `+clientColumns+` FROM client WHERE account_id=$1 AND tax_id=$2 ORDER BY updated_at DESC,id`, accountID, taxID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]domain.Client, 0)
	for rows.Next() {
		client, err := scanClient(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, *client)
	}
	return result, rows.Err()
}

// GetRequestContact reads the original request's declared data within its account and branch.
func (r *ClientRepository) GetRequestContact(ctx context.Context, q Querier, accountID, branchID, rfqID uuid.UUID) (*domain.ClientContactHints, error) {
	var hints domain.ClientContactHints
	err := q.QueryRow(ctx, `SELECT NULLIF(m.payload->'contact'->>'tax_id',''),
 COALESCE(NULLIF(m.payload->'contact'->>'name',''),r.client_label),
 NULLIF(m.payload->'contact'->>'phone',''),NULLIF(m.payload->'contact'->>'email','')
 FROM rfq r LEFT JOIN LATERAL (
 SELECT im.payload FROM inbound_channel_message im
 JOIN channel ch ON ch.account_id=im.account_id AND ch.branch_id=im.branch_id AND ch.id=im.channel_id AND ch.type='WEBAPP'
 WHERE im.account_id=r.account_id AND im.branch_id=r.branch_id AND im.rfq_id=r.id ORDER BY im.created_at,im.id LIMIT 1
 ) m ON TRUE WHERE r.account_id=$1 AND r.branch_id=$2 AND r.id=$3`, accountID, branchID, rfqID).Scan(&hints.TaxID, &hints.Name, &hints.Phone, &hints.Email)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	return &hints, err
}

// SetMissingTaxID fills an unassigned identifier without overwriting fiscal data.
func (r *ClientRepository) SetMissingTaxID(ctx context.Context, q Querier, accountID, clientID uuid.UUID, taxID string) error {
	result, err := q.Exec(ctx, `UPDATE client SET tax_id=$3,updated_at=now() WHERE account_id=$1 AND id=$2 AND merged_into IS NULL AND (tax_id IS NULL OR tax_id=$3)`, accountID, clientID, taxID)
	if isUniqueViolation(err, "uq_client_account_tax_id") {
		return domain.ErrConflict
	}
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return domain.ErrConflict
	}
	return nil
}
