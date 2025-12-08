# Implementation Plan: RAG-Powered Note Q&A System

## Summary

This plan outlines the implementation of a RAG (Retrieval-Augmented Generation) system that indexes markdown notes from two vaults (personal and work) and enables question-answering over the indexed content. The system uses:

- **Go** backend with embedded HTML UI
- **SQLite** for metadata storage (vaults, notes, chunks)
- **Qdrant** vector database for semantic search
- **llama.cpp** via OpenAI-compatible API for LLM and embeddings
- **RAG pipeline** to answer questions using retrieved context from notes

### Current Status

✅ **Completed:**
- **Phase 1:** Basic HTTP server with routing (`cmd/api/main.go`)
- **Phase 1:** Config package (`internal/config/config.go`) - loads env vars with validation
- **Phase 1:** HTTP router extracted to `internal/http/router.go` using chi router
- **Phase 1:** CORS middleware extracted to `internal/http/middleware.go`
- **Phase 1:** Embedded HTML UI with streaming chat interface (`cmd/api/index.html`)
- **Phase 1:** LLM client for chat completions (`internal/llm/client.go`)
- **Phase 1:** Service layer for chat (`internal/service/chat.go`)
- **Phase 1:** Chat handler with streaming support (`internal/handlers/chat.go`)
- **Phase 1:** Route: `POST /api/chat` (basic chat, not RAG yet)
- **Phase 2:** SQLite database connection and migrations (`internal/storage/database.go`)
- **Phase 2:** Storage models (`internal/storage/models.go`) - Vault, Note, Chunk structs
- **Phase 2:** Vault repository (`internal/storage/vault_repo.go`) - GetOrCreateByName, ListAll
- **Phase 2:** Note repository (`internal/storage/note_repo.go`) - GetByVaultAndPath, Upsert
- **Phase 2:** Chunk repository (`internal/storage/chunk_repo.go`) - Insert, DeleteByNote, ListIDsByNote
- **Phase 2:** Database initialization integrated into `main.go`

❌ **Remaining:**
- Embeddings client
- Qdrant vector store integration
- Vault manager and scanner
- Markdown chunker
- Indexing pipeline
- RAG engine
- `/api/v1/ask` endpoint (RAG-powered Q&A)
- UI updates for RAG (vault selection, references display)

### Implementation Phases

The plan is organized into 7 phases, building from basic infrastructure to full RAG capabilities. Each phase builds on the previous one, with clear interfaces and contracts to ensure clean composition.

---

## 0. Key Decisions (Locked In - No Ambiguity)

These decisions must be followed exactly to ensure consistent implementation:

### 0.1 LLM Client Interface Strategy
**Decision:** Use **Option A** - Extend existing `llm.Client` with new method `ChatWithMessages(ctx context.Context, messages []Message, params ChatParams) (string, error)`. Keep existing `Chat(ctx, message string)` method for backward compatibility. The RAG engine will use `ChatWithMessages`.

### 0.2 UUID Generation
**Decision:** Use `github.com/google/uuid` package. Generate UUIDs with `uuid.New()` for both note IDs and chunk IDs. Store as strings in database (TEXT type).

### 0.3 Qdrant Client Library
**Decision:** Use `github.com/qdrant/go-client` (official Go client) rather than raw HTTP. This provides better type safety and error handling.

### 0.4 Chunker Implementation
**Decision:** Use `github.com/yuin/goldmark` with `goldmark/ast` for parsing markdown structure. This provides proper AST parsing rather than regex, ensuring accurate heading detection and section boundaries.

