package handler

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type WebhookHandler struct {
	webhookSecret string
	logger        *zap.Logger
}

func NewWebhookHandler(secret string, logger *zap.Logger) *WebhookHandler {
	return &WebhookHandler{
		webhookSecret: secret,
		logger:        logger,
	}
}

type GitHubPushPayload struct {
	Ref        string `json:"ref"`
	After      string `json:"after"` // Commit SHA
	Before     string `json:"before"`
	Repository struct {
		ID       int64  `json:"id"`
		Name     string `json:"name"`
		FullName string `json:"full_name"`
		Owner    struct {
			Login string `json:"login"`
		} `json:"owner"`
		DefaultBranch string `json:"default_branch"`
	} `json:"repository"`
}

// HandleWebhook handles POST /github/webhooks with HMAC-SHA256 signature verification.
func (h *WebhookHandler) HandleWebhook(c *gin.Context) {
	signature := c.GetHeader("X-Hub-Signature-256")
	event := c.GetHeader("X-GitHub-Event")
	deliveryID := c.GetHeader("X-GitHub-Delivery")

	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Failed to read request body",
		})
		return
	}

	// 1. Verify HMAC-SHA256 signature if a secret is configured
	if h.webhookSecret != "" {
		if !verifyHMACSignature(body, signature, h.webhookSecret) {
			c.JSON(http.StatusUnauthorized, gin.H{
				"type":   "https://contextforge.dev/errors/unauthorized",
				"title":  "Invalid Webhook Signature",
				"status": http.StatusUnauthorized,
				"detail": "X-Hub-Signature-256 header did not match expected HMAC signature",
			})
			return
		}
	}

	// 2. Dispatch based on event type
	switch event {
	case "push":
		var push GitHubPushPayload
		if err := json.Unmarshal(body, &push); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid push payload"})
			return
		}

		branch := strings.TrimPrefix(push.Ref, "refs/heads/")
		h.logger.Info("received verified github push event",
			zap.String("repo", push.Repository.FullName),
			zap.String("branch", branch),
			zap.String("commit", push.After),
			zap.String("delivery_id", deliveryID),
		)

		// Acknowledge receipt
		c.JSON(http.StatusAccepted, gin.H{
			"status":      "accepted",
			"event":       event,
			"delivery_id": deliveryID,
			"repository":  push.Repository.FullName,
			"branch":      branch,
			"commit":      push.After,
		})

	case "ping":
		c.JSON(http.StatusOK, gin.H{"status": "pong", "delivery_id": deliveryID})

	default:
		c.JSON(http.StatusAccepted, gin.H{
			"status":      "ignored",
			"event":       event,
			"delivery_id": deliveryID,
		})
	}
}

// verifyHMACSignature verifies GitHub's sha256=... signature using constant-time comparison.
func verifyHMACSignature(payload []byte, signatureHeader, secret string) bool {
	if !strings.HasPrefix(signatureHeader, "sha256=") {
		return false
	}
	actualSigHex := strings.TrimPrefix(signatureHeader, "sha256=")
	actualSig, err := hex.DecodeString(actualSigHex)
	if err != nil {
		return false
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	expectedSig := mac.Sum(nil)

	return hmac.Equal(actualSig, expectedSig)
}
