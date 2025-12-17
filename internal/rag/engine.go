package rag

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"helloworld-ai/internal/contextutil"
	"helloworld-ai/internal/llm"
	"helloworld-ai/internal/storage"
	"helloworld-ai/internal/vectorstore"
)

const (
	minAutoK                = 3
	defaultAutoK            = 5
	maxAutoK                = 8
	candidateKPerScope      = 15
	maxCandidates           = 200
	rerankKeep              = maxAutoK
	vectorScoreWeight       = 0.7
	lexicalScoreWeight      = 0.3
	minVectorScoreThreshold = 0.3
	minFinalScoreThreshold  = 0.4
)

type rerankCandidate struct {
	result       vectorstore.SearchResult
	chunk        *storage.ChunkRecord
	vaultName    string
	relPath      string
	headingPath  string
	chunkIndex   int
	vectorScore  float32
	lexicalScore float32
	finalScore   float32
	originalRank int
}

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
	}
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
	logger := contextutil.LoggerFromContext(ctx)

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
- Only include folders that are DIRECTLY relevant to answering the question
- Exclude folders that are only tangentially related
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

	// Check for empty response
	llmResponse = strings.TrimSpace(llmResponse)
	if llmResponse == "" {
		logger.WarnContext(ctx, "LLM returned empty response for folder selection, using all available folders",
			"prompt_length", len(prompt),
			"folder_count", len(foldersWithVaultNames),
		)
		// Fallback: add all remaining folders in original order
		orderedFolders = append(orderedFolders, foldersForLLM...)
		return orderedFolders
	}

	// Parse JSON response
	var llmRankedFolders []string

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
	logger := contextutil.LoggerFromContext(ctx)

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

	autoK := determineAutoK(req.Question, req.Folders, req.Detail)
	userHintK := clampUserProvidedK(req.K)
	targetK := autoK
	kSource := "auto"
	if userHintK > 0 {
		targetK = userHintK
		kSource = "user_override"
	}

	logger.InfoContext(ctx, "k selection completed",
		"auto_k", autoK,
		"user_hint_k", userHintK,
		"target_k", targetK,
		"detail_hint", req.Detail,
		"k_source", kSource,
	)

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
	logger.InfoContext(ctx, "searching vector store",
		"vault_count", len(vaultIDs),
		"vault_ids", vaultIDs,
		"folder_count", len(orderedFolders),
		"candidate_k_per_scope", candidateKPerScope,
	)

	// If no folders selected (neither user nor LLM selected any), search all folders (no folder filter)
	if len(orderedFolders) == 0 {
		logger.InfoContext(ctx, "no folders selected by user or LLM, searching all folders")
		for _, vaultID := range vaultIDs {
			filters := make(map[string]any)
			filters["vault_id"] = vaultID
			// No folder filter means search all folders

			logger.DebugContext(ctx, "searching vault (all folders)", "vault_id", vaultID, "k", candidateKPerScope)
			results, err := e.vectorStore.Search(ctx, e.collection, queryVector, candidateKPerScope, filters)
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

			logger.DebugContext(ctx, "searching folder", "vault_id", vaultID, "folder", folder, "folder_index", folderIdx, "weight", folderWeight, "k", candidateKPerScope)
			results, err := e.vectorStore.Search(ctx, e.collection, queryVector, candidateKPerScope, filters)
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

	sort.Slice(deduplicated, func(i, j int) bool {
		return deduplicated[i].Score > deduplicated[j].Score
	})

	logger.InfoContext(ctx, "deduplicated vector results",
		"raw_count", len(allSearchResults),
		"deduplicated_count", len(deduplicated),
	)

	if len(deduplicated) == 0 {
		logger.InfoContext(ctx, "no search results found")
		return AskResponse{
			Answer:     "I couldn't find any relevant information in your notes to answer this question.",
			References: []Reference{},
		}, nil
	}

	if len(deduplicated) > maxCandidates {
		logger.InfoContext(ctx, "trimming candidates to global cap",
			"before_trim", len(deduplicated),
			"cap", maxCandidates,
		)
		deduplicated = deduplicated[:maxCandidates]
	}

	// Fetch chunk texts and compute lexical scores for reranking
	candidates := make([]rerankCandidate, 0, len(deduplicated))
	for idx, result := range deduplicated {
		vectorScore := result.Score
		if vectorScore < minVectorScoreThreshold {
			logger.DebugContext(ctx, "skipping candidate below vector threshold",
				"point_id", result.PointID,
				"vector_score", vectorScore,
			)
			continue
		}

		chunk, err := e.chunkRepo.GetByID(ctx, result.PointID)
		if err != nil {
			logger.WarnContext(ctx, "failed to fetch chunk text during rerank", "chunk_id", result.PointID, "error", err)
			continue
		}

		vaultName, _ := result.Meta["vault_name"].(string)
		relPath, _ := result.Meta["rel_path"].(string)
		headingPath := chunk.HeadingPath
		if headingPath == "" {
			headingPath, _ = result.Meta["heading_path"].(string)
		}
		chunkIndex := chunk.ChunkIndex
		if chunkIndex == 0 {
			if chunkIndexFloat, ok := result.Meta["chunk_index"].(float64); ok {
				chunkIndex = int(chunkIndexFloat)
			}
		}

		lexScore := lexicalScore(req.Question, chunk.Text, headingPath)
		finalScore := combineScores(vectorScore, lexScore)
		candidates = append(candidates, rerankCandidate{
			result:       result,
			chunk:        chunk,
			vaultName:    vaultName,
			relPath:      relPath,
			headingPath:  headingPath,
			chunkIndex:   chunkIndex,
			vectorScore:  vectorScore,
			lexicalScore: lexScore,
			finalScore:   finalScore,
			originalRank: idx + 1,
		})
	}

	if len(candidates) == 0 {
		logger.InfoContext(ctx, "no candidates passed vector threshold after rerank preparation")
		return AskResponse{
			Answer:     "I couldn't find any relevant information in your notes to answer this question.",
			References: []Reference{},
		}, nil
	}

	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].finalScore == candidates[j].finalScore {
			return candidates[i].vectorScore > candidates[j].vectorScore
		}
		return candidates[i].finalScore > candidates[j].finalScore
	})

	filteredCandidates := make([]rerankCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.finalScore < minFinalScoreThreshold {
			logger.DebugContext(ctx, "candidate dropped by final score",
				"point_id", candidate.result.PointID,
				"final_score", candidate.finalScore,
				"vector_score", candidate.vectorScore,
				"lexical_score", candidate.lexicalScore,
			)
			continue
		}
		filteredCandidates = append(filteredCandidates, candidate)
	}

	logger.InfoContext(ctx, "rerank completed",
		"candidates_considered", len(candidates),
		"candidates_after_threshold", len(filteredCandidates),
		"target_k", targetK,
	)

	if len(filteredCandidates) == 0 {
		logger.InfoContext(ctx, "no candidates met final score threshold")
		return AskResponse{
			Answer:     "I couldn't find any relevant information in your notes to answer this question.",
			References: []Reference{},
		}, nil
	}

	// Determine final chunk count respecting rerank cap
	finalCount := targetK
	if finalCount > rerankKeep {
		finalCount = rerankKeep
	}
	if finalCount > len(filteredCandidates) {
		finalCount = len(filteredCandidates)
	}
	if finalCount <= 0 {
		finalCount = len(filteredCandidates)
	}

	selectedCandidates := filteredCandidates[:finalCount]

	// Log top candidate scores to aid tuning
	logPreview := make([]map[string]any, 0, len(selectedCandidates))
	for i := 0; i < len(selectedCandidates) && i < 5; i++ {
		candidate := selectedCandidates[i]
		logPreview = append(logPreview, map[string]any{
			"rank":          i + 1,
			"point_id":      candidate.result.PointID,
			"vector_score":  candidate.vectorScore,
			"lexical_score": candidate.lexicalScore,
			"final_score":   candidate.finalScore,
		})
	}
	logger.DebugContext(ctx, "top reranked candidates", "preview", logPreview)

	type chunkData struct {
		text        string
		vaultName   string
		relPath     string
		headingPath string
		chunkIndex  int
		result      vectorstore.SearchResult
	}

	chunks := make([]chunkData, 0, len(selectedCandidates))
	for rank, candidate := range selectedCandidates {
		chunks = append(chunks, chunkData{
			text:        candidate.chunk.Text,
			vaultName:   candidate.vaultName,
			relPath:     candidate.relPath,
			headingPath: candidate.headingPath,
			chunkIndex:  candidate.chunkIndex,
			result:      candidate.result,
		})

		textPreview := candidate.chunk.Text
		if len(textPreview) > 100 {
			textPreview = textPreview[:100] + "..."
		}
		logger.DebugContext(ctx, "selected chunk",
			"rank", rank+1,
			"final_score", candidate.finalScore,
			"vector_score", candidate.vectorScore,
			"lexical_score", candidate.lexicalScore,
			"vault", candidate.vaultName,
			"rel_path", candidate.relPath,
			"heading_path", candidate.headingPath,
			"chunk_index", candidate.chunkIndex,
			"text_preview", textPreview,
			"text_length", len(candidate.chunk.Text),
		)
	}

	logger.InfoContext(ctx, "chunks selected after rerank",
		"total_selected", len(chunks),
		"requested_k", targetK,
		"rerank_cap", rerankKeep,
	)

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

	resp := AskResponse{
		Answer:     answer,
		References: references,
	}

	// Collect debug information if requested
	if req.Debug {
		debugInfo := e.buildDebugInfo(ctx, deduplicated, candidates, selectedCandidates, orderedFolders, availableFolders, vaultIDToNameMap)
		resp.Debug = debugInfo
	}

	return resp, nil
}

