package storage

import (
	"database/sql"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// New opens a SQLite database connection at the given path.
// It enables foreign keys and sets connection pool settings.
func New(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, err
	}

	// Enable foreign keys (disabled by default in SQLite)
	if _, err := db.Exec("PRAGMA foreign_keys = ON;"); err != nil {
		_ = db.Close()
		return nil, err
	}

	// Set connection pool settings
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	// Verify connection
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, err
	}

	return db, nil
}

// Migrate runs database migrations to create the required tables.
// It is idempotent and can be run multiple times safely.
func Migrate(db *sql.DB) error {
	schema := []string{
		`CREATE TABLE IF NOT EXISTS vaults (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL UNIQUE,
			root_path TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS notes (
			id TEXT PRIMARY KEY,
			vault_id INTEGER NOT NULL,
			rel_path TEXT NOT NULL,
			folder TEXT NOT NULL,
			title TEXT,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			hash TEXT NOT NULL,
			FOREIGN KEY (vault_id) REFERENCES vaults(id),
			UNIQUE (vault_id, rel_path)
		);`,
		`CREATE TABLE IF NOT EXISTS chunks (
			id TEXT PRIMARY KEY,
			note_id TEXT NOT NULL,
			chunk_index INTEGER NOT NULL,
			heading_path TEXT,
			text TEXT NOT NULL,
			FOREIGN KEY (note_id) REFERENCES notes(id) ON DELETE CASCADE
		);`,
	}

	for _, stmt := range schema {
		if _, err := db.Exec(stmt); err != nil {
			return err
		}
	}

	return nil
}
