package indexer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"sort"
	"unicode/utf8"

	"helloworld-ai/internal/storage"
)

const (
	// ChunkerVersion is the version identifier for the chunker implementation.
	// Update this when chunking logic changes significantly.
	ChunkerVersion = "v1.0"
	// TokensPerRune is an approximation for token counting (4 chars per token).
	TokensPerRune = 4.0
)

// IndexingCoverageStats contains statistics about the indexing process.
type IndexingCoverageStats struct {
	// DocsProcessed is the total number of documents processed.
	DocsProcessed int `json:"docs_processed"`
	// DocsWith0Chunks is the number of documents that produced 0 chunks.
	DocsWith0Chunks int `json:"docs_with_0_chunks"`
	// ChunksAttempted is the total number of chunks that were attempted to be embedded.
	ChunksAttempted int `json:"chunks_attempted"`
	// ChunksEmbedded is the number of chunks successfully embedded and stored.
	ChunksEmbedded int `json:"chunks_embedded"`
	// ChunksSkipped is the number of chunks skipped (e.g., due to context size limits).
	ChunksSkipped int `json:"chunks_skipped"`
	// ChunksSkippedReasons is a breakdown of why chunks were skipped.
	ChunksSkippedReasons map[string]int `json:"chunks_skipped_reasons,omitempty"`
	// ChunkTokenStats contains statistics about token counts per chunk.
	ChunkTokenStats ChunkTokenStats `json:"chunk_token_stats"`
	// ChunkerVersion is the version of the chunker used.
	ChunkerVersion string `json:"chunker_version"`
	// IndexVersion is a hash identifying the index build (chunker + embedding model + params).
	IndexVersion string `json:"index_version"`
}

// ChunkTokenStats contains statistics about token counts in chunks.
type ChunkTokenStats struct {
	// Min is the minimum token count across all chunks.
	Min int `json:"min"`
	// Max is the maximum token count across all chunks.
	Max int `json:"max"`
	// Mean is the mean token count across all chunks.
	Mean float64 `json:"mean"`
	// P95 is the 95th percentile token count.
	P95 int `json:"p95"`
}

// GetIndexingCoverageStats computes indexing coverage statistics from the database.
// This method queries the current state of the index to compute stats.
func (p *Pipeline) GetIndexingCoverageStats(ctx context.Context, embeddingModelName string) (*IndexingCoverageStats, error) {
	// Get note repo to query notes
	noteRepo, ok := p.noteRepo.(*storage.NoteRepo)
	if !ok {
		return nil, fmt.Errorf("noteRepo is not *storage.NoteRepo, cannot query stats")
	}

	// Get chunk repo to query chunks
	chunkRepo, ok := p.chunkRepo.(*storage.ChunkRepo)
	if !ok {
		return nil, fmt.Errorf("chunkRepo is not *storage.ChunkRepo, cannot query stats")
	}

	stats := &IndexingCoverageStats{
		ChunksSkippedReasons: make(map[string]int),
		ChunkerVersion:       ChunkerVersion,
	}

	// Query total docs processed (all notes in database)
	docsProcessed, err := p.countNotes(ctx, noteRepo)
	if err != nil {
		return nil, fmt.Errorf("failed to count notes: %w", err)
	}
	stats.DocsProcessed = docsProcessed

	// Query docs with 0 chunks
	docsWith0Chunks, err := p.countNotesWith0Chunks(ctx, noteRepo, chunkRepo)
	if err != nil {
		return nil, fmt.Errorf("failed to count notes with 0 chunks: %w", err)
	}
	stats.DocsWith0Chunks = docsWith0Chunks

	// Query all chunks to compute token stats
	chunks, err := p.getAllChunks(ctx, chunkRepo)
	if err != nil {
		return nil, fmt.Errorf("failed to get chunks: %w", err)
	}

	stats.ChunksEmbedded = len(chunks)
	stats.ChunksAttempted = stats.ChunksEmbedded // We don't track attempted separately, so use embedded as approximation
	stats.ChunksSkipped = 0                       // We don't track skipped chunks in DB, so set to 0

	// Compute token statistics from chunk texts
	if len(chunks) > 0 {
		tokenCounts := make([]int, 0, len(chunks))
		for _, chunk := range chunks {
			// Estimate tokens from rune count (approximation: ~4 chars per token)
			runeCount := utf8.RuneCountInString(chunk.Text)
			tokenCount := int(math.Round(float64(runeCount) / TokensPerRune))
			if tokenCount < 1 {
				tokenCount = 1 // Minimum 1 token
			}
			tokenCounts = append(tokenCounts, tokenCount)
		}

		stats.ChunkTokenStats = computeTokenStats(tokenCounts)
	} else {
		// No chunks, set default stats
		stats.ChunkTokenStats = ChunkTokenStats{
			Min:  0,
			Max:  0,
			Mean: 0,
			P95:  0,
		}
	}

	// Generate index version hash (chunker_version + embedding_model + chunking_params)
	// Use chunker constants from chunker.go
	const minChunkSize = 50
	const maxChunkSize = 700
	indexVersionInput := fmt.Sprintf("%s|%s|minChunkSize=%d|maxChunkSize=%d",
		ChunkerVersion, embeddingModelName, minChunkSize, maxChunkSize)
	hash := sha256.Sum256([]byte(indexVersionInput))
	stats.IndexVersion = hex.EncodeToString(hash[:])[:16] // 16 hex chars = 64 bits

	return stats, nil
}

