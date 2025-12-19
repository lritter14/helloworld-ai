package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"helloworld-ai/internal/contextutil"
	"helloworld-ai/internal/indexer"
	"helloworld-ai/internal/rag"
	"helloworld-ai/internal/storage"
)

// AskHandler handles HTTP requests for RAG queries.
type AskHandler struct {
	ragEngine        rag.Engine
	vaultRepo        storage.VaultStore
	indexerPipeline  *indexer.Pipeline
	embeddingModelName string
}

// NewAskHandler creates a new AskHandler.
func NewAskHandler(ragEngine rag.Engine, vaultRepo storage.VaultStore, indexerPipeline *indexer.Pipeline, embeddingModelName string) *AskHandler {
	return &AskHandler{
		ragEngine:        ragEngine,
		vaultRepo:        vaultRepo,
		indexerPipeline:  indexerPipeline,
		embeddingModelName: embeddingModelName,
	}
}

// AskRequest represents the HTTP request payload for RAG queries.
// This mirrors the rag.AskRequest but is defined here for HTTP layer separation.
//
// swagger:model AskRequest
type AskRequest struct {
	Question string   `json:"question"`
	Vaults   []string `json:"vaults,omitempty"`
	Folders  []string `json:"folders,omitempty"`
	K        int      `json:"k,omitempty"`
	Detail   string   `json:"detail,omitempty"`
}

// AskResponse represents the HTTP response payload for RAG queries.
// This mirrors the rag.AskResponse but is defined here for HTTP layer separation.
//
// swagger:model AskResponse
type AskResponse struct {
	// The generated answer from the RAG system
	Answer string `json:"answer"`

	// List of references to source chunks used in the answer
	References []ReferenceResponse `json:"references"`

	// Abstained indicates whether the system abstained from answering (explicit abstention flag).
	Abstained bool `json:"abstained,omitempty"`

	// AbstainReason provides the reason for abstention (e.g., "no_relevant_context", "ambiguous_question", "insufficient_information").
	AbstainReason string `json:"abstain_reason,omitempty"`

	// Debug contains debug information when debug mode is enabled (via ?debug=true query parameter).
	Debug *DebugInfo `json:"debug,omitempty"`
}

// DebugInfo contains debug information when debug mode is enabled.
//
// swagger:model DebugInfo
type DebugInfo struct {
	// RetrievedChunks contains all retrieved chunks with scores and ranks.
	RetrievedChunks []DebugRetrievedChunk `json:"retrieved_chunks"`
	// FolderSelection contains folder selection information.
	FolderSelection *DebugFolderSelection `json:"folder_selection,omitempty"`
	// Latency contains timing breakdown for each phase of the RAG pipeline.
	Latency *LatencyBreakdown `json:"latency,omitempty"`
	// IndexingCoverage contains indexing coverage statistics.
	IndexingCoverage *IndexingCoverage `json:"indexing_coverage,omitempty"`
}

// ReferenceResponse represents a reference in the HTTP response.
//
// swagger:model ReferenceResponse
type ReferenceResponse struct {
	// Name of the vault containing the source
	Vault string `json:"vault"`

	// Relative path to the markdown file within the vault
	RelPath string `json:"rel_path"`

	// Heading path within the document (e.g., "H1 > H2 > H3")
	HeadingPath string `json:"heading_path"`

	// Index of the chunk within the document
	ChunkIndex int `json:"chunk_index"`
}

// LatencyBreakdown contains timing information for each phase of the RAG pipeline.
//
// swagger:model LatencyBreakdown
type LatencyBreakdown struct {
	// FolderSelectionMs is the time spent in folder selection (milliseconds).
	FolderSelectionMs int64 `json:"folder_selection_ms"`
	// RetrievalMs is the time spent in vector search and reranking (milliseconds).
	RetrievalMs int64 `json:"retrieval_ms"`
	// GenerationMs is the time spent in LLM generation (milliseconds).
	GenerationMs int64 `json:"generation_ms"`
	// JudgeMs is the time spent in answer judging (milliseconds). Always 0 in Go API (judging happens in Python).
	JudgeMs int64 `json:"judge_ms"`
	// TotalMs is the total time for the entire RAG query (milliseconds).
	TotalMs int64 `json:"total_ms"`
}

