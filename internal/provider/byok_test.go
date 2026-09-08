package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/your-org/contextforge/internal/model"
)

// fastRetryConfig returns a RetryConfig configured for sub-millisecond retries during testing.
func fastRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:     3,
		InitialBackoff: 1 * time.Millisecond,
		MaxBackoff:     10 * time.Millisecond,
	}
}

// -----------------------------------------------------------------------------
// OpenAI Tests
// -----------------------------------------------------------------------------

func TestOpenAIProvider_GenerateCompletion(t *testing.T) {
	apiKey := "sk-test-key-12345"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/chat/completions" {
			t.Errorf("expected /chat/completions, got %s", r.URL.Path)
		}
		if auth := r.Header.Get("Authorization"); auth != "Bearer "+apiKey {
			t.Errorf("expected Authorization header Bearer %s, got %s", apiKey, auth)
		}

		var reqBody openAIChatRequest
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			t.Errorf("failed to decode request body: %v", err)
		}

		if reqBody.Model != "gpt-4o-mini" {
			t.Errorf("expected model gpt-4o-mini, got %s", reqBody.Model)
		}
		if len(reqBody.Messages) != 1 || reqBody.Messages[0].Content != "Hello OpenAI" {
			t.Errorf("unexpected messages: %+v", reqBody.Messages)
		}

		resp := openAIChatResponse{
			ID:    "chatcmpl-test",
			Model: "gpt-4o-mini",
			Choices: []struct {
				Index        int               `json:"index"`
				Message      model.ChatMessage `json:"message"`
				FinishReason string            `json:"finish_reason"`
			}{
				{
					Index:        0,
					Message:      model.ChatMessage{Role: "assistant", Content: "Hello from OpenAI!"},
					FinishReason: "stop",
				},
			},
			Usage: struct {
				PromptTokens     int `json:"prompt_tokens"`
				CompletionTokens int `json:"completion_tokens"`
				TotalTokens      int `json:"total_tokens"`
			}{
				PromptTokens:     10,
				CompletionTokens: 15,
				TotalTokens:      25,
			},
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider := NewOpenAIProvider(OpenAIConfig{
		APIKey:     apiKey,
		BaseURL:    server.URL,
		Retry:      fastRetryConfig(),
		HTTPClient: server.Client(),
	})

	ctx := context.Background()
	req := &model.CompletionRequest{
		Messages: []model.ChatMessage{{Role: "user", Content: "Hello OpenAI"}},
		Model:    "gpt-4o-mini",
	}

	resp, err := provider.GenerateCompletion(ctx, req)
	if err != nil {
		t.Fatalf("GenerateCompletion failed: %v", err)
	}

	if resp.Content != "Hello from OpenAI!" {
		t.Errorf("expected content 'Hello from OpenAI!', got %q", resp.Content)
	}
	if resp.Model != "gpt-4o-mini" {
		t.Errorf("expected model 'gpt-4o-mini', got %q", resp.Model)
	}
	if resp.TokensUsed != 25 {
		t.Errorf("expected 25 tokens used, got %d", resp.TokensUsed)
	}
}

func TestOpenAIProvider_StreamCompletion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("expected http.Flusher")
		}

		events := []string{
			`data: {"id":"1","model":"gpt-4o-mini","choices":[{"index":0,"delta":{"content":"Hello"}}]}`,
			`data: {"id":"1","model":"gpt-4o-mini","choices":[{"index":0,"delta":{"content":" streaming"}}]}`,
			`data: {"id":"1","model":"gpt-4o-mini","choices":[{"index":0,"delta":{"content":" world!"}}],"usage":{"total_tokens":30}}`,
			`data: [DONE]`,
		}

		for _, event := range events {
			_, _ = fmt.Fprintf(w, "%s\n\n", event)
			flusher.Flush()
		}
	}))
	defer server.Close()

	provider := NewOpenAIProvider(OpenAIConfig{
		BaseURL:    server.URL,
		Retry:      fastRetryConfig(),
		HTTPClient: server.Client(),
	})

	var receivedChunks []string
	var doneReceived bool

	resp, err := provider.StreamCompletion(context.Background(), &model.CompletionRequest{
		Messages: []model.ChatMessage{{Role: "user", Content: "stream me"}},
	}, func(chunk *model.StreamChunk) error {
		if chunk.Done {
			doneReceived = true
		} else {
			receivedChunks = append(receivedChunks, chunk.Delta)
		}
		return nil
	})

	if err != nil {
		t.Fatalf("StreamCompletion failed: %v", err)
	}
	if !doneReceived {
		t.Error("expected done chunk")
	}
	expectedFull := "Hello streaming world!"
	if resp.Content != expectedFull {
		t.Errorf("expected accumulated content %q, got %q", expectedFull, resp.Content)
	}
	if strings.Join(receivedChunks, "") != expectedFull {
		t.Errorf("expected joined chunks %q, got %q", expectedFull, strings.Join(receivedChunks, ""))
	}
	if resp.TokensUsed != 30 {
		t.Errorf("expected 30 tokens, got %d", resp.TokensUsed)
	}
}

