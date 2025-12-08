package storage

import "time"

// VaultRecord represents a vault (personal or work) in the database.
type VaultRecord struct {
	ID        int       `db:"id"`
	Name      string    `db:"name"`
	RootPath  string    `db:"root_path"`
	CreatedAt time.Time `db:"created_at"`
}

// NoteRecord represents a markdown note file in the database.
type NoteRecord struct {
	ID        string    `db:"id"`       // UUID
	VaultID   int       `db:"vault_id"` // Foreign key to vaults.id
	RelPath   string    `db:"rel_path"` // Relative path from vault root
	Folder    string    `db:"folder"`   // Folder path (path components except filename)
	Title     string    `db:"title"`    // Extracted title from markdown
	UpdatedAt time.Time `db:"updated_at"`
	Hash      string    `db:"hash"` // SHA256 hex string of file content
}

// ChunkRecord represents a chunk of text from a note, indexed for vector search.
type ChunkRecord struct {
	ID          string `db:"id"`           // UUID (same as Qdrant point ID)
	NoteID      string `db:"note_id"`      // UUID (foreign key to notes.id)
	ChunkIndex  int    `db:"chunk_index"`  // Index within note (starts at 0)
	HeadingPath string `db:"heading_path"` // Format: "# Heading1 > ## Heading2"
	Text        string `db:"text"`         // Chunk text content
}

// Legacy type aliases for backward compatibility during migration
// These will be removed once all code is updated
type Vault = VaultRecord
type Note = NoteRecord
type Chunk = ChunkRecord
