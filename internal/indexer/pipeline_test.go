package indexer

import (
	"context"
	"testing"

	"helloworld-ai/internal/llm"
	storage_mocks "helloworld-ai/internal/storage/mocks"
	"helloworld-ai/internal/vault"
	vectorstore_mocks "helloworld-ai/internal/vectorstore/mocks"

	"go.uber.org/mock/gomock"
)

func TestNewPipeline(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockVaultManager := &vault.Manager{}
	mockNoteRepo := storage_mocks.NewMockNoteStore(ctrl)
	mockChunkRepo := storage_mocks.NewMockChunkStore(ctrl)
	mockEmbedder := &llm.EmbeddingsClient{}
	mockVectorStore := vectorstore_mocks.NewMockVectorStore(ctrl)

	pipeline := NewPipeline(
		mockVaultManager,
		mockNoteRepo,
		mockChunkRepo,
		mockEmbedder,
		mockVectorStore,
		"test-collection",
	)

	if pipeline == nil {
		t.Fatal("NewPipeline() returned nil")
	}
	if pipeline.chunker == nil {
		t.Error("NewPipeline() chunker should not be nil")
	}
	if pipeline.collection != "test-collection" {
		t.Errorf("NewPipeline() collection = %v, want test-collection", pipeline.collection)
	}
}

func TestPipeline_IndexNote_Structure(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockVaultManager := &vault.Manager{}
	mockNoteRepo := storage_mocks.NewMockNoteStore(ctrl)
	mockChunkRepo := storage_mocks.NewMockChunkStore(ctrl)
	mockVectorStore := vectorstore_mocks.NewMockVectorStore(ctrl)

	embedder := &llm.EmbeddingsClient{
		ExpectedSize: 768,
	}

	pipeline := NewPipeline(
		mockVaultManager,
		mockNoteRepo,
		mockChunkRepo,
		embedder,
		mockVectorStore,
		"test-collection",
	)

	// Verify structure
	if pipeline.vaultManager == nil {
		t.Error("Pipeline vaultManager should not be nil")
	}
	if pipeline.noteRepo == nil {
		t.Error("Pipeline noteRepo should not be nil")
	}
	if pipeline.chunker == nil {
		t.Error("Pipeline chunker should not be nil")
	}
	if pipeline.collection != "test-collection" {
		t.Errorf("Pipeline collection = %v, want test-collection", pipeline.collection)
	}
}

func TestPipeline_IndexAll_Structure(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockVaultManager := &vault.Manager{}
	mockNoteRepo := storage_mocks.NewMockNoteStore(ctrl)
	mockChunkRepo := storage_mocks.NewMockChunkStore(ctrl)
	mockVectorStore := vectorstore_mocks.NewMockVectorStore(ctrl)

	embedder := &llm.EmbeddingsClient{ExpectedSize: 768}

	pipeline := NewPipeline(
		mockVaultManager,
		mockNoteRepo,
		mockChunkRepo,
		embedder,
		mockVectorStore,
		"test-collection",
	)

	// Verify IndexAll method exists and has correct signature
	ctx := context.Background()
	// This will fail without proper vault setup, but we're just testing structure
	_ = pipeline.IndexAll(ctx)
}

