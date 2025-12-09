package vault

import (
	"context"
	"fmt"
	"path/filepath"

	"helloworld-ai/internal/storage"
)

// Manager manages vault configuration and provides vault lookup and path resolution.
type Manager struct {
	vaultRepo storage.VaultStore
	vaults    map[string]storage.VaultRecord // Cache vaults by name
}

// NewManager creates a new vault manager and initializes personal and work vaults.
func NewManager(ctx context.Context, vaultRepo storage.VaultStore, personalPath, workPath string) (*Manager, error) {
	m := &Manager{
		vaultRepo: vaultRepo,
		vaults:    make(map[string]storage.VaultRecord),
	}

	// Initialize personal vault
	personalVault, err := vaultRepo.GetOrCreateByName(ctx, "personal", personalPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create vault personal: %w", err)
	}
	m.vaults["personal"] = personalVault

	// Initialize work vault
	workVault, err := vaultRepo.GetOrCreateByName(ctx, "work", workPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create vault work: %w", err)
	}
	m.vaults["work"] = workVault

	return m, nil
}

// VaultByName returns the vault record for the given vault name.
func (m *Manager) VaultByName(name string) (storage.VaultRecord, error) {
	vault, ok := m.vaults[name]
	if !ok {
		return storage.VaultRecord{}, fmt.Errorf("vault not found: %s", name)
	}
	return vault, nil
}

// AbsPath returns the absolute path for a file given its vault ID and relative path.
func (m *Manager) AbsPath(vaultID int, relPath string) string {
	// Find vault by ID
	for _, vault := range m.vaults {
		if vault.ID == vaultID {
			return filepath.Join(vault.RootPath, relPath)
		}
	}
	// If vault not found, return empty string (should not happen in practice)
	return ""
}

