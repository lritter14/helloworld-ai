package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewEmbeddingsClient(t *testing.T) {
	client := NewEmbeddingsClient("http://localhost:8080", "test-key", "test-model", 768)
	if client == nil {
		t.Fatal("NewEmbeddingsClient() returned nil")
	}
	if client.BaseURL != "http://localhost:8080" {
		t.Errorf("NewEmbeddingsClient() BaseURL = %v, want http://localhost:8080", client.BaseURL)
	}
	if client.ExpectedSize != 768 {
		t.Errorf("NewEmbeddingsClient() ExpectedSize = %v, want 768", client.ExpectedSize)
	}
}

func TestEmbeddingsClient_EmbedTexts(t *testing.T) {
	tests := []struct {
		name         string
		texts        []string
		expectedSize int
		serverResp   func(w http.ResponseWriter, r *http.Request)
		wantErr      bool
		wantCount    int
	}{
		{
			name:         "successful embedding",
			texts:        []string{"Hello", "World"},
			expectedSize: 768,
			serverResp: func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					t.Errorf("expected POST, got %s", r.Method)
				}
				if r.URL.Path != "/v1/embeddings" {
					t.Errorf("expected /v1/embeddings, got %s", r.URL.Path)
				}

				resp := EmbeddingsResponse{
					Data: []EmbeddingData{
						{Embedding: make([]float64, 768)},
						{Embedding: make([]float64, 768)},
					},
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(resp)
			},
			wantErr:   false,
			wantCount: 2,
		},
		{
			name:         "empty input",
			texts:        []string{},
			expectedSize: 768,
			serverResp: func(w http.ResponseWriter, r *http.Request) {
				// Should not be called
			},
			wantErr: true,
		},
		{
			name:         "wrong embedding count",
			texts:        []string{"Hello", "World"},
			expectedSize: 768,
			serverResp: func(w http.ResponseWriter, r *http.Request) {
				resp := EmbeddingsResponse{
					Data: []EmbeddingData{
						{Embedding: make([]float64, 768)},
					},
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(resp)
			},
			wantErr: true,
		},
		{
			name:         "wrong vector size",
			texts:        []string{"Hello"},
			expectedSize: 768,
			serverResp: func(w http.ResponseWriter, r *http.Request) {
				resp := EmbeddingsResponse{
					Data: []EmbeddingData{
						{Embedding: make([]float64, 512)}, // Wrong size
					},
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(resp)
			},
			wantErr: true,
		},
		{
			name:         "server error",
			texts:        []string{"Hello"},
			expectedSize: 768,
			serverResp: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte("internal server error"))
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(tt.serverResp))
			defer server.Close()

			client := NewEmbeddingsClient(server.URL, "test-key", "test-model", tt.expectedSize)
			embeddings, err := client.EmbedTexts(context.Background(), tt.texts)

			if tt.wantErr {
				if err == nil {
					t.Errorf("EmbedTexts() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("EmbedTexts() unexpected error: %v", err)
				return
			}

			if len(embeddings) != tt.wantCount {
				t.Errorf("EmbedTexts() returned %d embeddings, want %d", len(embeddings), tt.wantCount)
			}

			for i, emb := range embeddings {
				if len(emb) != tt.expectedSize {
					t.Errorf("EmbedTexts() embedding[%d] size = %d, want %d", i, len(emb), tt.expectedSize)
				}
			}
		})
	}
}

func TestEmbeddingsClient_EmbedTexts_ConvertsFloat64ToFloat32(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return embeddings with float64 values
		resp := EmbeddingsResponse{
			Data: []EmbeddingData{
				{Embedding: []float64{1.5, 2.5, 3.5}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewEmbeddingsClient(server.URL, "test-key", "test-model", 3)
	embeddings, err := client.EmbedTexts(context.Background(), []string{"test"})
	if err != nil {
		t.Fatalf("EmbedTexts() error = %v", err)
	}

	if len(embeddings) != 1 {
		t.Fatalf("EmbedTexts() returned %d embeddings, want 1", len(embeddings))
	}

	// Check that values are float32
	emb := embeddings[0]
	if len(emb) != 3 {
		t.Fatalf("EmbedTexts() embedding size = %d, want 3", len(emb))
	}

	// Verify conversion (with some tolerance for float precision)
	if emb[0] != float32(1.5) {
		t.Errorf("EmbedTexts() embedding[0] = %v, want 1.5", emb[0])
	}
	if emb[1] != float32(2.5) {
		t.Errorf("EmbedTexts() embedding[1] = %v, want 2.5", emb[1])
	}
	if emb[2] != float32(3.5) {
		t.Errorf("EmbedTexts() embedding[2] = %v, want 3.5", emb[2])
	}
}
