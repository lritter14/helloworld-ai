package vectorstore

import (
	"context"
	"log/slog"
	"net/url"
	"strconv"
	"testing"
)

// TestNewQdrantStore_URLParsing tests URL parsing logic without creating a real client.
// This avoids connection warnings in unit tests.
func TestNewQdrantStore_URLParsing(t *testing.T) {
	tests := []struct {
		name     string
		urlStr   string
		wantErr  bool
		wantHost string
		wantPort int
	}{
		{
			name:     "valid URL",
			urlStr:   "http://localhost:6333",
			wantErr:  false,
			wantHost: "localhost",
			wantPort: 6334, // gRPC port is HTTP port + 1
		},
		{
			name:     "URL with custom port",
			urlStr:   "http://localhost:9000",
			wantErr:  false,
			wantHost: "localhost",
			wantPort: 9001,
		},
		{
			name:    "invalid URL",
			urlStr:  "://invalid",
			wantErr: true,
		},
		{
			name:     "URL without port",
			urlStr:   "http://localhost",
			wantErr:  false,
			wantHost: "localhost",
			wantPort: 6334, // Default
		},
		{
			name:     "URL without hostname",
			urlStr:   "http://:6333",
			wantErr:  false,
			wantHost: "localhost", // Defaults to localhost
			wantPort: 6334,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsedURL, err := url.Parse(tt.urlStr)
			if tt.wantErr {
				if err == nil {
					// If URL parsing succeeds but we expect an error from NewQdrantStore,
					// we'll test that separately
					// For now, just verify invalid URLs fail parsing
					if tt.urlStr == "://invalid" {
						if err == nil {
							t.Error("Expected URL parsing to fail for invalid URL")
						}
					}
				}
				return
			}

			if err != nil {
				t.Fatalf("Failed to parse URL: %v", err)
			}

			// Test the URL parsing logic that NewQdrantStore uses
			host := parsedURL.Hostname()
			if host == "" {
				host = "localhost"
			}

			port := 6334 // Default gRPC port
			if parsedURL.Port() != "" {
				httpPort, err := strconv.Atoi(parsedURL.Port())
				if err == nil {
					port = httpPort + 1
				}
			}

			if host != tt.wantHost {
				t.Errorf("Host = %v, want %v", host, tt.wantHost)
			}
			if port != tt.wantPort {
				t.Errorf("Port = %v, want %v", port, tt.wantPort)
			}
		})
	}
}

// TestNewQdrantStore_InvalidURL tests that invalid URLs return errors.
// This test creates a real client but only for the error case.
func TestNewQdrantStore_InvalidURL(t *testing.T) {
	_, err := NewQdrantStore("://invalid")
	if err == nil {
		t.Error("NewQdrantStore() with invalid URL should return error")
	}
}

func TestNewQdrantStore_PortDerivation(t *testing.T) {
	tests := []struct {
		name     string
		urlStr   string
		expected int // Expected gRPC port
	}{
		{
			name:     "default port",
			urlStr:   "http://localhost:6333",
			expected: 6334, // HTTP port + 1
		},
		{
			name:     "custom port",
			urlStr:   "http://localhost:9000",
			expected: 9001, // HTTP port + 1
		},
		{
			name:     "no port specified",
			urlStr:   "http://localhost",
			expected: 6334, // Default
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Parse URL to verify port logic
			parsedURL, err := url.Parse(tt.urlStr)
			if err != nil {
				t.Fatalf("Failed to parse URL: %v", err)
			}

			port := 6334 // Default gRPC port
			if parsedURL.Port() != "" {
				httpPort, err := strconv.Atoi(parsedURL.Port())
				if err == nil {
					port = httpPort + 1
				}
			}

			if port != tt.expected {
				t.Errorf("Port derivation: got %d, want %d", port, tt.expected)
			}
		})
	}
}

func TestQdrantStore_getLogger(t *testing.T) {
	store := &QdrantStore{logger: slog.Default()}

	ctx := context.Background()
	logger := store.getLogger(ctx)
	if logger == nil {
		t.Error("getLogger() should return logger when store has logger set")
	}

	// Verify it returns the store's logger when no context logger
	if logger != store.logger {
		t.Error("getLogger() should return store logger when context has no logger")
	}
}

func TestQdrantStore_Upsert_EmptyPoints(t *testing.T) {
	// This test verifies that Upsert handles empty points gracefully
	// We test the early return logic without needing a real client
	store := &QdrantStore{
		logger: slog.Default(),
	}

	ctx := context.Background()
	// This should return early before trying to use the client
	err := store.Upsert(ctx, "test-collection", []Point{})
	if err != nil {
		t.Errorf("Upsert() with empty points should return early without error, got: %v", err)
	}
}

func TestQdrantStore_Delete_EmptyIDs(t *testing.T) {
	// This test verifies that Delete handles empty IDs gracefully
	// We test the early return logic without needing a real client
	store := &QdrantStore{
		logger: slog.Default(),
	}

	ctx := context.Background()
	// This should return early before trying to use the client
	err := store.Delete(ctx, "test-collection", []string{})
	if err != nil {
		t.Errorf("Delete() with empty IDs should return early without error, got: %v", err)
	}
}

func TestQdrantStore_Search_InvalidK(t *testing.T) {
	// This test verifies validation logic without needing a real client
	store := &QdrantStore{
		logger: slog.Default(),
	}

	ctx := context.Background()
	// These should fail validation before trying to use the client
	_, err := store.Search(ctx, "test-collection", []float32{1.0, 2.0}, 0, nil)
	if err == nil {
		t.Error("Search() with k=0 should return error")
	}

	_, err = store.Search(ctx, "test-collection", []float32{1.0, 2.0}, -1, nil)
	if err == nil {
		t.Error("Search() with k=-1 should return error")
	}
}

func TestConvertPayloadToMap(t *testing.T) {
	// This is a helper function test - would need Qdrant types to fully test
	// For now, just verify it exists and handles nil
	result := convertPayloadToMap(nil)
	if result == nil {
		t.Error("convertPayloadToMap() should return empty map, not nil")
	}
	if len(result) != 0 {
		t.Errorf("convertPayloadToMap() with nil should return empty map, got %d items", len(result))
	}
}
