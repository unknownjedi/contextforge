package database_test

import (
	"context"
	"database/sql"
	"os"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/your-org/contextforge/internal/config"
	"github.com/your-org/contextforge/internal/database"
)

func TestAutoMigrate_LiveOrSkip(t *testing.T) {
	cfg, err := config.Load()
	if err != nil || cfg.Database.URL == "" {
		t.Skip("skipping database migration test; no database config")
	}

	db, err := sql.Open("pgx", cfg.Database.URL)
	if err != nil {
		t.Skip("failed to open database connection:", err)
	}
	defer db.Close()

	ctx := context.Background()
	if err := db.PingContext(ctx); err != nil {
		t.Skip("skipping test; database unreachable:", err)
	}

	err = database.EnsureMigrationTable(ctx, db)
	require.NoError(t, err)

	applied, err := database.GetAppliedMigrations(ctx, db)
	require.NoError(t, err)
	assert.NotEmpty(t, applied)

	// AutoMigrate should be idempotent and succeed
	migrationsDir := "../../migrations"
	if _, err := os.Stat(migrationsDir); os.IsNotExist(err) {
		migrationsDir = "migrations"
	}
	err = database.AutoMigrate(ctx, db, migrationsDir, zap.NewNop())
	require.NoError(t, err)
}
