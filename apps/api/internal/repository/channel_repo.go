package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// channelColumns reports whether a configuration exists rather than selecting it: the column holds
// provider credentials, and no read path in the product has a reason to hold one in memory.
const channelColumns = `id, account_id, branch_id, type, is_active, identifier,
	config IS NOT NULL AS is_configured, created_at, updated_at`

const (
	channelIdentifierIndex   = "uq_channel_branch_type_identifier"
	channelNoIdentifierIndex = "uq_channel_branch_type_no_identifier"
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
		`SELECT `+channelColumns+`
		 FROM channel
		 WHERE account_id = $1 AND branch_id = $2 AND is_active = TRUE
		 ORDER BY type, identifier NULLS FIRST, id`,
		accountID, branchID)
	if err != nil {
		return nil, err
	}
	return scanChannels(rows)
}

// ListAllByBranch returns every channel of one branch, closed ones included, so an administrator
// can reopen one.
func (r *ChannelRepository) ListAllByBranch(
	ctx context.Context, q Querier, accountID, branchID uuid.UUID,
) ([]domain.Channel, error) {
	rows, err := q.Query(ctx,
		`SELECT `+channelColumns+`
		 FROM channel
		 WHERE account_id = $1 AND branch_id = $2
		 ORDER BY type, identifier NULLS FIRST, id`,
		accountID, branchID)
	if err != nil {
		return nil, err
	}
	return scanChannels(rows)
}

// ListActiveByType returns active branch channels of one type.
func (r *ChannelRepository) ListActiveByType(
	ctx context.Context, q Querier, accountID, branchID uuid.UUID, channelType domain.ChannelType,
) ([]domain.Channel, error) {
	rows, err := q.Query(ctx,
		`SELECT `+channelColumns+`
		 FROM channel
		 WHERE account_id = $1 AND branch_id = $2 AND type = $3 AND is_active = TRUE
		 ORDER BY identifier NULLS FIRST, id`,
		accountID, branchID, channelType)
	if err != nil {
		return nil, err
	}
	return scanChannels(rows)
}

// GetActiveByID returns an active channel in the requested branch.
func (r *ChannelRepository) GetActiveByID(
	ctx context.Context, q Querier, accountID, branchID, channelID uuid.UUID,
) (*domain.Channel, error) {
	return r.getByID(ctx, q,
		`SELECT `+channelColumns+`
		 FROM channel
		 WHERE account_id = $1 AND branch_id = $2 AND id = $3 AND is_active = TRUE`,
		accountID, branchID, channelID)
}

// GetByID returns one channel of the branch whatever its state, which administering it needs.
func (r *ChannelRepository) GetByID(
	ctx context.Context, q Querier, accountID, branchID, channelID uuid.UUID,
) (*domain.Channel, error) {
	return r.getByID(ctx, q,
		`SELECT `+channelColumns+`
		 FROM channel
		 WHERE account_id = $1 AND branch_id = $2 AND id = $3`,
		accountID, branchID, channelID)
}

// Create opens a channel on the branch. Returns domain.ErrConflict when the branch already holds
// one of that type and identifier, the absent identifier included.
func (r *ChannelRepository) Create(
	ctx context.Context, q Querier, accountID, branchID uuid.UUID, in domain.NewChannel,
) (*domain.Channel, error) {
	channel, err := scanChannel(q.QueryRow(ctx,
		`INSERT INTO channel (account_id, branch_id, type, identifier, config)
		 VALUES ($1, $2, $3, $4, $5::jsonb)
		 RETURNING `+channelColumns,
		accountID, branchID, in.Type, in.Identifier, in.Config))
	if isChannelConflict(err) {
		return nil, domain.ErrConflict
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

// Update replaces the channel's editable fields. A nil config leaves the stored one alone and
// clearConfig removes it, so editing an identifier cannot silently discard a credential.
func (r *ChannelRepository) Update(
	ctx context.Context, q Querier, accountID, branchID, channelID uuid.UUID,
	in domain.ChannelUpdate,
) (*domain.Channel, error) {
	channel, err := scanChannel(q.QueryRow(ctx,
		`UPDATE channel
		 SET identifier = $4,
		     is_active = COALESCE($5, is_active),
		     config = CASE WHEN $6 THEN NULL ELSE COALESCE($7::jsonb, config) END
		 WHERE account_id = $1 AND branch_id = $2 AND id = $3
		 RETURNING `+channelColumns,
		accountID, branchID, channelID, in.Identifier, in.IsActive, in.ClearConfig, in.Config))
	if isChannelConflict(err) {
		return nil, domain.ErrConflict
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &channel, nil
}

// Deactivate closes a channel without removing it, so the orders that arrived through it stay
// explainable.
func (r *ChannelRepository) Deactivate(
	ctx context.Context, q Querier, accountID, branchID, channelID uuid.UUID,
) error {
	tag, err := q.Exec(ctx,
		`UPDATE channel SET is_active = FALSE
		 WHERE account_id = $1 AND branch_id = $2 AND id = $3`,
		accountID, branchID, channelID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *ChannelRepository) getByID(
	ctx context.Context, q Querier, query string, args ...any,
) (*domain.Channel, error) {
	channel, err := scanChannel(q.QueryRow(ctx, query, args...))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &channel, nil
}

func isChannelConflict(err error) bool {
	return isUniqueViolation(err, channelIdentifierIndex) ||
		isUniqueViolation(err, channelNoIdentifierIndex)
}

type channelScanner interface {
	Scan(dest ...any) error
}

func scanChannels(rows pgx.Rows) ([]domain.Channel, error) {
	defer rows.Close()

	channels := make([]domain.Channel, 0)
	for rows.Next() {
		channel, err := scanChannel(rows)
		if err != nil {
			return nil, err
		}
		channels = append(channels, channel)
	}
	return channels, rows.Err()
}

func scanChannel(row channelScanner) (domain.Channel, error) {
	var channel domain.Channel
	err := row.Scan(&channel.ID, &channel.AccountID, &channel.BranchID, &channel.Type,
		&channel.IsActive, &channel.Identifier, &channel.IsConfigured,
		&channel.CreatedAt, &channel.UpdatedAt)
	return channel, err
}
