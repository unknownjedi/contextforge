package config_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/your-org/contextforge/internal/config"
)

const (
	validHexKey = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	validSecret = "test-jwt-secret-value-12345"
)

func TestNewDefaultConfig(t *testing.T) {
	cfg := config.NewDefaultConfig()
	require.NotNil(t, cfg)

	// Server defaults
	assert.Equal(t, 8080, cfg.Server.Port)
	assert.Equal(t, "development", cfg.Server.Env)
	assert.Equal(t, []string{"http://localhost:3000", "http://localhost:8080"}, cfg.Server.CORS.AllowedOrigins)
	assert.True(t, cfg.Server.CORS.AllowCredentials)

	// Database defaults
	assert.Equal(t, "postgres://postgres:postgres@localhost:5432/contextforge?sslmode=disable", cfg.Database.URL)
	assert.Equal(t, 25, cfg.Database.MaxOpenConns)
	assert.Equal(t, 10, cfg.Database.MaxIdleConns)

	// Redis defaults
	assert.Equal(t, "redis://localhost:6379/0", cfg.Redis.URL)

	// Auth defaults
	assert.Empty(t, cfg.Auth.JWTSecret)
	assert.Empty(t, cfg.Auth.TokenEncryptionKey)
	assert.Equal(t, 72*time.Hour, cfg.Auth.SessionExpiry)

	// Providers defaults
	assert.Equal(t, "openai", cfg.Providers.Defaults.Embedding)
	assert.Equal(t, "opencode-cli", cfg.Providers.Defaults.LLM)
	assert.Empty(t, cfg.Providers.APIKeys.OpenAI)
	assert.Empty(t, cfg.Providers.CLIPaths.OpenCodeCLI)
}

func TestLoad_DefaultsWithValidAuth(t *testing.T) {
	v := viper.New()
	t.Setenv("CF_AUTH_JWT_SECRET", validSecret)
	t.Setenv("CF_AUTH_TOKEN_ENCRYPTION_KEY", validHexKey)

	cfg, err := config.LoadWithViper(v)
	require.NoError(t, err)
	require.NotNil(t, cfg)

	// Verify defaults were retained
	assert.Equal(t, 8080, cfg.Server.Port)
	assert.Equal(t, "development", cfg.Server.Env)
	assert.Equal(t, 25, cfg.Database.MaxOpenConns)
	assert.Equal(t, 10, cfg.Database.MaxIdleConns)
	assert.Equal(t, "postgres://postgres:postgres@localhost:5432/contextforge?sslmode=disable", cfg.Database.URL)
	assert.Equal(t, "redis://localhost:6379/0", cfg.Redis.URL)
	assert.Equal(t, 72*time.Hour, cfg.Auth.SessionExpiry)
	assert.Equal(t, validSecret, cfg.Auth.JWTSecret)
	assert.Equal(t, validHexKey, cfg.Auth.TokenEncryptionKey)
	assert.Equal(t, "openai", cfg.Providers.Defaults.Embedding)
	assert.Equal(t, "opencode-cli", cfg.Providers.Defaults.LLM)
}

