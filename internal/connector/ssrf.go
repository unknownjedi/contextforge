package connector

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

var (
	ErrUnsupportedScheme = errors.New("connector: unsupported database connection scheme")
	ErrInvalidHost       = errors.New("connector: invalid database host")
	ErrInvalidPort       = errors.New("connector: invalid database port")
	ErrPrivateIPBlocked  = errors.New("connector: connection to private/internal network address is blocked by security policy")
	ErrUnsafeCharacters  = errors.New("connector: connection URL contains illegal or dangerous characters")
)

var (
	unsafeCharRegex    = regexp.MustCompile(`[;\x00-\x1F\x7F]`)
	urlCredentialRegex = regexp.MustCompile(`://([^:/\s@]+):([^@/\s]+)@`)
	passwordKVRegex    = regexp.MustCompile(`(?i)(password|passwd|pwd|secret|token|api_?key)\s*([=:])\s*([^;\s&]+)`)
)

var privateIPBlocks []*net.IPNet

func init() {
	cidrs := []string{
		"0.0.0.0/8",          // RFC 1122 Current network (this host, 0.0.0.0)
		"10.0.0.0/8",         // RFC 1918 Private
		"100.64.0.0/10",      // RFC 6598 Shared Address Space / CGNAT / Cloud internal
		"127.0.0.0/8",        // RFC 1122 Loopback
		"169.254.0.0/16",     // RFC 3927 Link-Local / AWS/GCP Metadata Service (169.254.169.254)
		"172.16.0.0/12",      // RFC 1918 Private
		"192.0.0.0/24",       // RFC 6890 IETF Protocol Assignments
		"192.0.2.0/24",       // RFC 5737 TEST-NET-1
		"192.88.99.0/24",     // RFC 7526 6to4 Relay Anycast
		"192.168.0.0/16",     // RFC 1918 Private
		"198.18.0.0/15",      // RFC 2544 Network Benchmark Testing
		"198.51.100.0/24",    // RFC 5737 TEST-NET-2
		"203.0.113.0/24",     // RFC 5737 TEST-NET-3
		"224.0.0.0/4",        // RFC 5771 Multicast
		"240.0.0.0/4",        // RFC 1112 Reserved for future use
		"255.255.255.255/32", // RFC 919 Limited Broadcast
		"::/128",             // RFC 4291 Unspecified
		"::1/128",            // RFC 4291 Loopback
		"64:ff9b::/96",       // RFC 6052 IPv4/IPv6 translation
		"100::/64",           // RFC 6666 Discard-only
		"2001:db8::/32",      // RFC 3849 Documentation
		"fc00::/7",           // RFC 4193 Unique Local Address (ULA)
		"fe80::/10",          // RFC 4291 Link-Local Unicast
		"ff00::/8",           // RFC 4291 Multicast
	}
	for _, cidr := range cidrs {
		_, block, err := net.ParseCIDR(cidr)
		if err == nil {
			privateIPBlocks = append(privateIPBlocks, block)
		}
	}
}

// IsPrivateIP checks if an IP is in loopback, link-local, private RFC1918, CGNAT, or cloud metadata ranges.
func IsPrivateIP(ip net.IP) bool {
	if ip == nil {
		return false
	}
	// Unwrap IPv4-mapped IPv6 (e.g. ::ffff:127.0.0.1 or ::ffff:169.254.169.254)
	if ip4 := ip.To4(); ip4 != nil {
		ip = ip4
	}
	if ip.IsUnspecified() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsPrivate() {
		return true
	}
	for _, block := range privateIPBlocks {
		if block.Contains(ip) {
			return true
		}
	}
	return false
}

// ValidateNetworkTarget verifies that the target host and port are safe to connect to.
func ValidateNetworkTarget(host string, port int, allowPrivate bool) error {
	trimmedHost := strings.TrimSpace(host)
	if trimmedHost == "" {
		return ErrInvalidHost
	}

	if unsafeCharRegex.MatchString(trimmedHost) {
		return fmt.Errorf("%w: host contains forbidden characters", ErrUnsafeCharacters)
	}

	if port <= 0 || port > 65535 {
		return ErrInvalidPort
	}

	if !allowPrivate {
		// Check if host is direct IP
		ip := net.ParseIP(trimmedHost)
		if ip != nil {
			if IsPrivateIP(ip) {
				return fmt.Errorf("%w: %s", ErrPrivateIPBlocked, trimmedHost)
			}
			return nil
		}

		// Check common loopback and internal hostnames
		lowerHost := strings.ToLower(trimmedHost)
		if lowerHost == "localhost" || lowerHost == "localhost.localdomain" ||
			strings.HasSuffix(lowerHost, ".local") || strings.HasSuffix(lowerHost, ".internal") ||
			strings.HasSuffix(lowerHost, ".localhost") {
			return fmt.Errorf("%w: %s", ErrPrivateIPBlocked, trimmedHost)
		}

		// Resolve DNS hostname to verify all target IPs
		ips, err := net.LookupIP(trimmedHost)
		if err != nil {
			return fmt.Errorf("resolving host %s: %w", trimmedHost, err)
		}
		if len(ips) == 0 {
			return fmt.Errorf("no IP addresses resolved for host %s", trimmedHost)
		}
		for _, resolvedIP := range ips {
			if IsPrivateIP(resolvedIP) {
				return fmt.Errorf("%w: host %s resolves to internal IP %s", ErrPrivateIPBlocked, trimmedHost, resolvedIP)
			}
		}
	}

	return nil
}

// SanitizeConnectionError strips passwords and sensitive tokens from error messages.
func SanitizeConnectionError(err error, rawURL string) error {
	if err == nil {
		return nil
	}

	msg := err.Error()

	// Parse rawURL if provided to identify user/password strings to redact
	if rawURL != "" {
		if u, parseErr := url.Parse(rawURL); parseErr == nil {
			if pass, hasPass := u.User.Password(); hasPass && pass != "" {
				msg = strings.ReplaceAll(msg, pass, "******")
				msg = strings.ReplaceAll(msg, url.QueryEscape(pass), "******")
			}
			if u.User != nil {
				userStr := u.User.Username()
				if userStr != "" && strings.Contains(msg, userStr+":") {
					msg = strings.ReplaceAll(msg, userStr+":", "user:******@")
				}
			}
		}
	}

	// Mask standard URL credentials scheme://user:pass@host
	msg = urlCredentialRegex.ReplaceAllString(msg, "://$1:******@")

	// Mask key-value password patterns (password=..., pwd:..., etc.)
	msg = passwordKVRegex.ReplaceAllString(msg, "$1$2******")

	return errors.New(msg)
}

// ParsePort extracts or defaults the port from a port string.
func ParsePort(portStr string, defaultPort int) int {
	if portStr == "" {
		return defaultPort
	}
	p, err := strconv.Atoi(portStr)
	if err != nil || p <= 0 || p > 65535 {
		return defaultPort
	}
	return p
}
