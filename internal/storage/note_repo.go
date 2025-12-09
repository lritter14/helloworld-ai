package storage

//go:generate go run go.uber.org/mock/mockgen@latest -destination=mocks/mock_note_store.go -package=mocks helloworld-ai/internal/storage NoteStore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

var (
	// ErrNotFound is returned when a record is not found.
	ErrNotFound = errors.New("record not found")
)

// NoteStore defines the interface for note storage operations.
type NoteStore interface {
	// GetByVaultAndPath gets a note by vault ID and relative path.
	// Returns nil and ErrNotFound if not found.
	GetByVaultAndPath(ctx context.Context, vaultID int, relPath string) (*NoteRecord, error)
	// Upsert inserts a new note or updates an existing one.
	Upsert(ctx context.Context, note *NoteRecord) error
}

// NoteRepo provides methods for note operations.
// It implements the NoteStore interface.
type NoteRepo struct {
	db *sql.DB
}

// NewNoteRepo creates a new NoteRepo.
func NewNoteRepo(db *sql.DB) *NoteRepo {
	return &NoteRepo{db: db}
}

// GetByVaultAndPath gets a note by vault ID and relative path.
// Returns nil and ErrNotFound if not found.
func (r *NoteRepo) GetByVaultAndPath(ctx context.Context, vaultID int, relPath string) (*NoteRecord, error) {
	var note NoteRecord
	var updatedAtStr string

	err := r.db.QueryRowContext(ctx,
		"SELECT id, vault_id, rel_path, folder, title, updated_at, hash FROM notes WHERE vault_id = ? AND rel_path = ?",
		vaultID, relPath,
	).Scan(&note.ID, &note.VaultID, &note.RelPath, &note.Folder, &note.Title, &updatedAtStr, &note.Hash)

	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query note: %w", err)
	}

	// Parse updated_at DATETIME string
	note.UpdatedAt, err = time.Parse("2006-01-02 15:04:05", updatedAtStr)
	if err != nil {
		// Try alternative format (SQLite might use different format)
		note.UpdatedAt, err = time.Parse(time.RFC3339, updatedAtStr)
		if err != nil {
			return nil, fmt.Errorf("failed to parse updated_at timestamp: %w", err)
		}
	}

	return &note, nil
}

// Upsert inserts a new note or updates an existing one.
// If the note doesn't exist (by vault_id and rel_path), generates a new UUID.
// If it exists, updates title, updated_at, and hash while preserving the ID.
func (r *NoteRepo) Upsert(ctx context.Context, note *NoteRecord) error {
	// Check if note exists to determine if we need to generate UUID
	existing, err := r.GetByVaultAndPath(ctx, note.VaultID, note.RelPath)
	if err != nil && err != ErrNotFound {
		return fmt.Errorf("failed to check existing note: %w", err)
	}

	// Generate UUID for new notes only
	if existing == nil && note.ID == "" {
		note.ID = uuid.New().String()
	} else if existing != nil {
		// Preserve existing ID
		note.ID = existing.ID
	}

	// Use SQLite INSERT ... ON CONFLICT syntax for upsert
	_, err = r.db.ExecContext(ctx,
		`INSERT INTO notes (id, vault_id, rel_path, folder, title, updated_at, hash) 
		 VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP, ?)
		 ON CONFLICT (vault_id, rel_path) DO UPDATE SET 
		 title = excluded.title, updated_at = CURRENT_TIMESTAMP, hash = excluded.hash`,
		note.ID, note.VaultID, note.RelPath, note.Folder, note.Title, note.Hash,
	)
	if err != nil {
		return fmt.Errorf("failed to upsert note: %w", err)
	}

	return nil
}
