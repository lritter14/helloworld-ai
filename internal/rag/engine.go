package rag

import (
	"context"
	"encoding/json"
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
	noteRepo    storage.NoteStore
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
	noteRepo storage.NoteStore,
	llmClient *llm.Client,
) Engine {
	return &ragEngine{
		embedder:    embedder,
		vectorStore: vectorStore,
		collection:  collection,
		chunkRepo:   chunkRepo,
		vaultRepo:   vaultRepo,
		noteRepo:    noteRepo,
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

// truncateString truncates a string to a maximum length, appending "..." if truncated.
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// selectRelevantFolders uses LLM to rank folders by relevance to the question.
// Returns ordered list: user-provided folders first, then LLM-ranked folders.
// availableFolders format is "<vaultID>/folder" (e.g., "1/projects/work").
// userFolders format can be "<vaultID>/folder" or just "folder" (prefix matching).
// Returns folders in format "<vaultName>/folder" (e.g., "personal/workouts").
func (e *ragEngine) selectRelevantFolders(ctx context.Context, question string, availableFolders []string, userFolders []string, vaultIDs []int, vaultMap map[int]string) []string {
	logger := e.getLogger(ctx)

	// Start with user-provided folders (they are already prioritized)
	orderedFolders := make([]string, 0, len(userFolders))
	seenFolders := make(map[string]bool)

	// Add user folders first - match them to available folders
	for _, userFolder := range userFolders {
		// User folder might be in format "folder", "<vaultID>/folder", or "<vaultName>/folder"
		// Match against available folders which are in format "<vaultID>/folder"
		for _, availFolder := range availableFolders {
			// Extract folder part from available folder (after vaultID/)
			parts := strings.SplitN(availFolder, "/", 2)
			if len(parts) != 2 {
				continue
			}
			availFolderPath := parts[1] // folder path without vaultID

			// Convert available folder to vault name format for comparison
			var vaultID int
			if _, err := fmt.Sscanf(parts[0], "%d", &vaultID); err == nil {
				if vaultName, ok := vaultMap[vaultID]; ok {
					availFolderWithName := fmt.Sprintf("%s/%s", vaultName, availFolderPath)

					// Check if user folder matches (exact or prefix)
					if userFolder == availFolder || // Exact match with vaultID
						userFolder == availFolderWithName || // Exact match with vaultName
						availFolderPath == userFolder || // Exact match without vault prefix
						strings.HasPrefix(availFolderPath, userFolder+"/") || // Prefix match
						strings.HasPrefix(userFolder, availFolderPath+"/") { // User folder is more specific
						if !seenFolders[availFolder] {
							orderedFolders = append(orderedFolders, availFolder)
							seenFolders[availFolder] = true
						}
					}
				}
			}
		}
	}

	// If no available folders, return empty list
	if len(availableFolders) == 0 {
		logger.WarnContext(ctx, "no available folders for selection")
		return orderedFolders
	}

	// Filter out user folders from available list for LLM ranking
	foldersForLLM := make([]string, 0)
	for _, folder := range availableFolders {
		if !seenFolders[folder] {
			foldersForLLM = append(foldersForLLM, folder)
		}
	}

	// If no folders left for LLM, return user folders only
	if len(foldersForLLM) == 0 {
		logger.InfoContext(ctx, "all folders already selected by user", "folder_count", len(orderedFolders))
		return orderedFolders
	}

	// Convert folders to use vault names instead of IDs for LLM
	foldersWithVaultNames := make([]string, 0, len(foldersForLLM))
	vaultIDToNameMap := make(map[string]string) // Maps "vaultID/folder" -> "vaultName/folder"
	for _, folder := range foldersForLLM {
		parts := strings.SplitN(folder, "/", 2)
		if len(parts) != 2 {
			continue
		}
		var vaultID int
		if _, err := fmt.Sscanf(parts[0], "%d", &vaultID); err != nil {
			continue
		}
		vaultName, ok := vaultMap[vaultID]
		if !ok {
			continue
		}
		folderWithName := fmt.Sprintf("%s/%s", vaultName, parts[1])
		foldersWithVaultNames = append(foldersWithVaultNames, folderWithName)
		vaultIDToNameMap[folder] = folderWithName
	}

	// Build LLM prompt with improved instructions for cleaner JSON output
	folderList := strings.Join(foldersWithVaultNames, ", ")
	prompt := fmt.Sprintf(`You are a folder ranking assistant. Your task is to rank folders by relevance to answer a user's question.

Question: %s
Available folders: %s

Instructions:
- Return ONLY a valid JSON array, nothing else
- No explanations, no reasoning, no markdown formatting
- Use this exact format: ["vaultname/folder1", "vaultname/folder2", ...]
- Order folders from most relevant to least relevant
- Only include folders from the available list above

Your response (JSON array only):`, question, folderList)

	logger.InfoContext(ctx, "selecting relevant folders with LLM",
		"question_length", len(question),
		"available_folders", len(foldersForLLM),
		"user_folders", len(userFolders),
	)

	// Call LLM
	messages := []llm.Message{
		{Role: "user", Content: prompt},
	}

	llmResponse, err := e.llmClient.ChatWithMessages(ctx, messages, llm.ChatParams{
		Model:       "",  // Use default from client
		MaxTokens:   500, // Limit response size
		Temperature: 0.3, // Lower temperature for more consistent ranking
	})

	if err != nil {
		logger.WarnContext(ctx, "failed to get LLM response for folder selection, using all available folders", "error", err)
		// Fallback: add all remaining folders in original order
		orderedFolders = append(orderedFolders, foldersForLLM...)
		return orderedFolders
	}

	// Parse JSON response
	var llmRankedFolders []string
	// Try to extract JSON array from response (might have extra text)
	llmResponse = strings.TrimSpace(llmResponse)

	// First, try to find JSON array pattern in the response
	// Look for pattern: [ ... ] where ... contains quoted strings
	jsonStart := strings.Index(llmResponse, "[")
	jsonEnd := strings.LastIndex(llmResponse, "]")

	if jsonStart >= 0 && jsonEnd > jsonStart {
		// Extract the JSON array portion
		jsonCandidate := llmResponse[jsonStart : jsonEnd+1]
		// Try parsing this extracted portion
		if err := json.Unmarshal([]byte(jsonCandidate), &llmRankedFolders); err == nil {
			// Successfully parsed the extracted JSON
			logger.DebugContext(ctx, "extracted JSON array from LLM response",
				"extracted_json", jsonCandidate,
				"parsed_folders", llmRankedFolders,
			)
		} else {
			// Try removing markdown code blocks if present
			cleanedResponse := llmResponse
			if strings.HasPrefix(cleanedResponse, "```") {
				lines := strings.Split(cleanedResponse, "\n")
				if len(lines) > 1 {
					cleanedResponse = strings.Join(lines[1:len(lines)-1], "\n")
				}
				cleanedResponse = strings.TrimSpace(cleanedResponse)
			}
			// Remove json prefix if present
			if strings.HasPrefix(cleanedResponse, "json") {
				cleanedResponse = strings.TrimPrefix(cleanedResponse, "json")
				cleanedResponse = strings.TrimSpace(cleanedResponse)
			}
			// Try parsing the cleaned response
			if err := json.Unmarshal([]byte(cleanedResponse), &llmRankedFolders); err != nil {
				logger.WarnContext(ctx, "failed to parse LLM response as JSON, using all available folders", "error", err, "response_preview", truncateString(llmResponse, 200))
				// Fallback: add all remaining folders in original order
				orderedFolders = append(orderedFolders, foldersForLLM...)
				return orderedFolders
			}
		}
	} else {
		// No JSON array pattern found, try parsing the whole response after cleaning
		cleanedResponse := llmResponse
		// Remove markdown code blocks if present
		if strings.HasPrefix(cleanedResponse, "```") {
			lines := strings.Split(cleanedResponse, "\n")
			if len(lines) > 1 {
				cleanedResponse = strings.Join(lines[1:len(lines)-1], "\n")
			}
			cleanedResponse = strings.TrimSpace(cleanedResponse)
		}
		// Remove json prefix if present
		if strings.HasPrefix(cleanedResponse, "json") {
			cleanedResponse = strings.TrimPrefix(cleanedResponse, "json")
			cleanedResponse = strings.TrimSpace(cleanedResponse)
		}
		// Try parsing the cleaned response
		if err := json.Unmarshal([]byte(cleanedResponse), &llmRankedFolders); err != nil {
			logger.WarnContext(ctx, "failed to parse LLM response as JSON, using all available folders", "error", err, "response_preview", truncateString(llmResponse, 200))
			// Fallback: add all remaining folders in original order
			orderedFolders = append(orderedFolders, foldersForLLM...)
			return orderedFolders
		}
	}

	logger.DebugContext(ctx, "LLM folder ranking response",
		"llm_response_preview", truncateString(llmResponse, 500),
		"parsed_folders", llmRankedFolders,
	)

	// Filter out folders not in available list and add to ordered list
	// Convert vault names back to vault IDs for internal use
	for _, folderWithName := range llmRankedFolders {
		// Find the corresponding folder with vault ID
		found := false
		for _, availFolderWithID := range foldersForLLM {
			// Check if this matches when converted to vault name format
			expectedWithName, ok := vaultIDToNameMap[availFolderWithID]
			if ok && expectedWithName == folderWithName {
				if !seenFolders[availFolderWithID] {
					// Convert back to vault ID format for internal use
					orderedFolders = append(orderedFolders, availFolderWithID)
					seenFolders[availFolderWithID] = true
				}
				found = true
				break
			}
		}
		if !found {
			logger.DebugContext(ctx, "LLM returned folder not in available list, skipping", "folder", folderWithName)
		}
	}

	// Only return user folders and LLM-ranked folders
	// If both are empty, return all available folders
	if len(orderedFolders) == 0 && len(userFolders) == 0 && len(llmRankedFolders) == 0 {
		logger.InfoContext(ctx, "no user or LLM folders selected, returning all available folders")
		return availableFolders
	}

	logger.InfoContext(ctx, "folder selection completed",
		"user_folders", len(userFolders),
		"llm_selected", len(llmRankedFolders),
		"total_ordered", len(orderedFolders),
	)
	logger.DebugContext(ctx, "selected folders in order",
		"ordered_folders", orderedFolders,
		"user_folders", userFolders,
		"llm_ranked_folders", llmRankedFolders,
	)

	return orderedFolders
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

	// Get all unique folders for selected vaults
	availableFolders, err := e.noteRepo.ListUniqueFolders(ctx, vaultIDs)
	if err != nil {
		logger.WarnContext(ctx, "failed to list unique folders, searching all folders", "error", err)
		availableFolders = []string{} // Empty list means search all folders
	}

	// Build map of vault ID to name for folder conversion
	vaultIDToNameMap := make(map[int]string)
	for _, vault := range allVaults {
		vaultIDToNameMap[vault.ID] = vault.Name
	}

	// Select relevant folders using LLM
	orderedFolders := e.selectRelevantFolders(ctx, req.Question, availableFolders, req.Folders, vaultIDs, vaultIDToNameMap)

	logger.InfoContext(ctx, "folder selection completed",
		"available_folders", len(availableFolders),
		"ordered_folders", len(orderedFolders),
		"user_folders", len(req.Folders),
	)
	logger.DebugContext(ctx, "final ordered folder list",
		"ordered_folders", orderedFolders,
		"available_folders", availableFolders,
	)

	// Search vector store - search each vault and folder separately
	var allSearchResults []vectorstore.SearchResult
	logger.InfoContext(ctx, "searching vector store", "vault_count", len(vaultIDs), "vault_ids", vaultIDs, "folder_count", len(orderedFolders))

	// If no folders selected (neither user nor LLM selected any), search all folders (no folder filter)
	if len(orderedFolders) == 0 {
		logger.InfoContext(ctx, "no folders selected by user or LLM, searching all folders")
		for _, vaultID := range vaultIDs {
			filters := make(map[string]any)
			filters["vault_id"] = vaultID
			// No folder filter means search all folders

			logger.DebugContext(ctx, "searching vault (all folders)", "vault_id", vaultID, "k", k)
			results, err := e.vectorStore.Search(ctx, e.collection, queryVector, k, filters)
			if err != nil {
				logger.ErrorContext(ctx, "failed to search vector store", "vault_id", vaultID, "error", err)
				// Continue with other vaults
				continue
			}
			allSearchResults = append(allSearchResults, results...)
		}
	} else {
		// Search each folder separately
		// Weight scores based on folder position (earlier = higher priority)
		maxFolderWeight := float32(1.0)
		folderWeightStep := float32(0.1) // Each position reduces weight by 0.1

		for folderIdx, folderPath := range orderedFolders {
			// Parse folder path: "<vaultID>/folder"
			parts := strings.SplitN(folderPath, "/", 2)
			if len(parts) != 2 {
				logger.WarnContext(ctx, "invalid folder format, skipping", "folder", folderPath)
				continue
			}

			var vaultID int
			if _, err := fmt.Sscanf(parts[0], "%d", &vaultID); err != nil {
				logger.WarnContext(ctx, "failed to parse vault ID from folder, skipping", "folder", folderPath, "error", err)
				continue
			}

			// Check if this vault ID is in our list
			vaultInList := false
			for _, vid := range vaultIDs {
				if vid == vaultID {
					vaultInList = true
					break
				}
			}
			if !vaultInList {
				logger.DebugContext(ctx, "folder vault not in search list, skipping", "folder", folderPath, "vault_id", vaultID)
				continue
			}

			folder := parts[1] // folder path without vaultID

			filters := make(map[string]any)
			filters["vault_id"] = vaultID
			filters["folder"] = folder

			// Calculate weight for this folder (earlier folders get higher weight)
			folderWeight := maxFolderWeight - (float32(folderIdx) * folderWeightStep)
			if folderWeight < 0.1 {
				folderWeight = 0.1 // Minimum weight
			}

			logger.DebugContext(ctx, "searching folder", "vault_id", vaultID, "folder", folder, "folder_index", folderIdx, "weight", folderWeight, "k", k)
			results, err := e.vectorStore.Search(ctx, e.collection, queryVector, k, filters)
			if err != nil {
				logger.ErrorContext(ctx, "failed to search vector store", "vault_id", vaultID, "folder", folder, "error", err)
				// Continue with other folders
				continue
			}

			// Apply weight to scores based on folder position
			for i := range results {
				results[i].Score = results[i].Score * folderWeight
			}

			allSearchResults = append(allSearchResults, results...)
		}
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
