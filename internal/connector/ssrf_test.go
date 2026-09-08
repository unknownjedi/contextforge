package connector

import (
	"errors"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsPrivateIP(t *testing.T) {
	tests := []struct {
		ip       string
		expected bool
	}{
		{"127.0.0.1", true},
		{"0.0.0.0", true},
		{"::", true},
		{"10.0.0.1", true},
		{"172.16.0.1", true},
		{"172.31.255.255", true},
		{"192.168.1.100", true},
		{"169.254.1.1", true},
		{"169.254.169.254", true}, // AWS / GCP metadata
		{"100.64.0.1", true},      // CGNAT / Cloud internal
		{"::1", true},
		{"::ffff:127.0.0.1", true},     // IPv4-mapped IPv6 loopback
		{"::ffff:169.254.169.254", true}, // IPv4-mapped metadata
		{"::ffff:10.0.0.1", true},       // IPv4-mapped private
		{"fc00::1", true},
		{"fe80::1", true},
		{"8.8.8.8", false},
		{"1.1.1.1", false},
		{"142.250.190.46", false},
	}

	for _, tt := range tests {
		ip := net.ParseIP(tt.ip)
		require.NotNil(t, ip, "failed to parse test ip %s", tt.ip)
		assert.Equal(t, tt.expected, IsPrivateIP(ip), "ip: %s", tt.ip)
	}
}

func TestValidateNetworkTarget(t *testing.T) {
	t.Run("invalid ports", func(t *testing.T) {
		assert.ErrorIs(t, ValidateNetworkTarget("example.com", -1, true), ErrInvalidPort)
		assert.ErrorIs(t, ValidateNetworkTarget("example.com", 0, true), ErrInvalidPort)
		assert.ErrorIs(t, ValidateNetworkTarget("example.com", 70000, true), ErrInvalidPort)
	})

	t.Run("empty host", func(t *testing.T) {
		assert.ErrorIs(t, ValidateNetworkTarget("", 5432, true), ErrInvalidHost)
	})

	t.Run("dangerous characters in host", func(t *testing.T) {
		assert.Error(t, ValidateNetworkTarget("example.com; rm -rf /", 5432, true))
	})

	t.Run("private IP blocked in production", func(t *testing.T) {
		assert.ErrorIs(t, ValidateNetworkTarget("127.0.0.1", 5432, false), ErrPrivateIPBlocked)
		assert.ErrorIs(t, ValidateNetworkTarget("0.0.0.0", 5432, false), ErrPrivateIPBlocked)
		assert.ErrorIs(t, ValidateNetworkTarget("169.254.169.254", 5432, false), ErrPrivateIPBlocked)
		assert.ErrorIs(t, ValidateNetworkTarget("::ffff:127.0.0.1", 5432, false), ErrPrivateIPBlocked)
		assert.ErrorIs(t, ValidateNetworkTarget("localhost", 5432, false), ErrPrivateIPBlocked)
		assert.ErrorIs(t, ValidateNetworkTarget("service.internal", 5432, false), ErrPrivateIPBlocked)
	})

	t.Run("private IP allowed when flag true", func(t *testing.T) {
		assert.NoError(t, ValidateNetworkTarget("127.0.0.1", 5432, true))
		assert.NoError(t, ValidateNetworkTarget("localhost", 5432, true))
		assert.NoError(t, ValidateNetworkTarget("0.0.0.0", 5432, true))
	})
}

func TestSanitizeConnectionError(t *testing.T) {
	t.Run("with rawURL", func(t *testing.T) {
		rawURL := "postgres://myuser:supersecretpass@db.example.com:5432/app"
		err := errors.New("failed to connect with password supersecretpass to db.example.com")

		sanitized := SanitizeConnectionError(err, rawURL)
		assert.NotContains(t, sanitized.Error(), "supersecretpass")
		assert.Contains(t, sanitized.Error(), "******")
	})

	t.Run("without rawURL scrubs URL embedded password", func(t *testing.T) {
		err := errors.New("dial tcp postgres://admin:secret12345@198.51.100.1:5432/mydb: connection refused")
		sanitized := SanitizeConnectionError(err, "")
		assert.NotContains(t, sanitized.Error(), "secret12345")
		assert.Contains(t, sanitized.Error(), "://admin:******@")
	})

	t.Run("without rawURL scrubs key-value passwords", func(t *testing.T) {
		err := errors.New("connection failed: Password=TopSecret;User ID=admin")
		sanitized := SanitizeConnectionError(err, "")
		assert.NotContains(t, sanitized.Error(), "TopSecret")
		assert.Contains(t, sanitized.Error(), "Password=******")
	})
}
