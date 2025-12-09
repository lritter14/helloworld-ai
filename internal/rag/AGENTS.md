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
    noteRepo    storage.NoteStore  // For ListUniqueFolders
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

2. **Resolve Vaults:**
   - Resolve vault names to IDs (if provided)
   - If no vaults specified, use all vaults
   - Build vault name to ID map for folder conversion

3. **Select Relevant Folders:**
   - Get available folders via `noteRepo.ListUniqueFolders(ctx, vaultIDs)`
   - User-provided folders are prioritized (exact or prefix matching)
   - Use LLM to rank remaining folders by relevance to question
   - Returns ordered list: user folders first, then LLM-ranked folders
   - If no folders selected, search all folders (no folder filter)

4. **Search Vector Store:**
   - Search each folder separately (with folder filter)
   - Apply folder position weighting (earlier folders = higher weight)
   - If no folders selected, search all folders per vault (no folder filter)
   - Combine and deduplicate results by PointID
   - Sort by weighted score (highest first)
   - **Filter by score threshold:** Remove results below minimum similarity score (0.55)
   - Take top K results (default 5, max 20)

5. **Fetch Chunk Texts:**

   ```go
   chunk, err := e.chunkRepo.GetByID(ctx, result.PointID)
   ```

6. **Format Context:**

   ```text
   --- Context from notes ---
   
   [Vault: personal] File: projects/meeting-notes.md
   Section: # Meetings > ## Weekly Standup
   Content: [chunk text here]
   
   --- End Context ---
   ```

7. **Call LLM:**

   ```go
   messages := []llm.Message{
       {Role: "system", Content: systemPrompt},
       {Role: "user", Content: question + context},
   }
   answer, err := e.llmClient.ChatWithMessages(ctx, messages, params)
   ```

8. **Build References:**
   Extract metadata from search results to build reference list

## System Prompt

Use exact system prompt from plan:

```text
You are a helpful assistant that answers questions based on the provided context from the user's notes. 
Answer the question using only the information from the context below. If the context doesn't contain 
enough information to answer the question, say so. Cite specific sections when possible.
```

## Folder Selection Pattern

The RAG engine uses intelligent folder selection to improve search relevance:

### selectRelevantFolders Method

```go
func (e *ragEngine) selectRelevantFolders(ctx context.Context, question string, 
    availableFolders []string, userFolders []string, vaultIDs []int, 
    vaultMap map[int]string) []string
```

**Workflow:**

1. **User Folders First:** Match user-provided folders to available folders (exact or prefix matching)
   - Supports formats: `"folder"`, `"<vaultID>/folder"`, `"<vaultName>/folder"`
   - Prefix matching: `"projects"` matches `"projects/work"`

2. **LLM Ranking:** Use LLM to rank remaining folders by relevance to question
   - Converts folders to vault name format for LLM (e.g., `"personal/workouts"`)
   - Prompt explicitly instructs LLM to only include DIRECTLY relevant folders
   - Prompt instructs LLM to exclude tangentially related folders
   - LLM returns JSON array of ranked folders
   - Handles markdown code blocks and JSON prefixes in LLM response
   - Falls back to all available folders if LLM fails

3. **Return Ordered List:** User folders first, then LLM-ranked folders

**Folder Format Conversion:**

- Internal format: `"<vaultID>/folder"` (e.g., `"1/projects/work"`)
- LLM format: `"<vaultName>/folder"` (e.g., `"personal/projects/work"`)
- Conversion handled automatically via vault ID to name map

## Multiple Vault Handling

When multiple vaults are requested, search each vault separately and combine results:

```go
var allSearchResults []vectorstore.SearchResult
for _, vaultID := range vaultIDs {
    filters := map[string]any{"vault_id": vaultID}
    // If folders selected, search each folder separately with weighting
    // If no folders, search all folders (no folder filter)
    results, _ := e.vectorStore.Search(ctx, collection, queryVector, k, filters)
    allSearchResults = append(allSearchResults, results...)
}
// Deduplicate by PointID and sort by weighted score
```

**Folder Weighting:**

- Earlier folders in ordered list get higher weight (1.0, 0.9, 0.8, ...)
- Minimum weight: 0.1
- Applied to search result scores before deduplication

**Score Threshold Filtering:**

- After sorting by weighted score, filter out low-relevance results
- Minimum similarity score threshold: 0.55
- Results below threshold are excluded before taking top K
- Logs filtered results at INFO level, individual filtered chunks at DEBUG level
- Improves answer quality by excluding tangentially related chunks

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
- Use intelligent folder selection (user folders + LLM ranking)
- Apply folder position weighting to search scores
- Filter results by score threshold (0.55 minimum) before taking top K
- Format context per plan specification
- Use exact system prompt from plan
- Return references from search result metadata
- Handle all error returns properly
- Use `noteRepo.ListUniqueFolders()` to get available folders for selection