### 0.5 Chunking Strategy
**Decision:** Chunk by heading hierarchy:
- Each chunk starts at a heading (##, ###, etc.) and includes all content until the next heading of equal or higher level
- First chunk (before any heading) uses the document title as heading path
- Minimum chunk size: 50 characters (merge tiny chunks with next)
- Maximum chunk size: 2000 characters (split if exceeded, but prefer heading boundaries)
- Heading path format: `"# Heading1 > ## Heading2 > ### Heading3"` (use `>` separator)

### 0.6 Folder Calculation
**Decision:** Extract folder from `relPath` by taking all path components except the filename. Example: `relPath = "projects/work/meeting-notes.md"` → `folder = "projects/work"`. Root-level files have `folder = ""` (empty string).

### 0.7 Note Title Extraction
**Decision:** Extract title in this order:
1. First `# Heading` (level 1) in the document
2. If no level 1 heading, use first `## Heading` (level 2)
3. If no headings at all, use filename without extension (capitalize first letter of each word)

### 0.8 Hash Algorithm
**Decision:** Use SHA256 for content hashing. Use `crypto/sha256` standard library. Store hash as hex string (64 characters).

### 0.9 Default K Value
**Decision:** Default `K = 5` chunks retrieved for RAG queries if not specified in request. Maximum allowed: `K = 20`.

### 0.10 System Prompt for RAG
**Decision:** Use this exact system prompt:

```text
You are a helpful assistant that answers questions based on the provided context from the user's notes. 
Answer the question using only the information from the context below. If the context doesn't contain 
enough information to answer the question, say so. Cite specific sections when possible.
```

### 0.11 Context Formatting
**Decision:** Format context string as follows (one chunk per section):

```text
--- Context from notes ---

[Vault: personal] File: projects/meeting-notes.md
Section: # Meetings > ## Weekly Standup
Content: [chunk text here]

[Vault: work] File: docs/api-design.md  
Section: # API Design > ## Endpoints
Content: [chunk text here]

--- End Context ---
```

### 0.12 Vector Store Collection
**Decision:** Use a **single collection** for all vaults. Collection name comes from `QDRANT_COLLECTION` env var (default: `"notes"`). Filter by `vault_id` and `folder` in metadata for scoped searches.

### 0.13 Vector Size Configuration
**Decision:** Vector size must match the embedding model output. Common values: 384, 768, 1536. Read from `QDRANT_VECTOR_SIZE` env var. **Must validate** that embedding client returns vectors of this size, fail fast if mismatch.

### 0.14 Folder Filtering
**Decision:** Folder filters use **prefix matching**. If `Folders: ["projects"]` is provided, match any folder that starts with `"projects"` (e.g., `"projects/work"`, `"projects/personal"`). Empty string matches root-level files only.

### 0.15 Indexing Strategy
**Decision:**
- Index all notes **once at startup** (synchronous, blocking)
- Re-index only if file hash changes (detected during `IndexNote`)
- **No file watching** in initial implementation (Phase 6)
- If indexing fails for a file, log error and continue with other files

### 0.16 Streaming for RAG
**Decision:** **Do NOT stream RAG responses** initially. Return complete `AskResponse` with answer and references. Streaming can be added later if needed. Keep `/api/chat` endpoint for streaming basic chat.

### 0.17 Database Path
**Decision:** Default `DB_PATH = "./data/helloworld-ai.db"`. Create `./data` directory if it doesn't exist.

### 0.18 Port Configuration
**Decision:** Default `API_PORT = "9000"` (from existing code). Read from env var `API_PORT`.

### 0.19 Error Handling Strategy
**Decision:**
- **Indexing errors:** Log and continue (don't fail entire startup)
- **RAG query errors:** Return HTTP 500 with error message in JSON
- **Invalid requests:** Return HTTP 400 with validation error details
- **Vector store errors:** Log error, return HTTP 503 (service unavailable)
- **LLM/embedding errors:** Log error, return HTTP 502 (bad gateway)

### 0.20 Metadata Fields in Qdrant Points
**Decision:** Store these exact fields in Qdrant point metadata (all as strings or integers):
- `vault_id` (integer)
- `vault_name` (string)
- `note_id` (string, UUID)
- `rel_path` (string)
- `folder` (string)
- `heading_path` (string)
- `chunk_index` (integer)
- `note_title` (string)

### 0.21 HTTP Router Structure
**Decision:** Extract router to `internal/http/router.go` with `NewRouter(deps *Deps) http.Handler`. Use **chi router** (`github.com/go-chi/chi/v5`) instead of standard library. Move CORS middleware to `internal/http/middleware.go`. Keep existing `/api/chat` endpoint, add new `/api/v1/ask` endpoint.

**Dependencies struct:**
```go
type Deps struct {
    ChatService service.ChatService  // for /api/chat
    RAGEngine   rag.Engine           // for /api/v1/ask
    IndexHTML   string                // embedded HTML content
}
```

**Note:** Chi middleware (Logger, Recoverer) are included. RequestID and RealIP middleware were removed as optional.

### 0.22 Config Package Structure
**Decision:** Create `internal/config/config.go` with:
```go
type Config struct {
    LLMBaseURL         string
    LLMModelName       string
    LLMAPIKey          string
    EmbeddingBaseURL   string  // defaults to LLMBaseURL
    EmbeddingModelName string  // defaults to LLMModelName
    DBPath             string
    VaultPersonalPath  string
    VaultWorkPath      string
    QdrantURL          string
    QdrantCollection   string
    QdrantVectorSize   int
    APIPort            string
}
func Load() (*Config, error) // reads from env vars with defaults
```

**Note:** `EmbeddingBaseURL` defaults to `LLMBaseURL` and `EmbeddingModelName` defaults to `LLMModelName`, allowing a single llama-server instance to handle both chat and embeddings.

### 0.23 Chunk Index Numbering
**Decision:** Chunk indices start at 0. First chunk (before any heading) is index 0. Each subsequent chunk increments by 1.

### 0.24 Empty Vaults Handling
**Decision:** If `Vaults` array in `AskRequest` is empty or not provided, search **all vaults**. If specific vaults are provided, only search those.

---

## 1. Final choices (locked in)

- **Language:** Go
- **UI:** Single embedded HTML page served by Go (no React)
- **Model runtime:** `llama.cpp` with OpenAI-compatible HTTP API
- **Vector DB:** Qdrant in Docker
- **Metadata DB:** SQLite
- **Vaults:** 2 vaults (personal + work), both read/write
- **Deployment:** runs on home server, accessed via Tailscale from Mac

---

## 2. Core architecture & package layout

**Current Structure:**
- `cmd/api/main.go` - HTTP server, wiring, static assets, database initialization
- `internal/config/config.go` - Configuration loading and validation
- `internal/http/router.go` - Chi router setup with dependency injection
- `internal/http/middleware.go` - CORS middleware
- `internal/handlers/chat.go` - HTTP handlers
- `internal/service/chat.go` - Service layer (business logic)
- `internal/llm/client.go` - LLM client implementation
- `internal/storage/database.go` - SQLite connection and migrations
- `internal/storage/models.go` - Vault, Note, Chunk models
- `internal/storage/vault_repo.go` - Vault repository
- `internal/storage/note_repo.go` - Note repository
- `internal/storage/chunk_repo.go` - Chunk repository

**Note:** The current code uses a service layer pattern (`internal/service`) which is good practice. The plan's interfaces can be integrated into this structure. The service layer can wrap the RAG engine and coordinate between handlers and domain logic.

Suggested repo layout (aligned with plan, integrating existing structure):

```text
cmd/
  api/
    main.go          # wiring, HTTP server, static assets ✅

internal/
  config/
    config.go        # env + configuration structs ✅

  http/
    router.go        # chi router & routes ✅
    middleware.go    # CORS middleware ✅

  llm/
    client.go        # Chat client
    embeddings.go    # Embeddings client
    types.go         # Message, ChatParams, etc.

  storage/
    database.go      # DB connection + migrations ✅
    vault_repo.go    # Vault repository ✅
    note_repo.go     # Note repository ✅
    chunk_repo.go    # Chunk repository ✅
    models.go        # Vault, Note, Chunk structs ✅

  vectorstore/
    interface.go     # VectorStore, Point, SearchResult
    qdrant.go        # Qdrant implementation

  vault/
    manager.go       # VaultManager (IDs, paths)
    scanner.go       # ScanAll for *.md
    // watcher.go    # (later)
    writer.go       # (later, for mutations)

  indexer/
    chunker.go      # ChunkMarkdown
    pipeline.go     # Indexer implementation
    types.go

  rag/
    engine.go       # Ask logic (RAGEngine)
    types.go        # AskRequest, AskResponse, Reference

  handlers/
    ask.go          # POST /api/v1/ask
    ui.go           # GET / (serve HTML)
```

You can adjust naming, but the boundaries are important.

---

## 3. Key interfaces (contracts for Cursor)

These are worth literally dropping into your codebase first so everything else composes cleanly.

### 3.1 LLM / embeddings

**Current State:** `internal/llm/client.go` has `Chat(ctx, message string) (string, error)` which works for simple chat. For RAG, we need structured messages with system prompts.

**Plan Interface:**

```go
// internal/llm/types.go

type Message struct {
    Role    string `json:"role"`
    Content string `json:"content"`
}

type ChatParams struct {
    Model       string
    MaxTokens   int
    Temperature float32
}

type LLMClient interface {
    Chat(ctx context.Context, messages []Message, params ChatParams) (string, error)
}

type EmbeddingsClient interface {
    EmbedTexts(ctx context.Context, texts []string) ([][]float32, error)
}
```

**Decision:** See Section 0.1 - Use Option A: extend existing client with `ChatWithMessages` method.

### 3.2 Vector store

```go
// internal/vectorstore/interface.go

type Point struct {
    ID   string
    Vec  []float32
    Meta map[string]any
}

type SearchResult struct {
    PointID string
    Score   float32
    Meta    map[string]any
}

type VectorStore interface {
    Upsert(ctx context.Context, collection string, points []Point) error
    Search(ctx context.Context, collection string, query []float32, k int, filters map[string]any) ([]SearchResult, error)
    Delete(ctx context.Context, collection string, ids []string) error
}
```

### 3.3 Chunker + indexer

```go
// internal/indexer/types.go

type Chunk struct {
    Index       int
    HeadingPath string
    Text        string
}

// internal/indexer/chunker.go

type Chunker interface {
    ChunkMarkdown(content []byte) (title string, chunks []Chunk, err error)
}

// internal/indexer/pipeline.go

type Indexer interface {
    IndexAll(ctx context.Context) error
    IndexNote(ctx context.Context, vaultID int, relPath string) error
}
```

### 3.4 RAG engine

```go
// internal/rag/types.go

type AskRequest struct {
    Question string   `json:"question"`
    Vaults   []string `json:"vaults,omitempty"`
    Folders  []string `json:"folders,omitempty"`
    K        int      `json:"k,omitempty"`
}

type Reference struct {
    Vault       string `json:"vault"`
    RelPath     string `json:"rel_path"`
    HeadingPath string `json:"heading_path"`
    ChunkIndex  int    `json:"chunk_index"`
}

type AskResponse struct {
    Answer     string      `json:"answer"`
    References []Reference `json:"references"`
}

// internal/rag/engine.go

type Engine interface {
    Ask(ctx context.Context, req AskRequest) (AskResponse, error)
}
```

---

## 4. Implementation plan / TODOs (in order)

### Phase 1 – Config, main, and HTTP skeleton

**Goal:** Service starts, static HTML served, routing ready.

**Status:** ✅ **COMPLETE** - Config package, router extraction, and main refactoring completed.

1. **Config**

   * [x] `internal/config/config.go`:

     * ✅ Implemented config structure per Section 0.22.
     * ✅ Reads env vars with defaults:
       * `LLM_BASE_URL` (default: `"http://localhost:8080"`)
       * `LLM_MODEL` (default: `"local-model"`) - Note: uses `LLM_MODEL` env var for backward compatibility
       * `LLM_API_KEY` (default: `"dummy-key"`)
       * `EMBEDDING_BASE_URL` (default: same as `LLM_BASE_URL`)
       * `EMBEDDING_MODEL_NAME` (default: same as `LLM_MODEL`)
       * `DB_PATH` (default: `"./data/helloworld-ai.db"`, see Section 0.17)
       * `VAULT_PERSONAL_PATH` (required)
       * `VAULT_WORK_PATH` (required)
       * `QDRANT_URL` (default: `"http://localhost:6333"`)
       * `QDRANT_COLLECTION` (default: `"notes"`, see Section 0.12)
       * `QDRANT_VECTOR_SIZE` (required, must match embedding model)
       * `API_PORT` (default: `"9000"`, see Section 0.18)
     * ✅ Creates `./data` directory if it doesn't exist (for DB).
     * ✅ Provides `func Load() (*Config, error)` that validates required fields.

2. **HTTP router**

   * [x] Router extracted to `internal/http/router.go`:
     * ✅ Uses **chi router** (`github.com/go-chi/chi/v5`) instead of standard library
     * ✅ Exposes `func NewRouter(deps *Deps) http.Handler` where `Deps` holds chat service, RAG engine (nil for now), and HTML content
     * ✅ Includes chi middleware: Logger and Recoverer
     * ✅ CORS middleware moved to `internal/http/middleware.go`
     * ✅ Routes: `POST /api/chat` and `GET /`

3. **Main**

   * [x] `cmd/api/main.go` refactored:
     * ✅ Loads config via config package
     * ✅ Creates router using `http.NewRouter()`
     * ✅ Serves on configured port
     * ✅ DB connection and migrations integrated (Phase 2)
     * ⏭️ Other client initialization deferred to later phases

4. **Static HTML UI**

   * [x] HTML embedded via `//go:embed index.html` in `main.go`
   * [x] Route `GET /` serves HTML ✅
   * [x] Basic chat UI with streaming ✅
   * ⏭️ **Deferred to Phase 7:** Update HTML for RAG:
     * Add vault selector (personal/work checkboxes).
     * Change endpoint from `/api/chat` to `/api/v1/ask` (or add new endpoint).
     * Add references display section.
     * Update JS to send `AskRequest` format and render `AskResponse` with references.

---

### Phase 2 – SQLite storage / repositories

**Goal:** Have vaults, notes, chunks in a DB with clean repo APIs.

**Status:** ✅ **COMPLETE** - Database connection, migrations, models, and all repositories implemented and integrated.

1. **DB connection & migrations**

   * [x] `internal/storage/database.go`:

     * ✅ `func New(path string) (*sql.DB, error)` - Opens SQLite DB, enables foreign keys, sets connection pool
     * ✅ `func Migrate(db *sql.DB) error` - Creates tables with schema:

       ```sql
       CREATE TABLE IF NOT EXISTS vaults (
           id INTEGER PRIMARY KEY AUTOINCREMENT,
           name TEXT NOT NULL UNIQUE,
           root_path TEXT NOT NULL,
           created_at DATETIME DEFAULT CURRENT_TIMESTAMP
       );

       CREATE TABLE IF NOT EXISTS notes (
           id TEXT PRIMARY KEY,         -- UUID
           vault_id INTEGER NOT NULL,
           rel_path TEXT NOT NULL,
           folder TEXT NOT NULL,
           title TEXT,
           updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
           hash TEXT NOT NULL,
           FOREIGN KEY (vault_id) REFERENCES vaults(id),
           UNIQUE (vault_id, rel_path)
       );

       CREATE TABLE IF NOT EXISTS chunks (
           id TEXT PRIMARY KEY,         -- UUID = Qdrant point ID
           note_id TEXT NOT NULL,
           chunk_index INTEGER NOT NULL,
           heading_path TEXT,
           text TEXT NOT NULL,
           FOREIGN KEY (note_id) REFERENCES notes(id) ON DELETE CASCADE
       );
       ```

2. **Models & repos**

   * [x] `internal/storage/models.go`: ✅ Define structs:
     * ✅ `Vault` with fields: `ID int`, `Name string`, `RootPath string`, `CreatedAt time.Time`
     * ✅ `Note` with fields: `ID string` (UUID), `VaultID int`, `RelPath string`, `Folder string`, `Title string`, `UpdatedAt time.Time`, `Hash string`
     * ✅ `Chunk` with fields: `ID string` (UUID), `NoteID string`, `ChunkIndex int`, `HeadingPath string`, `Text string`
   * [x] `internal/storage/vault_repo.go`: ✅
     * ✅ `GetOrCreateByName(name, rootPath) (Vault, error)`
     * ✅ `ListAll() ([]Vault, error)`
   * [x] `internal/storage/note_repo.go`: ✅
     * ✅ `GetByVaultAndPath(vaultID int, relPath string) (*Note, error)`
     * ✅ `Upsert(note *Note) error` - Uses `INSERT ... ON CONFLICT` for atomic upserts
   * [x] `internal/storage/chunk_repo.go`: ✅
     * ✅ `Insert(chunk *Chunk) error`
     * ✅ `DeleteByNote(noteID string) error`
     * ✅ `ListIDsByNote(noteID string) ([]string, error)`

3. **Integration**

   * [x] `cmd/api/main.go`: ✅
     * ✅ Database initialization (`storage.New`, `storage.Migrate`)
     * ✅ Repository instances created (vaultRepo, noteRepo, chunkRepo)
     * ✅ Ready for Phase 5 (vault manager integration)

---

### Phase 3 – LLM + embeddings clients

**Goal:** Robust clients for llama.cpp chat and embeddings.

**Status:** ✅ Chat client exists, but needs interface alignment. ❌ Embeddings client missing.

1. **Chat client**

   * [x] `internal/llm/client.go` exists with basic chat functionality ✅
   * [ ] Align with plan interface:

     * Current: `Chat(ctx, message string) (string, error)` - simple string-based
     * Plan: `Chat(ctx, messages []Message, params ChatParams) (string, error)` - structured
     * Options:
       * A) Extend existing client to support both (add new method)
       * B) Refactor to match plan interface exactly
       * C) Keep current for simple chat, add new method for RAG use case
   * [x] Uses `LLM_BASE_URL` and `LLM_MODEL` ✅
   * [x] Implements OpenAI-style `/v1/chat/completions` ✅
   * [x] Supports streaming ✅

   **Note:** Current implementation works but uses simpler interface. RAG engine will need structured messages with system prompts, so consider adding support for `[]Message` format.

