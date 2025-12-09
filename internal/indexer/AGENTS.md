# Indexer Layer - Agent Guide

Markdown chunking and indexing pipeline patterns.

## Core Responsibilities

- Parse markdown files using goldmark AST
- Chunk markdown by heading hierarchy
- Extract document titles
- Generate embeddings for chunks
- Store chunks in SQLite (metadata) and Qdrant (vectors)
- Hash-based change detection (skip unchanged files)

## Architecture

The indexer consists of two main components:

1. **Chunker** (`chunker.go`) - Parses markdown and creates chunks
2. **Pipeline** (`pipeline.go`) - Orchestrates indexing workflow

## Chunker

The `GoldmarkChunker` uses goldmark AST parsing to chunk markdown by heading hierarchy.

### Chunking Strategy

- **Heading-based:** Each chunk starts at a heading and includes content until the next heading of equal or higher level
- **First chunk:** Uses document title as heading path if no headings exist
- **Size constraints:**
  - Minimum: 50 characters (merge tiny chunks with next)
  - Maximum: 2000 characters (split if exceeded, prefer heading boundaries)
- **Heading path format:** `"# Heading1 > ## Heading2 > ### Heading3"` (uses `>` separator)

### Title Extraction

Extracts title in this order:

1. First `# Heading` (level 1)
2. If none, first `## Heading` (level 2)
3. If none, use filename without extension (capitalize words)

### Usage

```go
chunker := indexer.NewGoldmarkChunker()
title, chunks, err := chunker.ChunkMarkdown(content, filename)
if err != nil {
    return fmt.Errorf("failed to chunk: %w", err)
}
```

### Chunk Structure

```go
type Chunk struct {
    Index       int    // Chunk index within note (starts at 0)
    HeadingPath string // Format: "# Heading1 > ## Heading2"
    Text        string // Chunk text content
}
```

## Pipeline

The `Pipeline` orchestrates the indexing workflow for markdown files.

### Initialization

```go
pipeline := indexer.NewPipeline(
    vaultManager,
    noteRepo,
    chunkRepo,
    embedder,
    vectorStore,
    collectionName,
)
```

### Indexing a Single Note

```go
err := pipeline.IndexNote(ctx, vaultID, relPath, folder)
if err != nil {
    return fmt.Errorf("failed to index note: %w", err)
}
```

**IndexNote Workflow:**

1. Get absolute path via `vaultManager.AbsPath(vaultID, relPath)`
2. Read file content
3. Compute SHA256 hash
4. Check existing note - skip if hash matches (unchanged)
5. Chunk content using `chunker.ChunkMarkdown()`
6. Use folder passed as parameter (already calculated during scanning)
7. Upsert note record (generate UUID if new)
8. If existing note, delete old chunks (SQLite + Qdrant)
9. Generate embeddings for chunk texts in batches (with automatic retry on errors)
10. Insert chunks into SQLite
11. Upsert vectors to Qdrant with metadata

### Indexing All Vaults

```go
err := pipeline.IndexAll(ctx)
if err != nil {
    log.Printf("Indexing completed with errors: %v", err)
    // Don't fail startup - log and continue
}
```

**IndexAll Workflow:**

