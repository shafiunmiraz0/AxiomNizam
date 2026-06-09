package apigateway

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// endpointRateLimiter tracks request counts per endpoint per key (IP, token, or API key).
type endpointRateLimiter struct {
	counters map[string]*slidingWindow
	mu       sync.RWMutex
	cleanup  time.Duration
}

// slidingWindow implements a simple sliding window rate limiter.
type slidingWindow struct {
	windowStart time.Time
	windowSize  time.Duration
	maxRequests int
	count       int
}

// newEndpointRateLimiter creates a rate limiter that runs a background cleanup.
func newEndpointRateLimiter(cleanupInterval time.Duration) *endpointRateLimiter {
	rl := &endpointRateLimiter{
		counters: make(map[string]*slidingWindow),
		cleanup:  cleanupInterval,
	}
	go rl.cleanupLoop()
	return rl
}

// Allow checks whether a request is allowed under the rate limit.
func (rl *endpointRateLimiter) Allow(key string, window time.Duration, maxRequests int) (allowed bool, remaining int, retryAfter time.Duration) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	counter, exists := rl.counters[key]
	now := time.Now()

	if !exists || now.Sub(counter.windowStart) >= counter.windowSize {
		// New window
		rl.counters[key] = &slidingWindow{
			windowStart: now,
			windowSize:  window,
			maxRequests: maxRequests,
			count:       1,
		}
		return true, maxRequests - 1, 0
	}

	if counter.count >= counter.maxRequests {
		// Rate limit exceeded
		retry := counter.windowStart.Add(counter.windowSize).Sub(now)
		return false, 0, retry
	}

	counter.count++
	return true, counter.maxRequests - counter.count, 0
}

// cleanupLoop periodically removes stale counters.
func (rl *endpointRateLimiter) cleanupLoop() {
	ticker := time.NewTicker(rl.cleanup)
	defer ticker.Stop()

	for range ticker.C {
		rl.mu.Lock()
		now := time.Now()
		for key, counter := range rl.counters {
			if now.Sub(counter.windowStart) > counter.windowSize*2 {
				delete(rl.counters, key)
			}
		}
		rl.mu.Unlock()
	}
}

// EndpointRateLimitMiddleware returns gin middleware that enforces per-endpoint
// rate limits. Endpoints without a specific limit use the gateway default
// (if configured). Rate limit info is added to response headers.
func (g *Gateway) EndpointRateLimitMiddleware() gin.HandlerFunc {
	limiter := newEndpointRateLimiter(5 * time.Minute)

	return func(c *gin.Context) {
		if !g.config.Enabled {
			c.Next()
			return
		}

		limit := g.GetEndpointRateLimit(c.Request.Method, c.Request.URL.Path)
		if limit == nil && g.config.DefaultRateLimit > 0 {
			// Use default rate limit
			limit = &EndpointRateLimit{
				Path:        c.Request.URL.Path,
				Method:      c.Request.Method,
				MaxRequests: g.config.DefaultRateLimit,
				Window:      g.config.DefaultRateWindow,
				KeyBy:       "ip",
			}
		}

		if limit == nil {
			c.Next()
			return
		}

		// Build rate limit key: endpoint+keyBy+value
		rateKey := fmt.Sprintf("%s:%s:%s:%s",
			limit.Method, limit.Path, limit.KeyBy, RateLimitKey(c, limit.KeyBy))

		allowed, remaining, retryAfter := limiter.Allow(rateKey, limit.Window, limit.MaxRequests)

		c.Header("X-RateLimit-Limit-Endpoint", fmt.Sprintf("%d", limit.MaxRequests))
		c.Header("X-RateLimit-Remaining-Endpoint", fmt.Sprintf("%d", remaining))
		c.Header("X-RateLimit-Window", limit.Window.String())

		if !allowed {
			c.Header("Retry-After", fmt.Sprintf("%d", int(retryAfter.Seconds())+1))
			c.JSON(http.StatusTooManyRequests, gin.H{
				"error":       "endpoint rate limit exceeded",
				"message":     fmt.Sprintf("too many requests to %s %s. limit: %d per %s", limit.Method, limit.Path, limit.MaxRequests, limit.Window),
				"limit":       limit.MaxRequests,
				"window":      limit.Window.String(),
				"retry_after": retryAfter.String(),
			})
			c.Abort()
			return
		}

		c.Next()
	}
}
