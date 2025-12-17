package indexer

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"helloworld-ai/internal/contextutil"
	"helloworld-ai/internal/llm"
	"helloworld-ai/internal/storage"
	"helloworld-ai/internal/vault"
	"helloworld-ai/internal/vectorstore"
)

// Pipeline orchestrates the indexing of markdown files into SQLite and Qdrant.
type Pipeline struct {
	vaultManager *vault.Manager
	noteRepo     storage.NoteStore
	chunkRepo    storage.ChunkStore
	embedder     *llm.EmbeddingsClient
	vectorStore  vectorstore.VectorStore
	collection   string
	chunker      *GoldmarkChunker
}

// NewPipeline creates a new indexing pipeline.
func NewPipeline(
	vaultManager *vault.Manager,
	noteRepo storage.NoteStore,
	chunkRepo storage.ChunkStore,
	embedder *llm.EmbeddingsClient,
	vectorStore vectorstore.VectorStore,
	collection string,
) *Pipeline {
	return &Pipeline{
		vaultManager: vaultManager,
		noteRepo:     noteRepo,
		chunkRepo:    chunkRepo,
		embedder:     embedder,
		vectorStore:  vectorStore,
		collection:   collection,
		chunker:      NewGoldmarkChunker(),
	}
}

// ErrChunkSkipped is returned when a chunk is too large to embed and is skipped.
var ErrChunkSkipped = errors.New("chunk skipped due to context size limit")

// embedTextsWithRetry generates embeddings for texts, automatically reducing batch size
// if the server returns an "input is too large" error.
// This function recursively splits batches in half when encountering size limit errors.
// If a single chunk is too large, it returns ErrChunkSkipped and the caller should skip that chunk.
// Note: The embedding model (granite-embedding-278m-multilingual) has n_ctx=512 tokens.
func (p *Pipeline) embedTextsWithRetry(ctx context.Context, texts []string, relPath string, logger *slog.Logger) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, fmt.Errorf("empty input array")
	}

	// Try to embed the batch
	embeddings, err := p.embedder.EmbedTexts(ctx, texts)
	if err == nil {
		return embeddings, nil
	}

	// Check if this is an "input too large" error using structured error parsing
	var embedErr *llm.EmbeddingError
	isTooLarge := false
	if errors.As(err, &embedErr) {
		isTooLarge = embedErr.IsExceedContextSizeError()
		if isTooLarge && embedErr.LlamaError != nil {
			logger.DebugContext(ctx, "detected exceed_context_size_error",
				"n_prompt_tokens", embedErr.LlamaError.Error.NPromptTokens,
				"n_ctx", embedErr.LlamaError.Error.NCtx,
			)
		}
	}

	// Fallback to string matching if structured parsing didn't work
	if !isTooLarge {
		errStr := err.Error()
		errStrLower := strings.ToLower(errStr)
		isTooLarge = strings.Contains(errStrLower, "input is too large") ||
			strings.Contains(errStrLower, "too large to process") ||
			strings.Contains(errStrLower, "increase the physical batch size") ||
			strings.Contains(errStrLower, "exceed_context_size") ||
			strings.Contains(errStrLower, "larger than the max context size")
	}

	// If not a size error, return the error
	if !isTooLarge {
		return nil, err
	}

	// If we're down to a single chunk that's too large, skip it with a warning
	if len(texts) == 1 {
		chunkRunes := utf8.RuneCountInString(texts[0])
		logFields := []any{
			"rel_path", relPath,
			"chunk_size_runes", chunkRunes,
			"error", err.Error(),
		}

		// Add structured error details if available
		if embedErr != nil && embedErr.LlamaError != nil {
			logFields = append(logFields,
				"n_prompt_tokens", embedErr.LlamaError.Error.NPromptTokens,
				"n_ctx", embedErr.LlamaError.Error.NCtx,
			)
		}

		logger.WarnContext(ctx, "chunk exceeds context size, skipping", logFields...)
		return nil, ErrChunkSkipped
	}

	// Log that we're reducing batch size
	logger.WarnContext(ctx, "batch too large, splitting in half and retrying",
		"rel_path", relPath,
		"batch_size", len(texts),
		"error", err.Error(),
	)

	// Split batch in half and retry each half
	mid := len(texts) / 2
	firstHalf := texts[:mid]
	secondHalf := texts[mid:]

	// Recursively embed each half
	firstEmbeddings, err := p.embedTextsWithRetry(ctx, firstHalf, relPath, logger)
	if err != nil {
		// If first half failed with skip error, continue with second half
		if errors.Is(err, ErrChunkSkipped) {
			firstEmbeddings = nil
		} else {
			return nil, fmt.Errorf("failed to embed first half: %w", err)
		}
	}

	secondEmbeddings, err := p.embedTextsWithRetry(ctx, secondHalf, relPath, logger)
	if err != nil {
		// If second half failed with skip error, return first half results
		if errors.Is(err, ErrChunkSkipped) {
			secondEmbeddings = nil
		} else {
			return nil, fmt.Errorf("failed to embed second half: %w", err)
		}
	}

	// Combine results (handling nil slices from skipped chunks)
	result := make([][]float32, 0)
	if firstEmbeddings != nil {
		result = append(result, firstEmbeddings...)
	}
	if secondEmbeddings != nil {
		result = append(result, secondEmbeddings...)
	}

	return result, nil
}

