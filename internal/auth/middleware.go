package auth

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	// ContextKeyUserID is the key used to store the authenticated user's uuid.UUID in gin.Context.
	ContextKeyUserID = "user_id"

	// ContextKeyClaims is the key used to store the authenticated user's *Claims in gin.Context.
	ContextKeyClaims = "claims"

	// SessionCookieName is the cookie name contextforge checks for session authentication.
	SessionCookieName = "cf_session"

	// AuthorizationHeader is the HTTP header inspected for the Bearer token.
	AuthorizationHeader = "Authorization"

	// BearerSchemePrefix is the Bearer authentication prefix.
	BearerSchemePrefix = "bearer "
)

var (
	// ErrUserNotFound is returned by GetUserID when ContextKeyUserID is absent from gin.Context.
	ErrUserNotFound = errors.New("auth: user_id not found in request context")

	// ErrInvalidUserType is returned when ContextKeyUserID value is neither uuid.UUID nor a parseable UUID string.
	ErrInvalidUserType = errors.New("auth: context user_id is of invalid type")

	// ErrClaimsNotFound is returned by GetClaims when ContextKeyClaims is absent from gin.Context.
	ErrClaimsNotFound = errors.New("auth: claims not found in request context")
)

// ProblemDetails represents an RFC 7807 Problem Details payload.
type ProblemDetails struct {
	Type     string `json:"type"`
	Title    string `json:"title"`
	Status   int    `json:"status"`
	Detail   string `json:"detail,omitempty"`
	Instance string `json:"instance,omitempty"`
}

// AuthMiddleware creates a Gin middleware that extracts and validates the JWT authentication token.
// The token is extracted from either:
//  1. The "Authorization: Bearer <token>" HTTP header
//  2. The "cf_session" cookie
//
// Upon successful token validation, ContextKeyUserID ("user_id") and ContextKeyClaims ("claims")
// are set in the gin.Context and the request proceeds.
// If the token is missing, expired, or invalid, the middleware aborts the request with an
// RFC 7807 Problem Details JSON payload and HTTP 401 Unauthorized status.
func AuthMiddleware(jwtSecret []byte) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := extractToken(c)
		if token == "" {
			respondUnauthorized(c, "Authentication token is missing. Provide Authorization Bearer token or cf_session cookie.")
			return
		}

		claims, err := ValidateToken(token, jwtSecret)
		if err != nil {
			detail := "Authentication token is invalid"
			if errors.Is(err, ErrExpiredToken) {
				detail = "Authentication token has expired"
			} else if errors.Is(err, ErrInvalidSignature) {
				detail = "Authentication token signature is invalid"
			}
			respondUnauthorized(c, detail)
			return
		}

		c.Set(ContextKeyUserID, claims.UserID)
		c.Set(ContextKeyClaims, claims)
		c.Next()
	}
}

// extractToken attempts to retrieve a JWT token string from Authorization header,
// falling back to the cf_session cookie.
func extractToken(c *gin.Context) string {
	authHeader := c.GetHeader(AuthorizationHeader)
	if authHeader != "" {
		trimmed := strings.TrimSpace(authHeader)
		if len(trimmed) > len(BearerSchemePrefix) && strings.EqualFold(trimmed[:len(BearerSchemePrefix)], BearerSchemePrefix) {
			token := strings.TrimSpace(trimmed[len(BearerSchemePrefix):])
			if token != "" {
				return token
			}
		}
	}

	if cookie, err := c.Cookie(SessionCookieName); err == nil {
		cookie = strings.TrimSpace(cookie)
		if cookie != "" {
			return cookie
		}
	}

	return ""
}

// respondUnauthorized aborts the request with an RFC 7807 401 Unauthorized problem response.
func respondUnauthorized(c *gin.Context, detail string) {
	problem := ProblemDetails{
		Type:     "https://contextforge.dev/errors/unauthorized",
		Title:    "Unauthorized",
		Status:   http.StatusUnauthorized,
		Detail:   detail,
		Instance: c.Request.URL.Path,
	}
	c.Header("Content-Type", "application/problem+json")
	c.AbortWithStatusJSON(http.StatusUnauthorized, problem)
}

// GetUserID retrieves the authenticated user's uuid.UUID from the gin.Context.
// It supports both uuid.UUID and string representations.
func GetUserID(c *gin.Context) (uuid.UUID, error) {
	val, exists := c.Get(ContextKeyUserID)
	if !exists {
		return uuid.Nil, ErrUserNotFound
	}

	switch v := val.(type) {
	case uuid.UUID:
		return v, nil
	case string:
		id, err := uuid.Parse(v)
		if err != nil {
			return uuid.Nil, fmt.Errorf("%w: failed to parse UUID string: %v", ErrInvalidUserType, err)
		}
		return id, nil
	default:
		return uuid.Nil, ErrInvalidUserType
	}
}

// GetClaims retrieves the authenticated user's *Claims from the gin.Context.
func GetClaims(c *gin.Context) (*Claims, error) {
	val, exists := c.Get(ContextKeyClaims)
	if !exists {
		return nil, ErrClaimsNotFound
	}

	claims, ok := val.(*Claims)
	if !ok {
		return nil, errors.New("auth: context claims is not of type *Claims")
	}
	return claims, nil
}
