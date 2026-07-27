package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// tenantGUC is the session variable the row level security policies read. Set with
// set_config's is_local = true so it is scoped to the transaction and cannot leak to
// the next request that borrows the same pooled connection.
const tenantGUC = "app.current_account_id"

// DB owns both connection pools.
//
// app is the restricted, RLS-subject role and carries every request-scoped query.
// admin is the owner role, which bypasses RLS; only the follow-up cron and the
// pre-auth lookups that cannot know the account yet may touch it.
type DB struct {
	app   *pgxpool.Pool
	admin *pgxpool.Pool
}

// NewDB opens both pools and verifies each one answers before returning.
func NewDB(ctx context.Context, cfg config.DatabaseConfig) (*DB, error) {
	app, err := openPool(ctx, cfg, cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("app pool: %w", err)
	}
	admin, err := openPool(ctx, cfg, cfg.AdminURL)
	if err != nil {
		app.Close()
		return nil, fmt.Errorf("admin pool: %w", err)
	}
	return &DB{app: app, admin: admin}, nil
}

// Close releases both pools.
func (db *DB) Close() {
	db.app.Close()
	db.admin.Close()
}

// Ping verifies both pools still answer. Used by the readiness probe.
func (db *DB) Ping(ctx context.Context) error {
	if err := db.app.Ping(ctx); err != nil {
		return fmt.Errorf("app pool: %w", err)
	}
	if err := db.admin.Ping(ctx); err != nil {
		return fmt.Errorf("admin pool: %w", err)
	}
	return nil
}

// InTenantTx runs fn inside a transaction scoped to the tenant's account.
//
// This is the only way request-scoped queries reach the database. The account is
// pushed into a transaction-local GUC before fn runs, so every statement inside is
// filtered by the row level security policies. Committing on success and rolling
// back on error or panic is handled here.
//
// It has to be a transaction: the GUC is transaction-scoped, so a bare pool query
// would run outside it, match no policy, and silently read zero rows.
func (db *DB) InTenantTx(ctx context.Context, tenant domain.Tenant, fn func(Querier) error) error {
	if tenant.AccountID == uuid.Nil {
		return domain.ErrNoTenantContext
	}

	tx, err := db.app.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() {
		// No-op once committed; ErrTxClosed is the expected result on the happy path.
		if rbErr := tx.Rollback(ctx); rbErr != nil && !errors.Is(rbErr, pgx.ErrTxClosed) {
			// Nothing actionable left — the transaction is already failing.
			_ = rbErr
		}
	}()

	if _, err := tx.Exec(ctx,
		`SELECT set_config($1, $2, true)`,
		tenantGUC, tenant.AccountID.String(),
	); err != nil {
		return fmt.Errorf("set tenant scope: %w", err)
	}

	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// CrossAccount returns a Querier on the owner pool, which bypasses row level
// security.
//
// Only three callers are legitimate: the follow-up cron, which sweeps every account;
// login by email, which cannot know the account before it finds the user; and
// resolving a quote_send.public_token for the public webapp, which has no session.
// The token flow resolves the account here and then continues through InTenantTx —
// it does not keep using this pool.
//
// Every other use is a cross-tenant data leak.
func (db *DB) CrossAccount() Querier {
	return db.admin
}

// AdminTx begins a transaction on the owner pool for multi-step writes that
// legitimately span accounts, such as the follow-up cron. The caller commits.
func (db *DB) AdminTx(ctx context.Context) (pgx.Tx, error) {
	return db.admin.Begin(ctx)
}

func openPool(ctx context.Context, cfg config.DatabaseConfig, url string) (*pgxpool.Pool, error) {
	poolCfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("parse connection string: %w", err)
	}
	poolCfg.MaxConns = cfg.MaxConns
	poolCfg.MinConns = cfg.MinConns
	poolCfg.MaxConnLifetime = cfg.MaxConnLifetime
	poolCfg.MaxConnIdleTime = cfg.MaxConnIdleTime
	poolCfg.ConnConfig.ConnectTimeout = cfg.ConnectTimeout

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("open pool: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, cfg.ConnectTimeout)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}
	return pool, nil
}
