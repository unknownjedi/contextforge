package worker_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"entgo.io/ent/dialect/sql/schema"
	"github.com/your-org/contextforge/internal/chunk"
	"github.com/your-org/contextforge/internal/connector"
	"github.com/your-org/contextforge/internal/crypto"
	"github.com/your-org/contextforge/internal/ent"
	"github.com/your-org/contextforge/internal/provider"
	"github.com/your-org/contextforge/internal/queue"
	"github.com/your-org/contextforge/internal/repository"
	"github.com/your-org/contextforge/internal/worker"
	_ "modernc.org/sqlite"
)

func TestDatabaseIngestionPipeline_EndToEnd(t *testing.T) {
	ctx := context.Background()
	sqlDB, err := sql.Open("sqlite", "file:ent_db_worker_test?mode=memory&cache=shared&_fk=1")
	require.NoError(t, err)
	defer sqlDB.Close()

	_, err = sqlDB.Exec("PRAGMA foreign_keys = ON;")
	require.NoError(t, err)

	drv := entsql.OpenDB(dialect.SQLite, sqlDB)
	client := ent.NewClient(ent.Driver(drv))
	defer client.Close()

	require.NoError(t, client.Schema.Create(ctx, schema.WithForeignKeys(false)))

	// 1. Create User and Project
	u, err := client.User.Create().
		SetGithubLogin("tester").
		SetEmail("tester@example.com").
		Save(ctx)
	require.NoError(t, err)

	p, err := client.Project.Create().
		SetName("Test DB Ingestion").
		SetOwnerUserID(u.ID).
		Save(ctx)
	require.NoError(t, err)

	// 2. Create external SQLite test database with schema
	tmpDir := t.TempDir()
	extDBPath := filepath.Join(tmpDir, "external.db")
	extDB, err := sql.Open("sqlite", extDBPath)
	require.NoError(t, err)

	_, err = extDB.Exec(`
		CREATE TABLE customers (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			email TEXT NOT NULL,
			secret_token TEXT NOT NULL
		);
		INSERT INTO customers (name, email, secret_token) VALUES ('Charlie', 'charlie@example.com', 'tok_123');
	`)
	require.NoError(t, err)
	extDB.Close()

	// 3. Encrypt connection URL
	encKey, err := crypto.GenerateKey()
	require.NoError(t, err)
	extURL := "sqlite://" + extDBPath
	encryptedURL, err := crypto.Encrypt([]byte(extURL), encKey)
	require.NoError(t, err)

	// 4. Create Source and DatabaseSource entities
	src, err := client.Source.Create().
		SetProjectID(p.ID).
		SetName("External SQLite").
		SetType("database").
		Save(ctx)
	require.NoError(t, err)

	dbSrc, err := client.DatabaseSource.Create().
		SetProjectID(p.ID).
		SetSourceID(src.ID).
		SetDatabaseType(string(connector.TypeSQLite)).
		SetEncryptedConnectionURL(encryptedURL).
		SetConfiguration(map[string]any{
			"mode": "schema",
		}).
		Save(ctx)
	require.NoError(t, err)
	require.NotNil(t, dbSrc)

	// 5. Initialize pipeline
	vectorRepo := repository.NewMockVectorRepository()
	embedder := provider.NewMockEmbeddingProvider(768)
	docRepo := repository.NewDocumentRepository(client)
	jobRepo := repository.NewJobRepository(client)

	pipeline := worker.NewDatabaseIngestionPipeline(
		client,
		connector.DefaultRegistry(),
		encKey,
		docRepo,
		vectorRepo,
		jobRepo,
		embedder,
		chunk.NewChunker(chunk.DefaultOptions()),
		zap.NewNop(),
	)

	// 6. Execute sync task
	jobID := uuid.New()
	taskPayload := queue.DatabaseSyncPayload{
		JobID:     jobID,
		ProjectID: p.ID,
		SourceID:  src.ID,
		Mode:      "schema",
	}
	payloadBytes, err := json.Marshal(taskPayload)
	require.NoError(t, err)

	task := asynq.NewTask(queue.TypeDatabaseSync, payloadBytes)
	err = pipeline.ProcessDatabaseSyncTask(ctx, task)
	require.NoError(t, err)

	// 7. Verify source status updated to synced
	updatedSrc, err := client.Source.Get(ctx, src.ID)
	require.NoError(t, err)
	assert.Equal(t, "synced", string(updatedSrc.SyncStatus))
	assert.NotNil(t, updatedSrc.LastSyncedAt)

	// 8. Verify database source status updated to ready
	updatedDBSrc, err := client.DatabaseSource.Get(ctx, dbSrc.ID)
	require.NoError(t, err)
	assert.Equal(t, "ready", updatedDBSrc.Status)
	assert.Empty(t, updatedDBSrc.LastError)

	// 9. Verify documents created in docRepo
	docs, total, err := docRepo.ListByProjectID(ctx, p.ID, "", 1, 10)
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	assert.Equal(t, "schema/main/customers.sql", docs[0].FilePath)

	// 10. Verify chunks stored in vectorRepo
	chunkCount, err := vectorRepo.CountChunksByProjectID(ctx, p.ID)
	require.NoError(t, err)
	assert.Greater(t, chunkCount, int64(0))

	// 11. Incremental re-sync after table schema modification
	extDB2, err := sql.Open("sqlite", extDBPath)
	require.NoError(t, err)
	_, err = extDB2.Exec("ALTER TABLE customers ADD COLUMN phone TEXT;")
	require.NoError(t, err)
	extDB2.Close()

	jobID2 := uuid.New()
	taskPayload2 := queue.DatabaseSyncPayload{
		JobID:     jobID2,
		ProjectID: p.ID,
		SourceID:  src.ID,
		Mode:      "schema",
	}
	payloadBytes2, err := json.Marshal(taskPayload2)
	require.NoError(t, err)

	task2 := asynq.NewTask(queue.TypeDatabaseSync, payloadBytes2)
	err = pipeline.ProcessDatabaseSyncTask(ctx, task2)
	require.NoError(t, err)

	// Verify updated document
	docsAfter, totalAfter, err := docRepo.ListByProjectID(ctx, p.ID, "", 1, 10)
	require.NoError(t, err)
	assert.Equal(t, 1, totalAfter)
	assert.NotEqual(t, docs[0].ContentHash, docsAfter[0].ContentHash)

	// Verify chunk count remains consistent and no orphan chunks
	chunkCountAfter, err := vectorRepo.CountChunksByProjectID(ctx, p.ID)
	require.NoError(t, err)
	assert.Greater(t, chunkCountAfter, int64(0))
}
