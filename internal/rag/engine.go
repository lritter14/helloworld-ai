package rag

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"helloworld-ai/internal/llm"
	"helloworld-ai/internal/storage"
	"helloworld-ai/internal/vectorstore"
)

// Engine provides RAG (Retrieval-Augmented Generation) functionality.
type Engine interface {
	// Ask answers a question using RAG by retrieving relevant chunks and generating an answer.
	Ask(ctx context.Context, req AskRequest) (AskResponse, error)
}

// ragEngine implements the Engine interface.
type ragEngine struct {
	embedder    *llm.EmbeddingsClient
	vectorStore vectorstore.VectorStore
	collection  string
	chunkRepo   storage.ChunkStore
	vaultRepo   storage.VaultStore
	llmClient   *llm.Client
	logger      *slog.Logger
}

// NewEngine creates a new RAG engine.
func NewEngine(
	embedder *llm.EmbeddingsClient,
	vectorStore vectorstore.VectorStore,
	collection string,
	chunkRepo storage.ChunkStore,
	vaultRepo storage.VaultStore,
	llmClient *llm.Client,
) Engine {
	return &ragEngine{
		embedder:    embedder,
		vectorStore: vectorStore,
		collection:  collection,
		chunkRepo:   chunkRepo,
		vaultRepo:   vaultRepo,
		llmClient:   llmClient,
		logger:      slog.Default(),
	}
}

// getLogger extracts logger from context or returns default logger.
func (e *ragEngine) getLogger(ctx context.Context) *slog.Logger {
	type loggerKeyType string
	const loggerKey loggerKeyType = "logger"
	if ctxLogger := ctx.Value(loggerKey); ctxLogger != nil {
		if l, ok := ctxLogger.(*slog.Logger); ok {
			return l
		}
	}
	return e.logger
}

