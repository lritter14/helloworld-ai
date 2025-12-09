package main

import (
	"context"
	_ "embed"
	"log"
	nethttp "net/http"

	"helloworld-ai/internal/config"
	"helloworld-ai/internal/http"
	"helloworld-ai/internal/indexer"
	"helloworld-ai/internal/llm"
	"helloworld-ai/internal/service"
	"helloworld-ai/internal/storage"
	"helloworld-ai/internal/vault"
	"helloworld-ai/internal/vectorstore"
)

//go:embed index.html
var indexHTML string

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

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
	log.Printf("Database initialized at %s", cfg.DBPath)

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
	log.Printf("Vault manager initialized (personal: %s, work: %s)", cfg.VaultPersonalPath, cfg.VaultWorkPath)
	vectorStore, err := vectorstore.NewQdrantStore(cfg.QdrantURL)
	if err != nil {
		log.Fatalf("Failed to create Qdrant client: %v", err)
	}

	// Ensure collection exists with correct vector size
	if err := vectorStore.EnsureCollection(ctx, cfg.QdrantCollection, cfg.QdrantVectorSize); err != nil {
		log.Fatalf("Failed to ensure Qdrant collection: %v", err)
	}
	log.Printf("Qdrant collection '%s' ready (vector size: %d)", cfg.QdrantCollection, cfg.QdrantVectorSize)

	// Validate embedding client vector size (fail-fast)
	embedder := llm.NewEmbeddingsClient(cfg.EmbeddingBaseURL, cfg.LLMAPIKey, cfg.EmbeddingModelName, cfg.QdrantVectorSize)
	testEmbeddings, err := embedder.EmbedTexts(ctx, []string{"test"})
	if err != nil {
		log.Fatalf("Failed to validate embedding client: %v", err)
	}
	if len(testEmbeddings) == 0 || len(testEmbeddings[0]) != cfg.QdrantVectorSize {
		log.Fatalf("Embedding vector size mismatch: expected %d, got %d", cfg.QdrantVectorSize, len(testEmbeddings[0]))
	}
	log.Printf("Embedding client validated (vector size: %d)", cfg.QdrantVectorSize)

	// Create indexing pipeline
	indexerPipeline := indexer.NewPipeline(
		vaultManager,
		noteRepo,
		chunkRepo,
		embedder,
		vectorStore,
		cfg.QdrantCollection,
	)

	// Index all vaults at startup
	log.Printf("Starting indexing of vaults...")
	if err := indexerPipeline.IndexAll(ctx); err != nil {
		log.Printf("Indexing completed with errors: %v", err)
		// Don't fail startup - log and continue per Section 0.19
	} else {
		log.Printf("Indexing completed successfully")
	}

	// Create LLM client (external service layer)
	llmClient := llm.NewClient(cfg.LLMBaseURL, cfg.LLMAPIKey, cfg.LLMModelName)

	// Create chat service (business logic layer)
	chatService := service.NewChatService(llmClient)

	// Create router with dependencies
	deps := &http.Deps{
		ChatService: chatService,
		RAGEngine:   nil, // Will be set in Phase 7
		IndexHTML:   indexHTML,
	}
	router := http.NewRouter(deps)

	// Start API server
	addr := ":" + cfg.APIPort
	log.Printf("Starting API server on %s", addr)
	log.Printf("LLM Base URL: %s", cfg.LLMBaseURL)
	log.Printf("LLM Model: %s", cfg.LLMModelName)
	if err := nethttp.ListenAndServe(addr, router); err != nil {
		log.Fatalf("API server failed to start: %v", err)
	}
}
