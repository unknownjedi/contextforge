package connector

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultRegistry(t *testing.T) {
	reg := DefaultRegistry()
	require.NotNil(t, reg)

	expectedTypes := []DatabaseType{
		TypePostgres,
		TypeCockroachDB,
		TypeMySQL,
		TypeMariaDB,
		TypeSQLite,
		TypeSQLServer,
	}

	for _, dt := range expectedTypes {
		c, err := reg.Get(dt)
		assert.NoError(t, err, "connector for %s should be registered", dt)
		assert.NotNil(t, c)
		assert.Equal(t, dt, c.Type())
	}

	// Unsupported type
	_, err := reg.Get("oracle")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported database type")
}

func TestURLParsing(t *testing.T) {
	reg := DefaultRegistry()

	t.Run("postgres", func(t *testing.T) {
		c, err := reg.Get(TypePostgres)
		require.NoError(t, err)

		cfg, err := c.ParseURL("postgres://user:secret@localhost:5432/testdb?sslmode=disable")
		require.NoError(t, err)
		assert.Equal(t, "localhost", cfg.Host)
		assert.Equal(t, 5432, cfg.Port)
		assert.Equal(t, "testdb", cfg.Database)
		assert.Equal(t, "user", cfg.Username)
		assert.Equal(t, "secret", cfg.Password)
		assert.Equal(t, "disable", cfg.SSLMode)
	})

	t.Run("mysql", func(t *testing.T) {
		c, err := reg.Get(TypeMySQL)
		require.NoError(t, err)

		cfg, err := c.ParseURL("mysql://root:pass@127.0.0.1:3306/shop")
		require.NoError(t, err)
		assert.Equal(t, "127.0.0.1", cfg.Host)
		assert.Equal(t, 3306, cfg.Port)
		assert.Equal(t, "shop", cfg.Database)
		assert.Equal(t, "root", cfg.Username)
		assert.Equal(t, "pass", cfg.Password)
	})

	t.Run("sqlite", func(t *testing.T) {
		c, err := reg.Get(TypeSQLite)
		require.NoError(t, err)

		cfg, err := c.ParseURL("sqlite:///tmp/test.db")
		require.NoError(t, err)
		assert.Equal(t, "/tmp/test.db", cfg.FilePath)
		assert.Equal(t, "test.db", cfg.Database)
	})

	t.Run("sqlserver", func(t *testing.T) {
		c, err := reg.Get(TypeSQLServer)
		require.NoError(t, err)

		cfg, err := c.ParseURL("sqlserver://sa:StrongPass123@dbserver:1433?database=analytics")
		require.NoError(t, err)
		assert.Equal(t, "dbserver", cfg.Host)
		assert.Equal(t, 1433, cfg.Port)
		assert.Equal(t, "analytics", cfg.Database)
		assert.Equal(t, "sa", cfg.Username)
		assert.Equal(t, "StrongPass123", cfg.Password)
	})
}
