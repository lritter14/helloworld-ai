package storage

import (
	"database/sql"
	"time"
)

// VaultRepo provides methods for vault operations.
type VaultRepo struct {
	db *sql.DB
}

// NewVaultRepo creates a new VaultRepo.
func NewVaultRepo(db *sql.DB) *VaultRepo {
	return &VaultRepo{db: db}
}

// GetOrCreateByName gets an existing vault by name, or creates it if it doesn't exist.
func (r *VaultRepo) GetOrCreateByName(name, rootPath string) (Vault, error) {
	// Try to get existing vault
	var vault Vault
	err := r.db.QueryRow(
		"SELECT id, name, root_path, created_at FROM vaults WHERE name = ?",
		name,
	).Scan(&vault.ID, &vault.Name, &vault.RootPath, &vault.CreatedAt)

	if err == nil {
		// Found existing vault
		return vault, nil
	}

	if err != sql.ErrNoRows {
		// Unexpected error
		return Vault{}, err
	}

	// Vault doesn't exist, create it
	result, err := r.db.Exec(
		"INSERT INTO vaults (name, root_path) VALUES (?, ?)",
		name, rootPath,
	)
	if err != nil {
		return Vault{}, err
	}

	// Get the inserted ID
	id, err := result.LastInsertId()
	if err != nil {
		return Vault{}, err
	}

	// Get the created vault with timestamp
	err = r.db.QueryRow(
		"SELECT id, name, root_path, created_at FROM vaults WHERE id = ?",
		id,
	).Scan(&vault.ID, &vault.Name, &vault.RootPath, &vault.CreatedAt)
	if err != nil {
		return Vault{}, err
	}

	return vault, nil
}

// ListAll returns all vaults ordered by name.
func (r *VaultRepo) ListAll() ([]Vault, error) {
	rows, err := r.db.Query(
		"SELECT id, name, root_path, created_at FROM vaults ORDER BY name",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var vaults []Vault
	for rows.Next() {
		var vault Vault
		var createdAtStr string
		if err := rows.Scan(&vault.ID, &vault.Name, &vault.RootPath, &createdAtStr); err != nil {
			return nil, err
		}

		// Parse created_at DATETIME string
		vault.CreatedAt, err = time.Parse("2006-01-02 15:04:05", createdAtStr)
		if err != nil {
			// Try alternative format (SQLite might use different format)
			vault.CreatedAt, err = time.Parse(time.RFC3339, createdAtStr)
			if err != nil {
				return nil, err
			}
		}

		vaults = append(vaults, vault)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return vaults, nil
}

