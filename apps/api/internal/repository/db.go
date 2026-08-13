package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	pgxdecimal "github.com/jackc/pgx-shopspring-decimal"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	pgxvector "github.com/pgvector/pgvector-go/pgx"

	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// tenantGUC is the session variable the row level security policies read. It is set with
// is_local = true so it cannot leak to whoever borrows the connection next.
const tenantGUC = "app.current_account_id"

// TenantDB is the restricted, RLS-subject pool on its own. A process that never legitimately
// crosses accounts opens this instead of DB, and then holds no RLS-exempt connection at all —
// the boundary is the type rather than a rule nobody can check.
type TenantDB struct {
	app *pgxpool.Pool
}

// NewTenantDB opens only the restricted pool and verifies it answers before returning.
func NewTenantDB(ctx context.Context, cfg config.DatabaseConfig) (*TenantDB, error) {
	app, err := openPool(ctx, cfg, cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("app pool: %w", err)
	}
	return &TenantDB{app: app}, nil
}

// Close releases the pool.
func (db *TenantDB) Close() {
	db.app.Close()
}

// Ping verifies the pool still answers.
func (db *TenantDB) Ping(ctx context.Context) error {
	if err := db.app.Ping(ctx); err != nil {
		return fmt.Errorf("app pool: %w", err)
	}
	return nil
}

// DB adds the owner pool, which bypasses RLS, to the restricted one every request-scoped query
// runs on. Only a process with a legitimate cross-account job opens it.
type DB struct {
	*TenantDB
	admin *pgxpool.Pool
}

// NewDB opens both pools and verifies each one answers before returning.
func NewDB(ctx context.Context, cfg config.DatabaseConfig) (*DB, error) {
	tenant, err := NewTenantDB(ctx, cfg)
	if err != nil {
		return nil, err
	}
	admin, err := openPool(ctx, cfg, cfg.AdminURL)
	if err != nil {
		tenant.Close()
		return nil, fmt.Errorf("admin pool: %w", err)
	}
	return &DB{TenantDB: tenant, admin: admin}, nil
}

// Close releases both pools.
func (db *DB) Close() {
	db.TenantDB.Close()
	db.admin.Close()
}

// Ping verifies both pools still answer. Used by the readiness probe.
func (db *DB) Ping(ctx context.Context) error {
	if err := db.TenantDB.Ping(ctx); err != nil {
		return err
	}
	if err := db.admin.Ping(ctx); err != nil {
		return fmt.Errorf("admin pool: %w", err)
	}
	return nil
}

// InTenantTx runs fn inside a transaction scoped to the tenant's account, and is the only
// way request-scoped queries reach the database.
//
// It has to be a transaction: the GUC is transaction-scoped, so a bare pool query would
// run outside it, match no policy, and silently read zero rows.
func (db *TenantDB) InTenantTx(ctx context.Context, tenant domain.Tenant, fn func(Querier) error) error {
	if tenant.AccountID == uuid.Nil {
		return domain.ErrNoTenantContext
	}

	tx, err := db.app.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	// A no-op once committed, and a failing rollback leaves nothing actionable.
	defer func() { _ = tx.Rollback(ctx) }()

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

// CrossAccount returns a Querier on the owner pool, which bypasses row level security.
//
// Three callers are legitimate: the follow-up cron, login by email, and resolving a
// quote_send.public_token. Every other use is a cross-tenant data leak.
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

	// Both codecs have to be registered per connection, not once in main: the pool opens
	// connections on its own schedule, replacements for dead ones included. The vector one
	// looks its type up in the database, so it costs a query per connection.
	poolCfg.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		pgxdecimal.Register(conn.TypeMap())
		return pgxvector.RegisterTypes(ctx, conn)
	}

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
