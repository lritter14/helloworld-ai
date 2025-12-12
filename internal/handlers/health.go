package handlers

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"helloworld-ai/internal/contextutil"
	"helloworld-ai/internal/llm"
	"helloworld-ai/internal/vectorstore"
)

// HealthHandler handles HTTP requests for health checks.
type HealthHandler struct {
	vectorStore        vectorstore.VectorStore
	llmClient          *llm.Client
	collectionName     string
	healthCheckTimeout time.Duration
}

// NewHealthHandler creates a new HealthHandler.
func NewHealthHandler(vectorStore vectorstore.VectorStore, llmClient *llm.Client, collectionName string) *HealthHandler {
	return &HealthHandler{
		vectorStore:        vectorStore,
		llmClient:          llmClient,
		collectionName:     collectionName,
		healthCheckTimeout: 5 * time.Second,
	}
}

// HealthResponse represents the health check response.
//
// swagger:model HealthResponse
type HealthResponse struct {
	// Overall health status: "healthy", "degraded", or "unhealthy"
	Status string `json:"status"`

	// Timestamp of the health check
	Timestamp string `json:"timestamp"`

	// Individual check results
	Checks map[string]string `json:"checks"`

	// List of issues (only present if status is degraded or unhealthy)
	Issues []string `json:"issues,omitempty"`
}

// ServeHTTP handles HTTP requests for health checks.
//
// Check the health status of the system and its dependencies.
// Returns 200 OK if healthy, 503 Service Unavailable if degraded or unhealthy.
//
// swagger:route GET /api/health healthCheck
//
// # Health check endpoint
//
// Returns the health status of the system including vector store and LLM service.
//
// ---
// produces:
// - application/json
// responses:
//
//	'200':
//	  description: System is healthy
//	  schema:
//	    "$ref": "#/definitions/HealthResponse"
//	'503':
//	  description: System is degraded or unhealthy
//	  schema:
//	    "$ref": "#/definitions/HealthResponse"
func (h *HealthHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := contextutil.LoggerFromContext(ctx)

	if r.Method != http.MethodGet {
		logger.WarnContext(ctx, "method not allowed", "method", r.Method)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	// Create context with timeout for health checks
	checkCtx, cancel := context.WithTimeout(ctx, h.healthCheckTimeout)
	defer cancel()

	checks := make(map[string]string)
	var issues []string

	// Check vector store (Qdrant)
	vectorStoreOK := h.checkVectorStore(checkCtx, logger)
	if vectorStoreOK {
		checks["vector_store"] = "ok"
	} else {
		checks["vector_store"] = "error"
		issues = append(issues, "vector_store_unavailable")
	}

	// Check LLM service (optional - skip to avoid latency)
	// For now, we'll skip LLM health check as it may add latency
	// and the vector store is the critical dependency

	// Determine overall status
	status := "healthy"
	httpStatus := http.StatusOK
	if len(issues) > 0 {
		status = "unhealthy"
		httpStatus = http.StatusServiceUnavailable
	}

	response := HealthResponse{
		Status:    status,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Checks:    checks,
	}

	if len(issues) > 0 {
		response.Issues = issues
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(httpStatus)

	if err := json.NewEncoder(w).Encode(response); err != nil {
		logger.ErrorContext(ctx, "failed to encode health response", "error", err)
	}
}

// checkVectorStore checks if the vector store is accessible.
func (h *HealthHandler) checkVectorStore(ctx context.Context, logger *slog.Logger) bool {
	exists, err := h.vectorStore.CollectionExists(ctx, h.collectionName)
	if err != nil {
		logger.WarnContext(ctx, "vector store health check failed", "error", err)
		return false
	}
	if !exists {
		logger.WarnContext(ctx, "vector store collection does not exist", "collection", h.collectionName)
		return false
	}
	return true
}
