package middleware

import (
	"fmt"
	"math"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/your-org/contextforge/internal/auth"
)

// KeyFunc extracts a rate limit identifier from the Gin context.
type KeyFunc func(c *gin.Context) string

// DefaultKeyFunc extracts the authenticated user ID if present; otherwise, it falls back to the client IP.
func DefaultKeyFunc(c *gin.Context) string {
	if uid, err := auth.GetUserID(c); err == nil && uid != uuid.Nil {
		return "user:" + uid.String()
	}
	ip := c.ClientIP()
	if ip == "" {
		ip = "127.0.0.1"
	}
	return "ip:" + ip
}

// IPKeyFunc extracts the client IP address as the rate limit key.
func IPKeyFunc(c *gin.Context) string {
	ip := c.ClientIP()
	if ip == "" {
		ip = "127.0.0.1"
	}
	return "ip:" + ip
}

// UserKeyFunc extracts the authenticated user ID if present, otherwise falling back to client IP.
func UserKeyFunc(c *gin.Context) string {
	return DefaultKeyFunc(c)
}

// bucket represents an in-memory token bucket for a specific visitor key.
type bucket struct {
	tokens     float64
	lastRefill time.Time
	lastSeen   time.Time
}

// Option configures a RateLimiter.
type Option func(*RateLimiter)

// WithTTL sets the inactivity duration after which a visitor bucket is evicted.
func WithTTL(ttl time.Duration) Option {
	return func(rl *RateLimiter) {
		if ttl > 0 {
			rl.ttl = ttl
		}
	}
}

// WithCleanupInterval sets the period for automatic background stale bucket cleanup.
func WithCleanupInterval(interval time.Duration) Option {
	return func(rl *RateLimiter) {
		rl.cleanupInterval = interval
	}
}

// WithKeyFunc overrides the default key extraction function.
func WithKeyFunc(fn KeyFunc) Option {
	return func(rl *RateLimiter) {
		if fn != nil {
			rl.keyFunc = fn
		}
	}
}

// RateLimiter manages token buckets per client key with thread-safe operations.
type RateLimiter struct {
	mu              sync.Mutex
	rate            float64       // Tokens replenished per second
	burst           int           // Maximum token capacity
	ttl             time.Duration // Time-to-live before evicting stale visitors
	cleanupInterval time.Duration // Interval between background eviction sweeps
	keyFunc         KeyFunc
	visitors        map[string]*bucket
	done            chan struct{}
	stopOnce        sync.Once
}

// NewRateLimiter creates a new RateLimiter.
// rate is the number of tokens added per second; burst is the maximum bucket capacity.
func NewRateLimiter(rate float64, burst int, opts ...Option) *RateLimiter {
	if rate <= 0 {
		rate = 1.0
	}
	if burst <= 0 {
		burst = 1
	}

	rl := &RateLimiter{
		rate:            rate,
		burst:           burst,
		ttl:             5 * time.Minute,
		cleanupInterval: 1 * time.Minute,
		keyFunc:         DefaultKeyFunc,
		visitors:        make(map[string]*bucket),
		done:            make(chan struct{}),
	}

	for _, opt := range opts {
		opt(rl)
	}

	if rl.cleanupInterval > 0 {
		rl.startCleanup()
	}

	return rl
}

// Allow checks if 1 token is available for the given visitor key.
// Returns whether the request is allowed and the wait duration until 1 token is available.
func (rl *RateLimiter) Allow(key string) (bool, time.Duration) {
	return rl.AllowN(key, 1.0)
}

// AllowN checks if n tokens are available for the given visitor key.
func (rl *RateLimiter) AllowN(key string, n float64) (bool, time.Duration) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	b, exists := rl.visitors[key]
	if !exists {
		b = &bucket{
			tokens:     float64(rl.burst),
			lastRefill: now,
			lastSeen:   now,
		}
		rl.visitors[key] = b
	} else {
		// Refill tokens proportional to elapsed time
		elapsed := now.Sub(b.lastRefill).Seconds()
		if elapsed > 0 {
			b.tokens += elapsed * rl.rate
			if b.tokens > float64(rl.burst) {
				b.tokens = float64(rl.burst)
			}
			b.lastRefill = now
		}
		b.lastSeen = now
	}

	if b.tokens >= n {
		b.tokens -= n
		return true, 0
	}

	// Calculate wait time until n tokens are available
	missing := n - b.tokens
	waitSec := missing / rl.rate
	waitDuration := time.Duration(waitSec * float64(time.Second))
	if waitDuration < time.Millisecond {
		waitDuration = time.Millisecond
	}

	return false, waitDuration
}

// CleanupStale evicts visitor buckets that have been inactive longer than the specified ttl.
// Returns the count of pruned buckets.
func (rl *RateLimiter) CleanupStale(ttl time.Duration) int {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	removed := 0
	for key, b := range rl.visitors {
		if now.Sub(b.lastSeen) > ttl {
			delete(rl.visitors, key)
			removed++
		}
	}
	return removed
}

// VisitorCount returns the number of tracked visitor buckets.
func (rl *RateLimiter) VisitorCount() int {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	return len(rl.visitors)
}

// Close gracefully terminates the background cleanup goroutine.
func (rl *RateLimiter) Close() {
	rl.stopOnce.Do(func() {
		close(rl.done)
	})
}

// startCleanup initiates a background goroutine to periodically purge stale visitors.
func (rl *RateLimiter) startCleanup() {
	go func() {
		ticker := time.NewTicker(rl.cleanupInterval)
		defer ticker.Stop()

		for {
			select {
			case <-rl.done:
				return
			case <-ticker.C:
				rl.CleanupStale(rl.ttl)
			}
		}
	}()
}

// Middleware returns a Gin HandlerFunc enforcing the rate limit policy.
func (rl *RateLimiter) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		key := rl.keyFunc(c)
		allowed, waitDuration := rl.Allow(key)
		if !allowed {
			retryAfterSec := int(math.Ceil(waitDuration.Seconds()))
			if retryAfterSec < 1 {
				retryAfterSec = 1
			}

			c.Header("Retry-After", strconv.Itoa(retryAfterSec))
			c.Header("Content-Type", "application/problem+json")
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"type":     "https://contextforge.dev/errors/rate-limit-exceeded",
				"title":    "Too Many Requests",
				"status":   http.StatusTooManyRequests,
				"detail":   fmt.Sprintf("Rate limit exceeded. Try again in %d seconds.", retryAfterSec),
				"instance": c.Request.URL.Path,
			})
			return
		}

		c.Next()
	}
}

// RateLimit is a convenience constructor that creates a RateLimiter and returns its Gin middleware.
func RateLimit(rate float64, burst int, opts ...Option) gin.HandlerFunc {
	limiter := NewRateLimiter(rate, burst, opts...)
	return limiter.Middleware()
}