func TestLoad_EnvOverrides(t *testing.T) {
	v := viper.New()

	// Server overrides
	t.Setenv("CF_SERVER_PORT", "9090")
	t.Setenv("CF_SERVER_ENV", "production")

	// Database overrides
	t.Setenv("CF_DATABASE_URL", "postgres://custom:pass@dbhost:5432/customdb?sslmode=require")
	t.Setenv("CF_DATABASE_MAX_OPEN_CONNS", "50")
	t.Setenv("CF_DATABASE_MAX_IDLE_CONNS", "20")

	// Redis overrides
	t.Setenv("CF_REDIS_URL", "redis://redis-host:6380/2")

	// Auth overrides
	t.Setenv("CF_AUTH_JWT_SECRET", "custom-super-secret-jwt")
	t.Setenv("CF_AUTH_TOKEN_ENCRYPTION_KEY", "fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210")
	t.Setenv("CF_AUTH_SESSION_EXPIRY", "24h")

	// Providers overrides
	t.Setenv("CF_PROVIDERS_DEFAULTS_EMBEDDING", "gemini")
	t.Setenv("CF_PROVIDERS_DEFAULTS_LLM", "claude-cli")
	t.Setenv("CF_PROVIDERS_API_KEYS_OPENAI", "sk-proj-test12345")
	t.Setenv("CF_PROVIDERS_API_KEYS_ANTHROPIC", "sk-ant-test12345")
	t.Setenv("CF_PROVIDERS_API_KEYS_GEMINI", "gemini-api-key")
	t.Setenv("CF_PROVIDERS_API_KEYS_VOYAGE", "voyage-api-key")
	t.Setenv("CF_PROVIDERS_CLI_PATHS_OPENCODE_CLI", "/usr/local/bin/opencode")
	t.Setenv("CF_PROVIDERS_CLI_PATHS_CLAUDE_CLI", "/usr/local/bin/claude")
	t.Setenv("CF_PROVIDERS_CLI_PATHS_GEMINI_CLI", "/usr/local/bin/gemini")
	t.Setenv("CF_PROVIDERS_CLI_PATHS_CODEX_CLI", "/usr/local/bin/codex")

	cfg, err := config.LoadWithViper(v)
	require.NoError(t, err)
	require.NotNil(t, cfg)

	assert.Equal(t, 9090, cfg.Server.Port)
	assert.Equal(t, "production", cfg.Server.Env)

	assert.Equal(t, "postgres://custom:pass@dbhost:5432/customdb?sslmode=require", cfg.Database.URL)
	assert.Equal(t, 50, cfg.Database.MaxOpenConns)
	assert.Equal(t, 20, cfg.Database.MaxIdleConns)

	assert.Equal(t, "redis://redis-host:6380/2", cfg.Redis.URL)

	assert.Equal(t, "custom-super-secret-jwt", cfg.Auth.JWTSecret)
	assert.Equal(t, "fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210", cfg.Auth.TokenEncryptionKey)
	assert.Equal(t, 24*time.Hour, cfg.Auth.SessionExpiry)

	assert.Equal(t, "gemini", cfg.Providers.Defaults.Embedding)
	assert.Equal(t, "claude-cli", cfg.Providers.Defaults.LLM)
	assert.Equal(t, "sk-proj-test12345", cfg.Providers.APIKeys.OpenAI)
	assert.Equal(t, "sk-ant-test12345", cfg.Providers.APIKeys.Anthropic)
	assert.Equal(t, "gemini-api-key", cfg.Providers.APIKeys.Gemini)
	assert.Equal(t, "voyage-api-key", cfg.Providers.APIKeys.Voyage)
	assert.Equal(t, "/usr/local/bin/opencode", cfg.Providers.CLIPaths.OpenCodeCLI)
	assert.Equal(t, "/usr/local/bin/claude", cfg.Providers.CLIPaths.ClaudeCLI)
	assert.Equal(t, "/usr/local/bin/gemini", cfg.Providers.CLIPaths.GeminiCLI)
	assert.Equal(t, "/usr/local/bin/codex", cfg.Providers.CLIPaths.CodexCLI)
}

func TestLoad_FallbackEnvVars(t *testing.T) {
	v := viper.New()

	t.Setenv("PORT", "3001")
	t.Setenv("APP_ENV", "test")
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost:5432/testdb")
	t.Setenv("REDIS_URL", "redis://localhost:6379/1")
	t.Setenv("SESSION_SECRET", "fallback-session-secret")
	t.Setenv("ENCRYPTION_KEY_SECRET", validHexKey)
	t.Setenv("OPENAI_API_KEY", "fallback-openai-key")

	cfg, err := config.LoadWithViper(v)
	require.NoError(t, err)
	require.NotNil(t, cfg)

	assert.Equal(t, 3001, cfg.Server.Port)
	assert.Equal(t, "test", cfg.Server.Env)
	assert.Equal(t, "postgres://user:pass@localhost:5432/testdb", cfg.Database.URL)
	assert.Equal(t, "redis://localhost:6379/1", cfg.Redis.URL)
	assert.Equal(t, "fallback-session-secret", cfg.Auth.JWTSecret)
	assert.Equal(t, validHexKey, cfg.Auth.TokenEncryptionKey)
	assert.Equal(t, "fallback-openai-key", cfg.Providers.APIKeys.OpenAI)
}

