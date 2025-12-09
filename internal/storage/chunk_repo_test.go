package storage

import (
	"context"
	"testing"
)

func TestNewChunkRepo(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := tmpDir + "/test.db"

	db, err := New(dbPath)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer func() {
		_ = db.Close()
	}()

	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	repo := NewChunkRepo(db)
	if repo == nil {
		t.Fatal("NewChunkRepo() returned nil")
	}
}

func TestChunkRepo_Insert(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := tmpDir + "/test.db"

	db, err := New(dbPath)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer func() {
		_ = db.Close()
	}()

	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	// Create test vault and note
	vaultRepo := NewVaultRepo(db)
	vault, err := vaultRepo.GetOrCreateByName(context.Background(), "test", "/tmp/test")
	if err != nil {
		t.Fatalf("GetOrCreateByName() error = %v", err)
	}

	noteRepo := NewNoteRepo(db)
	note := &NoteRecord{
		VaultID: vault.ID,
		RelPath: "test.md",
		Folder:  "",
		Title:   "Test",
		Hash:    "hash",
	}
	if err := noteRepo.Upsert(context.Background(), note); err != nil {
		t.Fatalf("Upsert() error = %v", err)
	}

	repo := NewChunkRepo(db)

	tests := []struct {
		name    string
		chunk   *ChunkRecord
		wantErr bool
	}{
		{
			name: "valid chunk",
			chunk: &ChunkRecord{
				ID:          "chunk-1",
				NoteID:      note.ID,
				ChunkIndex:  0,
				HeadingPath: "# Heading",
				Text:        "Chunk text",
			},
			wantErr: false,
		},
		{
			name: "chunk with empty text",
			chunk: &ChunkRecord{
				ID:          "chunk-2",
				NoteID:      note.ID,
				ChunkIndex:  1,
				HeadingPath: "",
				Text:        "",
			},
			wantErr: false, // Empty text is allowed
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clean up
			_, _ = db.Exec("DELETE FROM chunks")

			err := repo.Insert(context.Background(), tt.chunk)

			if tt.wantErr {
				if err == nil {
					t.Errorf("Insert() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("Insert() unexpected error: %v", err)
			}
		})
	}
}

func TestChunkRepo_DeleteByNote(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := tmpDir + "/test.db"

	db, err := New(dbPath)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer func() {
		_ = db.Close()
	}()

	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	vaultRepo := NewVaultRepo(db)
	vault, err := vaultRepo.GetOrCreateByName(context.Background(), "test", "/tmp/test")
	if err != nil {
		t.Fatalf("GetOrCreateByName() error = %v", err)
	}

	noteRepo := NewNoteRepo(db)
	note := &NoteRecord{
		VaultID: vault.ID,
		RelPath: "test.md",
		Folder:  "",
		Title:   "Test",
		Hash:    "hash",
	}
	if err := noteRepo.Upsert(context.Background(), note); err != nil {
		t.Fatalf("Upsert() error = %v", err)
	}

	repo := NewChunkRepo(db)

	// Insert test chunks
	chunks := []*ChunkRecord{
		{ID: "chunk-1", NoteID: note.ID, ChunkIndex: 0, HeadingPath: "# H1", Text: "Text 1"},
		{ID: "chunk-2", NoteID: note.ID, ChunkIndex: 1, HeadingPath: "# H2", Text: "Text 2"},
		{ID: "chunk-3", NoteID: note.ID, ChunkIndex: 2, HeadingPath: "# H3", Text: "Text 3"},
	}

	for _, chunk := range chunks {
		if err := repo.Insert(context.Background(), chunk); err != nil {
			t.Fatalf("Insert() error = %v", err)
		}
	}

	// Delete chunks
	err = repo.DeleteByNote(context.Background(), note.ID)
	if err != nil {
		t.Fatalf("DeleteByNote() error = %v", err)
	}

	// Verify chunks are deleted
	ids, err := repo.ListIDsByNote(context.Background(), note.ID)
	if err != nil {
		t.Fatalf("ListIDsByNote() error = %v", err)
	}

	if len(ids) != 0 {
		t.Errorf("DeleteByNote() should delete all chunks, got %d remaining", len(ids))
	}
}

func TestChunkRepo_DeleteByNote_NonExistent(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := tmpDir + "/test.db"

	db, err := New(dbPath)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer func() {
		_ = db.Close()
	}()

	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	repo := NewChunkRepo(db)

	// Delete non-existent note should not error
	err = repo.DeleteByNote(context.Background(), "non-existent-id")
	if err != nil {
		t.Errorf("DeleteByNote() with non-existent note should not error, got: %v", err)
	}
}

func TestChunkRepo_ListIDsByNote(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := tmpDir + "/test.db"

	db, err := New(dbPath)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer func() {
		_ = db.Close()
	}()

	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	vaultRepo := NewVaultRepo(db)
	vault, err := vaultRepo.GetOrCreateByName(context.Background(), "test", "/tmp/test")
	if err != nil {
		t.Fatalf("GetOrCreateByName() error = %v", err)
	}

	noteRepo := NewNoteRepo(db)
	note := &NoteRecord{
		VaultID: vault.ID,
		RelPath: "test.md",
		Folder:  "",
		Title:   "Test",
		Hash:    "hash",
	}
	if err := noteRepo.Upsert(context.Background(), note); err != nil {
		t.Fatalf("Upsert() error = %v", err)
	}

	repo := NewChunkRepo(db)

	tests := []struct {
		name    string
		setup   func()
		noteID  string
		wantIDs []string
		wantErr bool
	}{
		{
			name: "multiple chunks",
			setup: func() {
				chunks := []*ChunkRecord{
					{ID: "chunk-1", NoteID: note.ID, ChunkIndex: 0, HeadingPath: "# H1", Text: "Text 1"},
					{ID: "chunk-2", NoteID: note.ID, ChunkIndex: 2, HeadingPath: "# H2", Text: "Text 2"},
					{ID: "chunk-3", NoteID: note.ID, ChunkIndex: 1, HeadingPath: "# H3", Text: "Text 3"},
				}
				for _, chunk := range chunks {
					_ = repo.Insert(context.Background(), chunk)
				}
			},
			noteID:  note.ID,
			wantIDs: []string{"chunk-1", "chunk-3", "chunk-2"}, // Ordered by chunk_index
			wantErr: false,
		},
		{
			name:    "no chunks",
			setup:   func() {},
			noteID:  note.ID,
			wantIDs: []string{},
			wantErr: false,
		},
		{
			name:    "non-existent note",
			setup:   func() {},
			noteID:  "non-existent",
			wantIDs: []string{},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clean up
			_, _ = db.Exec("DELETE FROM chunks")

			tt.setup()

			ids, err := repo.ListIDsByNote(context.Background(), tt.noteID)

			if tt.wantErr {
				if err == nil {
					t.Errorf("ListIDsByNote() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("ListIDsByNote() unexpected error: %v", err)
				return
			}

			if len(ids) != len(tt.wantIDs) {
				t.Errorf("ListIDsByNote() returned %d IDs, want %d", len(ids), len(tt.wantIDs))
				return
			}

			for i, id := range ids {
				if id != tt.wantIDs[i] {
					t.Errorf("ListIDsByNote() ID[%d] = %v, want %v", i, id, tt.wantIDs[i])
				}
			}
		})
	}
}

func TestChunkRepo_ListIDsByNote_OrderedByIndex(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := tmpDir + "/test.db"

	db, err := New(dbPath)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer func() {
		_ = db.Close()
	}()

	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	vaultRepo := NewVaultRepo(db)
	vault, err := vaultRepo.GetOrCreateByName(context.Background(), "test", "/tmp/test")
	if err != nil {
		t.Fatalf("GetOrCreateByName() error = %v", err)
	}

	noteRepo := NewNoteRepo(db)
	note := &NoteRecord{
		VaultID: vault.ID,
		RelPath: "test.md",
		Folder:  "",
		Title:   "Test",
		Hash:    "hash",
	}
	if err := noteRepo.Upsert(context.Background(), note); err != nil {
		t.Fatalf("Upsert() error = %v", err)
	}

	repo := NewChunkRepo(db)

	// Insert chunks in non-sequential order
	chunks := []*ChunkRecord{
		{ID: "chunk-3", NoteID: note.ID, ChunkIndex: 2, HeadingPath: "# H3", Text: "Text 3"},
		{ID: "chunk-1", NoteID: note.ID, ChunkIndex: 0, HeadingPath: "# H1", Text: "Text 1"},
		{ID: "chunk-2", NoteID: note.ID, ChunkIndex: 1, HeadingPath: "# H2", Text: "Text 2"},
	}

	for _, chunk := range chunks {
		if err := repo.Insert(context.Background(), chunk); err != nil {
			t.Fatalf("Insert() error = %v", err)
		}
	}

	ids, err := repo.ListIDsByNote(context.Background(), note.ID)
	if err != nil {
		t.Fatalf("ListIDsByNote() error = %v", err)
	}

	// Should be ordered by chunk_index
	expected := []string{"chunk-1", "chunk-2", "chunk-3"}
	if len(ids) != len(expected) {
		t.Fatalf("ListIDsByNote() returned %d IDs, want %d", len(ids), len(expected))
	}

	for i, id := range ids {
		if id != expected[i] {
			t.Errorf("ListIDsByNote() ID[%d] = %v, want %v", i, id, expected[i])
		}
	}
}
