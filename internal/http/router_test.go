package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"helloworld-ai/internal/indexer"
	"helloworld-ai/internal/rag"
	"helloworld-ai/internal/storage"
)

type stubRAGEngine struct{}

func (stubRAGEngine) Ask(context.Context, rag.AskRequest) (rag.AskResponse, error) {
	return rag.AskResponse{}, nil
}

type stubVaultStore struct{}

func (stubVaultStore) GetOrCreateByName(context.Context, string, string) (storage.VaultRecord, error) {
	return storage.VaultRecord{}, nil
}

func (stubVaultStore) ListAll(context.Context) ([]storage.VaultRecord, error) {
	return []storage.VaultRecord{}, nil
}

func newTestDeps() *Deps {
	return &Deps{
		RAGEngine:       stubRAGEngine{},
		VaultRepo:       stubVaultStore{},
		IndexerPipeline: &indexer.Pipeline{},
	}
}

func TestNewRouter(t *testing.T) {
	router := NewRouter(newTestDeps())

	if router == nil {
		t.Fatal("NewRouter() returned nil")
	}
}

func TestRouter_Routes(t *testing.T) {
	router := NewRouter(newTestDeps())

	tests := []struct {
		name       string
		method     string
		path       string
		wantStatus int
	}{
		{
			name:       "POST /api/v1/ask exists",
			method:     http.MethodPost,
			path:       "/api/v1/ask",
			wantStatus: http.StatusBadRequest, // Bad request due to invalid body, but route exists
		},
		{
			name:       "GET /api/v1/ask method not allowed",
			method:     http.MethodGet,
			path:       "/api/v1/ask",
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
	router := NewRouter(newTestDeps())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Router GET / status = %v, want %v", w.Code, http.StatusOK)
	}

	body := w.Body.String()
	if !strings.Contains(body, "<!DOCTYPE html>") {
		t.Errorf("Router GET / body did not include HTML, got %q", body[:min(len(body), 100)])
	}
}

func TestRouter_MiddlewareApplied(t *testing.T) {
	router := NewRouter(newTestDeps())

	req := httptest.NewRequest(http.MethodPost, "/api/v1/ask", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	// Check CORS headers are present
	if w.Header().Get("Access-Control-Allow-Origin") == "" {
		t.Error("Router should apply CORS middleware")
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
