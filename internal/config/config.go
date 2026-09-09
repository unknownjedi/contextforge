package config

import (
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/viper"
	"github.com/subosito/gotenv"
)

// Config holds all configuration for the ContextForge application.
type Config struct {
	Server    ServerConfig    `mapstructure:"server" json:"server"`
	Database  DatabaseConfig  `mapstructure:"database" json:"database"`
	Redis     RedisConfig     `mapstructure:"redis" json:"redis"`
	Auth      AuthConfig      `mapstructure:"auth" json:"auth"`
	Providers ProvidersConfig `mapstructure:"providers" json:"providers"`
}

// ServerConfig contains HTTP server configuration.
type ServerConfig struct {
	Port int        `mapstructure:"port" json:"port"`
	Env  string     `mapstructure:"env" json:"env"`
	CORS CORSConfig `mapstructure:"cors" json:"cors"`
}

// CORSConfig contains Cross-Origin Resource Sharing settings.
type CORSConfig struct {
	AllowedOrigins   []string `mapstructure:"allowed_origins" json:"allowed_origins"`
	AllowedMethods   []string `mapstructure:"allowed_methods" json:"allowed_methods"`
	AllowedHeaders   []string `mapstructure:"allowed_headers" json:"allowed_headers"`
	AllowCredentials bool     `mapstructure:"allow_credentials" json:"allow_credentials"`
}

// DatabaseConfig contains PostgreSQL connection settings.
type DatabaseConfig struct {
	URL          string `mapstructure:"url" json:"url"`
	MaxOpenConns int    `mapstructure:"max_open_conns" json:"max_open_conns"`
	MaxIdleConns int    `mapstructure:"max_idle_conns" json:"max_idle_conns"`
}

// RedisConfig contains Redis connection settings.
type RedisConfig struct {
	URL string `mapstructure:"url" json:"url"`
}

// AuthConfig contains authentication and cryptography settings.
type AuthConfig struct {
	JWTSecret          string        `mapstructure:"jwt_secret" json:"jwt_secret"`
	TokenEncryptionKey string        `mapstructure:"token_encryption_key" json:"token_encryption_key"`
	WebhookSecret      string        `mapstructure:"webhook_secret" json:"webhook_secret"`
	SessionExpiry      time.Duration `mapstructure:"session_expiry" json:"session_expiry"`
	GithubPAT          string        `mapstructure:"github_pat" json:"-"`
}

// ProvidersConfig contains LLM and embedding provider settings.
type ProvidersConfig struct {
	Defaults ProvidersDefaultsConfig `mapstructure:"defaults" json:"defaults"`
	APIKeys  ProvidersAPIKeysConfig  `mapstructure:"api_keys" json:"api_keys"`
	CLIPaths ProvidersCLIPathsConfig `mapstructure:"cli_paths" json:"cli_paths"`
}

// ProvidersDefaultsConfig holds default provider names.
type ProvidersDefaultsConfig struct {
	Embedding          string `mapstructure:"embedding" json:"embedding"`
	EmbeddingModel     string `mapstructure:"embedding_model" json:"embedding_model"`
	EmbeddingDimension int    `mapstructure:"embedding_dimensions" json:"embedding_dimensions"`
	OllamaBaseURL      string `mapstructure:"ollama_base_url" json:"ollama_base_url"`
	LLM                string `mapstructure:"llm" json:"llm"`
	LLMModel           string `mapstructure:"llm_model" json:"llm_model"`
}

// ProvidersAPIKeysConfig holds cloud provider API keys.
type ProvidersAPIKeysConfig struct {
	OpenAI    string `mapstructure:"openai" json:"openai"`
	Anthropic string `mapstructure:"anthropic" json:"anthropic"`
	Gemini    string `mapstructure:"gemini" json:"gemini"`
	Voyage    string `mapstructure:"voyage" json:"voyage"`
}

