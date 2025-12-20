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
    K        int      `json:"k,omitempty"`       // Legacy manual override (auto-selected otherwise)
    Detail   string   `json:"detail,omitempty"`  // "brief", "normal", "detailed" hint
    Debug    bool     `json:"debug,omitempty"`   // Enable debug mode for detailed retrieval info
}

type AskResponse struct {
    Answer        string      `json:"answer"`
    References    []Reference `json:"references"`
    Abstained     bool        `json:"abstained,omitempty"`     // Explicit abstention flag
    AbstainReason string      `json:"abstain_reason,omitempty"` // Reason for abstention
    Debug         *DebugInfo  `json:"debug,omitempty"`         // Debug information when debug mode enabled
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

### Automatic K Selection

- K is now auto-selected per query (min 3, default 5, max 8) before reranking.
- Heuristics consider:
  - Answer detail hint (`brief`, `normal`, `detailed`)
  - Question breadth (length, multiple question marks, broad keywords like "overview"/"everything")
  - Folder filters (narrow filters nudge K lower)
- Legacy requests with an explicit `K` still override auto-selection (clamped to 3–8) for backward compatibility.

4. **Search Vector Store + Build Candidate Pool:**
   - Search each folder separately (with folder filter) using `candidateKPerScope` (15) hits per scope to maximize recall
   - Apply folder position weighting (earlier folders = higher weight)
   - If no folders selected, search all folders per vault (no folder filter)
   - Combine and deduplicate results by PointID
   - Sort by weighted vector score, then trim to `maxCandidates` (200) before reranking
   - Drop any candidate with vector score `< 0.3` to avoid obvious noise

5. **Lexical Rerank:**
   - Fetch chunk text for each remaining candidate (already required later) and score it with `lexicalScore(question, chunkText, headingPath)`
   - Lexical scoring details:
     - Lowercase/tokenize query + chunk text, skip stopwords, count term frequency matches
     - Normalize matches by chunk length (`lexicalLengthScale = 10`) and clamp to `[0, 0.4]`
     - Add a small heading bonus (`0.1`) when tokens appear in the heading path
   - Blend scores: `finalScore = 0.7*vectorScore + 0.3*lexicalScore`
   - Drop candidates with `finalScore < 0.4`
   - Sort by `finalScore` and keep up to `rerankKeep` (8) results, respecting the auto-selected `k` (range 3–8, unless a legacy request overrides it)

6. **Fetch Chunk Texts (already available during rerank):**

   ```go
   chunk, err := e.chunkRepo.GetByID(ctx, result.PointID)
   ```

7. **Format Context:**

   ```text
   --- Context from notes ---
   
   [Chunk 1]
   [Vault: personal] File: projects/meeting-notes.md
   Section: # Meetings > ## Weekly Standup
   Content: [chunk text here]
   
   [Chunk 2]
   [Vault: personal] File: projects/meeting-notes.md
   Section: # Meetings > ## Daily Standup
   Content: [chunk text here]
   
   --- End Context ---
   
   When citing sources, use the format '[File: filename.md, Section: section name]' matching the exact filename and section name from the context above.
   ```

8. **Call LLM:**

   ```go
   messages := []llm.Message{
       {Role: "system", Content: systemPrompt},
       {Role: "user", Content: question + context},
   }
   answer, err := e.llmClient.ChatWithMessages(ctx, messages, params)
   ```

9. **Build References:**
   - Extract citations from LLM answer using `extractCitationsFromAnswer()` method
   - Parse citations in format `[File: filename.md, Section: section name]` from the answer
   - Match cited files and sections to chunks to build references for only cited chunks
   - Fall back to all chunks if no citations found (backward compatibility)
   - This ensures references align with actual citations, improving Attribution Hit Rate

10. **Collect Debug Information (if requested):**
    - If `req.Debug` is true, build debug info from retrieval results
    - Include all candidates considered during reranking (not just final selection)
    - Include scores (vector, lexical, final), ranks, and metadata
    - Include folder selection information (selected and available folders)
    - Convert folder formats for display (vaultID/folder → vaultName/folder)

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

**Lexical Rerank Filtering:**

- After dedupe, cap candidates (`maxCandidates = 200`) and score each chunk lexically
- Blend vector + lexical scores and drop anything below the final threshold (`finalScore < 0.4`)
- Keep up to `rerankKeep = 8` candidates (bounded by requested `k`)
- Logs vector vs lexical vs final scores for the top items so weights can be tuned

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

## Debug Mode

The RAG engine supports debug mode for evaluation frameworks and debugging:

### Debug Information

When `req.Debug` is true, the response includes:

- **RetrievedChunks:** All candidates considered during reranking (before final selection)
  - Chunk ID (stable, deterministic)
  - Rel path, heading path, text
  - Scores: vector, lexical, final
  - Rank (1-based)
- **FolderSelection:** Folder selection information
  - Selected folders (in order, with vault names)
  - Available folders (with vault names)

### Usage

```go
req := AskRequest{
    Question: "What is the project about?",
    Debug:    true,
}
resp, err := engine.Ask(ctx, req)
if resp.Debug != nil {
    // Access debug information
    for _, chunk := range resp.Debug.RetrievedChunks {
        fmt.Printf("Chunk %s: score=%.2f, rank=%d\n", 
            chunk.ChunkID, chunk.ScoreFinal, chunk.Rank)
    }
}
```

### Implementation

Debug information is built by `buildDebugInfo()` method:

- Collects all candidates from reranking phase
- Converts folder formats for display
- Includes full chunk text and metadata

## Abstention

The RAG engine supports explicit abstention when no relevant content is found:

### Abstention Behavior

The engine sets `Abstained: true` and `AbstainReason` in the following cases:

1. **No Search Results:** When vector search returns no results
   - `Abstained: true`
   - `AbstainReason: "no_relevant_context"`

2. **No Candidates After Vector Threshold:** When all candidates are filtered out due to low vector scores
   - `Abstained: true`
   - `AbstainReason: "no_relevant_context"`

3. **No Candidates After Final Score Threshold:** When all candidates are filtered out after reranking
   - `Abstained: true`
   - `AbstainReason: "no_relevant_context"`

### Abstention Reasons

Current supported reasons:

- `"no_relevant_context"`: No relevant chunks found in the indexed content

Future reasons (for LLM-based abstention detection):

- `"ambiguous_question"`: Question is too ambiguous to answer
- `"insufficient_information"`: Context contains some information but not enough to answer

### Abstention Usage Example

```go
resp, err := engine.Ask(ctx, req)
if resp.Abstained {
    fmt.Printf("System abstained: %s\n", resp.AbstainReason)
    // Handle abstention case
}
```

**Rationale:** Explicit abstention flags are critical for evaluation frameworks to distinguish between:

- "No answer found" (abstained) - correct behavior when content doesn't exist
- "Answer generated" - may be correct or hallucinated

This enables proper abstention metrics calculation in evaluation frameworks.

## Rules

- NO HTTP types - Domain models only
- Extract logger from context
- Handle multiple vaults by searching separately
- Use intelligent folder selection (user folders + LLM ranking)
- Apply folder position weighting to search scores
- Always rerank via lexical score blending before selecting final chunks
- Format context per plan specification
- Use exact system prompt from plan
- Return references from search result metadata
- Handle all error returns properly
- Use `noteRepo.ListUniqueFolders()` to get available folders for selection
- **Debug mode:** Collect debug information when `req.Debug` is true, include all candidates from reranking phase
