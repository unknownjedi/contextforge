package handler_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/your-org/contextforge/internal/api/handler"
	"github.com/your-org/contextforge/internal/model"
)

func setupChatWithConversationRouter(ragSvc handler.RAGService, convRepo *mockConversationRepo) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := handler.NewChatHandler(ragSvc, zap.NewNop())
	h.SetConversationRepository(convRepo)
	h.RegisterRoutes(r)
	return r
}

func TestChatHandler_ConversationPersistence_Stream(t *testing.T) {
	projectID := uuid.New()
	mockSvc := &mockRAGService{}
	repo := newMockConversationRepo()
	router := setupChatWithConversationRouter(mockSvc, repo)

	t.Run("Streams and persists user query and assistant response, updating default title", func(t *testing.T) {
		conv, err := repo.CreateConversation(context.Background(), projectID, "New Chat")
		require.NoError(t, err)

		prompt := "What is the security architecture of the authentication service?"
		reqBody := fmt.Sprintf(`{
			"message": %q,
			"conversation_id": %q
		}`, prompt, conv.ID)

		req := httptest.NewRequest(
			http.MethodPost,
			fmt.Sprintf("/projects/%s/chat/completions/stream", projectID),
			strings.NewReader(reqBody),
		)
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		// Check conversation title was auto-updated to first 40 runes of prompt
		updatedConv, err := repo.GetByID(context.Background(), conv.ID, projectID)
		require.NoError(t, err)
		expectedTitle := string([]rune(prompt)[:40])
		assert.Equal(t, expectedTitle, updatedConv.Title)

		// Check messages persisted in conversation
		msgs, err := repo.ListMessages(context.Background(), conv.ID)
		require.NoError(t, err)
		require.Len(t, msgs, 2)

		// User message
		assert.Equal(t, "user", msgs[0].Role)
		assert.Equal(t, prompt, msgs[0].Content)

		// Assistant message
		assert.Equal(t, "assistant", msgs[1].Role)
		assert.Equal(t, "ContextForge isolates data per project.", msgs[1].Content)
		require.Len(t, msgs[1].Citations, 1)
		assert.Equal(t, "internal/crypto/encrypt.go", msgs[1].Citations[0].FilePath)
		assert.Equal(t, 50, msgs[1].TokensUsed)
		assert.Equal(t, int64(25), msgs[1].DurationMs)
	})

	t.Run("Does not overwrite custom conversation title", func(t *testing.T) {
		customTitle := "My Custom Discussion"
		conv, err := repo.CreateConversation(context.Background(), projectID, customTitle)
		require.NoError(t, err)

		prompt := "Tell me about project isolation in multitenancy"
		reqBody := fmt.Sprintf(`{
			"message": %q,
			"conversation_id": %q
		}`, prompt, conv.ID)

		req := httptest.NewRequest(
			http.MethodPost,
			fmt.Sprintf("/projects/%s/chat/completions/stream", projectID),
			strings.NewReader(reqBody),
		)
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		updatedConv, err := repo.GetByID(context.Background(), conv.ID, projectID)
		require.NoError(t, err)
		assert.Equal(t, customTitle, updatedConv.Title)
	})

	t.Run("Non-existent conversation ID returns 404", func(t *testing.T) {
		nonExistentID := uuid.New()
		reqBody := fmt.Sprintf(`{
			"message": "hello",
			"conversation_id": %q
		}`, nonExistentID)

		req := httptest.NewRequest(
			http.MethodPost,
			fmt.Sprintf("/projects/%s/chat/completions/stream", projectID),
			strings.NewReader(reqBody),
		)
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
		assert.Contains(t, w.Body.String(), "conversation not found")
	})
}

func TestChatHandler_ConversationPersistence_Sync(t *testing.T) {
	projectID := uuid.New()
	mockSvc := &mockRAGService{}
	repo := newMockConversationRepo()
	router := setupChatWithConversationRouter(mockSvc, repo)

	t.Run("Synchronous completions endpoint persists messages and updates title", func(t *testing.T) {
		conv, err := repo.CreateConversation(context.Background(), projectID, "New Chat")
		require.NoError(t, err)

		prompt := "Short prompt"
		reqBody := fmt.Sprintf(`{
			"message": %q,
			"conversation_id": %q
		}`, prompt, conv.ID)

		req := httptest.NewRequest(
			http.MethodPost,
			fmt.Sprintf("/projects/%s/chat/completions", projectID),
			strings.NewReader(reqBody),
		)
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp model.ChatResponse
		err = json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)

		// Title should be the prompt itself since len <= 40
		updatedConv, err := repo.GetByID(context.Background(), conv.ID, projectID)
		require.NoError(t, err)
		assert.Equal(t, prompt, updatedConv.Title)

		// Messages should be stored
		msgs, err := repo.ListMessages(context.Background(), conv.ID)
		require.NoError(t, err)
		require.Len(t, msgs, 2)
		assert.Equal(t, "user", msgs[0].Role)
		assert.Equal(t, prompt, msgs[0].Content)
		assert.Equal(t, "assistant", msgs[1].Role)
		assert.Equal(t, resp.Answer, msgs[1].Content)
		assert.Equal(t, 42, msgs[1].TokensUsed)
		assert.Equal(t, int64(15), msgs[1].DurationMs)
	})

	t.Run("Non-existent conversation ID in sync completion returns 404", func(t *testing.T) {
		nonExistentID := uuid.New()
		reqBody := fmt.Sprintf(`{
			"message": "hello",
			"conversation_id": %q
		}`, nonExistentID)

		req := httptest.NewRequest(
			http.MethodPost,
			fmt.Sprintf("/projects/%s/chat/completions", projectID),
			strings.NewReader(reqBody),
		)
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
		assert.Contains(t, w.Body.String(), "conversation not found")
	})
}
