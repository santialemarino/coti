package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// channelColumns reports whether a configuration exists rather than selecting it: the column holds
// provider credentials, and no read path in the product has a reason to hold one in memory.
const channelColumns = `id, account_id, branch_id, type, is_active, identifier,
	config IS NOT NULL AS is_configured, created_at, updated_at`

const (
	channelIdentifierIndex            = "uq_channel_branch_type_identifier"
	channelNoIdentifierIndex          = "uq_channel_branch_type_no_identifier"
	channelEmailIdentifierIndex       = "uq_channel_active_email_identifier_global"
	channelWhatsAppPhoneNumberIDIndex = "uq_channel_active_whatsapp_phone_number_id"
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

// GetActiveByTypeAndIdentifiersCrossAccount resolves active external inboxes before a tenant is known.
func (r *ChannelRepository) GetActiveByTypeAndIdentifiersCrossAccount(
	ctx context.Context, q Querier, channelType domain.ChannelType, identifiers []string,
) (map[string]domain.Channel, error) {
	if len(identifiers) == 0 {
		return map[string]domain.Channel{}, nil
	}

	switch channelType {
	case domain.ChannelTypeWhatsApp:
		return r.getActiveInboundChannels(ctx, q,
			`SELECT `+channelColumns+`, config ->> 'phone_number_id'
			 FROM channel
			 WHERE type = 'WHATSAPP' AND is_active = TRUE
			   AND config ->> 'phone_number_id' = ANY($1)`,
			identifiers)
	case domain.ChannelTypeEmail:
		return r.getActiveInboundChannels(ctx, q,
			`SELECT `+channelColumns+`, lower(identifier)
			 FROM channel
			 WHERE type = 'EMAIL' AND is_active = TRUE
			   AND lower(identifier) = ANY($1)`,
			identifiers)
	default:
		return nil, fmt.Errorf("%w: %s has no external inbox route", domain.ErrInvalidInput,
			channelType)
	}
}

// ListActiveWhatsAppConfigurationsCrossAccount loads encrypted configurations only to verify a
// provider callback before a tenant is known.
func (r *ChannelRepository) ListActiveWhatsAppConfigurationsCrossAccount(
	ctx context.Context, q Querier,
) ([]domain.ChannelConfiguration, error) {
	rows, err := q.Query(ctx,
		`SELECT id, account_id, branch_id, type, config
		 FROM channel
		 WHERE type = 'WHATSAPP' AND is_active = TRUE AND config IS NOT NULL`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	configs := make([]domain.ChannelConfiguration, 0)
	for rows.Next() {
		var config domain.ChannelConfiguration
		if err := rows.Scan(&config.ChannelID, &config.AccountID, &config.BranchID, &config.Type,
			&config.Config); err != nil {
			return nil, err
		}
		configs = append(configs, config)
	}
	return configs, rows.Err()
}

// GetActiveConfigurationsByIDs loads encrypted active channel settings for one branch in batch.
func (r *ChannelRepository) GetActiveConfigurationsByIDs(
	ctx context.Context, q Querier, accountID, branchID uuid.UUID, channelIDs []uuid.UUID,
) (map[uuid.UUID][]byte, error) {
	if len(channelIDs) == 0 {
		return map[uuid.UUID][]byte{}, nil
	}
	rows, err := q.Query(ctx,
		`SELECT id, config
		 FROM channel
		 WHERE account_id = $1 AND branch_id = $2 AND id = ANY($3)
		   AND is_active = TRUE AND config IS NOT NULL`,
		accountID, branchID, channelIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	configs := make(map[uuid.UUID][]byte, len(channelIDs))
	for rows.Next() {
		var channelID uuid.UUID
		var config []byte
		if err := rows.Scan(&channelID, &config); err != nil {
			return nil, err
		}
		configs[channelID] = config
	}
	return configs, rows.Err()
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

// CreateDefaults opens the channels every branch is born with: manual entry, and the WhatsApp and
// email routes a quote is sent through. email is the branch mailbox, nil when it has none yet.
func (r *ChannelRepository) CreateDefaults(
	ctx context.Context, q Querier, accountID, branchID uuid.UUID, email *string,
) error {
	_, err := q.Exec(ctx,
		`INSERT INTO channel (account_id, branch_id, type)
		 VALUES ($1, $2, 'MANUAL_ENTRY'), ($1, $2, 'WHATSAPP')
		 ON CONFLICT DO NOTHING`,
		accountID, branchID)
	if err != nil {
		return err
	}
	if email == nil {
		_, err = q.Exec(ctx,
			`INSERT INTO channel (account_id, branch_id, type, identifier)
			 VALUES ($1, $2, 'EMAIL', NULL)
			 ON CONFLICT DO NOTHING`,
			accountID, branchID)
	} else {
		_, err = q.Exec(ctx,
			`INSERT INTO channel (account_id, branch_id, type, identifier)
			 VALUES ($1, $2, 'EMAIL', $3)
			 ON CONFLICT (branch_id, type, identifier) DO NOTHING`,
			accountID, branchID, email)
	}
	if isChannelConflict(err) {
		return domain.ErrConflict
	}
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

func (r *ChannelRepository) getActiveInboundChannels(
	ctx context.Context, q Querier, query string, identifiers []string,
) (map[string]domain.Channel, error) {
	rows, err := q.Query(ctx, query, identifiers)
	if err != nil {
		return nil, err
	}
	return scanInboundChannels(rows)
}

func isChannelConflict(err error) bool {
	return isUniqueViolation(err, channelIdentifierIndex) ||
		isUniqueViolation(err, channelNoIdentifierIndex) ||
		isUniqueViolation(err, channelEmailIdentifierIndex) ||
		isUniqueViolation(err, channelWhatsAppPhoneNumberIDIndex)
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

func scanInboundChannels(rows pgx.Rows) (map[string]domain.Channel, error) {
	defer rows.Close()

	channels := make(map[string]domain.Channel)
	for rows.Next() {
		channel, identifier, err := scanInboundChannel(rows)
		if err != nil {
			return nil, err
		}
		if _, exists := channels[identifier]; exists {
			return nil, domain.ErrConflict
		}
		channels[identifier] = channel
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return channels, nil
}

func scanChannel(row channelScanner) (domain.Channel, error) {
	var channel domain.Channel
	err := row.Scan(&channel.ID, &channel.AccountID, &channel.BranchID, &channel.Type,
		&channel.IsActive, &channel.Identifier, &channel.IsConfigured,
		&channel.CreatedAt, &channel.UpdatedAt)
	return channel, err
}

func scanInboundChannel(row channelScanner) (domain.Channel, string, error) {
	var channel domain.Channel
	var identifier string
	err := row.Scan(&channel.ID, &channel.AccountID, &channel.BranchID, &channel.Type,
		&channel.IsActive, &channel.Identifier, &channel.IsConfigured,
		&channel.CreatedAt, &channel.UpdatedAt, &identifier)
	return channel, identifier, err
}
