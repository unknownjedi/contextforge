package handler_test

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"

	"github.com/your-org/contextforge/internal/api/handler"
)

func computeHMAC(payload []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func setupWebhookRouter(secret string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := handler.NewWebhookHandler(secret, zap.NewNop())
	r.POST("/github/webhooks", h.HandleWebhook)
	return r
}

func TestWebhookHandler(t *testing.T) {
	secret := "super-secret-webhook-token"
	r := setupWebhookRouter(secret)

	payload := []byte(`{
		"ref": "refs/heads/main",
		"after": "abc1234567890",
		"repository": {
			"full_name": "octocat/hello-world",
			"name": "hello-world",
			"owner": {"login": "octocat"}
		}
	}`)

	t.Run("Valid HMAC signature on push event returns 202", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/github/webhooks", bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-GitHub-Event", "push")
		req.Header.Set("X-GitHub-Delivery", "delivery-123")
		req.Header.Set("X-Hub-Signature-256", computeHMAC(payload, secret))
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusAccepted, w.Code)
		assert.Contains(t, w.Body.String(), "octocat/hello-world")
		assert.Contains(t, w.Body.String(), "main")
	})

	t.Run("Invalid HMAC signature returns 401", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/github/webhooks", bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-GitHub-Event", "push")
		req.Header.Set("X-Hub-Signature-256", "sha256=invalid_signature_hex_value")
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("Missing HMAC signature returns 401", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/github/webhooks", bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-GitHub-Event", "push")
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("Ping event with valid HMAC returns 200", func(t *testing.T) {
		pingBody := []byte(`{"zen":"Design for failure."}`)
		req := httptest.NewRequest(http.MethodPost, "/github/webhooks", bytes.NewReader(pingBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-GitHub-Event", "ping")
		req.Header.Set("X-Hub-Signature-256", computeHMAC(pingBody, secret))
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), "pong")
	})
}