2. **Embeddings client**

   * [ ] `internal/llm/embeddings.go`:

     * Implements `EmbeddingsClient` interface from plan:
       ```go
       type EmbeddingsClient interface {
           EmbedTexts(ctx context.Context, texts []string) ([][]float32, error)
       }
       ```
     * Uses `/v1/embeddings` endpoint (OpenAI-compatible)
     * Uses `EMBEDDING_MODEL_NAME` from config

---

### Phase 4 – Qdrant vector store

**Goal:** Working `VectorStore` backed by Qdrant.

1. **Qdrant client**

   * [ ] `internal/vectorstore/qdrant.go`:

     * Use `github.com/qdrant/go-client` (see Section 0.3).
     * Implement:

       * `Upsert` → upsert points.
       * `Search` → vector search with payload filters.
       * `Delete` → delete by IDs.

2. **Collection init**

   * [ ] On startup, ensure `QDRANT_COLLECTION` exists (default: `"notes"`, see Section 0.12).
   * [ ] Create collection if missing with:
     * Vector size from `QDRANT_VECTOR_SIZE` config (see Section 0.13).
     * Distance metric: `Cosine`.
     * Validate embedding client returns vectors of matching size.

---

### Phase 5 – Vault manager + scanner

**Goal:** Enumerate `.md` notes across both vaults with vault IDs/paths.

