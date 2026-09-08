package ingest

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"strings"
)

// ScannedFile represents a discovered source file eligible for indexing.
type ScannedFile struct {
	Path        string `json:"path"`
	Language    string `json:"language"`
	Content     string `json:"content"`
	ContentHash string `json:"content_hash"`
	SizeBytes   int64  `json:"size_bytes"`
}

// ScannerOptions configures file scanning and filtering rules.
type ScannerOptions struct {
	MaxFileSizeBytes int64    // Maximum size in bytes before skipping file (default 1MB)
	IgnoredPrefixes  []string // Directory prefixes to ignore
	IgnoredSuffixes  []string // File extensions or names to ignore
}

// DefaultScannerOptions returns production-standard ignore filters.
func DefaultScannerOptions() ScannerOptions {
	return ScannerOptions{
		MaxFileSizeBytes: 1024 * 1024, // 1MB
		IgnoredPrefixes: []string{
			".git/",
			".github/",
			"node_modules/",
			"vendor/",
			"dist/",
			"build/",
			".next/",
			"target/",
			"__pycache__/",
			".venv/",
		},
		IgnoredSuffixes: []string{
			".lock",
			"-lock.json",
			"-lock.yaml",
			".lock.yaml",
			".lockb",
			".sum",
			".exe",
			".bin",
			".zip",
			".tar",
			".gz",
			".png",
			".jpg",
			".jpeg",
			".gif",
			".ico",
			".svg",
			".woff",
			".woff2",
			".ttf",
			".pdf",
			".pyc",
			".wasm",
			".so",
			".dylib",
			".dll",
			".class",
			".jar",
			".war",
			".db",
			".sqlite",
			".sqlite3",
			".o",
			".a",
			".7z",
			".rar",
			".DS_Store",
		},
	}
}

// IsBinaryContent checks if content contains null bytes (standard Git heuristic for binary files).
func IsBinaryContent(content string) bool {
	checkLen := len(content)
	if checkLen > 8000 {
		checkLen = 8000
	}
	return strings.IndexByte(content[:checkLen], 0) != -1
}

// FileFilter determines whether a file path and size should be processed or skipped.
type FileFilter struct {
	opts ScannerOptions
}

// NewFileFilter creates a file filter with specified options.
func NewFileFilter(opts ScannerOptions) *FileFilter {
	if opts.MaxFileSizeBytes <= 0 {
		opts.MaxFileSizeBytes = 1024 * 1024
	}
	return &FileFilter{opts: opts}
}

// ShouldIndex checks if a file should be indexed based on its path and size.
func (f *FileFilter) ShouldIndex(path string, sizeBytes int64) bool {
	if sizeBytes > f.opts.MaxFileSizeBytes {
		return false
	}

	normPath := filepath.ToSlash(path)
	normPath = strings.TrimPrefix(normPath, "/")

	// Check ignored path segments / prefixes
	for _, prefix := range f.opts.IgnoredPrefixes {
		if strings.HasPrefix(normPath, prefix) || strings.Contains(normPath, "/"+prefix) {
			return false
		}
	}

	// Check ignored file extensions / suffixes
	lowerPath := strings.ToLower(normPath)
	for _, suffix := range f.opts.IgnoredSuffixes {
		if strings.HasSuffix(lowerPath, suffix) {
			return false
		}
	}

	return true
}

// DetectLanguage maps file extension to a normalized language identifier.
func DetectLanguage(filePath string) string {
	ext := strings.ToLower(filepath.Ext(filePath))
	switch ext {
	case ".go":
		return "go"
	case ".ts", ".tsx":
		return "typescript"
	case ".js", ".jsx", ".mjs", ".cjs":
		return "javascript"
	case ".py":
		return "python"
	case ".md", ".markdown":
		return "markdown"
	case ".rs":
		return "rust"
	case ".java":
		return "java"
	case ".c", ".h":
		return "c"
	case ".cpp", ".cc", ".cxx", ".hpp":
		return "cpp"
	case ".json":
		return "json"
	case ".yaml", ".yml":
		return "yaml"
	case ".sql":
		return "sql"
	case ".sh", ".bash":
		return "bash"
	case ".html", ".htm":
		return "html"
	case ".css":
		return "css"
	default:
		return "text"
	}
}

// ComputeHash returns hex SHA-256 digest of file content.
func ComputeHash(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}
