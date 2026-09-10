package urlfetch

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"

	"github.com/your-org/contextforge/internal/connector"
)

var (
	ErrEmptyURL         = errors.New("urlfetch: URL cannot be empty")
	ErrInvalidURL       = errors.New("urlfetch: invalid URL format")
	ErrInvalidScheme    = errors.New("urlfetch: only http and https schemes are supported")
	ErrEmptyHost        = errors.New("urlfetch: URL host cannot be empty")
	ErrPrivateIPBlocked = errors.New("urlfetch: connection to private/internal network address is blocked by security policy")
)

// forbiddenHostnames contains hostnames that are strictly disallowed regardless of DNS response.
var forbiddenHostnames = map[string]struct{}{
	"localhost":                {},
	"localhost.localdomain":    {},
	"metadata.google.internal": {},
	"instance-data":            {},
}

// IsForbiddenHostname checks whether the provided hostname is an internal or reserved name.
func IsForbiddenHostname(host string) bool {
	h := strings.ToLower(strings.TrimSpace(host))
	if h == "" {
		return true
	}
	if _, found := forbiddenHostnames[h]; found {
		return true
	}
	if strings.HasSuffix(h, ".local") ||
		strings.HasSuffix(h, ".internal") ||
		strings.HasSuffix(h, ".localhost") {
		return true
	}
	return false
}

// ValidateIP checks if the given IP address is safe to connect to (rejects private, loopback, link-local, cloud metadata, etc.).
func ValidateIP(ip net.IP) error {
	if ip == nil {
		return errors.New("urlfetch: invalid nil IP address")
	}
	if connector.IsPrivateIP(ip) {
		return fmt.Errorf("%w: %s", ErrPrivateIPBlocked, ip.String())
	}
	return nil
}

// lookupIP allows overriding DNS resolution in unit tests.
var lookupIP = net.LookupIP

// SetLookupIPForTesting overrides lookupIP for testing and returns a cleanup func.
func SetLookupIPForTesting(fn func(string) ([]net.IP, error)) func() {
	orig := lookupIP
	lookupIP = fn
	return func() { lookupIP = orig }
}

// ValidateHost resolves the given host and verifies that all resolved IP addresses are public and safe.
func ValidateHost(host string) error {
	trimmed := strings.ToLower(strings.TrimSpace(host))
	if trimmed == "" {
		return ErrEmptyHost
	}

	// Remove port if present
	if h, _, err := net.SplitHostPort(trimmed); err == nil {
		trimmed = h
	}

	// Check forbidden hostnames
	if IsForbiddenHostname(trimmed) {
		return fmt.Errorf("%w: forbidden hostname %q", ErrPrivateIPBlocked, trimmed)
	}

	// Direct IP check (IPv4 or IPv6)
	if ip := net.ParseIP(trimmed); ip != nil {
		return ValidateIP(ip)
	}

	// DNS resolution
	ips, err := lookupIP(trimmed)
	if err != nil {
		return fmt.Errorf("resolving host %s: %w", trimmed, err)
	}
	if len(ips) == 0 {
		return fmt.Errorf("no IP addresses resolved for host %s", trimmed)
	}

	for _, ip := range ips {
		if err := ValidateIP(ip); err != nil {
			return fmt.Errorf("%w: host %s resolves to %s", ErrPrivateIPBlocked, trimmed, ip.String())
		}
	}

	return nil
}

// ValidateURL performs full SSRF validation on a raw URL:
// - Parses the URL
// - Ensures scheme is http or https
// - Checks for forbidden internal hostnames
// - Resolves DNS and checks all returned IPs against private ranges using connector.IsPrivateIP
func ValidateURL(rawURL string) (*url.URL, error) {
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" {
		return nil, ErrEmptyURL
	}

	parsed, err := url.Parse(trimmed)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidURL, err)
	}

	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return nil, fmt.Errorf("%w: %q", ErrInvalidScheme, parsed.Scheme)
	}

	host := parsed.Hostname()
	if host == "" {
		return nil, ErrEmptyHost
	}

	if err := ValidateHost(host); err != nil {
		return nil, err
	}

	return parsed, nil
}
