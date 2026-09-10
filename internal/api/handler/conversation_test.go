package handler_test

import (
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
	"github.com/your-org/contextforge/internal/ent"
	"github.com/your-org/contextforge/internal/model"
)

type mockConversationRepo struct {
	conversations map[uuid.UUID]*ent.Conversation
	messages      map[uuid.UUID][]*ent.ChatMessage
}

func newMockConversationRepo() *mockConversationRepo {
	return &mockConversationRepo{
		conversations: make(map[uuid.UUID]*ent.Conversation),
		messages:      make(map[uuid.UUID][]*ent.ChatMessage),
	}
}

func (m *mockConversationRepo) CreateConversation(ctx context.Context, projectID uuid.UUID, title string) (*ent.Conversation, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		title = "New Chat"
	}
	id := uuid.New()
	conv := &ent.Conversation{
		ID:        id,
		ProjectID: projectID,
		Title:     title,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	m.conversations[id] = conv
	return conv, nil
}

func (m *mockConversationRepo) ListByProjectID(ctx context.Context, projectID uuid.UUID) ([]*ent.Conversation, error) {
	var list []*ent.Conversation
	for _, c := range m.conversations {
		if c.ProjectID == projectID {
			list = append(list, c)
		}
	}
	return list, nil
}

func (m *mockConversationRepo) GetByID(ctx context.Context, convID uuid.UUID, projectID uuid.UUID) (*ent.Conversation, error) {
	conv, ok := m.conversations[convID]
	if !ok || conv.ProjectID != projectID {
		return nil, fmt.Errorf("conversation not found")
	}
	// Copy and populate Edges.Messages
	copyConv := *conv
	copyConv.Edges.Messages = m.messages[convID]
	return &copyConv, nil
}

func (m *mockConversationRepo) UpdateTitle(ctx context.Context, convID uuid.UUID, projectID uuid.UUID, title string) (*ent.Conversation, error) {
	conv, ok := m.conversations[convID]
	if !ok || conv.ProjectID != projectID {
		return nil, fmt.Errorf("conversation not found")
	}
	title = strings.TrimSpace(title)
	if title == "" {
		title = "New Chat"
	}
	conv.Title = title
	conv.UpdatedAt = time.Now()
	return conv, nil
}

func (m *mockConversationRepo) Delete(ctx context.Context, convID uuid.UUID, projectID uuid.UUID) error {
	conv, ok := m.conversations[convID]
	if !ok || conv.ProjectID != projectID {
		return fmt.Errorf("conversation not found or unauthorized")
	}
	delete(m.conversations, convID)
	delete(m.messages, convID)
	return nil
}

func (m *mockConversationRepo) CreateMessage(ctx context.Context, convID uuid.UUID, role, content string, citations []model.Citation, tokensUsed int, durationMs int64) (*ent.ChatMessage, error) {
	if _, ok := m.conversations[convID]; !ok {
		return nil, fmt.Errorf("conversation not found")
	}
	if citations == nil {
		citations = []model.Citation{}
	}
	msg := &ent.ChatMessage{
		ID:             uuid.New(),
		ConversationID: convID,
		Role:           role,
		Content:        content,
		Citations:      citations,
		TokensUsed:     tokensUsed,
		DurationMs:     durationMs,
		CreatedAt:      time.Now(),
	}
	m.messages[convID] = append(m.messages[convID], msg)
	m.conversations[convID].UpdatedAt = time.Now()
	return msg, nil
}

func (m *mockConversationRepo) ListMessages(ctx context.Context, convID uuid.UUID) ([]*ent.ChatMessage, error) {
	msgs, ok := m.messages[convID]
	if !ok {
		return []*ent.ChatMessage{}, nil
	}
	return msgs, nil
}

func setupConversationRouter(repo *mockConversationRepo) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := handler.NewConversationHandler(repo, zap.NewNop())

	r.GET("/projects/:id/conversations", h.ListConversations)
	r.POST("/projects/:id/conversations", h.CreateConversation)
	r.GET("/projects/:id/conversations/:conv_id", h.GetConversation)
	r.DELETE("/projects/:id/conversations/:conv_id", h.DeleteConversation)

	return r
}

