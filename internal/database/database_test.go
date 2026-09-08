package database_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/your-org/contextforge/internal/config"
	"github.com/your-org/contextforge/internal/database"
)

func TestNew_EmptyURL(t *testing.T) {
	log := zap.NewNop()
	cfg := &config.DatabaseConfig{URL: ""}

	db, err := database.New(cfg, log)
	assert.Error(t, err)
	assert.Nil(t, db)
}

func TestNew_ValidConfigInitializesPool(t *testing.T) {
	log := zap.NewNop()
	cfg := &config.DatabaseConfig{
		URL:          "postgres://test:test@localhost:5432/test?sslmode=disable",
		MaxOpenConns: 50,
		MaxIdleConns: 20,
	}

	db, err := database.New(cfg, log)
	require.NoError(t, err)
	require.NotNil(t, db)
	defer db.Close()

	assert.NotNil(t, db.EntClient)
	assert.NotNil(t, db.SQLDB)

	stats := db.SQLDB.Stats()
	assert.Equal(t, 50, stats.MaxOpenConnections)
}

func TestDatabase_Check_FailureWhenUnreachable(t *testing.T) {
	log := zap.NewNop()
	cfg := &config.DatabaseConfig{
		URL: "postgres://invalid:invalid@localhost:9999/invalid?sslmode=disable",
	}

	db, err := database.New(cfg, log)
	require.NoError(t, err)
	defer db.Close()

	ctx := context.Background()
	err = db.Check(ctx)
	assert.Error(t, err, "ping should fail on unreachable port")
}
