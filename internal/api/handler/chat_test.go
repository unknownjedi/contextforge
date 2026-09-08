package handler_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
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

type mockRAGService struct {
	customGenerate   func(ctx context.Context, projectID uuid.UUID, req *model.ChatRequest) (*model.ChatResponse, error)
	customStreamChat func(ctx context.Context, projectID uuid.UUID, req *model.ChatRequest, onCitation func(*model.Citation) error, onToken func(string) error) (*model.ChatResponse, error)
}

func (m *mockRAGService) Generate(ctx context.Context, projectID uuid.UUID, req *model.ChatRequest) (*model.ChatResponse, error) {
	if m.customGenerate != nil {
		return m.customGenerate(ctx, projectID, req)
	}
	return &model.ChatResponse{
		Answer: "ContextForge isolates data per project [1].",
		Citations: []model.Citation{
			{
				SourceID:   uuid.New(),
				FilePath:   "internal/crypto/encrypt.go",
				StartLine:  12,
				EndLine:    35,
				Similarity: 0.884,
				Snippet:    "func Encrypt()",
			},
		},
		TokensUsed: 42,
		DurationMs: 15,
	}, nil
}

func (m *mockRAGService) StreamChat(
	ctx context.Context,
	projectID uuid.UUID,
	req *model.ChatRequest,
	onCitation func(*model.Citation) error,
	onToken func(string) error,
) (*model.ChatResponse, error) {
	if m.customStreamChat != nil {
		return m.customStreamChat(ctx, projectID, req, onCitation, onToken)
	}

	citation := &model.Citation{
		SourceID:   uuid.New(),
		FilePath:   "internal/crypto/encrypt.go",
		StartLine:  12,
		EndLine:    35,
		Similarity: 0.884,
		Snippet:    "func Encrypt()",
	}

	if onCitation != nil {
		if err := onCitation(citation); err != nil {
			return nil, err
		}
	}

	tokens := []string{"ContextForge ", "isolates ", "data ", "per ", "project."}
	for _, tok := range tokens {
		if onToken != nil {
			if err := onToken(tok); err != nil {
				return nil, err
			}
		}
	}

	return &model.ChatResponse{
		Answer:     "ContextForge isolates data per project.",
		Citations:  []model.Citation{*citation},
		TokensUsed: 50,
		DurationMs: 25,
	}, nil
}

func setupChatRouter(svc handler.RAGService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := handler.NewChatHandler(svc, zap.NewNop())
	h.RegisterRoutes(r)
	return r
}