// ProvidersCLIPathsConfig holds local CLI tool paths.
type ProvidersCLIPathsConfig struct {
	OpenCodeCLI string `mapstructure:"opencode_cli" json:"opencode_cli"`
	ClaudeCLI   string `mapstructure:"claude_cli" json:"claude_cli"`
	GeminiCLI   string `mapstructure:"gemini_cli" json:"gemini_cli"`
	CodexCLI    string `mapstructure:"codex_cli" json:"codex_cli"`
}

// NewDefaultConfig returns a Config struct populated with sensible defaults.
func NewDefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Port: 8080,
			Env:  "development",
			CORS: CORSConfig{
				AllowedOrigins:   []string{"http://localhost:3000", "http://localhost:8080"},
				AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"},
				AllowedHeaders:   []string{"Origin", "Content-Type", "Accept", "Authorization", "X-Correlation-ID", "X-Request-ID"},
				AllowCredentials: true,
			},
		},
		Database: DatabaseConfig{
			URL:          "postgres://postgres:postgres@localhost:5432/contextforge?sslmode=disable",
			MaxOpenConns: 25,
			MaxIdleConns: 10,
		},
		Redis: RedisConfig{
			URL: "redis://localhost:6379/0",
		},
		Auth: AuthConfig{
			JWTSecret:          "",
			TokenEncryptionKey: "",
			WebhookSecret:      "",
			SessionExpiry:      72 * time.Hour,
			GithubPAT:          "",
		},
		Providers: ProvidersConfig{
			Defaults: ProvidersDefaultsConfig{
				Embedding:          "openai",
				EmbeddingModel:     "text-embedding-3-small",
				EmbeddingDimension: 1536,
				OllamaBaseURL:      "http://localhost:11434",
				LLM:                "opencode-cli",
				LLMModel:           "default",
			},
			APIKeys: ProvidersAPIKeysConfig{
				OpenAI:    "",
				Anthropic: "",
				Gemini:    "",
				Voyage:    "",
			},
			CLIPaths: ProvidersCLIPathsConfig{
				OpenCodeCLI: "",
				ClaudeCLI:   "",
				GeminiCLI:   "",
				CodexCLI:    "",
			},
		},
	}
}

// Load loads configuration from environment variables (prefixed with CF_) and an optional .env file.
// If filePaths are provided, the first readable file will be loaded. Otherwise, it checks for ".env" in ".".
func Load(filePaths ...string) (*Config, error) {
	v := viper.New()
	return LoadWithViper(v, filePaths...)
}

// LoadWithViper allows passing a specific Viper instance (helpful for isolated testing).
func LoadWithViper(v *viper.Viper, filePaths ...string) (*Config, error) {
	// Attempt to load .env file into environment if present
	envFileFound := false
	if len(filePaths) > 0 {
		for _, fp := range filePaths {
			if _, err := os.Stat(fp); err == nil {
				_ = gotenv.Load(fp)
				v.SetConfigFile(fp)
				_ = v.ReadInConfig()
				envFileFound = true
				break
			}
		}
	}

	if !envFileFound {
		if _, err := os.Stat(".env"); err == nil {
			_ = gotenv.Load(".env")
			v.SetConfigFile(".env")
			_ = v.ReadInConfig()
		}
	}

	// Environment variable settings
	v.SetEnvPrefix("CF")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// Register defaults
	defaults := NewDefaultConfig()
	setDefaults(v, defaults)

	// Bind environment variables explicitly to ensure Viper maps env vars into nested structs
	bindEnvVars(v)

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal configuration: %w", err)
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %w", err)
	}

	return &cfg, nil
}

