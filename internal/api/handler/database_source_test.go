package handler_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"entgo.io/ent/dialect/sql/schema"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/your-org/contextforge/internal/api/handler"
	"github.com/your-org/contextforge/internal/connector"
	"github.com/your-org/contextforge/internal/crypto"
	"github.com/your-org/contextforge/internal/ent"
	"github.com/your-org/contextforge/internal/service"
	_ "modernc.org/sqlite"
)

func setupTestDB(t *testing.T) (*ent.Client, []byte, string) {
	ctx := context.Background()
	sqlDB, err := sql.Open("sqlite", "file:ent_db_api_test?mode=memory&cache=shared&_fk=1")
	require.NoError(t, err)

	_, err = sqlDB.Exec("PRAGMA foreign_keys = ON;")
	require.NoError(t, err)

	drv := entsql.OpenDB(dialect.SQLite, sqlDB)
	client := ent.NewClient(ent.Driver(drv))
	require.NoError(t, client.Schema.Create(ctx, schema.WithForeignKeys(false)))

	encKey, err := crypto.GenerateKey()
	require.NoError(t, err)

	// Create a sample SQLite external database
	tmpDir := t.TempDir()
	extDBPath := filepath.Join(tmpDir, "test_api_ext.db")
	extDB, err := sql.Open("sqlite", extDBPath)
	require.NoError(t, err)
	_, err = extDB.Exec(`
		CREATE TABLE items (id INTEGER PRIMARY KEY, name TEXT, secret_key TEXT);
		INSERT INTO items VALUES (1, 'hammer', 'sec_123');
	`)
	require.NoError(t, err)
	extDB.Close()

	return client, encKey, extDBPath
}

