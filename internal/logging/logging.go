// Package logging is the canonical project-wide logging entry point.
//
// P3.1 — today the workspace uses three loggers:
//
//   - stdlib `log` (~70 files)
//   - `go.uber.org/zap` directly (~7 files)
//   - `axiomnizam.bitbd.net/axiomnizam/internal/utils/logger` (zap wrapper, 4 files)
//
// Rather than force a mass migration in one pass this package gives us
// a single front door: `logging.L()` returns a `*zap.Logger` that is
// consistent, context-aware, and cheap to call.  New code should import
// THIS package; legacy imports of `log` / `utils/logger` continue to
// work and can be migrated incrementally.
//
// The implementation piggybacks on the existing `utils/logger` wrapper
// (which already builds zap with project conventions) so we keep a
// single binary dependency graph.
package logging

import (
	"context"
	"sync"

	"axiomnizam.bitbd.net/axiomnizam/internal/utils/logger"

	"go.uber.org/zap"
)

var (
	initOnce sync.Once
	global   *logger.Logger
)

// Init configures the global logger.  `env` is "production" or
// "development".  Safe to call multiple times; only the first call
// wins.
func Init(env string) {
	initOnce.Do(func() {
		l, err := logger.New(env)
		if err != nil {
			// Fall back to development on failure — never panic during
			// logging setup.
			l, _ = logger.NewDevelopment()
		}
		global = l
	})
}

// L returns the process-wide logger, initialising it with a
// development config on first use if Init was not called.
func L() *logger.Logger {
	if global == nil {
		Init("development")
	}
	return global
}

// Z returns the underlying *zap.Logger for call sites that want raw
// zap API access (e.g. `.With(zap.String(...))`).
func Z() *zap.Logger { return L().Logger }

// FromContext returns a logger enriched with any request/correlation
// IDs the caller has attached to the context.
func FromContext(ctx context.Context) *logger.Logger {
	return logger.FromContext(ctx, L())
}

// With returns a logger with additional structured fields.
func With(fields ...zap.Field) *zap.Logger {
	return Z().With(fields...)
}

// For returns a named logger for a specific module/component.
// Example: logging.For("storage") produces a logger tagged "[storage]".
func For(module string) *zap.Logger {
	return Z().Named(module)
}
