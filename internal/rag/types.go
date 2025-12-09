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
}