// buildDebugInfo constructs debug information from retrieval results.
func (e *ragEngine) buildDebugInfo(
	ctx context.Context,
	deduplicated []vectorstore.SearchResult,
	candidates []rerankCandidate,
	selectedCandidates []rerankCandidate,
	orderedFolders []string,
	availableFolders []string,
	vaultIDToNameMap map[int]string,
) *DebugInfo {
	logger := contextutil.LoggerFromContext(ctx)

	// Build retrieved chunks list from all candidates (before final selection)
	retrievedChunks := make([]RetrievedChunk, 0, len(candidates))
	for rank, candidate := range candidates {
		retrievedChunks = append(retrievedChunks, RetrievedChunk{
			ChunkID:     candidate.result.PointID,
			RelPath:     candidate.relPath,
			HeadingPath: candidate.headingPath,
			ScoreVector: float64(candidate.vectorScore),
			ScoreLexical: float64(candidate.lexicalScore),
			ScoreFinal:  float64(candidate.finalScore),
			Text:        candidate.chunk.Text,
			Rank:        rank + 1,
		})
	}

	// Convert folder format from "vaultID/folder" to "vaultName/folder" for display
	displayOrderedFolders := make([]string, 0, len(orderedFolders))
	for _, folder := range orderedFolders {
		parts := strings.SplitN(folder, "/", 2)
		if len(parts) == 2 {
			var vaultID int
			if _, err := fmt.Sscanf(parts[0], "%d", &vaultID); err == nil {
				if vaultName, ok := vaultIDToNameMap[vaultID]; ok {
					displayOrderedFolders = append(displayOrderedFolders, fmt.Sprintf("%s/%s", vaultName, parts[1]))
				} else {
					displayOrderedFolders = append(displayOrderedFolders, folder)
				}
			} else {
				displayOrderedFolders = append(displayOrderedFolders, folder)
			}
		} else {
			displayOrderedFolders = append(displayOrderedFolders, folder)
		}
	}

	displayAvailableFolders := make([]string, 0, len(availableFolders))
	for _, folder := range availableFolders {
		parts := strings.SplitN(folder, "/", 2)
		if len(parts) == 2 {
			var vaultID int
			if _, err := fmt.Sscanf(parts[0], "%d", &vaultID); err == nil {
				if vaultName, ok := vaultIDToNameMap[vaultID]; ok {
					displayAvailableFolders = append(displayAvailableFolders, fmt.Sprintf("%s/%s", vaultName, parts[1]))
				} else {
					displayAvailableFolders = append(displayAvailableFolders, folder)
				}
			} else {
				displayAvailableFolders = append(displayAvailableFolders, folder)
			}
		} else {
			displayAvailableFolders = append(displayAvailableFolders, folder)
		}
	}

	logger.DebugContext(ctx, "building debug info",
		"retrieved_chunks_count", len(retrievedChunks),
		"selected_folders_count", len(displayOrderedFolders),
	)

	return &DebugInfo{
		RetrievedChunks: retrievedChunks,
		FolderSelection: &FolderSelection{
			SelectedFolders:  displayOrderedFolders,
			AvailableFolders: displayAvailableFolders,
		},
	}
}

