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

## Rules

- NO business logic - Only persistence and queries
- Return `ErrNotFound` for missing records
- Wrap database errors with context
- Use context-aware operations (`QueryRowContext`, `ExecContext`)
