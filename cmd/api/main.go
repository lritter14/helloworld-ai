package main

import (
	"context"
	"errors"
	"log"
	"log/slog"
	nethttp "net/http"
	"os"
	"path/filepath"
	"time"

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

	// Load models into llama.cpp server (router mode)
	// This ensures models are available before we try to use them
	modelLoader := llm.NewModelLoader(cfg.LLMBaseURL)

	// Get absolute path to models directory (relative to project root)
	// This helps avoid relative path resolution issues when llama.cpp spawns subprocesses
	wd, err := os.Getwd()
	if err != nil {
		log.Fatalf("Failed to get working directory: %v", err)
	}
	modelsDir := filepath.Join(wd, "..", "llama.cpp", "models")
	absModelsDir, err := filepath.Abs(modelsDir)
	if err != nil {
		slog.Warn("Failed to resolve absolute models directory, using relative path",
			"models_dir", modelsDir,
			"error", err)
		absModelsDir = modelsDir
	}

	// Load chat model
	chatModelPath := filepath.Join(absModelsDir, cfg.LLMModelName+".gguf")
	chatModelArgs := []string{
		"--ctx-size", "8192",
		"--threads", "8",
		"--batch-size", "384",
		"--ubatch-size", "96",
		"--model", chatModelPath, // Use absolute path to avoid relative path resolution issues
	}
	// Check if already loaded before attempting to load
	chatLoaded, err := modelLoader.IsModelLoaded(ctx, cfg.LLMModelName)
	if err != nil {
		slog.Warn("Failed to check if chat model is loaded, attempting to load",
			"model", cfg.LLMModelName,
			"error", err)
		// Attempt to load even if check failed
		if err := modelLoader.LoadModel(ctx, cfg.LLMModelName, chatModelArgs); err != nil {
			slog.Warn("Failed to load chat model (will be loaded on first use)",
				"model", cfg.LLMModelName,
				"error", err)
		} else {
			slog.Info("Chat model loaded", "model", cfg.LLMModelName)
		}
		// Wait 10 seconds before loading next model (after any load attempt)
		slog.Info("Waiting 10 seconds before loading next model...")
		time.Sleep(10 * time.Second)
	} else if chatLoaded {
		slog.Info("Chat model already loaded", "model", cfg.LLMModelName)
		// No delay needed if already loaded
	} else {
		// Model not loaded, attempt to load it
		if err := modelLoader.LoadModel(ctx, cfg.LLMModelName, chatModelArgs); err != nil {
			slog.Warn("Failed to load chat model (will be loaded on first use)",
				"model", cfg.LLMModelName,
				"error", err)
		} else {
			slog.Info("Chat model loaded", "model", cfg.LLMModelName)
		}
		// Wait 10 seconds before loading next model (after any load attempt)
		slog.Info("Waiting 10 seconds before loading next model...")
		time.Sleep(10 * time.Second)
	}

	// Load embeddings model
	embeddingModelPath := filepath.Join(absModelsDir, cfg.EmbeddingModelName+".gguf")
	embeddingModelArgs := []string{
		"--embeddings",
		"--pooling", "mean",
		"--ctx-size", "2048",
		"--ubatch-size", "2048",
		"--model", embeddingModelPath, // Use absolute path to avoid relative path resolution issues
	}
	// Check if already loaded before attempting to load
	embeddingLoaded, err := modelLoader.IsModelLoaded(ctx, cfg.EmbeddingModelName)
	if err != nil {
		slog.Warn("Failed to check if embedding model is loaded, attempting to load",
			"model", cfg.EmbeddingModelName,
			"error", err)
	} else if embeddingLoaded {
		slog.Info("Embedding model already loaded", "model", cfg.EmbeddingModelName)
	} else {
		if err := modelLoader.LoadModel(ctx, cfg.EmbeddingModelName, embeddingModelArgs); err != nil {
			slog.Warn("Failed to load embedding model (will be loaded on first use)",
				"model", cfg.EmbeddingModelName,
				"error", err)
		} else {
			slog.Info("Embedding model loaded", "model", cfg.EmbeddingModelName)
		}
	}

	// Validate embedding client vector size (fail-fast)
	embedder := llm.NewEmbeddingsClient(cfg.EmbeddingBaseURL, cfg.LLMAPIKey, cfg.EmbeddingModelName, cfg.QdrantVectorSize)
	testEmbeddings, err := embedder.EmbedTexts(ctx, []string{"test"})
	if err != nil {
		// Check if error is due to model not being loaded (router mode)
		var embedErr *llm.EmbeddingError
		if errors.As(err, &embedErr) && embedErr.IsModelNotFoundError() {
			slog.Warn("Embedding model not loaded yet",
				"model", cfg.EmbeddingModelName,
				"message", "Model will be loaded on first use. Use scripts/load-models.sh to pre-load models.")
		} else {
			log.Fatalf("Failed to validate embedding client: %v", err)
		}
	} else {
		// Model is loaded, validate vector size
		if len(testEmbeddings) == 0 || len(testEmbeddings[0]) != cfg.QdrantVectorSize {
			log.Fatalf("Embedding vector size mismatch: expected %d, got %d", cfg.QdrantVectorSize, len(testEmbeddings[0]))
		}
		slog.Info("Embedding client validated", "vector_size", cfg.QdrantVectorSize)
	}

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
		RAGEngine:          ragEngine,
		VaultRepo:          vaultRepo,
		IndexerPipeline:    indexerPipeline,
		VaultManager:       vaultManager,
		VectorStore:        vectorStore,
		LLMClient:          llmClient,
		CollectionName:     cfg.QdrantCollection,
		EmbeddingModelName: cfg.EmbeddingModelName,
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
