package provider

import (
	"context"
	"math"
	"testing"

	"github.com/your-org/contextforge/internal/model"
)

func TestFactory_BuiltinProviders(t *testing.T) {
	factory := NewFactory()

	t.Run("OpenAI LLM", func(t *testing.T) {
		p, err := factory.CreateLLM(ProviderOpenAI, FactoryConfig{APIKey: "test-key"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, ok := p.(*OpenAIProvider); !ok {
			t.Fatalf("expected *OpenAIProvider, got %T", p)
		}
	})

	t.Run("Anthropic LLM", func(t *testing.T) {
		p, err := factory.CreateLLM(ProviderAnthropic, FactoryConfig{APIKey: "test-key"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, ok := p.(*AnthropicProvider); !ok {
			t.Fatalf("expected *AnthropicProvider, got %T", p)
		}
	})

	t.Run("Gemini LLM", func(t *testing.T) {
		p, err := factory.CreateLLM(ProviderGemini, FactoryConfig{APIKey: "test-key"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, ok := p.(*GeminiProvider); !ok {
			t.Fatalf("expected *GeminiProvider, got %T", p)
		}
	})

	t.Run("CLI Bridges", func(t *testing.T) {
		cliTypes := []string{
			ProviderCLIOpenCode, "opencode",
			ProviderCLIClaude, "claude",
			ProviderCLIGemini,
			ProviderCLICodex, "codex",
			ProviderCLI,
		}
		for _, ct := range cliTypes {
			p, err := factory.CreateLLM(ct, FactoryConfig{BinaryPath: "echo"})
			if err != nil {
				t.Fatalf("unexpected error for %s: %v", ct, err)
			}
			if _, ok := p.(*CLIBridgeProvider); !ok {
				t.Fatalf("expected *CLIBridgeProvider for %s, got %T", ct, p)
			}
		}
	})

	t.Run("Mock LLM", func(t *testing.T) {
		p, err := factory.CreateLLM(ProviderMock, FactoryConfig{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, ok := p.(*MockLLMProvider); !ok {
			t.Fatalf("expected *MockLLMProvider, got %T", p)
		}
	})

	t.Run("Ollama LLM", func(t *testing.T) {
		p, err := factory.CreateLLM(ProviderOllama, FactoryConfig{Model: "qwen3.5:latest"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, ok := p.(*OllamaLLMProvider); !ok {
			t.Fatalf("expected *OllamaLLMProvider, got %T", p)
		}
	})

	t.Run("OpenAI Embedding", func(t *testing.T) {
		p, err := factory.CreateEmbedding(ProviderOpenAI, FactoryConfig{APIKey: "test-key"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, ok := p.(*OpenAIEmbeddingProvider); !ok {
			t.Fatalf("expected *OpenAIEmbeddingProvider, got %T", p)
		}
	})

	t.Run("Gemini Embedding", func(t *testing.T) {
		p, err := factory.CreateEmbedding(ProviderGemini, FactoryConfig{APIKey: "test-key"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, ok := p.(*GeminiProvider); !ok {
			t.Fatalf("expected *GeminiProvider, got %T", p)
		}
	})

	t.Run("Ollama Embedding", func(t *testing.T) {
		p, err := factory.CreateEmbedding(ProviderOllama, FactoryConfig{Dimension: 768})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, ok := p.(*OllamaEmbeddingProvider); !ok {
			t.Fatalf("expected *OllamaEmbeddingProvider, got %T", p)
		}
	})

	t.Run("Mock Embedding", func(t *testing.T) {
		p, err := factory.CreateEmbedding(ProviderMock, FactoryConfig{Dimension: 384})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if mockEmb, ok := p.(*MockEmbeddingProvider); !ok {
			t.Fatalf("expected *MockEmbeddingProvider, got %T", p)
		} else if mockEmb.Dimension() != 384 {
			t.Fatalf("expected dimension 384, got %d", mockEmb.Dimension())
		}
	})

	t.Run("Unsupported LLM Provider", func(t *testing.T) {
		_, err := factory.CreateLLM("unknown-provider", FactoryConfig{})
		if err == nil {
			t.Fatal("expected error for unsupported provider, got nil")
		}
	})

	t.Run("OpenAI LLM Missing Key", func(t *testing.T) {
		_, err := factory.CreateLLM(ProviderOpenAI, FactoryConfig{})
		if err == nil {
			t.Fatal("expected error for missing API key, got nil")
		}
	})

	t.Run("OpenAI Embedding Missing Key", func(t *testing.T) {
		_, err := factory.CreateEmbedding(ProviderOpenAI, FactoryConfig{})
		if err == nil {
			t.Fatal("expected error for missing API key, got nil")
		}
	})

	t.Run("Unsupported Embedding Provider", func(t *testing.T) {
		_, err := factory.CreateEmbedding("unknown-provider", FactoryConfig{})
		if err == nil {
			t.Fatal("expected error for unsupported provider, got nil")
		}
	})
}

func TestFactory_CustomRegistration(t *testing.T) {
	factory := NewFactory()

	factory.RegisterLLM("custom-llm", func(cfg FactoryConfig) (LLMProvider, error) {
		return NewMockLLMProvider("custom-response"), nil
	})

	p, err := factory.CreateLLM("custom-llm", FactoryConfig{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp, err := p.GenerateCompletion(context.Background(), &model.CompletionRequest{
		Messages: []model.ChatMessage{{Role: "user", Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "custom-response" {
		t.Fatalf("expected 'custom-response', got %q", resp.Content)
	}

	factory.RegisterEmbedding("custom-embed", func(cfg FactoryConfig) (EmbeddingProvider, error) {
		return NewMockEmbeddingProvider(512), nil
	})

	ep, err := factory.CreateEmbedding("custom-embed", FactoryConfig{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	vec, err := ep.EmbedQuery(context.Background(), "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(vec) != 512 {
		t.Fatalf("expected vector length 512, got %d", len(vec))
	}
}

func TestFactory_PackageLevelFunctions(t *testing.T) {
	llm, err := NewLLMProvider(FactoryConfig{Type: ProviderMock})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if llm == nil {
		t.Fatal("expected non-nil LLMProvider")
	}

	emb, err := NewEmbeddingProvider(FactoryConfig{Type: ProviderMock, Dimension: 768})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if emb == nil {
		t.Fatal("expected non-nil EmbeddingProvider")
	}

	llmByName, err := NewLLMProviderByName(ProviderMock, FactoryConfig{})
	if err != nil || llmByName == nil {
		t.Fatalf("unexpected error: %v", err)
	}

	embByName, err := NewEmbeddingProviderByName(ProviderMock, FactoryConfig{Dimension: 768})
	if err != nil || embByName == nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMockLLMProvider_GenerateAndStream(t *testing.T) {
	mock := NewMockLLMProvider("hello ContextForge world")

	ctx := context.Background()
	req := &model.CompletionRequest{
		Messages: []model.ChatMessage{
			{Role: "user", Content: "hi"},
		},
		Model: "test-model",
	}

	// Non-streaming
	resp, err := mock.GenerateCompletion(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "hello ContextForge world" {
		t.Fatalf("expected 'hello ContextForge world', got %q", resp.Content)
	}
	if resp.Model != "test-model" {
		t.Fatalf("expected 'test-model', got %q", resp.Model)
	}

	// Streaming
	var chunks []string
	var doneReceived bool
	streamResp, err := mock.StreamCompletion(ctx, req, func(chunk *model.StreamChunk) error {
		if chunk.Done {
			doneReceived = true
		} else {
			chunks = append(chunks, chunk.Delta)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !doneReceived {
		t.Fatal("expected done=true chunk")
	}
	if streamResp.Content != "hello ContextForge world" {
		t.Fatalf("expected accumulated content 'hello ContextForge world', got %q", streamResp.Content)
	}
	if len(chunks) != 3 {
		t.Fatalf("expected 3 word chunks, got %d: %v", len(chunks), chunks)
	}
}

func TestMockEmbeddingProvider(t *testing.T) {
	dim := 768
	mock := NewMockEmbeddingProvider(dim)

	if mock.Dimension() != dim {
		t.Fatalf("expected dim %d, got %d", dim, mock.Dimension())
	}

	ctx := context.Background()
	texts := []string{"ContextForge", "RAG Pipeline"}
	embeddings, err := mock.EmbedDocuments(ctx, texts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(embeddings) != 2 {
		t.Fatalf("expected 2 embeddings, got %d", len(embeddings))
	}
	for i, emb := range embeddings {
		if len(emb) != dim {
			t.Fatalf("expected embedding %d to have length %d, got %d", i, dim, len(emb))
		}
		// Verify normalization (L2 norm ~ 1.0)
		var sumSq float64
		for _, v := range emb {
			sumSq += float64(v * v)
		}
		norm := math.Sqrt(sumSq)
		if math.Abs(norm-1.0) > 1e-4 {
			t.Fatalf("expected unit norm, got %f", norm)
		}
	}

	// Verify deterministic
	embeddings2, _ := mock.EmbedDocuments(ctx, texts)
	for i := range embeddings[0] {
		if embeddings[0][i] != embeddings2[0][i] {
			t.Fatalf("expected deterministic embeddings, differed at %d", i)
		}
	}

	// EmbedQuery
	queryVec, err := mock.EmbedQuery(ctx, "ContextForge")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(queryVec) != dim {
		t.Fatalf("expected query vector length %d, got %d", dim, len(queryVec))
	}
}
