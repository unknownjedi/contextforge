package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/your-org/contextforge/internal/model"
)

const (
	DefaultGeminiBaseURL        = "https://generativelanguage.googleapis.com/v1beta"
	DefaultGeminiModel          = "gemini-1.5-flash"
	DefaultGeminiEmbeddingModel = "text-embedding-004"
	geminiEmbeddingBatchSize    = 100
)

// GeminiConfig configures the Google Gemini provider.
type GeminiConfig struct {
	APIKey                string
	BaseURL               string
	DefaultModel          string
	DefaultEmbeddingModel string
	HTTPClient            *http.Client
	Retry                 RetryConfig
}

// GeminiProvider implements LLMProvider and EmbeddingProvider for Google Gemini models.
type GeminiProvider struct {
	apiKey                string
	baseURL               string
	defaultModel          string
	defaultEmbeddingModel string
	httpClient            *http.Client
	retry                 RetryConfig
}

// NewGeminiProvider creates a new GeminiProvider instance.
func NewGeminiProvider(cfg GeminiConfig) *GeminiProvider {
	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	if baseURL == "" {
		baseURL = DefaultGeminiBaseURL
	}
	defaultModel := cfg.DefaultModel
	if defaultModel == "" {
		defaultModel = DefaultGeminiModel
	}
	defaultEmbeddingModel := cfg.DefaultEmbeddingModel
	if defaultEmbeddingModel == "" {
		defaultEmbeddingModel = DefaultGeminiEmbeddingModel
	}
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	return &GeminiProvider{
		apiKey:                cfg.APIKey,
		baseURL:               baseURL,
		defaultModel:          defaultModel,
		defaultEmbeddingModel: defaultEmbeddingModel,
		httpClient:            httpClient,
		retry:                 cfg.Retry,
	}
}

type geminiPart struct {
	Text string `json:"text"`
}

type geminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []geminiPart `json:"parts"`
}

type geminiSystemInstruction struct {
	Parts []geminiPart `json:"parts"`
}

type geminiGenerationConfig struct {
	Temperature     float32  `json:"temperature,omitempty"`
	MaxOutputTokens int      `json:"maxOutputTokens,omitempty"`
	StopSequences   []string `json:"stopSequences,omitempty"`
}

type geminiGenerateRequest struct {
	Contents          []geminiContent          `json:"contents"`
	SystemInstruction *geminiSystemInstruction `json:"systemInstruction,omitempty"`
	GenerationConfig  *geminiGenerationConfig  `json:"generationConfig,omitempty"`
}

type geminiCandidate struct {
	Content      geminiContent `json:"content"`
	FinishReason string        `json:"finishReason"`
}

type geminiUsageMetadata struct {
	PromptTokenCount     int `json:"promptTokenCount"`
	CandidatesTokenCount int `json:"candidatesTokenCount"`
	TotalTokenCount      int `json:"totalTokenCount"`
}

type geminiGenerateResponse struct {
	Candidates    []geminiCandidate   `json:"candidates"`
	UsageMetadata geminiUsageMetadata `json:"usageMetadata"`
	ModelVersion  string              `json:"modelVersion"`
}

func cleanModelName(m string) string {
	return strings.TrimPrefix(m, "models/")
}

func (p *GeminiProvider) buildURL(path string, query url.Values) string {
	if query == nil {
		query = url.Values{}
	}
	if p.apiKey != "" {
		query.Set("key", p.apiKey)
	}
	endpoint := fmt.Sprintf("%s/%s", p.baseURL, strings.TrimLeft(path, "/"))
	if len(query) > 0 {
		return fmt.Sprintf("%s?%s", endpoint, query.Encode())
	}
	return endpoint
}