1. Scan all vaults via `vaultManager.ScanAll(ctx)`
2. Loop through scanned files
3. Call `IndexNote` for each file
4. Log errors but continue (don't fail entire indexing)
5. Log summary: total files, success count, error count

### Hash-Based Change Detection

The indexer uses SHA256 hashing to detect file changes:

- Hash entire file content before chunking
- Store hash as hex string (64 characters) in `notes.hash`
- Skip re-indexing if hash matches existing note
- Delete old chunks and re-index if hash differs

### Metadata Storage

Each chunk is stored with metadata in Qdrant:

```go
Meta: map[string]any{
    "vault_id":    vaultID,      // int
    "vault_name":  vaultName,     // string
    "note_id":     noteID,        // string (UUID)
    "rel_path":    relPath,       // string
    "folder":      folder,        // string
    "heading_path": chunk.HeadingPath, // string
    "chunk_index": chunk.Index,   // int
    "note_title":  title,         // string
}
```

## Embedding Batch Processing

The indexer generates embeddings in batches to avoid exceeding server limits:

### Batch Limits

```go
const maxBatchCount = 5    // Max number of chunks per batch
const maxBatchChars = 8000 // Max total characters per batch
```

### Batch Building

- Respects both count and character limits
- Warns if single chunk exceeds character limit (still processes it)
- Builds batches sequentially until all chunks are processed

### Automatic Retry with Batch Reduction

The `embedTextsWithRetry` method automatically handles "input too large" errors:

```go
func (p *Pipeline) embedTextsWithRetry(ctx context.Context, texts []string, 
    relPath string, logger *slog.Logger) ([][]float32, error)
```

**Behavior:**

1. Attempts to embed the batch
2. If error contains "input is too large" or "too large to process":
   - Logs warning with batch size
   - Recursively splits batch in half
   - Retries each half separately
   - Combines results
3. If single chunk still fails, returns error
4. If non-size error, returns error immediately

**Error Detection:**

Checks for various error message patterns (case-insensitive):
- "input is too large"
- "too large to process"
- "increase the physical batch size"

## Integration Points

### Dependencies

- **Vault Manager** - File discovery and path resolution
- **Note Repository** - Note metadata storage
- **Chunk Repository** - Chunk metadata storage
- **Embeddings Client** - Vector generation (with batch size handling)
- **Vector Store** - Vector storage (Qdrant)

### Startup Integration

The indexer runs at startup in `main.go`:

```go
// Validate embedding client vector size (fail-fast)
embedder := llm.NewEmbeddingsClient(...)
testEmbeddings, err := embedder.EmbedTexts(ctx, []string{"test"})
if len(testEmbeddings[0]) != cfg.QdrantVectorSize {
    log.Fatalf("Embedding vector size mismatch: expected %d, got %d", ...)
}

// After validation
indexerPipeline := indexer.NewPipeline(...)
log.Printf("Starting indexing of vaults...")
if err := indexerPipeline.IndexAll(ctx); err != nil {
    log.Printf("Indexing completed with errors: %v", err)
    // Don't fail startup - log and continue
} else {
    log.Printf("Indexing completed successfully")
}
```

**Embedding Validation:**

- Validates embedding vector size at startup (fail-fast if mismatch)
- Ensures embeddings match Qdrant collection vector size
- Prevents indexing with incorrect vector dimensions

## Error Handling

- **File read errors:** Log and continue with next file
- **Chunking errors:** Return error (fails indexing for that file)
- **Embedding errors:** Automatic batch size reduction and retry
  - "Input too large" errors trigger automatic batch splitting
  - Recursively splits batches in half until successful or single chunk fails
  - Logs warnings when batches are split
- **Storage errors:** Return error (fails indexing for that file)
- **IndexAll:** Logs errors but doesn't fail startup (per Section 0.19)
- **Startup validation:** Embedding vector size mismatch fails startup immediately

## Testing

### Mock Generation

Dependencies have `//go:generate` directives in their respective packages:
- `storage.NoteStore` - in storage package
- `storage.ChunkStore` - in storage package
- `vectorstore.VectorStore` - in vectorstore package

### Test Patterns

**Mock Usage:**

```go
ctrl := gomock.NewController(t)
defer ctrl.Finish()

mockVaultManager := &vault.Manager{}
mockNoteRepo := storage_mocks.NewMockNoteStore(ctrl)
mockChunkRepo := storage_mocks.NewMockChunkStore(ctrl)
mockVectorStore := vectorstore_mocks.NewMockVectorStore(ctrl)

embedder := &llm.EmbeddingsClient{
    ExpectedSize: 768,
}

pipeline := indexer.NewPipeline(
    mockVaultManager,
    mockNoteRepo,
    mockChunkRepo,
    embedder,
    mockVectorStore,
    "test-collection",
)
```

**Chunker Testing:**

Test chunking logic with various markdown structures:

```go
tests := []struct {
    name     string
    content  string
    filename string
    wantTitle string
    wantChunks int
}{
    {
        name: "multiple headings",
        content: "# H1\nContent\n## H2\nMore content",
        filename: "test.md",
        wantTitle: "H1",
        wantChunks: 2,
    },
}
```

**Error Handling:**

Properly handle all error returns:

```go
logger := slog.New(slog.NewTextHandler(io.Discard, nil)) // Suppress logs in tests
```

## Rules

- **Hash-based skipping:** Always check hash before re-indexing
- **Batch operations:** Generate embeddings in batches respecting count and character limits
- **Automatic retry:** Use `embedTextsWithRetry` for automatic batch size reduction on errors
- **Batch limits:** Respect both `maxBatchCount` (5) and `maxBatchChars` (8000) limits
- **UUID generation:** Use `uuid.New()` for note IDs and chunk IDs
- **Context support:** All operations accept `context.Context` for cancellation
- **Error wrapping:** Wrap errors with context using `fmt.Errorf("...: %w", err)`
- **Logging:** Extract logger from context, fallback to default logger
- **Log batch operations:** Log batch sizes and retry attempts for observability
- **Test Isolation:** Use mocks for all dependencies
- **Log Suppression:** Suppress log output during tests for cleaner test runs
- **Startup validation:** Validate embedding vector size matches Qdrant collection (fail-fast)

## Chunking Edge Cases

- **Empty files:** Return title from filename, empty chunks array
- **No headings:** Use document title as heading path for first chunk
- **Very long sections:** Split at paragraph boundaries, then sentence boundaries, then hard split
- **Tiny chunks:** Merge with next chunk if below minimum size

