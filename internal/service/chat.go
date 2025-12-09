package service

//go:generate go run go.uber.org/mock/mockgen@latest -destination=mocks/mock_llm_client.go -package=mocks helloworld-ai/internal/service LLMClient
//go:generate go run go.uber.org/mock/mockgen@latest -destination=mocks/mock_chat_service.go -package=mocks -mock_names=ChatService=MockChatService helloworld-ai/internal/service ChatService

import (
	"context"
	"log/slog"
)

// LLMClient is an interface for interacting with an LLM API.
// This interface is defined from the service layer's perspective (consumer-first).
type LLMClient interface {
	// Chat sends a message to the LLM and returns the reply.
	Chat(ctx context.Context, message string) (string, error)
	// StreamChat sends a message to the LLM and streams the reply via callback.
	StreamChat(ctx context.Context, message string, callback func(chunk string) error) error
}

// ChatRequest represents a chat request in the domain layer.
type ChatRequest struct {
	Message string `validate:"required"`
}

// ChatResponse represents a chat response in the domain layer.
type ChatResponse struct {
	Reply string
}

// ChatService provides chat functionality.
type ChatService interface {
	// ProcessChat processes a chat request and returns a response.
	ProcessChat(ctx context.Context, req ChatRequest) (ChatResponse, error)
	// StreamChat processes a chat request and streams the response via callback.
	StreamChat(ctx context.Context, req ChatRequest, callback func(chunk string) error) error
}

// chatService implements ChatService.
type chatService struct {
	llmClient LLMClient
	logger    *slog.Logger
}

// NewChatService creates a new ChatService.
func NewChatService(llmClient LLMClient) ChatService {
	return &chatService{
		llmClient: llmClient,
		logger:    slog.Default(),
	}
}

// ProcessChat processes a chat request.
func (s *chatService) ProcessChat(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	logger := s.logger
	type loggerKeyType string
	const loggerKey loggerKeyType = "logger"
	if ctxLogger := ctx.Value(loggerKey); ctxLogger != nil {
		if l, ok := ctxLogger.(*slog.Logger); ok {
			logger = l
		}
	}

	// Business validation
	if req.Message == "" {
		logger.WarnContext(ctx, "empty message in chat request")
		return ChatResponse{}, &ValidationError{
			Field:   "message",
			Message: "cannot be empty",
		}
	}

	// Call external LLM service
	reply, err := s.llmClient.Chat(ctx, req.Message)
	if err != nil {
		logger.ErrorContext(ctx, "failed to get LLM response", "error", err)
		return ChatResponse{}, WrapError(err, "failed to get LLM response")
	}

	logger.InfoContext(ctx, "chat request processed successfully", "message_length", len(req.Message), "reply_length", len(reply))
	return ChatResponse{
		Reply: reply,
	}, nil
}

// StreamChat processes a chat request and streams the response.
func (s *chatService) StreamChat(ctx context.Context, req ChatRequest, callback func(chunk string) error) error {
	logger := s.logger
	type loggerKeyType string
	const loggerKey loggerKeyType = "logger"
	if ctxLogger := ctx.Value(loggerKey); ctxLogger != nil {
		if l, ok := ctxLogger.(*slog.Logger); ok {
			logger = l
		}
	}

	// Business validation
	if req.Message == "" {
		logger.WarnContext(ctx, "empty message in streaming chat request")
		return &ValidationError{
			Field:   "message",
			Message: "cannot be empty",
		}
	}

	// Call external LLM service with streaming
	err := s.llmClient.StreamChat(ctx, req.Message, callback)
	if err != nil {
		logger.ErrorContext(ctx, "failed to stream LLM response", "error", err)
		return WrapError(err, "failed to stream LLM response")
	}

	logger.InfoContext(ctx, "streaming chat request processed successfully", "message_length", len(req.Message))
	return nil
}
