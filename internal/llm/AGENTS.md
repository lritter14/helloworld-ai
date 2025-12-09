# LLM Client Layer - Agent Guide

External service client patterns.

## Core Responsibilities

- Communicate with external LLM services (chat completions)
- Generate embeddings for text (embeddings API)
- Implement interfaces defined by service layer
- Handle external service errors
- Encapsulate external API details

## Client Pattern

```go
type Client struct {
    BaseURL string
    APIKey  string
    Model   string
    client  *http.Client
}

func NewClient(baseURL, apiKey, model string) *Client {
    return &Client{
        BaseURL: baseURL,
        APIKey:  apiKey,
        Model:   model,
        client:  http.DefaultClient,
    }
}
```

## Types

Shared types for LLM operations:

```go
// Message represents a single message in a chat conversation
type Message struct {
    Role    string `json:"role"`
    Content string `json:"content"`
}

// ChatParams holds parameters for chat completion requests
type ChatParams struct {
    Model       string  // If empty, uses client's default model
    MaxTokens   int     // If 0, no limit
    Temperature float32 // Default 0.7 if not specified
}
```

## Interface Implementation

Service layer defines interface, client implements:

```go
// Service defines
type LLMClient interface {
    Chat(ctx context.Context, message string) (string, error)
}

// Client implements
func (c *Client) Chat(ctx context.Context, message string) (string, error) {
    // Implementation
}
```

## Structured Messages

For RAG and complex conversations, use `ChatWithMessages`:

```go
messages := []Message{
    {Role: "system", Content: "You are a helpful assistant."},
    {Role: "user", Content: "What is RAG?"},
}

params := ChatParams{
    Model:       "", // Uses client default
    MaxTokens:   500,
    Temperature: 0.7,
}

reply, err := client.ChatWithMessages(ctx, messages, params)
```

**Note:** `Chat` and `StreamChat` remain for backward compatibility. `ChatWithMessages` is used by the RAG engine.

## HTTP Request Pattern

```go
req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(body))
req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.APIKey))
req.Header.Set("Content-Type", "application/json")

resp, err := c.client.Do(req)
defer resp.Body.Close()

if resp.StatusCode != http.StatusOK {
    return "", fmt.Errorf("bad status %d", resp.StatusCode)
}
```

## Streaming (SSE)

```go
req.Header.Set("Accept", "text/event-stream")

scanner := bufio.NewScanner(resp.Body)
for scanner.Scan() {
    line := scanner.Text()
    if strings.HasPrefix(line, "data: ") {
        data := strings.TrimPrefix(line, "data: ")
        if data == "[DONE]" {
            break
        }
        // Parse and call callback
    }
}
```

## Embeddings Client

Separate client for generating embeddings:

```go
type EmbeddingsClient struct {
    BaseURL      string
    APIKey       string
    Model        string
    ExpectedSize int // Vector size for validation
    client       *http.Client
}

func NewEmbeddingsClient(baseURL, apiKey, model string, expectedSize int) *EmbeddingsClient

func (c *EmbeddingsClient) EmbedTexts(ctx context.Context, texts []string) ([][]float32, error)
```

**Usage:**

```go
client := llm.NewEmbeddingsClient(
    cfg.EmbeddingBaseURL,
    cfg.LLMAPIKey,
    cfg.EmbeddingModelName,
    cfg.QdrantVectorSize, // Validates all vectors match this size
)

vectors, err := client.EmbedTexts(ctx, []string{"text1", "text2"})
// Returns [][]float32 where each inner slice is one embedding vector
```

**Validation:**

- Validates vector size matches `ExpectedSize` (from `QDRANT_VECTOR_SIZE` config)
- Converts `[]float64` from JSON to `[]float32`
- Returns error if vector size mismatch or empty input

**Structured Error Handling:**

The embeddings client returns structured errors for better error handling:

```go
type EmbeddingError struct {
    StatusCode int
    RawBody    string
    LlamaError *LlamaError
    Err        error
}

type LlamaError struct {
    Error struct {
        Code          int    `json:"code"`
        Message       string `json:"message"`
        Type          string `json:"type"`
        NPromptTokens int    `json:"n_prompt_tokens"`
        NCtx          int    `json:"n_ctx"`
    } `json:"error"`
}
```

**Context Size Limits:**

- The embedding model (`granite-embedding-278m-multilingual`) has a hard context size limit of 512 tokens
- Errors with type `"exceed_context_size_error"` indicate the input exceeded this limit
- Use `IsExceedContextSizeError()` to check for context size errors
- The indexer automatically skips chunks that exceed this limit

## Testing

### Test Patterns

**HTTP Test Server:**

Use `httptest.NewServer` to mock HTTP responses:

```go
server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    resp := ChatResponse{
        ID:     "test-id",
        Object: "chat.completion",
        Choices: []ChatChoice{
            {
                Message: ChatChoiceMessage{
                    Content: "Hello!",
                },
            },
        },
    }
    w.Header().Set("Content-Type", "application/json")
    _ = json.NewEncoder(w).Encode(resp) // Ignore error in test
}))
defer server.Close()

client := NewClient(server.URL, "test-key", "test-model")
```

**Streaming Tests:**

```go
server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    flusher, _ := w.(http.Flusher)
    chunks := []string{"Hello", " ", "world"}
    
    for _, chunk := range chunks {
        _, _ = w.Write([]byte("data: " + chunk + "\n\n")) // Ignore error in test
        flusher.Flush()
    }
    _, _ = w.Write([]byte("data: [DONE]\n\n")) // Ignore error in test
}))
```

**Error Handling:**

Properly handle all error returns in test code:

```go
_ = json.NewEncoder(w).Encode(resp) // Ignore error in test handler
_, _ = w.Write([]byte("error")) // Ignore error in test handler
```

## Rules

- Use context in all requests
- Wrap errors with context
- Close response body
- Check status codes before parsing
- Validate vector sizes in embeddings client
- Keep backward compatibility (don't break existing `Chat` method)
- Handle all error returns (use `_` for intentional ignores in tests)
- Return structured errors (`EmbeddingError`) for better error handling
- Support context size limit detection via `IsExceedContextSizeError()`
- Parse structured error responses from llama.cpp API when available
