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

const (
	DefaultAnthropicBaseURL = "https://api.anthropic.com/v1"
	DefaultAnthropicModel   = "claude-3-5-sonnet-20241022"
	anthropicAPIVersion     = "2023-06-01"
	defaultMaxTokens        = 4096
)

// AnthropicConfig configures the Anthropic provider.
type AnthropicConfig struct {
	APIKey       string
	BaseURL      string
	DefaultModel string
	HTTPClient   *http.Client
	Retry        RetryConfig
}

// AnthropicProvider implements LLMProvider for Anthropic Claude models.
type AnthropicProvider struct {
	apiKey       string
	baseURL      string
	defaultModel string
	httpClient   *http.Client
	retry        RetryConfig
}

// NewAnthropicProvider creates a new AnthropicProvider.
func NewAnthropicProvider(cfg AnthropicConfig) *AnthropicProvider {
	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	if baseURL == "" {
		baseURL = DefaultAnthropicBaseURL
	}
	defaultModel := cfg.DefaultModel
	if defaultModel == "" {
		defaultModel = DefaultAnthropicModel
	}
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	return &AnthropicProvider{
		apiKey:       cfg.APIKey,
		baseURL:      baseURL,
		defaultModel: defaultModel,
		httpClient:   httpClient,
		retry:        cfg.Retry,
	}
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicRequest struct {
	Model         string             `json:"model"`
	Messages      []anthropicMessage `json:"messages"`
	System        string             `json:"system,omitempty"`
	MaxTokens     int                `json:"max_tokens"`
	Temperature   float32            `json:"temperature,omitempty"`
	StopSequences []string           `json:"stop_sequences,omitempty"`
	Stream        bool               `json:"stream,omitempty"`
}

type anthropicContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type anthropicUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type anthropicResponse struct {
	ID         string                  `json:"id"`
	Type       string                  `json:"type"`
	Role       string                  `json:"role"`
	Content    []anthropicContentBlock `json:"content"`
	Model      string                  `json:"model"`
	StopReason string                  `json:"stop_reason"`
	Usage      anthropicUsage          `json:"usage"`
}

// prepareRequest separates system messages from user/assistant messages.
func (p *AnthropicProvider) prepareRequest(req *model.CompletionRequest, stream bool) anthropicRequest {
	modelName := req.Model
	if modelName == "" {
		modelName = p.defaultModel
	}

	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = defaultMaxTokens
	}

	var systemParts []string
	var messages []anthropicMessage

	for _, msg := range req.Messages {
		if strings.EqualFold(msg.Role, model.RoleSystem) {
			systemParts = append(systemParts, msg.Content)
		} else {
			messages = append(messages, anthropicMessage{
				Role:    msg.Role,
				Content: msg.Content,
			})
		}
	}

	systemPrompt := strings.Join(systemParts, "\n\n")

	return anthropicRequest{
		Model:         modelName,
		Messages:      messages,
		System:        systemPrompt,
		MaxTokens:     maxTokens,
		Temperature:   req.Temperature,
		StopSequences: req.Stop,
		Stream:        stream,
	}
}