func (p *GeminiProvider) prepareRequest(req *model.CompletionRequest) (string, geminiGenerateRequest) {
	modelName := req.Model
	if modelName == "" {
		modelName = p.defaultModel
	}

	var systemParts []geminiPart
	var contents []geminiContent

	for _, msg := range req.Messages {
		if strings.EqualFold(msg.Role, model.RoleSystem) {
			systemParts = append(systemParts, geminiPart{Text: msg.Content})
		} else {
			role := msg.Role
			if role == model.RoleAssistant {
				role = "model"
			}
			contents = append(contents, geminiContent{
				Role:  role,
				Parts: []geminiPart{{Text: msg.Content}},
			})
		}
	}

	var sysInstruction *geminiSystemInstruction
	if len(systemParts) > 0 {
		sysInstruction = &geminiSystemInstruction{Parts: systemParts}
	}

	var genConfig *geminiGenerationConfig
	if req.Temperature > 0 || req.MaxTokens > 0 || len(req.Stop) > 0 {
		genConfig = &geminiGenerationConfig{
			Temperature:     req.Temperature,
			MaxOutputTokens: req.MaxTokens,
			StopSequences:   req.Stop,
		}
	}

	payload := geminiGenerateRequest{
		Contents:          contents,
		SystemInstruction: sysInstruction,
		GenerationConfig:  genConfig,
	}

	return cleanModelName(modelName), payload
}

