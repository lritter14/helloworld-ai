# HelloWorld AI - Agent Guide

This document outlines core design principles and architectural patterns for building consistent, maintainable, and scalable Go services. These principles ensure code quality, promote reusability, and facilitate team collaboration.

**Remember:** These are guidelines, not rigid rules. Use judgment when applying them.

## Project Overview

HelloWorld AI is a Go-based API server with an embedded web UI for interacting with local LLMs via llama.cpp. The project implements a RAG (Retrieval-Augmented Generation) system that indexes markdown notes from vaults and enables question-answering over the indexed content.

### Technology Stack

- **Language:** Go 1.25.3+
- **UI:** Single embedded HTML page served by Go
- **Model Runtime:** llama.cpp with OpenAI-compatible HTTP API
- **Vector DB:** Qdrant (Docker)
- **Metadata DB:** SQLite
- **Vaults:** 2 vaults (personal + work)

## 1. Layered Architecture

Services follow a distinct layered architecture pattern that promotes separation of concerns and maintainability.

### 1.1 Storage Layer (`internal/storage`)

**Purpose:** Handle all database and external data source interactions.

**Responsibilities:**

- Database connection management
- Data persistence operations (CRUD)
- Query execution and result mapping
- Transaction management

**Guidelines:**

- Keep business logic out of this layer
- Use repository pattern for data access abstraction
- Implement proper error handling and logging
- Use interfaces to abstract different storage backends

### 1.2 Service Layer (`internal/service`)

**Purpose:** Contain all business logic and orchestrate operations between layers.

**Responsibilities:**

- Business rule implementation
- Data validation and transformation
- Workflow orchestration
- Integration with external services

**Guidelines:**

- Keep this layer protocol-agnostic (no HTTP/gRPC types)
- Implement comprehensive business logic validation
- Use dependency injection for testability (using interfaces)
- Maintain clear boundaries with other layers

### 1.3 Ingress Layer (`internal/handlers`)

**Purpose:** Handle protocol-specific communication and translate between external APIs and internal service calls.

**Responsibilities:**

- HTTP endpoint handling
- Request/response marshaling
- Protocol-specific error handling
- Input sanitization and validation

**Guidelines:**

- Keep business logic minimal in this layer
- Focus on protocol translation
- Implement proper middleware for cross-cutting concerns
- Ensure consistent error response formatting

### Layer Rules

- **Handlers MUST NOT contain business logic** - All business logic belongs in the service layer
- **Services MUST NOT know about HTTP** - Services work with domain models, not HTTP requests/responses
- **Services define interfaces** - Service layer defines what it needs (e.g., `LLMClient`), external layer implements it
- **Storage uses repository pattern** - Each entity has a repository with an interface (e.g., `NoteStore`)
- **Dependencies flow inward** - Outer layers depend on inner layers, not vice versa

## 2. Consumer Interface Model

Design interfaces from the consumer's perspective to create more intuitive and maintainable APIs.

### 2.1 Interface Design Principles

- **Consumer-Centric:** Define interfaces based on what consumers need, not what providers implement
- **Minimal and Focused:** Keep interfaces small and focused on specific responsibilities
- **Stable Contracts:** Ensure interfaces provide stable contracts that don't frequently change

### 2.2 Implementation Guidelines

**Good:** Consumer-focused interface

```go
// Service layer defines what it needs
type LLMClient interface {
    Chat(ctx context.Context, message string) (string, error)
    StreamChat(ctx context.Context, message string, callback func(chunk string) error) error
}
```

**Avoid:** Provider-centric interface

```go
// Don't design interfaces around implementation details
type LLMRepository interface {
    ExecuteQuery(query string, args ...interface{}) (*sql.Rows, error)
    BeginTransaction() (*sql.Tx, error)
    // Too low-level and implementation-specific
}
```

### 2.3 Interface Placement

- Place interfaces in the consuming package, not the implementing package
- This allows consumers to define exactly what they need
- Reduces coupling between packages

## 3. Context Usage for Cross-Cutting Concerns

