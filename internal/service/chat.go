package service

import (
	"context"
	"fmt"
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
	Message string
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
}

// NewChatService creates a new ChatService.
func NewChatService(llmClient LLMClient) ChatService {
	return &chatService{
		llmClient: llmClient,
	}
}

// ProcessChat processes a chat request.
func (s *chatService) ProcessChat(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	// Business validation
	if req.Message == "" {
		return ChatResponse{}, fmt.Errorf("message cannot be empty")
	}

	// Call external LLM service
	reply, err := s.llmClient.Chat(ctx, req.Message)
	if err != nil {
		return ChatResponse{}, fmt.Errorf("failed to get LLM response: %w", err)
	}

	return ChatResponse{
		Reply: reply,
	}, nil
}

// StreamChat processes a chat request and streams the response.
func (s *chatService) StreamChat(ctx context.Context, req ChatRequest, callback func(chunk string) error) error {
	// Business validation
	if req.Message == "" {
		return fmt.Errorf("message cannot be empty")
	}

	// Call external LLM service with streaming
	return s.llmClient.StreamChat(ctx, req.Message, callback)
}

