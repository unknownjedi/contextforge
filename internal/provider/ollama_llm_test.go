package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/your-org/contextforge/internal/model"
)

func TestNewOllamaLLMProvider(t *testing.T) {
	p := NewOllamaLLMProvider("", "", 0)
	if p.baseURL != DefaultOllamaBaseURL {
		t.Errorf("expected baseURL %q, got %q", DefaultOllamaBaseURL, p.baseURL)
	}
	if p.defaultModel != "qwen3.5:latest" {
		t.Errorf("expected defaultModel 'qwen3.5:latest', got %q", p.defaultModel)
	}
	if p.httpClient.Timeout != 90*time.Second {
		t.Errorf("expected timeout 90s, got %v", p.httpClient.Timeout)
	}

	custom := NewOllamaLLMProvider("http://custom:11434/", "my-model", 30*time.Second)
	if custom.baseURL != "http://custom:11434" {
		t.Errorf("expected trimmed baseURL 'http://custom:11434', got %q", custom.baseURL)
	}
	if custom.defaultModel != "my-model" {
		t.Errorf("expected defaultModel 'my-model', got %q", custom.defaultModel)
	}
	if custom.httpClient.Timeout != 30*time.Second {
		t.Errorf("expected timeout 30s, got %v", custom.httpClient.Timeout)
	}
}

func TestOllamaLLMProvider_GenerateCompletion(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost || r.URL.Path != "/api/chat" {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			var req ollamaChatRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if req.Stream {
				http.Error(w, "expected stream=false", http.StatusBadRequest)
				return
			}
			if req.Model != "qwen3.5:latest" {
				http.Error(w, fmt.Sprintf("unexpected model %s", req.Model), http.StatusBadRequest)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(ollamaChatChunk{
				Model:     req.Model,
				Message:   ollamaChatMessage{Role: "assistant", Content: "Hello from Ollama!"},
				Done:      true,
				EvalCount: 15,
			})
		}))
		defer server.Close()

		p := NewOllamaLLMProvider(server.URL, "qwen3.5:latest", 5*time.Second)
		resp, err := p.GenerateCompletion(context.Background(), &model.CompletionRequest{
			Messages: []model.ChatMessage{
				{Role: "user", Content: "Hi"},
			},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.Content != "Hello from Ollama!" {
			t.Errorf("expected 'Hello from Ollama!', got %q", resp.Content)
		}
		if resp.TokensUsed != 15 {
			t.Errorf("expected tokensUsed=15, got %d", resp.TokensUsed)
		}
		if resp.Model != "qwen3.5:latest" {
			t.Errorf("expected model 'qwen3.5:latest', got %q", resp.Model)
		}
	})

	t.Run("CustomModelAndOptions", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var req ollamaChatRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			if req.Model != "llama3.2:1b" {
				http.Error(w, "wrong model", http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(ollamaChatChunk{
				Model:   req.Model,
				Message: ollamaChatMessage{Role: "assistant", Content: "Response from llama3.2"},
				Done:    true,
			})
		}))
		defer server.Close()

		p := NewOllamaLLMProvider(server.URL, "qwen3.5:latest", 5*time.Second)
		resp, err := p.GenerateCompletion(context.Background(), &model.CompletionRequest{
			Model:       "llama3.2:1b",
			Temperature: 0.7,
			MaxTokens:   500,
			Messages: []model.ChatMessage{
				{Role: "user", Content: "Tell me a joke"},
			},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.Content != "Response from llama3.2" {
			t.Errorf("unexpected content: %q", resp.Content)
		}
	})

	t.Run("HTTPError", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "model not found", http.StatusNotFound)
		}))
		defer server.Close()

		p := NewOllamaLLMProvider(server.URL, "qwen3.5:latest", 5*time.Second)
		_, err := p.GenerateCompletion(context.Background(), &model.CompletionRequest{})
		if err == nil {
			t.Fatal("expected error for 404, got nil")
		}
	})
}

func TestOllamaLLMProvider_StreamCompletion(t *testing.T) {
	t.Run("StreamingSuccess", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/x-ndjson")
			flusher, ok := w.(http.Flusher)
			if !ok {
				t.Fatal("expected Flusher")
			}

			chunks := []ollamaChatChunk{
				{Message: ollamaChatMessage{Role: "assistant", Content: "Thinking: "}},
				{Message: ollamaChatMessage{Role: "assistant", Content: "Here is your "}},
				{Message: ollamaChatMessage{Role: "assistant", Content: "code answer."}},
				{Done: true, EvalCount: 24},
			}

			for _, c := range chunks {
				b, _ := json.Marshal(c)
				_, _ = w.Write(append(b, '\n'))
				flusher.Flush()
			}
		}))
		defer server.Close()

		p := NewOllamaLLMProvider(server.URL, "qwen3.5:latest", 5*time.Second)
		var chunkCount int32
		var collected string

		resp, err := p.StreamCompletion(context.Background(), &model.CompletionRequest{
			Messages: []model.ChatMessage{{Role: "user", Content: "Hi"}},
		}, func(c *model.StreamChunk) error {
			atomic.AddInt32(&chunkCount, 1)
			collected += c.Delta
			return nil
		})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if atomic.LoadInt32(&chunkCount) != 3 {
			t.Errorf("expected 3 chunks, got %d", chunkCount)
		}
		expectedContent := "Thinking: Here is your code answer."
		if resp.Content != expectedContent || collected != expectedContent {
			t.Errorf("expected %q, got resp=%q collected=%q", expectedContent, resp.Content, collected)
		}
		if resp.TokensUsed != 24 {
			t.Errorf("expected 24 tokens used, got %d", resp.TokensUsed)
		}
	})

	t.Run("OnChunkErrorAbortsStream", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/x-ndjson")
			flusher := w.(http.Flusher)
			for i := 0; i < 5; i++ {
				b, _ := json.Marshal(ollamaChatChunk{
					Message: ollamaChatMessage{Role: "assistant", Content: fmt.Sprintf("chunk-%d", i)},
				})
				_, _ = w.Write(append(b, '\n'))
				flusher.Flush()
			}
		}))
		defer server.Close()

		p := NewOllamaLLMProvider(server.URL, "qwen3.5:latest", 5*time.Second)
		_, err := p.StreamCompletion(context.Background(), &model.CompletionRequest{}, func(c *model.StreamChunk) error {
			return fmt.Errorf("client disconnected")
		})

		if err == nil || err.Error() != "client disconnected" {
			t.Fatalf("expected 'client disconnected' error, got %v", err)
		}
	})

	t.Run("HTTPError", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "internal error", http.StatusInternalServerError)
		}))
		defer server.Close()

		p := NewOllamaLLMProvider(server.URL, "qwen3.5:latest", 5*time.Second)
		_, err := p.StreamCompletion(context.Background(), &model.CompletionRequest{}, nil)
		if err == nil {
			t.Fatal("expected error for HTTP 500, got nil")
		}
	})
}