// Ask answers a question using RAG.
func (e *ragEngine) Ask(ctx context.Context, req AskRequest) (AskResponse, error) {
	logger := e.getLogger(ctx)

	// Embed the question
	embeddings, err := e.embedder.EmbedTexts(ctx, []string{req.Question})
	if err != nil {
		logger.ErrorContext(ctx, "failed to embed question", "error", err)
		return AskResponse{}, fmt.Errorf("failed to embed question: %w", err)
	}
	if len(embeddings) == 0 {
		return AskResponse{}, fmt.Errorf("no embedding returned for question")
	}
	queryVector := embeddings[0]

	// Resolve vault names to IDs if provided
	var vaultIDs []int
	if len(req.Vaults) > 0 {
		allVaults, err := e.vaultRepo.ListAll(ctx)
		if err != nil {
			logger.ErrorContext(ctx, "failed to list vaults", "error", err)
			return AskResponse{}, fmt.Errorf("failed to list vaults: %w", err)
		}

		// Build map of vault name to ID
		vaultMap := make(map[string]int)
		for _, vault := range allVaults {
			vaultMap[vault.Name] = vault.ID
		}

		// Collect vault IDs for requested vaults
		for _, vaultName := range req.Vaults {
			if vaultID, ok := vaultMap[vaultName]; ok {
				vaultIDs = append(vaultIDs, vaultID)
			} else {
				logger.WarnContext(ctx, "unknown vault name", "vault", vaultName)
				// Continue with other vaults
			}
		}
	}

	// Default K to 5, enforce max 20
	k := req.K
	if k == 0 {
		k = 5
	}
	if k > 20 {
		k = 20
	}

	// Search vector store - handle multiple vaults by searching each separately
	var allSearchResults []vectorstore.SearchResult
	if len(vaultIDs) > 0 {
		// Search each vault separately and combine results
		for _, vaultID := range vaultIDs {
			filters := make(map[string]any)
			filters["vault_id"] = vaultID

			// Add folder filter if provided (prefix matching)
			if len(req.Folders) > 0 {
				// For now, use the first folder. TODO: Support multiple folders with OR filter
				filters["folder"] = req.Folders[0]
			}

			results, err := e.vectorStore.Search(ctx, e.collection, queryVector, k, filters)
			if err != nil {
				logger.ErrorContext(ctx, "failed to search vector store", "vault_id", vaultID, "error", err)
				// Continue with other vaults
				continue
			}
			allSearchResults = append(allSearchResults, results...)
		}

		// Deduplicate by PointID and sort by score (highest first)
		seen := make(map[string]bool)
		deduplicated := make([]vectorstore.SearchResult, 0, len(allSearchResults))
		for _, result := range allSearchResults {
			if !seen[result.PointID] {
				seen[result.PointID] = true
				deduplicated = append(deduplicated, result)
			}
		}

		// Sort by score (descending)
		for i := 0; i < len(deduplicated)-1; i++ {
			for j := i + 1; j < len(deduplicated); j++ {
				if deduplicated[i].Score < deduplicated[j].Score {
					deduplicated[i], deduplicated[j] = deduplicated[j], deduplicated[i]
				}
			}
		}

		// Take top K results
		if len(deduplicated) > k {
			deduplicated = deduplicated[:k]
		}
		allSearchResults = deduplicated
	} else {
		// No vault filter - search all vaults
		filters := make(map[string]any)

		// Add folder filter if provided (prefix matching)
		if len(req.Folders) > 0 {
			// For now, use the first folder. TODO: Support multiple folders with OR filter
			filters["folder"] = req.Folders[0]
		}

		var err error
		allSearchResults, err = e.vectorStore.Search(ctx, e.collection, queryVector, k, filters)
		if err != nil {
			logger.ErrorContext(ctx, "failed to search vector store", "error", err)
			return AskResponse{}, fmt.Errorf("failed to search vector store: %w", err)
		}
	}

	searchResults := allSearchResults

	if len(searchResults) == 0 {
		logger.InfoContext(ctx, "no search results found")
		return AskResponse{
			Answer:     "I couldn't find any relevant information in your notes to answer this question.",
			References: []Reference{},
		}, nil
	}

	// Fetch chunk texts from database
	type chunkData struct {
		text        string
		vaultName   string
		relPath     string
		headingPath string
		chunkIndex  int
	}

	chunks := make([]chunkData, 0, len(searchResults))
	for _, result := range searchResults {
		// Extract metadata from search result
		vaultName, _ := result.Meta["vault_name"].(string)
		relPath, _ := result.Meta["rel_path"].(string)
		headingPath, _ := result.Meta["heading_path"].(string)
		chunkIndexFloat, _ := result.Meta["chunk_index"].(float64)
		chunkIndex := int(chunkIndexFloat)

		// Fetch chunk text from database
		chunk, err := e.chunkRepo.GetByID(ctx, result.PointID)
		if err != nil {
			logger.WarnContext(ctx, "failed to fetch chunk text", "chunk_id", result.PointID, "error", err)
			continue // Skip this chunk
		}

		chunks = append(chunks, chunkData{
			text:        chunk.Text,
			vaultName:   vaultName,
			relPath:     relPath,
			headingPath: headingPath,
			chunkIndex:  chunkIndex,
		})
	}

	// Format context string
	var contextBuilder strings.Builder
	contextBuilder.WriteString("--- Context from notes ---\n\n")

	for _, chunk := range chunks {
		contextBuilder.WriteString(fmt.Sprintf("[Vault: %s] File: %s\n", chunk.vaultName, chunk.relPath))
		contextBuilder.WriteString(fmt.Sprintf("Section: %s\n", chunk.headingPath))
		contextBuilder.WriteString(fmt.Sprintf("Content: %s\n\n", chunk.text))
	}

	contextBuilder.WriteString("--- End Context ---")

	// Construct LLM messages
	systemPrompt := "You are a helpful assistant that answers questions based on the provided context from the user's notes. " +
		"Answer the question using only the information from the context below. If the context doesn't contain " +
		"enough information to answer the question, say so. Cite specific sections when possible."

	userMessage := fmt.Sprintf("%s\n\n%s", req.Question, contextBuilder.String())

	messages := []llm.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userMessage},
	}

	// Call LLM
	answer, err := e.llmClient.ChatWithMessages(ctx, messages, llm.ChatParams{
		Model:       "", // Use default from client
		MaxTokens:   0,  // No limit
		Temperature: 0.7,
	})
	if err != nil {
		logger.ErrorContext(ctx, "failed to get LLM response", "error", err)
		return AskResponse{}, fmt.Errorf("failed to get LLM response: %w", err)
	}

	// Build references from search results
	references := make([]Reference, 0, len(chunks))
	for _, chunk := range chunks {
		references = append(references, Reference{
			Vault:       chunk.vaultName,
			RelPath:     chunk.relPath,
			HeadingPath: chunk.headingPath,
			ChunkIndex:  chunk.chunkIndex,
		})
	}

	logger.InfoContext(ctx, "RAG query completed", "question_length", len(req.Question), "chunks_used", len(chunks), "answer_length", len(answer))

	return AskResponse{
		Answer:     answer,
		References: references,
	}, nil
}

