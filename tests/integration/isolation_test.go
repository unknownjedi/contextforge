package integration_test

import (
	"context"
	"errors"
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
	"github.com/your-org/contextforge/internal/api/middleware"
	"github.com/your-org/contextforge/internal/auth"
	"github.com/your-org/contextforge/internal/crypto"
	"github.com/your-org/contextforge/internal/ent"
	"github.com/your-org/contextforge/internal/model"
	"github.com/your-org/contextforge/internal/repository"
)

// mockProjectRepo implements repository.ProjectRepository for isolation testing.
type mockProjectRepo struct {
	projects map[uuid.UUID]*ent.Project
}

func newMockProjectRepo() *mockProjectRepo {
	return &mockProjectRepo{
		projects: make(map[uuid.UUID]*ent.Project),
	}
}

func (m *mockProjectRepo) Create(ctx context.Context, p *ent.Project) (*ent.Project, error) {
	if p.ID == uuid.Nil {
		p.ID = uuid.New()
	}
	m.projects[p.ID] = p
	return p, nil
}

func (m *mockProjectRepo) GetByID(ctx context.Context, id, ownerUserID uuid.UUID) (*ent.Project, error) {
	p, exists := m.projects[id]
	if !exists {
		return nil, errors.New("project not found")
	}
	if p.OwnerUserID != ownerUserID {
		return nil, errors.New("unauthorized project access")
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
	existing, err := m.GetByID(ctx, p.ID, ownerUserID)
	if err != nil {
		return nil, err
	}
	m.projects[existing.ID] = p
	return p, nil
}

func (m *mockProjectRepo) Delete(ctx context.Context, id, ownerUserID uuid.UUID) error {
	_, err := m.GetByID(ctx, id, ownerUserID)
	if err != nil {
		return err
	}
	delete(m.projects, id)
	return nil
}

// TestVectorDataIsolation verifies that vector searches strictly isolate project data.
func TestVectorDataIsolation(t *testing.T) {
	ctx := context.Background()
	vectorRepo := repository.NewMockVectorRepository()

	projectA := uuid.New()
	projectB := uuid.New()
	docA := uuid.New()
	docB := uuid.New()

	embedding := []float32{0.5, 0.5, 0.5, 0.5}

	chunkA := &model.DocumentChunk{
		ID:          uuid.New(),
		ProjectID:   projectA,
		DocumentID:  docA,
		ChunkIndex:  0,
		StartLine:   1,
		EndLine:     10,
		Content:     "Project A proprietary source code algorithm",
		ContentHash: "hash_a",
		TokenCount:  8,
		Embedding:   embedding,
		CreatedAt:   time.Now(),
	}

	chunkB := &model.DocumentChunk{
		ID:          uuid.New(),
		ProjectID:   projectB,
		DocumentID:  docB,
		ChunkIndex:  0,
		StartLine:   1,
		EndLine:     10,
		Content:     "Project B proprietary financial secrets",
		ContentHash: "hash_b",
		TokenCount:  8,
		Embedding:   embedding,
		CreatedAt:   time.Now(),
	}

	// Index chunks in mock vector repository
	err := vectorRepo.UpsertChunks(ctx, []*model.DocumentChunk{chunkA, chunkB})
	require.NoError(t, err)

	// Search in Project A scope
	matchesA, err := vectorRepo.SearchSimilar(ctx, model.VectorSearchParams{
		ProjectID:      projectA,
		QueryEmbedding: embedding,
		TopK:           10,
		SimilarityMin:  0.0,
	})
	require.NoError(t, err)
	assert.Len(t, matchesA, 1)
	assert.Equal(t, projectA, matchesA[0].Chunk.ProjectID)
	assert.Equal(t, chunkA.Content, matchesA[0].Chunk.Content)

	// Search in Project B scope
	matchesB, err := vectorRepo.SearchSimilar(ctx, model.VectorSearchParams{
		ProjectID:      projectB,
		QueryEmbedding: embedding,
		TopK:           10,
		SimilarityMin:  0.0,
	})
	require.NoError(t, err)
	assert.Len(t, matchesB, 1)
	assert.Equal(t, projectB, matchesB[0].Chunk.ProjectID)
	assert.Equal(t, chunkB.Content, matchesB[0].Chunk.Content)

	// Clean up Project A
	err = vectorRepo.DeleteChunksByProjectID(ctx, projectA)
	require.NoError(t, err)

	// Verify Project A is empty and Project B is untouched
	countA, err := vectorRepo.CountChunksByProjectID(ctx, projectA)
	require.NoError(t, err)
	assert.Equal(t, int64(0), countA)

	countB, err := vectorRepo.CountChunksByProjectID(ctx, projectB)
	require.NoError(t, err)
	assert.Equal(t, int64(1), countB)
}

// TestAPITenantIsolation verifies cross-tenant access rejection at HTTP layer.
func TestAPITenantIsolation(t *testing.T) {
	gin.SetMode(gin.TestMode)

	userA := uuid.New()
	userB := uuid.New()
	jwtSecret := []byte("test_secret_32_bytes_long_123456")

	// Generate valid tokens
	tokenA, err := auth.GenerateToken(userA, "usera", jwtSecret, time.Hour)
	require.NoError(t, err)

	tokenB, err := auth.GenerateToken(userB, "userb", jwtSecret, time.Hour)
	require.NoError(t, err)

	repo := newMockProjectRepo()
	projectA := &ent.Project{
		ID:          uuid.New(),
		Name:        "Project Alpha",
		OwnerUserID: userA,
	}
	projectB := &ent.Project{
		ID:          uuid.New(),
		Name:        "Project Beta",
		OwnerUserID: userB,
	}
	_, _ = repo.Create(context.Background(), projectA)
	_, _ = repo.Create(context.Background(), projectB)

	h := handler.NewProjectHandler(repo, zap.NewNop())

	router := gin.New()
	api := router.Group("/api/v1")
	api.Use(auth.AuthMiddleware(jwtSecret))
	{
		api.GET("/projects", h.ListProjects)
		api.POST("/projects", h.CreateProject)

		projectGroup := api.Group("/projects/:id")
		projectGroup.Use(middleware.RequireProjectAccess(repo))
		{
			projectGroup.GET("", h.GetProject)
			projectGroup.PATCH("", h.UpdateProject)
			projectGroup.DELETE("", h.DeleteProject)
		}
	}

	// 1. User A lists their projects -> sees Project A, not Project B
	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects", nil)
	req.Header.Set("Authorization", "Bearer "+tokenA)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), projectA.Name)
	assert.NotContains(t, w.Body.String(), projectB.Name)

	// 2. User A attempts to GET Project B -> 404 Cloaked
	reqB := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+projectB.ID.String(), nil)
	reqB.Header.Set("Authorization", "Bearer "+tokenA)
	wB := httptest.NewRecorder()
	router.ServeHTTP(wB, reqB)
	assert.Equal(t, http.StatusNotFound, wB.Code)

	// 3. User B accesses Project B -> 200 OK
	reqBValid := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+projectB.ID.String(), nil)
	reqBValid.Header.Set("Authorization", "Bearer "+tokenB)
	wBValid := httptest.NewRecorder()
	router.ServeHTTP(wBValid, reqBValid)
	assert.Equal(t, http.StatusOK, wBValid.Code)
	assert.Contains(t, wBValid.Body.String(), projectB.Name)

	// 4. User A attempts to DELETE Project B -> 404 Cloaked
	reqDel := httptest.NewRequest(http.MethodDelete, "/api/v1/projects/"+projectB.ID.String(), nil)
	reqDel.Header.Set("Authorization", "Bearer "+tokenA)
	wDel := httptest.NewRecorder()
	router.ServeHTTP(wDel, reqDel)
	assert.Equal(t, http.StatusNotFound, wDel.Code)

	// Verify Project B still exists
	assert.NotNil(t, repo.projects[projectB.ID])
}

// TestCryptoKeyIsolation verifies that ciphertext encrypted with Key A cannot be read with Key B.
func TestCryptoKeyIsolation(t *testing.T) {
	keyA, err := crypto.KeyFromHex("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	require.NoError(t, err)

	keyB, err := crypto.KeyFromHex("fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210")
	require.NoError(t, err)

	secretData := []byte("ghp_userA_super_secret_github_pat_token_12345")

	encrypted, err := crypto.Encrypt(secretData, keyA)
	require.NoError(t, err)

	// Attempt decrypt with Key B
	_, err = crypto.Decrypt(encrypted, keyB)
	assert.Error(t, err, "decryption with distinct key must fail")

	// Decrypt with Key A
	decrypted, err := crypto.Decrypt(encrypted, keyA)
	require.NoError(t, err)
	assert.Equal(t, secretData, decrypted)
}
