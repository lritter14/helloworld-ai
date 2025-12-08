package main

import (
	_ "embed"
	"log"
	nethttp "net/http"

	"helloworld-ai/internal/config"
	"helloworld-ai/internal/http"
	"helloworld-ai/internal/llm"
	"helloworld-ai/internal/service"
	"helloworld-ai/internal/storage"
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
	defer db.Close()

	if err := storage.Migrate(db); err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}
	log.Printf("Database initialized at %s", cfg.DBPath)

	// Create repository instances (ready for Phase 5)
	vaultRepo := storage.NewVaultRepo(db)
	noteRepo := storage.NewNoteRepo(db)
	chunkRepo := storage.NewChunkRepo(db)
	_ = vaultRepo // Will be used in Phase 5
	_ = noteRepo  // Will be used in Phase 5
	_ = chunkRepo // Will be used in Phase 5

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