Use `context.Context` consistently for handling metrics, tracing, logging, feature flags, and metadata throughout the application.

### 3.1 Context Patterns

**Always pass context as first parameter:**

```go
func (s *userService) CreateUser(ctx context.Context, req CreateUserRequest) (*User, error) {
    // Business logic here
}
```

**Structured Logging:**

```go
func (s *service) ProcessChat(ctx context.Context, req ChatRequest) (ChatResponse, error) {
    logger := s.getLogger(ctx)
    logger.InfoContext(ctx, "chat request processed", "message_length", len(req.Message))
    // Business logic
}
```

### 3.2 Context Best Practices

- Always pass context as the first parameter
- Never store context in structs; pass it through function calls
- Use `context.WithValue` sparingly and only for request-scoped data
- Create typed context keys to avoid collisions
- Database operations use context: `QueryRowContext`, `ExecContext`, etc.

## 4. Data Structure Locality

Define data structures close to their usage instead of creating shared structures across layers. This promotes loose coupling, clearer boundaries, and easier maintenance.

### 4.1 Layer-Specific Data Structures

**Storage layer:** Define structures that closely map to database schema

```go
// internal/storage/models.go
type NoteRecord struct {
    ID        string    `db:"id"`
    VaultID   int       `db:"vault_id"`
    RelPath   string    `db:"rel_path"`
    Title     string    `db:"title"`
    UpdatedAt time.Time `db:"updated_at"`
}
```

**Service layer:** Define structures that represent business domain

```go
// internal/service/chat.go
type ChatRequest struct {
    Message string `validate:"required"`
}

type ChatResponse struct {
    Reply string
}
```

**Ingress layer:** Define structures optimized for HTTP API

```go
// internal/handlers/chat.go
type ChatRequest struct {
    Message string `json:"message"`
}

type ChatResponse struct {
    Reply string `json:"reply"`
}
```

### 4.2 Data Transformation Between Layers

Each layer is responsible for transforming data to and from its neighboring layers:

```go
// Handler: HTTP → Service
svcReq := service.ChatRequest{
    Message: req.Message,
}

// Service: Service → Storage
record := &storage.NoteRecord{
    ID:      note.ID,
    Title:   note.Title,
}
```

### 4.3 Guidelines Summary

- Define structures where they're used - Each layer defines its own data structures
- Transform at layer boundaries - Convert between layer-specific structures
- Avoid shared model packages - Resist the temptation to create shared structure packages
- Test transformations - Ensure data transformations between layers are well-tested

## 5. Error Handling

### 5.1 Error Types

Use structured errors with proper error wrapping:

```go
var (
    ErrInvalidInput    = errors.New("invalid input")
    ErrNotFound        = errors.New("not found")
    ErrExternalService = errors.New("external service error")
)

type ValidationError struct {
    Field   string
    Message string
}

func WrapError(err error, msg string) error {
    if err == nil {
        return nil
    }
    return fmt.Errorf("%s: %w", msg, err)
}
```

### 5.2 Error Handling Rules

- Use structured error types and wrap errors with context
- Log errors in the appropriate layer (usually service layer)
- In ingress, map errors to consistent HTTP status codes
- Check errors explicitly using `errors.Is()` and `errors.As()`
- Storage layer returns `ErrNotFound`, not `sql.ErrNoRows` directly

## 6. Logging

### 6.1 Logging Guidelines

- Use structured logging with `slog` and key-value pairs
- Extract logger from context when available, fallback to default logger
- Log levels:
  - `Error` - Errors that require attention
  - `Warn` - Warnings (e.g., invalid input)
  - `Info` - Important events
- Include relevant context fields (e.g., `message_length`, `error`)

## 7. Testing Strategy

### 7.1 Testing Guidelines

- Unit test each layer independently
- Use dependency injection to facilitate testing (mock dependencies using interfaces)
- Test error cases and edge cases
- Use `context.Background()` or `context.WithTimeout()` in tests
- Add integration tests for critical end-to-end paths
- Test data transformations between layers explicitly

