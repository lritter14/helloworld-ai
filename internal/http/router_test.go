package http

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"helloworld-ai/internal/service/mocks"
	"go.uber.org/mock/gomock"
)

func TestNewRouter(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

		mockChatService := mocks.NewMockChatService(ctrl)

	deps := &Deps{
		ChatService: mockChatService,
		IndexHTML:   "<html><body>Test</body></html>",
	}

	router := NewRouter(deps)

	if router == nil {
		t.Fatal("NewRouter() returned nil")
	}
}

func TestRouter_Routes(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

		mockChatService := mocks.NewMockChatService(ctrl)

	deps := &Deps{
		ChatService: mockChatService,
		IndexHTML:   "<html><body>Test</body></html>",
	}

	router := NewRouter(deps)

	tests := []struct {
		name       string
		method     string
		path       string
		wantStatus int
	}{
		{
			name:       "GET root serves HTML",
			method:     http.MethodGet,
			path:       "/",
			wantStatus: http.StatusOK,
		},
		{
			name:       "POST /api/chat exists",
			method:     http.MethodPost,
			path:       "/api/chat",
			wantStatus: http.StatusBadRequest, // Bad request due to invalid body, but route exists
		},
		{
			name:       "GET /api/chat method not allowed",
			method:     http.MethodGet,
			path:       "/api/chat",
			wantStatus: http.StatusMethodNotAllowed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("Router %s %s status = %v, want %v", tt.method, tt.path, w.Code, tt.wantStatus)
			}
		})
	}
}

func TestRouter_RootServesHTML(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

		mockChatService := mocks.NewMockChatService(ctrl)

	htmlContent := "<html><body>Test HTML</body></html>"
	deps := &Deps{
		ChatService: mockChatService,
		IndexHTML:   htmlContent,
	}

	router := NewRouter(deps)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Router GET / status = %v, want %v", w.Code, http.StatusOK)
	}

	if w.Body.String() != htmlContent {
		t.Errorf("Router GET / body = %v, want %v", w.Body.String(), htmlContent)
	}

	if w.Header().Get("Content-Type") != "text/html; charset=utf-8" {
		t.Errorf("Router GET / Content-Type = %v, want text/html; charset=utf-8", w.Header().Get("Content-Type"))
	}
}

func TestRouter_MiddlewareApplied(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockChatService := mocks.NewMockChatService(ctrl)

	deps := &Deps{
		ChatService: mockChatService,
		IndexHTML:   "<html></html>",
	}

	router := NewRouter(deps)

	req := httptest.NewRequest(http.MethodPost, "/api/chat", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	// Check CORS headers are present
	if w.Header().Get("Access-Control-Allow-Origin") == "" {
		t.Error("Router should apply CORS middleware")
	}
}