func TestConversationHandler(t *testing.T) {
	repo := newMockConversationRepo()
	router := setupConversationRouter(repo)
	projectID := uuid.New()

	t.Run("CreateConversation with explicit title returns 201", func(t *testing.T) {
		body := `{"title": "Auth Architecture Discussion"}`
		req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/projects/%s/conversations", projectID), strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusCreated, w.Code)
		var conv ent.Conversation
		err := json.Unmarshal(w.Body.Bytes(), &conv)
		require.NoError(t, err)
		assert.Equal(t, "Auth Architecture Discussion", conv.Title)
		assert.Equal(t, projectID, conv.ProjectID)
	})

	t.Run("CreateConversation with empty title defaults to 'New Chat'", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/projects/%s/conversations", projectID), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusCreated, w.Code)
		var conv ent.Conversation
		err := json.Unmarshal(w.Body.Bytes(), &conv)
		require.NoError(t, err)
		assert.Equal(t, "New Chat", conv.Title)
	})

	t.Run("CreateConversation with invalid project ID returns 400", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/projects/invalid-uuid/conversations", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("ListConversations returns project conversations", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/projects/%s/conversations", projectID), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		var list []*ent.Conversation
		err := json.Unmarshal(w.Body.Bytes(), &list)
		require.NoError(t, err)
		assert.Len(t, list, 2)
	})

	t.Run("ListConversations with invalid project ID returns 400", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/projects/not-uuid/conversations", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("GetConversation returns details and messages", func(t *testing.T) {
		conv, err := repo.CreateConversation(context.Background(), projectID, "Detailed Conversation")
		require.NoError(t, err)

		_, err = repo.CreateMessage(context.Background(), conv.ID, "user", "What is JWT?", nil, 5, 0)
		require.NoError(t, err)
		_, err = repo.CreateMessage(context.Background(), conv.ID, "assistant", "JWT is a JSON Web Token.", []model.Citation{
			{FilePath: "internal/auth/jwt.go", StartLine: 1, EndLine: 10},
		}, 15, 45)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/projects/%s/conversations/%s", projectID, conv.ID), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		var detail handler.ConversationDetailResponse
		err = json.Unmarshal(w.Body.Bytes(), &detail)
		require.NoError(t, err)
		assert.Equal(t, conv.ID, detail.ID)
		assert.Equal(t, "Detailed Conversation", detail.Title)
		require.Len(t, detail.Messages, 2)
		assert.Equal(t, "user", detail.Messages[0].Role)
		assert.Equal(t, "What is JWT?", detail.Messages[0].Content)
		assert.Equal(t, "assistant", detail.Messages[1].Role)
		assert.Equal(t, "JWT is a JSON Web Token.", detail.Messages[1].Content)
		require.Len(t, detail.Messages[1].Citations, 1)
		assert.Equal(t, "internal/auth/jwt.go", detail.Messages[1].Citations[0].FilePath)
	})

	t.Run("GetConversation non-existent returns 404", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/projects/%s/conversations/%s", projectID, uuid.New()), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("GetConversation invalid conv_id returns 400", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/projects/%s/conversations/not-a-uuid", projectID), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("DeleteConversation returns 204", func(t *testing.T) {
		conv, err := repo.CreateConversation(context.Background(), projectID, "To Delete")
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/projects/%s/conversations/%s", projectID, conv.ID), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNoContent, w.Code)

		// Subsequent get returns 404
		reqGet := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/projects/%s/conversations/%s", projectID, conv.ID), nil)
		wGet := httptest.NewRecorder()
		router.ServeHTTP(wGet, reqGet)
		assert.Equal(t, http.StatusNotFound, wGet.Code)
	})

	t.Run("DeleteConversation non-existent returns 404", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/projects/%s/conversations/%s", projectID, uuid.New()), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}
