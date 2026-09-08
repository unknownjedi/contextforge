package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	entsql "entgo.io/ent/dialect/sql"
	_ "github.com/jackc/pgx/v5/stdlib"
	"go.uber.org/zap"

	"github.com/your-org/contextforge/internal/config"
	"github.com/your-org/contextforge/internal/ent"
)

// Database wraps the Ent client and underlying database connection pool.
type Database struct {
	EntClient *ent.Client
	SQLDB     *sql.DB
	logger    *zap.Logger
}

// New initializes the PostgreSQL connection pool and constructs the Ent client.
func New(cfg *config.DatabaseConfig, log *zap.Logger) (*Database, error) {
	if cfg.URL == "" {
		return nil, fmt.Errorf("database URL cannot be empty")
	}

	db, err := sql.Open("pgx", cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("failed to open database connection: %w", err)
	}

	maxOpen := cfg.MaxOpenConns
	if maxOpen <= 0 {
		maxOpen = 25
	}
	maxIdle := cfg.MaxIdleConns
	if maxIdle <= 0 {
		maxIdle = 10
	}

	db.SetMaxOpenConns(maxOpen)
	db.SetMaxIdleConns(maxIdle)
	db.SetConnMaxLifetime(5 * time.Minute)
	db.SetConnMaxIdleTime(2 * time.Minute)

	drv := entsql.OpenDB("postgres", db)
	client := ent.NewClient(ent.Driver(drv))

	return &Database{
		EntClient: client,
		SQLDB:     db,
		logger:    log,
	}, nil
}

// Check satisfies handler.DependencyChecker for readiness probes.
func (d *Database) Check(ctx context.Context) error {
	if d.SQLDB == nil {
		return fmt.Errorf("database connection is nil")
	}
	return d.SQLDB.PingContext(ctx)
}

// Close gracefully closes the Ent client and database connection pool.
func (d *Database) Close() error {
	var errs []error
	if d.EntClient != nil {
		if err := d.EntClient.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close ent client: %w", err))
		}
	}
	if d.SQLDB != nil {
		if err := d.SQLDB.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close sql.DB: %w", err))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("errors closing database: %v", errs)
	}
	return nil
}

// WithTx executes a callback inside a managed database transaction with automatic commit/rollback.
func (d *Database) WithTx(ctx context.Context, fn func(tx *ent.Tx) error) error {
	tx, err := d.EntClient.Tx(ctx)
	if err != nil {
		return fmt.Errorf("starting transaction: %w", err)
	}
	defer func() {
		if r := recover(); r != nil {
			_ = tx.Rollback()
			panic(r)
		}
	}()
	if err := fn(tx); err != nil {
		if rerr := tx.Rollback(); rerr != nil {
			return fmt.Errorf("transaction error: %v, rollback error: %w", err, rerr)
		}
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing transaction: %w", err)
	}
	return nil
}
