package urlfetch

import (
	"context"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSSRFProtection(t *testing.T) {
	blockedCases := []struct {
		name string
		url  string
	}{
		{"localhost", "http://localhost:8080/api"},
		{"localhost uppercase", "HTTP://LOCALHOST/admin"},
		{"localhost localdomain", "http://localhost.localdomain/secret"},
		{"internal domain", "http://service.internal/status"},
		{"local domain", "http://myhost.local/info"},
		{"localhost subdomain", "http://app.localhost:3000"},
		{"google cloud metadata hostname", "http://metadata.google.internal/computeMetadata/v1/"},
		{"aws metadata hostname", "http://instance-data/latest/meta-data/"},
		{"loopback IPv4 127.0.0.1", "http://127.0.0.1:8080"},
		{"loopback IPv4 127.0.0.2", "http://127.0.0.2:9000"},
		{"loopback IPv6", "http://[::1]:8080/foo"},
		{"rfc1918 10.x", "http://10.0.0.1/admin"},
		{"rfc1918 172.16.x", "http://172.16.0.1/dashboard"},
		{"rfc1918 192.168.x", "http://192.168.1.1/"},
		{"link-local metadata 169.254.169.254", "http://169.254.169.254/latest/meta-data/"},
		{"link-local IPv6", "http://[fe80::1]/test"},
		{"ula IPv6", "http://[fc00::1]/internal"},
		{"ftp scheme", "ftp://example.com/file.txt"},
		{"file scheme", "file:///etc/passwd"},
		{"gopher scheme", "gopher://example.com/7"},
		{"javascript scheme", "javascript:alert(1)"},
		{"empty host", "http://"},
		{"empty url", ""},
		{"whitespace url", "   "},
	}

	for _, tc := range blockedCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ValidateURL(tc.url)
			assert.Error(t, err, "expected error for URL: %s", tc.url)
		})
	}
}

func TestSSRFProtection_AllowedURLs(t *testing.T) {
	origLookup := lookupIP
	defer func() { lookupIP = origLookup }()
	lookupIP = func(host string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("93.184.216.34")}, nil
	}

	allowedCases := []string{
		"https://example.com",
		"http://example.com/docs",
		"https://github.com/your-org/contextforge",
	}

	for _, u := range allowedCases {
		t.Run(u, func(t *testing.T) {
			parsed, err := ValidateURL(u)
			require.NoError(t, err)
			assert.NotEmpty(t, parsed.Host)
		})
	}
}

func TestHTMLToMarkdownParser(t *testing.T) {
	rawHTML := `<!DOCTYPE html>
<html>
<head>
    <title>ContextForge Guide &amp; Docs</title>
    <style>body { background: #fff; } .hide { display: none; }</style>
    <script>alert("malicious script");</script>
</head>
<body>
    <noscript><p>Please enable JS</p></noscript>
    <svg><circle cx="50" cy="50" r="40" /></svg>
    
    <h1>ContextForge Documentation</h1>
    <p>Welcome to <strong>ContextForge</strong>, an <em>enterprise</em> context engine.</p>
    
    <h2>Features</h2>
    <ul>
        <li>Fast chunking and indexing</li>
        <li>SSRF protection</li>
        <li>Hybrid vector search</li>
    </ul>

    <h3>Quick Steps</h3>
    <ol>
        <li>Configure database</li>
        <li>Set embedding provider</li>
        <li>Ingest code or docs</li>
    </ol>

    <p>Check the <a href="https://contextforge.dev/api">API Reference</a> for details.</p>

    <blockquote>
        Context is all you need for intelligent code assistance.
    </blockquote>

    <h2>Code Example</h2>
    <pre><code class="language-go">package main

import "fmt"

func main() {
    fmt.Println("Hello ContextForge!")
}</code></pre>

    <p>Inline code: <code>go run main.go</code></p>

    <hr/>

    <h2>Comparison Table</h2>
    <table>
        <tr><th>Feature</th><th>ContextForge</th><th>Others</th></tr>
        <tr><td>SSRF Guard</td><td>Built-in</td><td>Manual</td></tr>
        <tr><td>Multi-tenant</td><td>Yes</td><td>No</td></tr>
    </table>
</body>
</html>`

	page, err := ParseHTML(rawHTML)
	require.NoError(t, err)

	// Verify Title
	assert.Equal(t, "ContextForge Guide & Docs", page.Title)

	// Verify unwanted tags are stripped
	assert.NotContains(t, page.Markdown, "malicious script")
	assert.NotContains(t, page.Markdown, "background: #fff")
	assert.NotContains(t, page.Markdown, "Please enable JS")
	assert.NotContains(t, page.Markdown, "circle cx=")

	// Verify Headings
	assert.Contains(t, page.Markdown, "# ContextForge Documentation")
	assert.Contains(t, page.Markdown, "## Features")
	assert.Contains(t, page.Markdown, "### Quick Steps")

	// Verify Formatting
	assert.Contains(t, page.Markdown, "**ContextForge**")
	assert.Contains(t, page.Markdown, "*enterprise*")
	assert.Contains(t, page.Markdown, "[API Reference](https://contextforge.dev/api)")

	// Verify Lists
	assert.Contains(t, page.Markdown, "- Fast chunking and indexing")
	assert.Contains(t, page.Markdown, "- SSRF protection")
	assert.Contains(t, page.Markdown, "1. Configure database")
	assert.Contains(t, page.Markdown, "2. Set embedding provider")

	// Verify Blockquote
	assert.Contains(t, page.Markdown, "> Context is all you need")

	// Verify Pre Code
	assert.Contains(t, page.Markdown, "```go")
	assert.Contains(t, page.Markdown, "func main() {")

	// Verify Inline Code
	assert.Contains(t, page.Markdown, "`go run main.go`")

	// Verify Table
	assert.Contains(t, page.Markdown, "| Feature | ContextForge | Others |")
	assert.Contains(t, page.Markdown, "| SSRF Guard | Built-in | Manual |")
}