func TestOpenAIProvider_BackoffRetryOn429(t *testing.T) {
	var attempts int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		current := atomic.AddInt32(&attempts, 1)
		if current <= 2 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error": {"message": "Rate limit reached"}}`))
			return
		}

		resp := openAIChatResponse{
			ID:    "ok",
			Model: "gpt-4o-mini",
			Choices: []struct {
				Index        int               `json:"index"`
				Message      model.ChatMessage `json:"message"`
				FinishReason string            `json:"finish_reason"`
			}{
				{Index: 0, Message: model.ChatMessage{Content: "recovered after rate limit"}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider := NewOpenAIProvider(OpenAIConfig{
		BaseURL:    server.URL,
		Retry:      fastRetryConfig(),
		HTTPClient: server.Client(),
	})

	resp, err := provider.GenerateCompletion(context.Background(), &model.CompletionRequest{
		Messages: []model.ChatMessage{{Role: "user", Content: "test"}},
	})
	if err != nil {
		t.Fatalf("expected success after retry, got: %v", err)
	}
	if resp.Content != "recovered after rate limit" {
		t.Errorf("unexpected content: %q", resp.Content)
	}
	if atomic.LoadInt32(&attempts) != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

func TestOpenAIProvider_MaxRetriesExceeded(t *testing.T) {
	var attempts int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error": "internal server error"}`))
	}))
	defer server.Close()

	provider := NewOpenAIProvider(OpenAIConfig{
		BaseURL:    server.URL,
		Retry:      fastRetryConfig(),
		HTTPClient: server.Client(),
	})

	_, err := provider.GenerateCompletion(context.Background(), &model.CompletionRequest{
		Messages: []model.ChatMessage{{Role: "user", Content: "test"}},
	})
	if err == nil {
		t.Fatal("expected error after max retries exceeded, got nil")
	}
	if atomic.LoadInt32(&attempts) != 4 { // Initial attempt + 3 retries
		t.Errorf("expected 4 total attempts, got %d", attempts)
	}
}

// -----------------------------------------------------------------------------
// Anthropic Tests
// -----------------------------------------------------------------------------

func TestAnthropicProvider_GenerateCompletion(t *testing.T) {
	apiKey := "sk-ant-test-key"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/messages" {
			t.Errorf("unexpected method/path: %s %s", r.Method, r.URL.Path)
		}
		if key := r.Header.Get("x-api-key"); key != apiKey {
			t.Errorf("expected x-api-key %s, got %s", apiKey, key)
		}
		if v := r.Header.Get("anthropic-version"); v != "2023-06-01" {
			t.Errorf("expected anthropic-version 2023-06-01, got %s", v)
		}

		var reqBody anthropicRequest
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			t.Fatalf("failed to decode anthropic request: %v", err)
		}

		if reqBody.System != "You are ContextForge agent." {
			t.Errorf("expected system prompt, got %q", reqBody.System)
		}
		if len(reqBody.Messages) != 1 || reqBody.Messages[0].Content != "Hello Claude" {
			t.Errorf("unexpected messages: %+v", reqBody.Messages)
		}

		resp := anthropicResponse{
			ID:    "msg_test",
			Type:  "message",
			Role:  "assistant",
			Model: "claude-3-5-sonnet-20241022",
			Content: []anthropicContentBlock{
				{Type: "text", Text: "Hello from Claude!"},
			},
			StopReason: "end_turn",
			Usage: anthropicUsage{
				InputTokens:  12,
				OutputTokens: 18,
			},
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider := NewAnthropicProvider(AnthropicConfig{
		APIKey:     apiKey,
		BaseURL:    server.URL,
		Retry:      fastRetryConfig(),
		HTTPClient: server.Client(),
	})

	req := &model.CompletionRequest{
		Messages: []model.ChatMessage{
			{Role: model.RoleSystem, Content: "You are ContextForge agent."},
			{Role: model.RoleUser, Content: "Hello Claude"},
		},
		Model: "claude-3-5-sonnet-20241022",
	}

	resp, err := provider.GenerateCompletion(context.Background(), req)
	if err != nil {
		t.Fatalf("GenerateCompletion failed: %v", err)
	}

	if resp.Content != "Hello from Claude!" {
		t.Errorf("expected 'Hello from Claude!', got %q", resp.Content)
	}
	if resp.TokensUsed != 30 {
		t.Errorf("expected 30 total tokens, got %d", resp.TokensUsed)
	}
}

