package vault

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"helloworld-ai/internal/storage"
	"helloworld-ai/internal/storage/mocks"
	"go.uber.org/mock/gomock"
)

func TestManager_ScanAll(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test vault structure
	personalDir := filepath.Join(tmpDir, "personal")
	workDir := filepath.Join(tmpDir, "work")

	if err := os.MkdirAll(personalDir, 0755); err != nil {
		t.Fatalf("Failed to create personal dir: %v", err)
	}
	if err := os.MkdirAll(workDir, 0755); err != nil {
		t.Fatalf("Failed to create work dir: %v", err)
	}

	// Create test markdown files
	testFiles := []struct {
		dir  string
		path string
	}{
		{personalDir, "note1.md"},
		{personalDir, "folder/note2.md"},
		{workDir, "project.md"},
		{workDir, "docs/readme.md"},
	}

	for _, tf := range testFiles {
		fullPath := filepath.Join(tf.dir, tf.path)
		dir := filepath.Dir(fullPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("Failed to create dir: %v", err)
		}
		if err := os.WriteFile(fullPath, []byte("# Test"), 0644); err != nil {
			t.Fatalf("Failed to create file: %v", err)
		}
	}

	// Create .obsidian directory (should be skipped)
	obsidianDir := filepath.Join(personalDir, ".obsidian")
	if err := os.MkdirAll(obsidianDir, 0755); err != nil {
		t.Fatalf("Failed to create .obsidian dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(obsidianDir, "config.json"), []byte("{}"), 0644); err != nil {
		t.Fatalf("Failed to create .obsidian file: %v", err)
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

		mockVaultRepo := mocks.NewMockVaultStore(ctrl)

	personalVault := storage.VaultRecord{ID: 1, Name: "personal", RootPath: personalDir}
	workVault := storage.VaultRecord{ID: 2, Name: "work", RootPath: workDir}

	mockVaultRepo.EXPECT().
		GetOrCreateByName(gomock.Any(), "personal", personalDir).
		Return(personalVault, nil)

	mockVaultRepo.EXPECT().
		GetOrCreateByName(gomock.Any(), "work", workDir).
		Return(workVault, nil)

	manager, err := NewManager(context.Background(), mockVaultRepo, personalDir, workDir)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	files, err := manager.ScanAll(context.Background())
	if err != nil {
		t.Fatalf("ScanAll() error = %v", err)
	}

	// Should find 4 markdown files (not .obsidian files)
	if len(files) != 4 {
		t.Errorf("ScanAll() found %d files, want 4", len(files))
	}

	// Verify files are found
	foundPaths := make(map[string]bool)
	for _, file := range files {
		foundPaths[file.RelPath] = true
	}

	expectedPaths := []string{
		"note1.md",
		"folder/note2.md",
		"project.md",
		"docs/readme.md",
	}

	for _, expected := range expectedPaths {
		if !foundPaths[expected] {
			t.Errorf("ScanAll() did not find expected path: %s", expected)
		}
	}
}

func TestManager_ScanAll_SkipsObsidian(t *testing.T) {
	tmpDir := t.TempDir()
	vaultDir := filepath.Join(tmpDir, "vault")

	if err := os.MkdirAll(vaultDir, 0755); err != nil {
		t.Fatalf("Failed to create vault dir: %v", err)
	}

	// Create .obsidian directory with markdown file
	obsidianDir := filepath.Join(vaultDir, ".obsidian")
	if err := os.MkdirAll(obsidianDir, 0755); err != nil {
		t.Fatalf("Failed to create .obsidian dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(obsidianDir, "note.md"), []byte("# Test"), 0644); err != nil {
		t.Fatalf("Failed to create .obsidian file: %v", err)
	}

	// Create regular markdown file
	if err := os.WriteFile(filepath.Join(vaultDir, "regular.md"), []byte("# Test"), 0644); err != nil {
		t.Fatalf("Failed to create regular file: %v", err)
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

		mockVaultRepo := mocks.NewMockVaultStore(ctrl)

	vault := storage.VaultRecord{ID: 1, Name: "test", RootPath: vaultDir}

	mockVaultRepo.EXPECT().
		GetOrCreateByName(gomock.Any(), "personal", vaultDir).
		Return(vault, nil)

	mockVaultRepo.EXPECT().
		GetOrCreateByName(gomock.Any(), "work", vaultDir).
		Return(vault, nil)

	manager, err := NewManager(context.Background(), mockVaultRepo, vaultDir, vaultDir)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	files, err := manager.ScanAll(context.Background())
	if err != nil {
		t.Fatalf("ScanAll() error = %v", err)
	}

	// Should only find regular.md, not .obsidian/note.md
	if len(files) != 2 { // One for personal, one for work vault
		t.Errorf("ScanAll() found %d files, want 2", len(files))
	}

	for _, file := range files {
		if filepath.Base(file.RelPath) == ".obsidian" || filepath.Dir(file.RelPath) == ".obsidian" {
			t.Errorf("ScanAll() should skip .obsidian directory, found: %s", file.RelPath)
		}
	}
}

func TestManager_ScanAll_OnlyMarkdown(t *testing.T) {
	tmpDir := t.TempDir()
	vaultDir := filepath.Join(tmpDir, "vault")

	if err := os.MkdirAll(vaultDir, 0755); err != nil {
		t.Fatalf("Failed to create vault dir: %v", err)
	}

	// Create various file types
	files := []struct {
		name string
		ext  string
	}{
		{"note.md", ".md"},
		{"document.txt", ".txt"},
		{"image.png", ".png"},
		{"code.go", ".go"},
	}

	for _, f := range files {
		path := filepath.Join(vaultDir, f.name)
		if err := os.WriteFile(path, []byte("content"), 0644); err != nil {
			t.Fatalf("Failed to create file: %v", err)
		}
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

		mockVaultRepo := mocks.NewMockVaultStore(ctrl)

	vault := storage.VaultRecord{ID: 1, Name: "test", RootPath: vaultDir}

	mockVaultRepo.EXPECT().
		GetOrCreateByName(gomock.Any(), "personal", vaultDir).
		Return(vault, nil)

	mockVaultRepo.EXPECT().
		GetOrCreateByName(gomock.Any(), "work", vaultDir).
		Return(vault, nil)

	manager, err := NewManager(context.Background(), mockVaultRepo, vaultDir, vaultDir)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	scannedFiles, err := manager.ScanAll(context.Background())
	if err != nil {
		t.Fatalf("ScanAll() error = %v", err)
	}

	// Should only find .md files
	if len(scannedFiles) != 2 { // One for personal, one for work
		t.Errorf("ScanAll() found %d files, want 2", len(scannedFiles))
	}

	for _, file := range scannedFiles {
		if filepath.Ext(file.RelPath) != ".md" {
			t.Errorf("ScanAll() should only return .md files, found: %s", file.RelPath)
		}
	}
}

func TestManager_ScanAll_ContextCancellation(t *testing.T) {
	tmpDir := t.TempDir()
	vaultDir := filepath.Join(tmpDir, "vault")

	if err := os.MkdirAll(vaultDir, 0755); err != nil {
		t.Fatalf("Failed to create vault dir: %v", err)
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

		mockVaultRepo := mocks.NewMockVaultStore(ctrl)

	vault := storage.VaultRecord{ID: 1, Name: "test", RootPath: vaultDir}

	mockVaultRepo.EXPECT().
		GetOrCreateByName(gomock.Any(), "personal", vaultDir).
		Return(vault, nil)

	mockVaultRepo.EXPECT().
		GetOrCreateByName(gomock.Any(), "work", vaultDir).
		Return(vault, nil)

	manager, err := NewManager(context.Background(), mockVaultRepo, vaultDir, vaultDir)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	// Create cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = manager.ScanAll(ctx)
	if err == nil {
		t.Error("ScanAll() with cancelled context should return error")
	}
	if err != context.Canceled {
		t.Errorf("ScanAll() error = %v, want context.Canceled", err)
	}
}

func TestScannedFile_Fields(t *testing.T) {
	tmpDir := t.TempDir()
	vaultDir := filepath.Join(tmpDir, "vault")
	subDir := filepath.Join(vaultDir, "folder")

	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("Failed to create sub dir: %v", err)
	}

	filePath := filepath.Join(subDir, "note.md")
	if err := os.WriteFile(filePath, []byte("# Test"), 0644); err != nil {
		t.Fatalf("Failed to create file: %v", err)
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

		mockVaultRepo := mocks.NewMockVaultStore(ctrl)

	vault := storage.VaultRecord{ID: 1, Name: "test", RootPath: vaultDir}

	mockVaultRepo.EXPECT().
		GetOrCreateByName(gomock.Any(), "personal", vaultDir).
		Return(vault, nil)

	mockVaultRepo.EXPECT().
		GetOrCreateByName(gomock.Any(), "work", vaultDir).
		Return(vault, nil)

	manager, err := NewManager(context.Background(), mockVaultRepo, vaultDir, vaultDir)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	files, err := manager.ScanAll(context.Background())
	if err != nil {
		t.Fatalf("ScanAll() error = %v", err)
	}

	// Find the file we created
	var foundFile *ScannedFile
	for i := range files {
		if files[i].RelPath == "folder/note.md" || files[i].RelPath == "folder\\note.md" {
			foundFile = &files[i]
			break
		}
	}

	if foundFile == nil {
		t.Fatal("ScanAll() did not find expected file")
	}

	if foundFile.VaultID != vault.ID {
		t.Errorf("ScannedFile.VaultID = %d, want %d", foundFile.VaultID, vault.ID)
	}

	if foundFile.Folder != "folder" {
		t.Errorf("ScannedFile.Folder = %q, want folder", foundFile.Folder)
	}

	if foundFile.AbsPath != filePath {
		t.Errorf("ScannedFile.AbsPath = %q, want %q", foundFile.AbsPath, filePath)
	}
}

