package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// ChannelRepository owns persistence for channel.
type ChannelRepository struct{}

// NewChannelRepository builds a ChannelRepository.
func NewChannelRepository() *ChannelRepository {
	return &ChannelRepository{}
}

// ListActiveByBranch returns the active intake channels configured for one branch.
func (r *ChannelRepository) ListActiveByBranch(
	ctx context.Context, q Querier, accountID, branchID uuid.UUID,
) ([]domain.Channel, error) {
	rows, err := q.Query(ctx,
		`SELECT id, account_id, branch_id, type, is_active, identifier, created_at, updated_at
		 FROM channel
		 WHERE account_id = $1 AND branch_id = $2 AND is_active = TRUE
		 ORDER BY type, identifier NULLS FIRST, id`,
		accountID, branchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	channels := make([]domain.Channel, 0)
	for rows.Next() {
		channel, scanErr := scanChannel(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		channels = append(channels, channel)
	}
	return channels, rows.Err()
}

// ListActiveByType returns active branch channels of one type.
func (r *ChannelRepository) ListActiveByType(
	ctx context.Context, q Querier, accountID, branchID uuid.UUID, channelType domain.ChannelType,
) ([]domain.Channel, error) {
	rows, err := q.Query(ctx,
		`SELECT id, account_id, branch_id, type, is_active, identifier, created_at, updated_at
		 FROM channel
		 WHERE account_id = $1 AND branch_id = $2 AND type = $3 AND is_active = TRUE
		 ORDER BY identifier NULLS FIRST, id`,
		accountID, branchID, channelType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	channels := make([]domain.Channel, 0)
	for rows.Next() {
		channel, scanErr := scanChannel(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		channels = append(channels, channel)
	}
	return channels, rows.Err()
}

// GetActiveByID returns an active channel in the requested branch.
func (r *ChannelRepository) GetActiveByID(
	ctx context.Context, q Querier, accountID, branchID, channelID uuid.UUID,
) (*domain.Channel, error) {
	channel, err := scanChannel(q.QueryRow(ctx,
		`SELECT id, account_id, branch_id, type, is_active, identifier, created_at, updated_at
		 FROM channel
		 WHERE account_id = $1 AND branch_id = $2 AND id = $3 AND is_active = TRUE`,
		accountID, branchID, channelID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &channel, nil
}

// CreateManualEntry opens the identifier-less manual-entry channel every branch needs.
func (r *ChannelRepository) CreateManualEntry(
	ctx context.Context, q Querier, accountID, branchID uuid.UUID,
) error {
	_, err := q.Exec(ctx,
		`INSERT INTO channel (account_id, branch_id, type)
		 VALUES ($1, $2, 'MANUAL_ENTRY')
		 ON CONFLICT (branch_id, type) WHERE identifier IS NULL DO NOTHING`,
		accountID, branchID)
	return err
}

type channelScanner interface {
	Scan(dest ...any) error
}

func scanChannel(row channelScanner) (domain.Channel, error) {
	var channel domain.Channel
	err := row.Scan(&channel.ID, &channel.AccountID, &channel.BranchID, &channel.Type,
		&channel.IsActive, &channel.Identifier, &channel.CreatedAt, &channel.UpdatedAt)
	return channel, err
}