func TestAnthropicProvider_StreamCompletion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("expected http.Flusher")
		}

		events := []string{
			"event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_1\",\"model\":\"claude-3-5-sonnet-20241022\",\"usage\":{\"input_tokens\":10,\"output_tokens\":0}}}\n\n",
			"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"ContextForge\"}}\n\n",
			"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\" Rocks!\"}}\n\n",
			"event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":20}}\n\n",
			"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n",
		}

		for _, e := range events {
			_, _ = fmt.Fprint(w, e)
			flusher.Flush()
		}
	}))
	defer server.Close()

	provider := NewAnthropicProvider(AnthropicConfig{
		BaseURL:    server.URL,
		Retry:      fastRetryConfig(),
		HTTPClient: server.Client(),
	})

	var chunks []string
	var doneReceived bool

	resp, err := provider.StreamCompletion(context.Background(), &model.CompletionRequest{
		Messages: []model.ChatMessage{{Role: "user", Content: "Stream test"}},
	}, func(chunk *model.StreamChunk) error {
		if chunk.Done {
			doneReceived = true
		} else {
			chunks = append(chunks, chunk.Delta)
		}
		return nil
	})

	if err != nil {
		t.Fatalf("StreamCompletion failed: %v", err)
	}
	if !doneReceived {
		t.Error("expected done chunk")
	}
	if resp.Content != "ContextForge Rocks!" {
		t.Errorf("expected 'ContextForge Rocks!', got %q", resp.Content)
	}
	if resp.TokensUsed != 30 {
		t.Errorf("expected 30 tokens, got %d", resp.TokensUsed)
	}
}

