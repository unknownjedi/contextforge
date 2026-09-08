package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/your-org/contextforge/internal/api/handler"
	"github.com/your-org/contextforge/internal/auth"
	"github.com/your-org/contextforge/internal/ent"
)

type mockAuthService struct {
	validPAT string
	user     *ent.User
	token    string
}

func (m *mockAuthService) ValidateAndAuthenticatePAT(ctx context.Context, pat string) (*ent.User, string, error) {
	if pat == m.validPAT {
		return m.user, m.token, nil
	}
	return nil, "", errors.New("bad credentials")
}

func (m *mockAuthService) ExchangeOAuthCode(ctx context.Context, code string) (*ent.User, string, error) {
	return nil, "", errors.New("not implemented")
}

func (m *mockAuthService) GetUser(ctx context.Context, userID uuid.UUID) (*ent.User, error) {
	if m.user != nil && m.user.ID == userID {
		return m.user, nil
	}
	return nil, errors.New("user not found")
}

func setupAuthRouter(svc *mockAuthService, simulateUserID uuid.UUID, devPAT ...string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	h := handler.NewAuthHandler(svc, devPAT...)

	r.POST("/auth/pat", h.AuthenticatePAT)
	r.GET("/auth/auto", h.AutoAuthenticateDev)
	r.POST("/auth/logout", h.Logout)

	authed := r.Group("/")
	authed.Use(func(c *gin.Context) {
		if simulateUserID != uuid.Nil {
			c.Set(auth.ContextKeyUserID, simulateUserID)
		}
		c.Next()
	})
	authed.GET("/auth/me", h.GetMe)

	return r
}

func TestAuthHandler_AuthenticatePAT(t *testing.T) {
	userID := uuid.New()
	mockUser := &ent.User{
		ID:          userID,
		GithubLogin: "octocat",
		Email:       "octocat@github.com",
		Name:        "The Octocat",
		CreatedAt:   time.Now(),
	}

	svc := &mockAuthService{
		validPAT: "ghp_valid_test_token_123",
		user:     mockUser,
		token:    "jwt_session_token_xyz",
	}

	r := setupAuthRouter(svc, uuid.Nil)

	t.Run("Valid PAT returns 200 and token", func(t *testing.T) {
		body := bytes.NewBufferString(`{"pat":"ghp_valid_test_token_123"}`)
		req := httptest.NewRequest(http.MethodPost, "/auth/pat", body)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)

		assert.Equal(t, "jwt_session_token_xyz", resp["token"])
		userMap := resp["user"].(map[string]interface{})
		assert.Equal(t, "octocat", userMap["github_login"])

		// Check session cookie
		cookies := w.Result().Cookies()
		require.NotEmpty(t, cookies)
		assert.Equal(t, "cf_session", cookies[0].Name)
		assert.Equal(t, "jwt_session_token_xyz", cookies[0].Value)
	})

	t.Run("Invalid PAT returns 401", func(t *testing.T) {
		body := bytes.NewBufferString(`{"pat":"ghp_invalid_token"}`)
		req := httptest.NewRequest(http.MethodPost, "/auth/pat", body)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
		assert.Contains(t, w.Body.String(), "Authentication Failed")
	})

	t.Run("Empty body returns 400", func(t *testing.T) {
		body := bytes.NewBufferString(`{}`)
		req := httptest.NewRequest(http.MethodPost, "/auth/pat", body)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestAuthHandler_GetMe(t *testing.T) {
	userID := uuid.New()
	mockUser := &ent.User{
		ID:          userID,
		GithubLogin: "octocat",
		Email:       "octocat@github.com",
		Name:        "The Octocat",
		CreatedAt:   time.Now(),
	}

	svc := &mockAuthService{user: mockUser}

	t.Run("Authenticated user returns profile", func(t *testing.T) {
		r := setupAuthRouter(svc, userID)
		req := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), "octocat")
	})

	t.Run("Unauthenticated user returns 401", func(t *testing.T) {
		r := setupAuthRouter(svc, uuid.Nil)
		req := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})
}

func TestAuthHandler_Logout(t *testing.T) {
	r := setupAuthRouter(&mockAuthService{}, uuid.Nil)
	req := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
	cookies := w.Result().Cookies()
	require.NotEmpty(t, cookies)
	assert.Equal(t, "cf_session", cookies[0].Name)
	assert.Equal(t, -1, cookies[0].MaxAge)
}

func TestAuthHandler_AutoAuthenticateDev(t *testing.T) {
	mockUser := &ent.User{
		ID:          uuid.New(),
		GithubLogin: "octocat",
		Email:       "octocat@github.com",
		Name:        "The Octocat",
		CreatedAt:   time.Now(),
	}

	t.Run("Not configured returns 200 with configured false", func(t *testing.T) {
		svc := &mockAuthService{validPAT: "ghp_valid", user: mockUser, token: "jwt_tok"}
		r := setupAuthRouter(svc, uuid.Nil)
		req := httptest.NewRequest(http.MethodGet, "/auth/auto", nil)
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), `"configured":false`)
	})

	t.Run("Configured valid PAT returns 200 and token", func(t *testing.T) {
		svc := &mockAuthService{validPAT: "ghp_valid", user: mockUser, token: "jwt_tok"}
		r := setupAuthRouter(svc, uuid.Nil, "ghp_valid")
		req := httptest.NewRequest(http.MethodGet, "/auth/auto", nil)
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), "jwt_tok")
		assert.Contains(t, w.Body.String(), `"configured":true`)
	})

	t.Run("Configured invalid PAT returns 401", func(t *testing.T) {
		svc := &mockAuthService{validPAT: "ghp_valid", user: mockUser, token: "jwt_tok"}
		r := setupAuthRouter(svc, uuid.Nil, "ghp_wrong")
		req := httptest.NewRequest(http.MethodGet, "/auth/auto", nil)
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
		assert.Contains(t, w.Body.String(), "bad credentials")
	})
}