// GenerateCompletion sends a non-streaming completion request to Anthropic.
func (p *AnthropicProvider) GenerateCompletion(ctx context.Context, req *model.CompletionRequest) (*model.CompletionResponse, error) {
	startTime := time.Now()

	payload := p.prepareRequest(req, false)
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal anthropic request: %w", err)
	}

	endpoint := fmt.Sprintf("%s/messages", p.baseURL)

	resp, err := doWithRetry(ctx, p.httpClient, func() (*http.Request, error) {
		r, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
		if err != nil {
			return nil, err
		}
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("anthropic-version", anthropicAPIVersion)
		if p.apiKey != "" {
			r.Header.Set("x-api-key", p.apiKey)
		}
		return r, nil
	}, p.retry)

	if err != nil {
		return nil, fmt.Errorf("anthropic messages request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read anthropic response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("anthropic api error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var msgResp anthropicResponse
	if err := json.Unmarshal(respBody, &msgResp); err != nil {
		return nil, fmt.Errorf("failed to decode anthropic response: %w", err)
	}

	var contentBuilder strings.Builder
	for _, block := range msgResp.Content {
		if block.Type == "text" {
			contentBuilder.WriteString(block.Text)
		}
	}

	totalTokens := msgResp.Usage.InputTokens + msgResp.Usage.OutputTokens
	durationMs := time.Since(startTime).Milliseconds()

	return &model.CompletionResponse{
		Content:    contentBuilder.String(),
		Model:      msgResp.Model,
		TokensUsed: totalTokens,
		DurationMs: durationMs,
	}, nil
}

// StreamCompletion sends a streaming request to Anthropic and invokes onChunk for each text delta.
func (p *AnthropicProvider) StreamCompletion(ctx context.Context, req *model.CompletionRequest, onChunk func(chunk *model.StreamChunk) error) (*model.CompletionResponse, error) {
	startTime := time.Now()

	payload := p.prepareRequest(req, true)
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal anthropic streaming request: %w", err)
	}

	endpoint := fmt.Sprintf("%s/messages", p.baseURL)

	resp, err := doWithRetry(ctx, p.httpClient, func() (*http.Request, error) {
		r, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
		if err != nil {
			return nil, err
		}
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Accept", "text/event-stream")
		r.Header.Set("anthropic-version", anthropicAPIVersion)
		if p.apiKey != "" {
			r.Header.Set("x-api-key", p.apiKey)
		}
		return r, nil
	}, p.retry)

	if err != nil {
		return nil, fmt.Errorf("anthropic streaming request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("anthropic api error (status %d): %s", resp.StatusCode, string(respBody))
	}

	scanner := bufio.NewScanner(resp.Body)
	var fullContent strings.Builder
	var lastModel string = payload.Model
	var inputTokens int
	var outputTokens int

	var currentEvent string

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}

		if strings.HasPrefix(line, "event:") {
			currentEvent = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			continue
		}

		if strings.HasPrefix(line, "data:") {
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))

			switch currentEvent {
			case "message_start":
				var startData struct {
					Message struct {
						Model string         `json:"model"`
						Usage anthropicUsage `json:"usage"`
					} `json:"message"`
				}
				if err := json.Unmarshal([]byte(data), &startData); err == nil {
					if startData.Message.Model != "" {
						lastModel = startData.Message.Model
					}
					inputTokens = startData.Message.Usage.InputTokens
					outputTokens = startData.Message.Usage.OutputTokens
				}

			case "content_block_delta":
				var deltaData struct {
					Delta struct {
						Type string `json:"type"`
						Text string `json:"text"`
					} `json:"delta"`
				}
				if err := json.Unmarshal([]byte(data), &deltaData); err == nil {
					if deltaData.Delta.Type == "text_delta" && deltaData.Delta.Text != "" {
						fullContent.WriteString(deltaData.Delta.Text)
						if onChunk != nil {
							if err := onChunk(&model.StreamChunk{
								Delta:      deltaData.Delta.Text,
								Done:       false,
								TokensUsed: inputTokens + outputTokens,
							}); err != nil {
								return nil, err
							}
						}
					}
				}

			case "message_delta":
				var deltaUsage struct {
					Usage struct {
						OutputTokens int `json:"output_tokens"`
					} `json:"usage"`
				}
				if err := json.Unmarshal([]byte(data), &deltaUsage); err == nil {
					if deltaUsage.Usage.OutputTokens > 0 {
						outputTokens = deltaUsage.Usage.OutputTokens
					}
				}

			case "message_stop":
				// Stream finished
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading anthropic stream: %w", err)
	}

	tokensUsed := inputTokens + outputTokens
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
