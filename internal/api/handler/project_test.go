package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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
	"github.com/your-org/contextforge/internal/auth"
	"github.com/your-org/contextforge/internal/ent"
)

type mockProjectRepo struct {
	projects map[uuid.UUID]*ent.Project
}

func newMockProjectRepo() *mockProjectRepo {
	return &mockProjectRepo{projects: make(map[uuid.UUID]*ent.Project)}
}

func (m *mockProjectRepo) Create(ctx context.Context, p *ent.Project) (*ent.Project, error) {
	if p.ID == uuid.Nil {
		p.ID = uuid.New()
	}
	m.projects[p.ID] = p
	return p, nil
}

func (m *mockProjectRepo) GetByID(ctx context.Context, id, ownerUserID uuid.UUID) (*ent.Project, error) {
	p, ok := m.projects[id]
	if !ok || p.OwnerUserID != ownerUserID {
		return nil, fmt.Errorf("project not found")
	}
	return p, nil
}

func (m *mockProjectRepo) ListByOwner(ctx context.Context, ownerUserID uuid.UUID, page, pageSize int) ([]*ent.Project, int, error) {
	var results []*ent.Project
	for _, p := range m.projects {
		if p.OwnerUserID == ownerUserID {
			results = append(results, p)
		}
	}
	return results, len(results), nil
}

func (m *mockProjectRepo) Update(ctx context.Context, p *ent.Project, ownerUserID uuid.UUID) (*ent.Project, error) {
	existing, ok := m.projects[p.ID]
	if !ok || existing.OwnerUserID != ownerUserID {
		return nil, fmt.Errorf("project not found")
	}
	m.projects[p.ID] = p
	return p, nil
}

func (m *mockProjectRepo) Delete(ctx context.Context, id, ownerUserID uuid.UUID) error {
	existing, ok := m.projects[id]
	if !ok || existing.OwnerUserID != ownerUserID {
		return fmt.Errorf("project not found")
	}
	delete(m.projects, id)
	return nil
}

func TestProjectHandler_Lifecycle(t *testing.T) {
	gin.SetMode(gin.TestMode)
	jwtSecret := []byte("32_byte_secret_key_for_testing!!")
	userID := uuid.New()
	token, err := auth.GenerateToken(userID, "testuser", jwtSecret, time.Hour)
	require.NoError(t, err)

	repo := newMockProjectRepo()
	h := handler.NewProjectHandler(repo, zap.NewNop())

	router := gin.New()
	authed := router.Group("/api/v1")
	authed.Use(auth.AuthMiddleware(jwtSecret))
	{
		authed.GET("/projects", h.ListProjects)
		authed.POST("/projects", h.CreateProject)
		authed.GET("/projects/:id", h.GetProject)
		authed.PATCH("/projects/:id", h.UpdateProject)
		authed.PUT("/projects/:id", h.UpdateProject)
		authed.DELETE("/projects/:id", h.DeleteProject)
	}

	// 1. Create Project
	createPayload := handler.CreateProjectRequest{
		Name:        "Test Project",
		Description: "A test project",
	}
	payloadBytes, _ := json.Marshal(createPayload)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects", bytes.NewReader(payloadBytes))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code)
	var created ent.Project
	err = json.Unmarshal(w.Body.Bytes(), &created)
	require.NoError(t, err)
	assert.Equal(t, "Test Project", created.Name)
	projectID := created.ID

	// 2. Update Project via PUT
	newName := "Updated Via PUT"
	putPayload := handler.UpdateProjectRequest{
		Name: &newName,
	}
	putBytes, _ := json.Marshal(putPayload)
	reqPUT := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/v1/projects/%s", projectID), bytes.NewReader(putBytes))
	reqPUT.Header.Set("Authorization", "Bearer "+token)
	reqPUT.Header.Set("Content-Type", "application/json")
	wPUT := httptest.NewRecorder()
	router.ServeHTTP(wPUT, reqPUT)

	require.Equal(t, http.StatusOK, wPUT.Code)
	var updated ent.Project
	err = json.Unmarshal(wPUT.Body.Bytes(), &updated)
	require.NoError(t, err)
	assert.Equal(t, "Updated Via PUT", updated.Name)

	// 3. Delete Project
	reqDel := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/projects/%s", projectID), nil)
	reqDel.Header.Set("Authorization", "Bearer "+token)
	wDel := httptest.NewRecorder()
	router.ServeHTTP(wDel, reqDel)

	require.Equal(t, http.StatusNoContent, wDel.Code)
}
