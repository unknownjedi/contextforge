package database

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"go.uber.org/zap"
)

// MigrationStatus tracks the status of an individual migration.
type MigrationStatus struct {
	Version string
	Applied bool
}

// EnsureMigrationTable creates the schema_migrations table if it doesn't exist.
func EnsureMigrationTable(ctx context.Context, db *sql.DB) error {
	query := `
	CREATE TABLE IF NOT EXISTS schema_migrations (
		version VARCHAR(255) PRIMARY KEY,
		applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	);`
	_, err := db.ExecContext(ctx, query)
	return err
}

// GetAppliedMigrations returns a map of applied migration versions.
func GetAppliedMigrations(ctx context.Context, db *sql.DB) (map[string]bool, error) {
	rows, err := db.QueryContext(ctx, "SELECT version FROM schema_migrations")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	applied := make(map[string]bool)
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		applied[v] = true
	}
	return applied, rows.Err()
}

// AutoMigrate discovers and runs all pending .up.sql migrations in order.
func AutoMigrate(ctx context.Context, db *sql.DB, dir string, logger *zap.Logger) error {
	if dir == "" {
		dir = "migrations"
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			dir = filepath.Join("..", "migrations")
		}
	}

	if err := EnsureMigrationTable(ctx, db); err != nil {
		return fmt.Errorf("ensuring migration table: %w", err)
	}

	files, err := filepath.Glob(filepath.Join(dir, "*.up.sql"))
	if err != nil {
		return fmt.Errorf("listing up migration files: %w", err)
	}
	sort.Strings(files)

	applied, err := GetAppliedMigrations(ctx, db)
	if err != nil {
		return fmt.Errorf("retrieving applied migrations: %w", err)
	}

	appliedCount := 0
	for _, file := range files {
		base := filepath.Base(file)
		version := strings.TrimSuffix(base, ".up.sql")

		if applied[version] {
			continue
		}

		if logger != nil {
			logger.Info("applying database migration", zap.String("migration", base))
		}

		content, err := os.ReadFile(file)
		if err != nil {
			return fmt.Errorf("reading migration file %s: %w", file, err)
		}

		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("beginning tx for %s: %w", base, err)
		}

		if _, err := tx.ExecContext(ctx, string(content)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("executing migration %s: %w", base, err)
		}

		if _, err := tx.ExecContext(ctx, "INSERT INTO schema_migrations (version) VALUES ($1)", version); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("recording migration %s: %w", base, err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("committing migration %s: %w", base, err)
		}

		appliedCount++
		if logger != nil {
			logger.Info("migration applied successfully", zap.String("migration", base))
		}
	}

	if appliedCount > 0 && logger != nil {
		logger.Info("all pending migrations applied", zap.Int("count", appliedCount))
	}

	return nil
}
