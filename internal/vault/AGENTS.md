# Vault Layer - Agent Guide

Vault management and file scanning patterns.

## Core Responsibilities

- Vault initialization and caching
- File system scanning for markdown files
- Path resolution (absolute/relative path conversion)
- Vault metadata management

## Vault Manager

The `Manager` struct manages vault configuration and provides vault lookup and path resolution.

### Initialization

```go
vaultManager, err := vault.NewManager(ctx, vaultRepo, cfg.VaultPersonalPath, cfg.VaultWorkPath)
if err != nil {
    log.Fatalf("Failed to initialize vault manager: %v", err)
}
```

**Behavior:**
- Automatically creates/retrieves "personal" and "work" vaults from database
- Caches vaults in memory for O(1) lookup
- Returns error if vault initialization fails

### Vault Lookup

```go
vault, err := vaultManager.VaultByName("personal")
if err != nil {
    return fmt.Errorf("vault not found: %w", err)
}
```

**Usage:**
- Lookup vault by name (e.g., "personal", "work")
- Returns cached vault record
- Used by indexer and RAG engine to resolve vault IDs

### Path Resolution

```go
absPath := vaultManager.AbsPath(vaultID, "projects/meeting-notes.md")
// Returns: /Users/user/Projects/obsidian/personal/projects/meeting-notes.md
```

**Usage:**
- Convert vault ID + relative path to absolute file path
- Used by indexer to read note files
- Handles cross-platform path separators

## File Scanner

The `ScanAll` method discovers all markdown files in configured vaults.

### Scanning

```go
scannedFiles, err := vaultManager.ScanAll(ctx)
if err != nil {
    return fmt.Errorf("failed to scan vaults: %w", err)
}

for _, file := range scannedFiles {
    fmt.Printf("Found: %s (vault: %d, folder: %s)\n", 
        file.RelPath, file.VaultID, file.Folder)
}
```

### ScannedFile Structure

```go
type ScannedFile struct {
    VaultID int    // Vault ID from database
    RelPath string // Relative path from vault root (e.g., "projects/meeting-notes.md")
    Folder  string // Folder path (path components except filename, e.g., "projects")
    AbsPath string // Absolute file path
}
```

**Folder Calculation:**
- Root-level files: `folder = ""` (empty string)
- Nested files: `folder = "projects/work"` (all path components except filename)
- Uses forward slashes for consistency (`filepath.ToSlash`)

### Scanning Behavior

- **Filters:** Only `.md` files are included
- **Skips:** `.obsidian` directory (Obsidian configuration)
- **Error Handling:** Continues scanning other vaults if one fails
- **Context Support:** Respects context cancellation
- **Cross-platform:** Uses `filepath` package for path operations

## Integration with Storage

The vault manager depends on `storage.VaultStore` interface:

```go
type VaultStore interface {
    GetOrCreateByName(ctx context.Context, name, rootPath string) (VaultRecord, error)
    ListAll(ctx context.Context) ([]VaultRecord, error)
}
```

## Testing

### Mock Generation

The `VaultStore` interface has a `//go:generate` directive (in storage package).

### Test Patterns

**Mock Usage:**

```go
ctrl := gomock.NewController(t)
defer ctrl.Finish()

mockVaultStore := storage_mocks.NewMockVaultStore(ctrl)
mockVaultStore.EXPECT().GetOrCreateByName(gomock.Any(), "personal", gomock.Any()).Return(storage.VaultRecord{ID: 1, Name: "personal"}, nil)

manager, err := vault.NewManager(context.Background(), mockVaultStore, "/tmp/personal", "/tmp/work")
```

**File System Testing:**

Use temporary directories for test isolation:

```go
tmpDir := t.TempDir()
vaultPath := filepath.Join(tmpDir, "vault")
os.MkdirAll(vaultPath, 0755)
```

**Error Handling:**

Properly handle all error returns:

```go
defer func() {
    _ = db.Close() // Ignore error in test cleanup
}()
```

## Rules

- **Vault Caching:** Manager caches vaults for efficient lookup
- **Path Normalization:** Always use `filepath` package for cross-platform compatibility
- **Error Handling:** Wrap errors with context, continue scanning on per-vault errors
- **Context Support:** All operations accept `context.Context` for cancellation
- **Folder Calculation:** Empty string for root-level files per Section 0.6 of plan.md
- **Test Isolation:** Use temporary directories for file system tests
- **Error Handling:** Handle all error returns (use `_` for intentional ignores in cleanup)

## Usage in Indexer (Phase 6)

The vault manager is used by the indexer to:
1. Discover files: `vaultManager.ScanAll(ctx)`
2. Read files: `vaultManager.AbsPath(vaultID, relPath)`
3. Resolve vault IDs: `vaultManager.VaultByName(name)`

