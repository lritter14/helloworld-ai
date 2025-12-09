package handlers

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"helloworld-ai/internal/indexer"
)

// IndexHandler handles HTTP requests for triggering re-indexing.
type IndexHandler struct {
	indexerPipeline *indexer.Pipeline
	logger          *slog.Logger
}

// NewIndexHandler creates a new IndexHandler.
func NewIndexHandler(indexerPipeline *indexer.Pipeline) *IndexHandler {
	return &IndexHandler{
		indexerPipeline: indexerPipeline,
		logger:          slog.Default(),
	}
}

// getLogger extracts logger from context or returns default logger.
func (h *IndexHandler) getLogger(ctx context.Context) *slog.Logger {
	type loggerKeyType string
	const loggerKey loggerKeyType = "logger"
	if ctxLogger := ctx.Value(loggerKey); ctxLogger != nil {
		if l, ok := ctxLogger.(*slog.Logger); ok {
			return l
		}
	}
	return h.logger
}

// IndexResponse represents the response from the index endpoint.
type IndexResponse struct {
	Message string `json:"message"`
	Status  string `json:"status"`
}

// ServeHTTP handles HTTP requests for triggering re-indexing.
func (h *IndexHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := h.getLogger(ctx)

	if r.Method != http.MethodPost {
		logger.WarnContext(ctx, "method not allowed", "method", r.Method)
		h.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	// Check for force parameter
	force := r.URL.Query().Get("force") == "true"

	if force {
		logger.InfoContext(ctx, "force re-indexing triggered via API")
	} else {
		logger.InfoContext(ctx, "re-indexing triggered via API")
	}

	// Trigger indexing in a goroutine so it doesn't block the HTTP response
	// Use background context so indexing continues after HTTP request completes
	go func() {
		indexCtx := context.Background()
		if force {
			// Clear all existing data first
			if err := h.indexerPipeline.ClearAll(indexCtx); err != nil {
				logger.ErrorContext(indexCtx, "failed to clear existing data", "error", err)
				return
			}
			logger.InfoContext(indexCtx, "cleared all existing indexed data")
		}
		if err := h.indexerPipeline.IndexAll(indexCtx); err != nil {
			logger.ErrorContext(indexCtx, "re-indexing completed with errors", "error", err)
		} else {
			logger.InfoContext(indexCtx, "re-indexing completed successfully")
		}
	}()

	// Return immediately with accepted status
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	message := "Indexing started. Check server logs for progress."
	if force {
		message = "Force re-indexing started (all existing data cleared). Check server logs for progress."
	}
	_ = json.NewEncoder(w).Encode(IndexResponse{
		Message: message,
		Status:  "accepted",
	})
}

// writeError writes an error response.
func (h *IndexHandler) writeError(w http.ResponseWriter, statusCode int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(ErrorResponse{
		Error: message,
	})
}
