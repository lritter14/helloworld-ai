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
	// Debug contains debug information when debug mode is enabled.
	Debug *DebugInfo `json:"debug,omitempty"`
}

// DebugInfo contains detailed retrieval information for debugging and evaluation.
type DebugInfo struct {
	// RetrievedChunks contains all retrieved chunks with scores and ranks.
	RetrievedChunks []RetrievedChunk `json:"retrieved_chunks"`
	// FolderSelection contains folder selection information.
	FolderSelection *FolderSelection `json:"folder_selection,omitempty"`
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