func setDefaults(v *viper.Viper, d *Config) {
	// Server
	v.SetDefault("server.port", d.Server.Port)
	v.SetDefault("server.env", d.Server.Env)
	v.SetDefault("server.cors.allowed_origins", d.Server.CORS.AllowedOrigins)
	v.SetDefault("server.cors.allowed_methods", d.Server.CORS.AllowedMethods)
	v.SetDefault("server.cors.allowed_headers", d.Server.CORS.AllowedHeaders)
	v.SetDefault("server.cors.allow_credentials", d.Server.CORS.AllowCredentials)

	// Database
	v.SetDefault("database.url", d.Database.URL)
	v.SetDefault("database.max_open_conns", d.Database.MaxOpenConns)
	v.SetDefault("database.max_idle_conns", d.Database.MaxIdleConns)

	// Redis
	v.SetDefault("redis.url", d.Redis.URL)

	// Auth
	v.SetDefault("auth.jwt_secret", d.Auth.JWTSecret)
	v.SetDefault("auth.token_encryption_key", d.Auth.TokenEncryptionKey)
	v.SetDefault("auth.session_expiry", d.Auth.SessionExpiry)
	v.SetDefault("auth.github_pat", d.Auth.GithubPAT)

	// Providers
	v.SetDefault("providers.defaults.embedding", d.Providers.Defaults.Embedding)
	v.SetDefault("providers.defaults.embedding_model", d.Providers.Defaults.EmbeddingModel)
	v.SetDefault("providers.defaults.embedding_dimensions", d.Providers.Defaults.EmbeddingDimension)
	v.SetDefault("providers.defaults.ollama_base_url", d.Providers.Defaults.OllamaBaseURL)
	v.SetDefault("providers.defaults.llm", d.Providers.Defaults.LLM)
	v.SetDefault("providers.defaults.llm_model", d.Providers.Defaults.LLMModel)
	v.SetDefault("providers.api_keys.openai", d.Providers.APIKeys.OpenAI)
	v.SetDefault("providers.api_keys.anthropic", d.Providers.APIKeys.Anthropic)
	v.SetDefault("providers.api_keys.gemini", d.Providers.APIKeys.Gemini)
	v.SetDefault("providers.api_keys.voyage", d.Providers.APIKeys.Voyage)
	v.SetDefault("providers.cli_paths.opencode_cli", d.Providers.CLIPaths.OpenCodeCLI)
	v.SetDefault("providers.cli_paths.claude_cli", d.Providers.CLIPaths.ClaudeCLI)
	v.SetDefault("providers.cli_paths.gemini_cli", d.Providers.CLIPaths.GeminiCLI)
	v.SetDefault("providers.cli_paths.codex_cli", d.Providers.CLIPaths.CodexCLI)
}

