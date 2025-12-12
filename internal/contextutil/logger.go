package contextutil

import (
	"context"
	"log/slog"
)

type contextKey string

const loggerKey contextKey = "logger"

// LoggerFromContext extracts a logger from context if available, otherwise returns the default logger.
// This helper can be used by any package that needs to extract a logger from context.
func LoggerFromContext(ctx context.Context) *slog.Logger {
	if ctxLogger := ctx.Value(loggerKey); ctxLogger != nil {
		if l, ok := ctxLogger.(*slog.Logger); ok {
			return l
		}
	}
	return slog.Default()
}

// LoggerKey returns the context key used for storing loggers in context.
// This is exported so middleware can use it to set the logger in context.
func LoggerKey() contextKey {
	return loggerKey
}
