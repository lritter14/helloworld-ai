package storage

//go:generate go run go.uber.org/mock/mockgen@latest -destination=mocks/mock_chunk_store.go -package=mocks helloworld-ai/internal/storage ChunkStore

import (
	"context"
	"database/sql"
	"fmt"
)

// ChunkStore defines the interface for chunk storage operations.
type ChunkStore interface {
	// Insert inserts a single chunk into the database.
	// The chunk.ID must be set (UUID) before calling this method.
	Insert(ctx context.Context, chunk *ChunkRecord) error
	// DeleteByNote deletes all chunks for a given note ID.
	DeleteByNote(ctx context.Context, noteID string) error
	// ListIDsByNote returns all chunk IDs for a given note, ordered by chunk_index.
	ListIDsByNote(ctx context.Context, noteID string) ([]string, error)
	// GetByID gets a chunk by its ID. Returns ErrNotFound if not found.
	GetByID(ctx context.Context, id string) (*ChunkRecord, error)
}

// ChunkRepo provides methods for chunk operations.
// It implements the ChunkStore interface.
type ChunkRepo struct {
	db *sql.DB
}

// NewChunkRepo creates a new ChunkRepo.
func NewChunkRepo(db *sql.DB) *ChunkRepo {
	return &ChunkRepo{db: db}
}

// Insert inserts a single chunk into the database.
// The chunk.ID must be set (UUID) before calling this method.
func (r *ChunkRepo) Insert(ctx context.Context, chunk *ChunkRecord) error {
	_, err := r.db.ExecContext(ctx,
		"INSERT INTO chunks (id, note_id, chunk_index, heading_path, text) VALUES (?, ?, ?, ?, ?)",
		chunk.ID, chunk.NoteID, chunk.ChunkIndex, chunk.HeadingPath, chunk.Text,
	)
	if err != nil {
		return fmt.Errorf("failed to insert chunk: %w", err)
	}
	return nil
}

// DeleteByNote deletes all chunks for a given note ID.
// Used when re-indexing a note to remove old chunks before inserting new ones.
func (r *ChunkRepo) DeleteByNote(ctx context.Context, noteID string) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM chunks WHERE note_id = ?", noteID)
	if err != nil {
		return fmt.Errorf("failed to delete chunks by note: %w", err)
	}
	return nil
}

// ListIDsByNote returns all chunk IDs for a given note, ordered by chunk_index.
// Returns an empty slice if no chunks exist (not an error).
// Used to get Qdrant point IDs for deletion before re-indexing.
func (r *ChunkRepo) ListIDsByNote(ctx context.Context, noteID string) ([]string, error) {
	rows, err := r.db.QueryContext(ctx,
		"SELECT id FROM chunks WHERE note_id = ? ORDER BY chunk_index",
		noteID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query chunk IDs: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("failed to scan chunk ID: %w", err)
		}
		ids = append(ids, id)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("row iteration error: %w", err)
	}

	return ids, nil
}

// GetByID gets a chunk by its ID. Returns ErrNotFound if not found.
func (r *ChunkRepo) GetByID(ctx context.Context, id string) (*ChunkRecord, error) {
	var chunk ChunkRecord
	err := r.db.QueryRowContext(ctx,
		"SELECT id, note_id, chunk_index, heading_path, text FROM chunks WHERE id = ?",
		id,
	).Scan(&chunk.ID, &chunk.NoteID, &chunk.ChunkIndex, &chunk.HeadingPath, &chunk.Text)

	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query chunk: %w", err)
	}

	return &chunk, nil
}
