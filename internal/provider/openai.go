package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/your-org/contextforge/internal/model"
)

const (
	DefaultOpenAIBaseURL        = "https://api.openai.com/v1"
	DefaultOpenAIModel          = "gpt-4o-mini"
	DefaultOpenAIEmbeddingModel = "text-embedding-3-small"
	openAIEmbeddingBatchSize    = 500
)

// OpenAIConfig configures the OpenAI provider.
type OpenAIConfig struct {
	APIKey                string
	BaseURL               string
	DefaultModel          string
	DefaultEmbeddingModel string
	HTTPClient            *http.Client
	Retry                 RetryConfig
}

// OpenAIProvider implements LLMProvider and EmbeddingProvider for OpenAI-compatible APIs.
type OpenAIProvider struct {
	apiKey                string
	baseURL               string
	defaultModel          string
	defaultEmbeddingModel string
	httpClient            *http.Client
	retry                 RetryConfig
}

// NewOpenAIProvider creates an OpenAIProvider instance with BYOK API key and optional configuration.
func NewOpenAIProvider(cfg OpenAIConfig) *OpenAIProvider {
	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	if baseURL == "" {
		baseURL = DefaultOpenAIBaseURL
	}
	defaultModel := cfg.DefaultModel
	if defaultModel == "" {
		defaultModel = DefaultOpenAIModel
	}
	defaultEmbeddingModel := cfg.DefaultEmbeddingModel
	if defaultEmbeddingModel == "" {
		defaultEmbeddingModel = DefaultOpenAIEmbeddingModel
	}
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	return &OpenAIProvider{
		apiKey:                cfg.APIKey,
		baseURL:               baseURL,
		defaultModel:          defaultModel,
		defaultEmbeddingModel: defaultEmbeddingModel,
		httpClient:            httpClient,
		retry:                 cfg.Retry,
	}
}

// openAIChatRequest is the payload sent to /chat/completions.
type openAIChatRequest struct {
	Model         string              `json:"model"`
	Messages      []model.ChatMessage `json:"messages"`
	Temperature   float32             `json:"temperature,omitempty"`
	MaxTokens     int                 `json:"max_tokens,omitempty"`
	Stream        bool                `json:"stream,omitempty"`
	Stop          []string            `json:"stop,omitempty"`
	StreamOptions *openAIStreamOption `json:"stream_options,omitempty"`
}

type openAIStreamOption struct {
	IncludeUsage bool `json:"include_usage"`
}

