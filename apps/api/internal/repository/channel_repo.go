package repository

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

const channelColumns = `id, account_id, branch_id, type, config, is_active, created_at, updated_at,
	identifier`

// ChannelRepository owns persistence for channel.
type ChannelRepository struct{}

// NewChannelRepository builds a ChannelRepository.
func NewChannelRepository() *ChannelRepository {
	return &ChannelRepository{}
}

// GetActiveByTypeAndIdentifiersCrossAccount resolves configured channels for public webhooks.
func (r *ChannelRepository) GetActiveByTypeAndIdentifiersCrossAccount(
	ctx context.Context, q Querier, typ domain.ChannelType, identifiers []string,
) (map[string]domain.Channel, error) {
	if len(identifiers) == 0 {
		return map[string]domain.Channel{}, nil
	}

	rows, err := q.Query(ctx,
		`SELECT `+channelColumns+`
		 FROM channel
		 WHERE type = $1
		   AND identifier = ANY($2)
		   AND is_active = TRUE`,
		typ, identifiers)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	channels := make(map[string]domain.Channel)
	for rows.Next() {
		channel, err := scanChannel(rows)
		if err != nil {
			return nil, err
		}
		if channel.Identifier != nil {
			channels[*channel.Identifier] = channel
		}
	}
	return channels, rows.Err()
}

// CreateManualEntry opens the branch's manual-entry channel, the one a counter, phone or
// unintegrated-messaging order arrives through. Every branch has exactly one, created with the
// branch, because rfq.channel_id is NOT NULL and a manual order has no other channel to point
// at. The conflict target is the partial index covering the identifier-less case.
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

func scanChannel(row interface {
	Scan(dest ...any) error
}) (domain.Channel, error) {
	var c domain.Channel
	var typ string
	var rawConfig []byte
	if err := row.Scan(&c.ID, &c.AccountID, &c.BranchID, &typ, &rawConfig, &c.IsActive,
		&c.CreatedAt, &c.UpdatedAt, &c.Identifier); err != nil {
		return domain.Channel{}, err
	}
	c.Type = domain.ChannelType(typ)
	if rawConfig != nil {
		c.Config = json.RawMessage(rawConfig)
	}
	return c, nil
}
