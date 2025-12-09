# Service Layer - Agent Guide

Business logic and domain patterns.

## Core Responsibilities

- Business rules and validation
- Domain error definitions
- Orchestrate storage and external services
- Domain model definitions

## Service Pattern

The service layer primarily provides domain error definitions and validation patterns. Business logic for RAG queries is handled by the RAG engine (`internal/rag`), which uses the LLM client directly.

## Domain Errors

Define in `errors.go`:

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
```

## Business Validation

```go
if req.Message == "" {
    return ChatResponse{}, &ValidationError{
        Field:   "message",
        Message: "cannot be empty",
    }
}
```

**Note:** The service layer's validation patterns are used as examples. RAG validation is handled in the handlers layer (AskHandler).

## Consumer-First Interfaces

Service layer defines what it needs:

```go
// Service defines interface
type LLMClient interface {
    Chat(ctx context.Context, message string) (string, error)
}

// External layer implements it
type Client struct { /* ... */ }
func (c *Client) Chat(ctx context.Context, message string) (string, error) { /* ... */ }
```

## Data Transformations

Convert between storage and domain models:

```go
// Storage → Domain
user := &User{
    ID:   record.ID,
    Name: record.Name,
}

// Domain → Storage
record := &storage.UserRecord{
    ID:   user.ID,
    Name: user.Name,
}
```

## Testing

### Mock Generation

Interfaces have `//go:generate` directives for automatic mock generation:

```go
//go:generate go run go.uber.org/mock/mockgen@latest -destination=mocks/mock_llm_client.go -package=mocks helloworld-ai/internal/service LLMClient

type LLMClient interface {
    Chat(ctx context.Context, message string) (string, error)
    StreamChat(ctx context.Context, message string, callback func(chunk string) error) error
}
```

**Note:** The LLMClient interface is defined here for reference, but it's primarily used by the RAG engine. The service layer focuses on domain error definitions.

### Test Patterns

**External Test Package:**

Tests use `package service_test` to avoid import cycles:

```go
package service_test

import (
    "helloworld-ai/internal/service"
    "helloworld-ai/internal/service/mocks"
    "go.uber.org/mock/gomock"
)
```

**Log Suppression:**

Suppress log output during tests:

```go
func init() {
    slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
}
```

**Mock Usage:**

```go
ctrl := gomock.NewController(t)
defer ctrl.Finish()

mockLLMClient := mocks.NewMockLLMClient(ctrl)
mockLLMClient.EXPECT().Chat(gomock.Any(), "test").Return("response", nil)

// Use mock in tests as needed
```

## Rules

- Protocol-agnostic - No HTTP/gRPC types
- Define interfaces from consumer perspective
- Wrap external errors with context
- Log business events
- Use `//go:generate` directives for mock generation
- Tests use external test packages to import mocks
