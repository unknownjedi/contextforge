package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func setupTestRouter(secret []byte) *gin.Engine {
	r := gin.New()
	r.Use(AuthMiddleware(secret))

	r.GET("/api/v1/user/profile", func(c *gin.Context) {
		userID, err := GetUserID(c)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		claims, err := GetClaims(c)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"user_id": userID.String(),
			"login":   claims.GitHubLogin,
		})
	})

	return r
}

func TestAuthMiddleware_BearerHeader_Success(t *testing.T) {
	router := setupTestRouter(testSecret)
	userID := uuid.New()
	login := "octocat"

	token, err := GenerateToken(userID, login, testSecret, 1*time.Hour)
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	req, _ := http.NewRequest(http.MethodGet, "/api/v1/user/profile", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200 OK, got %d: %s", w.Code, w.Body.String())
	}

	var res map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if res["user_id"] != userID.String() {
		t.Errorf("expected user_id %s, got %s", userID.String(), res["user_id"])
	}
	if res["login"] != login {
		t.Errorf("expected login %s, got %s", login, res["login"])
	}
}

func TestAuthMiddleware_BearerHeader_CaseInsensitive(t *testing.T) {
	router := setupTestRouter(testSecret)
	userID := uuid.New()
	token, _ := GenerateToken(userID, "octocat", testSecret, 1*time.Hour)

	req, _ := http.NewRequest(http.MethodGet, "/api/v1/user/profile", nil)
	req.Header.Set("Authorization", "bearer "+token)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200 OK for lowercase 'bearer', got %d: %s", w.Code, w.Body.String())
	}
}

func TestAuthMiddleware_Cookie_Success(t *testing.T) {
	router := setupTestRouter(testSecret)
	userID := uuid.New()
	login := "devuser"

	token, err := GenerateToken(userID, login, testSecret, 1*time.Hour)
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	req, _ := http.NewRequest(http.MethodGet, "/api/v1/user/profile", nil)
	req.AddCookie(&http.Cookie{
		Name:  SessionCookieName,
		Value: token,
	})

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200 OK via cookie, got %d: %s", w.Code, w.Body.String())
	}

	var res map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if res["user_id"] != userID.String() {
		t.Errorf("expected user_id %s, got %s", userID.String(), res["user_id"])
	}
}

func TestAuthMiddleware_HeaderPrecedenceOverCookie(t *testing.T) {
	router := setupTestRouter(testSecret)
	userHeader := uuid.New()
	userCookie := uuid.New()

	tokenHeader, _ := GenerateToken(userHeader, "header_user", testSecret, 1*time.Hour)
	tokenCookie, _ := GenerateToken(userCookie, "cookie_user", testSecret, 1*time.Hour)

	req, _ := http.NewRequest(http.MethodGet, "/api/v1/user/profile", nil)
	req.Header.Set("Authorization", "Bearer "+tokenHeader)
	req.AddCookie(&http.Cookie{
		Name:  SessionCookieName,
		Value: tokenCookie,
	})

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200 OK, got %d: %s", w.Code, w.Body.String())
	}

	var res map[string]string
	_ = json.Unmarshal(w.Body.Bytes(), &res)

	if res["user_id"] != userHeader.String() {
		t.Errorf("expected header user ID %s to take precedence, got %s", userHeader.String(), res["user_id"])
	}
}

func TestAuthMiddleware_MissingAuth_RFC7807(t *testing.T) {
	router := setupTestRouter(testSecret)

	req, _ := http.NewRequest(http.MethodGet, "/api/v1/user/profile", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401 Unauthorized, got %d", w.Code)
	}

	contentType := w.Header().Get("Content-Type")
	if contentType != "application/problem+json; charset=utf-8" && contentType != "application/problem+json" {
		t.Errorf("expected Content-Type application/problem+json, got %s", contentType)
	}

	var problem ProblemDetails
	if err := json.Unmarshal(w.Body.Bytes(), &problem); err != nil {
		t.Fatalf("failed to unmarshal ProblemDetails: %v", err)
	}

	if problem.Status != http.StatusUnauthorized {
		t.Errorf("expected problem status 401, got %d", problem.Status)
	}
	if problem.Type != "https://contextforge.dev/errors/unauthorized" {
		t.Errorf("expected problem type 'https://contextforge.dev/errors/unauthorized', got %s", problem.Type)
	}
	if problem.Title != "Unauthorized" {
		t.Errorf("expected problem title 'Unauthorized', got %s", problem.Title)
	}
	if problem.Instance != "/api/v1/user/profile" {
		t.Errorf("expected problem instance '/api/v1/user/profile', got %s", problem.Instance)
	}
	if problem.Detail == "" {
		t.Errorf("expected non-empty problem detail")
	}
}

