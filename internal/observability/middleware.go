package observability

import (
	"strings"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	"axiomnizam.bitbd.net/axiomnizam/internal/utils"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

const (
	// HeaderRequestID is the standard request ID header.
	HeaderRequestID = "X-Request-ID"

	// HeaderTraceID is the response header carrying the trace ID.
	HeaderTraceID = "X-Trace-ID"

	// HeaderTraceParent is the W3C Trace Context header.
	HeaderTraceParent = "traceparent"
)

// RequestIDMiddleware generates or propagates a request ID for every request.
// If the incoming request carries X-Request-ID, it is reused; otherwise a
// new UUID is generated.  The ID is injected into the logging context and
// returned in the response header.
func RequestIDMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		rid := c.GetHeader(HeaderRequestID)
		if rid == "" {
			rid = uuid.New().String()
		}

		// Derive trace ID — prefer W3C traceparent, fall back to request ID.
		traceID := extractTraceID(c.GetHeader(HeaderTraceParent))
		if traceID == "" {
			traceID = rid
		}

		// Inject into context for downstream logging.
		ctx := logging.WithRequestID(c.Request.Context(), rid)
		ctx = logging.WithTraceID(ctx, traceID)
		c.Request = c.Request.WithContext(ctx)

		// Propagate to response so clients can correlate.
		c.Writer.Header().Set(HeaderRequestID, rid)
		c.Writer.Header().Set(HeaderTraceID, traceID)

		c.Next()
	}
}

// AccessLogMiddleware logs every HTTP request with structured fields
// including method, path, status, latency, trace_id, and request_id.
func AccessLogMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery

		c.Next()

		latency := time.Since(start)
		status := c.Writer.Status()

		logger := logging.TraceLogger(c.Request.Context())
		fields := []zap.Field{
			zap.String("method", c.Request.Method),
			zap.String("path", path),
			zap.Int("status", status),
			zap.Duration("latency", latency),
			zap.String("client_ip", utils.RealIP(c)),
			zap.Int("body_size", c.Writer.Size()),
		}
		if query != "" {
			fields = append(fields, zap.String("query", query))
		}
		if ua := c.Request.UserAgent(); ua != "" {
			fields = append(fields, zap.String("user_agent", ua))
		}

		msg := "HTTP request"
		switch {
		case status >= 500:
			logger.Error(msg, fields...)
		case status >= 400:
			logger.Warn(msg, fields...)
		default:
			logger.Info(msg, fields...)
		}
	}
}

// extractTraceID parses a W3C traceparent header (version-traceid-spanid-flags)
// and returns the 32-hex-char trace ID.  Returns empty string on malformed input.
func extractTraceID(traceparent string) string {
	// Format: 00-<trace_id 32 hex>-<span_id 16 hex>-<flags 2 hex>
	parts := strings.Split(traceparent, "-")
	if len(parts) != 4 {
		return ""
	}
	if len(parts[1]) != 32 {
		return ""
	}
	return parts[1]
}
