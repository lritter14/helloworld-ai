package storage

import "time"

// Vault represents a vault (personal or work) in the database.
type Vault struct {
	ID        int
	Name      string
	RootPath  string
	CreatedAt time.Time
}

// Note represents a markdown note file in the database.
type Note struct {
	ID        string    // UUID
	VaultID   int       // Foreign key to vaults.id
	RelPath   string   // Relative path from vault root
	Folder    string   // Folder path (path components except filename)
	Title     string   // Extracted title from markdown
	UpdatedAt time.Time
	Hash      string   // SHA256 hex string of file content
}

// Chunk represents a chunk of text from a note, indexed for vector search.
type Chunk struct {
	ID          string // UUID (same as Qdrant point ID)
	NoteID      string // UUID (foreign key to notes.id)
	ChunkIndex  int    // Index within note (starts at 0)
	HeadingPath string // Format: "# Heading1 > ## Heading2"
	Text        string // Chunk text content
}