type mockRoundTripper func(*http.Request) (*http.Response, error)

func (m mockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return m(req)
}

func TestFetcher_HTMLContentExtraction(t *testing.T) {
	testURL := "http://127.0.0.1:8080/test-page"

	// Default fetcher without allowPrivateIP should reject 127.0.0.1 due to SSRF
	defaultFetcher := NewFetcher()
	_, err := defaultFetcher.Fetch(context.Background(), testURL)
	assert.Error(t, err, "default fetcher should block 127.0.0.1 test server due to SSRF")

	// Fetcher with AllowPrivateIP enabled and in-memory transport
	client := &http.Client{
		Transport: mockRoundTripper(func(req *http.Request) (*http.Response, error) {
			header := make(http.Header)
			header.Set("Content-Type", "text/html; charset=utf-8")
			body := `<!DOCTYPE html>
<html>
<head><title>Test Page Title</title></head>
<body>
    <h1>Heading 1</h1>
    <p>This is test content for ingestion.</p>
</body>
</html>`
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     header,
				Body:       io.NopCloser(strings.NewReader(body)),
				Request:    req,
			}, nil
		}),
	}

	testFetcher := NewFetcher(
		WithAllowPrivateIP(true),
		WithTimeout(5*time.Second),
		WithHTTPClient(client),
	)

	result, err := testFetcher.Fetch(context.Background(), testURL)
	require.NoError(t, err)
	assert.Equal(t, testURL, result.URL)
	assert.Equal(t, "Test Page Title", result.Title)
	assert.Contains(t, result.Content, "# Heading 1")
	assert.Contains(t, result.Content, "This is test content for ingestion.")
	assert.Equal(t, "text/html", result.ContentType)
	assert.Equal(t, http.StatusOK, result.StatusCode)
	assert.NotEmpty(t, result.ContentHash)
}

func TestFetcher_PlainTextExtraction(t *testing.T) {
	testURL := "http://127.0.0.1:8080/plain"

	client := &http.Client{
		Transport: mockRoundTripper(func(req *http.Request) (*http.Response, error) {
			header := make(http.Header)
			header.Set("Content-Type", "text/plain; charset=utf-8")
			body := `# Plain Document

Line one of the text.
Line two of the text.`
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     header,
				Body:       io.NopCloser(strings.NewReader(body)),
				Request:    req,
			}, nil
		}),
	}

	fetcher := NewFetcher(
		WithAllowPrivateIP(true),
		WithHTTPClient(client),
	)
	result, err := fetcher.Fetch(context.Background(), testURL)
	require.NoError(t, err)

	assert.Equal(t, "Plain Document", result.Title)
	assert.Contains(t, result.Content, "Line one of the text.")
	assert.Equal(t, "text/plain", result.ContentType)
	assert.NotEmpty(t, result.ContentHash)
}

func TestFetcher_UnsupportedContentType(t *testing.T) {
	testURL := "http://127.0.0.1:8080/image.png"

	client := &http.Client{
		Transport: mockRoundTripper(func(req *http.Request) (*http.Response, error) {
			header := make(http.Header)
			header.Set("Content-Type", "image/png")
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     header,
				Body:       io.NopCloser(strings.NewReader("\x89PNG\r\n\x1a\nfake image bytes")),
				Request:    req,
			}, nil
		}),
	}

	fetcher := NewFetcher(
		WithAllowPrivateIP(true),
		WithHTTPClient(client),
	)
	_, err := fetcher.Fetch(context.Background(), testURL)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrUnsupportedContentType)
}

func TestFetcher_SizeLimitEnforcement(t *testing.T) {
	testURL := "http://127.0.0.1:8080/large.txt"

	client := &http.Client{
		Transport: mockRoundTripper(func(req *http.Request) (*http.Response, error) {
			header := make(http.Header)
			header.Set("Content-Type", "text/plain")
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     header,
				Body:       io.NopCloser(strings.NewReader(strings.Repeat("A", 100))),
				Request:    req,
			}, nil
		}),
	}

	fetcher := NewFetcher(
		WithAllowPrivateIP(true),
		WithMaxBytes(50),
		WithHTTPClient(client),
	)

	_, err := fetcher.Fetch(context.Background(), testURL)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrContentTooLarge)
}

func TestFetcher_Non200Status(t *testing.T) {
	testURL := "http://127.0.0.1:8080/not-found"

	client := &http.Client{
		Transport: mockRoundTripper(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusNotFound,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader("not found")),
				Request:    req,
			}, nil
		}),
	}

	fetcher := NewFetcher(
		WithAllowPrivateIP(true),
		WithHTTPClient(client),
	)
	_, err := fetcher.Fetch(context.Background(), testURL)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrHTTPStatus)
}
