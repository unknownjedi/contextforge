package middleware_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/your-org/contextforge/internal/api/middleware"
	"github.com/your-org/contextforge/internal/auth"
)

type problemResponse struct {
	Type     string `json:"type"`
	Title    string `json:"title"`
	Status   int    `json:"status"`
	Detail   string `json:"detail"`
	Instance string `json:"instance"`
}

func setupRateLimitRouter(limiter *middleware.RateLimiter) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(limiter.Middleware())
	r.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})
	return r
}

func TestRateLimiter_BurstAndLimit(t *testing.T) {
	// Rate: 1 token/sec, Burst: 3 tokens
	limiter := middleware.NewRateLimiter(1.0, 3, middleware.WithCleanupInterval(0))
	defer limiter.Close()

	r := setupRateLimitRouter(limiter)

	// Send 3 requests (within burst capacity) -> all should succeed with 200
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.RemoteAddr = "192.0.2.1:1234"
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code, "request %d should succeed", i+1)
	}

	// 4th request exceeds burst capacity -> should receive 429
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "192.0.2.1:1234"
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusTooManyRequests, w.Code)

	// Check Retry-After header
	retryAfter := w.Header().Get("Retry-After")
	require.NotEmpty(t, retryAfter, "Retry-After header must be present")
	retryAfterVal, err := strconv.Atoi(retryAfter)
	require.NoError(t, err, "Retry-After must be a valid integer")
	assert.GreaterOrEqual(t, retryAfterVal, 1, "Retry-After must be at least 1 second")

	// Check RFC 7807 Problem Details response
	assert.Equal(t, "application/problem+json", w.Header().Get("Content-Type"))
	var problem problemResponse
	err = json.Unmarshal(w.Body.Bytes(), &problem)
	require.NoError(t, err, "Response body should parse as RFC 7807 Problem Details")
	assert.Equal(t, "https://contextforge.dev/errors/rate-limit-exceeded", problem.Type)
	assert.Equal(t, "Too Many Requests", problem.Title)
	assert.Equal(t, http.StatusTooManyRequests, problem.Status)
	assert.Contains(t, problem.Detail, "Rate limit exceeded")
	assert.Equal(t, "/test", problem.Instance)
}

func TestRateLimiter_SteadyRefill(t *testing.T) {
	// Rate: 10 tokens/sec (1 token per 100ms), Burst: 2 tokens
	limiter := middleware.NewRateLimiter(10.0, 2, middleware.WithCleanupInterval(0))
	defer limiter.Close()

	r := setupRateLimitRouter(limiter)

	// Exhaust 2 tokens
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.RemoteAddr = "192.0.2.10:1234"
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	}

	// Immediately send 3rd -> 429
	req3 := httptest.NewRequest(http.MethodGet, "/test", nil)
	req3.RemoteAddr = "192.0.2.10:1234"
	w3 := httptest.NewRecorder()
	r.ServeHTTP(w3, req3)
	assert.Equal(t, http.StatusTooManyRequests, w3.Code)

	// Wait 130ms for >1 token to refill (10 tokens/sec = 100ms per token)
	time.Sleep(130 * time.Millisecond)

	// Request should now succeed
	req4 := httptest.NewRequest(http.MethodGet, "/test", nil)
	req4.RemoteAddr = "192.0.2.10:1234"
	w4 := httptest.NewRecorder()
	r.ServeHTTP(w4, req4)
	assert.Equal(t, http.StatusOK, w4.Code, "request after steady refill should succeed")
}

func TestRateLimiter_ClientIsolation_IP(t *testing.T) {
	limiter := middleware.NewRateLimiter(1.0, 1, middleware.WithCleanupInterval(0))
	defer limiter.Close()

	r := setupRateLimitRouter(limiter)

	// Exhaust Client A (IP 192.0.2.20)
	reqA1 := httptest.NewRequest(http.MethodGet, "/test", nil)
	reqA1.RemoteAddr = "192.0.2.20:1234"
	wA1 := httptest.NewRecorder()
	r.ServeHTTP(wA1, reqA1)
	assert.Equal(t, http.StatusOK, wA1.Code)

	reqA2 := httptest.NewRequest(http.MethodGet, "/test", nil)
	reqA2.RemoteAddr = "192.0.2.20:1234"
	wA2 := httptest.NewRecorder()
	r.ServeHTTP(wA2, reqA2)
	assert.Equal(t, http.StatusTooManyRequests, wA2.Code)

	// Client B (IP 192.0.2.21) should still have available quota
	reqB := httptest.NewRequest(http.MethodGet, "/test", nil)
	reqB.RemoteAddr = "192.0.2.21:1234"
	wB := httptest.NewRecorder()
	r.ServeHTTP(wB, reqB)
	assert.Equal(t, http.StatusOK, wB.Code, "different client IP must have its own bucket")
}

