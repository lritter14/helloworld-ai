# Handlers Layer - Agent Guide

HTTP request/response handling patterns for the ingress layer.

## Core Responsibilities

- HTTP-specific concerns (status codes, headers, JSON encoding/decoding)
- Convert HTTP requests to service requests
- Convert service responses to HTTP responses
- Map service errors to HTTP status codes

## Handler Pattern

```go
type ChatHandler struct {
    chatService service.ChatService
    logger      *slog.Logger
}

func (h *ChatHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    logger := h.getLogger(ctx)
    
    // Validate method, decode request, call service, encode response
}
```

## Request/Response DTOs

Define separate DTOs in handler package:

```go
type ChatRequest struct {
    Message string `json:"message"`
}

type ChatResponse struct {
    Reply string `json:"reply"`
}
```

Convert at boundaries:

```go
// HTTP → Service
svcReq := service.ChatRequest{Message: req.Message}

// Service → HTTP
resp := ChatResponse{Reply: svcResp.Reply}
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

## Streaming (SSE)

```go
w.Header().Set("Content-Type", "text/event-stream")
w.Header().Set("Cache-Control", "no-cache")

flusher, _ := w.(http.Flusher)
err := h.chatService.StreamChat(ctx, svcReq, func(chunk string) error {
    fmt.Fprintf(w, "data: %s\n\n", chunk)
    flusher.Flush()
    return nil
})
```

## Rules

- NO business logic - Delegate to service layer immediately
- Set Content-Type header
- Extract logger from context
- Validate HTTP method if needed
