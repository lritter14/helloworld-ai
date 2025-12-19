package rag

// AskRequest represents a RAG query request.
type AskRequest struct {
	// Question is the user's question to answer.
	Question string `json:"question"`
	// Vaults specifies which vaults to search. If empty, searches all vaults.
	Vaults []string `json:"vaults,omitempty"`
	// Folders specifies folder filters using prefix matching. If empty, searches all folders.
	Folders []string `json:"folders,omitempty"`
	// K optionally specifies the desired chunk count. Auto-selection overrides this unless explicitly provided.
	K int `json:"k,omitempty"`
	// Detail optionally hints at answer length ("brief", "normal", "detailed").
	Detail string `json:"detail,omitempty"`
	// Debug enables debug mode, returning detailed retrieval information.
	Debug bool `json:"debug,omitempty"`
}

// Reference represents a reference to a chunk that was used in the answer.
type Reference struct {
	// Vault is the vault name (e.g., "personal", "work").
	Vault string `json:"vault"`
	// RelPath is the relative path to the note file.
	RelPath string `json:"rel_path"`
	// HeadingPath is the heading path (e.g., "# Heading1 > ## Heading2").
	HeadingPath string `json:"heading_path"`
	// ChunkIndex is the chunk index within the note.
	ChunkIndex int `json:"chunk_index"`
}

// AskResponse represents the response from a RAG query.
type AskResponse struct {
	// Answer is the generated answer from the LLM.
	Answer string `json:"answer"`
	// References are the chunks that were used to generate the answer.
	References []Reference `json:"references"`
	// Abstained indicates whether the system abstained from answering (explicit abstention flag).
	Abstained bool `json:"abstained,omitempty"`
	// AbstainReason provides the reason for abstention (e.g., "no_relevant_context", "ambiguous_question", "insufficient_information").
	AbstainReason string `json:"abstain_reason,omitempty"`
	// Debug contains debug information when debug mode is enabled.
	Debug *DebugInfo `json:"debug,omitempty"`
}

// DebugInfo contains detailed retrieval information for debugging and evaluation.
type DebugInfo struct {
	// RetrievedChunks contains all retrieved chunks with scores and ranks.
	RetrievedChunks []RetrievedChunk `json:"retrieved_chunks"`
	// FolderSelection contains folder selection information.
	FolderSelection *FolderSelection `json:"folder_selection,omitempty"`
	// Latency contains timing breakdown for each phase of the RAG pipeline.
	Latency *LatencyBreakdown `json:"latency,omitempty"`
	// IndexingCoverage contains indexing coverage statistics.
	IndexingCoverage *IndexingCoverage `json:"indexing_coverage,omitempty"`
}

// LatencyBreakdown contains timing information for each phase of the RAG pipeline.
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

// RetrievedChunk represents a retrieved chunk with scoring information.
type RetrievedChunk struct {
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

// FolderSelection contains information about folder selection.
type FolderSelection struct {
	// SelectedFolders is the list of folders selected for search (in order).
	SelectedFolders []string `json:"selected_folders"`
	// AvailableFolders is the list of all available folders.
	AvailableFolders []string `json:"available_folders,omitempty"`
}

// IndexingCoverage contains indexing coverage statistics.
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
