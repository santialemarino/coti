package repository

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// PublicRFQRepository resolves public intake destinations and reads retained contact snapshots.
type PublicRFQRepository struct{}

// NewPublicRFQRepository builds a PublicRFQRepository.
func NewPublicRFQRepository() *PublicRFQRepository { return &PublicRFQRepository{} }

// GetDestinationCrossAccount resolves an active public channel before establishing tenant scope.
func (r *PublicRFQRepository) GetDestinationCrossAccount(ctx context.Context, q Querier, channelID uuid.UUID) (*domain.PublicRFQDestination, error) {
	var result domain.PublicRFQDestination
	result.Route.ChannelType = domain.ChannelTypeWebApp
	err := q.QueryRow(ctx, `SELECT c.account_id, c.branch_id, c.id, a.name, b.name, a.brand_logo_url
		FROM channel c
		JOIN account a ON a.id = c.account_id AND a.is_active
		JOIN branch b ON b.id = c.branch_id AND b.account_id = c.account_id AND b.is_active
		WHERE c.id = $1 AND c.type = 'WEBAPP' AND c.is_active`, channelID).
		Scan(&result.Route.AccountID, &result.Route.BranchID, &result.Route.ChannelID,
			&result.SupplierName, &result.BranchName, &result.LogoURL)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// FindPublicContactByRFQID reads only the public request's declarations within its tenant and branch.
func (r *PublicRFQRepository) FindPublicContactByRFQID(ctx context.Context, q Querier, accountID, branchID, rfqID uuid.UUID) (*domain.PublicRFQContact, error) {
	var raw []byte
	err := q.QueryRow(ctx, `SELECT m.payload -> 'contact'
		FROM inbound_channel_message m
		JOIN channel c ON c.id = m.channel_id AND c.account_id = m.account_id AND c.branch_id = m.branch_id
		WHERE m.account_id = $1 AND m.branch_id = $2 AND m.rfq_id = $3 AND c.type = 'WEBAPP'
		LIMIT 1`, accountID, branchID, rfqID).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var contact domain.PublicRFQContact
	if err := json.Unmarshal(raw, &contact); err != nil {
		return nil, err
	}
	return &contact, nil
}
