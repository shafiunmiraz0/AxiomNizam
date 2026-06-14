package cache

import (
	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// CacheMiddleware provides HTTP caching for GET requests
type CacheMiddleware struct {
	cache   Cache
	ttl     time.Duration
	enabled bool
}

// NewCacheMiddleware creates a new cache middleware
func NewCacheMiddleware(cache Cache, ttl time.Duration) *CacheMiddleware {
	if ttl == 0 {
		ttl = 5 * time.Minute
	}

	return &CacheMiddleware{
		cache:   cache,
		ttl:     ttl,
		enabled: true,
	}
}

// GenerateCacheKey creates a cache key from request
func (cm *CacheMiddleware) GenerateCacheKey(c *gin.Context) string {
	// Create a key from method, path, and query parameters
	h := md5.New()
	h.Write([]byte(c.Request.Method + ":" + c.Request.URL.Path + ":" + c.Request.URL.RawQuery))
	return "http:" + hex.EncodeToString(h.Sum(nil))
}

// Middleware returns the gin middleware function
func (cm *CacheMiddleware) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Only cache GET requests
		if c.Request.Method != http.MethodGet || !cm.enabled {
			c.Next()
			return
		}

		// Skip caching for certain paths
		if cm.shouldSkipCache(c.Request.URL.Path) {
			c.Next()
			return
		}

		cacheKey := cm.GenerateCacheKey(c)

		// Try to get from cache
		cached, err := cm.cache.Get(c.Request.Context(), cacheKey)
		if err == nil {
			logging.Z().Info(fmt.Sprintf("Cache HIT for %s", c.Request.URL.Path))
			c.Data(http.StatusOK, "application/json", cached.([]byte))
			c.Header("X-Cache", "HIT")
			return
		}

		// Create a response writer wrapper to capture response
		writer := &responseWriter{
			ResponseWriter: c.Writer,
			body:           make([]byte, 0),
		}
		c.Writer = writer

		// Continue with request
		c.Next()

		// Only cache successful responses
		if c.Writer.Status() == http.StatusOK {
			// Store in cache
			if err := cm.cache.Set(c.Request.Context(), cacheKey, writer.body, cm.ttl); err != nil {
				logging.Z().Info(fmt.Sprintf("Error caching response: %v", err))
			} else {
				logging.Z().Info(fmt.Sprintf("Cache SET for %s (TTL: %s)", c.Request.URL.Path, cm.ttl))
				c.Header("X-Cache", "MISS")
			}
		}
	}
}

// shouldSkipCache returns true if cache should be skipped for this path
func (cm *CacheMiddleware) shouldSkipCache(path string) bool {
	skipPaths := []string{
		"/api/auth/login",
		"/api/auth/register",
		"/api/auth/logout",
		"/health",
		"/metrics",
	}

	for _, skip := range skipPaths {
		if path == skip {
			return true
		}
	}

	return false
}

// responseWriter wraps gin.ResponseWriter to capture response body
type responseWriter struct {
	gin.ResponseWriter
	body []byte
}

// Write implements io.Writer
func (rw *responseWriter) Write(data []byte) (int, error) {
	rw.body = append(rw.body, data...)
	return rw.ResponseWriter.Write(data)
}

// CacheControl middleware adds cache control headers
func CacheControl(maxAge int) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", fmt.Sprintf("public, max-age=%d", maxAge))
		c.Header("Expires", time.Now().Add(time.Duration(maxAge)*time.Second).Format(http.TimeFormat))
		c.Next()
	}
}

// NoCacheControl middleware disables caching for response
func NoCacheControl() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
		c.Header("Pragma", "no-cache")
		c.Header("Expires", "0")
		c.Next()
	}
}

// ETag middleware adds ETag support for cache validation
func ETag(c *gin.Context) {
	c.Next()

	// Only add ETag for successful GET requests
	if c.Request.Method == http.MethodGet && c.Writer.Status() == http.StatusOK {
		if body, ok := c.Get(gin.BodyBytesKey); ok {
			h := md5.New()
			h.Write(body.([]byte))
			etag := `"` + hex.EncodeToString(h.Sum(nil)) + `"`
			c.Header("ETag", etag)

			// Check If-None-Match header
			if match := c.GetHeader("If-None-Match"); match == etag {
				c.Status(http.StatusNotModified)
				c.Abort()
			}
		}
	}
}

// SetCacheEnabled enables or disables caching
func (cm *CacheMiddleware) SetCacheEnabled(enabled bool) {
	cm.enabled = enabled
	logging.Z().Info(fmt.Sprintf("Cache middleware enabled: %v", enabled))
}

// InvalidateCache removes a key from cache
func (cm *CacheMiddleware) InvalidateCache(key string) error {
	if err := cm.cache.Delete(nil, key); err != nil {
		logging.Z().Info(fmt.Sprintf("Error invalidating cache key %s: %v", key, err))
		return err
	}
	logging.Z().Info(fmt.Sprintf("Cache invalidated: %s", key))
	return nil
}

// InvalidatePattern invalidates all keys matching a pattern
// Note: This is a simple implementation. For Redis, you might want to use KEYS command
func (cm *CacheMiddleware) InvalidatePattern(pattern string) error {
	// For now, we'll just log this - actual implementation depends on cache backend
	logging.Z().Info(fmt.Sprintf("Invalidating cache pattern: %s", pattern))
	return nil
}

// CacheStats returns cache statistics
type CacheStats struct {
	TotalHits   int64
	TotalMisses int64
	HitRate     float64
}

// GetStats returns current cache statistics (placeholder)
func (cm *CacheMiddleware) GetStats() *CacheStats {
	return &CacheStats{
		TotalHits:   0,
		TotalMisses: 0,
		HitRate:     0,
	}
}