// IndexNote indexes a single note file.
// It checks if the file has changed (via hash), chunks it, generates embeddings,
// and stores chunks in both SQLite and Qdrant.
// folder is the folder path (already calculated from relPath during scanning).
func (p *Pipeline) IndexNote(ctx context.Context, vaultID int, relPath, folder string) error {
	logger := contextutil.LoggerFromContext(ctx)

	// Get absolute path
	absPath := p.vaultManager.AbsPath(vaultID, relPath)
	if absPath == "" {
		return fmt.Errorf("failed to resolve absolute path for vault %d, relPath %s", vaultID, relPath)
	}

	// Read file content
	content, err := os.ReadFile(absPath)
	if err != nil {
		return fmt.Errorf("failed to read file %s: %w", absPath, err)
	}

	// Compute SHA256 hash
	hash := sha256.Sum256(content)
	hashHex := fmt.Sprintf("%x", hash)

	// Check existing note
	existingNote, err := p.noteRepo.GetByVaultAndPath(ctx, vaultID, relPath)
	if err != nil && err != storage.ErrNotFound {
		return fmt.Errorf("failed to check existing note: %w", err)
	}

	// Skip re-indexing if hash matches (unless force is enabled)
	// Force reindex is handled at the IndexAll level by clearing all data first
	if existingNote != nil && existingNote.Hash == hashHex {
		logger.DebugContext(ctx, "skipping unchanged file", "rel_path", relPath, "hash", hashHex)
		return nil
	}

	// Extract filename for title fallback
	filename := filepath.Base(relPath)

	// Chunk content
	title, chunks, err := p.chunker.ChunkMarkdown(content, filename)
	if err != nil {
		return fmt.Errorf("failed to chunk markdown: %w", err)
	}

	if len(chunks) == 0 {
		logger.WarnContext(ctx, "no chunks generated", "rel_path", relPath)
		return nil
	}

	// Folder is already calculated during scanning, use it as-is
	// (normalize to forward slashes if needed)
	if folder != "" {
		folder = filepath.ToSlash(folder)
	}

	// Get vault name for metadata by checking known vault names
	vaultName := ""
	for name := range map[string]struct{}{"personal": {}, "work": {}} {
		if v, err := p.vaultManager.VaultByName(name); err == nil && v.ID == vaultID {
			vaultName = name
			break
		}
	}
	if vaultName == "" {
		logger.WarnContext(ctx, "vault name not found for vault ID", "vault_id", vaultID)
		vaultName = "unknown" // Fallback
	}

	// Generate or get note ID
	var noteID string
	if existingNote != nil {
		noteID = existingNote.ID
	} else {
		noteID = uuid.New().String()
	}

	// Upsert note record
	noteRecord := &storage.NoteRecord{
		ID:      noteID,
		VaultID: vaultID,
		RelPath: relPath,
		Folder:  folder,
		Title:   title,
		Hash:    hashHex,
	}
	if err := p.noteRepo.Upsert(ctx, noteRecord); err != nil {
		return fmt.Errorf("failed to upsert note: %w", err)
	}

	// If existing note, delete old chunks
	if existingNote != nil {
		oldChunkIDs, err := p.chunkRepo.ListIDsByNote(ctx, noteID)
		if err != nil {
			return fmt.Errorf("failed to list old chunk IDs: %w", err)
		}

		if len(oldChunkIDs) > 0 {
			// Delete from Qdrant
			if err := p.vectorStore.Delete(ctx, p.collection, oldChunkIDs); err != nil {
				logger.WarnContext(ctx, "failed to delete old chunks from Qdrant", "error", err, "count", len(oldChunkIDs))
				// Continue anyway - we'll overwrite with new chunks
			}

			// Delete from SQLite
			if err := p.chunkRepo.DeleteByNote(ctx, noteID); err != nil {
				return fmt.Errorf("failed to delete old chunks from SQLite: %w", err)
			}
		}
	}

	// Extract chunk texts for embedding
	chunkTexts := make([]string, len(chunks))
	for i, chunk := range chunks {
		chunkTexts[i] = chunk.Text
	}

	// Generate embeddings in batches to avoid exceeding server batch size limits.
	// The embedding model (granite-embedding-278m-multilingual) has n_ctx=512 tokens.
	// We use conservative limits to avoid hitting the context size limit.
	// Limit by both count and total character size to handle large chunks.
	// Using rune count (not byte count) for better approximation of token count.
	const maxBatchCount = 3    // Max number of chunks per batch
	const maxBatchChars = 1000 // Max total runes per batch (target ~350-400 tokens, ~4 chars/token)
	embeddings := make([][]float32, 0, len(chunks))

	i := 0
	chunkToEmbeddingMap := make(map[int]int) // Maps chunk index to embedding index
	embeddingIdx := 0
	for i < len(chunkTexts) {
		// Build batch respecting both count and character limits
		batch := make([]string, 0, maxBatchCount)
		batchIndices := make([]int, 0, maxBatchCount) // Track original chunk indices
		batchChars := 0
		startIdx := i

		for i < len(chunkTexts) && len(batch) < maxBatchCount {
			chunkText := chunkTexts[i]
			chunkRunes := utf8.RuneCountInString(chunkText)

			// If adding this chunk would exceed character limit, stop
			if len(batch) > 0 && batchChars+chunkRunes > maxBatchChars {
				break
			}

			// If single chunk exceeds limit, we still need to process it (but warn)
			if chunkRunes > maxBatchChars {
				logger.WarnContext(ctx, "chunk exceeds batch character limit, processing individually",
					"rel_path", relPath,
					"chunk_index", i,
					"chunk_size_runes", chunkRunes,
					"limit", maxBatchChars,
				)
			}

			batch = append(batch, chunkText)
			batchIndices = append(batchIndices, i)
			batchChars += chunkRunes
			i++
		}

		if len(batch) == 0 {
			// Shouldn't happen, but safety check
			break
		}

		// Generate embeddings with automatic batch size reduction on "input too large" errors
		batchEmbeddings, err := p.embedTextsWithRetry(ctx, batch, relPath, logger)
		if err != nil {
			// Check if this is a skip error - if so, skip all chunks in this batch
			if errors.Is(err, ErrChunkSkipped) {
				logger.WarnContext(ctx, "batch skipped due to context size limit",
					"rel_path", relPath,
					"batch_start", startIdx,
					"batch_end", i-1,
					"batch_size", len(batch),
				)
				// Don't add any embeddings for this batch - chunks will be skipped
				continue
			}
			return fmt.Errorf("failed to generate embeddings for batch %d-%d: %w", startIdx, i-1, err)
		}

		// Map chunk indices to embedding indices
		// If we got fewer embeddings than chunks, the last N chunks were skipped
		// (recursive splitting preserves order, so skipped chunks are at the end)
		numEmbeddings := len(batchEmbeddings)
		for j, chunkIdx := range batchIndices {
			if j < numEmbeddings {
				chunkToEmbeddingMap[chunkIdx] = embeddingIdx
				embeddingIdx++
			} else {
				// This chunk was skipped (no embedding generated)
				logger.DebugContext(ctx, "chunk skipped during batch processing",
					"rel_path", relPath,
					"chunk_index", chunkIdx,
				)
			}
		}

		embeddings = append(embeddings, batchEmbeddings...)
	}

	// Handle skipped chunks - we may have fewer embeddings than chunks
	if len(embeddings) < len(chunks) {
		skippedCount := len(chunks) - len(embeddings)
		logger.WarnContext(ctx, "some chunks were skipped due to context size limits",
			"rel_path", relPath,
			"total_chunks", len(chunks),
			"embeddings_generated", len(embeddings),
			"chunks_skipped", skippedCount,
		)
	}

	// Prepare chunks and vectors for storage
	// Only include chunks that have embeddings (skip those that were too large)
	chunkRecords := make([]*storage.ChunkRecord, 0, len(embeddings))
	points := make([]vectorstore.Point, 0, len(embeddings))

	for i, chunk := range chunks {
		// Check if this chunk has an embedding
		embIdx, hasEmbedding := chunkToEmbeddingMap[i]
		if !hasEmbedding {
			// This chunk was skipped - don't include it
			continue
		}

		// Ensure we have a valid embedding index
		if embIdx >= len(embeddings) {
			logger.WarnContext(ctx, "invalid embedding index for chunk, skipping",
				"rel_path", relPath,
				"chunk_index", i,
				"embedding_index", embIdx,
			)
			continue
		}

		// Generate chunk ID
		chunkID := uuid.New().String()

		// Create chunk record
		chunkRecords = append(chunkRecords, &storage.ChunkRecord{
			ID:          chunkID,
			NoteID:      noteID,
			ChunkIndex:  chunk.Index,
			HeadingPath: chunk.HeadingPath,
			Text:        chunk.Text,
		})

		// Create vector point with metadata
		points = append(points, vectorstore.Point{
			ID:  chunkID,
			Vec: embeddings[embIdx],
			Meta: map[string]any{
				"vault_id":     vaultID,
				"vault_name":   vaultName,
				"note_id":      noteID,
				"rel_path":     relPath,
				"folder":       folder,
				"heading_path": chunk.HeadingPath,
				"chunk_index":  chunk.Index,
				"note_title":   title,
			},
		})
	}

	// Insert chunks into SQLite (only chunks that have embeddings)
	if len(chunkRecords) > 0 {
		for _, chunkRecord := range chunkRecords {
			if err := p.chunkRepo.Insert(ctx, chunkRecord); err != nil {
				return fmt.Errorf("failed to insert chunk: %w", err)
			}
		}

		// Batch upsert points to Qdrant
		if err := p.vectorStore.Upsert(ctx, p.collection, points); err != nil {
			return fmt.Errorf("failed to upsert vectors: %w", err)
		}
	}

	logger.InfoContext(ctx, "indexed note",
		"rel_path", relPath,
		"total_chunks", len(chunks),
		"indexed_chunks", len(chunkRecords),
		"skipped_chunks", len(chunks)-len(chunkRecords),
		"title", title,
	)
	return nil
}