func TestLoad_DotEnvFile(t *testing.T) {
	tmpDir := t.TempDir()
	envPath := filepath.Join(tmpDir, ".env")

	envContent := `
CF_SERVER_PORT=8888
CF_SERVER_ENV=staging
CF_AUTH_JWT_SECRET=from-dot-env-file
CF_AUTH_TOKEN_ENCRYPTION_KEY=0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
`
	err := os.WriteFile(envPath, []byte(envContent), 0600)
	require.NoError(t, err)

	v := viper.New()
	cfg, err := config.LoadWithViper(v, envPath)
	require.NoError(t, err)
	require.NotNil(t, cfg)

	assert.Equal(t, 8888, cfg.Server.Port)
	assert.Equal(t, "staging", cfg.Server.Env)
	assert.Equal(t, "from-dot-env-file", cfg.Auth.JWTSecret)
	assert.Equal(t, validHexKey, cfg.Auth.TokenEncryptionKey)
}

func TestValidation_Failures(t *testing.T) {
	tests := []struct {
		name        string
		modifyCfg   func(c *config.Config)
		errContains string
	}{
		{
			name: "empty JWT secret",
			modifyCfg: func(c *config.Config) {
				c.Auth.JWTSecret = ""
			},
			errContains: "CF_AUTH_JWT_SECRET must not be empty",
		},
		{
			name: "whitespace only JWT secret",
			modifyCfg: func(c *config.Config) {
				c.Auth.JWTSecret = "    "
			},
			errContains: "CF_AUTH_JWT_SECRET must not be empty",
		},
		{
			name: "empty token encryption key",
			modifyCfg: func(c *config.Config) {
				c.Auth.TokenEncryptionKey = ""
			},
			errContains: "CF_AUTH_TOKEN_ENCRYPTION_KEY must be a 32-byte hex string (64 hex characters)",
		},
		{
			name: "token encryption key too short (32 chars)",
			modifyCfg: func(c *config.Config) {
				c.Auth.TokenEncryptionKey = "0123456789abcdef0123456789abcdef"
			},
			errContains: "must be a 32-byte hex string (64 hex characters), got length 32",
		},
		{
			name: "token encryption key too long (65 chars)",
			modifyCfg: func(c *config.Config) {
				c.Auth.TokenEncryptionKey = validHexKey + "a"
			},
			errContains: "must be a 32-byte hex string (64 hex characters), got length 65",
		},
		{
			name: "token encryption key invalid hex characters",
			modifyCfg: func(c *config.Config) {
				// Replace last two characters with non-hex 'zz'
				c.Auth.TokenEncryptionKey = validHexKey[:62] + "zz"
			},
			errContains: "must be a valid hex string",
		},
		{
			name: "invalid server port 0",
			modifyCfg: func(c *config.Config) {
				c.Server.Port = 0
			},
			errContains: "server port must be between 1 and 65535",
		},
		{
			name: "invalid server port too high",
			modifyCfg: func(c *config.Config) {
				c.Server.Port = 70000
			},
			errContains: "server port must be between 1 and 65535",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.NewDefaultConfig()
			cfg.Auth.JWTSecret = validSecret
			cfg.Auth.TokenEncryptionKey = validHexKey

			tt.modifyCfg(cfg)
			err := cfg.Validate()
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.errContains)
		})
	}
}

func TestLoad_ValidationFailureOnMissingAuth(t *testing.T) {
	// Clean env of auth keys
	t.Setenv("CF_AUTH_JWT_SECRET", "")
	t.Setenv("CF_AUTH_TOKEN_ENCRYPTION_KEY", "")
	t.Setenv("SESSION_SECRET", "")
	t.Setenv("ENCRYPTION_KEY_SECRET", "")

	v := viper.New()
	_, err := config.LoadWithViper(v)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "configuration validation failed")
}