func TestChatHandler_StreamChatCompletions(t *testing.T) {
	projectID := uuid.New()
	mockSvc := &mockRAGService{}
	router := setupChatRouter(mockSvc)

	t.Run("Streams citations, messages, and done events matching OpenAPI spec", func(t *testing.T) {
		reqBody := `{"message": "How is encryption implemented?", "top_k": 3}`
		req := httptest.NewRequest(
			http.MethodPost,
			fmt.Sprintf("/projects/%s/chat/completions/stream", projectID),
			strings.NewReader(reqBody),
		)
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Header().Get("Content-Type"), "text/event-stream")
		assert.Equal(t, "no-cache", w.Header().Get("Cache-Control"))

		// Parse SSE response
		scanner := bufio.NewScanner(w.Body)
		var events []struct {
			Event string
			Data  string
		}

		var currentEvent, currentData string
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "event: ") {
				currentEvent = strings.TrimPrefix(line, "event: ")
			} else if strings.HasPrefix(line, "data: ") {
				currentData = strings.TrimPrefix(line, "data: ")
			} else if line == "" && currentEvent != "" {
				events = append(events, struct {
					Event string
					Data  string
				}{Event: currentEvent, Data: currentData})
				currentEvent = ""
				currentData = ""
			}
		}

		require.NotEmpty(t, events)

		// 1. First event should be citation
		assert.Equal(t, "citation", events[0].Event)
		var cit model.Citation
		err := json.Unmarshal([]byte(events[0].Data), &cit)
		require.NoError(t, err)
		assert.Equal(t, "internal/crypto/encrypt.go", cit.FilePath)
		assert.Equal(t, 12, cit.StartLine)
		assert.Equal(t, 35, cit.EndLine)
		assert.InDelta(t, float32(0.884), cit.Similarity, 1e-4)

		// 2. Middle events should be message deltas
		var messageEvents []string
		for _, ev := range events[1 : len(events)-1] {
			assert.Equal(t, "message", ev.Event)
			var msgEv model.ChatStreamMessageEvent
			err := json.Unmarshal([]byte(ev.Data), &msgEv)
			require.NoError(t, err)
			messageEvents = append(messageEvents, msgEv.Delta)
		}
		accumulated := strings.Join(messageEvents, "")
		assert.Equal(t, "ContextForge isolates data per project.", accumulated)

		// 3. Final event should be done
		lastEv := events[len(events)-1]
		assert.Equal(t, "done", lastEv.Event)
		var doneEv model.ChatStreamDoneEvent
		err = json.Unmarshal([]byte(lastEv.Data), &doneEv)
		require.NoError(t, err)
		assert.Equal(t, 50, doneEv.TotalTokens)
		assert.Equal(t, int64(25), doneEv.DurationMs)
	})

	t.Run("API v1 prefix route works identically", func(t *testing.T) {
		reqBody := `{"message": "test query"}`
		req := httptest.NewRequest(
			http.MethodPost,
			fmt.Sprintf("/api/v1/projects/%s/chat/completions/stream", projectID),
			strings.NewReader(reqBody),
		)
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Header().Get("Content-Type"), "text/event-stream")
	})

	t.Run("Invalid project UUID returns 400 Bad Request", func(t *testing.T) {
		req := httptest.NewRequest(
			http.MethodPost,
			"/projects/not-a-valid-uuid/chat/completions/stream",
			strings.NewReader(`{"message": "hi"}`),
		)
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "invalid project ID format")
	})

	t.Run("Missing message returns 400 Bad Request", func(t *testing.T) {
		req := httptest.NewRequest(
			http.MethodPost,
			fmt.Sprintf("/projects/%s/chat/completions/stream", projectID),
			strings.NewReader(`{"message": "   "}`),
		)
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "message cannot be empty")
	})

	t.Run("Malformed JSON payload returns 400 Bad Request", func(t *testing.T) {
		req := httptest.NewRequest(
			http.MethodPost,
			fmt.Sprintf("/projects/%s/chat/completions/stream", projectID),
			bytes.NewReader([]byte("{invalid-json}")),
		)
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("RAGService stream failure emits error event in stream", func(t *testing.T) {
		failingSvc := &mockRAGService{
			customStreamChat: func(ctx context.Context, projectID uuid.UUID, req *model.ChatRequest, onCitation func(*model.Citation) error, onToken func(string) error) (*model.ChatResponse, error) {
				return nil, errors.New("upstream LLM timeout")
			},
		}
		failRouter := setupChatRouter(failingSvc)

		req := httptest.NewRequest(
			http.MethodPost,
			fmt.Sprintf("/projects/%s/chat/completions/stream", projectID),
			strings.NewReader(`{"message": "test"}`),
		)
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		failRouter.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), "event: error")
		assert.Contains(t, w.Body.String(), "upstream LLM timeout")
	})
}

func TestChatHandler_ChatCompletionsSync(t *testing.T) {
	projectID := uuid.New()
	mockSvc := &mockRAGService{}
	router := setupChatRouter(mockSvc)

	t.Run("Synchronous completions endpoint returns ChatCompletionResponse JSON", func(t *testing.T) {
		reqBody := `{"message": "How does project isolation work?"}`
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
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)

		assert.Contains(t, resp.Answer, "ContextForge isolates data")
		require.Len(t, resp.Citations, 1)
		assert.Equal(t, "internal/crypto/encrypt.go", resp.Citations[0].FilePath)
		assert.Equal(t, 42, resp.TokensUsed)
		assert.Equal(t, int64(15), resp.DurationMs)
	})
}