func TestAuthMiddleware_EmptyBearerToken(t *testing.T) {
	router := setupTestRouter(testSecret)

	req, _ := http.NewRequest(http.MethodGet, "/api/v1/user/profile", nil)
	req.Header.Set("Authorization", "Bearer    ")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401 Unauthorized, got %d", w.Code)
	}
}

func TestAuthMiddleware_InvalidHeaderScheme(t *testing.T) {
	router := setupTestRouter(testSecret)

	req, _ := http.NewRequest(http.MethodGet, "/api/v1/user/profile", nil)
	req.Header.Set("Authorization", "Basic dXNlcjpwYXNzd29yZA==")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401 Unauthorized, got %d", w.Code)
	}
}

func TestAuthMiddleware_ExpiredToken(t *testing.T) {
	router := setupTestRouter(testSecret)
	userID := uuid.New()
	token, _ := GenerateToken(userID, "octocat", testSecret, -5*time.Minute)

	req, _ := http.NewRequest(http.MethodGet, "/api/v1/user/profile", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401 Unauthorized for expired token, got %d", w.Code)
	}

	var problem ProblemDetails
	if err := json.Unmarshal(w.Body.Bytes(), &problem); err != nil {
		t.Fatalf("failed to unmarshal ProblemDetails: %v", err)
	}

	if problem.Status != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", problem.Status)
	}
	if problem.Detail != "Authentication token has expired" {
		t.Errorf("expected 'Authentication token has expired', got %q", problem.Detail)
	}
}

func TestAuthMiddleware_TamperedSignature(t *testing.T) {
	router := setupTestRouter(testSecret)
	userID := uuid.New()
	token, _ := GenerateToken(userID, "octocat", testSecret, 1*time.Hour)

	// Tamper signature
	tamperedToken := token + "invalid"

	req, _ := http.NewRequest(http.MethodGet, "/api/v1/user/profile", nil)
	req.Header.Set("Authorization", "Bearer "+tamperedToken)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401 Unauthorized for tampered token, got %d", w.Code)
	}

	var problem ProblemDetails
	if err := json.Unmarshal(w.Body.Bytes(), &problem); err != nil {
		t.Fatalf("failed to unmarshal ProblemDetails: %v", err)
	}

	if problem.Status != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", problem.Status)
	}
}

func TestAuthMiddleware_MalformedToken(t *testing.T) {
	router := setupTestRouter(testSecret)

	req, _ := http.NewRequest(http.MethodGet, "/api/v1/user/profile", nil)
	req.Header.Set("Authorization", "Bearer not-a-valid-jwt")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401 Unauthorized for malformed token, got %d", w.Code)
	}
}

func TestGetUserID_ContextVariations(t *testing.T) {
	t.Run("valid uuid.UUID in context", func(t *testing.T) {
		expectedID := uuid.New()
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Set(ContextKeyUserID, expectedID)

		id, err := GetUserID(c)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if id != expectedID {
			t.Fatalf("expected %v, got %v", expectedID, id)
		}
	})

	t.Run("valid string UUID in context", func(t *testing.T) {
		expectedID := uuid.New()
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Set(ContextKeyUserID, expectedID.String())

		id, err := GetUserID(c)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if id != expectedID {
			t.Fatalf("expected %v, got %v", expectedID, id)
		}
	})

	t.Run("missing user_id in context", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		id, err := GetUserID(c)
		if err == nil {
			t.Fatalf("expected error for missing user_id, got nil")
		}
		if id != uuid.Nil {
			t.Fatalf("expected uuid.Nil, got %v", id)
		}
	})

	t.Run("invalid user_id type in context", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Set(ContextKeyUserID, 12345) // int is invalid

		_, err := GetUserID(c)
		if err == nil {
			t.Fatalf("expected error for int user_id, got nil")
		}
	})

	t.Run("unparseable string in context", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Set(ContextKeyUserID, "not-a-uuid")

		_, err := GetUserID(c)
		if err == nil {
			t.Fatalf("expected error for unparseable string, got nil")
		}
	})
}

func TestGetClaims_ContextVariations(t *testing.T) {
	t.Run("valid claims in context", func(t *testing.T) {
		expectedClaims := &Claims{
			UserID:      uuid.New(),
			GitHubLogin: "octocat",
		}
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Set(ContextKeyClaims, expectedClaims)

		claims, err := GetClaims(c)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if claims.UserID != expectedClaims.UserID {
			t.Fatalf("expected UserID %v, got %v", expectedClaims.UserID, claims.UserID)
		}
	})

	t.Run("missing claims in context", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		_, err := GetClaims(c)
		if err == nil {
			t.Fatalf("expected error for missing claims, got nil")
		}
	})

	t.Run("invalid claims type in context", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Set(ContextKeyClaims, "not-a-claims-pointer")

		_, err := GetClaims(c)
		if err == nil {
			t.Fatalf("expected error for invalid type, got nil")
		}
	})
}
