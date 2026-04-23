// Package migrator applies embedded SQL migrations in lexicographic order.
// Applied versions are recorded in schema_migrations so re-runs are safe.
package migrator

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"log"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

type Result struct {
	Applied []string `json:"applied"`
	Skipped []string `json:"skipped"`
}

func Apply(ctx context.Context, pool *pgxpool.Pool) (Result, error) {
	if _, err := pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
        version TEXT PRIMARY KEY,
        applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
    )`); err != nil {
		return Result{}, fmt.Errorf("ensure schema_migrations: %w", err)
	}

	applied := map[string]struct{}{}
	rows, err := pool.Query(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return Result{}, fmt.Errorf("load applied: %w", err)
	}
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			rows.Close()
			return Result{}, err
		}
		applied[v] = struct{}{}
	}
	rows.Close()

	files, err := listFiles()
	if err != nil {
		return Result{}, err
	}

	var res Result
	for _, name := range files {
		if _, ok := applied[name]; ok {
			res.Skipped = append(res.Skipped, name)
			continue
		}
		sqlBytes, err := migrationsFS.ReadFile("migrations/" + name)
		if err != nil {
			return res, fmt.Errorf("read %s: %w", name, err)
		}
		log.Printf("applying %s", name)
		tx, err := pool.Begin(ctx)
		if err != nil {
			return res, fmt.Errorf("begin %s: %w", name, err)
		}
		if _, err := tx.Exec(ctx, string(sqlBytes)); err != nil {
			_ = tx.Rollback(ctx)
			return res, fmt.Errorf("apply %s: %w", name, err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO schema_migrations (version) VALUES ($1)`, name); err != nil {
			_ = tx.Rollback(ctx)
			return res, fmt.Errorf("record %s: %w", name, err)
		}
		if err := tx.Commit(ctx); err != nil {
			return res, fmt.Errorf("commit %s: %w", name, err)
		}
		res.Applied = append(res.Applied, name)
	}
	return res, nil
}

func listFiles() ([]string, error) {
	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		return nil, fmt.Errorf("read dir: %w", err)
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)
	return names, nil
}
