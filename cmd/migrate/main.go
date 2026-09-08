package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/your-org/contextforge/internal/config"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	command := os.Args[1]

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	db, err := sql.Open("pgx", cfg.Database.URL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to open database: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	ctx := context.Background()
	if err := db.PingContext(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "failed to ping database: %v\n", err)
		os.Exit(1)
	}

	migrationsDir := "migrations"
	if _, err := os.Stat(migrationsDir); os.IsNotExist(err) {
		migrationsDir = filepath.Join("..", "migrations")
	}

	if err := ensureMigrationTable(ctx, db); err != nil {
		fmt.Fprintf(os.Stderr, "failed to ensure schema_migrations table: %v\n", err)
		os.Exit(1)
	}

	switch command {
	case "up":
		if err := runUp(ctx, db, migrationsDir); err != nil {
			fmt.Fprintf(os.Stderr, "migrate up failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("All migrations applied successfully.")
	case "down":
		if err := runDown(ctx, db, migrationsDir); err != nil {
			fmt.Fprintf(os.Stderr, "migrate down failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Latest migration rolled back successfully.")
	case "reset":
		if err := runReset(ctx, db, migrationsDir); err != nil {
			fmt.Fprintf(os.Stderr, "migrate reset failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Database reset and migrations re-applied successfully.")
	case "status":
		if err := printStatus(ctx, db, migrationsDir); err != nil {
			fmt.Fprintf(os.Stderr, "migrate status failed: %v\n", err)
			os.Exit(1)
		}
	default:
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("Usage: go run cmd/migrate/main.go [up|down|reset|status]")
}

func ensureMigrationTable(ctx context.Context, db *sql.DB) error {
	query := `
	CREATE TABLE IF NOT EXISTS schema_migrations (
		version VARCHAR(255) PRIMARY KEY,
		applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	);`
	_, err := db.ExecContext(ctx, query)
	return err
}

func getAppliedMigrations(ctx context.Context, db *sql.DB) (map[string]bool, error) {
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

func runUp(ctx context.Context, db *sql.DB, dir string) error {
	files, err := filepath.Glob(filepath.Join(dir, "*.up.sql"))
	if err != nil {
		return fmt.Errorf("listing up migration files: %w", err)
	}
	sort.Strings(files)

	applied, err := getAppliedMigrations(ctx, db)
	if err != nil {
		return fmt.Errorf("retrieving applied migrations: %w", err)
	}

	for _, file := range files {
		base := filepath.Base(file)
		version := strings.TrimSuffix(base, ".up.sql")

		if applied[version] {
			continue
		}

		fmt.Printf("Applying migration %s...\n", base)
		content, err := os.ReadFile(file)
		if err != nil {
			return fmt.Errorf("reading file %s: %w", file, err)
		}

		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("beginning tx: %w", err)
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
		fmt.Printf("Applied migration %s\n", base)
	}
	return nil
}

func runDown(ctx context.Context, db *sql.DB, dir string) error {
	applied, err := getAppliedMigrations(ctx, db)
	if err != nil {
		return fmt.Errorf("retrieving applied migrations: %w", err)
	}

	files, err := filepath.Glob(filepath.Join(dir, "*.down.sql"))
	if err != nil {
		return fmt.Errorf("listing down migration files: %w", err)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(files)))

	for _, file := range files {
		base := filepath.Base(file)
		version := strings.TrimSuffix(base, ".down.sql")

		if !applied[version] {
			continue
		}

		fmt.Printf("Reverting migration %s...\n", base)
		content, err := os.ReadFile(file)
		if err != nil {
			return fmt.Errorf("reading file %s: %w", file, err)
		}

		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("beginning tx: %w", err)
		}

		if _, err := tx.ExecContext(ctx, string(content)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("executing down migration %s: %w", base, err)
		}

		if _, err := tx.ExecContext(ctx, "DELETE FROM schema_migrations WHERE version = $1", version); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("removing migration record %s: %w", base, err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("committing revert %s: %w", base, err)
		}
		fmt.Printf("Reverted migration %s\n", base)
		return nil
	}

	fmt.Println("No applied migrations found to revert.")
	return nil
}

func runReset(ctx context.Context, db *sql.DB, dir string) error {
	for {
		applied, err := getAppliedMigrations(ctx, db)
		if err != nil {
			return err
		}
		if len(applied) == 0 {
			break
		}
		if err := runDown(ctx, db, dir); err != nil {
			return err
		}
	}
	return runUp(ctx, db, dir)
}

func printStatus(ctx context.Context, db *sql.DB, dir string) error {
	applied, err := getAppliedMigrations(ctx, db)
	if err != nil {
		return err
	}

	files, err := filepath.Glob(filepath.Join(dir, "*.up.sql"))
	if err != nil {
		return err
	}
	sort.Strings(files)

	fmt.Println("Migration Status:")
	for _, file := range files {
		base := filepath.Base(file)
		version := strings.TrimSuffix(base, ".up.sql")
		status := "[PENDING]"
		if applied[version] {
			status = "[APPLIED]"
		}
		fmt.Printf("  %s  %s\n", status, version)
	}
	return nil
}
