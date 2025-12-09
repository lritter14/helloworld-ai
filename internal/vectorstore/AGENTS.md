# Vector Store Layer - Agent Guide

Vector database operations and semantic search patterns.

## Core Responsibilities

- Vector storage and retrieval (Qdrant)
- Semantic similarity search
- Collection management
- Metadata filtering for scoped searches

## Interface Design

Consumer-first interface defined in `interface.go`:

```go
type VectorStore interface {
    Upsert(ctx context.Context, collection string, points []Point) error
    Search(ctx context.Context, collection string, query []float32, k int, filters map[string]any) ([]SearchResult, error)
    Delete(ctx context.Context, collection string, ids []string) error
}
```

## Data Structures

```go
// Point represents a vector point with metadata
type Point struct {
    ID   string         // UUID (matches chunk ID from SQLite)
    Vec  []float32      // Embedding vector
    Meta map[string]any // Metadata for filtering (vault_id, folder, etc.)
}

// SearchResult represents a search result
type SearchResult struct {
    PointID string         // UUID of the point
    Score   float32        // Similarity score
    Meta    map[string]any // Metadata from point
}
```

## Qdrant Implementation

**Client Creation:**

```go
vectorStore, err := vectorstore.NewQdrantStore(cfg.QdrantURL)
// URL format: "http://localhost:6333" (gRPC port 6334 is auto-derived)
```

**Collection Management:**

```go
// Ensure collection exists with correct vector size
err := vectorStore.EnsureCollection(ctx, cfg.QdrantCollection, cfg.QdrantVectorSize)
// Creates collection if missing, validates vector size if exists
```

## Upsert Pattern

```go
points := []vectorstore.Point{
    {
        ID:  chunkID,           // UUID string
        Vec: embedding,         // []float32 from embeddings client
        Meta: map[string]any{
            "vault_id":     vaultID,
            "vault_name":   vaultName,
            "note_id":      noteID,
            "rel_path":     relPath,
            "folder":       folder,
            "heading_path": headingPath,
            "chunk_index":  chunkIndex,
            "note_title":   noteTitle,
        },
    },
}

err := vectorStore.Upsert(ctx, collection, points)
```

## Search Pattern

```go
// Build filters
filters := map[string]any{
    "vault_id": 1,              // Exact match
    "folder":   "projects",      // Prefix match (matches "projects/work", etc.)
}

results, err := vectorStore.Search(ctx, collection, queryVector, k, filters)
// Returns []SearchResult ordered by similarity score
```

**Filter Types:**

- `vault_id` - Exact integer match
- `folder` - Prefix matching (empty string = root-level files only)

## Delete Pattern

```go
chunkIDs := []string{"uuid1", "uuid2", "uuid3"}
err := vectorStore.Delete(ctx, collection, chunkIDs)
```

## Collection Initialization

Collections are initialized at startup in `main.go`:

```go
// Create vector store client
vectorStore, err := vectorstore.NewQdrantStore(cfg.QdrantURL)

// Ensure collection exists with correct vector size
err = vectorStore.EnsureCollection(ctx, cfg.QdrantCollection, cfg.QdrantVectorSize)

// Validate embedding client returns matching vector size
embedder := llm.NewEmbeddingsClient(...)
testEmbeddings, _ := embedder.EmbedTexts(ctx, []string{"test"})
// Fail-fast if vector size mismatch
```

## Metadata Fields

Per Section 0.20 of plan.md, store these exact fields in point metadata:

- `vault_id` (integer)
- `vault_name` (string)
- `note_id` (string, UUID)
- `rel_path` (string)
- `folder` (string)
- `heading_path` (string)
- `chunk_index` (integer)
- `note_title` (string)

## Error Handling

```go
if err != nil {
    logger.ErrorContext(ctx, "vector store operation failed", "error", err)
    return fmt.Errorf("failed to upsert points: %w", err)
}
```

- Wrap Qdrant errors with context
- Log errors with structured logging
- Return descriptive error messages

## Rules

- Use context in all operations
- Extract logger from context (fallback to default)
- Validate vector sizes match collection config
- Use prefix matching for folder filters
- Store metadata as specified in plan (Section 0.20)
- Single collection for all vaults (filter by metadata)

