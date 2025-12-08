package storage

import "database/sql"

// ChunkRepo provides methods for chunk operations.
type ChunkRepo struct {
	db *sql.DB
}

// NewChunkRepo creates a new ChunkRepo.
func NewChunkRepo(db *sql.DB) *ChunkRepo {
	return &ChunkRepo{db: db}
}

// Insert inserts a single chunk into the database.
// The chunk.ID must be set (UUID) before calling this method.
func (r *ChunkRepo) Insert(chunk *Chunk) error {
	_, err := r.db.Exec(
		"INSERT INTO chunks (id, note_id, chunk_index, heading_path, text) VALUES (?, ?, ?, ?, ?)",
		chunk.ID, chunk.NoteID, chunk.ChunkIndex, chunk.HeadingPath, chunk.Text,
	)
	return err
}

// DeleteByNote deletes all chunks for a given note ID.
// Used when re-indexing a note to remove old chunks before inserting new ones.
func (r *ChunkRepo) DeleteByNote(noteID string) error {
	_, err := r.db.Exec("DELETE FROM chunks WHERE note_id = ?", noteID)
	return err
}

// ListIDsByNote returns all chunk IDs for a given note, ordered by chunk_index.
// Returns an empty slice if no chunks exist (not an error).
// Used to get Qdrant point IDs for deletion before re-indexing.
func (r *ChunkRepo) ListIDsByNote(noteID string) ([]string, error) {
	rows, err := r.db.Query(
		"SELECT id FROM chunks WHERE note_id = ? ORDER BY chunk_index",
		noteID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return ids, nil
}

