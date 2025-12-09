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

	logger.InfoContext(ctx, "RAG query started",
		"question", req.Question,
		"vaults", req.Vaults,
		"folders", req.Folders,
		"k", req.K,
	)

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

	// Get all vaults to resolve names to IDs
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

	// Resolve vault names to IDs - if no vaults specified, use all vaults
	var vaultIDs []int
	if len(req.Vaults) > 0 {
		// Collect vault IDs for requested vaults
		for _, vaultName := range req.Vaults {
			if vaultID, ok := vaultMap[vaultName]; ok {
				vaultIDs = append(vaultIDs, vaultID)
			} else {
				logger.WarnContext(ctx, "unknown vault name", "vault", vaultName)
				// Continue with other vaults
			}
		}
	} else {
		// No vaults specified - search all vaults
		for _, vault := range allVaults {
			vaultIDs = append(vaultIDs, vault.ID)
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

	// Search vector store - search each vault separately and combine results
	var allSearchResults []vectorstore.SearchResult
	logger.InfoContext(ctx, "searching vector store", "vault_count", len(vaultIDs), "vault_ids", vaultIDs)
	for _, vaultID := range vaultIDs {
		filters := make(map[string]any)
		filters["vault_id"] = vaultID

		// Add folder filter if provided (prefix matching)
		if len(req.Folders) > 0 {
			// For now, use the first folder. TODO: Support multiple folders with OR filter
			filters["folder"] = req.Folders[0]
		}

		logger.DebugContext(ctx, "searching vault", "vault_id", vaultID, "filters", filters, "k", k)
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

	searchResults := allSearchResults

	logger.InfoContext(ctx, "vector search completed", "results_count", len(searchResults), "k_requested", k)
	if len(searchResults) > 0 {
		topScores := make([]float32, 0, 3)
		for i := 0; i < len(searchResults) && i < 3; i++ {
			topScores = append(topScores, searchResults[i].Score)
		}
		logger.DebugContext(ctx, "top search results", "top_3_scores", topScores)
	}

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
	for i, result := range searchResults {
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

		// Log chunk details for debugging
		textPreview := chunk.Text
		if len(textPreview) > 100 {
			textPreview = textPreview[:100] + "..."
		}
		logger.DebugContext(ctx, "retrieved chunk",
			"rank", i+1,
			"score", result.Score,
			"vault", vaultName,
			"rel_path", relPath,
			"heading_path", headingPath,
			"chunk_index", chunkIndex,
			"text_preview", textPreview,
			"text_length", len(chunk.Text),
		)
	}

	logger.InfoContext(ctx, "chunks retrieved from database", "total_chunks", len(chunks), "search_results", len(searchResults))

	// Format context string
	var contextBuilder strings.Builder
	contextBuilder.WriteString("--- Context from notes ---\n\n")

	for _, chunk := range chunks {
		contextBuilder.WriteString(fmt.Sprintf("[Vault: %s] File: %s\n", chunk.vaultName, chunk.relPath))
		contextBuilder.WriteString(fmt.Sprintf("Section: %s\n", chunk.headingPath))
		contextBuilder.WriteString(fmt.Sprintf("Content: %s\n\n", chunk.text))
	}

	contextBuilder.WriteString("--- End Context ---")

	contextString := contextBuilder.String()
	logger.InfoContext(ctx, "context formatted for LLM",
		"context_length", len(contextString),
		"chunks_included", len(chunks),
	)
	logger.DebugContext(ctx, "full context being sent to LLM", "context", contextString)

	// Construct LLM messages
	systemPrompt := "You are a helpful assistant that answers questions based on the provided context from the user's notes. " +
		"Answer the question using only the information from the context below. If the context doesn't contain " +
		"enough information to answer the question, say so. Cite specific sections when possible."

	userMessage := fmt.Sprintf("%s\n\n%s", req.Question, contextString)

	messages := []llm.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userMessage},
	}

	logger.InfoContext(ctx, "sending request to LLM",
		"question", req.Question,
		"system_prompt_length", len(systemPrompt),
		"user_message_length", len(userMessage),
		"total_context_length", len(contextString),
	)
	userMessagePreview := userMessage
	if len(userMessagePreview) > 500 {
		userMessagePreview = userMessagePreview[:500] + "..."
	}
	logger.DebugContext(ctx, "LLM messages", "system_prompt", systemPrompt, "user_message_preview", userMessagePreview)

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

	logger.InfoContext(ctx, "received LLM response", "answer_length", len(answer))
	logger.DebugContext(ctx, "LLM answer", "answer", answer)

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
