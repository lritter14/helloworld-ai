package storage

import (
	"database/sql"
	"time"

	"github.com/google/uuid"
)

// NoteRepo provides methods for note operations.
type NoteRepo struct {
	db *sql.DB
}

// NewNoteRepo creates a new NoteRepo.
func NewNoteRepo(db *sql.DB) *NoteRepo {
	return &NoteRepo{db: db}
}

// GetByVaultAndPath gets a note by vault ID and relative path.
// Returns nil and sql.ErrNoRows if not found.
func (r *NoteRepo) GetByVaultAndPath(vaultID int, relPath string) (*Note, error) {
	var note Note
	var updatedAtStr string

	err := r.db.QueryRow(
		"SELECT id, vault_id, rel_path, folder, title, updated_at, hash FROM notes WHERE vault_id = ? AND rel_path = ?",
		vaultID, relPath,
	).Scan(&note.ID, &note.VaultID, &note.RelPath, &note.Folder, &note.Title, &updatedAtStr, &note.Hash)

	if err == sql.ErrNoRows {
		return nil, sql.ErrNoRows
	}
	if err != nil {
		return nil, err
	}

	// Parse updated_at DATETIME string
	note.UpdatedAt, err = time.Parse("2006-01-02 15:04:05", updatedAtStr)
	if err != nil {
		// Try alternative format (SQLite might use different format)
		note.UpdatedAt, err = time.Parse(time.RFC3339, updatedAtStr)
		if err != nil {
			return nil, err
		}
	}

	return &note, nil
}

// Upsert inserts a new note or updates an existing one.
// If the note doesn't exist (by vault_id and rel_path), generates a new UUID.
// If it exists, updates title, updated_at, and hash while preserving the ID.
func (r *NoteRepo) Upsert(note *Note) error {
	// Check if note exists to determine if we need to generate UUID
	existing, err := r.GetByVaultAndPath(note.VaultID, note.RelPath)
	if err != nil && err != sql.ErrNoRows {
		return err
	}

	// Generate UUID for new notes only
	if existing == nil && note.ID == "" {
		note.ID = uuid.New().String()
	} else if existing != nil {
		// Preserve existing ID
		note.ID = existing.ID
	}

	// Use SQLite INSERT ... ON CONFLICT syntax for upsert
	_, err = r.db.Exec(
		`INSERT INTO notes (id, vault_id, rel_path, folder, title, updated_at, hash) 
		 VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP, ?)
		 ON CONFLICT (vault_id, rel_path) DO UPDATE SET 
		 title = excluded.title, updated_at = CURRENT_TIMESTAMP, hash = excluded.hash`,
		note.ID, note.VaultID, note.RelPath, note.Folder, note.Title, note.Hash,
	)
	return err
}