// GenerateCompletion sends a non-streaming completion request to Gemini.
func (p *GeminiProvider) GenerateCompletion(ctx context.Context, req *model.CompletionRequest) (*model.CompletionResponse, error) {
	startTime := time.Now()

	modelName, payload := p.prepareRequest(req)
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal gemini request: %w", err)
	}

	endpoint := p.buildURL(fmt.Sprintf("models/%s:generateContent", modelName), nil)

	resp, err := doWithRetry(ctx, p.httpClient, func() (*http.Request, error) {
		r, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
		if err != nil {
			return nil, err
		}
		r.Header.Set("Content-Type", "application/json")
		if p.apiKey != "" {
			r.Header.Set("x-goog-api-key", p.apiKey)
		}
		return r, nil
	}, p.retry)

	if err != nil {
		return nil, fmt.Errorf("gemini generateContent failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read gemini response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gemini api error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var genResp geminiGenerateResponse
	if err := json.Unmarshal(respBody, &genResp); err != nil {
		return nil, fmt.Errorf("failed to decode gemini response: %w", err)
	}

	var contentBuilder strings.Builder
	if len(genResp.Candidates) > 0 {
		for _, part := range genResp.Candidates[0].Content.Parts {
			contentBuilder.WriteString(part.Text)
		}
	}

	durationMs := time.Since(startTime).Milliseconds()
	respModel := genResp.ModelVersion
	if respModel == "" {
		respModel = modelName
	}

	return &model.CompletionResponse{
		Content:    contentBuilder.String(),
		Model:      respModel,
		TokensUsed: genResp.UsageMetadata.TotalTokenCount,
		DurationMs: durationMs,
	}, nil
}

// StreamCompletion sends a streaming request to Gemini and invokes onChunk for each incoming segment.
func (p *GeminiProvider) StreamCompletion(ctx context.Context, req *model.CompletionRequest, onChunk func(chunk *model.StreamChunk) error) (*model.CompletionResponse, error) {
	startTime := time.Now()

	modelName, payload := p.prepareRequest(req)
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal gemini streaming request: %w", err)
	}

	q := url.Values{}
	q.Set("alt", "sse")
	endpoint := p.buildURL(fmt.Sprintf("models/%s:streamGenerateContent", modelName), q)

	resp, err := doWithRetry(ctx, p.httpClient, func() (*http.Request, error) {
		r, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
		if err != nil {
			return nil, err
		}
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Accept", "text/event-stream")
		if p.apiKey != "" {
			r.Header.Set("x-goog-api-key", p.apiKey)
		}
		return r, nil
	}, p.retry)

	if err != nil {
		return nil, fmt.Errorf("gemini streamGenerateContent failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("gemini api error (status %d): %s", resp.StatusCode, string(respBody))
	}

	scanner := bufio.NewScanner(resp.Body)
	var fullContent strings.Builder
	var lastModel string = modelName
	var tokensUsed int

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}

		if !strings.HasPrefix(line, "data:") {
			continue
		}

		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		var streamResp geminiGenerateResponse
		if err := json.Unmarshal([]byte(data), &streamResp); err != nil {
			continue
		}

		if streamResp.ModelVersion != "" {
			lastModel = streamResp.ModelVersion
		}
		if streamResp.UsageMetadata.TotalTokenCount > 0 {
			tokensUsed = streamResp.UsageMetadata.TotalTokenCount
		}

		if len(streamResp.Candidates) > 0 {
			for _, part := range streamResp.Candidates[0].Content.Parts {
				if part.Text != "" {
					fullContent.WriteString(part.Text)
					if onChunk != nil {
						if err := onChunk(&model.StreamChunk{
							Delta:      part.Text,
							Done:       false,
							TokensUsed: tokensUsed,
						}); err != nil {
							return nil, err
						}
					}
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading gemini stream: %w", err)
	}

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

type geminiBatchEmbedItem struct {
	Model   string        `json:"model"`
	Content geminiContent `json:"content"`
}

type geminiBatchEmbedRequest struct {
	Requests []geminiBatchEmbedItem `json:"requests"`
}

type geminiEmbedValues struct {
	Values []float32 `json:"values"`
}

type geminiBatchEmbedResponse struct {
	Embeddings []geminiEmbedValues `json:"embeddings"`
}

// EmbedDocuments generates embeddings for multiple documents using Gemini's batchEmbedContents endpoint.
func (p *GeminiProvider) EmbedDocuments(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return [][]float32{}, nil
	}

	var allEmbeddings [][]float32
	cleanModel := cleanModelName(p.defaultEmbeddingModel)
	modelWithPrefix := "models/" + cleanModel

	for i := 0; i < len(texts); i += geminiEmbeddingBatchSize {
		end := i + geminiEmbeddingBatchSize
		if end > len(texts) {
			end = len(texts)
		}
		batch := texts[i:end]

		var reqItems []geminiBatchEmbedItem
		for _, text := range batch {
			reqItems = append(reqItems, geminiBatchEmbedItem{
				Model: modelWithPrefix,
				Content: geminiContent{
					Parts: []geminiPart{{Text: text}},
				},
			})
		}

		payload := geminiBatchEmbedRequest{Requests: reqItems}
		bodyBytes, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal gemini embedding request: %w", err)
		}

		endpoint := p.buildURL(fmt.Sprintf("models/%s:batchEmbedContents", cleanModel), nil)

		resp, err := doWithRetry(ctx, p.httpClient, func() (*http.Request, error) {
			r, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
			if err != nil {
				return nil, err
			}
			r.Header.Set("Content-Type", "application/json")
			if p.apiKey != "" {
				r.Header.Set("x-goog-api-key", p.apiKey)
			}
			return r, nil
		}, p.retry)

		if err != nil {
			return nil, fmt.Errorf("gemini batchEmbedContents failed: %w", err)
		}
		defer resp.Body.Close()

		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to read gemini embedding response: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("gemini embedding api error (status %d): %s", resp.StatusCode, string(respBody))
		}

		var embResp geminiBatchEmbedResponse
		if err := json.Unmarshal(respBody, &embResp); err != nil {
			return nil, fmt.Errorf("failed to decode gemini embedding response: %w", err)
		}

		for _, item := range embResp.Embeddings {
			allEmbeddings = append(allEmbeddings, item.Values)
		}
	}

	return allEmbeddings, nil
}

// EmbedBatch generates embeddings for multiple documents (alias for EmbedDocuments).
func (p *GeminiProvider) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	return p.EmbedDocuments(ctx, texts)
}

// EmbedQuery generates an embedding for a single text query.
func (p *GeminiProvider) EmbedQuery(ctx context.Context, text string) ([]float32, error) {
	res, err := p.EmbedDocuments(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(res) == 0 {
		return nil, fmt.Errorf("gemini returned empty embedding")
	}
	return res[0], nil
}