## 8. Configuration Management

### 8.1 Configuration Guidelines

- Use environment variables for configuration
- Validate required fields at startup
- Provide sensible defaults for optional configuration
- Handle type conversion with proper error handling
- Create necessary directories (e.g., data directory)

## 9. Dependency Management

### 9.1 Dependency Guidelines

- Use Go modules for dependency management
- Keep dependencies minimal and up-to-date
- Pin dependency versions for reproducibility in production
- Regularly audit dependencies for security vulnerabilities

## 10. Naming Conventions

### 10.1 Package Names

- Lowercase - All package names are lowercase
- Singular preferred - Use singular names when possible

### 10.2 Type Names

- Exported types - PascalCase (e.g., `ChatHandler`, `ChatService`)
- Private types - camelCase (e.g., `chatService`)
- Interfaces - No `I` prefix, just descriptive name (e.g., `ChatService`)
- Database models - Use `*Record` suffix (e.g., `NoteRecord`)

### 10.3 Function Names

- Constructors - `New*` prefix (e.g., `NewChatHandler`)
- Methods - Descriptive verbs (e.g., `ProcessChat`)
- Private functions - camelCase (e.g., `getLogger`)

## 11. Code Organization

### 11.1 File Organization

- One main type per file - Each file typically contains one main type and its methods
- Related types together - Related types (e.g., request/response DTOs) can be in same file
- Error definitions - Domain errors in `errors.go` within the package

### 11.2 Dependency Injection

- Use constructor functions - All types have `New*` constructor functions
- Dependencies via structs - Group related dependencies in structs (e.g., `http.Deps`)
- Interfaces for dependencies - Services depend on interfaces, not concrete types
- Wire dependencies in main - All dependency wiring happens in `cmd/api/main.go`

## 12. General Go Best Practices

- Use `go:embed` for embedding static files (e.g., HTML) in binary
- Defer cleanup - Always defer resource cleanup (e.g., `defer db.Close()`)
- Handle errors - Never ignore errors, always handle or explicitly ignore with `_`
- Use `_` for unused imports - Use blank identifier for side-effect imports
- Document exported symbols - Add godoc comments for exported types and functions
- Keep functions focused - Functions should do one thing well
- Avoid global state - Pass dependencies explicitly

## Project-Specific Details

### Key Decisions

- **UUID Generation:** Use `github.com/google/uuid` package, store as strings
- **Qdrant Client:** Use `github.com/qdrant/go-client` (official Go client)
- **Chunker:** Use `github.com/yuin/goldmark` with `goldmark/ast` for markdown parsing
- **Chunking Strategy:** Chunk by heading hierarchy, min 50 chars, max 2000 chars
- **Default K Value:** Default `K = 5` chunks for RAG queries, max `K = 20`

### Environment Variables

**Required:**

- `VAULT_PERSONAL_PATH` - Path to personal vault directory
- `VAULT_WORK_PATH` - Path to work vault directory
- `QDRANT_VECTOR_SIZE` - Vector size for embeddings (must be > 0)

**Optional (with defaults):**

- `LLM_BASE_URL` - Base URL for llama.cpp server (default: `http://localhost:8080`)
- `LLM_API_KEY` - API key (default: `dummy-key`)
- `LLM_MODEL` - Model name (default: `local-model`)
- `DB_PATH` - Path to SQLite database (default: `./data/helloworld-ai.db`)
- `API_PORT` - Port for API server (default: `9000`)

## Layer-Specific Documentation

For detailed patterns specific to each layer, see:

- **Handlers:** `internal/handlers/AGENTS.md` - HTTP handler patterns, DTOs, streaming
- **Service:** `internal/service/AGENTS.md` - Business logic, domain errors, validation
- **Storage:** `internal/storage/AGENTS.md` - Repository pattern, database operations
- **LLM:** `internal/llm/AGENTS.md` - External service client patterns
- **HTTP:** `internal/http/AGENTS.md` - Middleware, router setup
- **Config:** `internal/config/AGENTS.md` - Configuration patterns
