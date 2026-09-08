package e2e_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
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
	"github.com/your-org/contextforge/internal/chunk"
	"github.com/your-org/contextforge/internal/ent"
	"github.com/your-org/contextforge/internal/ent/source"
	"github.com/your-org/contextforge/internal/model"
	"github.com/your-org/contextforge/internal/provider"
	"github.com/your-org/contextforge/internal/repository"
	"github.com/your-org/contextforge/internal/retrieval"
	"github.com/your-org/contextforge/internal/service"
)

// In-memory repositories for end-to-end user journey
type inMemProjectRepo struct {
	projects map[uuid.UUID]*ent.Project
}

func newInMemProjectRepo() *inMemProjectRepo {
	return &inMemProjectRepo{projects: make(map[uuid.UUID]*ent.Project)}
}

func (r *inMemProjectRepo) Create(ctx context.Context, p *ent.Project) (*ent.Project, error) {
	if p.ID == uuid.Nil {
		p.ID = uuid.New()
	}
	r.projects[p.ID] = p
	return p, nil
}

func (r *inMemProjectRepo) GetByID(ctx context.Context, id, ownerUserID uuid.UUID) (*ent.Project, error) {
	p, ok := r.projects[id]
	if !ok || p.OwnerUserID != ownerUserID {
		return nil, fmt.Errorf("project not found")
	}
	return p, nil
}

func (r *inMemProjectRepo) ListByOwner(ctx context.Context, ownerUserID uuid.UUID, page, pageSize int) ([]*ent.Project, int, error) {
	var list []*ent.Project
	for _, p := range r.projects {
		if p.OwnerUserID == ownerUserID {
			list = append(list, p)
		}
	}
	return list, len(list), nil
}

func (r *inMemProjectRepo) Update(ctx context.Context, p *ent.Project, ownerUserID uuid.UUID) (*ent.Project, error) {
	existing, ok := r.projects[p.ID]
	if !ok || existing.OwnerUserID != ownerUserID {
		return nil, fmt.Errorf("project not found")
	}
	r.projects[p.ID] = p
	return p, nil
}

func (r *inMemProjectRepo) Delete(ctx context.Context, id, ownerUserID uuid.UUID) error {
	existing, ok := r.projects[id]
	if !ok || existing.OwnerUserID != ownerUserID {
		return fmt.Errorf("project not found")
	}
	delete(r.projects, id)
	return nil
}

type inMemSourceRepo struct {
	sources map[uuid.UUID]*ent.Source
}

func newInMemSourceRepo() *inMemSourceRepo {
	return &inMemSourceRepo{sources: make(map[uuid.UUID]*ent.Source)}
}

func (r *inMemSourceRepo) Create(ctx context.Context, s *ent.Source) (*ent.Source, error) {
	if s.ID == uuid.Nil {
		s.ID = uuid.New()
	}
	r.sources[s.ID] = s
	return s, nil
}

func (r *inMemSourceRepo) GetByID(ctx context.Context, id, projectID uuid.UUID) (*ent.Source, error) {
	s, ok := r.sources[id]
	if !ok || s.ProjectID != projectID {
		return nil, fmt.Errorf("source not found")
	}
	return s, nil
}

func (r *inMemSourceRepo) ListByProjectID(ctx context.Context, projectID uuid.UUID) ([]*ent.Source, error) {
	var list []*ent.Source
	for _, s := range r.sources {
		if s.ProjectID == projectID {
			list = append(list, s)
		}
	}
	return list, nil
}

func (r *inMemSourceRepo) UpdateSyncStatus(ctx context.Context, id, projectID uuid.UUID, status source.SyncStatus, commitHash string, syncedAt *time.Time) (*ent.Source, error) {
	s, ok := r.sources[id]
	if !ok || s.ProjectID != projectID {
		return nil, fmt.Errorf("source not found")
	}
	s.SyncStatus = status
	s.LastCommitHash = commitHash
	s.LastSyncedAt = syncedAt
	return s, nil
}

func (r *inMemSourceRepo) Delete(ctx context.Context, id, projectID uuid.UUID) error {
	s, ok := r.sources[id]
	if !ok || s.ProjectID != projectID {
		return fmt.Errorf("source not found")
	}
	delete(r.sources, id)
	return nil
}

