package indexer

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/google/uuid"

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
	logger       *slog.Logger
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
		logger:       slog.Default(),
	}
}

// getLogger extracts logger from context or returns default logger.
func (p *Pipeline) getLogger(ctx context.Context) *slog.Logger {
	type loggerKeyType string
	const loggerKey loggerKeyType = "logger"
	if ctxLogger := ctx.Value(loggerKey); ctxLogger != nil {
		if l, ok := ctxLogger.(*slog.Logger); ok {
			return l
		}
	}
	return p.logger
}

// IndexNote indexes a single note file.
// It checks if the file has changed (via hash), chunks it, generates embeddings,
// and stores chunks in both SQLite and Qdrant.
// folder is the folder path (already calculated from relPath during scanning).
func (p *Pipeline) IndexNote(ctx context.Context, vaultID int, relPath, folder string) error {
	logger := p.getLogger(ctx)

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

	// Generate embeddings in batches to avoid exceeding server batch size limits
	// Limit by both count and total character size to handle large chunks
	const maxBatchCount = 5    // Max number of chunks per batch
	const maxBatchChars = 8000 // Max total characters per batch (conservative limit)
	embeddings := make([][]float32, 0, len(chunks))

	i := 0
	for i < len(chunkTexts) {
		// Build batch respecting both count and character limits
		batch := make([]string, 0, maxBatchCount)
		batchChars := 0
		startIdx := i

		for i < len(chunkTexts) && len(batch) < maxBatchCount {
			chunkText := chunkTexts[i]
			chunkChars := len(chunkText)

			// If adding this chunk would exceed character limit, stop
			if len(batch) > 0 && batchChars+chunkChars > maxBatchChars {
				break
			}

			// If single chunk exceeds limit, we still need to process it (but warn)
			if chunkChars > maxBatchChars {
				logger.WarnContext(ctx, "chunk exceeds batch character limit, processing individually",
					"rel_path", relPath,
					"chunk_index", i,
					"chunk_size", chunkChars,
					"limit", maxBatchChars,
				)
			}

			batch = append(batch, chunkText)
			batchChars += chunkChars
			i++
		}

		if len(batch) == 0 {
			// Shouldn't happen, but safety check
			break
		}

		batchEmbeddings, err := p.embedder.EmbedTexts(ctx, batch)
		if err != nil {
			return fmt.Errorf("failed to generate embeddings for batch %d-%d: %w", startIdx, i-1, err)
		}

		embeddings = append(embeddings, batchEmbeddings...)
	}

	if len(embeddings) != len(chunks) {
		return fmt.Errorf("embedding count mismatch: expected %d, got %d", len(chunks), len(embeddings))
	}

	// Prepare chunks and vectors for storage
	chunkRecords := make([]*storage.ChunkRecord, len(chunks))
	points := make([]vectorstore.Point, len(chunks))

	for i, chunk := range chunks {
		// Generate chunk ID
		chunkID := uuid.New().String()

		// Create chunk record
		chunkRecords[i] = &storage.ChunkRecord{
			ID:          chunkID,
			NoteID:      noteID,
			ChunkIndex:  chunk.Index,
			HeadingPath: chunk.HeadingPath,
			Text:        chunk.Text,
		}

		// Create vector point with metadata
		points[i] = vectorstore.Point{
			ID:  chunkID,
			Vec: embeddings[i],
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
		}
	}

	// Insert chunks into SQLite
	for _, chunkRecord := range chunkRecords {
		if err := p.chunkRepo.Insert(ctx, chunkRecord); err != nil {
			return fmt.Errorf("failed to insert chunk: %w", err)
		}
	}

	// Batch upsert points to Qdrant
	if err := p.vectorStore.Upsert(ctx, p.collection, points); err != nil {
		return fmt.Errorf("failed to upsert vectors: %w", err)
	}

	logger.InfoContext(ctx, "indexed note", "rel_path", relPath, "chunks", len(chunks), "title", title)
	return nil
}

// ClearAll deletes all indexed data (chunks, notes, and Qdrant points).
// This is used for force reindexing.
func (p *Pipeline) ClearAll(ctx context.Context) error {
	logger := p.getLogger(ctx)
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
	logger := p.getLogger(ctx)

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