func TestAnthropicProvider_BackoffRetry(t *testing.T) {
	var attempts int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		current := atomic.AddInt32(&attempts, 1)
		if current == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"type":"error","error":{"type":"rate_limit_error","message":"Number of request tokens has exceeded your daily limit."}}`))
			return
		}

		resp := anthropicResponse{
			ID:    "ok",
			Model: "claude-3-5-sonnet-20241022",
			Content: []anthropicContentBlock{
				{Type: "text", Text: "recovered"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider := NewAnthropicProvider(AnthropicConfig{
		BaseURL:    server.URL,
		Retry:      fastRetryConfig(),
		HTTPClient: server.Client(),
	})

	resp, err := provider.GenerateCompletion(context.Background(), &model.CompletionRequest{
		Messages: []model.ChatMessage{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("expected retry recovery, got: %v", err)
	}
	if resp.Content != "recovered" {
		t.Errorf("unexpected content: %q", resp.Content)
	}
}

// -----------------------------------------------------------------------------
// Gemini Tests
// -----------------------------------------------------------------------------

func TestGeminiProvider_GenerateCompletion(t *testing.T) {
	apiKey := "test-gemini-key"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if !strings.Contains(r.URL.Path, "generateContent") {
			t.Errorf("expected generateContent in path, got %s", r.URL.Path)
		}
		if key := r.Header.Get("x-goog-api-key"); key != apiKey {
			t.Errorf("expected x-goog-api-key %s, got %s", apiKey, key)
		}

		var reqBody geminiGenerateRequest
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			t.Fatalf("failed to decode gemini request: %v", err)
		}

		if reqBody.SystemInstruction == nil || len(reqBody.SystemInstruction.Parts) == 0 {
			t.Errorf("expected systemInstruction")
		} else if reqBody.SystemInstruction.Parts[0].Text != "System prompt" {
			t.Errorf("expected system prompt 'System prompt', got %q", reqBody.SystemInstruction.Parts[0].Text)
		}

		resp := geminiGenerateResponse{
			Candidates: []geminiCandidate{
				{
					Content: geminiContent{
						Role:  "model",
						Parts: []geminiPart{{Text: "Hello from Gemini!"}},
					},
					FinishReason: "STOP",
				},
			},
			UsageMetadata: geminiUsageMetadata{
				PromptTokenCount:     5,
				CandidatesTokenCount: 10,
				TotalTokenCount:      15,
			},
			ModelVersion: "gemini-1.5-flash",
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider := NewGeminiProvider(GeminiConfig{
		APIKey:     apiKey,
		BaseURL:    server.URL,
		Retry:      fastRetryConfig(),
		HTTPClient: server.Client(),
	})

	req := &model.CompletionRequest{
		Messages: []model.ChatMessage{
			{Role: model.RoleSystem, Content: "System prompt"},
			{Role: model.RoleUser, Content: "Hello Gemini"},
		},
		Model: "gemini-1.5-flash",
	}

	resp, err := provider.GenerateCompletion(context.Background(), req)
	if err != nil {
		t.Fatalf("GenerateCompletion failed: %v", err)
	}

	if resp.Content != "Hello from Gemini!" {
		t.Errorf("expected 'Hello from Gemini!', got %q", resp.Content)
	}
	if resp.TokensUsed != 15 {
		t.Errorf("expected 15 tokens, got %d", resp.TokensUsed)
	}
}

func TestGeminiProvider_StreamCompletion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("expected http.Flusher")
		}

		events := []string{
			`data: {"candidates":[{"content":{"parts":[{"text":"Stream "}]}}]}`,
			`data: {"candidates":[{"content":{"parts":[{"text":"Gemini"}]}}],"usageMetadata":{"totalTokenCount":25}}`,
		}

		for _, e := range events {
			_, _ = fmt.Fprintf(w, "%s\n\n", e)
			flusher.Flush()
		}
	}))
	defer server.Close()

	provider := NewGeminiProvider(GeminiConfig{
		BaseURL:    server.URL,
		Retry:      fastRetryConfig(),
		HTTPClient: server.Client(),
	})

	var chunks []string
	resp, err := provider.StreamCompletion(context.Background(), &model.CompletionRequest{
		Messages: []model.ChatMessage{{Role: "user", Content: "stream"}},
	}, func(chunk *model.StreamChunk) error {
		if !chunk.Done {
			chunks = append(chunks, chunk.Delta)
		}
		return nil
	})

	if err != nil {
		t.Fatalf("StreamCompletion failed: %v", err)
	}
	if resp.Content != "Stream Gemini" {
		t.Errorf("expected 'Stream Gemini', got %q", resp.Content)
	}
	if resp.TokensUsed != 25 {
		t.Errorf("expected 25 tokens, got %d", resp.TokensUsed)
	}
}

func TestGeminiProvider_BackoffRetry(t *testing.T) {
	var attempts int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		current := atomic.AddInt32(&attempts, 1)
		if current == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":{"code":429,"message":"Resource has been exhausted"}}`))
			return
		}

		resp := geminiGenerateResponse{
			Candidates: []geminiCandidate{
				{
					Content: geminiContent{
						Parts: []geminiPart{{Text: "gemini recovered"}},
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider := NewGeminiProvider(GeminiConfig{
		BaseURL:    server.URL,
		Retry:      fastRetryConfig(),
		HTTPClient: server.Client(),
	})

	resp, err := provider.GenerateCompletion(context.Background(), &model.CompletionRequest{
		Messages: []model.ChatMessage{{Role: "user", Content: "test"}},
	})
	if err != nil {
		t.Fatalf("expected success on retry, got: %v", err)
	}
	if resp.Content != "gemini recovered" {
		t.Errorf("unexpected content: %q", resp.Content)
	}
}