1. **Vault manager**

   * [ ] `internal/vault/manager.go`:

     * Holds DB + config.
     * On init:

       * `GetOrCreateByName("personal", VAULT_PERSONAL_PATH)`
       * `GetOrCreateByName("work", VAULT_WORK_PATH)`
     * Provides:

       * `VaultByName(name string) (storage.Vault, error)`
       * `AbsPath(vaultID int, relPath string) string`

2. **Scanner**

   * [ ] `internal/vault/scanner.go`:

     ```go
     type ScannedFile struct {
         VaultID int
         RelPath string
         Folder  string
         AbsPath string
     }

     func (m *Manager) ScanAll(ctx context.Context) ([]ScannedFile, error)
     ```

   * Walk each vault root, find `*.md` files.
   * Compute `relPath` relative to vault root.
   * Compute `folder` per Section 0.6 (path components except filename).

---

### Phase 6 – Chunker + indexing pipeline

**Goal:** Index both vaults into SQLite + Qdrant (one-time per start).

1. **Chunker**

   * [ ] `internal/indexer/chunker.go`:

     * Use `github.com/yuin/goldmark` with AST parsing (see Section 0.4).
     * Implement chunking strategy per Section 0.5:
       * Chunk by heading hierarchy
       * Min 50 chars, max 2000 chars per chunk
       * Heading path format: `"# Heading1 > ## Heading2"`
     * Extract title per Section 0.7 (first # heading, or filename).
     * Return title and list of `Chunk`.

2. **Indexer implementation**

   * [ ] `internal/indexer/pipeline.go`:

     * `Pipeline` struct holds:

       * `vaultManager`, `noteRepo`, `chunkRepo`, `embedder`, `vectorStore`, `collection`, `chunker`.

     * `IndexNote(ctx, vaultID, relPath)`:

       * Compute `absPath` via `vaultManager.AbsPath`.
       * Read file.
       * Hash content using SHA256 (see Section 0.8).
       * `noteRepo.GetByVaultAndPath`.
       * If hash unchanged → return (skip re-indexing).
       * Chunk using `chunker` (per Section 0.5).
       * Calculate `folder` from `relPath` (see Section 0.6).
       * Generate `noteID` using `uuid.New()` (see Section 0.2) if new.
       * `noteRepo.Upsert`.
       * For existing note:
         * `chunkRepo.ListIDsByNote` → `vectorStore.Delete` + `chunkRepo.DeleteByNote`.
       * `embedder.EmbedTexts` for chunk texts (validate vector size matches config).
       * For each chunk (index starting at 0, see Section 0.23):
         * Generate `chunkID` using `uuid.New()`.
         * `chunkRepo.Insert`.
         * Prepare `vectorstore.Point` with `ID=chunkID` and metadata per Section 0.20.
       * `vectorStore.Upsert` (batch all chunks for efficiency).

     * `IndexAll(ctx)`:

       * `vaultManager.ScanAll`.
       * Loop and call `IndexNote` for each file.
       * On error for a file, log and continue (see Section 0.19).

3. **Main wiring**

   * [ ] In `main.go`, after constructing `Pipeline`, call `indexer.IndexAll(ctx)` once at startup.

At this point, your notes are indexed.

---

### Phase 7 – RAG engine + `/api/v1/ask` + HTML wiring

**Goal:** Ask questions about notes via UI and get answers with references.

**Status:** ❌ Not started - Current `/api/chat` is basic LLM chat, not RAG.

1. **RAG engine**

   * [ ] `internal/rag/engine.go`:

     * `RAGEngine` struct with `embedder`, `vectorStore`, `collection`, `noteRepo`, `llm`, `vaultRepo`.
     * `Ask(ctx, req AskRequest)`:

       * Embed `req.Question` using `EmbeddingsClient`.
       * Build filters:
         * If `Vaults` empty/not provided → search all vaults (see Section 0.24).
         * Convert `Vaults` names to IDs (via `vaultRepo`).
         * Add folder filters if provided (prefix matching per Section 0.14).
       * Default `K` to 5 if zero (see Section 0.9), max 20.
       * `vectorStore.Search` with query vector and filters.
       * Build context string per Section 0.11 format.
       * Construct LLM messages:
         * System prompt: use exact prompt from Section 0.10.
         * User message: include question + formatted context.
       * Call `llmClient.ChatWithMessages` (see Section 0.1) with messages and default params.
       * Build `References` from search results metadata (extract from Qdrant point metadata).
       * Return `AskResponse` (non-streaming per Section 0.16).

2. **HTTP handler**

   * [ ] `internal/handlers/ask.go`:

     * Similar structure to existing `chat.go` handler.
     * Parse JSON `AskRequest`.
     * Validate:
       * Default `K` to 5 if zero (see Section 0.9).
       * Validate vault names exist (if provided).
       * Return HTTP 400 on validation errors (see Section 0.19).
     * Call `ragEngine.Ask`.
     * Return JSON `AskResponse` (non-streaming per Section 0.16).
     * Handle errors per Section 0.19.

   * [ ] Wire route `POST /api/v1/ask` in router.

   **Note:** Can keep `/api/chat` for basic chat and add `/api/v1/ask` for RAG, or migrate chat to use RAG engine with empty vaults.

3. **HTML / JS**

   * [ ] Update `cmd/api/index.html`:

     * Add vault selector UI:
       * Checkboxes for "personal" and "work" vaults.
       * Optional folder filter input.
     * Update `sendMessage()` function:
       * Change endpoint to `/api/v1/ask` (or add toggle for chat vs RAG mode).
       * Update request body to `AskRequest` format:
         ```json
         { "question": "...", "vaults": ["personal"], "k": 5 }
         ```
       * Handle `AskResponse` format with `answer` and `references`.
     * Add references display:
       * Render `references` array as clickable list.
       * Format: `vault/rel_path :: heading_path` (or similar).
       * Consider linking to note files if possible.

   **Note:** RAG responses are non-streaming per Section 0.16. Current streaming UI can be adapted or kept separate for `/api/chat`.
