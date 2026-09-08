package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/your-org/contextforge/internal/auth"
	"github.com/your-org/contextforge/internal/service"
)

type AuthHandler struct {
	authService service.AuthService
	devPAT      string
}

func NewAuthHandler(authService service.AuthService, devPAT ...string) *AuthHandler {
	pat := ""
	if len(devPAT) > 0 {
		pat = devPAT[0]
	}
	return &AuthHandler{authService: authService, devPAT: pat}
}

type PATLoginRequest struct {
	PAT string `json:"pat" binding:"required"`
}

// AuthenticatePAT handles POST /api/v1/auth/pat.
func (h *AuthHandler) AuthenticatePAT(c *gin.Context) {
	var req PATLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"type":     "https://contextforge.dev/errors/validation",
			"title":    "Validation Error",
			"status":   http.StatusBadRequest,
			"detail":   "Field 'pat' is required and must not be empty",
			"instance": c.Request.URL.Path,
		})
		return
	}

	u, token, err := h.authService.ValidateAndAuthenticatePAT(c.Request.Context(), req.PAT)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"type":     "https://contextforge.dev/errors/unauthorized",
			"title":    "Authentication Failed",
			"status":   http.StatusUnauthorized,
			"detail":   err.Error(),
			"instance": c.Request.URL.Path,
		})
		return
	}

	// Set session cookie for web clients
	c.SetCookie("cf_session", token, 3600*72, "/", "", false, true)

	c.JSON(http.StatusOK, gin.H{
		"token": token,
		"user": gin.H{
			"id":           u.ID,
			"github_login": u.GithubLogin,
			"email":        u.Email,
			"name":         u.Name,
			"avatar_url":   u.AvatarURL,
			"created_at":   u.CreatedAt,
		},
	})
}

// AutoAuthenticateDev handles GET /api/v1/auth/auto.
// If GITHUB_PAT is configured on the server environment, it automatically provisions
// a valid developer session without requiring manual PAT copy-paste.
func (h *AuthHandler) AutoAuthenticateDev(c *gin.Context) {
	if h.devPAT == "" {
		c.JSON(http.StatusOK, gin.H{
			"configured": false,
			"message":    "No GITHUB_PAT configured in server environment",
		})
		return
	}

	u, token, err := h.authService.ValidateAndAuthenticatePAT(c.Request.Context(), h.devPAT)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"type":       "https://contextforge.dev/errors/unauthorized",
			"title":      "Dev PAT Authentication Failed",
			"status":     http.StatusUnauthorized,
			"detail":     "Failed to validate environment GITHUB_PAT with GitHub: " + err.Error(),
			"instance":   c.Request.URL.Path,
			"configured": true,
		})
		return
	}

	// Set session cookie for web clients
	c.SetCookie("cf_session", token, 3600*72, "/", "", false, true)

	c.JSON(http.StatusOK, gin.H{
		"token":      token,
		"configured": true,
		"user": gin.H{
			"id":           u.ID,
			"github_login": u.GithubLogin,
			"email":        u.Email,
			"name":         u.Name,
			"avatar_url":   u.AvatarURL,
			"created_at":   u.CreatedAt,
		},
	})
}

// GetMe handles GET /api/v1/auth/me.
func (h *AuthHandler) GetMe(c *gin.Context) {
	userID, err := auth.GetUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"type":     "https://contextforge.dev/errors/unauthorized",
			"title":    "Unauthorized",
			"status":   http.StatusUnauthorized,
			"detail":   "Authentication required",
			"instance": c.Request.URL.Path,
		})
		return
	}

	u, err := h.authService.GetUser(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"type":     "https://contextforge.dev/errors/not-found",
			"title":    "User Not Found",
			"status":   http.StatusNotFound,
			"detail":   "User profile not found",
			"instance": c.Request.URL.Path,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"id":           u.ID,
		"github_login": u.GithubLogin,
		"email":        u.Email,
		"name":         u.Name,
		"avatar_url":   u.AvatarURL,
		"created_at":   u.CreatedAt,
	})
}

// Logout handles POST /api/v1/auth/logout.
func (h *AuthHandler) Logout(c *gin.Context) {
	c.SetCookie("cf_session", "", -1, "/", "", false, true)
	c.Status(http.StatusNoContent)
}

// LoginGitHub handles GET /api/v1/auth/github/login.
func (h *AuthHandler) LoginGitHub(c *gin.Context) {
	// For production GitHub App redirect:
	c.JSON(http.StatusOK, gin.H{
		"message": "GitHub App OAuth flow endpoint",
	})
}

// CallbackGitHub handles GET /api/v1/auth/github/callback.
func (h *AuthHandler) CallbackGitHub(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"message": "GitHub OAuth callback endpoint",
	})
}
