# Handlers Layer - Agent Guide

HTTP request/response handling patterns for the ingress layer.

## Core Responsibilities

- HTTP-specific concerns (status codes, headers, JSON encoding/decoding)
- Convert HTTP requests to service requests
- Convert service responses to HTTP responses
- Map service errors to HTTP status codes

## Handler Pattern

```go
type AskHandler struct {
    ragEngine rag.Engine
    vaultRepo storage.VaultStore
    logger    *slog.Logger
}

type IndexHandler struct {
    indexerPipeline *indexer.Pipeline
    logger          *slog.Logger
}

func (h *AskHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    logger := h.getLogger(ctx)
    
    // Validate method, decode request, call RAG engine, encode response
}
```

## Request/Response DTOs

Define separate DTOs in handler package:

```go
type AskRequest struct {
    Question string   `json:"question"`
    Vaults   []string `json:"vaults,omitempty"`
    Folders  []string `json:"folders,omitempty"`
    K        int      `json:"k,omitempty"`
    Detail   string   `json:"detail,omitempty"` // "brief", "normal", "detailed"
}

type AskResponse struct {
    Answer        string              `json:"answer"`
    References    []ReferenceResponse `json:"references"`
    Abstained     bool                `json:"abstained,omitempty"`
    AbstainReason string              `json:"abstain_reason,omitempty"`
    Debug         *DebugInfo          `json:"debug,omitempty"`
}
```

## Swagger Documentation

All API endpoints must be documented using go-swagger annotations. The specification is generated from code and served at `/api/docs/swagger.json`.

### Route Documentation

Document each handler's `ServeHTTP` method with `swagger:route` annotation:

```go
// ServeHTTP handles HTTP requests for RAG queries.
//
// Ask a question to the RAG system and get an answer based on indexed markdown notes.
// The system will search for relevant chunks across the specified vaults and folders,
// then generate an answer using the retrieved context.
//
// swagger:route POST /api/v1/ask askQuestion
//
// Ask a question using RAG
//
// Queries the RAG system with a question and optional filters for vaults and folders.
// Returns an answer generated from relevant indexed content along with source references.
//
// ---
// consumes:
// - application/json
// produces:
// - application/json
// responses:
//   '200':
//     description: Successful response with answer and references
//     schema:
//       "$ref": "#/definitions/askResponse"
//   '400':
//     description: Bad request (invalid question or vault name)
//     schema:
//       "$ref": "#/definitions/errorResponse"
//   '500':
//     description: Internal server error
//     schema:
//       "$ref": "#/definitions/errorResponse"
func (h *AskHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    // Handler implementation
}
```

### Parameter Documentation

Document request parameters using separate parameter structs with `swagger:parameters`:

```go
// Request struct (used in code)
type AskRequest struct {
    Question string   `json:"question"`
    Vaults   []string `json:"vaults,omitempty"`
    Folders  []string `json:"folders,omitempty"`
    K        int      `json:"k,omitempty"`
    Detail   string   `json:"detail,omitempty"`
}

// Parameter documentation (for Swagger)
// swagger:parameters askQuestion
//nolint:unused // Used by go-swagger for code generation
type askQuestionParams struct {
    // in: body
    // required: true
    Body AskRequest
}

// Query parameters (for GET endpoints or query params)
// swagger:parameters triggerIndex
//nolint:unused // Used by go-swagger for code generation
type triggerIndexParams struct {
    // If true, clears all existing indexed data before re-indexing
    //
    // in: query
    // type: boolean
    // default: false
    Force bool `json:"force"`
}
```

### Response Documentation

Document responses using separate response wrapper structs with `swagger:response`:

```go
// Response struct (used in code)
type AskResponse struct {
    Answer        string              `json:"answer"`
    References    []ReferenceResponse `json:"references"`
    Abstained     bool                `json:"abstained,omitempty"`
    AbstainReason string              `json:"abstain_reason,omitempty"`
    Debug         *DebugInfo          `json:"debug,omitempty"`
}

// Response documentation (for Swagger)
// swagger:response askResponse
//nolint:unused // Used by go-swagger for code generation
type askResponseWrapper struct {
    // in: body
    Body AskResponse
}

// Error response
type ErrorResponse struct {
    Error string `json:"error"`
}

// swagger:response errorResponse
//nolint:unused // Used by go-swagger for code generation
type errorResponseWrapper struct {
    // in: body
    Body ErrorResponse
}
```

### Field Documentation

Add descriptions to struct fields for better documentation:

