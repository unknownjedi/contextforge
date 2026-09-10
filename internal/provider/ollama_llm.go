package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/your-org/contextforge/internal/model"
)

// OllamaLLMProvider implements LLMProvider for local Ollama instances.
type OllamaLLMProvider struct {
	baseURL      string
	defaultModel string
	httpClient   *http.Client
}

// NewOllamaLLMProvider constructs an OllamaLLMProvider.
func NewOllamaLLMProvider(baseURL, defaultModel string, timeout time.Duration) *OllamaLLMProvider {
	trimmedURL := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if trimmedURL == "" {
		trimmedURL = DefaultOllamaBaseURL
	}
	modelName := strings.TrimSpace(defaultModel)
	if modelName == "" {
		modelName = "qwen3.5:latest"
	}
	if timeout <= 0 {
		timeout = 90 * time.Second
	}
	return &OllamaLLMProvider{
		baseURL:      trimmedURL,
		defaultModel: modelName,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

// ollamaChatMessage matches Ollama's message payload in /api/chat.
type ollamaChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ollamaChatRequest represents the request body for /api/chat.
type ollamaChatRequest struct {
	Model    string              `json:"model"`
	Messages []ollamaChatMessage `json:"messages"`
	Stream   bool                `json:"stream"`
	Options  map[string]any      `json:"options,omitempty"`
}

// ollamaChatChunk represents a single chunk from /api/chat streaming.
type ollamaChatChunk struct {
	Model     string            `json:"model"`
	CreatedAt string            `json:"created_at"`
	Message   ollamaChatMessage `json:"message"`
	Done      bool              `json:"done"`
	EvalCount int               `json:"eval_count,omitempty"`
}

// GenerateCompletion sends a non-streaming chat completion request to Ollama.
func (p *OllamaLLMProvider) GenerateCompletion(ctx context.Context, req *model.CompletionRequest) (*model.CompletionResponse, error) {
	modelName := req.Model
	if modelName == "" || modelName == "default" {
		modelName = p.defaultModel
	}

	messages := make([]ollamaChatMessage, 0, len(req.Messages))
	for _, m := range req.Messages {
		messages = append(messages, ollamaChatMessage{
			Role:    m.Role,
			Content: m.Content,
		})
	}

	options := make(map[string]any)
	if req.Temperature > 0 {
		options["temperature"] = req.Temperature
	}
	if req.MaxTokens > 0 {
		options["num_predict"] = req.MaxTokens
	}

	payload := ollamaChatRequest{
		Model:    modelName,
		Messages: messages,
		Stream:   false,
		Options:  options,
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshaling ollama request: %w", err)
	}

	url := fmt.Sprintf("%s/api/chat", p.baseURL)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("creating ollama request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("ollama chat request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ollama returned HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var chatResp ollamaChatChunk
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return nil, fmt.Errorf("decoding ollama response: %w", err)
	}

	return &model.CompletionResponse{
		Content:    chatResp.Message.Content,
		TokensUsed: chatResp.EvalCount,
		Model:      modelName,
	}, nil
}

// StreamCompletion streams tokens from Ollama's /api/chat endpoint.
func (p *OllamaLLMProvider) StreamCompletion(
	ctx context.Context,
	req *model.CompletionRequest,
	onChunk func(chunk *model.StreamChunk) error,
) (*model.CompletionResponse, error) {
	modelName := req.Model
	if modelName == "" || modelName == "default" {
		modelName = p.defaultModel
	}

	messages := make([]ollamaChatMessage, 0, len(req.Messages))
	for _, m := range req.Messages {
		messages = append(messages, ollamaChatMessage{
			Role:    m.Role,
			Content: m.Content,
		})
	}

	options := make(map[string]any)
	if req.Temperature > 0 {
		options["temperature"] = req.Temperature
	}
	if req.MaxTokens > 0 {
		options["num_predict"] = req.MaxTokens
	}

	payload := ollamaChatRequest{
		Model:    modelName,
		Messages: messages,
		Stream:   true,
		Options:  options,
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshaling ollama request: %w", err)
	}

	url := fmt.Sprintf("%s/api/chat", p.baseURL)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("creating ollama request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("ollama stream request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ollama returned HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	scanner := bufio.NewScanner(resp.Body)
	var fullContent strings.Builder
	tokensUsed := 0

	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}

		var chunk ollamaChatChunk
		if err := json.Unmarshal(line, &chunk); err != nil {
			continue
		}

		if chunk.Message.Content != "" {
			fullContent.WriteString(chunk.Message.Content)
			if onChunk != nil {
				if err := onChunk(&model.StreamChunk{Delta: chunk.Message.Content}); err != nil {
					return nil, err
				}
			}
		}

		if chunk.Done && chunk.EvalCount > 0 {
			tokensUsed = chunk.EvalCount
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading ollama stream: %w", err)
	}

	if tokensUsed == 0 {
		tokensUsed = len(fullContent.String()) / 4
	}

	return &model.CompletionResponse{
		Content:    fullContent.String(),
		TokensUsed: tokensUsed,
		Model:      modelName,
	}, nil
}
