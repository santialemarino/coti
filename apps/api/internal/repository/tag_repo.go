package repository

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

const tagColumns = `id, account_id, name, color, created_at`

var defaultClientTagNames = []string{"Recurrente", "Obra grande"}

// TagRepository owns reusable client labels and their profile links.
type TagRepository struct{}

// NewTagRepository builds a TagRepository.
func NewTagRepository() *TagRepository { return &TagRepository{} }

// List returns every reusable tag in one account.
func (r *TagRepository) List(
	ctx context.Context, q Querier, accountID uuid.UUID,
) ([]domain.ClientTag, error) {
	rows, err := q.Query(ctx, `SELECT `+tagColumns+` FROM tag
		WHERE account_id = $1 ORDER BY lower(name), id`, accountID)
	if err != nil {
		return nil, err
	}
	return scanTags(rows)
}

// ListByClientIDs batch-loads profile tags without a query per client.
func (r *TagRepository) ListByClientIDs(
	ctx context.Context, q Querier, accountID uuid.UUID, clientIDs []uuid.UUID,
) (map[uuid.UUID][]domain.ClientTag, error) {
	result := make(map[uuid.UUID][]domain.ClientTag, len(clientIDs))
	if len(clientIDs) == 0 {
		return result, nil
	}
	rows, err := q.Query(ctx, `SELECT link.client_id, tag.id, tag.account_id, tag.name,
		tag.color, tag.created_at
		FROM client_tag link
		JOIN tag ON tag.account_id = link.account_id AND tag.id = link.tag_id
		WHERE link.account_id = $1 AND link.client_id = ANY($2::uuid[])
		ORDER BY link.client_id, lower(tag.name), tag.id`, accountID, clientIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var clientID uuid.UUID
		var tag domain.ClientTag
		if err := rows.Scan(&clientID, &tag.ID, &tag.AccountID, &tag.Name, &tag.Color,
			&tag.CreatedAt); err != nil {
			return nil, err
		}
		result[clientID] = append(result[clientID], tag)
	}
	return result, rows.Err()
}

// GetByIDs validates and returns account-owned tags in one round trip.
func (r *TagRepository) GetByIDs(
	ctx context.Context, q Querier, accountID uuid.UUID, tagIDs []uuid.UUID,
) ([]domain.ClientTag, error) {
	if len(tagIDs) == 0 {
		return []domain.ClientTag{}, nil
	}
	rows, err := q.Query(ctx, `SELECT `+tagColumns+` FROM tag
		WHERE account_id = $1 AND id = ANY($2::uuid[])
		ORDER BY lower(name), id`, accountID, tagIDs)
	if err != nil {
		return nil, err
	}
	return scanTags(rows)
}

// Create returns the existing account tag when its normalized name is already present.
func (r *TagRepository) Create(
	ctx context.Context, q Querier, accountID uuid.UUID, name string,
) (*domain.ClientTag, error) {
	return scanTag(q.QueryRow(ctx, `INSERT INTO tag (account_id, name)
		VALUES ($1, $2)
		ON CONFLICT (account_id, lower(name)) DO UPDATE SET name = tag.name
		RETURNING `+tagColumns, accountID, name))
}

// CreateDefaults seeds the reusable tags every new account starts with.
func (r *TagRepository) CreateDefaults(
	ctx context.Context, q Querier, accountID uuid.UUID,
) error {
	_, err := q.Exec(ctx, `INSERT INTO tag (account_id, name)
		SELECT $1, name FROM unnest($2::text[]) AS defaults(name)
		ON CONFLICT (account_id, lower(name)) DO NOTHING`, accountID, defaultClientTagNames)
	return err
}

// ReplaceClientTags atomically replaces a profile's selected tags inside the caller's transaction.
func (r *TagRepository) ReplaceClientTags(
	ctx context.Context, q Querier, accountID, clientID uuid.UUID, tagIDs []uuid.UUID,
) error {
	if _, err := q.Exec(ctx, `DELETE FROM client_tag
		WHERE account_id = $1 AND client_id = $2`, accountID, clientID); err != nil {
		return err
	}
	_, err := q.Exec(ctx, `INSERT INTO client_tag (account_id, client_id, tag_id)
		SELECT $1, $2, tag_id FROM unnest($3::uuid[]) AS selected(tag_id)
		ON CONFLICT (client_id, tag_id) DO NOTHING`, accountID, clientID, tagIDs)
	return err
}

func scanTags(rows pgx.Rows) ([]domain.ClientTag, error) {
	defer rows.Close()
	tags := make([]domain.ClientTag, 0)
	for rows.Next() {
		tag, err := scanTag(rows)
		if err != nil {
			return nil, err
		}
		tags = append(tags, *tag)
	}
	return tags, rows.Err()
}

func scanTag(row pgx.Row) (*domain.ClientTag, error) {
	var tag domain.ClientTag
	if err := row.Scan(&tag.ID, &tag.AccountID, &tag.Name, &tag.Color, &tag.CreatedAt); err != nil {
		return nil, err
	}
	return &tag, nil
}
