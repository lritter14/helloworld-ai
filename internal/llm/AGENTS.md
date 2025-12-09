# LLM Client Layer - Agent Guide

External service client patterns.

## Core Responsibilities

- Communicate with external LLM services
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

## Rules

- Use context in all requests
- Wrap errors with context
- Close response body
- Check status codes before parsing
