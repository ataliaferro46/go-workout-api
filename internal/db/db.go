// Package db owns the Postgres connection lifecycle and embedded migrations.
//
// Two libraries are involved on purpose:
//   - pgx (pgxpool) is the runtime driver. It is pgx-native, supports the
//     pgxpool connection pool, and exposes pgx's typed Scan helpers, which the
//     workout PostgresRepository uses directly.
//   - goose runs migrations. It is built on database/sql, so we open a
//     transient sql.DB using the pgx stdlib adapter only during Migrate(); the
//     adapter is not used for any runtime queries.
//
// Migrations are embedded into the binary via embed.FS so a deployed container
// has everything it needs to bring an empty database up to schema — no separate
// migration image, no sidecar.
//
// sqlc was considered and intentionally not adopted. It is excellent at scale,
// but it adds a separate code-generation step (and a non-Go binary dependency)
// that breaks the README's promise that the project builds with nothing but a
// Go toolchain. For the current query surface, hand-written SQL in
// PostgresRepository is short enough to read in one sitting.
package db

import (
	"context"
	"database/sql"
	"embed"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib" // database/sql driver for goose
	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// NewPool creates and verifies a pgxpool.Pool against the given DSN. The
// caller owns the pool and must Close() it on shutdown.
//
// The pool is configured via the DSN's query parameters (e.g.
// pool_max_conns=20, pool_min_conns=2). We rely on pgxpool's defaults for
// anything unset, which are reasonable for an HTTP backend.
func NewPool(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse dsn: %w", err)
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}
	// Ping eagerly so a misconfigured DSN fails fast at startup rather than
	// surfacing as a confusing 500 on the first request.
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}
	return pool, nil
}

// Migrate brings the database at dsn up to the latest schema using the
// migrations embedded in this package. It is safe to call repeatedly: goose
// records applied versions in the goose_db_version table and skips them on
// subsequent runs.
//
// We open a dedicated database/sql connection here rather than reuse the
// runtime pgxpool because goose is built on database/sql. The sql.DB is closed
// before this function returns; pgxpool is untouched.
func Migrate(dsn string) error {
	sqlDB, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("open sql db: %w", err)
	}
	defer sqlDB.Close()

	goose.SetBaseFS(migrationsFS)
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("set dialect: %w", err)
	}
	if err := goose.Up(sqlDB, "migrations"); err != nil {
		return fmt.Errorf("goose up: %w", err)
	}
	return nil
}
