package utils

import (
	"net"
	"strings"

	"github.com/gin-gonic/gin"
)

// realIPKey is the context key for the resolved client IP.
const realIPKey = "realClientIP"

// RealIPMiddleware extracts the real client IP from proxy headers
// and stores it in Gin's context. Place this middleware first in the chain.
func RealIPMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(realIPKey, resolveRealIP(c))
		c.Next()
	}
}

// RealIP returns the real client IP for this request.
// Falls back to RemoteAddr if no proxy headers are present.
func RealIP(c *gin.Context) string {
	if ip, ok := c.Get(realIPKey); ok {
		if s, ok := ip.(string); ok && s != "" {
			return s
		}
	}
	return resolveRealIP(c)
}

// resolveRealIP extracts the leftmost (original client) IP from proxy
// headers, WITHOUT Gin's trusted-proxy filtering.
// Priority: X-Forwarded-For → X-Real-IP → RemoteAddr.
func resolveRealIP(c *gin.Context) string {
	if xff := c.GetHeader("X-Forwarded-For"); xff != "" {
		// X-Forwarded-For format: "client, proxy1, proxy2"
		// The leftmost IP is the original client.
		if ip := strings.TrimSpace(strings.SplitN(xff, ",", 2)[0]); ip != "" {
			if net.ParseIP(ip) != nil {
				return ip
			}
		}
	}
	if xri := c.GetHeader("X-Real-IP"); xri != "" {
		if ip := strings.TrimSpace(xri); ip != "" {
			if net.ParseIP(ip) != nil {
				return ip
			}
		}
	}
	// Fall back to direct connection IP (strip port).
	addr := c.Request.RemoteAddr
	if host, _, err := net.SplitHostPort(addr); err == nil {
		return host
	}
	return addr
}
