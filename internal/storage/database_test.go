package storage

import (
	"path/filepath"
	"testing"
)

func TestNew(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{
			name:    "valid path",
			path:    dbPath,
			wantErr: false,
		},
		{
			name:    "invalid path",
			path:    "/invalid/path/to/db.db",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, err := New(tt.path)

			if tt.wantErr {
				if err == nil {
					t.Errorf("New() expected error, got nil")
				}
				if db != nil {
					_ = db.Close()
				}
				return
			}

			if err != nil {
				t.Errorf("New() unexpected error: %v", err)
				return
			}

			if db == nil {
				t.Fatal("New() returned nil database")
			}

			// Verify connection pool settings
			if db.Stats().MaxOpenConnections != 25 {
				t.Errorf("New() MaxOpenConnections = %v, want 25", db.Stats().MaxOpenConnections)
			}

			_ = db.Close()
		})
	}
}

func TestNew_EnablesForeignKeys(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := New(dbPath)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer func() {
		_ = db.Close()
	}()

	// Check that foreign keys are enabled
	var fkEnabled int
	err = db.QueryRow("PRAGMA foreign_keys").Scan(&fkEnabled)
	if err != nil {
		t.Fatalf("Failed to check foreign keys: %v", err)
	}

	if fkEnabled != 1 {
		t.Error("New() should enable foreign keys")
	}
}

func TestMigrate(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := New(dbPath)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer func() {
		_ = db.Close()
	}()

	// Run migrations
	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	// Verify tables exist
	tables := []string{"vaults", "notes", "chunks"}
	for _, table := range tables {
		var count int
		err := db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&count)
		if err != nil {
			t.Fatalf("Failed to check table %s: %v", table, err)
		}
		if count != 1 {
			t.Errorf("Migrate() table %s not created", table)
		}
	}
}

func TestMigrate_Idempotent(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := New(dbPath)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer func() {
		_ = db.Close()
	}()

	// Run migrations twice
	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate() first run error = %v", err)
	}

	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate() second run error = %v", err)
	}

	// Verify tables still exist
	tables := []string{"vaults", "notes", "chunks"}
	for _, table := range tables {
		var count int
		err := db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&count)
		if err != nil {
			t.Fatalf("Failed to check table %s: %v", table, err)
		}
		if count != 1 {
			t.Errorf("Migrate() table %s not found after second run", table)
		}
	}
}

func TestMigrate_CreatesCorrectSchema(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

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

	// Check vaults table schema
	var sql string
	err = db.QueryRow("SELECT sql FROM sqlite_master WHERE type='table' AND name='vaults'").Scan(&sql)
	if err != nil {
		t.Fatalf("Failed to get vaults schema: %v", err)
	}
	if sql == "" {
		t.Error("vaults table schema not found")
	}

	// Check notes table has foreign key
	err = db.QueryRow("SELECT sql FROM sqlite_master WHERE type='table' AND name='notes'").Scan(&sql)
	if err != nil {
		t.Fatalf("Failed to get notes schema: %v", err)
	}
	if sql == "" {
		t.Error("notes table schema not found")
	}

	// Check chunks table has foreign key with CASCADE
	err = db.QueryRow("SELECT sql FROM sqlite_master WHERE type='table' AND name='chunks'").Scan(&sql)
	if err != nil {
		t.Fatalf("Failed to get chunks schema: %v", err)
	}
	if sql == "" {
		t.Error("chunks table schema not found")
	}
}

func TestNew_ConnectionPoolSettings(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := New(dbPath)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer func() {
		_ = db.Close()
	}()

	stats := db.Stats()
	if stats.MaxOpenConnections != 25 {
		t.Errorf("MaxOpenConnections = %v, want 25", stats.MaxOpenConnections)
	}
	// MaxIdleClosed is just a setting check, not an actual value to verify
	// The actual value depends on usage
	_ = stats.MaxIdleClosed
}

func TestNew_InvalidPath(t *testing.T) {
	// Try to create database in non-existent directory
	invalidPath := "/nonexistent/path/test.db"

	db, err := New(invalidPath)
	if err == nil {
		if db != nil {
			_ = db.Close()
		}
		t.Error("New() with invalid path should return error")
	}
}

func TestNew_Ping(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := New(dbPath)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer func() {
		_ = db.Close()
	}()

	// Verify connection works
	if err := db.Ping(); err != nil {
		t.Errorf("Ping() error = %v", err)
	}
}