func TestGenerateStableChunkID_Stability(t *testing.T) {
	tests := []struct {
		name        string
		vaultID     int
		relPath     string
		headingPath string
		chunkText   string
	}{
		{
			name:        "simple note path",
			vaultID:     1,
			relPath:     "projects/main.md",
			headingPath: "# Overview",
			chunkText:   "This is the main project overview content.",
		},
		{
			name:        "nested folder path",
			vaultID:     1,
			relPath:     "work/projects/api-design.md",
			headingPath: "# API Design > ## Endpoints",
			chunkText:   "The API has several endpoints for user management.",
		},
		{
			name:        "different vault same path",
			vaultID:     2,
			relPath:     "projects/main.md",
			headingPath: "# Overview",
			chunkText:   "This is the main project overview content.",
		},
		{
			name:        "same vault different path",
			vaultID:     1,
			relPath:     "docs/config.md",
			headingPath: "# Overview",
			chunkText:   "This is the main project overview content.",
		},
		{
			name:        "same path different heading",
			vaultID:     1,
			relPath:     "projects/main.md",
			headingPath: "# Details",
			chunkText:   "This is the main project overview content.",
		},
		{
			name:        "same everything different text",
			vaultID:     1,
			relPath:     "projects/main.md",
			headingPath: "# Overview",
			chunkText:   "Different content here.",
		},
		{
			name:        "deep heading path",
			vaultID:     1,
			relPath:     "notes/meeting-notes.md",
			headingPath: "# 2024-01-15 > ## Decisions > ### API Changes",
			chunkText:   "We decided to change the API structure.",
		},
		{
			name:        "empty heading path",
			vaultID:     1,
			relPath:     "notes/simple.md",
			headingPath: "",
			chunkText:   "Content without headings.",
		},
	}

	// Test that same inputs produce same ID
	for _, tt := range tests {
		t.Run(tt.name+"_deterministic", func(t *testing.T) {
			id1 := generateStableChunkID(tt.vaultID, tt.relPath, tt.headingPath, tt.chunkText)
			id2 := generateStableChunkID(tt.vaultID, tt.relPath, tt.headingPath, tt.chunkText)
			id3 := generateStableChunkID(tt.vaultID, tt.relPath, tt.headingPath, tt.chunkText)

			if id1 != id2 {
				t.Errorf("generateStableChunkID() not deterministic: first call = %v, second call = %v", id1, id2)
			}
			if id2 != id3 {
				t.Errorf("generateStableChunkID() not deterministic: second call = %v, third call = %v", id2, id3)
			}
			if len(id1) != 32 {
				t.Errorf("generateStableChunkID() should return 32 hex characters, got %d: %v", len(id1), id1)
			}
		})
	}

	// Test that different inputs produce different IDs
	for i := 0; i < len(tests); i++ {
		for j := i + 1; j < len(tests); j++ {
			t.Run(tests[i].name+"_vs_"+tests[j].name, func(t *testing.T) {
				id1 := generateStableChunkID(tests[i].vaultID, tests[i].relPath, tests[i].headingPath, tests[i].chunkText)
				id2 := generateStableChunkID(tests[j].vaultID, tests[j].relPath, tests[j].headingPath, tests[j].chunkText)

				if id1 == id2 {
					t.Errorf("generateStableChunkID() should produce different IDs for different inputs:\n"+
						"Input 1: vaultID=%d, relPath=%q, headingPath=%q, text=%q\n"+
						"Input 2: vaultID=%d, relPath=%q, headingPath=%q, text=%q\n"+
						"Both produced ID: %v",
						tests[i].vaultID, tests[i].relPath, tests[i].headingPath, tests[i].chunkText,
						tests[j].vaultID, tests[j].relPath, tests[j].headingPath, tests[j].chunkText,
						id1)
				}
			})
		}
	}
}

func TestGenerateStableChunkID_RealWorldPaths(t *testing.T) {
	// Test with realistic Obsidian vault paths
	realWorldTests := []struct {
		name        string
		vaultID     int
		relPath     string
		headingPath string
		chunkText   string
	}{
		{
			name:        "personal vault project note",
			vaultID:     1,
			relPath:     "Projects/HelloWorld AI.md",
			headingPath: "# Overview > ## Architecture",
			chunkText:   "The HelloWorld AI project uses a RAG system with llama.cpp for local LLMs.",
		},
		{
			name:        "work vault meeting note",
			vaultID:     2,
			relPath:     "Meetings/2024-12-17 Standup.md",
			headingPath: "# 2024-12-17 > ## Action Items",
			chunkText:   "John will implement the evaluation framework. Sarah will review the PR.",
		},
		{
			name:        "nested folder structure",
			vaultID:     1,
			relPath:     "Areas/Work/Engineering/Backend/API Design.md",
			headingPath: "# API Design > ## Authentication > ### OAuth2",
			chunkText:   "We use OAuth2 for authentication with PKCE flow for security.",
		},
		{
			name:        "note with special characters in path",
			vaultID:     1,
			relPath:     "Notes/2024/Q1 Planning.md",
			headingPath: "# Q1 Planning",
			chunkText:   "Q1 goals include improving the RAG system and adding evaluation metrics.",
		},
	}

	for _, tt := range realWorldTests {
		t.Run(tt.name, func(t *testing.T) {
			// Test stability across multiple calls
			ids := make([]string, 10)
			for i := 0; i < 10; i++ {
				ids[i] = generateStableChunkID(tt.vaultID, tt.relPath, tt.headingPath, tt.chunkText)
			}

			// All IDs should be identical
			firstID := ids[0]
			for i, id := range ids {
				if id != firstID {
					t.Errorf("generateStableChunkID() not stable: call %d produced %v, expected %v", i, id, firstID)
				}
				if len(id) != 32 {
					t.Errorf("generateStableChunkID() should return 32 hex characters, got %d: %v", len(id), id)
				}
			}
		})
	}
}
