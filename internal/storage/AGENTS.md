# Storage Layer - Agent Guide

Database operations and repository patterns.

## Core Responsibilities

- Database connection management
- Data persistence (CRUD)
- Query execution and result mapping
- Transaction management

## Repository Pattern

```go
type NoteStore interface {
    GetByVaultAndPath(ctx context.Context, vaultID int, relPath string) (*NoteRecord, error)
    Upsert(ctx context.Context, note *NoteRecord) error
    DeleteAll(ctx context.Context) error
    ListUniqueFolders(ctx context.Context, vaultIDs []int) ([]string, error) // For RAG folder selection
}

type ChunkStore interface {
    Insert(ctx context.Context, chunk *ChunkRecord) error
    DeleteByNote(ctx context.Context, noteID string) error
    ListIDsByNote(ctx context.Context, noteID string) ([]string, error)
    GetAllIDs(ctx context.Context) ([]string, error) // For clearing all data
    GetByID(ctx context.Context, id string) (*ChunkRecord, error) // For RAG queries
}

type NoteRepo struct {
    db *sql.DB
}

func NewNoteRepo(db *sql.DB) *NoteRepo {
    return &NoteRepo{db: db}
}
```

## Database Models

Use `*Record` suffix:

```go
type NoteRecord struct {
    ID        string    `db:"id"`
    VaultID   int       `db:"vault_id"`
    RelPath   string    `db:"rel_path"`
    Title     string    `db:"title"`
    UpdatedAt time.Time `db:"updated_at"`
}
```

## Context-Aware Queries

```go
err := r.db.QueryRowContext(ctx,
    "SELECT id, title FROM notes WHERE vault_id = ? AND rel_path = ?",
    vaultID, relPath,
).Scan(&note.ID, &note.Title)
```

## Error Handling

```go
if err == sql.ErrNoRows {
    return nil, ErrNotFound
}
if err != nil {
    return nil, fmt.Errorf("failed to query note: %w", err)
}
```

## Testing

### Test Patterns

**Database Setup:**

```go
tmpDir := t.TempDir()
dbPath := filepath.Join(tmpDir, "test.db")

db, err := New(dbPath)
if err != nil {
    t.Fatalf("New() error = %v", err)
}
defer func() {
    _ = db.Close() // Explicitly ignore error in test cleanup
}()

if err := Migrate(db); err != nil {
    t.Fatalf("Migrate() error = %v", err)
}
```

**Mock Generation:**

Repository interfaces have `//go:generate` directives:

```go
//go:generate go run go.uber.org/mock/mockgen@latest -destination=mocks/mock_note_store.go -package=mocks helloworld-ai/internal/storage NoteStore
```

**Error Handling:**

Properly handle all error returns:

```go
defer func() {
    _ = rows.Close() // Ignore error in defer
}()
```

**Test Cleanup:**

```go
_, _ = db.Exec("DELETE FROM notes") // Ignore error in test cleanup
```

## GetByID Pattern

For RAG queries, chunks are retrieved by ID (Qdrant point ID):

```go
func (r *ChunkRepo) GetByID(ctx context.Context, id string) (*ChunkRecord, error) {
    var chunk ChunkRecord
    err := r.db.QueryRowContext(ctx,
        "SELECT id, note_id, chunk_index, heading_path, text FROM chunks WHERE id = ?",
        id,
    ).Scan(&chunk.ID, &chunk.NoteID, &chunk.ChunkIndex, &chunk.HeadingPath, &chunk.Text)
    
    if err == sql.ErrNoRows {
        return nil, ErrNotFound
    }
    // ...
}
```

Used by RAG engine to fetch chunk text after vector search.

## ListUniqueFolders Pattern

For RAG folder selection, returns all unique folder paths optionally filtered by vault IDs:

```go
func (r *NoteRepo) ListUniqueFolders(ctx context.Context, vaultIDs []int) ([]string, error) {
    // Query distinct vault_id, folder pairs
    // Format as "<vaultID>/folder" (e.g., "1/projects/work")
    // Include all nested parent folders (e.g., "1/projects" and "1/projects/work")
    // Return unique list sorted by vault_id, folder
}
```

**Key Features:**

- Returns folders in format `"<vaultID>/folder"` for internal use
- Includes all nested parent folders (e.g., if "projects/work" exists, also includes "projects")
- If `vaultIDs` is empty, returns folders from all vaults
- Used by RAG engine for intelligent folder selection

**Example Output:**

```go
// For vault 1 with folders: "", "projects", "projects/work"
// Returns: ["1/", "1/projects", "1/projects/work"]
```

## Rules

- NO business logic - Only persistence and queries
- Return `ErrNotFound` for missing records
- Wrap database errors with context
- Use context-aware operations (`QueryRowContext`, `ExecContext`)
- Handle all error returns (use `_` for intentional ignores in cleanup)
- Use temporary directories for test isolation
- `GetByID` returns full chunk record including text (for RAG)
- `ListUniqueFolders` returns folders in format `"<vaultID>/folder"` including nested parents
