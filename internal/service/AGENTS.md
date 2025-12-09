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

## Testing

### Mock Generation

Interfaces have `//go:generate` directives for automatic mock generation:

```go
//go:generate go run go.uber.org/mock/mockgen@latest -destination=mocks/mock_llm_client.go -package=mocks helloworld-ai/internal/service LLMClient
//go:generate go run go.uber.org/mock/mockgen@latest -destination=mocks/mock_chat_service.go -package=mocks -mock_names=ChatService=MockChatService helloworld-ai/internal/service ChatService

type LLMClient interface {
    Chat(ctx context.Context, message string) (string, error)
    StreamChat(ctx context.Context, message string, callback func(chunk string) error) error
}
```

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

svc := service.NewChatService(mockLLMClient)
```

## Rules

- Protocol-agnostic - No HTTP/gRPC types
- Define interfaces from consumer perspective
- Wrap external errors with context
- Log business events
- Use `//go:generate` directives for mock generation
- Tests use external test packages to import mocks
