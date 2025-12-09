package vault

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// ScannedFile represents a markdown file found during vault scanning.
type ScannedFile struct {
	VaultID int    // Vault ID from database
	RelPath string // Relative path from vault root (e.g., "projects/meeting-notes.md")
	Folder  string // Folder path (path components except filename, e.g., "projects")
	AbsPath string // Absolute file path
}

// ScanAll scans all vaults and returns a list of all markdown files found.
func (m *Manager) ScanAll(ctx context.Context) ([]ScannedFile, error) {
	var scannedFiles []ScannedFile

	// Iterate over cached vaults
	for _, vault := range m.vaults {
		// Check for context cancellation
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		// Walk vault root directory
		err := filepath.Walk(vault.RootPath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				// Log error but continue scanning
				return fmt.Errorf("failed to access path %s: %w", path, err)
			}

			// Skip directories
			if info.IsDir() {
				// Skip .obsidian directory (Obsidian configuration)
				if info.Name() == ".obsidian" {
					return filepath.SkipDir
				}
				return nil
			}

			// Filter for markdown files
			if filepath.Ext(path) != ".md" {
				return nil
			}

			// Compute relative path from vault root
			relPath, err := filepath.Rel(vault.RootPath, path)
			if err != nil {
				return fmt.Errorf("failed to compute relative path for %s: %w", path, err)
			}

			// Normalize relative path (use forward slashes for consistency)
			relPath = filepath.ToSlash(relPath)

			// Compute folder per Section 0.6
			folder := filepath.Dir(relPath)
			if folder == "." || folder == "" {
				// Root-level file
				folder = ""
			} else {
				// Normalize folder path
				folder = filepath.ToSlash(folder)
			}

			// Create ScannedFile
			scannedFile := ScannedFile{
				VaultID: vault.ID,
				RelPath: relPath,
				Folder:  folder,
				AbsPath: path,
			}

			scannedFiles = append(scannedFiles, scannedFile)
			return nil
		})

		if err != nil {
			// Log error but continue with other vaults
			return scannedFiles, fmt.Errorf("failed to scan vault %s: %w", vault.Name, err)
		}
	}

	return scannedFiles, nil
}

