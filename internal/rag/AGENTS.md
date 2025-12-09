# RAG Layer - Agent Guide

RAG (Retrieval-Augmented Generation) engine for question-answering over indexed notes.

## Core Responsibilities

- Embed user questions for semantic search
- Retrieve relevant chunks from vector store
- Format context from retrieved chunks
- Generate answers using LLM with context
- Build references to source chunks

## RAG Engine Pattern

```go
type Engine interface {
    Ask(ctx context.Context, req AskRequest) (AskResponse, error)
}

type ragEngine struct {
    embedder    *llm.EmbeddingsClient
    vectorStore vectorstore.VectorStore
    collection  string
    chunkRepo   storage.ChunkStore
    vaultRepo   storage.VaultStore
    llmClient   *llm.Client
    logger      *slog.Logger
}
```

## Domain Types

Define request/response types in `types.go`:

```go
type AskRequest struct {
    Question string   `json:"question"`
    Vaults   []string `json:"vaults,omitempty"`  // Empty = all vaults
    Folders  []string `json:"folders,omitempty"` // Prefix matching
    K        int      `json:"k,omitempty"`       // Default 5, max 20
}

type AskResponse struct {
    Answer     string      `json:"answer"`
    References []Reference `json:"references"`
}

type Reference struct {
    Vault       string `json:"vault"`
    RelPath     string `json:"rel_path"`
    HeadingPath string `json:"heading_path"`
    ChunkIndex  int    `json:"chunk_index"`
}
```

## RAG Workflow

1. **Embed Question:**
   ```go
   embeddings, err := e.embedder.EmbedTexts(ctx, []string{req.Question})
   queryVector := embeddings[0]
   ```

2. **Build Filters:**
   - Resolve vault names to IDs (if provided)
   - Add folder filters (prefix matching)
   - Default K to 5, enforce max 20

3. **Search Vector Store:**
   - Handle multiple vaults by searching each separately
   - Combine and deduplicate results
   - Sort by score (highest first)
   - Take top K results

4. **Fetch Chunk Texts:**
   ```go
   chunk, err := e.chunkRepo.GetByID(ctx, result.PointID)
   ```

5. **Format Context:**
   ```
   --- Context from notes ---
   
   [Vault: personal] File: projects/meeting-notes.md
   Section: # Meetings > ## Weekly Standup
   Content: [chunk text here]
   
   --- End Context ---
   ```

6. **Call LLM:**
   ```go
   messages := []llm.Message{
       {Role: "system", Content: systemPrompt},
       {Role: "user", Content: question + context},
   }
   answer, err := e.llmClient.ChatWithMessages(ctx, messages, params)
   ```

7. **Build References:**
   Extract metadata from search results to build reference list

## System Prompt

Use exact system prompt from plan:

```text
You are a helpful assistant that answers questions based on the provided context from the user's notes. 
Answer the question using only the information from the context below. If the context doesn't contain 
enough information to answer the question, say so. Cite specific sections when possible.
```

## Multiple Vault Handling

When multiple vaults are requested, search each vault separately and combine results:

```go
var allSearchResults []vectorstore.SearchResult
for _, vaultID := range vaultIDs {
    filters := map[string]any{"vault_id": vaultID}
    results, _ := e.vectorStore.Search(ctx, collection, queryVector, k, filters)
    allSearchResults = append(allSearchResults, results...)
}
// Deduplicate and sort by score
```

## Error Handling

- Log errors with structured logging
- Return wrapped errors with context
- Handle empty search results gracefully (return helpful message)
- Continue with other vaults if one fails

## Testing

### Mock Generation

Mock dependencies using interfaces from other packages (vectorstore, storage, llm).

### Test Patterns

**Mock Dependencies:**

```go
ctrl := gomock.NewController(t)
defer ctrl.Finish()

mockVectorStore := mocks.NewMockVectorStore(ctrl)
mockChunkRepo := mocks.NewMockChunkStore(ctrl)
mockVaultRepo := mocks.NewMockVaultStore(ctrl)
mockLLMClient := &llm.Client{...} // Or use mock if available
mockEmbedder := &llm.EmbeddingsClient{...}

engine := NewEngine(mockEmbedder, mockVectorStore, "collection", 
    mockChunkRepo, mockVaultRepo, mockLLMClient)
```

**Test Scenarios:**

- Empty search results
- Multiple vaults
- Folder filtering
- K limits (default, max)
- Error handling (embedding, vector store, LLM)

## Rules

- NO HTTP types - Domain models only
- Extract logger from context
- Handle multiple vaults by searching separately
- Format context per plan specification
- Use exact system prompt from plan
- Return references from search result metadata
- Handle all error returns properly

