package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewClient(t *testing.T) {
	client := NewClient("http://localhost:8081", "test-key", "test-model")
	if client == nil {
		t.Fatal("NewClient() returned nil")
	}
	if client.BaseURL != "http://localhost:8081" {
		t.Errorf("NewClient() BaseURL = %v, want http://localhost:8081", client.BaseURL)
	}
	if client.APIKey != "test-key" {
		t.Errorf("NewClient() APIKey = %v, want test-key", client.APIKey)
	}
	if client.Model != "test-model" {
		t.Errorf("NewClient() Model = %v, want test-model", client.Model)
	}
	if client.client == nil {
		t.Error("NewClient() client should not be nil")
	}
}

func TestClient_Chat(t *testing.T) {
	tests := []struct {
		name       string
		message    string
		serverResp func(w http.ResponseWriter, r *http.Request)
		wantReply  string
		wantErr    bool
	}{
		{
			name:    "successful chat",
			message: "Hello",
			serverResp: func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					t.Errorf("expected POST, got %s", r.Method)
				}
				if r.URL.Path != "/v1/chat/completions" {
					t.Errorf("expected /v1/chat/completions, got %s", r.URL.Path)
				}
				if !strings.Contains(r.Header.Get("Authorization"), "Bearer") {
					t.Error("missing Authorization header")
				}

				resp := ChatResponse{
					ID:     "test-id",
					Object: "chat.completion",
					Choices: []ChatChoice{
						{
							Index: 0,
							Message: ChatChoiceMessage{
								Role:    "assistant",
								Content: "Hi there!",
							},
							FinishReason: "stop",
						},
					},
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(resp)
			},
			wantReply: "Hi there!",
			wantErr:   false,
		},
		{
			name:    "no choices returned",
			message: "Hello",
			serverResp: func(w http.ResponseWriter, r *http.Request) {
				resp := ChatResponse{
					ID:      "test-id",
					Object:  "chat.completion",
					Choices: []ChatChoice{},
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(resp)
			},
			wantErr: true,
		},
		{
			name:    "server error",
			message: "Hello",
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

			client := NewClient(server.URL, "test-key", "test-model")
			reply, err := client.Chat(context.Background(), tt.message)

			if tt.wantErr {
				if err == nil {
					t.Errorf("Chat() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("Chat() unexpected error: %v", err)
				return
			}

			if reply != tt.wantReply {
				t.Errorf("Chat() reply = %v, want %v", reply, tt.wantReply)
			}
		})
	}
}

func TestClient_StreamChat(t *testing.T) {
	tests := []struct {
		name       string
		message    string
		serverResp func(w http.ResponseWriter, r *http.Request)
		wantChunks []string
		wantErr    bool
	}{
		{
			name:    "successful streaming",
			message: "Hello",
			serverResp: func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Accept") != "text/event-stream" {
					t.Error("missing Accept header")
				}

				w.Header().Set("Content-Type", "text/event-stream")
				flusher, _ := w.(http.Flusher)

				chunks := []string{
					`{"choices":[{"delta":{"content":"Hello"}}]}`,
					`{"choices":[{"delta":{"content":" "}}]}`,
					`{"choices":[{"delta":{"content":"world"}}]}`,
					`{"choices":[{"finish_reason":"stop"}]}`,
				}

				for _, chunk := range chunks {
					_, _ = w.Write([]byte("data: " + chunk + "\n\n"))
					flusher.Flush()
				}
				_, _ = w.Write([]byte("data: [DONE]\n\n"))
			},
			wantChunks: []string{"Hello", " ", "world"},
			wantErr:    false,
		},
		{
			name:    "server error",
			message: "Hello",
			serverResp: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(tt.serverResp))
			defer server.Close()

			client := NewClient(server.URL, "test-key", "test-model")
			var receivedChunks []string

			err := client.StreamChat(context.Background(), tt.message, func(chunk string) error {
				receivedChunks = append(receivedChunks, chunk)
				return nil
			})

			if tt.wantErr {
				if err == nil {
					t.Errorf("StreamChat() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("StreamChat() unexpected error: %v", err)
				return
			}

			if len(receivedChunks) != len(tt.wantChunks) {
				t.Errorf("StreamChat() received %d chunks, want %d", len(receivedChunks), len(tt.wantChunks))
			}

			for i, chunk := range receivedChunks {
				if i < len(tt.wantChunks) && chunk != tt.wantChunks[i] {
					t.Errorf("StreamChat() chunk[%d] = %v, want %v", i, chunk, tt.wantChunks[i])
				}
			}
		})
	}
}

func TestClient_ChatWithMessages(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req ChatRequest
		_ = json.NewDecoder(r.Body).Decode(&req) // Ignore decode error in test

		if len(req.Messages) != 2 {
			t.Errorf("expected 2 messages, got %d", len(req.Messages))
		}

		resp := ChatResponse{
			ID:     "test-id",
			Object: "chat.completion",
			Choices: []ChatChoice{
				{
					Index: 0,
					Message: ChatChoiceMessage{
						Role:    "assistant",
						Content: "Response",
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key", "test-model")

	messages := []Message{
		{Role: "system", Content: "You are a helpful assistant"},
		{Role: "user", Content: "Hello"},
	}

	params := ChatParams{
		Model:       "custom-model",
		MaxTokens:   100,
		Temperature: 0.7,
	}

	reply, err := client.ChatWithMessages(context.Background(), messages, params)
	if err != nil {
		t.Fatalf("ChatWithMessages() error = %v", err)
	}

	if reply != "Response" {
		t.Errorf("ChatWithMessages() reply = %v, want Response", reply)
	}
}

func TestClient_ChatWithMessages_DefaultModel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req ChatRequest
		_ = json.NewDecoder(r.Body).Decode(&req) // Ignore decode error in test

		if req.Model != "test-model" {
			t.Errorf("expected model test-model, got %s", req.Model)
		}

		resp := ChatResponse{
			ID:     "test-id",
			Object: "chat.completion",
			Choices: []ChatChoice{
				{
					Index: 0,
					Message: ChatChoiceMessage{
						Content: "Response",
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key", "test-model")

	messages := []Message{
		{Role: "user", Content: "Hello"},
	}

	params := ChatParams{} // Empty model should use client default

	reply, err := client.ChatWithMessages(context.Background(), messages, params)
	if err != nil {
		t.Fatalf("ChatWithMessages() error = %v", err)
	}

	if reply != "Response" {
		t.Errorf("ChatWithMessages() reply = %v, want Response", reply)
	}
}
