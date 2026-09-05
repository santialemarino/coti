package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

const clientColumns = `id, account_id, name, phone, email, created_at, updated_at`

// ClientRepository owns the contact data used for quote delivery.
type ClientRepository struct{}

// NewClientRepository builds a ClientRepository.
func NewClientRepository() *ClientRepository { return &ClientRepository{} }

// GetByID loads one account-scoped client.
func (r *ClientRepository) GetByID(ctx context.Context, q Querier, accountID,
	clientID uuid.UUID) (*domain.Client, error) {
	return scanClient(q.QueryRow(ctx, `SELECT `+clientColumns+`
		FROM client WHERE account_id = $1 AND id = $2`, accountID, clientID))
}

// Create adds a client from contact data supplied by the seller.
func (r *ClientRepository) Create(ctx context.Context, q Querier, accountID uuid.UUID,
	in domain.NewClient) (*domain.Client, error) {
	return scanClient(q.QueryRow(ctx, `INSERT INTO client (account_id, name, phone, email)
		VALUES ($1, $2, $3, $4) RETURNING `+clientColumns,
		accountID, in.Name, in.Phone, in.Email))
}

// UpdateContact enriches an existing account-scoped client.
func (r *ClientRepository) UpdateContact(ctx context.Context, q Querier, accountID,
	clientID uuid.UUID, in domain.ClientContact) (*domain.Client, error) {
	return scanClient(q.QueryRow(ctx, `UPDATE client SET phone = $3, email = COALESCE($4, email)
		WHERE account_id = $1 AND id = $2 RETURNING `+clientColumns,
		accountID, clientID, in.Phone, in.Email))
}

func scanClient(row pgx.Row) (*domain.Client, error) {
	var client domain.Client
	err := row.Scan(&client.ID, &client.AccountID, &client.Name, &client.Phone, &client.Email,
		&client.CreatedAt, &client.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &client, nil
}
