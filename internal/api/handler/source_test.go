package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/your-org/contextforge/internal/api/handler"
	"github.com/your-org/contextforge/internal/ent"
	"github.com/your-org/contextforge/internal/ent/source"
	"github.com/your-org/contextforge/internal/urlfetch"
)

type mockSourceRepo struct {
	sources map[uuid.UUID]*ent.Source
}

func newMockSourceRepo() *mockSourceRepo {
	return &mockSourceRepo{sources: make(map[uuid.UUID]*ent.Source)}
}

func (m *mockSourceRepo) Create(ctx context.Context, s *ent.Source) (*ent.Source, error) {
	if s.ID == uuid.Nil {
		s.ID = uuid.New()
	}
	s.CreatedAt = time.Now()
	s.UpdatedAt = time.Now()
	m.sources[s.ID] = s
	return s, nil
}

func (m *mockSourceRepo) GetByID(ctx context.Context, id, projectID uuid.UUID) (*ent.Source, error) {
	s, ok := m.sources[id]
	if !ok || s.ProjectID != projectID {
		return nil, fmt.Errorf("source not found")
	}
	return s, nil
}

func (m *mockSourceRepo) ListByProjectID(ctx context.Context, projectID uuid.UUID) ([]*ent.Source, error) {
	var list []*ent.Source
	for _, s := range m.sources {
		if s.ProjectID == projectID {
			list = append(list, s)
		}
	}
	return list, nil
}

func (m *mockSourceRepo) UpdateSyncStatus(ctx context.Context, id, projectID uuid.UUID, status source.SyncStatus, commitHash string, syncedAt *time.Time) (*ent.Source, error) {
	s, ok := m.sources[id]
	if !ok || s.ProjectID != projectID {
		return nil, fmt.Errorf("source not found")
	}
	s.SyncStatus = status
	s.LastCommitHash = commitHash
	s.LastSyncedAt = syncedAt
	return s, nil
}

func (m *mockSourceRepo) Delete(ctx context.Context, id, projectID uuid.UUID) error {
	delete(m.sources, id)
	return nil
}

func setupSourceRouter(sourceRepo *mockSourceRepo, jobRepo *mockJobRepo) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	h := handler.NewSourceHandler(sourceRepo, jobRepo, nil, zap.NewNop())

	router.POST("/projects/:id/sources", h.CreateSource)
	router.GET("/projects/:id/sources", h.ListSources)
	router.DELETE("/projects/:id/sources/:source_id", h.DeleteSource)
	router.POST("/projects/:id/sources/:source_id/sync", h.SyncSource)

	return router
}

func TestSourceHandler_CreateURLSource_Success(t *testing.T) {
	cleanup := urlfetch.SetLookupIPForTesting(func(host string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("93.184.216.34")}, nil
	})
	defer cleanup()

	sourceRepo := newMockSourceRepo()
	jobRepo := newMockJobRepo()
	router := setupSourceRouter(sourceRepo, jobRepo)
	projectID := uuid.New()

	payload := map[string]string{
		"name": "ContextForge Docs",
		"type": "url",
		"url":  "https://example.com/docs",
	}
	body, _ := json.Marshal(payload)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/projects/%s/sources", projectID), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	assert.Equal(t, "ContextForge Docs", resp["name"])
	assert.Equal(t, "url", resp["type"])
	assert.Equal(t, "https://example.com/docs", resp["repo_name"])
	assert.NotEmpty(t, resp["job_id"])

	// Verify in repository
	sourceID, err := uuid.Parse(resp["id"].(string))
	require.NoError(t, err)
	stored, ok := sourceRepo.sources[sourceID]
	require.True(t, ok)
	assert.Equal(t, "url", stored.Type)
	assert.Equal(t, "https://example.com/docs", stored.RepoName)
}

func TestSourceHandler_CreateURLSource_SSRFBlocked(t *testing.T) {
	sourceRepo := newMockSourceRepo()
	jobRepo := newMockJobRepo()
	router := setupSourceRouter(sourceRepo, jobRepo)
	projectID := uuid.New()

	blockedURLs := []string{
		"http://127.0.0.1:8080/admin",
		"http://localhost:3000",
		"http://169.254.169.254/latest/meta-data/",
		"http://10.0.0.1/internal",
		"ftp://example.com/secret.txt",
		"file:///etc/passwd",
	}

	for _, u := range blockedURLs {
		t.Run(u, func(t *testing.T) {
			payload := map[string]string{
				"name": "Malicious Source",
				"type": "url",
				"url":  u,
			}
			body, _ := json.Marshal(payload)

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/projects/%s/sources", projectID), bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusBadRequest, w.Code)
			var resp map[string]string
			_ = json.Unmarshal(w.Body.Bytes(), &resp)
			assert.Contains(t, resp["error"], "invalid URL")
		})
	}
}

func TestSourceHandler_CreateURLSource_MissingURL(t *testing.T) {
	sourceRepo := newMockSourceRepo()
	jobRepo := newMockJobRepo()
	router := setupSourceRouter(sourceRepo, jobRepo)
	projectID := uuid.New()

	payload := map[string]string{
		"name": "Empty URL Source",
		"type": "url",
		"url":  "",
	}
	body, _ := json.Marshal(payload)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/projects/%s/sources", projectID), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	var resp map[string]string
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Contains(t, resp["error"], "url is required")
}

func TestSourceHandler_CreateGitHubSource(t *testing.T) {
	sourceRepo := newMockSourceRepo()
	jobRepo := newMockJobRepo()
	router := setupSourceRouter(sourceRepo, jobRepo)
	projectID := uuid.New()

	// Missing repo_owner / repo_name
	payload := map[string]string{
		"name": "ContextForge Code",
		"type": "github",
	}
	body, _ := json.Marshal(payload)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/projects/%s/sources", projectID), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	// Valid github source
	validPayload := map[string]string{
		"name":       "ContextForge Code",
		"type":       "github",
		"repo_owner": "testorg",
		"repo_name":  "contextforge",
		"branch":     "develop",
	}
	validBody, _ := json.Marshal(validPayload)

	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/projects/%s/sources", projectID), bytes.NewReader(validBody))
	req2.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w2, req2)

	assert.Equal(t, http.StatusCreated, w2.Code)
}

func TestSourceHandler_SyncURLSource(t *testing.T) {
	sourceRepo := newMockSourceRepo()
	jobRepo := newMockJobRepo()
	router := setupSourceRouter(sourceRepo, jobRepo)
	projectID := uuid.New()
	sourceID := uuid.New()

	// Add a URL source to repo
	sourceRepo.sources[sourceID] = &ent.Source{
		ID:        sourceID,
		ProjectID: projectID,
		Name:      "Docs",
		Type:      "url",
		RepoName:  "https://example.com/docs",
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/projects/%s/sources/%s/sync", projectID, sourceID), nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusAccepted, w.Code)
	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "queued", resp["status"])
	assert.NotEmpty(t, resp["job_id"])
}
