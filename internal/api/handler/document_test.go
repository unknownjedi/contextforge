package handler_test

import (
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
	"github.com/your-org/contextforge/internal/ent"
	"github.com/your-org/contextforge/internal/model"
	"github.com/your-org/contextforge/internal/repository"
)

type mockDocRepo struct {
	docs map[uuid.UUID]*ent.Document
}

func newMockDocRepo() *mockDocRepo {
	return &mockDocRepo{docs: make(map[uuid.UUID]*ent.Document)}
}

func (m *mockDocRepo) Create(ctx context.Context, d *ent.Document) (*ent.Document, error) {
	if d.ID == uuid.Nil {
		d.ID = uuid.New()
	}
	m.docs[d.ID] = d
	return d, nil
}

func (m *mockDocRepo) GetByID(ctx context.Context, id, projectID uuid.UUID) (*ent.Document, error) {
	d, ok := m.docs[id]
	if !ok || d.ProjectID != projectID {
		return nil, fmt.Errorf("document not found")
	}
	return d, nil
}

func (m *mockDocRepo) GetByPath(ctx context.Context, projectID, sourceID uuid.UUID, filePath string) (*ent.Document, error) {
	for _, d := range m.docs {
		if d.ProjectID == projectID && d.SourceID == sourceID && d.FilePath == filePath {
			return d, nil
		}
	}
	return nil, fmt.Errorf("document not found")
}

func (m *mockDocRepo) ListByProjectID(ctx context.Context, projectID uuid.UUID, filePathFilter string, page, pageSize int) ([]*ent.Document, int, error) {
	var results []*ent.Document
	for _, d := range m.docs {
		if d.ProjectID == projectID {
			results = append(results, d)
		}
	}
	return results, len(results), nil
}

func (m *mockDocRepo) UpdateContentHashAndChunks(ctx context.Context, id, projectID uuid.UUID, contentHash string, totalChunks int) (*ent.Document, error) {
	d, err := m.GetByID(ctx, id, projectID)
	if err != nil {
		return nil, err
	}
	d.ContentHash = contentHash
	d.TotalChunks = totalChunks
	return d, nil
}

func (m *mockDocRepo) Delete(ctx context.Context, id, projectID uuid.UUID) error {
	_, err := m.GetByID(ctx, id, projectID)
	if err != nil {
		return err
	}
	delete(m.docs, id)
	return nil
}

func (m *mockDocRepo) DeleteBySourceID(ctx context.Context, projectID, sourceID uuid.UUID) error {
	for id, d := range m.docs {
		if d.ProjectID == projectID && d.SourceID == sourceID {
			delete(m.docs, id)
		}
	}
	return nil
}

func TestDocumentHandler_GetDocumentWithChunks(t *testing.T) {
	gin.SetMode(gin.TestMode)
	projectID := uuid.New()
	sourceID := uuid.New()
	docID := uuid.New()

	docRepo := newMockDocRepo()
	doc := &ent.Document{
		ID:          docID,
		ProjectID:   projectID,
		SourceID:    sourceID,
		FilePath:    "internal/api/server.go",
		Language:    "go",
		ContentHash: "hash123",
		TotalChunks: 2,
	}
	_, _ = docRepo.Create(context.Background(), doc)

	vectorRepo := repository.NewMockVectorRepository()
	chunk1 := &model.DocumentChunk{
		ID:          uuid.New(),
		ProjectID:   projectID,
		DocumentID:  docID,
		ChunkIndex:  0,
		StartLine:   1,
		EndLine:     40,
		Content:     "package api\nfunc Start() {}",
		ContentHash: "h1",
		TokenCount:  20,
		Embedding:   []float32{0.1, 0.2},
		CreatedAt:   time.Now(),
	}
	chunk2 := &model.DocumentChunk{
		ID:          uuid.New(),
		ProjectID:   projectID,
		DocumentID:  docID,
		ChunkIndex:  1,
		StartLine:   41,
		EndLine:     80,
		Content:     "func Stop() {}",
		ContentHash: "h2",
		TokenCount:  15,
		Embedding:   []float32{0.3, 0.4},
		CreatedAt:   time.Now(),
	}
	require.NoError(t, vectorRepo.UpsertChunks(context.Background(), []*model.DocumentChunk{chunk1, chunk2}))

	docHandler := handler.NewDocumentHandler(docRepo, vectorRepo, zap.NewNop())

	router := gin.New()
	router.GET("/projects/:id/documents/:doc_id", docHandler.GetDocument)

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/projects/%s/documents/%s", projectID, docID), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp handler.DocumentDetailResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	assert.Equal(t, docID, resp.ID)
	assert.Equal(t, "internal/api/server.go", resp.FilePath)
	require.Len(t, resp.Chunks, 2, "GetDocument must return the actual chunks indexed in vectorRepo")
	assert.Equal(t, 0, resp.Chunks[0].ChunkIndex)
	assert.Equal(t, "package api\nfunc Start() {}", resp.Chunks[0].Content)
	assert.Equal(t, 1, resp.Chunks[1].ChunkIndex)
	assert.Equal(t, "func Stop() {}", resp.Chunks[1].Content)
}
