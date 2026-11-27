package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// ListAutomaticByClientIDs calculates tags from the caller's visible quotes in one batch.
func (r *TagRepository) ListAutomaticByClientIDs(
	ctx context.Context, q Querier, tenant domain.Tenant, clientIDs []uuid.UUID,
) (map[uuid.UUID]domain.AutomaticClientTags, error) {
	result := make(map[uuid.UUID]domain.AutomaticClientTags)
	if len(clientIDs) == 0 {
		return result, nil
	}

	rows, err := q.Query(ctx, `
		WITH rules AS (
		  SELECT tag.*, COALESCE(rule_minimum, CASE rule_kind
		            WHEN 'RECURRING' THEN $5::integer ELSE $7::integer END) AS minimum,
		        COALESCE(rule_window_days, CASE rule_kind
		            WHEN 'RECURRING' THEN $6::integer ELSE $8::integer END) AS window_days
		  FROM tag WHERE account_id = $1 AND rule_kind IS NOT NULL
		), activity AS (
		  SELECT quote.client_id, quote.current_status = 'ACCEPTED' AS accepted,
		         CASE WHEN quote.current_status = 'ACCEPTED'
		            THEN COALESCE(change.changed_at, quote.updated_at)
		            ELSE GREATEST(quote.archived_at, CASE WHEN quote.current_status = 'REJECTED'
		                 THEN COALESCE(change.changed_at, quote.updated_at) END)
		         END AS occurred_at
		  FROM quote
		  LEFT JOIN LATERAL (
		    SELECT changed_at FROM quote_status_change
		    WHERE account_id = $1 AND quote_id = quote.id AND new_status = quote.current_status
		      AND new_status IN ('ACCEPTED', 'REJECTED')
		    ORDER BY changed_at DESC LIMIT 1
		  ) change ON EXISTS (SELECT 1 FROM rules WHERE rule_enabled AND window_days > 0)
		  WHERE quote.account_id = $1 AND quote.client_id = ANY($2::uuid[])
		    AND (quote.current_status IN ('ACCEPTED', 'REJECTED') OR quote.archived_at IS NOT NULL)
		    AND ($3::uuid[] IS NULL OR quote.branch_id = ANY($3::uuid[]))
		    AND ($4::uuid IS NULL OR quote.seller_id = $4 OR quote.seller_id IS NULL)
		), qualified AS (
		  SELECT activity.client_id, rules.id AS tag_id
		  FROM activity CROSS JOIN rules
		  WHERE rules.rule_enabled
		  GROUP BY activity.client_id, rules.id, rules.rule_kind, rules.minimum, rules.window_days
		  HAVING (rules.rule_kind = 'RECURRING' OR NOT bool_or(activity.accepted))
		    AND count(*) FILTER (
		      WHERE activity.accepted = (rules.rule_kind = 'RECURRING')
		        AND (rules.window_days = 0 OR
		            extract(epoch FROM (now() - activity.occurred_at)) / 86400 BETWEEN 0 AND rules.window_days)
		    ) >= rules.minimum
		), exclusions AS (
		  SELECT client_id, tag_id FROM client_tag_exclusion
		  WHERE account_id = $1 AND client_id = ANY($2::uuid[])
		)
		SELECT COALESCE(qualified.client_id, exclusions.client_id), rules.id, rules.account_id,
		       rules.name, rules.color, rules.created_at, exclusions.client_id IS NOT NULL
		FROM qualified FULL JOIN exclusions USING (client_id, tag_id)
		JOIN rules ON rules.id = COALESCE(qualified.tag_id, exclusions.tag_id) AND rules.rule_enabled
		ORDER BY rules.name, rules.id`, tenant.AccountID, clientIDs, tenant.BranchFilter(),
		sellerParam(tenant), r.defaults[0].Minimum, r.defaults[0].WindowDays,
		r.defaults[1].Minimum, r.defaults[1].WindowDays)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var clientID uuid.UUID
		var excluded bool
		tag := domain.ClientTag{Automatic: true}
		if err := rows.Scan(&clientID, &tag.ID, &tag.AccountID, &tag.Name, &tag.Color,
			&tag.CreatedAt, &excluded); err != nil {
			return nil, err
		}
		tags := result[clientID]
		if excluded {
			tags.Excluded = append(tags.Excluded, tag)
		} else {
			tags.Applied = append(tags.Applied, tag)
		}
		result[clientID] = tags
	}
	return result, rows.Err()
}

// ListRules returns the account's rules with environment defaults filled in.
func (r *TagRepository) ListRules(
	ctx context.Context, q Querier, accountID uuid.UUID,
) ([]domain.ClientTagRule, error) {
	rows, err := q.Query(ctx, `SELECT rule_kind, rule_enabled, rule_minimum, rule_window_days
		FROM tag WHERE account_id = $1 AND rule_kind IS NOT NULL ORDER BY rule_kind`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	rules := make([]domain.ClientTagRule, 0, len(r.defaults))
	for rows.Next() {
		var rule domain.ClientTagRule
		var minimum, windowDays *int32
		if err := rows.Scan(&rule.Kind, &rule.Enabled, &minimum, &windowDays); err != nil {
			return nil, err
		}
		defaults := r.defaults[0]
		if rule.Kind == domain.ClientTagAbandoner {
			defaults = r.defaults[1]
		}
		rule.Minimum, rule.WindowDays = defaults.Minimum, defaults.WindowDays
		if minimum != nil {
			rule.Minimum = *minimum
		}
		if windowDays != nil {
			rule.WindowDays = *windowDays
		}
		rules = append(rules, rule)
	}
	return rules, rows.Err()
}

// UpdateRule saves one automatic rule within the account.
func (r *TagRepository) UpdateRule(
	ctx context.Context, q Querier, accountID uuid.UUID, rule domain.ClientTagRule,
) error {
	result, err := q.Exec(ctx, `UPDATE tag SET rule_enabled = $3, rule_minimum = $4, rule_window_days = $5
		WHERE account_id = $1 AND rule_kind = $2`, accountID, rule.Kind, rule.Enabled, rule.Minimum, rule.WindowDays)
	if err == nil && result.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return err
}

// SetExclusion toggles an account-owned automatic tag exclusion.
func (r *TagRepository) SetExclusion(
	ctx context.Context, q Querier, accountID, clientID, tagID uuid.UUID, excluded bool,
) error {
	var id uuid.UUID
	err := q.QueryRow(ctx, `SELECT tag.id FROM tag
		JOIN client ON client.account_id = tag.account_id AND client.id = $2
		WHERE tag.account_id = $1 AND tag.id = $3 AND tag.rule_kind IS NOT NULL
		  AND client.merged_into IS NULL
		FOR UPDATE OF client`, accountID, clientID, tagID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrNotFound
	}
	if err != nil {
		return err
	}
	if excluded {
		_, err = q.Exec(ctx, `INSERT INTO client_tag_exclusion (account_id, client_id, tag_id)
			VALUES ($1, $2, $3) ON CONFLICT (account_id, client_id, tag_id) DO NOTHING`, accountID, clientID, id)
	} else {
		_, err = q.Exec(ctx, `DELETE FROM client_tag_exclusion
			WHERE account_id = $1 AND client_id = $2 AND tag_id = $3`, accountID, clientID, id)
	}
	return err
}