```go
type ReferenceResponse struct {
    // Name of the vault containing the source
    Vault string `json:"vault"`

    // Relative path to the markdown file within the vault
    RelPath string `json:"rel_path"`

    // Heading path within the document (e.g., "H1 > H2 > H3")
    HeadingPath string `json:"heading_path"`

    // Index of the chunk within the document
    ChunkIndex int `json:"chunk_index"`
}
```

### Generating Swagger Spec

The Swagger specification is generated automatically during `make build-api` or manually with:

```bash
make generate-swagger
```

This requires the `swagger` CLI tool installed via:

```bash
go install github.com/go-swagger/go-swagger/cmd/swagger@latest
```

The generated spec is written to `cmd/api/swagger.json` and served by the API at `/api/docs/swagger.json`.

Convert at boundaries:

```go
// HTTP → RAG
ragReq := rag.AskRequest{
    Question: req.Question,
    Vaults:   req.Vaults,
    Folders:  req.Folders,
    K:        req.K,
}

// RAG → HTTP
resp := AskResponse{
    Answer:     ragResp.Answer,
    References: convertReferences(ragResp.References),
}
```

## Error Mapping

Map service errors to HTTP status codes:

```go
if errors.Is(err, service.ErrNotFound) {
    h.writeError(w, http.StatusNotFound, "Resource not found")
    return
}
if errors.Is(err, service.ErrExternalService) {
    h.writeError(w, http.StatusBadGateway, "External service error")
    return
}
```

## Index Handler

The `IndexHandler` handles re-indexing requests via `/api/index`:

```go
func (h *IndexHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    // Check for force parameter (?force=true)
    // Trigger indexing in goroutine (non-blocking)
    // Return HTTP 202 Accepted immediately
}
```

**Behavior:**

- Runs indexing asynchronously in a goroutine
- Returns HTTP 202 Accepted immediately
- Supports `?force=true` query parameter to clear existing data first

## Testing

### Mock Generation

The service interface has a `//go:generate` directive for mock generation (in service package).

### Test Patterns

**Mock Usage:**

```go
ctrl := gomock.NewController(t)
defer ctrl.Finish()

mockRAGEngine := mocks.NewMockEngine(ctrl)
mockVaultRepo := mocks.NewMockVaultStore(ctrl)

handler := NewAskHandler(mockRAGEngine, mockVaultRepo)
```

**HTTP Testing:**

```go
req := httptest.NewRequest(http.MethodPost, "/api/v1/ask", bytes.NewBuffer(body))
w := httptest.NewRecorder()

handler.ServeHTTP(w, req)

if w.Code != http.StatusOK {
    t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
}
```

**Error Handling:**

Properly handle error returns from HTTP operations:

```go
_, _ = fmt.Fprintf(w, "data: %s\n\n", chunk) // Ignore error in streaming
_, _ = w.Write([]byte(response)) // Ignore error in test scenarios
```

## RAG Handler (AskHandler)

The `AskHandler` handles RAG queries via `/api/v1/ask`:

```go
func (h *AskHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    // Parse AskRequest JSON
    // Validate: question required, K defaults to 5, max 20
    // Validate vault names exist (if provided)
    // Call ragEngine.Ask()
    // Return AskResponse JSON
}
```

**Error Mapping:**

- HTTP 400: Validation errors (empty question, invalid vaults, K > 20)
- HTTP 500: RAG engine errors
- HTTP 502: LLM/embedding errors
- HTTP 503: Vector store errors

**Validation:**

- Question required (non-empty)
- K defaults to 5 if zero, max 20
- Vault names validated against vaultRepo

**Debug Mode:**

- Enable via `?debug=true` or `?debug=1` query parameter
- When enabled, response includes detailed retrieval information:

  - All retrieved chunks with scores (vector, lexical, final) and ranks
  - Folder selection information (selected and available folders)
  - Chunk metadata (ID, rel_path, heading_path, text)

- Useful for evaluation frameworks and debugging retrieval quality

**Abstention:**

- `abstained` field indicates when the system explicitly abstains from answering
- `abstain_reason` provides the reason (e.g., "no_relevant_context", "ambiguous_question", "insufficient_information")
- Set to `true` when no relevant chunks are found or when retrieval fails
- Critical for evaluation frameworks to distinguish between "no answer found" and "answer generated"

## Rules

- NO business logic - Delegate to service/RAG layer immediately
- Set Content-Type header
- Extract logger from context
- Validate HTTP method if needed
- Validate vault names at ingress layer (AskHandler)
- Handle all error returns (use `_` for intentional ignores in streaming)
- **Document all endpoints** with Swagger annotations (`swagger:route`, `swagger:parameters`, `swagger:response`)
- Use `//nolint:unused` for Swagger-only wrapper types (they're used by code generation, not runtime)
- Add descriptive comments to all request/response struct fields