// DebugRetrievedChunk represents a retrieved chunk with scoring information.
//
// swagger:model DebugRetrievedChunk
type DebugRetrievedChunk struct {
	// ChunkID is the stable chunk identifier.
	ChunkID string `json:"chunk_id"`
	// RelPath is the relative path to the note file.
	RelPath string `json:"rel_path"`
	// HeadingPath is the heading hierarchy path (e.g., "# Overview > ## Details").
	HeadingPath string `json:"heading_path"`
	// ScoreVector is the vector similarity score.
	ScoreVector float64 `json:"score_vector"`
	// ScoreLexical is the lexical/BM25 score (if applicable).
	ScoreLexical float64 `json:"score_lexical,omitempty"`
	// ScoreFinal is the combined final score.
	ScoreFinal float64 `json:"score_final"`
	// Text is the chunk text (full or truncated).
	Text string `json:"text"`
	// Rank is the rank of this chunk in the retrieval results (1-based).
	Rank int `json:"rank"`
}

// DebugFolderSelection contains information about folder selection.
//
// swagger:model DebugFolderSelection
type DebugFolderSelection struct {
	// SelectedFolders is the list of folders selected for search (in order).
	SelectedFolders []string `json:"selected_folders"`
	// AvailableFolders is the list of all available folders.
	AvailableFolders []string `json:"available_folders,omitempty"`
}

// IndexingCoverage contains indexing coverage statistics.
//
// swagger:model IndexingCoverage
type IndexingCoverage struct {
	// DocsProcessed is the total number of documents processed.
	DocsProcessed int `json:"docs_processed"`
	// DocsWith0Chunks is the number of documents that produced 0 chunks.
	DocsWith0Chunks int `json:"docs_with_0_chunks"`
	// ChunksAttempted is the total number of chunks that were attempted to be embedded.
	ChunksAttempted int `json:"chunks_attempted"`
	// ChunksEmbedded is the number of chunks successfully embedded and stored.
	ChunksEmbedded int `json:"chunks_embedded"`
	// ChunksSkipped is the number of chunks skipped (e.g., due to context size limits).
	ChunksSkipped int `json:"chunks_skipped"`
	// ChunksSkippedReasons is a breakdown of why chunks were skipped.
	ChunksSkippedReasons map[string]int `json:"chunks_skipped_reasons,omitempty"`
	// ChunkTokenStats contains statistics about token counts per chunk.
	ChunkTokenStats *ChunkTokenStats `json:"chunk_token_stats,omitempty"`
	// ChunkerVersion is the version of the chunker used.
	ChunkerVersion string `json:"chunker_version,omitempty"`
	// IndexVersion is a hash identifying the index build (chunker + embedding model + params).
	IndexVersion string `json:"index_version,omitempty"`
}

// ChunkTokenStats contains statistics about token counts in chunks.
//
// swagger:model ChunkTokenStats
type ChunkTokenStats struct {
	// Min is the minimum token count across all chunks.
	Min int `json:"min"`
	// Max is the maximum token count across all chunks.
	Max int `json:"max"`
	// Mean is the mean token count across all chunks.
	Mean float64 `json:"mean"`
	// P95 is the 95th percentile token count.
	P95 int `json:"p95"`
}

// ErrorResponse represents an error response.
//
// swagger:model ErrorResponse
type ErrorResponse struct {
	Error string `json:"error"`
}

