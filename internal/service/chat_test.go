package service_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"testing"

	"helloworld-ai/internal/service"
	"helloworld-ai/internal/service/mocks"

	"go.uber.org/mock/gomock"
)

func init() {
	// Set default logger to discard output for cleaner test output
	// This suppresses logs from slog.Default() used in the service layer
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
}

// testContext returns a context for testing.
// The default logger is already set to discard in init().
func testContext() context.Context {
	return context.Background()
}

func TestNewChatService(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLLMClient := mocks.NewMockLLMClient(ctrl)
	svc := service.NewChatService(mockLLMClient)

	if svc == nil {
		t.Fatal("NewChatService() returned nil")
	}
}

func TestChatService_ProcessChat(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLLMClient := mocks.NewMockLLMClient(ctrl)
	svc := service.NewChatService(mockLLMClient)

	tests := []struct {
		name         string
		req          service.ChatRequest
		mockSetup    func()
		wantErr      bool
		wantReply    string
		checkErrType func(error) bool
	}{
		{
			name: "successful chat",
			req: service.ChatRequest{
				Message: "Hello, world!",
			},
			mockSetup: func() {
				mockLLMClient.EXPECT().
					Chat(gomock.Any(), "Hello, world!").
					Return("Hi there!", nil)
			},
			wantErr:   false,
			wantReply: "Hi there!",
		},
		{
			name: "empty message",
			req: service.ChatRequest{
				Message: "",
			},
			mockSetup: func() {
				// No mock call expected
			},
			wantErr: true,
			checkErrType: func(err error) bool {
				var validationErr *service.ValidationError
				return errors.As(err, &validationErr) && validationErr.Field == "message"
			},
		},
		{
			name: "LLM client error",
			req: service.ChatRequest{
				Message: "Hello",
			},
			mockSetup: func() {
				mockLLMClient.EXPECT().
					Chat(gomock.Any(), "Hello").
					Return("", errors.New("LLM service unavailable"))
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.mockSetup()

			ctx := testContext()
			resp, err := svc.ProcessChat(ctx, tt.req)

			if tt.wantErr {
				if err == nil {
					t.Errorf("ProcessChat() expected error, got nil")
					return
				}
				if tt.checkErrType != nil && !tt.checkErrType(err) {
					t.Errorf("ProcessChat() error type mismatch: %v", err)
				}
			} else {
				if err != nil {
					t.Errorf("ProcessChat() unexpected error: %v", err)
					return
				}
				if resp.Reply != tt.wantReply {
					t.Errorf("ProcessChat() reply = %v, want %v", resp.Reply, tt.wantReply)
				}
			}
		})
	}
}

func TestChatService_ProcessChat_WithLogger(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLLMClient := mocks.NewMockLLMClient(ctrl)
	svc := service.NewChatService(mockLLMClient)

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	// Use a typed key to avoid collisions (matching service layer pattern)
	type loggerKeyType string
	const loggerKey loggerKeyType = "logger"
	ctx := context.WithValue(context.Background(), loggerKey, logger)

	mockLLMClient.EXPECT().
		Chat(gomock.Any(), "test").
		Return("response", nil)

	resp, err := svc.ProcessChat(ctx, service.ChatRequest{Message: "test"})
	if err != nil {
		t.Fatalf("ProcessChat() error = %v", err)
	}
	if resp.Reply != "response" {
		t.Errorf("ProcessChat() reply = %v, want response", resp.Reply)
	}
}

func TestChatService_StreamChat(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLLMClient := mocks.NewMockLLMClient(ctrl)
	svc := service.NewChatService(mockLLMClient)

	tests := []struct {
		name         string
		req          service.ChatRequest
		mockSetup    func()
		wantErr      bool
		checkErrType func(error) bool
		chunks       []string
	}{
		{
			name: "successful streaming",
			req: service.ChatRequest{
				Message: "Hello",
			},
			mockSetup: func() {
				mockLLMClient.EXPECT().
					StreamChat(gomock.Any(), "Hello", gomock.Any()).
					DoAndReturn(func(ctx context.Context, msg string, callback func(chunk string) error) error {
						chunks := []string{"Hello", " ", "world", "!"}
						for _, chunk := range chunks {
							if err := callback(chunk); err != nil {
								return err
							}
						}
						return nil
					})
			},
			wantErr: false,
			chunks:  []string{"Hello", " ", "world", "!"},
		},
		{
			name: "empty message",
			req: service.ChatRequest{
				Message: "",
			},
			mockSetup: func() {
				// No mock call expected
			},
			wantErr: true,
			checkErrType: func(err error) bool {
				var validationErr *service.ValidationError
				return errors.As(err, &validationErr) && validationErr.Field == "message"
			},
		},
		{
			name: "LLM client error",
			req: service.ChatRequest{
				Message: "Hello",
			},
			mockSetup: func() {
				mockLLMClient.EXPECT().
					StreamChat(gomock.Any(), "Hello", gomock.Any()).
					Return(errors.New("stream error"))
			},
			wantErr: true,
		},
		{
			name: "callback error",
			req: service.ChatRequest{
				Message: "Hello",
			},
			mockSetup: func() {
				mockLLMClient.EXPECT().
					StreamChat(gomock.Any(), "Hello", gomock.Any()).
					DoAndReturn(func(ctx context.Context, msg string, callback func(chunk string) error) error {
						return callback("chunk")
					})
			},
			wantErr: false, // Callback errors are handled by LLM client
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.mockSetup()

			ctx := testContext()
			var receivedChunks []string
			err := svc.StreamChat(ctx, tt.req, func(chunk string) error {
				receivedChunks = append(receivedChunks, chunk)
				return nil
			})

			if tt.wantErr {
				if err == nil {
					t.Errorf("StreamChat() expected error, got nil")
					return
				}
				if tt.checkErrType != nil && !tt.checkErrType(err) {
					t.Errorf("StreamChat() error type mismatch: %v", err)
				}
			} else {
				if err != nil {
					t.Errorf("StreamChat() unexpected error: %v", err)
					return
				}
				if tt.chunks != nil {
					if len(receivedChunks) != len(tt.chunks) {
						t.Errorf("StreamChat() received %d chunks, want %d", len(receivedChunks), len(tt.chunks))
					}
				}
			}
		})
	}
}