// countNotes counts the total number of notes in the database.
func (p *Pipeline) countNotes(ctx context.Context, noteRepo *storage.NoteRepo) (int, error) {
	// Access the underlying database
	db := noteRepo.DB()
	if db == nil {
		return 0, fmt.Errorf("noteRepo.DB() returned nil")
	}

	var count int
	err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM notes").Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to query note count: %w", err)
	}
	return count, nil
}

// countNotesWith0Chunks counts notes that have no associated chunks.
func (p *Pipeline) countNotesWith0Chunks(ctx context.Context, noteRepo *storage.NoteRepo, chunkRepo *storage.ChunkRepo) (int, error) {
	db := noteRepo.DB()
	if db == nil {
		return 0, fmt.Errorf("noteRepo.DB() returned nil")
	}

	var count int
	// Count notes that don't have any chunks
	err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM notes 
		 WHERE id NOT IN (SELECT DISTINCT note_id FROM chunks)`).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to query notes with 0 chunks: %w", err)
	}
	return count, nil
}

// getAllChunks retrieves all chunks from the database.
func (p *Pipeline) getAllChunks(ctx context.Context, chunkRepo *storage.ChunkRepo) ([]*storage.ChunkRecord, error) {
	db := chunkRepo.DB()
	if db == nil {
		return nil, fmt.Errorf("chunkRepo.DB() returned nil")
	}

	rows, err := db.QueryContext(ctx, "SELECT id, note_id, chunk_index, heading_path, text FROM chunks")
	if err != nil {
		return nil, fmt.Errorf("failed to query chunks: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var chunks []*storage.ChunkRecord
	for rows.Next() {
		var chunk storage.ChunkRecord
		if err := rows.Scan(&chunk.ID, &chunk.NoteID, &chunk.ChunkIndex, &chunk.HeadingPath, &chunk.Text); err != nil {
			return nil, fmt.Errorf("failed to scan chunk: %w", err)
		}
		chunks = append(chunks, &chunk)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("row iteration error: %w", err)
	}

	return chunks, nil
}

// computeTokenStats computes min, max, mean, and p95 from token counts.
func computeTokenStats(tokenCounts []int) ChunkTokenStats {
	if len(tokenCounts) == 0 {
		return ChunkTokenStats{}
	}

	// Sort for percentile calculation
	sorted := make([]int, len(tokenCounts))
	copy(sorted, tokenCounts)
	sort.Ints(sorted)

	min := sorted[0]
	max := sorted[len(sorted)-1]

	// Compute mean
	sum := 0
	for _, count := range tokenCounts {
		sum += count
	}
	mean := float64(sum) / float64(len(tokenCounts))

	// Compute p95
	p95Index := int(math.Ceil(float64(len(sorted)) * 0.95))
	if p95Index >= len(sorted) {
		p95Index = len(sorted) - 1
	}
	p95 := sorted[p95Index]

	return ChunkTokenStats{
		Min:  min,
		Max:  max,
		Mean: math.Round(mean*100) / 100, // Round to 2 decimal places
		P95:  p95,
	}
}

