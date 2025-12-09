package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"helloworld-ai/internal/rag"
	"helloworld-ai/internal/storage"
)

// AskHandler handles HTTP requests for RAG queries.
type AskHandler struct {
	ragEngine rag.Engine
	vaultRepo storage.VaultStore
	logger    *slog.Logger
}

// NewAskHandler creates a new AskHandler.
func NewAskHandler(ragEngine rag.Engine, vaultRepo storage.VaultStore) *AskHandler {
	return &AskHandler{
		ragEngine: ragEngine,
		vaultRepo: vaultRepo,
		logger:    slog.Default(),
	}
}

// AskRequest represents the HTTP request payload for RAG queries.
// This mirrors the rag.AskRequest but is defined here for HTTP layer separation.
type AskRequest struct {
	Question string   `json:"question"`
	Vaults   []string `json:"vaults,omitempty"`
	Folders  []string `json:"folders,omitempty"`
	K        int      `json:"k,omitempty"`
}

// AskResponse represents the HTTP response payload for RAG queries.
// This mirrors the rag.AskResponse but is defined here for HTTP layer separation.
type AskResponse struct {
	Answer     string              `json:"answer"`
	References []ReferenceResponse `json:"references"`
}

// ReferenceResponse represents a reference in the HTTP response.
type ReferenceResponse struct {
	Vault       string `json:"vault"`
	RelPath     string `json:"rel_path"`
	HeadingPath string `json:"heading_path"`
	ChunkIndex  int    `json:"chunk_index"`
}

// ErrorResponse represents an error response.
type ErrorResponse struct {
	Error string `json:"error"`
}

// getLogger extracts logger from context or returns default logger.
func (h *AskHandler) getLogger(ctx context.Context) *slog.Logger {
	type loggerKeyType string
	const loggerKey loggerKeyType = "logger"
	if ctxLogger := ctx.Value(loggerKey); ctxLogger != nil {
		if l, ok := ctxLogger.(*slog.Logger); ok {
			return l
		}
	}
	return h.logger
}

// ServeHTTP handles HTTP requests for RAG queries.
func (h *AskHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := h.getLogger(ctx)

	if r.Method != http.MethodPost {
		logger.WarnContext(ctx, "method not allowed", "method", r.Method)
		h.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var req AskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logger.WarnContext(ctx, "invalid request body", "error", err)
		h.writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Validate request
	if req.Question == "" {
		logger.WarnContext(ctx, "empty question in request")
		h.writeError(w, http.StatusBadRequest, "Question is required")
		return
	}

	// Default K to 5 if zero
	if req.K == 0 {
		req.K = 5
	}

	// Enforce max K = 20
	if req.K > 20 {
		req.K = 20
	}

	// Validate vault names if provided
	if len(req.Vaults) > 0 {
		allVaults, err := h.vaultRepo.ListAll(ctx)
		if err != nil {
			logger.ErrorContext(ctx, "failed to list vaults for validation", "error", err)
			h.writeError(w, http.StatusInternalServerError, "Failed to validate vaults")
			return
		}

		// Build set of valid vault names
		validVaults := make(map[string]bool)
		for _, vault := range allVaults {
			validVaults[vault.Name] = true
		}

		// Validate each requested vault
		for _, vaultName := range req.Vaults {
			if !validVaults[vaultName] {
				logger.WarnContext(ctx, "invalid vault name", "vault", vaultName)
				h.writeError(w, http.StatusBadRequest, fmt.Sprintf("Invalid vault name: %s", vaultName))
				return
			}
		}
	}

	// Convert HTTP request to RAG request
	ragReq := rag.AskRequest{
		Question: req.Question,
		Vaults:   req.Vaults,
		Folders:  req.Folders,
		K:        req.K,
	}

	// Call RAG engine
	ragResp, err := h.ragEngine.Ask(ctx, ragReq)
	if err != nil {
		h.handleRAGError(w, ctx, err, "Failed to process RAG query")
		return
	}

	// Convert RAG response to HTTP response
	references := make([]ReferenceResponse, len(ragResp.References))
	for i, ref := range ragResp.References {
		references[i] = ReferenceResponse{
			Vault:       ref.Vault,
			RelPath:     ref.RelPath,
			HeadingPath: ref.HeadingPath,
			ChunkIndex:  ref.ChunkIndex,
		}
	}

	resp := AskResponse{
		Answer:     ragResp.Answer,
		References: references,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		logger.ErrorContext(ctx, "failed to encode response", "error", err)
		h.writeError(w, http.StatusInternalServerError, "Failed to encode response")
		return
	}
}

// handleRAGError maps RAG engine errors to appropriate HTTP status codes.
func (h *AskHandler) handleRAGError(w http.ResponseWriter, ctx context.Context, err error, defaultMsg string) {
	logger := h.getLogger(ctx)
	logger.ErrorContext(ctx, "RAG engine error", "error", err)

	if err == nil {
		h.writeError(w, http.StatusInternalServerError, defaultMsg)
		return
	}

	// Check error message for specific error types
	errMsg := strings.ToLower(err.Error())

	// Vector store errors -> 503
	if strings.Contains(errMsg, "vector store") ||
		strings.Contains(errMsg, "vectorstore") ||
		strings.Contains(errMsg, "qdrant") ||
		strings.Contains(errMsg, "failed to search") {
		h.writeError(w, http.StatusServiceUnavailable, "Vector store unavailable")
		return
	}

	// LLM/embedding errors -> 502
	if strings.Contains(errMsg, "embed") ||
		strings.Contains(errMsg, "llm") ||
		strings.Contains(errMsg, "failed to get llm") {
		h.writeError(w, http.StatusBadGateway, "External service error")
		return
	}

	// Default to 500
	h.writeError(w, http.StatusInternalServerError, defaultMsg)
}

// writeError writes an error response.
func (h *AskHandler) writeError(w http.ResponseWriter, statusCode int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(ErrorResponse{
		Error: message,
	})
}
