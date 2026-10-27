package repository

import (
	"context"

	"github.com/google/uuid"
	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// GetFiscal returns optional fiscal data without granting fiscal-identifier mutations.
func (r *ClientContactRepository) GetFiscal(ctx context.Context, q Querier, accountID, clientID uuid.UUID) (*domain.ClientFiscal, error) {
	var fiscal domain.ClientFiscal
	err := q.QueryRow(ctx, `SELECT legal_name,iva_condition,fiscal_address FROM client WHERE account_id=$1 AND id=$2 AND merged_into IS NULL`, accountID, clientID).Scan(&fiscal.LegalName, &fiscal.IVACondition, &fiscal.Address)
	return &fiscal, err
}

// SaveFiscal changes optional fiscal data without changing the immutable CUIT/CUIL.
func (r *ClientContactRepository) SaveFiscal(ctx context.Context, q Querier, accountID, clientID uuid.UUID, in domain.ClientContactUpdate) error {
	_, err := q.Exec(ctx, `UPDATE client SET legal_name=$3,iva_condition=$4,fiscal_address=$5 WHERE account_id=$1 AND id=$2 AND merged_into IS NULL`, accountID, clientID, in.LegalName, in.IVACondition, in.Address)
	return err
}