func TestDatabaseSourceHandler_EndToEnd(t *testing.T) {
	gin.SetMode(gin.TestMode)
	client, encKey, extDBPath := setupTestDB(t)
	defer client.Close()

	ctx := context.Background()
	user, err := client.User.Create().SetGithubLogin("bob").SetEmail("bob@example.com").Save(ctx)
	require.NoError(t, err)

	projA, err := client.Project.Create().SetName("Project A").SetOwnerUserID(user.ID).Save(ctx)
	require.NoError(t, err)

	projB, err := client.Project.Create().SetName("Project B").SetOwnerUserID(user.ID).Save(ctx)
	require.NoError(t, err)

	dbSvc := service.NewDatabaseSourceService(
		client,
		connector.DefaultRegistry(),
		encKey,
		nil,
		nil,
		true,
		zap.NewNop(),
	)
	h := handler.NewDatabaseSourceHandler(dbSvc, zap.NewNop())

	r := gin.New()
	pGroup := r.Group("/projects/:id/sources/database")
	{
		pGroup.POST("/test", h.TestRawConnection)
		pGroup.POST("", h.CreateDatabaseSource)
		pGroup.GET("", h.ListDatabaseSources)
		pGroup.GET("/:source_id", h.GetDatabaseSource)
		pGroup.PATCH("/:source_id", h.UpdateDatabaseSource)
		pGroup.DELETE("/:source_id", h.DeleteDatabaseSource)
		pGroup.POST("/:source_id/test", h.TestStoredConnection)
		pGroup.GET("/:source_id/metadata", h.GetMetadata)
		pGroup.POST("/:source_id/sync", h.TriggerSync)
		pGroup.GET("/:source_id/status", h.GetStatus)
	}

	// 1. Test Raw Connection
	t.Run("TestRawConnection", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"database_type":  "sqlite",
			"connection_url": "sqlite://" + extDBPath,
		})
		req := httptest.NewRequest("POST", "/projects/"+projA.ID.String()+"/sources/database/test", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		var res connector.ConnectionTestResult
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &res))
		assert.True(t, res.Success)
	})

	// 2. Create Database Source in Project A
	var sourceID, dbSourceID string
	t.Run("CreateDatabaseSource", func(t *testing.T) {
		body, _ := json.Marshal(map[string]any{
			"name":           "Inventory SQLite",
			"database_type":  "sqlite",
			"connection_url": "sqlite://" + extDBPath,
			"configuration": map[string]any{
				"mode": "schema",
			},
		})
		req := httptest.NewRequest("POST", "/projects/"+projA.ID.String()+"/sources/database", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusCreated, w.Code)
		var res service.DatabaseSourceResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &res))
		assert.Equal(t, "Inventory SQLite", res.Name)
		assert.Equal(t, "sqlite", res.DatabaseType)
		assert.Equal(t, projA.ID, res.ProjectID)
		sourceID = res.SourceID.String()
		dbSourceID = res.ID.String()

		// Verify zero secrets leaked in JSON response
		rawJSON := w.Body.String()
		assert.NotContains(t, rawJSON, "password")
		assert.NotContains(t, rawJSON, "encrypted_connection_url")
	})

	// 3. List Database Sources for Project A and Project B
	t.Run("ListDatabaseSources", func(t *testing.T) {
		// Project A should have 1 source
		reqA := httptest.NewRequest("GET", "/projects/"+projA.ID.String()+"/sources/database", nil)
		wA := httptest.NewRecorder()
		r.ServeHTTP(wA, reqA)
		assert.Equal(t, http.StatusOK, wA.Code)
		var listA []service.DatabaseSourceResponse
		require.NoError(t, json.Unmarshal(wA.Body.Bytes(), &listA))
		assert.Len(t, listA, 1)

		// Project B should have 0 sources (isolation)
		reqB := httptest.NewRequest("GET", "/projects/"+projB.ID.String()+"/sources/database", nil)
		wB := httptest.NewRecorder()
		r.ServeHTTP(wB, reqB)
		assert.Equal(t, http.StatusOK, wB.Code)
		var listB []service.DatabaseSourceResponse
		require.NoError(t, json.Unmarshal(wB.Body.Bytes(), &listB))
		assert.Len(t, listB, 0)
	})

	// 4. Project Isolation on GET: Project B cannot view Project A source
	t.Run("ProjectIsolation_GET", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/projects/"+projB.ID.String()+"/sources/database/"+sourceID, nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("ProjectIsolation_MutationsAndIntrospection", func(t *testing.T) {
		// Project B cannot patch Project A source
		patchBody, _ := json.Marshal(map[string]any{"name": "Hacked Name"})
		reqPatch := httptest.NewRequest("PATCH", "/projects/"+projB.ID.String()+"/sources/database/"+sourceID, bytes.NewReader(patchBody))
		reqPatch.Header.Set("Content-Type", "application/json")
		wPatch := httptest.NewRecorder()
		r.ServeHTTP(wPatch, reqPatch)
		assert.Equal(t, http.StatusNotFound, wPatch.Code)

		// Project B cannot get metadata of Project A source
		reqMeta := httptest.NewRequest("GET", "/projects/"+projB.ID.String()+"/sources/database/"+sourceID+"/metadata", nil)
		wMeta := httptest.NewRecorder()
		r.ServeHTTP(wMeta, reqMeta)
		assert.Equal(t, http.StatusNotFound, wMeta.Code)

		// Project B cannot test Project A source
		reqTest := httptest.NewRequest("POST", "/projects/"+projB.ID.String()+"/sources/database/"+sourceID+"/test", nil)
		wTest := httptest.NewRecorder()
		r.ServeHTTP(wTest, reqTest)
		assert.Equal(t, http.StatusNotFound, wTest.Code)

		// Project B cannot trigger sync on Project A source
		reqSync := httptest.NewRequest("POST", "/projects/"+projB.ID.String()+"/sources/database/"+sourceID+"/sync", nil)
		wSync := httptest.NewRecorder()
		r.ServeHTTP(wSync, reqSync)
		assert.Equal(t, http.StatusNotFound, wSync.Code)

		// Project B cannot delete Project A source
		reqDel := httptest.NewRequest("DELETE", "/projects/"+projB.ID.String()+"/sources/database/"+sourceID, nil)
		wDel := httptest.NewRecorder()
		r.ServeHTTP(wDel, reqDel)
		assert.Equal(t, http.StatusNotFound, wDel.Code)
	})

	// 5. Test Stored Connection
	t.Run("TestStoredConnection", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/projects/"+projA.ID.String()+"/sources/database/"+sourceID+"/test", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
		var res connector.ConnectionTestResult
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &res))
		assert.True(t, res.Success)
	})

	// 6. Get Metadata
	t.Run("GetMetadata", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/projects/"+projA.ID.String()+"/sources/database/"+sourceID+"/metadata", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
		var meta connector.DatabaseMetadata
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &meta))
		require.Len(t, meta.Schemas, 1)
		assert.Equal(t, "items", meta.Schemas[0].Tables[0].Name)
	})

	// 7. Get Status
	t.Run("GetStatus", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/projects/"+projA.ID.String()+"/sources/database/"+sourceID+"/status", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	// 8. Delete Database Source
	t.Run("DeleteDatabaseSource", func(t *testing.T) {
		req := httptest.NewRequest("DELETE", "/projects/"+projA.ID.String()+"/sources/database/"+sourceID, nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusNoContent, w.Code)

		// Verify deletion
		reqGet := httptest.NewRequest("GET", "/projects/"+projA.ID.String()+"/sources/database/"+sourceID, nil)
		wGet := httptest.NewRecorder()
		r.ServeHTTP(wGet, reqGet)
		assert.Equal(t, http.StatusNotFound, wGet.Code)
		_ = dbSourceID
	})
}