func TestRateLimiter_ClientIsolation_UserID(t *testing.T) {
	limiter := middleware.NewRateLimiter(1.0, 1, middleware.WithCleanupInterval(0))
	defer limiter.Close()

	userA := uuid.New()
	userB := uuid.New()

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(limiter.Middleware())
	r.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	// User A first request -> 200
	reqA1 := httptest.NewRequest(http.MethodGet, "/test", nil)
	reqA1.RemoteAddr = "192.0.2.30:1234" // same IP
	wA1 := httptest.NewRecorder()
	// Set user A context
	c1, _ := gin.CreateTestContext(wA1)
	c1.Request = reqA1
	c1.Set(auth.ContextKeyUserID, userA)
	limiter.Middleware()(c1)
	assert.False(t, c1.IsAborted())

	// User A second request -> 429
	reqA2 := httptest.NewRequest(http.MethodGet, "/test", nil)
	wA2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(wA2)
	c2.Request = reqA2
	c2.Set(auth.ContextKeyUserID, userA)
	limiter.Middleware()(c2)
	assert.True(t, c2.IsAborted())
	assert.Equal(t, http.StatusTooManyRequests, wA2.Code)

	// User B from same IP should succeed because key is user ID
	reqB := httptest.NewRequest(http.MethodGet, "/test", nil)
	reqB.RemoteAddr = "192.0.2.30:1234"
	wB := httptest.NewRecorder()
	cB, _ := gin.CreateTestContext(wB)
	cB.Request = reqB
	cB.Set(auth.ContextKeyUserID, userB)
	limiter.Middleware()(cB)
	assert.False(t, cB.IsAborted())
}

func TestRateLimiter_CleanupStale(t *testing.T) {
	ttl := 50 * time.Millisecond
	limiter := middleware.NewRateLimiter(10.0, 5,
		middleware.WithTTL(ttl),
		middleware.WithCleanupInterval(0), // Disable auto ticker for precise manual test
	)
	defer limiter.Close()

	// Populate buckets for 3 visitors
	limiter.Allow("visitor1")
	limiter.Allow("visitor2")
	limiter.Allow("visitor3")
	assert.Equal(t, 3, limiter.VisitorCount())

	// Immediate cleanup should remove nothing
	removed := limiter.CleanupStale(ttl)
	assert.Equal(t, 0, removed)
	assert.Equal(t, 3, limiter.VisitorCount())

	// Wait past TTL
	time.Sleep(70 * time.Millisecond)

	// Touch visitor1 to update its lastSeen
	limiter.Allow("visitor1")

	// Cleanup should prune visitor2 and visitor3, keeping visitor1
	removed = limiter.CleanupStale(ttl)
	assert.Equal(t, 2, removed)
	assert.Equal(t, 1, limiter.VisitorCount())

	// Wait past TTL again
	time.Sleep(70 * time.Millisecond)
	removed = limiter.CleanupStale(ttl)
	assert.Equal(t, 1, removed)
	assert.Equal(t, 0, limiter.VisitorCount())
}

func TestRateLimiter_BackgroundCleanup(t *testing.T) {
	ttl := 20 * time.Millisecond
	cleanupInterval := 25 * time.Millisecond
	limiter := middleware.NewRateLimiter(10.0, 5,
		middleware.WithTTL(ttl),
		middleware.WithCleanupInterval(cleanupInterval),
	)
	defer limiter.Close()

	limiter.Allow("auto-visitor-1")
	limiter.Allow("auto-visitor-2")
	assert.Equal(t, 2, limiter.VisitorCount())

	// Wait enough time for TTL + ticker to fire
	time.Sleep(80 * time.Millisecond)

	assert.Equal(t, 0, limiter.VisitorCount(), "background cleanup should have removed stale visitors")
}

func TestRateLimiter_CustomKeyFuncAndConvenience(t *testing.T) {
	customKeyFunc := func(c *gin.Context) string {
		return c.GetHeader("X-API-Key")
	}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.RateLimit(1.0, 1, middleware.WithKeyFunc(customKeyFunc)))
	r.GET("/api", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// Key A: first request 200
	reqA1 := httptest.NewRequest(http.MethodGet, "/api", nil)
	reqA1.Header.Set("X-API-Key", "key-alpha")
	wA1 := httptest.NewRecorder()
	r.ServeHTTP(wA1, reqA1)
	assert.Equal(t, http.StatusOK, wA1.Code)

	// Key A: second request 429
	reqA2 := httptest.NewRequest(http.MethodGet, "/api", nil)
	reqA2.Header.Set("X-API-Key", "key-alpha")
	wA2 := httptest.NewRecorder()
	r.ServeHTTP(wA2, reqA2)
	assert.Equal(t, http.StatusTooManyRequests, wA2.Code)

	// Key B: first request 200
	reqB := httptest.NewRequest(http.MethodGet, "/api", nil)
	reqB.Header.Set("X-API-Key", "key-beta")
	wB := httptest.NewRecorder()
	r.ServeHTTP(wB, reqB)
	assert.Equal(t, http.StatusOK, wB.Code)
}

func TestRateLimiter_ConcurrentAccessAndCleanup(t *testing.T) {
	limiter := middleware.NewRateLimiter(100.0, 50,
		middleware.WithTTL(50*time.Millisecond),
		middleware.WithCleanupInterval(10*time.Millisecond),
	)
	defer limiter.Close()

	done := make(chan struct{})
	concurrency := 50

	for i := 0; i < concurrency; i++ {
		go func(id int) {
			key := "client-" + strconv.Itoa(id%5)
			for {
				select {
				case <-done:
					return
				default:
					limiter.Allow(key)
					limiter.VisitorCount()
					time.Sleep(1 * time.Millisecond)
				}
			}
		}(i)
	}

	time.Sleep(100 * time.Millisecond)
	close(done)
}
