package provider

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Supported provider identifiers.
const (
	ProviderOpenAI      = "openai"
	ProviderAnthropic   = "anthropic"
	ProviderGemini      = "gemini"
	ProviderOllama      = "ollama"
	ProviderCLIOpenCode = "cli_opencode"
	ProviderCLIClaude   = "cli_claude"
	ProviderCLIGemini   = "cli_gemini"
	ProviderCLICodex    = "cli_codex"
	ProviderCLI         = "cli"
	ProviderMock        = "mock"
)

// FactoryConfig contains all configuration parameters to instantiate any LLM or Embedding provider.
type FactoryConfig struct {
	Type               string
	APIKey             string
	BaseURL            string
	Model              string
	EmbeddingModel     string
	Dimension          int
	BinaryPath         string
	Args               []string
	PassPromptViaStdin bool
	Timeout            time.Duration
	HTTPClient         *http.Client
	Retry              RetryConfig
}

// LLMConstructor defines a factory function to instantiate an LLMProvider.
type LLMConstructor func(cfg FactoryConfig) (LLMProvider, error)

// EmbeddingConstructor defines a factory function to instantiate an EmbeddingProvider.
type EmbeddingConstructor func(cfg FactoryConfig) (EmbeddingProvider, error)

// Factory manages registered LLM and Embedding provider constructors.
type Factory struct {
	mu                sync.RWMutex
	llmConstructors   map[string]LLMConstructor
	embedConstructors map[string]EmbeddingConstructor
}

// NewFactory initializes a new Provider Factory with all built-in drivers registered.
func NewFactory() *Factory {
	f := &Factory{
		llmConstructors:   make(map[string]LLMConstructor),
		embedConstructors: make(map[string]EmbeddingConstructor),
	}
	f.registerDefaults()
	return f
}

// registerDefaults sets up built-in drivers.
func (f *Factory) registerDefaults() {
	// OpenAI LLM
	f.RegisterLLM(ProviderOpenAI, func(cfg FactoryConfig) (LLMProvider, error) {
		return NewOpenAIProvider(OpenAIConfig{
			APIKey:                cfg.APIKey,
			BaseURL:               cfg.BaseURL,
			DefaultModel:          cfg.Model,
			DefaultEmbeddingModel: cfg.EmbeddingModel,
			HTTPClient:            cfg.HTTPClient,
			Retry:                 cfg.Retry,
		}), nil
	})

	// Anthropic LLM
	f.RegisterLLM(ProviderAnthropic, func(cfg FactoryConfig) (LLMProvider, error) {
		return NewAnthropicProvider(AnthropicConfig{
			APIKey:       cfg.APIKey,
			BaseURL:      cfg.BaseURL,
			DefaultModel: cfg.Model,
			HTTPClient:   cfg.HTTPClient,
			Retry:        cfg.Retry,
		}), nil
	})

	// Gemini LLM
	f.RegisterLLM(ProviderGemini, func(cfg FactoryConfig) (LLMProvider, error) {
		return NewGeminiProvider(GeminiConfig{
			APIKey:                cfg.APIKey,
			BaseURL:               cfg.BaseURL,
			DefaultModel:          cfg.Model,
			DefaultEmbeddingModel: cfg.EmbeddingModel,
			HTTPClient:            cfg.HTTPClient,
			Retry:                 cfg.Retry,
		}), nil
	})

	// CLI bridges
	f.RegisterLLM(ProviderCLIOpenCode, func(cfg FactoryConfig) (LLMProvider, error) {
		return NewOpenCodeCLIProvider(cfg.BinaryPath, cfg.Timeout), nil
	})
	f.RegisterLLM("opencode", func(cfg FactoryConfig) (LLMProvider, error) {
		return NewOpenCodeCLIProvider(cfg.BinaryPath, cfg.Timeout), nil
	})
	f.RegisterLLM("opencode-cli", func(cfg FactoryConfig) (LLMProvider, error) {
		return NewOpenCodeCLIProvider(cfg.BinaryPath, cfg.Timeout), nil
	})

	f.RegisterLLM(ProviderCLIClaude, func(cfg FactoryConfig) (LLMProvider, error) {
		return NewClaudeCLIProvider(cfg.BinaryPath, cfg.Timeout), nil
	})
	f.RegisterLLM("claude", func(cfg FactoryConfig) (LLMProvider, error) {
		return NewClaudeCLIProvider(cfg.BinaryPath, cfg.Timeout), nil
	})
	f.RegisterLLM("claude-cli", func(cfg FactoryConfig) (LLMProvider, error) {
		return NewClaudeCLIProvider(cfg.BinaryPath, cfg.Timeout), nil
	})

	f.RegisterLLM(ProviderCLIGemini, func(cfg FactoryConfig) (LLMProvider, error) {
		return NewGeminiCLIProvider(cfg.BinaryPath, cfg.Timeout), nil
	})
	f.RegisterLLM("gemini-cli", func(cfg FactoryConfig) (LLMProvider, error) {
		return NewGeminiCLIProvider(cfg.BinaryPath, cfg.Timeout), nil
	})

	f.RegisterLLM(ProviderCLICodex, func(cfg FactoryConfig) (LLMProvider, error) {
		return NewCodexCLIProvider(cfg.BinaryPath, cfg.Timeout), nil
	})
	f.RegisterLLM("codex", func(cfg FactoryConfig) (LLMProvider, error) {
		return NewCodexCLIProvider(cfg.BinaryPath, cfg.Timeout), nil
	})
	f.RegisterLLM("codex-cli", func(cfg FactoryConfig) (LLMProvider, error) {
		return NewCodexCLIProvider(cfg.BinaryPath, cfg.Timeout), nil
	})

	f.RegisterLLM(ProviderCLI, func(cfg FactoryConfig) (LLMProvider, error) {
		return NewCLIBridgeProvider(CLIBridgeConfig{
			BinaryPath:         cfg.BinaryPath,
			Args:               cfg.Args,
			PassPromptViaStdin: cfg.PassPromptViaStdin,
			Timeout:            cfg.Timeout,
		}), nil
	})

	// Mock LLM
	f.RegisterLLM(ProviderMock, func(cfg FactoryConfig) (LLMProvider, error) {
		return NewMockLLMProvider("mock completion"), nil
	})

	// Embedding providers
	// OpenAI Embedding
	f.RegisterEmbedding(ProviderOpenAI, func(cfg FactoryConfig) (EmbeddingProvider, error) {
		return NewOpenAIEmbeddingProvider(OpenAIConfig{
			APIKey:                cfg.APIKey,
			BaseURL:               cfg.BaseURL,
			DefaultModel:          cfg.Model,
			DefaultEmbeddingModel: cfg.EmbeddingModel,
			HTTPClient:            cfg.HTTPClient,
			Retry:                 cfg.Retry,
		}), nil
	})

	// Gemini Embedding
	f.RegisterEmbedding(ProviderGemini, func(cfg FactoryConfig) (EmbeddingProvider, error) {
		return NewGeminiProvider(GeminiConfig{
			APIKey:                cfg.APIKey,
			BaseURL:               cfg.BaseURL,
			DefaultModel:          cfg.Model,
			DefaultEmbeddingModel: cfg.EmbeddingModel,
			HTTPClient:            cfg.HTTPClient,
			Retry:                 cfg.Retry,
		}), nil
	})

	// Ollama Embedding
	f.RegisterEmbedding(ProviderOllama, func(cfg FactoryConfig) (EmbeddingProvider, error) {
		return NewOllamaEmbeddingProvider(OllamaConfig{
			BaseURL:    cfg.BaseURL,
			Model:      cfg.EmbeddingModel,
			Dimension:  cfg.Dimension,
			HTTPClient: cfg.HTTPClient,
		}), nil
	})

	// Mock Embedding
	f.RegisterEmbedding(ProviderMock, func(cfg FactoryConfig) (EmbeddingProvider, error) {
		dim := cfg.Dimension
		if dim <= 0 {
			dim = 768
		}
		return NewMockEmbeddingProvider(dim), nil
	})
}

