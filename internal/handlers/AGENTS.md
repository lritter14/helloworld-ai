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
}

type AskResponse struct {
    Answer     string              `json:"answer"`
    References []ReferenceResponse `json:"references"`
}
```

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

## Rules

- NO business logic - Delegate to service/RAG layer immediately
- Set Content-Type header
- Extract logger from context
- Validate HTTP method if needed
- Validate vault names at ingress layer (AskHandler)
- Handle all error returns (use `_` for intentional ignores in streaming)
