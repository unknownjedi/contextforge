package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/your-org/contextforge/internal/model"
	"github.com/your-org/contextforge/internal/provider"
	"github.com/your-org/contextforge/internal/rag"
	"github.com/your-org/contextforge/internal/retrieval"
)

// ProviderResolver resolves an LLM provider by name.
type ProviderResolver func(providerName string) (provider.LLMProvider, error)

// RAGService orchestrates context retrieval, structured prompt assembly,
// and LLM generation for both synchronous and streaming RAG queries.
type RAGService struct {
	retriever        retrieval.Retriever
	llmProvider      provider.LLMProvider
	defaultModel     string
	providerResolver ProviderResolver
}

// NewRAGService constructs a new RAG orchestration service.
func NewRAGService(retriever retrieval.Retriever, llmProvider provider.LLMProvider) *RAGService {
	return &RAGService{
		retriever:    retriever,
		llmProvider:  llmProvider,
		defaultModel: "gpt-4o",
	}
}

// SetDefaultModel overrides the default LLM completion model name.
func (s *RAGService) SetDefaultModel(modelName string) {
	if modelName != "" {
		s.defaultModel = modelName
	}
}

// SetProviderResolver sets a dynamic resolver to obtain LLM providers by name.
func (s *RAGService) SetProviderResolver(resolver ProviderResolver) {
	s.providerResolver = resolver
}

func (s *RAGService) resolveProvider(reqProvider string) (provider.LLMProvider, error) {
	if s.providerResolver != nil && reqProvider != "" {
		p, err := s.providerResolver(reqProvider)
		if err != nil {
			return nil, err
		}
		if p != nil {
			return p, nil
		}
	}
	if s.llmProvider == nil {
		return nil, errors.New("no LLM provider configured")
	}
	return s.llmProvider, nil
}

// Generate executes an end-to-end synchronous RAG query:
// embeds query -> hybrid retrieval -> extracts citations -> formats prompt -> calls LLM.
func (s *RAGService) Generate(ctx context.Context, projectID uuid.UUID, req *model.ChatRequest) (*model.ChatResponse, error) {
	if err := validateChatRequest(projectID, req); err != nil {
		return nil, err
	}

	startTime := time.Now()

	topK := req.TopK
	if topK <= 0 {
		topK = 5
	}

	temp := req.Temperature
	if temp <= 0 {
		temp = 0.2
	}

	modelName := req.Model
	if modelName == "" {
		modelName = s.defaultModel
	}

	// 1. Retrieve project-scoped context chunks using Hybrid Retriever
	matches, err := s.retriever.Retrieve(
		ctx,
		projectID,
		req.Message,
		topK,
		req.SimilarityThreshold,
		req.FileFilters,
	)
	if err != nil {
		return nil, fmt.Errorf("retrieving context: %w", err)
	}

	// 2. Extract structured citations matching openapi.yaml
	citations := rag.ExtractCitations(matches)

	// 3. Format prompt with line-anchored references [1], [2], etc.
	contextPrompt := rag.FormatContextPrompt(matches)
	systemPrompt := rag.BuildSystemPrompt(contextPrompt)

	messages := []model.ChatMessage{
		{
			Role:    model.RoleSystem,
			Content: systemPrompt,
		},
		{
			Role:    model.RoleUser,
			Content: req.Message,
		},
	}

	completionReq := &model.CompletionRequest{
		Messages:    messages,
		Model:       modelName,
		Temperature: temp,
		Stream:      false,
	}

	// 4. Generate completion via LLM Provider
	llm, err := s.resolveProvider(req.Provider)
	if err != nil {
		return nil, fmt.Errorf("resolving LLM provider: %w", err)
	}

	completionResp, err := llm.GenerateCompletion(ctx, completionReq)
	if err != nil {
		return nil, fmt.Errorf("generating completion: %w", err)
	}

	durationMs := time.Since(startTime).Milliseconds()

	return &model.ChatResponse{
		Answer:     completionResp.Content,
		Citations:  citations,
		TokensUsed: completionResp.TokensUsed,
		DurationMs: durationMs,
	}, nil
}

// StreamChat streams citations and token deltas for SSE streaming completions.
// onCitation is called for each retrieved citation chunk prior to LLM generation.
// onToken is called sequentially for each token delta returned by LLMProvider.
func (s *RAGService) StreamChat(
	ctx context.Context,
	projectID uuid.UUID,
	req *model.ChatRequest,
	onCitation func(*model.Citation) error,
	onToken func(string) error,
) (*model.ChatResponse, error) {
	if err := validateChatRequest(projectID, req); err != nil {
		return nil, err
	}

	startTime := time.Now()

	topK := req.TopK
	if topK <= 0 {
		topK = 5
	}

	temp := req.Temperature
	if temp <= 0 {
		temp = 0.2
	}

	modelName := req.Model
	if modelName == "" {
		modelName = s.defaultModel
	}

	// 1. Retrieve project-scoped context chunks
	matches, err := s.retriever.Retrieve(
		ctx,
		projectID,
		req.Message,
		topK,
		req.SimilarityThreshold,
		req.FileFilters,
	)
	if err != nil {
		return nil, fmt.Errorf("retrieving context: %w", err)
	}

	// 2. Extract structured citations
	citations := rag.ExtractCitations(matches)

	// 3. Emit each citation via callback before generating tokens
	for i := range citations {
		if onCitation != nil {
			if err := onCitation(&citations[i]); err != nil {
				return nil, fmt.Errorf("citation callback error: %w", err)
			}
		}
	}

	// 4. Format context prompt and assemble messages
	contextPrompt := rag.FormatContextPrompt(matches)
	systemPrompt := rag.BuildSystemPrompt(contextPrompt)

	messages := []model.ChatMessage{
		{
			Role:    model.RoleSystem,
			Content: systemPrompt,
		},
		{
			Role:    model.RoleUser,
			Content: req.Message,
		},
	}

	completionReq := &model.CompletionRequest{
		Messages:    messages,
		Model:       modelName,
		Temperature: temp,
		Stream:      true,
	}

	// 5. Stream tokens from LLM Provider
	llm, err := s.resolveProvider(req.Provider)
	if err != nil {
		return nil, fmt.Errorf("resolving LLM provider: %w", err)
	}

	var accumulated strings.Builder
	completionResp, err := llm.StreamCompletion(ctx, completionReq, func(chunk *model.StreamChunk) error {
		if chunk.Delta != "" {
			accumulated.WriteString(chunk.Delta)
			if onToken != nil {
				if err := onToken(chunk.Delta); err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("streaming completion: %w", err)
	}

	durationMs := time.Since(startTime).Milliseconds()
	tokensUsed := 0
	answer := accumulated.String()

	if completionResp != nil {
		tokensUsed = completionResp.TokensUsed
		if answer == "" {
			answer = completionResp.Content
		}
	}

	return &model.ChatResponse{
		Answer:     answer,
		Citations:  citations,
		TokensUsed: tokensUsed,
		DurationMs: durationMs,
	}, nil
}

func validateChatRequest(projectID uuid.UUID, req *model.ChatRequest) error {
	if projectID == uuid.Nil {
		return errors.New("project ID is required")
	}
	if req == nil {
		return errors.New("chat request cannot be nil")
	}
	if strings.TrimSpace(req.Message) == "" {
		return errors.New("message cannot be empty")
	}
	return nil
}
