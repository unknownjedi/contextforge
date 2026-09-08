package provider

import (
	"context"
	"fmt"
	"math/rand"
	"net/http"
	"strconv"
	"time"
)

// RetryConfig configures the exponential backoff retry behavior.
type RetryConfig struct {
	MaxRetries     int           // Maximum number of retry attempts
	InitialBackoff time.Duration // Initial delay before the first retry
	MaxBackoff     time.Duration // Maximum delay cap between retries
}

// DefaultRetryConfig returns sensible default retry settings.
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:     3,
		InitialBackoff: 200 * time.Millisecond,
		MaxBackoff:     5 * time.Second,
	}
}

// normalizeRetryConfig ensures retry settings have valid positive values.
func normalizeRetryConfig(cfg RetryConfig) RetryConfig {
	if cfg.MaxRetries < 0 {
		cfg.MaxRetries = 0
	} else if cfg.MaxRetries == 0 {
		cfg.MaxRetries = 3
	}
	if cfg.InitialBackoff <= 0 {
		cfg.InitialBackoff = 200 * time.Millisecond
	}
	if cfg.MaxBackoff <= 0 {
		cfg.MaxBackoff = 5 * time.Second
	}
	return cfg
}

// doWithRetry executes an HTTP request with exponential backoff on 429 and 5xx errors.
func doWithRetry(ctx context.Context, client *http.Client, makeReq func() (*http.Request, error), cfg RetryConfig) (*http.Response, error) {
	cfg = normalizeRetryConfig(cfg)
	if client == nil {
		client = http.DefaultClient
	}

	var lastResp *http.Response
	var lastErr error

	for attempt := 0; attempt <= cfg.MaxRetries; attempt++ {
		if err := ctx.Err(); err != nil {
			if lastResp != nil && lastResp.Body != nil {
				_ = lastResp.Body.Close()
			}
			return nil, err
		}

		req, err := makeReq()
		if err != nil {
			return nil, fmt.Errorf("failed to create HTTP request: %w", err)
		}
		req = req.WithContext(ctx)

		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			// Check if context canceled
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			if attempt < cfg.MaxRetries {
				sleepDuration := calculateBackoff(attempt, cfg, nil)
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(sleepDuration):
					continue
				}
			}
			return nil, fmt.Errorf("request failed after %d retries: %w", attempt, lastErr)
		}

		// Check if status code warrants a retry (429 Rate Limit or 5xx Server Error)
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			if attempt < cfg.MaxRetries {
				sleepDuration := calculateBackoff(attempt, cfg, resp)
				_ = resp.Body.Close()
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(sleepDuration):
					continue
				}
			}
		}

		return resp, nil
	}

	if lastResp != nil {
		return lastResp, nil
	}
	return nil, lastErr
}

// calculateBackoff determines the sleep duration based on attempt count or Retry-After header.
func calculateBackoff(attempt int, cfg RetryConfig, resp *http.Response) time.Duration {
	if resp != nil {
		if retryAfter := resp.Header.Get("Retry-After"); retryAfter != "" {
			if seconds, err := strconv.Atoi(retryAfter); err == nil && seconds > 0 {
				d := time.Duration(seconds) * time.Second
				if d <= cfg.MaxBackoff {
					return d
				}
				return cfg.MaxBackoff
			}
		}
	}

	multiplier := 1 << attempt
	backoff := cfg.InitialBackoff * time.Duration(multiplier)
	if backoff > cfg.MaxBackoff {
		backoff = cfg.MaxBackoff
	}

	// Add jitter (up to 20%)
	jitter := time.Duration(rand.Int63n(int64(backoff)/5 + 1))
	total := backoff + jitter
	if total > cfg.MaxBackoff {
		return cfg.MaxBackoff
	}
	return total
}