// ServeHTTP handles HTTP requests for RAG queries.
//
// Ask a question to the RAG system and get an answer based on indexed markdown notes.
// The system will search for relevant chunks across the specified vaults and folders,
// then generate an answer using the retrieved context.
//
// swagger:route POST /api/v1/ask askQuestion
//
// # Ask a question using RAG
//
// Queries the RAG system with a question and optional filters for vaults and folders.
// Returns an answer generated from relevant indexed content along with source references.
//
// Use the `debug=true` query parameter to include detailed retrieval information
// (retrieved chunks with scores, folder selection) in the response.
//
// ---
// consumes:
// - application/json
// produces:
// - application/json
// parameters:
//   - in: body
//     name: body
//     required: true
//     schema:
//     "$ref": "#/definitions/AskRequest"
//   - in: query
//     name: debug
//     type: boolean
//     description: Enable debug mode to include detailed retrieval information
//     required: false
//
// responses:
//
//	'200':
//	  description: Successful response with answer and references
//	  schema:
//	    "$ref": "#/definitions/AskResponse"
//	'400':
//	  description: Bad request (invalid question or vault name)
//	  schema:
//	    "$ref": "#/definitions/ErrorResponse"
//	'502':
//	  description: External service error (LLM or embedding service unavailable)
//	  schema:
//	    "$ref": "#/definitions/ErrorResponse"
//	'503':
//	  description: Vector store unavailable
//	  schema:
//	    "$ref": "#/definitions/ErrorResponse"
//	'500':
//	  description: Internal server error
//	  schema:
//	    "$ref": "#/definitions/ErrorResponse"
func (h *AskHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := contextutil.LoggerFromContext(ctx)

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

	// Enforce bounds for user-provided K (legacy clients). Zero means "auto".
	if req.K < 0 {
		req.K = 0
	}
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

	// Parse debug query parameter
	debug := false
	if debugParam := r.URL.Query().Get("debug"); debugParam != "" {
		debug = strings.ToLower(debugParam) == "true" || debugParam == "1"
	}

	// Convert HTTP request to RAG request
	detail := strings.ToLower(strings.TrimSpace(req.Detail))
	switch detail {
	case "brief", "normal", "detailed":
	default:
		detail = ""
	}

	ragReq := rag.AskRequest{
		Question: req.Question,
		Vaults:   req.Vaults,
		Folders:  req.Folders,
		K:        req.K,
		Detail:   detail,
		Debug:    debug,
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
		Answer:        ragResp.Answer,
		References:    references,
		Abstained:     ragResp.Abstained,
		AbstainReason: ragResp.AbstainReason,
	}

	// Include debug information if present
	if ragResp.Debug != nil {
		debugChunks := make([]DebugRetrievedChunk, 0, len(ragResp.Debug.RetrievedChunks))
		for _, chunk := range ragResp.Debug.RetrievedChunks {
			debugChunks = append(debugChunks, DebugRetrievedChunk{
				ChunkID:      chunk.ChunkID,
				RelPath:      chunk.RelPath,
				HeadingPath:  chunk.HeadingPath,
				ScoreVector:  chunk.ScoreVector,
				ScoreLexical: chunk.ScoreLexical,
				ScoreFinal:   chunk.ScoreFinal,
				Text:         chunk.Text,
				Rank:         chunk.Rank,
			})
		}

		var folderSelection *DebugFolderSelection
		if ragResp.Debug.FolderSelection != nil {
			folderSelection = &DebugFolderSelection{
				SelectedFolders:  ragResp.Debug.FolderSelection.SelectedFolders,
				AvailableFolders: ragResp.Debug.FolderSelection.AvailableFolders,
			}
		}

		var latency *LatencyBreakdown
		if ragResp.Debug.Latency != nil {
			latency = &LatencyBreakdown{
				FolderSelectionMs: ragResp.Debug.Latency.FolderSelectionMs,
				RetrievalMs:       ragResp.Debug.Latency.RetrievalMs,
				GenerationMs:      ragResp.Debug.Latency.GenerationMs,
				JudgeMs:           ragResp.Debug.Latency.JudgeMs,
				TotalMs:           ragResp.Debug.Latency.TotalMs,
			}
		}

		// Fetch indexing coverage stats if debug mode is enabled
		var indexingCoverage *IndexingCoverage
		if h.indexerPipeline != nil && h.embeddingModelName != "" {
			stats, err := h.indexerPipeline.GetIndexingCoverageStats(ctx, h.embeddingModelName)
			if err != nil {
				logger.WarnContext(ctx, "failed to get indexing coverage stats", "error", err)
			} else if stats != nil {
				// Convert indexer.IndexingCoverageStats to handlers.IndexingCoverage
				var tokenStats *ChunkTokenStats
				if stats.ChunkTokenStats.Min > 0 || stats.ChunkTokenStats.Max > 0 {
					tokenStats = &ChunkTokenStats{
						Min:  stats.ChunkTokenStats.Min,
						Max:  stats.ChunkTokenStats.Max,
						Mean: stats.ChunkTokenStats.Mean,
						P95:  stats.ChunkTokenStats.P95,
					}
				}
				indexingCoverage = &IndexingCoverage{
					DocsProcessed:        stats.DocsProcessed,
					DocsWith0Chunks:      stats.DocsWith0Chunks,
					ChunksAttempted:      stats.ChunksAttempted,
					ChunksEmbedded:       stats.ChunksEmbedded,
					ChunksSkipped:        stats.ChunksSkipped,
					ChunksSkippedReasons: stats.ChunksSkippedReasons,
					ChunkTokenStats:      tokenStats,
					ChunkerVersion:       stats.ChunkerVersion,
					IndexVersion:         stats.IndexVersion,
				}
			}
		}

		resp.Debug = &DebugInfo{
			RetrievedChunks:  debugChunks,
			FolderSelection:  folderSelection,
			Latency:          latency,
			IndexingCoverage: indexingCoverage,
		}
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
	logger := contextutil.LoggerFromContext(ctx)
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