// openAIChatResponse represents the response from /chat/completions.
type openAIChatResponse struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Choices []struct {
		Index        int               `json:"index"`
		Message      model.ChatMessage `json:"message"`
		FinishReason string            `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

// GenerateCompletion sends a non-streaming chat completion request to OpenAI.
func (p *OpenAIProvider) GenerateCompletion(ctx context.Context, req *model.CompletionRequest) (*model.CompletionResponse, error) {
	startTime := time.Now()

	modelName := req.Model
	if modelName == "" {
		modelName = p.defaultModel
	}

	payload := openAIChatRequest{
		Model:       modelName,
		Messages:    req.Messages,
		Temperature: req.Temperature,
		MaxTokens:   req.MaxTokens,
		Stream:      false,
		Stop:        req.Stop,
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	endpoint := fmt.Sprintf("%s/chat/completions", p.baseURL)

	resp, err := doWithRetry(ctx, p.httpClient, func() (*http.Request, error) {
		r, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
		if err != nil {
			return nil, err
		}
		r.Header.Set("Content-Type", "application/json")
		if p.apiKey != "" {
			r.Header.Set("Authorization", "Bearer "+p.apiKey)
		}
		return r, nil
	}, p.retry)

	if err != nil {
		return nil, fmt.Errorf("openai chat completion failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read openai response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("openai api error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var chatResp openAIChatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return nil, fmt.Errorf("failed to decode openai response: %w", err)
	}

	content := ""
	if len(chatResp.Choices) > 0 {
		content = chatResp.Choices[0].Message.Content
	}

	durationMs := time.Since(startTime).Milliseconds()

	return &model.CompletionResponse{
		Content:    content,
		Model:      chatResp.Model,
		TokensUsed: chatResp.Usage.TotalTokens,
		DurationMs: durationMs,
	}, nil
}

// openAIStreamChunk represents an incremental SSE chunk.
type openAIStreamChunk struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Choices []struct {
		Index int `json:"index"`
		Delta struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

// StreamCompletion sends a streaming chat completion request to OpenAI and invokes onChunk.
func (p *OpenAIProvider) StreamCompletion(ctx context.Context, req *model.CompletionRequest, onChunk func(chunk *model.StreamChunk) error) (*model.CompletionResponse, error) {
	startTime := time.Now()

	modelName := req.Model
	if modelName == "" {
		modelName = p.defaultModel
	}

	payload := openAIChatRequest{
		Model:         modelName,
		Messages:      req.Messages,
		Temperature:   req.Temperature,
		MaxTokens:     req.MaxTokens,
		Stream:        true,
		Stop:          req.Stop,
		StreamOptions: &openAIStreamOption{IncludeUsage: true},
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	endpoint := fmt.Sprintf("%s/chat/completions", p.baseURL)

	resp, err := doWithRetry(ctx, p.httpClient, func() (*http.Request, error) {
		r, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
		if err != nil {
			return nil, err
		}
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Accept", "text/event-stream")
		if p.apiKey != "" {
			r.Header.Set("Authorization", "Bearer "+p.apiKey)
		}
		return r, nil
	}, p.retry)

	if err != nil {
		return nil, fmt.Errorf("openai streaming chat failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("openai api error (status %d): %s", resp.StatusCode, string(respBody))
	}

	scanner := bufio.NewScanner(resp.Body)
	var fullContent strings.Builder
	var lastModel string = modelName
	var tokensUsed int

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, ":") {
			// Empty line or SSE keepalive comment
			continue
		}

		if !strings.HasPrefix(line, "data:") {
			continue
		}

		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			break
		}

		var chunk openAIStreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}

		if chunk.Model != "" {
			lastModel = chunk.Model
		}

		if chunk.Usage != nil && chunk.Usage.TotalTokens > 0 {
			tokensUsed = chunk.Usage.TotalTokens
		}

		var delta string
		if len(chunk.Choices) > 0 {
			delta = chunk.Choices[0].Delta.Content
		}

		if delta != "" {
			fullContent.WriteString(delta)
			if onChunk != nil {
				if err := onChunk(&model.StreamChunk{
					Delta:      delta,
					Done:       false,
					TokensUsed: tokensUsed,
				}); err != nil {
					return nil, err
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading stream: %w", err)
	}

	// Send final Done chunk
	if onChunk != nil {
		if err := onChunk(&model.StreamChunk{
			Delta:      "",
			Done:       true,
			TokensUsed: tokensUsed,
		}); err != nil {
			return nil, err
		}
	}

	durationMs := time.Since(startTime).Milliseconds()

	return &model.CompletionResponse{
		Content:    fullContent.String(),
		Model:      lastModel,
		TokensUsed: tokensUsed,
		DurationMs: durationMs,
	}, nil
}

// openAIEmbeddingRequest is the payload sent to /embeddings.
type openAIEmbeddingRequest struct {
	Input []string `json:"input"`
	Model string   `json:"model"`
}

type openAIEmbeddingData struct {
	Index     int       `json:"index"`
	Embedding []float32 `json:"embedding"`
}

type openAIEmbeddingResponse struct {
	Data  []openAIEmbeddingData `json:"data"`
	Model string                `json:"model"`
	Usage struct {
		PromptTokens int `json:"prompt_tokens"`
		TotalTokens  int `json:"total_tokens"`
	} `json:"usage"`
}

// EmbedDocuments generates embeddings for multiple documents, handling batching.
func (p *OpenAIProvider) EmbedDocuments(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return [][]float32{}, nil
	}

	var allEmbeddings [][]float32

	for i := 0; i < len(texts); i += openAIEmbeddingBatchSize {
		end := i + openAIEmbeddingBatchSize
		if end > len(texts) {
			end = len(texts)
		}
		batch := texts[i:end]

		batchEmbeddings, err := p.embedBatch(ctx, batch)
		if err != nil {
			return nil, err
		}
		allEmbeddings = append(allEmbeddings, batchEmbeddings...)
	}

	return allEmbeddings, nil
}

// EmbedQuery generates an embedding for a single text query.
func (p *OpenAIProvider) EmbedQuery(ctx context.Context, text string) ([]float32, error) {
	res, err := p.EmbedDocuments(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(res) == 0 {
		return nil, fmt.Errorf("openai returned empty embedding")
	}
	return res[0], nil
}

// embedBatch embeds a single batch of texts.
func (p *OpenAIProvider) embedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	payload := openAIEmbeddingRequest{
		Input: texts,
		Model: p.defaultEmbeddingModel,
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal embedding request: %w", err)
	}

	endpoint := fmt.Sprintf("%s/embeddings", p.baseURL)

	resp, err := doWithRetry(ctx, p.httpClient, func() (*http.Request, error) {
		r, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
		if err != nil {
			return nil, err
		}
		r.Header.Set("Content-Type", "application/json")
		if p.apiKey != "" {
			r.Header.Set("Authorization", "Bearer "+p.apiKey)
		}
		return r, nil
	}, p.retry)

	if err != nil {
		return nil, fmt.Errorf("openai embedding request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read embedding response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("openai embedding api error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var embResp openAIEmbeddingResponse
	if err := json.Unmarshal(respBody, &embResp); err != nil {
		return nil, fmt.Errorf("failed to decode openai embedding response: %w", err)
	}

	// Sort by index to guarantee ordering matches input
	sort.Slice(embResp.Data, func(i, j int) bool {
		return embResp.Data[i].Index < embResp.Data[j].Index
	})

	results := make([][]float32, len(embResp.Data))
	for i, d := range embResp.Data {
		results[i] = d.Embedding
	}

	return results, nil
}
