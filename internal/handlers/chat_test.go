package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"helloworld-ai/internal/service"
	"helloworld-ai/internal/service/mocks"
	"go.uber.org/mock/gomock"
)

func TestNewChatHandler(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

			mockChatService := mocks.NewMockChatService(ctrl)
	handler := NewChatHandler(mockChatService)

	if handler == nil {
		t.Fatal("NewChatHandler() returned nil")
	}
	if handler.chatService != mockChatService {
		t.Error("NewChatHandler() chatService not set correctly")
	}
}

func TestChatHandler_ServeHTTP(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	tests := []struct {
		name          string
		method        string
		body          interface{}
		mockSetup     func(*mocks.MockChatService)
		wantStatus    int
		wantResponse  interface{}
		checkResponse func(*httptest.ResponseRecorder) bool
	}{
		{
			name:   "successful POST request",
			method: http.MethodPost,
			body: ChatRequest{
				Message: "Hello",
			},
			mockSetup: func(m *mocks.MockChatService) {
				m.EXPECT().
					ProcessChat(gomock.Any(), service.ChatRequest{Message: "Hello"}).
					Return(service.ChatResponse{Reply: "Hi there!"}, nil)
			},
			wantStatus: http.StatusOK,
			checkResponse: func(w *httptest.ResponseRecorder) bool {
				var resp ChatResponse
				if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
					return false
				}
				return resp.Reply == "Hi there!"
			},
		},
		{
			name:   "method not allowed",
			method: http.MethodGet,
			mockSetup: func(m *mocks.MockChatService) {
				// No calls expected
			},
			wantStatus: http.StatusMethodNotAllowed,
		},
		{
			name:   "invalid JSON body",
			method: http.MethodPost,
			body:   "invalid json",
			mockSetup: func(m *mocks.MockChatService) {
				// No calls expected
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:   "validation error",
			method: http.MethodPost,
			body: ChatRequest{
				Message: "",
			},
			mockSetup: func(m *mocks.MockChatService) {
				m.EXPECT().
					ProcessChat(gomock.Any(), service.ChatRequest{Message: ""}).
					Return(service.ChatResponse{}, &service.ValidationError{
						Field:   "message",
						Message: "cannot be empty",
					})
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:   "service error",
			method: http.MethodPost,
			body: ChatRequest{
				Message: "Hello",
			},
			mockSetup: func(m *mocks.MockChatService) {
				m.EXPECT().
					ProcessChat(gomock.Any(), service.ChatRequest{Message: "Hello"}).
					Return(service.ChatResponse{}, errors.New("service error"))
			},
			wantStatus: http.StatusInternalServerError,
		},
		{
			name:   "ErrNotFound",
			method: http.MethodPost,
			body: ChatRequest{
				Message: "Hello",
			},
			mockSetup: func(m *mocks.MockChatService) {
				m.EXPECT().
					ProcessChat(gomock.Any(), service.ChatRequest{Message: "Hello"}).
					Return(service.ChatResponse{}, service.ErrNotFound)
			},
			wantStatus: http.StatusNotFound,
		},
		{
			name:   "ErrExternalService",
			method: http.MethodPost,
			body: ChatRequest{
				Message: "Hello",
			},
			mockSetup: func(m *mocks.MockChatService) {
				m.EXPECT().
					ProcessChat(gomock.Any(), service.ChatRequest{Message: "Hello"}).
					Return(service.ChatResponse{}, service.ErrExternalService)
			},
			wantStatus: http.StatusBadGateway,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockChatService := mocks.NewMockChatService(ctrl)
			tt.mockSetup(mockChatService)

			handler := NewChatHandler(mockChatService)

			var bodyBytes []byte
			if tt.body != nil {
				var err error
				bodyBytes, err = json.Marshal(tt.body)
				if err != nil {
					// For invalid JSON test case
					bodyBytes = []byte(tt.body.(string))
				}
			}

			req := httptest.NewRequest(tt.method, "/api/chat", bytes.NewBuffer(bodyBytes))
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("ServeHTTP() status = %v, want %v", w.Code, tt.wantStatus)
			}

			if tt.checkResponse != nil && !tt.checkResponse(w) {
				t.Error("ServeHTTP() response validation failed")
			}
		})
	}
}

func TestChatHandler_handleStreamingChat(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	tests := []struct {
		name       string
		body       interface{}
		mockSetup  func(*mocks.MockChatService)
		wantStatus int
		chunks     []string
	}{
		{
			name: "successful streaming",
			body: ChatRequest{
				Message: "Hello",
			},
			mockSetup: func(m *mocks.MockChatService) {
				m.EXPECT().
					StreamChat(gomock.Any(), service.ChatRequest{Message: "Hello"}, gomock.Any()).
					DoAndReturn(func(ctx context.Context, req service.ChatRequest, callback func(chunk string) error) error {
						chunks := []string{"Hello", " ", "world"}
						for _, chunk := range chunks {
							if err := callback(chunk); err != nil {
								return err
							}
						}
						return nil
					})
			},
			wantStatus: http.StatusOK,
			chunks:    []string{"Hello", " ", "world"},
		},
		{
			name: "invalid JSON body",
			body: "invalid json",
			mockSetup: func(m *mocks.MockChatService) {
				// No calls expected for invalid JSON
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "streaming error",
			body: interface{}(ChatRequest{
				Message: "Hello",
			}),
			mockSetup: func(m *mocks.MockChatService) {
				m.EXPECT().
					StreamChat(gomock.Any(), service.ChatRequest{Message: "Hello"}, gomock.Any()).
					Return(errors.New("stream error"))
			},
			wantStatus: http.StatusOK, // SSE sends error in stream, not HTTP status
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockChatService := mocks.NewMockChatService(ctrl)
			tt.mockSetup(mockChatService)

			handler := NewChatHandler(mockChatService)

			bodyBytes, _ := json.Marshal(tt.body)
			req := httptest.NewRequest(http.MethodPost, "/api/chat?stream=true", bytes.NewBuffer(bodyBytes))
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("handleStreamingChat() status = %v, want %v", w.Code, tt.wantStatus)
			}

			// Check SSE headers
			if tt.wantStatus == http.StatusOK && len(tt.chunks) > 0 {
				if w.Header().Get("Content-Type") != "text/event-stream" {
					t.Error("handleStreamingChat() missing Content-Type header")
				}
			}
		})
	}
}

func TestChatHandler_getLogger(t *testing.T) {
	handler := NewChatHandler(nil)

	// Test with context without logger
	ctx := context.Background()
	logger := handler.getLogger(ctx)
	if logger == nil {
		t.Error("getLogger() returned nil")
	}

	// Test with context with logger
	type loggerKeyType string
	const loggerKey loggerKeyType = "logger"
	ctxWithLogger := context.WithValue(ctx, loggerKey, handler.logger)
	logger2 := handler.getLogger(ctxWithLogger)
	if logger2 != handler.logger {
		t.Error("getLogger() should return logger from context")
	}
}

func TestChatHandler_writeError(t *testing.T) {
	handler := NewChatHandler(nil)
	w := httptest.NewRecorder()

	handler.writeError(w, http.StatusBadRequest, "test error")

	if w.Code != http.StatusBadRequest {
		t.Errorf("writeError() status = %v, want %v", w.Code, http.StatusBadRequest)
	}

	var resp ErrorResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("writeError() invalid JSON: %v", err)
	}

	if resp.Error != "test error" {
		t.Errorf("writeError() error = %v, want test error", resp.Error)
	}
}

