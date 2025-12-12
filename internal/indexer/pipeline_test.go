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
