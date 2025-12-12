package handlers

import (
	"context"
	"encoding/json"
	"net/http"

	"helloworld-ai/internal/contextutil"
	"helloworld-ai/internal/indexer"
)

// IndexHandler handles HTTP requests for triggering re-indexing.
type IndexHandler struct {
	indexerPipeline *indexer.Pipeline
}

// NewIndexHandler creates a new IndexHandler.
func NewIndexHandler(indexerPipeline *indexer.Pipeline) *IndexHandler {
	return &IndexHandler{
		indexerPipeline: indexerPipeline,
	}
}

// IndexResponse represents the response from the index endpoint.
//
// swagger:model IndexResponse
type IndexResponse struct {
	Message string `json:"message"`
	Status  string `json:"status"`
}

// ServeHTTP handles HTTP requests for triggering re-indexing.
//
// Trigger re-indexing of all markdown files in configured vaults.
// By default, only changed files are re-indexed. Use the force query parameter
// to clear all existing data and rebuild the index from scratch.
//
// swagger:route POST /api/index triggerIndex
//
// # Trigger re-indexing of vaults
//
// Starts an asynchronous re-indexing process that scans all markdown files
// in the configured vaults and updates the search index. The operation runs
// in the background and returns immediately with an accepted status.
//
// ---
// produces:
// - application/json
// parameters:
//   - in: query
//     name: force
//     type: boolean
//     default: false
//     description: If true, clears all existing indexed data before re-indexing
//
// responses:
//
//	'202':
//	  description: Indexing started successfully
//	  schema:
//	    "$ref": "#/definitions/IndexResponse"
//	'405':
//	  description: Method not allowed
//	  schema:
//	    "$ref": "#/definitions/ErrorResponse"
//	'500':
//	  description: Internal server error
//	  schema:
//	    "$ref": "#/definitions/ErrorResponse"
func (h *IndexHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := contextutil.LoggerFromContext(ctx)

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
		indexLogger := contextutil.LoggerFromContext(indexCtx)
		if force {
			// Clear all existing data first
			if err := h.indexerPipeline.ClearAll(indexCtx); err != nil {
				indexLogger.ErrorContext(indexCtx, "failed to clear existing data", "error", err)
				return
			}
			indexLogger.InfoContext(indexCtx, "cleared all existing indexed data")
		}
		if err := h.indexerPipeline.IndexAll(indexCtx); err != nil {
			indexLogger.ErrorContext(indexCtx, "re-indexing completed with errors", "error", err)
		} else {
			indexLogger.InfoContext(indexCtx, "re-indexing completed successfully")
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
