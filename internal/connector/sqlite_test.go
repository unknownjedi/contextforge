package connector

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSQLiteConnector_EndToEnd(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_contextforge.db")

	// 1. Initialize SQLite database with sample schema & data
	initDB, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)

	_, err = initDB.Exec(`
		CREATE TABLE users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			username TEXT NOT NULL,
			email TEXT NOT NULL UNIQUE,
			password_hash TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);

		CREATE TABLE orders (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			total_amount REAL NOT NULL,
			status TEXT DEFAULT 'pending',
			FOREIGN KEY(user_id) REFERENCES users(id)
		);

		CREATE INDEX idx_orders_status ON orders(status);

		INSERT INTO users (username, email, password_hash) VALUES 
		('alice', 'alice@example.com', 'hash_secret_123'),
		('bob', 'bob@example.com', 'hash_secret_456');

		INSERT INTO orders (user_id, total_amount, status) VALUES 
		(1, 49.99, 'completed'),
		(1, 120.00, 'pending'),
		(2, 15.50, 'completed');
	`)
	require.NoError(t, err)
	initDB.Close()

	// 2. Test Connector
	conn := NewSQLiteConnector()
	cfg, err := conn.ParseURL("sqlite://" + dbPath)
	require.NoError(t, err)

	ctx := context.Background()

	// Connection Test
	testRes, err := conn.TestConnection(ctx, cfg)
	require.NoError(t, err)
	assert.True(t, testRes.Success)
	assert.NotEmpty(t, testRes.DatabaseVersion)

	// Schema Extraction
	meta, err := conn.ExtractSchema(ctx, cfg, DefaultExtractionConfig())
	require.NoError(t, err)
	require.NotNil(t, meta)
	assert.Equal(t, string(TypeSQLite), meta.DatabaseType)
	require.Len(t, meta.Schemas, 1)

	schema := meta.Schemas[0]
	assert.Equal(t, "main", schema.Name)
	assert.Len(t, schema.Tables, 2)

	// Verify users table
	var usersTable, ordersTable *TableMetadata
	for i := range schema.Tables {
		if schema.Tables[i].Name == "users" {
			usersTable = &schema.Tables[i]
		} else if schema.Tables[i].Name == "orders" {
			ordersTable = &schema.Tables[i]
		}
	}
	require.NotNil(t, usersTable, "users table not found")
	require.NotNil(t, ordersTable, "orders table not found")

	assert.Contains(t, usersTable.PrimaryKey, "id")
	hasPasswordHash := false
	for _, col := range usersTable.Columns {
		if col.Name == "password_hash" {
			hasPasswordHash = true
			assert.True(t, col.IsSensitive, "password_hash must be marked sensitive")
		}
	}
	assert.True(t, hasPasswordHash)

	// Verify orders table relationships
	assert.Len(t, ordersTable.ForeignKeys, 1)
	assert.Equal(t, "users", ordersTable.ForeignKeys[0].ReferencedTable)
	assert.Equal(t, []string{"user_id"}, ordersTable.ForeignKeys[0].Columns)
	assert.Equal(t, []string{"id"}, ordersTable.ForeignKeys[0].ReferencedColumns)

	// Verify row extraction
	rows, err := conn.ExtractRows(ctx, cfg, "main", "users", []string{"id", "username", "email"}, 10, 0)
	require.NoError(t, err)
	assert.Len(t, rows, 2)
	assert.Equal(t, "alice", rows[0].Values["username"])
	assert.Equal(t, "bob", rows[1].Values["username"])

	// Test non-existent file
	badCfg := &ConnectionConfig{
		Type:     TypeSQLite,
		FilePath: filepath.Join(tmpDir, "does_not_exist.db"),
	}
	badTest, err := conn.TestConnection(ctx, badCfg)
	require.NoError(t, err)
	assert.False(t, badTest.Success)
	assert.Contains(t, badTest.ErrorMessage, "does not exist")
}