func combineScores(vectorScore, lexicalScore float32) float32 {
	return (vectorScore * vectorScoreWeight) + (lexicalScore * lexicalScoreWeight)
}

var broadQueryKeywords = []string{
	"overview", "summary", "summaries", "all", "everything", "compare", "comparison",
	"list", "recap", "broad", "topics", "outline",
}

func clampAutoK(value int) int {
	if value < minAutoK {
		return minAutoK
	}
	if value > maxAutoK {
		return maxAutoK
	}
	return value
}

func clampUserProvidedK(value int) int {
	if value <= 0 {
		return 0
	}
	if value < minAutoK {
		return minAutoK
	}
	if value > maxAutoK {
		return maxAutoK
	}
	return value
}

func determineAutoK(question string, folders []string, detail string) int {
	k := defaultAutoK
	switch strings.ToLower(detail) {
	case "brief":
		k = minAutoK
	case "detailed":
		k = maxAutoK
	case "normal":
		k = defaultAutoK
	}

	meaningfulTokens, uniqueTokenCount := analyzeQuestionTokens(question)

	if len(meaningfulTokens) >= 12 || uniqueTokenCount >= 10 {
		k++
	} else if len(meaningfulTokens) > 0 && len(meaningfulTokens) <= 4 {
		k--
	}

	lowerQuestion := strings.ToLower(question)
	containsBroadKeyword := false
	for _, kw := range broadQueryKeywords {
		if strings.Contains(lowerQuestion, kw) {
			containsBroadKeyword = true
			k++
			break
		}
	}

	if strings.Count(question, "?") > 1 {
		k++
	}
	if len(question) > 200 {
		k++
	}
	if len(folders) > 0 && !containsBroadKeyword {
		k--
	}

	return clampAutoK(k)
}

func analyzeQuestionTokens(question string) ([]string, int) {
	tokens := tokenize(question)
	if len(tokens) == 0 {
		return nil, 0
	}
	meaningful := filterStopwords(tokens)
	if len(meaningful) == 0 {
		return nil, 0
	}

	unique := make(map[string]struct{}, len(meaningful))
	for _, token := range meaningful {
		unique[token] = struct{}{}
	}
	return meaningful, len(unique)
}
