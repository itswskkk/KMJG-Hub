// Package postgres implements the KMJG Hub Server's Repository interfaces
// against PostgreSQL, per docs/ARCHITECTURE.md "Server Database". Migrations
// are managed explicitly (docs/ARCHITECTURE.md "Database Migrations") using a
// minimal embedded runner appropriate for v1's small, linear schema history.
package postgres

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

// connectAttempts and connectRetryDelay bound how long Open waits for
// PostgreSQL to accept connections, so a `docker compose up` startup race
// between the server and postgres containers resolves itself instead of
// crash-looping the server container.
const (
	connectAttempts   = 10
	connectRetryDelay = 2 * time.Second
)

// Open connects a pooled PostgreSQL client using the given connection
// string (docs/ARCHITECTURE.md config: KMJG_DATABASE_URL), retrying briefly
// if the database is not yet accepting connections.
func Open(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("create postgres pool: %w", err)
	}

	var pingErr error
	for attempt := 1; attempt <= connectAttempts; attempt++ {
		if pingErr = pool.Ping(ctx); pingErr == nil {
			return pool, nil
		}
		if attempt < connectAttempts {
			select {
			case <-ctx.Done():
				pool.Close()
				return nil, ctx.Err()
			case <-time.After(connectRetryDelay):
			}
		}
	}

	pool.Close()
	return nil, fmt.Errorf("ping postgres after %d attempts: %w", connectAttempts, pingErr)
}

// Migrate applies any embedded SQL migrations that have not yet been
// recorded as applied, in filename order, each inside its own transaction.
func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	if _, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)
	`); err != nil {
		return fmt.Errorf("ensure schema_migrations table: %w", err)
	}

	entries, err := fs.ReadDir(migrationFS, "migrations")
	if err != nil {
		return fmt.Errorf("read embedded migrations: %w", err)
	}

	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	for _, name := range names {
		var applied bool
		if err := pool.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = $1)`, name,
		).Scan(&applied); err != nil {
			return fmt.Errorf("check migration %s: %w", name, err)
		}
		if applied {
			continue
		}

		sqlBytes, err := migrationFS.ReadFile("migrations/" + name)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", name, err)
		}

		tx, err := pool.Begin(ctx)
		if err != nil {
			return fmt.Errorf("begin migration %s: %w", name, err)
		}

		if _, err := tx.Exec(ctx, string(sqlBytes)); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("apply migration %s: %w", name, err)
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO schema_migrations (version) VALUES ($1)`, name,
		); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("record migration %s: %w", name, err)
		}

		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit migration %s: %w", name, err)
		}
	}

	return nil
}