// RegisterLLM registers a new constructor for an LLM provider type.
func (f *Factory) RegisterLLM(name string, constructor LLMConstructor) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.llmConstructors[strings.ToLower(name)] = constructor
}

// RegisterEmbedding registers a new constructor for an Embedding provider type.
func (f *Factory) RegisterEmbedding(name string, constructor EmbeddingConstructor) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.embedConstructors[strings.ToLower(name)] = constructor
}

// CreateLLM instantiates an LLMProvider based on the provider type.
func (f *Factory) CreateLLM(name string, cfg FactoryConfig) (LLMProvider, error) {
	f.mu.RLock()
	constructor, exists := f.llmConstructors[strings.ToLower(name)]
	f.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("unsupported LLM provider: %q", name)
	}

	return constructor(cfg)
}

// CreateEmbedding instantiates an EmbeddingProvider based on the provider type.
func (f *Factory) CreateEmbedding(name string, cfg FactoryConfig) (EmbeddingProvider, error) {
	f.mu.RLock()
	constructor, exists := f.embedConstructors[strings.ToLower(name)]
	f.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("unsupported embedding provider: %q", name)
	}

	return constructor(cfg)
}

// Default package-level factory instance.
var defaultFactory = NewFactory()

// RegisterLLMProvider registers an LLM constructor on the default factory.
func RegisterLLMProvider(name string, constructor LLMConstructor) {
	defaultFactory.RegisterLLM(name, constructor)
}

// RegisterEmbeddingProvider registers an Embedding constructor on the default factory.
func RegisterEmbeddingProvider(name string, constructor EmbeddingConstructor) {
	defaultFactory.RegisterEmbedding(name, constructor)
}

// NewLLMProvider constructs an LLMProvider using cfg.Type from the default factory.
func NewLLMProvider(cfg FactoryConfig) (LLMProvider, error) {
	return defaultFactory.CreateLLM(cfg.Type, cfg)
}

// NewLLMProviderByName constructs an LLMProvider by explicit name from the default factory.
func NewLLMProviderByName(name string, cfg FactoryConfig) (LLMProvider, error) {
	return defaultFactory.CreateLLM(name, cfg)
}

// NewEmbeddingProvider constructs an EmbeddingProvider using cfg.Type from the default factory.
func NewEmbeddingProvider(cfg FactoryConfig) (EmbeddingProvider, error) {
	return defaultFactory.CreateEmbedding(cfg.Type, cfg)
}

// NewEmbeddingProviderByName constructs an EmbeddingProvider by explicit name from the default factory.
func NewEmbeddingProviderByName(name string, cfg FactoryConfig) (EmbeddingProvider, error) {
	return defaultFactory.CreateEmbedding(name, cfg)
}
