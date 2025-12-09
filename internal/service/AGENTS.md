# Service Layer - Agent Guide

Business logic and domain patterns.

## Core Responsibilities

- Business rules and validation
- Domain error definitions
- Orchestrate storage and external services
- Domain model definitions

## Service Pattern

```go
type ChatService interface {
    ProcessChat(ctx context.Context, req ChatRequest) (ChatResponse, error)
}

type chatService struct {
    llmClient LLMClient
    logger    *slog.Logger
}

func NewChatService(llmClient LLMClient) ChatService {
    return &chatService{
        llmClient: llmClient,
        logger:    slog.Default(),
    }
}
```

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

## Rules

- Protocol-agnostic - No HTTP/gRPC types
- Define interfaces from consumer perspective
- Wrap external errors with context
- Log business events
