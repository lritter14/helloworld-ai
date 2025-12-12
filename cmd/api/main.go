package main

import (
	"context"
	"log"
	"log/slog"
	nethttp "net/http"
	"os"

	"helloworld-ai/internal/config"
	"helloworld-ai/internal/http"
	"helloworld-ai/internal/indexer"
	"helloworld-ai/internal/llm"
	"helloworld-ai/internal/rag"
	"helloworld-ai/internal/storage"
	"helloworld-ai/internal/vault"
	"helloworld-ai/internal/vectorstore"
)

//go:generate swagger generate spec -o swagger.json

// General API information
//
// This API provides RAG (Retrieval-Augmented Generation) functionality for querying indexed markdown notes from Obsidian vaults.
//
// swagger:meta
//
// ---
// swagger: '2.0'
// info:
//   title: HelloWorld AI API
//   description: |
//     RAG (Retrieval-Augmented Generation) API for querying indexed markdown notes from Obsidian vaults.
//     The API allows you to ask questions and get answers based on content indexed from your vaults.
//   version: 1.0.0
// schemes:
//   - http
//   - https
// consumes:
//   - application/json
// produces:
//   - application/json

func main() {
	// Load configuration first (needed for log level)
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Configure structured logging with configurable level and format
	opts := &slog.HandlerOptions{
		Level: cfg.LogLevel,
	}
	var handler slog.Handler
	if cfg.LogFormat == "json" {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}
	logger := slog.New(handler)
	slog.SetDefault(logger)
	slog.Debug("Logging configured", "level", cfg.LogLevel.String(), "format", cfg.LogFormat)

	// Initialize database
	db, err := storage.New(cfg.DBPath)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer func() {
		_ = db.Close()
	}()

	if err := storage.Migrate(db); err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}
	slog.Info("Database initialized", "path", cfg.DBPath)

	// Create repository instances
	vaultRepo := storage.NewVaultRepo(db)
	noteRepo := storage.NewNoteRepo(db)
	chunkRepo := storage.NewChunkRepo(db)

	// Initialize Qdrant vector store
	ctx := context.Background()

	// Initialize vault manager
	vaultManager, err := vault.NewManager(ctx, vaultRepo, cfg.VaultPersonalPath, cfg.VaultWorkPath)
	if err != nil {
		log.Fatalf("Failed to initialize vault manager: %v", err)
	}
	slog.Info("Vault manager initialized", "personal", cfg.VaultPersonalPath, "work", cfg.VaultWorkPath)
	vectorStore, err := vectorstore.NewQdrantStore(cfg.QdrantURL)
	if err != nil {
		log.Fatalf("Failed to create Qdrant client: %v", err)
	}

	// Ensure collection exists with correct vector size
	if err := vectorStore.EnsureCollection(ctx, cfg.QdrantCollection, cfg.QdrantVectorSize); err != nil {
		log.Fatalf("Failed to ensure Qdrant collection: %v", err)
	}
	slog.Info("Qdrant collection ready", "collection", cfg.QdrantCollection, "vector_size", cfg.QdrantVectorSize)

	// Validate embedding client vector size (fail-fast)
	embedder := llm.NewEmbeddingsClient(cfg.EmbeddingBaseURL, cfg.LLMAPIKey, cfg.EmbeddingModelName, cfg.QdrantVectorSize)
	testEmbeddings, err := embedder.EmbedTexts(ctx, []string{"test"})
	if err != nil {
		log.Fatalf("Failed to validate embedding client: %v", err)
	}
	if len(testEmbeddings) == 0 || len(testEmbeddings[0]) != cfg.QdrantVectorSize {
		log.Fatalf("Embedding vector size mismatch: expected %d, got %d", cfg.QdrantVectorSize, len(testEmbeddings[0]))
	}
	slog.Info("Embedding client validated", "vector_size", cfg.QdrantVectorSize)

	// Create indexing pipeline
	indexerPipeline := indexer.NewPipeline(
		vaultManager,
		noteRepo,
		chunkRepo,
		embedder,
		vectorStore,
		cfg.QdrantCollection,
	)

	// Create LLM client (external service layer)
	llmClient := llm.NewClient(cfg.LLMBaseURL, cfg.LLMAPIKey, cfg.LLMModelName)

	// Create RAG engine
	ragEngine := rag.NewEngine(
		embedder,
		vectorStore,
		cfg.QdrantCollection,
		chunkRepo,
		vaultRepo,
		noteRepo,
		llmClient,
	)
	slog.Info("RAG engine initialized")

	// Create router with dependencies
	deps := &http.Deps{
		RAGEngine:       ragEngine,
		VaultRepo:       vaultRepo,
		IndexerPipeline: indexerPipeline,
		VaultManager:    vaultManager,
		VectorStore:     vectorStore,
		LLMClient:       llmClient,
		CollectionName:  cfg.QdrantCollection,
	}
	router := http.NewRouter(deps)

	// Start indexing in background after router is ready
	go func() {
		indexCtx := context.Background()
		slog.Info("Starting background indexing of vaults")
		if err := indexerPipeline.IndexAll(indexCtx); err != nil {
			slog.Error("Indexing completed with errors", "error", err)
		} else {
			slog.Info("Indexing completed successfully")
		}
	}()

	// Start API server
	addr := ":" + cfg.APIPort
	slog.Info("Starting API server", "addr", addr)
	slog.Debug("LLM configuration", "base_url", cfg.LLMBaseURL, "model", cfg.LLMModelName)
	if err := nethttp.ListenAndServe(addr, router); err != nil {
		log.Fatalf("API server failed to start: %v", err)
	}
}