// TestEndToEndUserJourney tests the complete user workflow:
// Login -> Create Project -> Add Source -> Ingest & Chunk Code -> Stream RAG Chat with Citations -> Cleanup
func TestEndToEndUserJourney(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logger := zap.NewNop()
	jwtSecret := []byte("secret_key_32_bytes_super_secure_!")

	// 1. Setup in-memory repositories and providers
	projectRepo := newInMemProjectRepo()
	sourceRepo := newInMemSourceRepo()
	vectorRepo := repository.NewMockVectorRepository()
	embedder := provider.NewMockEmbeddingProvider(768)
	llm := provider.NewMockLLMProvider("The authentication engine uses AES-256-GCM with 12-byte random nonces.")

	hybridRetriever := retrieval.NewHybridRetriever(vectorRepo, embedder)
	ragService := service.NewRAGService(hybridRetriever, llm)

	projectH := handler.NewProjectHandler(projectRepo, logger)
	sourceH := handler.NewSourceHandler(sourceRepo, nil, nil, logger)
	chatH := handler.NewChatHandler(ragService, logger)

	// Setup Router
	router := gin.New()
	router.Use(gin.Recovery())

	api := router.Group("/api/v1")
	api.Use(auth.AuthMiddleware(jwtSecret))
	{
		api.GET("/projects", projectH.ListProjects)
		api.POST("/projects", projectH.CreateProject)

		projectGroup := api.Group("/projects/:id")
		projectGroup.Use(middleware.RequireProjectAccess(projectRepo))
		{
			projectGroup.GET("", projectH.GetProject)
			projectGroup.PATCH("", projectH.UpdateProject)
			projectGroup.DELETE("", projectH.DeleteProject)

			projectGroup.GET("/sources", sourceH.ListSources)
			projectGroup.POST("/sources", sourceH.CreateSource)
			projectGroup.DELETE("/sources/:source_id", sourceH.DeleteSource)

			projectGroup.POST("/chat/completions/stream", chatH.StreamChatCompletions)
		}
	}

	// 2. Authenticate User
	userID := uuid.New()
	token, err := auth.GenerateToken(userID, "testuser", jwtSecret, time.Hour)
	require.NoError(t, err)
	authHeader := "Bearer " + token

	// 3. Step 1: Create Project
	createProjPayload := map[string]interface{}{
		"name":                "ContextForge Core",
		"description":         "High performance codebase context engine",
		"embedding_provider":  "ollama",
		"embedding_model":     "nomic-embed-text",
		"embedding_dimension": 768,
		"llm_provider":        "openai",
		"llm_model":           "gpt-4o",
	}
	bodyBytes, _ := json.Marshal(createProjPayload)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects", bytes.NewReader(bodyBytes))
	req.Header.Set("Authorization", authHeader)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code)
	var createdProject ent.Project
	err = json.Unmarshal(w.Body.Bytes(), &createdProject)
	require.NoError(t, err)
	assert.Equal(t, "ContextForge Core", createdProject.Name)
	assert.Equal(t, userID, createdProject.OwnerUserID)
	projectID := createdProject.ID

	// 4. Step 2: Add GitHub Source Repository
	createSourcePayload := map[string]interface{}{
		"name":       "contextforge-repo",
		"type":       "github",
		"repo_owner": "testorg",
		"repo_name":  "contextforge",
		"branch":     "main",
	}
	sBodyBytes, _ := json.Marshal(createSourcePayload)
	reqSource := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/projects/%s/sources", projectID), bytes.NewReader(sBodyBytes))
	reqSource.Header.Set("Authorization", authHeader)
	reqSource.Header.Set("Content-Type", "application/json")
	wSource := httptest.NewRecorder()
	router.ServeHTTP(wSource, reqSource)

	require.Equal(t, http.StatusCreated, wSource.Code)
	var createdSource ent.Source
	err = json.Unmarshal(wSource.Body.Bytes(), &createdSource)
	require.NoError(t, err)
	assert.Equal(t, "contextforge-repo", createdSource.Name)
	sourceID := createdSource.ID

	// 5. Step 3: Chunk and Ingest Code Files
	codeFileContent := `package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
)

// Encrypt performs AES-256-GCM encryption on the provided plaintext.
func Encrypt(plaintext []byte, key []byte) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, 12)
	rand.Read(nonce)
	return string(gcm.Seal(nonce, nonce, plaintext, nil)), nil
}
`
	chunker := chunk.NewChunker(chunk.DefaultOptions())
	chunks := chunker.ChunkText(codeFileContent, "go")
	require.NotEmpty(t, chunks)

	// Embed chunks and index into vector repo
	docID := uuid.New()
	var docChunks []*model.DocumentChunk
	for _, c := range chunks {
		vec, err := embedder.EmbedQuery(context.Background(), c.Content)
		require.NoError(t, err)

		chunkID := uuid.New()
		docChunks = append(docChunks, &model.DocumentChunk{
			ID:          chunkID,
			ProjectID:   projectID,
			DocumentID:  docID,
			ChunkIndex:  c.Index,
			StartLine:   c.StartLine,
			EndLine:     c.EndLine,
			Content:     c.Content,
			ContentHash: "hash_123",
			TokenCount:  c.TokenCount,
			Embedding:   vec,
			CreatedAt:   time.Now(),
		})
		vectorRepo.SetChunkMetadata(chunkID, "crypto/encrypt.go", sourceID)
	}

	err = vectorRepo.UpsertChunks(context.Background(), docChunks)
	require.NoError(t, err)

	// 6. Step 4: Execute Streaming RAG Chat Request
	chatPayload := map[string]interface{}{
		"message":              "How does encryption work in this codebase?",
		"top_k":                5,
		"similarity_threshold": 0.0,
	}
	cBodyBytes, _ := json.Marshal(chatPayload)
	reqChat := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/projects/%s/chat/completions/stream", projectID), bytes.NewReader(cBodyBytes))
	reqChat.Header.Set("Authorization", authHeader)
	reqChat.Header.Set("Content-Type", "application/json")
	wChat := httptest.NewRecorder()
	router.ServeHTTP(wChat, reqChat)

	require.Equal(t, http.StatusOK, wChat.Code)
	assert.Contains(t, wChat.Header().Get("Content-Type"), "text/event-stream")

	// Parse SSE Stream Events
	scanner := bufio.NewScanner(wChat.Body)
	var events []string
	var citationsFound int
	var messageTokens []string
	var doneFound bool

	var currentEvent string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "event: ") {
			currentEvent = strings.TrimPrefix(line, "event: ")
			events = append(events, currentEvent)
		} else if strings.HasPrefix(line, "data: ") {
			data := strings.TrimPrefix(line, "data: ")
			switch currentEvent {
			case "citation":
				citationsFound++
				assert.Contains(t, data, "crypto/encrypt.go")
			case "message":
				var msg model.ChatStreamMessageEvent
				if err := json.Unmarshal([]byte(data), &msg); err == nil {
					messageTokens = append(messageTokens, msg.Delta)
				}
			case "done":
				doneFound = true
			}
		}
	}

	assert.Contains(t, events, "citation")
	assert.Contains(t, events, "message")
	assert.Contains(t, events, "done")
	assert.Greater(t, citationsFound, 0, "must receive at least one structured citation")
	assert.NotEmpty(t, messageTokens, "must receive streamed tokens")
	assert.True(t, doneFound, "must receive done event")

	// 7. Step 5: Verify Cross-Project Isolation
	otherUser := uuid.New()
	otherToken, _ := auth.GenerateToken(otherUser, "otheruser", jwtSecret, time.Hour)
	reqCross := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/projects/%s", projectID), nil)
	reqCross.Header.Set("Authorization", "Bearer "+otherToken)
	wCross := httptest.NewRecorder()
	router.ServeHTTP(wCross, reqCross)

	// Other user cannot access Project -> 404 Cloaked
	assert.Equal(t, http.StatusNotFound, wCross.Code)

	// 8. Step 6: Cleanup
	reqDelSource := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/projects/%s/sources/%s", projectID, sourceID), nil)
	reqDelSource.Header.Set("Authorization", authHeader)
	wDelSource := httptest.NewRecorder()
	router.ServeHTTP(wDelSource, reqDelSource)
	assert.Equal(t, http.StatusNoContent, wDelSource.Code)

	reqDelProj := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/projects/%s", projectID), nil)
	reqDelProj.Header.Set("Authorization", authHeader)
	wDelProj := httptest.NewRecorder()
	router.ServeHTTP(wDelProj, reqDelProj)
	assert.Equal(t, http.StatusNoContent, wDelProj.Code)
}