func bindEnvVars(v *viper.Viper) {
	// Server
	_ = v.BindEnv("server.port", "CF_SERVER_PORT", "PORT")
	_ = v.BindEnv("server.env", "CF_SERVER_ENV", "CF_APP_ENV", "APP_ENV")
	_ = v.BindEnv("server.cors.allowed_origins", "CF_SERVER_CORS_ALLOWED_ORIGINS")
	_ = v.BindEnv("server.cors.allowed_methods", "CF_SERVER_CORS_ALLOWED_METHODS")
	_ = v.BindEnv("server.cors.allowed_headers", "CF_SERVER_CORS_ALLOWED_HEADERS")
	_ = v.BindEnv("server.cors.allow_credentials", "CF_SERVER_CORS_ALLOW_CREDENTIALS")

	// Database
	_ = v.BindEnv("database.url", "CF_DATABASE_URL", "DATABASE_URL")
	_ = v.BindEnv("database.max_open_conns", "CF_DATABASE_MAX_OPEN_CONNS", "DB_MAX_OPEN_CONNS")
	_ = v.BindEnv("database.max_idle_conns", "CF_DATABASE_MAX_IDLE_CONNS", "DB_MAX_IDLE_CONNS")

	// Redis
	_ = v.BindEnv("redis.url", "CF_REDIS_URL", "REDIS_URL")

	// Auth
	_ = v.BindEnv("auth.jwt_secret", "CF_AUTH_JWT_SECRET", "SESSION_SECRET", "JWT_SECRET")
	_ = v.BindEnv("auth.token_encryption_key", "CF_AUTH_TOKEN_ENCRYPTION_KEY", "ENCRYPTION_KEY_SECRET")
	_ = v.BindEnv("auth.session_expiry", "CF_AUTH_SESSION_EXPIRY")
	_ = v.BindEnv("auth.github_pat", "CF_AUTH_GITHUB_PAT", "GITHUB_PAT", "CF_GITHUB_PAT_FALLBACK")

	// Providers
	_ = v.BindEnv("providers.defaults.embedding", "CF_PROVIDERS_DEFAULTS_EMBEDDING", "EMBEDDING_PROVIDER")
	_ = v.BindEnv("providers.defaults.embedding_model", "CF_EMBEDDING_MODEL", "EMBEDDING_MODEL")
	_ = v.BindEnv("providers.defaults.embedding_dimensions", "CF_EMBEDDING_DIMENSIONS", "EMBEDDING_DIMENSIONS")
	_ = v.BindEnv("providers.defaults.ollama_base_url", "CF_OLLAMA_BASE_URL", "OLLAMA_BASE_URL")
	_ = v.BindEnv("providers.defaults.llm", "CF_PROVIDERS_DEFAULTS_LLM", "LLM_PROVIDER")
	_ = v.BindEnv("providers.defaults.llm_model", "CF_LLM_MODEL", "LLM_MODEL")
	_ = v.BindEnv("providers.api_keys.openai", "CF_PROVIDERS_API_KEYS_OPENAI", "OPENAI_API_KEY")
	_ = v.BindEnv("providers.api_keys.anthropic", "CF_PROVIDERS_API_KEYS_ANTHROPIC", "ANTHROPIC_API_KEY")
	_ = v.BindEnv("providers.api_keys.gemini", "CF_PROVIDERS_API_KEYS_GEMINI", "GEMINI_API_KEY")
	_ = v.BindEnv("providers.api_keys.voyage", "CF_PROVIDERS_API_KEYS_VOYAGE", "VOYAGE_API_KEY")
	_ = v.BindEnv("providers.cli_paths.opencode_cli", "CF_PROVIDERS_CLI_PATHS_OPENCODE_CLI")
	_ = v.BindEnv("providers.cli_paths.claude_cli", "CF_PROVIDERS_CLI_PATHS_CLAUDE_CLI")
	_ = v.BindEnv("providers.cli_paths.gemini_cli", "CF_PROVIDERS_CLI_PATHS_GEMINI_CLI")
	_ = v.BindEnv("providers.cli_paths.codex_cli", "CF_PROVIDERS_CLI_PATHS_CODEX_CLI")
}

// Validate ensures all mandatory configurations are valid.
func (c *Config) Validate() error {
	// Validate CF_AUTH_JWT_SECRET is non-empty
	if strings.TrimSpace(c.Auth.JWTSecret) == "" {
		return errors.New("CF_AUTH_JWT_SECRET must not be empty")
	}

	// Validate CF_AUTH_TOKEN_ENCRYPTION_KEY is a valid 32-byte hex string (64 hex characters)
	if len(c.Auth.TokenEncryptionKey) != 64 {
		return fmt.Errorf("CF_AUTH_TOKEN_ENCRYPTION_KEY must be a 32-byte hex string (64 hex characters), got length %d", len(c.Auth.TokenEncryptionKey))
	}
	decoded, err := hex.DecodeString(c.Auth.TokenEncryptionKey)
	if err != nil {
		return fmt.Errorf("CF_AUTH_TOKEN_ENCRYPTION_KEY must be a valid hex string: %w", err)
	}
	if len(decoded) != 32 {
		return fmt.Errorf("CF_AUTH_TOKEN_ENCRYPTION_KEY decoded byte length must be 32, got %d", len(decoded))
	}

	// Validate Server port
	if c.Server.Port <= 0 || c.Server.Port > 65535 {
		return fmt.Errorf("server port must be between 1 and 65535, got %d", c.Server.Port)
	}

	return nil
}
