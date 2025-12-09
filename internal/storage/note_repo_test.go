package storage

import (
	"context"
	"testing"
	"time"
)

func TestNewNoteRepo(t *testing.T) {
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

	repo := NewNoteRepo(db)
	if repo == nil {
		t.Fatal("NewNoteRepo() returned nil")
	}
}

func TestNoteRepo_GetByVaultAndPath(t *testing.T) {
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

	// Create test vault
	vaultRepo := NewVaultRepo(db)
	vault, err := vaultRepo.GetOrCreateByName(context.Background(), "test", "/tmp/test")
	if err != nil {
		t.Fatalf("GetOrCreateByName() error = %v", err)
	}

	repo := NewNoteRepo(db)

	tests := []struct {
		name    string
		setup   func()
		vaultID int
		relPath string
		wantErr bool
		check   func(*NoteRecord) bool
	}{
		{
			name: "existing note",
			setup: func() {
				note := &NoteRecord{
					ID:      "test-id",
					VaultID: vault.ID,
					RelPath: "test.md",
					Folder:  "",
					Title:   "Test Note",
					Hash:    "abc123",
				}
				_ = repo.Upsert(context.Background(), note)
			},
			vaultID: vault.ID,
			relPath: "test.md",
			wantErr: false,
			check: func(note *NoteRecord) bool {
				return note != nil && note.ID == "test-id" && note.Title == "Test Note"
			},
		},
		{
			name:    "non-existent note",
			setup:   func() {},
			vaultID: vault.ID,
			relPath: "nonexistent.md",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clean up
			_, _ = db.Exec("DELETE FROM notes")

			tt.setup()

			note, err := repo.GetByVaultAndPath(context.Background(), tt.vaultID, tt.relPath)

			if tt.wantErr {
				if err == nil {
					t.Errorf("GetByVaultAndPath() expected error, got nil")
				}
				if err != ErrNotFound && err != nil {
					t.Errorf("GetByVaultAndPath() error = %v, want ErrNotFound", err)
				}
				return
			}

			if err != nil {
				t.Errorf("GetByVaultAndPath() unexpected error: %v", err)
				return
			}

			if tt.check != nil && !tt.check(note) {
				t.Error("GetByVaultAndPath() result validation failed")
			}
		})
	}
}

func TestNoteRepo_Upsert(t *testing.T) {
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

	repo := NewNoteRepo(db)

	tests := []struct {
		name    string
		note    *NoteRecord
		wantErr bool
		check   func() bool
	}{
		{
			name: "insert new note",
			note: &NoteRecord{
				VaultID: vault.ID,
				RelPath: "new.md",
				Folder:  "",
				Title:   "New Note",
				Hash:    "hash1",
			},
			wantErr: false,
			check: func() bool {
				note, err := repo.GetByVaultAndPath(context.Background(), vault.ID, "new.md")
				return err == nil && note != nil && note.Title == "New Note" && note.ID != ""
			},
		},
		{
			name: "update existing note",
			note: &NoteRecord{
				VaultID: vault.ID,
				RelPath: "update.md",
				Folder:  "",
				Title:   "Updated Title",
				Hash:    "hash2",
			},
			wantErr: false,
			check: func() bool {
				// Insert first
				note1 := &NoteRecord{
					VaultID: vault.ID,
					RelPath: "update.md",
					Folder:  "",
					Title:   "Original Title",
					Hash:    "hash1",
				}
				_ = repo.Upsert(context.Background(), note1)
				originalID := note1.ID

				// Update
				note2 := &NoteRecord{
					VaultID: vault.ID,
					RelPath: "update.md",
					Folder:  "",
					Title:   "Updated Title",
					Hash:    "hash2",
				}
				_ = repo.Upsert(context.Background(), note2)

				// Check
				note, err := repo.GetByVaultAndPath(context.Background(), vault.ID, "update.md")
				return err == nil && note != nil && note.Title == "Updated Title" && note.ID == originalID
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clean up
			_, _ = db.Exec("DELETE FROM notes")

			err := repo.Upsert(context.Background(), tt.note)

			if tt.wantErr {
				if err == nil {
					t.Errorf("Upsert() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("Upsert() unexpected error: %v", err)
				return
			}

			if tt.check != nil && !tt.check() {
				t.Error("Upsert() result validation failed")
			}
		})
	}
}

func TestNoteRepo_Upsert_GeneratesUUID(t *testing.T) {
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

	repo := NewNoteRepo(db)

	note := &NoteRecord{
		VaultID: vault.ID,
		RelPath: "uuid-test.md",
		Folder:  "",
		Title:   "UUID Test",
		Hash:    "hash",
	}

	err = repo.Upsert(context.Background(), note)
	if err != nil {
		t.Fatalf("Upsert() error = %v", err)
	}

	if note.ID == "" {
		t.Error("Upsert() should generate UUID for new note")
	}

	// Verify UUID format (basic check)
	if len(note.ID) != 36 {
		t.Errorf("Upsert() generated ID length = %d, want 36", len(note.ID))
	}
}

func TestNoteRecord_UpdatedAt(t *testing.T) {
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

	repo := NewNoteRepo(db)

	note := &NoteRecord{
		VaultID: vault.ID,
		RelPath: "time-test.md",
		Folder:  "",
		Title:   "Time Test",
		Hash:    "hash",
	}

	err = repo.Upsert(context.Background(), note)
	if err != nil {
		t.Fatalf("Upsert() error = %v", err)
	}

	retrieved, err := repo.GetByVaultAndPath(context.Background(), vault.ID, "time-test.md")
	if err != nil {
		t.Fatalf("GetByVaultAndPath() error = %v", err)
	}

	// Check that UpdatedAt is set
	if retrieved.UpdatedAt.IsZero() {
		t.Error("UpdatedAt should be set")
	}

	// Check that UpdatedAt is recent (within last minute)
	if time.Since(retrieved.UpdatedAt) > time.Minute {
		t.Error("UpdatedAt should be recent")
	}
}
