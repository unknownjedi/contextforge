package urlfetch

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"
)

const (
	// DefaultTimeout is the maximum duration for an HTTP fetch request.
	DefaultTimeout = 30 * time.Second

	// MaxFetchSize is the maximum allowed response body size (10 MB).
	MaxFetchSize = 10 * 1024 * 1024
)

var (
	ErrContentTooLarge        = errors.New("urlfetch: response body exceeds 10MB limit")
	ErrUnsupportedContentType = errors.New("urlfetch: unsupported content type")
	ErrHTTPStatus             = errors.New("urlfetch: non-2xx HTTP response status")
)

// FetchResult contains the parsed and indexed information from a fetched web page.
type FetchResult struct {
	URL         string `json:"url"`
	Title       string `json:"title"`
	Content     string `json:"content"`
	ContentType string `json:"content_type"`
	StatusCode  int    `json:"status_code"`
	ContentHash string `json:"content_hash"`
}

// URLFetcher defines the contract for fetching and parsing web URL sources.
type URLFetcher interface {
	Fetch(ctx context.Context, targetURL string) (*FetchResult, error)
}

// FetcherOption configures a Fetcher instance.
type FetcherOption func(*Fetcher)

// WithTimeout sets a custom HTTP client timeout.
func WithTimeout(timeout time.Duration) FetcherOption {
	return func(f *Fetcher) {
		f.timeout = timeout
	}
}

// WithMaxBytes sets a custom body size limit.
func WithMaxBytes(maxBytes int64) FetcherOption {
	return func(f *Fetcher) {
		f.maxBytes = maxBytes
	}
}

// WithAllowPrivateIP enables or disables private/internal IP access (used for test environments).
func WithAllowPrivateIP(allow bool) FetcherOption {
	return func(f *Fetcher) {
		f.allowPrivateIP = allow
	}
}

// WithHTTPClient provides a custom HTTP client.
func WithHTTPClient(client *http.Client) FetcherOption {
	return func(f *Fetcher) {
		f.client = client
	}
}

// Fetcher implements safe web content retrieval and HTML-to-Markdown conversion.
type Fetcher struct {
	client         *http.Client
	timeout        time.Duration
	maxBytes       int64
	allowPrivateIP bool
}

// NewFetcher creates a new Fetcher with production-grade SSRF protection and timeouts.
func NewFetcher(opts ...FetcherOption) *Fetcher {
	f := &Fetcher{
		timeout:        DefaultTimeout,
		maxBytes:       MaxFetchSize,
		allowPrivateIP: false,
	}

	for _, opt := range opts {
		opt(f)
	}

	if f.client == nil {
		dialer := &net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}

		transport := &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				host, _, err := net.SplitHostPort(addr)
				if err != nil {
					return nil, err
				}

				if !f.allowPrivateIP {
					if err := ValidateHost(host); err != nil {
						return nil, fmt.Errorf("ssrf blocked dial to %s: %w", host, err)
					}
				}

				return dialer.DialContext(ctx, network, addr)
			},
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          50,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		}

		f.client = &http.Client{
			Transport: transport,
			Timeout:   f.timeout,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 10 {
					return errors.New("stopped after 10 redirects")
				}
				if !f.allowPrivateIP {
					if _, err := ValidateURL(req.URL.String()); err != nil {
						return fmt.Errorf("ssrf blocked redirect to %s: %w", req.URL.String(), err)
					}
				}
				return nil
			},
		}
	}

	return f
}

// Fetch retrieves the URL, validates SSRF security policies, checks Content-Type,
// reads up to maxBytes, and parses the content into clean Markdown.
func (f *Fetcher) Fetch(ctx context.Context, targetURL string) (*FetchResult, error) {
	trimmedURL := strings.TrimSpace(targetURL)
	if trimmedURL == "" {
		return nil, ErrEmptyURL
	}

	if !f.allowPrivateIP {
		if _, err := ValidateURL(trimmedURL); err != nil {
			return nil, err
		}
	} else {
		// Basic URL format validation when private IP is allowed
		if _, err := url.Parse(trimmedURL); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrInvalidURL, err)
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, trimmedURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("User-Agent", "ContextForge-URLFetcher/1.0 (+https://contextforge.dev)")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,text/plain,text/markdown;q=0.9,*/*;q=0.1")

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching URL %s: %w", trimmedURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%w: status %d from %s", ErrHTTPStatus, resp.StatusCode, trimmedURL)
	}

	// Validate Content-Type
	rawCT := resp.Header.Get("Content-Type")
	mediaType, _, _ := mime.ParseMediaType(rawCT)
	mediaType = strings.ToLower(strings.TrimSpace(mediaType))
	if mediaType == "" {
		mediaType = "text/html" // Default fallback
	}

	if !isSupportedContentType(mediaType) {
		return nil, fmt.Errorf("%w: %q is not an indexable text or document format", ErrUnsupportedContentType, mediaType)
	}

	// Limit reader to maxBytes + 1 to detect overflows
	lr := io.LimitReader(resp.Body, f.maxBytes+1)
	bodyBytes, err := io.ReadAll(lr)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}

	if int64(len(bodyBytes)) > f.maxBytes {
		return nil, ErrContentTooLarge
	}

	var title string
	var content string

	if mediaType == "text/html" || mediaType == "application/xhtml+xml" {
		parsed, err := ParseHTML(string(bodyBytes))
		if err != nil {
			return nil, fmt.Errorf("parsing HTML content: %w", err)
		}
		title = parsed.Title
		content = parsed.Markdown
	} else {
		// Plain text or markdown
		content = strings.TrimSpace(string(bodyBytes))
		title = deriveTitleFromText(content, trimmedURL)
	}

	if title == "" {
		title = deriveTitleFromURL(trimmedURL)
	}

	// Calculate SHA256 of extracted content
	h := sha256.Sum256([]byte(content))
	contentHash := hex.EncodeToString(h[:])

	return &FetchResult{
		URL:         trimmedURL,
		Title:       title,
		Content:     content,
		ContentType: mediaType,
		StatusCode:  resp.StatusCode,
		ContentHash: contentHash,
	}, nil
}

func isSupportedContentType(mediaType string) bool {
	switch mediaType {
	case "text/html",
		"application/xhtml+xml",
		"text/plain",
		"text/markdown",
		"text/x-markdown",
		"text/csv",
		"text/tab-separated-values",
		"application/json",
		"text/xml",
		"application/xml":
		return true
	}
	return strings.HasPrefix(mediaType, "text/")
}

func deriveTitleFromURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	base := path.Base(strings.TrimRight(u.Path, "/"))
	if base == "" || base == "/" || base == "." {
		return u.Hostname()
	}
	// Strip file extension if present
	ext := path.Ext(base)
	return strings.TrimSuffix(base, ext)
}

func deriveTitleFromText(text, rawURL string) string {
	lines := strings.Split(text, "\n")
	for _, l := range lines {
		trimmed := strings.TrimSpace(l)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "# ") {
			return strings.TrimSpace(strings.TrimPrefix(trimmed, "# "))
		}
		if len(trimmed) > 0 {
			if len(trimmed) > 80 {
				return trimmed[:80] + "..."
			}
			return trimmed
		}
	}
	return deriveTitleFromURL(rawURL)
}