// ClearAll deletes all indexed data (chunks, notes, and Qdrant points).
// This is used for force reindexing.
func (p *Pipeline) ClearAll(ctx context.Context) error {
	logger := contextutil.LoggerFromContext(ctx)
	logger.InfoContext(ctx, "clearing all indexed data")

	// Get all chunk IDs from database before deleting
	chunkIDs, err := p.chunkRepo.GetAllIDs(ctx)
	if err != nil {
		return fmt.Errorf("failed to get chunk IDs: %w", err)
	}

	// Delete all points from Qdrant
	if len(chunkIDs) > 0 {
		if err := p.vectorStore.Delete(ctx, p.collection, chunkIDs); err != nil {
			logger.WarnContext(ctx, "failed to delete some points from Qdrant", "error", err)
			// Continue even if Qdrant deletion fails
		} else {
			logger.InfoContext(ctx, "deleted points from Qdrant", "count", len(chunkIDs))
		}
	}

	// Delete all chunks from database
	if err := p.chunkRepo.DeleteAll(ctx); err != nil {
		return fmt.Errorf("failed to delete chunks: %w", err)
	}
	logger.InfoContext(ctx, "deleted all chunks from database")

	// Delete all notes
	if err := p.noteRepo.DeleteAll(ctx); err != nil {
		return fmt.Errorf("failed to delete notes: %w", err)
	}
	logger.InfoContext(ctx, "deleted all notes from database")

	return nil
}

// IndexAll scans all vaults and indexes all markdown files.
// Errors for individual files are logged but don't stop the indexing process.
func (p *Pipeline) IndexAll(ctx context.Context) error {
	logger := contextutil.LoggerFromContext(ctx)

	// Scan all vaults
	scannedFiles, err := p.vaultManager.ScanAll(ctx)
	if err != nil {
		return fmt.Errorf("failed to scan vaults: %w", err)
	}

	logger.InfoContext(ctx, "starting indexing", "total_files", len(scannedFiles))

	var successCount, errorCount int

	// Index each file
	for _, file := range scannedFiles {
		// Check for context cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if err := p.IndexNote(ctx, file.VaultID, file.RelPath, file.Folder); err != nil {
			errorCount++
			logger.ErrorContext(ctx, "failed to index file", "rel_path", file.RelPath, "error", err)
			// Continue with next file
			continue
		}

		successCount++
	}

	logger.InfoContext(ctx, "indexing completed", "total_files", len(scannedFiles), "success", successCount, "errors", errorCount)

	if errorCount > 0 {
		return fmt.Errorf("indexing completed with %d errors", errorCount)
	}

	return nil
}
