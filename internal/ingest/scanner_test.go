package ingest_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/your-org/contextforge/internal/ingest"
)

func TestFileFilter_ShouldIndex(t *testing.T) {
	filter := ingest.NewFileFilter(ingest.DefaultScannerOptions())

	tests := []struct {
		path      string
		sizeBytes int64
		expected  bool
	}{
		// Valid files
		{"cmd/api/main.go", 1024, true},
		{"web/src/app/page.tsx", 2048, true},
		{"internal/crypto/encrypt.go", 4096, true},
		{"docs/overview.md", 512, true},
		{"schema.sql", 8192, true},

		// Ignored prefixes
		{".git/config", 200, false},
		{"node_modules/react/index.js", 1024, false},
		{"web/node_modules/clsx/dist/clsx.mjs", 1024, false},
		{"dist/bundle.js", 50000, false},
		{"build/output.bin", 50000, false},
		{".next/static/chunks/app.js", 20000, false},

		// Ignored extensions and lockfiles
		{"package-lock.json", 150000, false},
		{"pnpm-lock.yaml", 80000, false},
		{"go.sum", 45000, false},
		{"assets/logo.png", 12000, false},
		{"binary.exe", 100000, false},
		{"archive.tar.gz", 200000, false},

		// Exceeds max file size (> 1MB)
		{"huge_data.json", 2 * 1024 * 1024, false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			result := filter.ShouldIndex(tt.path, tt.sizeBytes)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDetectLanguage(t *testing.T) {
	assert.Equal(t, "go", ingest.DetectLanguage("main.go"))
	assert.Equal(t, "typescript", ingest.DetectLanguage("app.tsx"))
	assert.Equal(t, "javascript", ingest.DetectLanguage("index.mjs"))
	assert.Equal(t, "python", ingest.DetectLanguage("script.py"))
	assert.Equal(t, "markdown", ingest.DetectLanguage("README.md"))
	assert.Equal(t, "rust", ingest.DetectLanguage("lib.rs"))
	assert.Equal(t, "text", ingest.DetectLanguage("unknown.xyz"))
}
