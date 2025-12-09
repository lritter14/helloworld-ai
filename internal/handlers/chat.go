package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"helloworld-ai/internal/service"
)

// ChatHandler handles HTTP requests for chat.
type ChatHandler struct {
	chatService service.ChatService
	logger      *slog.Logger
}

// NewChatHandler creates a new ChatHandler.
func NewChatHandler(chatService service.ChatService) *ChatHandler {
	return &ChatHandler{
		chatService: chatService,
		logger:      slog.Default(),
	}
}

// ChatRequest represents the HTTP request payload for chat.
type ChatRequest struct {
	Message string `json:"message"`
}

// ChatResponse represents the HTTP response payload for chat.
type ChatResponse struct {
	Reply string `json:"reply"`
}

// ErrorResponse represents an error response.
type ErrorResponse struct {
	Error string `json:"error"`
}

// getLogger extracts logger from context or returns default logger.
func (h *ChatHandler) getLogger(ctx context.Context) *slog.Logger {
	type loggerKeyType string
	const loggerKey loggerKeyType = "logger"
	if ctxLogger := ctx.Value(loggerKey); ctxLogger != nil {
		if l, ok := ctxLogger.(*slog.Logger); ok {
			return l
		}
	}
	return h.logger
}

// ServeHTTP handles HTTP requests for chat.
func (h *ChatHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := h.getLogger(ctx)

	if r.Method != http.MethodPost {
		logger.WarnContext(ctx, "method not allowed", "method", r.Method)
		h.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	// Check if streaming is requested
	stream := r.URL.Query().Get("stream") == "true"

	if stream {
		h.handleStreamingChat(w, r, ctx)
		return
	}

	var req ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logger.WarnContext(ctx, "invalid request body", "error", err)
		h.writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Convert HTTP request to service request
	svcReq := service.ChatRequest{
		Message: req.Message,
	}

	// Call service layer
	svcResp, err := h.chatService.ProcessChat(ctx, svcReq)
	if err != nil {
		h.handleServiceError(w, ctx, err, "Failed to process chat request")
		return
	}

	// Convert service response to HTTP response
	resp := ChatResponse{
		Reply: svcResp.Reply,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		logger.ErrorContext(ctx, "failed to encode response", "error", err)
		h.writeError(w, http.StatusInternalServerError, "Failed to encode response")
		return
	}
}

// handleStreamingChat handles streaming chat requests using Server-Sent Events.
func (h *ChatHandler) handleStreamingChat(w http.ResponseWriter, r *http.Request, ctx context.Context) {
	logger := h.getLogger(ctx)

	var req ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logger.WarnContext(ctx, "invalid request body for streaming", "error", err)
		h.writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Set up Server-Sent Events headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	// CORS headers for streaming
	origin := r.Header.Get("Origin")
	if origin != "" {
		w.Header().Set("Access-Control-Allow-Origin", origin)
	} else {
		w.Header().Set("Access-Control-Allow-Origin", "*")
	}
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

	// Convert HTTP request to service request
	svcReq := service.ChatRequest{
		Message: req.Message,
	}

	// Create a flusher to send data immediately
	flusher, ok := w.(http.Flusher)
	if !ok {
		logger.ErrorContext(ctx, "streaming not supported by response writer")
		h.writeError(w, http.StatusInternalServerError, "Streaming not supported")
		return
	}

	// Stream chat response
	err := h.chatService.StreamChat(ctx, svcReq, func(chunk string) error {
		// Write chunk as SSE format: "data: <chunk>\n\n"
		// If chunk contains newlines, we need to prefix continuation lines with a space
		// For simplicity, we'll just send the chunk as-is since most SSE parsers handle it
		_, err := fmt.Fprintf(w, "data: %s\n\n", chunk)
		if err != nil {
			return err
		}
		flusher.Flush()
		return nil
	})

	if err != nil {
		logger.ErrorContext(ctx, "error streaming chat", "error", err)
		// Send error as SSE
		_, _ = fmt.Fprintf(w, "data: {\"error\":\"%s\"}\n\n", err.Error())
		flusher.Flush()
		return
	}

	// Send done signal
	_, _ = fmt.Fprintf(w, "data: [DONE]\n\n")
	flusher.Flush()
}

// handleServiceError maps service errors to appropriate HTTP status codes and responses.
func (h *ChatHandler) handleServiceError(w http.ResponseWriter, ctx context.Context, err error, defaultMsg string) {
	logger := h.getLogger(ctx)
	logger.ErrorContext(ctx, "service error", "error", err)

	var validationErr *service.ValidationError
	if errors.As(err, &validationErr) {
		h.writeError(w, http.StatusBadRequest, fmt.Sprintf("Validation error: %s", validationErr.Error()))
		return
	}

	// Check for wrapped errors
	if errors.Is(err, service.ErrInvalidInput) {
		h.writeError(w, http.StatusBadRequest, "Invalid input")
		return
	}

	if errors.Is(err, service.ErrNotFound) {
		h.writeError(w, http.StatusNotFound, "Resource not found")
		return
	}

	if errors.Is(err, service.ErrExternalService) {
		h.writeError(w, http.StatusBadGateway, "External service error")
		return
	}

	// Default to internal server error
	h.writeError(w, http.StatusInternalServerError, defaultMsg)
}

// writeError writes an error response.
func (h *ChatHandler) writeError(w http.ResponseWriter, statusCode int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(ErrorResponse{
		Error: message,
	})
}
