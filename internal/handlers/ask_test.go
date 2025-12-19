package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"helloworld-ai/internal/rag"
	storage_mocks "helloworld-ai/internal/storage/mocks"

	"go.uber.org/mock/gomock"
)

func TestAskHandler_DebugMode(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRAGEngine := &mockRAGEngine{}
	mockVaultRepo := storage_mocks.NewMockVaultStore(ctrl)

	handler := NewAskHandler(mockRAGEngine, mockVaultRepo, nil, "")

	tests := []struct {
		name           string
		debugParam     string
		expectDebug    bool
		requestBody    AskRequest
		ragResponse    rag.AskResponse
		expectedStatus int
	}{
		{
			name:        "debug mode enabled via true",
			debugParam:  "true",
			expectDebug: true,
			requestBody: AskRequest{
				Question: "What is the project about?",
			},
			ragResponse: rag.AskResponse{
				Answer: "The project is about RAG systems.",
				References: []rag.Reference{
					{
						Vault:       "personal",
						RelPath:     "projects/main.md",
						HeadingPath: "# Overview",
						ChunkIndex:  0,
					},
				},
				Debug: &rag.DebugInfo{
					RetrievedChunks: []rag.RetrievedChunk{
						{
							ChunkID:     "abc123",
							RelPath:     "projects/main.md",
							HeadingPath: "# Overview",
							ScoreVector: 0.95,
							ScoreLexical: 0.80,
							ScoreFinal:  0.90,
							Text:        "The project is about RAG systems...",
							Rank:        1,
						},
					},
					FolderSelection: &rag.FolderSelection{
						SelectedFolders: []string{"personal/projects"},
					},
				},
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:        "debug mode enabled via 1",
			debugParam:  "1",
			expectDebug: true,
			requestBody: AskRequest{
				Question: "What is the project about?",
			},
			ragResponse: rag.AskResponse{
				Answer:     "The project is about RAG systems.",
				References: []rag.Reference{},
				Debug: &rag.DebugInfo{
					RetrievedChunks: []rag.RetrievedChunk{},
					FolderSelection: &rag.FolderSelection{
						SelectedFolders: []string{},
					},
				},
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:        "debug mode disabled",
			debugParam:  "false",
			expectDebug: false,
			requestBody: AskRequest{
				Question: "What is the project about?",
			},
			ragResponse: rag.AskResponse{
				Answer:     "The project is about RAG systems.",
				References: []rag.Reference{},
				Debug:      nil,
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:        "debug mode not specified",
			debugParam:  "",
			expectDebug: false,
			requestBody: AskRequest{
				Question: "What is the project about?",
			},
			ragResponse: rag.AskResponse{
				Answer:     "The project is about RAG systems.",
				References: []rag.Reference{},
				Debug:      nil,
			},
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRAGEngine.reset()
			mockRAGEngine.response = tt.ragResponse

			// Create request body
			body, err := json.Marshal(tt.requestBody)
			if err != nil {
				t.Fatalf("failed to marshal request body: %v", err)
			}

			// Create request with debug parameter
			req := httptest.NewRequest(http.MethodPost, "/api/v1/ask", bytes.NewReader(body))
			if tt.debugParam != "" {
				req.URL.RawQuery = "debug=" + tt.debugParam
			}
			req.Header.Set("Content-Type", "application/json")

			// Create response recorder
			w := httptest.NewRecorder()

			// Call handler
			handler.ServeHTTP(w, req)

			// Check status code
			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}

			// Check that debug flag was passed correctly
			if mockRAGEngine.lastRequest.Debug != tt.expectDebug {
				t.Errorf("expected debug flag %v, got %v", tt.expectDebug, mockRAGEngine.lastRequest.Debug)
			}

			// Parse response
			var resp AskResponse
			if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
				t.Fatalf("failed to decode response: %v", err)
			}

			// Check debug info in response
			if tt.expectDebug && tt.ragResponse.Debug != nil {
				if resp.Debug == nil {
					t.Error("expected debug info in response, got nil")
				} else {
					if len(resp.Debug.RetrievedChunks) != len(tt.ragResponse.Debug.RetrievedChunks) {
						t.Errorf("expected %d retrieved chunks, got %d",
							len(tt.ragResponse.Debug.RetrievedChunks),
							len(resp.Debug.RetrievedChunks))
					}
					if resp.Debug.FolderSelection == nil && tt.ragResponse.Debug.FolderSelection != nil {
						t.Error("expected folder selection in debug info, got nil")
					}
				}
			} else {
				if resp.Debug != nil {
					t.Errorf("expected no debug info in response, got %+v", resp.Debug)
				}
			}
		})
	}
}

// mockRAGEngine is a simple mock for testing
type mockRAGEngine struct {
	lastRequest rag.AskRequest
	response    rag.AskResponse
	err         error
}

func (m *mockRAGEngine) reset() {
	m.lastRequest = rag.AskRequest{}
	m.response = rag.AskResponse{}
	m.err = nil
}

func (m *mockRAGEngine) Ask(ctx context.Context, req rag.AskRequest) (rag.AskResponse, error) {
	m.lastRequest = req
	if m.err != nil {
		return rag.AskResponse{}, m.err
	}
	return m.response, nil
}

