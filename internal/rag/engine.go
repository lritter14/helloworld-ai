package rag

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

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

// chunkData represents a chunk with its metadata for context formatting and citation extraction.
type chunkData struct {
	text        string
	vaultName   string
	relPath     string
	headingPath string
	chunkIndex  int
	result      vectorstore.SearchResult
}

// normalizePath normalizes a file path for comparison by:
// - Removing trailing slashes
// - Normalizing path separators
// - Converting to lowercase for case-insensitive comparison
func normalizePath(path string) string {
	normalized := strings.TrimSpace(path)
	normalized = filepath.Clean(normalized)
	normalized = strings.ToLower(normalized)
	return normalized
}

// matchFilePath attempts to match a cited file path against a chunk's file path using multiple strategies:
// 1. Exact match after normalization
// 2. Basename matching (only if one path has no directory components)
// 3. Path component matching (split by "/" and compare components)
// 4. Suffix matching (chunk path ends with cited path)
func matchFilePath(citedPath, chunkPath string) bool {
	// Normalize both paths
	normalizedCited := normalizePath(citedPath)
	normalizedChunk := normalizePath(chunkPath)

	// Strategy 1: Exact match after normalization
	if normalizedCited == normalizedChunk {
		return true
	}

	// Strategy 2: Basename matching
	// This handles cases like "file.md" matching "folder/file.md" or vice versa
	citedBasename := strings.ToLower(filepath.Base(citedPath))
	chunkBasename := strings.ToLower(filepath.Base(chunkPath))
	if citedBasename == chunkBasename && citedBasename != "" {
		// Allow basename match if:
		// - Cited path has no directory (e.g., "file.md" matches "folder/file.md")
		// - Chunk path has no directory (e.g., "folder/file.md" matches "file.md")
		// - Chunk path ends with cited path (e.g., "parent/folder/file.md" matches "folder/file.md")
		if !strings.Contains(citedPath, "/") ||
			!strings.Contains(chunkPath, "/") ||
			strings.HasSuffix(normalizedChunk, "/"+normalizedCited) {
			return true
		}
	}

	// Strategy 3: Path component matching
	// Split both paths and compare components
	citedParts := strings.Split(normalizedCited, "/")
	chunkParts := strings.Split(normalizedChunk, "/")

	// Check if cited path components match the end of chunk path
	// Only if cited path has at least 2 components (directory + file)
	if len(citedParts) >= 2 && len(citedParts) <= len(chunkParts) {
		chunkEnd := chunkParts[len(chunkParts)-len(citedParts):]
		match := true
		for i, part := range citedParts {
			if part != chunkEnd[i] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}

	// Strategy 4: Check if chunk path ends with cited path
	// This handles cases like "folder/file.md" matching "parent/folder/file.md"
	if strings.HasSuffix(normalizedChunk, "/"+normalizedCited) ||
		strings.HasSuffix(normalizedChunk, normalizedCited) {
		return true
	}

	return false
}

// normalizeSection normalizes a section name for comparison by:
// - Removing markdown heading markers (#, ##, ###)
// - Trimming whitespace
// - Converting to lowercase
// - Removing special characters for fuzzy matching
func normalizeSection(section string) string {
	normalized := strings.TrimSpace(section)
	// Remove markdown heading markers
	for strings.HasPrefix(normalized, "#") {
		normalized = strings.TrimPrefix(normalized, "#")
		normalized = strings.TrimSpace(normalized)
	}
	normalized = strings.ToLower(normalized)
	return normalized
}

// tokenizeSection splits a section string into tokens (words) for token-based matching.
func tokenizeSection(section string) map[string]bool {
	normalized := normalizeSection(section)
	// Split by whitespace and common separators
	tokens := strings.FieldsFunc(normalized, func(r rune) bool {
		return r == ' ' || r == '-' || r == '_' || r == '>' || r == '|'
	})
	tokenSet := make(map[string]bool, len(tokens))
	for _, token := range tokens {
		// Remove empty tokens and very short tokens
		if len(token) > 1 {
			tokenSet[token] = true
		}
	}
	return tokenSet
}

// matchSection attempts to match a cited section against a chunk's heading path using multiple strategies:
// 1. Exact match after normalization
// 2. Contains matching (bidirectional, but require minimum length)
// 3. Token-based matching (split into words, check overlap)
// 4. Handle notion-id cases (if heading path is notion-id, try partial matching)
func matchSection(citedSection, headingPath string) bool {
	// Normalize both
	normalizedCited := normalizeSection(citedSection)
	normalizedHeading := normalizeSection(headingPath)

	// Strategy 1: Exact match after normalization
	if normalizedCited == normalizedHeading {
		return true
	}

	// Strategy 2: Contains matching (bidirectional, but require minimum length to avoid false positives)
	// Only match if the shorter string is at least 3 characters and is contained in the longer
	if len(normalizedCited) >= 3 && len(normalizedHeading) >= 3 {
		citedWords := strings.Fields(normalizedCited)
		headingWords := strings.Fields(normalizedHeading)

		// Check contains match
		if strings.Contains(normalizedHeading, normalizedCited) ||
			strings.Contains(normalizedCited, normalizedHeading) {
			// If both are single words, require exact match (already checked above)
			if len(citedWords) == 1 && len(headingWords) == 1 {
				// Single word match already handled by exact match above
				return false
			}
			// If one is a single word contained in the other, allow it
			if len(citedWords) == 1 || len(headingWords) == 1 {
				return true
			}
			// If both have multiple words, require significant overlap (handled by token matching below)
			// But also allow if shorter is substantial portion of longer
			shorter := normalizedCited
			longer := normalizedHeading
			if len(normalizedCited) > len(normalizedHeading) {
				shorter = normalizedHeading
				longer = normalizedCited
			}
			// Allow if shorter is at least 60% of longer (to avoid "Section One" matching "Section Two")
			if len(shorter)*10 >= len(longer)*6 {
				return true
			}
		}
	}

	// Strategy 3: Token-based matching
	citedTokens := tokenizeSection(citedSection)
	headingTokens := tokenizeSection(headingPath)

	// Count overlapping tokens
	overlapCount := 0
	for token := range citedTokens {
		if headingTokens[token] {
			overlapCount++
		}
	}

	// If significant overlap, consider it a match
	// Require at least 2 tokens in both sets to avoid single-word false matches
	if len(citedTokens) >= 2 && len(headingTokens) >= 2 {
		minTokens := len(citedTokens)
		if len(headingTokens) < minTokens {
			minTokens = len(headingTokens)
		}
		// Require at least 2 overlapping tokens AND at least 60% overlap of the shorter set
		// This prevents "Section One" from matching "Section Two" (only 1/2 = 50% overlap)
		if overlapCount >= 2 && minTokens > 0 && overlapCount*10 >= minTokens*6 {
			return true
		}
	}

	// Strategy 4: Handle notion-id cases
	// If heading path contains "notion-id:", try to match against the section name more flexibly
	if strings.Contains(strings.ToLower(headingPath), "notion-id:") {
		// For notion-ids, we can't match by ID, but we can try to match if the cited section
		// appears anywhere in the heading path (already covered by contains match above)
		// Or if there's significant token overlap (already covered above)
		// This is a fallback that might help in some cases
	}

	return false
}

// extractCitationsFromAnswer parses citations from the LLM answer and returns references
// for only the chunks that were actually cited. Citations are expected in the format:
// [File: filename.md, Section: section name]
func (e *ragEngine) extractCitationsFromAnswer(ctx context.Context, answer string, chunks []chunkData) []Reference {
	logger := contextutil.LoggerFromContext(ctx)

	// Find all citation patterns in the answer
	// Pattern: [File: filename.md, Section: section name]
	citedFiles := make(map[string]map[string]bool) // filename -> section -> true

	// Split answer into lines to look for citations
	lines := strings.Split(answer, "\n")
	for _, line := range lines {
		// Look for [File: ...] pattern - handle variations in format
		// Check for both "[File:" and "Section:" in the line (case-insensitive)
		lineLower := strings.ToLower(line)
		if strings.Contains(lineLower, "[file:") && strings.Contains(lineLower, "section:") {
			// Find all citations in this line (may have multiple)
			lineRemaining := line
			for {
				// Find the start of [File:
				fileStart := strings.Index(strings.ToLower(lineRemaining), "[file:")
				if fileStart == -1 {
					break
				}

				// Find the matching closing bracket
				citationEnd := -1
				bracketCount := 0
				for i := fileStart; i < len(lineRemaining); i++ {
					if lineRemaining[i] == '[' {
						bracketCount++
					} else if lineRemaining[i] == ']' {
						bracketCount--
						if bracketCount == 0 {
							citationEnd = i + 1
							break
						}
					}
				}

				if citationEnd == -1 {
					break
				}

				// Extract citation text (skip "[File:" prefix)
				citationText := lineRemaining[fileStart+6 : citationEnd-1] // Skip "[File:" and closing "]"

				// Parse "filename, Section: section name" - handle variations
				// Try different separators and formats
				var filename, sectionName string
				parts := strings.SplitN(citationText, ", Section:", 2)
				if len(parts) == 2 {
					filename = strings.TrimSpace(parts[0])
					sectionName = strings.TrimSpace(parts[1])
				} else {
					// Try with different case
					parts = strings.SplitN(citationText, ", section:", 2)
					if len(parts) == 2 {
						filename = strings.TrimSpace(parts[0])
						sectionName = strings.TrimSpace(parts[1])
					} else {
						// Try with colon separator
						parts = strings.SplitN(citationText, ":", 2)
						if len(parts) == 2 {
							filename = strings.TrimSpace(parts[0])
							sectionName = strings.TrimSpace(parts[1])
						}
					}
				}

				if filename != "" && sectionName != "" {
					// Store original values (normalization happens during matching)
					// Use original filename as key to preserve path information
					if citedFiles[filename] == nil {
						citedFiles[filename] = make(map[string]bool)
					}
					citedFiles[filename][sectionName] = true
				}

				// Continue searching in the rest of the line
				lineRemaining = lineRemaining[citationEnd:]
			}
		}
	}

	// Log citations found
	if len(citedFiles) == 0 {
		logger.DebugContext(ctx, "no citations found in answer")
		return nil
	}

	citationCount := 0
	for _, sections := range citedFiles {
		citationCount += len(sections)
	}
	logger.DebugContext(ctx, "citations extracted from answer",
		"citations_found", citationCount,
		"unique_files", len(citedFiles))

	// Match cited files and sections to chunks
	references := make([]Reference, 0)
	matchedCitations := make(map[string]bool) // Track which citations were matched

	for _, chunk := range chunks {
		// Check if this chunk's file and section match any citation
		var matchedFile string
		var matchedSection string
		var matchStrategy string

		// Try to match filename using improved matching
		for citedFile := range citedFiles {
			if matchFilePath(citedFile, chunk.relPath) {
				matchedFile = citedFile
				matchStrategy = "file_path"

				// Check if section matches using improved matching
				for citedSection := range citedFiles[citedFile] {
					// Skip if this is the normalized version (we'll check the original)
					if matchSection(citedSection, chunk.headingPath) {
						matchedSection = citedSection
						matchStrategy = "file_path+section"
						break
					}
				}
				if matchedSection != "" {
					break
				}
			}
		}

		if matchedFile != "" && matchedSection != "" {
			references = append(references, Reference{
				Vault:       chunk.vaultName,
				RelPath:     chunk.relPath,
				HeadingPath: chunk.headingPath,
				ChunkIndex:  chunk.chunkIndex,
			})
			matchedCitations[matchedFile+":"+matchedSection] = true

			logger.DebugContext(ctx, "citation matched",
				"chunk_path", chunk.relPath,
				"chunk_section", chunk.headingPath,
				"cited_file", matchedFile,
				"cited_section", matchedSection,
				"strategy", matchStrategy)
		} else {
			// Log failed match attempts
			logger.DebugContext(ctx, "citation not matched",
				"chunk_path", chunk.relPath,
				"chunk_section", chunk.headingPath,
				"reason", func() string {
					if matchedFile == "" {
						return "file_mismatch"
					}
					return "section_mismatch"
				}())
		}
	}

	// Log unmatched citations
	for citedFile, sections := range citedFiles {
		for citedSection := range sections {
			key := citedFile + ":" + citedSection
			if !matchedCitations[key] {
				logger.WarnContext(ctx, "citation not matched to any chunk",
					"cited_file", citedFile,
					"cited_section", citedSection)
			}
		}
	}

	logger.InfoContext(ctx, "citation extraction completed",
		"citations_found", citationCount,
		"references_matched", len(references),
		"total_chunks", len(chunks))

	return references
}

// Ask answers a question using RAG.
func (e *ragEngine) Ask(ctx context.Context, req AskRequest) (AskResponse, error) {
	logger := contextutil.LoggerFromContext(ctx)

	// Track total time for the entire RAG query
	startTime := time.Now()

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

	// Track folder selection time
	folderSelectionStart := time.Now()
	// Select relevant folders using LLM
	orderedFolders := e.selectRelevantFolders(ctx, req.Question, availableFolders, req.Folders, vaultIDs, vaultIDToNameMap)
	folderSelectionMs := time.Since(folderSelectionStart).Milliseconds()

	logger.InfoContext(ctx, "folder selection completed",
		"available_folders", len(availableFolders),
		"ordered_folders", len(orderedFolders),
		"user_folders", len(req.Folders),
	)
	logger.DebugContext(ctx, "final ordered folder list",
		"ordered_folders", orderedFolders,
		"available_folders", availableFolders,
	)

	// Track retrieval time (vector search + reranking)
	retrievalStart := time.Now()

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
		resp := AskResponse{
			Answer:        "I couldn't find any relevant information in your notes to answer this question.",
			References:    []Reference{},
			Abstained:     true,
			AbstainReason: "no_relevant_context",
		}
		// Build debug info even when no results, if requested
		if req.Debug {
			maxDebugChunks := targetK * 2
			if maxDebugChunks > 50 {
				maxDebugChunks = 50
			}
			// Retrieval completed but found no results, so calculate retrieval time
			retrievalMs := time.Since(retrievalStart).Milliseconds()
			generationMs := int64(0)
			totalMs := time.Since(startTime).Milliseconds()
			debugInfo := e.buildDebugInfo(ctx, deduplicated, []rerankCandidate{}, []rerankCandidate{}, orderedFolders, availableFolders, vaultIDToNameMap, maxDebugChunks, folderSelectionMs, retrievalMs, generationMs, totalMs)
			resp.Debug = debugInfo
		}
		return resp, nil
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
		vaultName, _ := result.Meta["vault_name"].(string)
		relPath, _ := result.Meta["rel_path"].(string)
		headingPathMeta, _ := result.Meta["heading_path"].(string)

		var headingPath string
		var chunkText string
		var chunkIndex int

		if err != nil {
			// Chunk not found in SQLite - use metadata from Qdrant
			// This handles data consistency issues where chunks exist in Qdrant but not SQLite
			logger.WarnContext(ctx, "chunk not found in SQLite, using Qdrant metadata",
				"chunk_id", result.PointID,
				"rel_path", relPath,
				"error", err)

			headingPath = headingPathMeta
			chunkText = "" // Text not available from Qdrant metadata
			if chunkIndexFloat, ok := result.Meta["chunk_index"].(float64); ok {
				chunkIndex = int(chunkIndexFloat)
			}

			// Create a minimal chunk record for reranking
			// Use empty text - lexical score will be 0, but we can still use vector score
			chunk = &storage.ChunkRecord{
				ID:          result.PointID,
				HeadingPath: headingPath,
				Text:        chunkText,
				ChunkIndex:  chunkIndex,
			}
		} else {
			// Chunk found in SQLite - use it
			headingPath = chunk.HeadingPath
			if headingPath == "" {
				headingPath = headingPathMeta
			}
			chunkText = chunk.Text
			chunkIndex = chunk.ChunkIndex
			if chunkIndex == 0 {
				if chunkIndexFloat, ok := result.Meta["chunk_index"].(float64); ok {
					chunkIndex = int(chunkIndexFloat)
				}
			}
		}

		lexScore := lexicalScore(req.Question, chunkText, headingPath)
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
		resp := AskResponse{
			Answer:        "I couldn't find any relevant information in your notes to answer this question.",
			References:    []Reference{},
			Abstained:     true,
			AbstainReason: "no_relevant_context",
		}
		// Build debug info even when no candidates, if requested
		// This shows what was retrieved from vector store even if chunks couldn't be fetched from DB
		if req.Debug {
			maxDebugChunks := targetK * 2
			if maxDebugChunks > 50 {
				maxDebugChunks = 50
			}
			// Retrieval completed but no generation happened
			retrievalMs := time.Since(retrievalStart).Milliseconds()
			generationMs := int64(0)
			totalMs := time.Since(startTime).Milliseconds()
			debugInfo := e.buildDebugInfo(ctx, deduplicated, candidates, []rerankCandidate{}, orderedFolders, availableFolders, vaultIDToNameMap, maxDebugChunks, folderSelectionMs, retrievalMs, generationMs, totalMs)
			resp.Debug = debugInfo
		}
		return resp, nil
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
		resp := AskResponse{
			Answer:        "I couldn't find any relevant information in your notes to answer this question.",
			References:    []Reference{},
			Abstained:     true,
			AbstainReason: "no_relevant_context",
		}
		// Build debug info even when no candidates passed threshold, if requested
		// This shows what was retrieved and scored even if it didn't meet the threshold
		if req.Debug {
			maxDebugChunks := targetK * 2
			if maxDebugChunks > 50 {
				maxDebugChunks = 50
			}
			// Retrieval completed but no generation happened
			retrievalMs := time.Since(retrievalStart).Milliseconds()
			generationMs := int64(0)
			totalMs := time.Since(startTime).Milliseconds()
			debugInfo := e.buildDebugInfo(ctx, deduplicated, candidates, []rerankCandidate{}, orderedFolders, availableFolders, vaultIDToNameMap, maxDebugChunks, folderSelectionMs, retrievalMs, generationMs, totalMs)
			resp.Debug = debugInfo
		}
		return resp, nil
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

	for i, chunk := range chunks {
		contextBuilder.WriteString(fmt.Sprintf("[Chunk %d]\n", i+1))
		contextBuilder.WriteString(fmt.Sprintf("[Vault: %s] File: %s\n", chunk.vaultName, chunk.relPath))
		contextBuilder.WriteString(fmt.Sprintf("Section: %s\n", chunk.headingPath))
		contextBuilder.WriteString(fmt.Sprintf("Content: %s\n\n", chunk.text))
	}

	contextBuilder.WriteString("--- End Context ---\n")
	contextBuilder.WriteString("\nWhen citing sources, use the format '[File: filename.md, Section: section name]' matching the exact filename and section name from the context above.")

	contextString := contextBuilder.String()
	logger.InfoContext(ctx, "context formatted for LLM",
		"context_length", len(contextString),
		"chunks_included", len(chunks),
	)
	logger.DebugContext(ctx, "full context being sent to LLM", "context", contextString)

	// Retrieval phase complete (vector search + reranking)
	retrievalMs := time.Since(retrievalStart).Milliseconds()

	// Track generation time (LLM call)
	generationStart := time.Now()

	// Construct LLM messages
	systemPrompt := "You are a helpful assistant that answers questions based on the provided context from the user's notes. " +
		"Your primary goal is to provide accurate, complete answers to the question. " +
		"Answer the question using only the information from the context below. " +
		"CRITICAL: You MUST cite all major claims and factual statements using the exact format '[File: filename.md, Section: section name]' where the filename and section name match the context provided. " +
		"Do NOT make any unsupported claims - if information is not in the context, explicitly state that it is not available. " +
		"If the context doesn't contain enough information to answer the question, say so clearly. " +
		"REQUIRED: At the END of your answer, you MUST include a 'Citations:' section listing all sources used. " +
		"Example format:\n" +
		"Citations:\n" +
		"[File: Software/LeetCode Tips.md, Section: Golang Tips & Oddities]\n" +
		"[File: Software/Data Structures & Algorithms/Hash Tables.md, Section: Designing a HashMap]\n" +
		"Remember: Answer quality comes first, but citations are required for all major claims."

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
		Model:       "",  // Use default from client
		MaxTokens:   0,   // No limit
		Temperature: 0.3, // Lower temperature for more focused, citation-aware responses with less hallucination
	})
	if err != nil {
		logger.ErrorContext(ctx, "failed to get LLM response", "error", err)
		return AskResponse{}, fmt.Errorf("failed to get LLM response: %w", err)
	}

	logger.InfoContext(ctx, "received LLM response", "answer_length", len(answer))
	logger.DebugContext(ctx, "LLM answer", "answer", answer)

	// Generation phase complete
	generationMs := time.Since(generationStart).Milliseconds()

	// Extract citations from answer and build references from only cited chunks
	references := e.extractCitationsFromAnswer(ctx, answer, chunks)
	if len(references) == 0 {
		// Check if answer contains any citation-like patterns (even if not in expected format)
		hasCitationPatterns := false
		citationPatterns := []string{"[File:", "[file:", "File:", "file:", "Section:", "section:"}
		answerLower := strings.ToLower(answer)
		for _, pattern := range citationPatterns {
			if strings.Contains(answerLower, strings.ToLower(pattern)) {
				hasCitationPatterns = true
				break
			}
		}

		if hasCitationPatterns {
			// Answer contains citation-like patterns but extraction failed
			logger.WarnContext(ctx, "citation patterns detected but extraction failed, falling back to all chunks",
				"answer_length", len(answer),
				"chunks_available", len(chunks),
				"answer_preview", truncateString(answer, 200))
		} else {
			// No citation patterns at all
			logger.InfoContext(ctx, "no citations found in answer, falling back to all chunks",
				"answer_length", len(answer),
				"chunks_available", len(chunks))
		}

		// Fallback: include all chunks (backward compatibility)
		references = make([]Reference, 0, len(chunks))
		for _, chunk := range chunks {
			references = append(references, Reference{
				Vault:       chunk.vaultName,
				RelPath:     chunk.relPath,
				HeadingPath: chunk.headingPath,
				ChunkIndex:  chunk.chunkIndex,
			})
		}
	} else {
		logger.InfoContext(ctx, "extracted citations from answer",
			"citations_found", len(references),
			"total_chunks", len(chunks))
	}

	logger.InfoContext(ctx, "RAG query completed", "question_length", len(req.Question), "chunks_used", len(chunks), "answer_length", len(answer))

	resp := AskResponse{
		Answer:     answer,
		References: references,
	}

	// Collect debug information if requested
	if req.Debug {
		maxDebugChunks := targetK * 2
		if maxDebugChunks > 50 {
			maxDebugChunks = 50
		}
		totalMs := time.Since(startTime).Milliseconds()
		debugInfo := e.buildDebugInfo(ctx, deduplicated, candidates, selectedCandidates, orderedFolders, availableFolders, vaultIDToNameMap, maxDebugChunks, folderSelectionMs, retrievalMs, generationMs, totalMs)
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
	maxDebugChunks int,
	folderSelectionMs int64,
	retrievalMs int64,
	generationMs int64,
	totalMs int64,
) *DebugInfo {
	logger := contextutil.LoggerFromContext(ctx)

	// Limit debug chunks to a reasonable number for labeling/evaluation
	// Default to 50 if not specified, or use 2x the requested K
	if maxDebugChunks <= 0 {
		maxDebugChunks = 50
	}

	// Build retrieved chunks list from all candidates (before final selection)
	// If candidates is empty (e.g., chunks couldn't be fetched from DB), fall back to deduplicated results
	retrievedChunks := make([]RetrievedChunk, 0)
	if len(candidates) > 0 {
		// Use candidates (has full info including text and lexical scores)
		// Limit to maxDebugChunks for labeling workflow
		limit := len(candidates)
		if limit > maxDebugChunks {
			limit = maxDebugChunks
		}
		for rank := 0; rank < limit; rank++ {
			candidate := candidates[rank]
			chunkText := candidate.chunk.Text

			// Fallback: if text is empty, try to fetch from database
			// This handles cases where chunks might have been stored without text
			if chunkText == "" {
				if chunk, err := e.chunkRepo.GetByID(ctx, candidate.result.PointID); err == nil {
					chunkText = chunk.Text
					if chunkText != "" {
						logger.DebugContext(ctx, "fetched chunk text from DB fallback",
							"chunk_id", candidate.result.PointID)
					}
				}
			}

			retrievedChunks = append(retrievedChunks, RetrievedChunk{
				ChunkID:      candidate.result.PointID,
				RelPath:      candidate.relPath,
				HeadingPath:  candidate.headingPath,
				ScoreVector:  float64(candidate.vectorScore),
				ScoreLexical: float64(candidate.lexicalScore),
				ScoreFinal:   float64(candidate.finalScore),
				Text:         chunkText,
				Rank:         rank + 1,
			})
		}
		if len(candidates) > maxDebugChunks {
			logger.DebugContext(ctx, "debug chunks limited for labeling workflow",
				"total_candidates", len(candidates),
				"shown_chunks", maxDebugChunks)
		}
	} else if len(deduplicated) > 0 {
		// Fall back to deduplicated results (from vector search, but chunks not fetched from DB)
		// This helps debug when chunks exist in Qdrant but not in SQLite
		// Limit to maxDebugChunks for labeling workflow
		limit := len(deduplicated)
		if limit > maxDebugChunks {
			limit = maxDebugChunks
		}
		for rank := 0; rank < limit; rank++ {
			result := deduplicated[rank]
			relPath, _ := result.Meta["rel_path"].(string)
			headingPath, _ := result.Meta["heading_path"].(string)

			// Try to fetch chunk text from database
			chunkText := ""
			if chunk, err := e.chunkRepo.GetByID(ctx, result.PointID); err == nil {
				chunkText = chunk.Text
			} else {
				logger.DebugContext(ctx, "failed to fetch chunk text from DB",
					"chunk_id", result.PointID,
					"error", err)
			}

			retrievedChunks = append(retrievedChunks, RetrievedChunk{
				ChunkID:      result.PointID,
				RelPath:      relPath,
				HeadingPath:  headingPath,
				ScoreVector:  float64(result.Score),
				ScoreLexical: 0,                     // Not computed yet (requires chunk text)
				ScoreFinal:   float64(result.Score), // Use vector score as final when no lexical score
				Text:         chunkText,
				Rank:         rank + 1,
			})
		}
		logger.DebugContext(ctx, "debug info built from deduplicated results (chunks not fetched from DB)",
			"total_results", len(deduplicated),
			"shown_chunks", len(retrievedChunks))
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
		Latency: &LatencyBreakdown{
			FolderSelectionMs: folderSelectionMs,
			RetrievalMs:       retrievalMs,
			GenerationMs:      generationMs,
			JudgeMs:           0, // Judging happens in Python, not Go
			TotalMs:           totalMs,
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
