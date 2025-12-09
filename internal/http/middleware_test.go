package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLoggerMiddleware(t *testing.T) {
	var capturedCtx context.Context
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedCtx = r.Context()
		w.WriteHeader(http.StatusOK)
	})

	middleware := LoggerMiddleware(handler)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	middleware.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("LoggerMiddleware() status = %v, want %v", w.Code, http.StatusOK)
	}

	// Check that logger was added to context
	if capturedCtx == nil {
		t.Fatal("LoggerMiddleware() should capture context")
	}
	logger := capturedCtx.Value(loggerKey)
	if logger == nil {
		t.Error("LoggerMiddleware() should add logger to context")
	}
}

func TestRequestLogger(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		path       string
		statusCode int
		shouldLog  bool
	}{
		{
			name:       "regular request",
			method:     http.MethodPost,
			path:       "/api/v1/ask",
			statusCode: http.StatusOK,
			shouldLog:  true,
		},
		{
			name:       "health check skipped",
			method:     http.MethodGet,
			path:       "/",
			statusCode: http.StatusOK,
			shouldLog:  false,
		},
		{
			name:       "non-200 health check logged",
			method:     http.MethodGet,
			path:       "/",
			statusCode: http.StatusNotFound,
			shouldLog:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
			})

			middleware := RequestLogger(handler)
			req := httptest.NewRequest(tt.method, tt.path, nil)
			w := httptest.NewRecorder()

			middleware.ServeHTTP(w, req)

			if w.Code != tt.statusCode {
				t.Errorf("RequestLogger() status = %v, want %v", w.Code, tt.statusCode)
			}
		})
	}
}

func TestResponseWriter_WriteHeader(t *testing.T) {
	w := httptest.NewRecorder()
	rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

	rw.WriteHeader(http.StatusNotFound)

	if rw.statusCode != http.StatusNotFound {
		t.Errorf("responseWriter.WriteHeader() statusCode = %v, want %v", rw.statusCode, http.StatusNotFound)
	}

	if w.Code != http.StatusNotFound {
		t.Errorf("responseWriter.WriteHeader() underlying status = %v, want %v", w.Code, http.StatusNotFound)
	}
}

func TestCORS(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	middleware := CORS(handler)

	tests := []struct {
		name           string
		method         string
		origin         string
		wantStatusCode int
		checkHeaders   func(*httptest.ResponseRecorder) bool
	}{
		{
			name:           "preflight OPTIONS",
			method:         http.MethodOptions,
			origin:         "http://localhost:3000",
			wantStatusCode: http.StatusNoContent,
			checkHeaders: func(w *httptest.ResponseRecorder) bool {
				return w.Header().Get("Access-Control-Allow-Origin") != ""
			},
		},
		{
			name:           "request with origin",
			method:         http.MethodPost,
			origin:         "http://localhost:3000",
			wantStatusCode: http.StatusOK,
			checkHeaders: func(w *httptest.ResponseRecorder) bool {
				return w.Header().Get("Access-Control-Allow-Origin") == "http://localhost:3000"
			},
		},
		{
			name:           "request without origin",
			method:         http.MethodPost,
			origin:         "",
			wantStatusCode: http.StatusOK,
			checkHeaders: func(w *httptest.ResponseRecorder) bool {
				return w.Header().Get("Access-Control-Allow-Origin") == "*"
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "/test", nil)
			if tt.origin != "" {
				req.Header.Set("Origin", tt.origin)
			}
			w := httptest.NewRecorder()

			middleware.ServeHTTP(w, req)

			if w.Code != tt.wantStatusCode {
				t.Errorf("CORS() status = %v, want %v", w.Code, tt.wantStatusCode)
			}

			if tt.checkHeaders != nil && !tt.checkHeaders(w) {
				t.Error("CORS() header validation failed")
			}
		})
	}
}

func TestCORS_Headers(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	middleware := CORS(handler)

	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	w := httptest.NewRecorder()

	middleware.ServeHTTP(w, req)

	headers := map[string]string{
		"Access-Control-Allow-Origin":  "http://localhost:3000",
		"Access-Control-Allow-Methods": "GET, POST, OPTIONS",
		"Access-Control-Allow-Headers": "Content-Type, Authorization",
		"Access-Control-Max-Age":       "3600",
	}

	for header, wantValue := range headers {
		gotValue := w.Header().Get(header)
		if gotValue != wantValue {
			t.Errorf("CORS() header %s = %v, want %v", header, gotValue, wantValue)
		}
	}
}
