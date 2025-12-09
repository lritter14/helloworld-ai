package storage

//go:generate go run go.uber.org/mock/mockgen@latest -destination=mocks/mock_vault_store.go -package=mocks helloworld-ai/internal/storage VaultStore

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// VaultStore defines the interface for vault storage operations.
type VaultStore interface {
	// GetOrCreateByName gets an existing vault by name, or creates it if it doesn't exist.
	GetOrCreateByName(ctx context.Context, name, rootPath string) (VaultRecord, error)
	// ListAll returns all vaults ordered by name.
	ListAll(ctx context.Context) ([]VaultRecord, error)
}

// VaultRepo provides methods for vault operations.
// It implements the VaultStore interface.
type VaultRepo struct {
	db *sql.DB
}

// NewVaultRepo creates a new VaultRepo.
func NewVaultRepo(db *sql.DB) *VaultRepo {
	return &VaultRepo{db: db}
}

// GetOrCreateByName gets an existing vault by name, or creates it if it doesn't exist.
func (r *VaultRepo) GetOrCreateByName(ctx context.Context, name, rootPath string) (VaultRecord, error) {
	// Try to get existing vault
	var vault VaultRecord
	err := r.db.QueryRowContext(ctx,
		"SELECT id, name, root_path, created_at FROM vaults WHERE name = ?",
		name,
	).Scan(&vault.ID, &vault.Name, &vault.RootPath, &vault.CreatedAt)

	if err == nil {
		// Found existing vault
		return vault, nil
	}

	if err != sql.ErrNoRows {
		// Unexpected error
		return VaultRecord{}, fmt.Errorf("failed to query vault by name: %w", err)
	}

	// Vault doesn't exist, create it
	result, err := r.db.ExecContext(ctx,
		"INSERT INTO vaults (name, root_path) VALUES (?, ?)",
		name, rootPath,
	)
	if err != nil {
		return VaultRecord{}, fmt.Errorf("failed to insert vault: %w", err)
	}

	// Get the inserted ID
	id, err := result.LastInsertId()
	if err != nil {
		return VaultRecord{}, fmt.Errorf("failed to get last insert id: %w", err)
	}

	// Get the created vault with timestamp
	err = r.db.QueryRowContext(ctx,
		"SELECT id, name, root_path, created_at FROM vaults WHERE id = ?",
		id,
	).Scan(&vault.ID, &vault.Name, &vault.RootPath, &vault.CreatedAt)
	if err != nil {
		return VaultRecord{}, fmt.Errorf("failed to query created vault: %w", err)
	}

	return vault, nil
}

// ListAll returns all vaults ordered by name.
func (r *VaultRepo) ListAll(ctx context.Context) ([]VaultRecord, error) {
	rows, err := r.db.QueryContext(ctx,
		"SELECT id, name, root_path, created_at FROM vaults ORDER BY name",
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query vaults: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var vaults []VaultRecord
	for rows.Next() {
		var vault VaultRecord
		var createdAtStr string
		if err := rows.Scan(&vault.ID, &vault.Name, &vault.RootPath, &createdAtStr); err != nil {
			return nil, fmt.Errorf("failed to scan vault: %w", err)
		}

		// Parse created_at DATETIME string
		vault.CreatedAt, err = time.Parse("2006-01-02 15:04:05", createdAtStr)
		if err != nil {
			// Try alternative format (SQLite might use different format)
			vault.CreatedAt, err = time.Parse(time.RFC3339, createdAtStr)
			if err != nil {
				return nil, fmt.Errorf("failed to parse created_at timestamp: %w", err)
			}
		}

		vaults = append(vaults, vault)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("row iteration error: %w", err)
	}

	return vaults, nil
}
