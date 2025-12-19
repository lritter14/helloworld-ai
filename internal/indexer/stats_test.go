package indexer

import (
	"context"
	"testing"

	"helloworld-ai/internal/storage"
)

func TestGetIndexingCoverageStats(t *testing.T) {
	// Create temporary database
	db, err := storage.New(":memory:")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer func() {
		_ = db.Close()
	}()

	// Create schema using Migrate
	if err := storage.Migrate(db); err != nil {
		t.Fatalf("failed to migrate database: %v", err)
	}

	// Create repos
	noteRepo := storage.NewNoteRepo(db)
	chunkRepo := storage.NewChunkRepo(db)

	// Create pipeline with test dependencies
	pipeline := &Pipeline{
		noteRepo: noteRepo,
		chunkRepo: chunkRepo,
	}

	ctx := context.Background()
	embeddingModelName := "test-embedding-model"

	// Test with empty database
	stats, err := pipeline.GetIndexingCoverageStats(ctx, embeddingModelName)
	if err != nil {
		t.Fatalf("GetIndexingCoverageStats() error = %v", err)
	}

	if stats.DocsProcessed != 0 {
		t.Errorf("DocsProcessed = %d, want 0", stats.DocsProcessed)
	}
	if stats.DocsWith0Chunks != 0 {
		t.Errorf("DocsWith0Chunks = %d, want 0", stats.DocsWith0Chunks)
	}
	if stats.ChunksEmbedded != 0 {
		t.Errorf("ChunksEmbedded = %d, want 0", stats.ChunksEmbedded)
	}
	if stats.ChunkerVersion != ChunkerVersion {
		t.Errorf("ChunkerVersion = %s, want %s", stats.ChunkerVersion, ChunkerVersion)
	}
	if stats.IndexVersion == "" {
		t.Error("IndexVersion should not be empty")
	}

	// Insert test data
	noteID1 := "note-1"
	noteID2 := "note-2"
	noteID3 := "note-3"

	// Insert a vault first (required by foreign key)
	if _, err := db.Exec(`INSERT INTO vaults (id, name, root_path) VALUES (1, 'test', '/test')`); err != nil {
		t.Fatalf("failed to insert vault: %v", err)
	}

	// Note 1: has chunks
	if _, err := db.Exec(`
		INSERT INTO notes (id, vault_id, rel_path, folder, title, hash)
		VALUES (?, 1, 'test1.md', 'folder1', 'Test 1', 'hash1')
	`, noteID1); err != nil {
		t.Fatalf("failed to insert note 1: %v", err)
	}

	// Note 2: has chunks
	if _, err := db.Exec(`
		INSERT INTO notes (id, vault_id, rel_path, folder, title, hash)
		VALUES (?, 1, 'test2.md', 'folder1', 'Test 2', 'hash2')
	`, noteID2); err != nil {
		t.Fatalf("failed to insert note 2: %v", err)
	}

	// Note 3: no chunks (should be counted in DocsWith0Chunks)
	if _, err := db.Exec(`
		INSERT INTO notes (id, vault_id, rel_path, folder, title, hash)
		VALUES (?, 1, 'test3.md', 'folder2', 'Test 3', 'hash3')
	`, noteID3); err != nil {
		t.Fatalf("failed to insert note 3: %v", err)
	}

	// Insert chunks for note 1 (varying sizes for token stats)
	chunkTexts := []string{
		"Short chunk",                                    // ~3 tokens
		"This is a medium length chunk with more content", // ~10 tokens
		"This is a very long chunk with lots of content that should generate a higher token count because it has many words and sentences that need to be tokenized properly", // ~30 tokens
	}

	for i, text := range chunkTexts {
		chunkID := "chunk-" + noteID1 + "-" + string(rune('a'+i))
		if _, err := db.Exec(`
			INSERT INTO chunks (id, note_id, chunk_index, heading_path, text)
			VALUES (?, ?, ?, ?, ?)
		`, chunkID, noteID1, i, "# Heading", text); err != nil {
			t.Fatalf("failed to insert chunk %d: %v", i, err)
		}
	}

	// Insert chunks for note 2
	for i := 0; i < 2; i++ {
		chunkID := "chunk-" + noteID2 + "-" + string(rune('a'+i))
		if _, err := db.Exec(`
			INSERT INTO chunks (id, note_id, chunk_index, heading_path, text)
			VALUES (?, ?, ?, ?, ?)
		`, chunkID, noteID2, i, "# Heading", "Chunk text"); err != nil {
			t.Fatalf("failed to insert chunk for note 2: %v", err)
		}
	}

	// Get stats again
	stats, err = pipeline.GetIndexingCoverageStats(ctx, embeddingModelName)
	if err != nil {
		t.Fatalf("GetIndexingCoverageStats() error = %v", err)
	}

	if stats.DocsProcessed != 3 {
		t.Errorf("DocsProcessed = %d, want 3", stats.DocsProcessed)
	}
	if stats.DocsWith0Chunks != 1 {
		t.Errorf("DocsWith0Chunks = %d, want 1", stats.DocsWith0Chunks)
	}
	if stats.ChunksEmbedded != 5 {
		t.Errorf("ChunksEmbedded = %d, want 5", stats.ChunksEmbedded)
	}
	if stats.ChunksAttempted != 5 {
		t.Errorf("ChunksAttempted = %d, want 5", stats.ChunksAttempted)
	}

	// Check token stats
	if stats.ChunkTokenStats.Min < 1 {
		t.Errorf("ChunkTokenStats.Min = %d, want >= 1", stats.ChunkTokenStats.Min)
	}
	if stats.ChunkTokenStats.Max < stats.ChunkTokenStats.Min {
		t.Errorf("ChunkTokenStats.Max = %d, should be >= Min = %d", stats.ChunkTokenStats.Max, stats.ChunkTokenStats.Min)
	}
	if stats.ChunkTokenStats.Mean < 1 {
		t.Errorf("ChunkTokenStats.Mean = %f, want >= 1", stats.ChunkTokenStats.Mean)
	}
	if stats.ChunkTokenStats.P95 < stats.ChunkTokenStats.Min || stats.ChunkTokenStats.P95 > stats.ChunkTokenStats.Max {
		t.Errorf("ChunkTokenStats.P95 = %d, should be between Min=%d and Max=%d",
			stats.ChunkTokenStats.P95, stats.ChunkTokenStats.Min, stats.ChunkTokenStats.Max)
	}

	// Check version fields
	if stats.ChunkerVersion != ChunkerVersion {
		t.Errorf("ChunkerVersion = %s, want %s", stats.ChunkerVersion, ChunkerVersion)
	}
	if stats.IndexVersion == "" {
		t.Error("IndexVersion should not be empty")
	}
}

func TestComputeTokenStats(t *testing.T) {
	tests := []struct {
		name        string
		tokenCounts []int
		want        ChunkTokenStats
	}{
		{
			name:        "empty",
			tokenCounts: []int{},
			want:        ChunkTokenStats{},
		},
		{
			name:        "single value",
			tokenCounts: []int{10},
			want: ChunkTokenStats{
				Min:  10,
				Max:  10,
				Mean: 10.0,
				P95:  10,
			},
		},
		{
			name:        "multiple values",
			tokenCounts: []int{5, 10, 15, 20, 25},
			want: ChunkTokenStats{
				Min:  5,
				Max:  25,
				Mean: 15.0,
				P95:  25, // 95th percentile of 5 values = index 4 (0-indexed) = 25
			},
		},
		{
			name:        "unsorted values",
			tokenCounts: []int{30, 5, 20, 10, 15},
			want: ChunkTokenStats{
				Min:  5,
				Max:  30,
				Mean: 16.0, // (30+5+20+10+15)/5 = 16
				P95:  30,
			},
		},
		{
			name:        "many values for p95",
			tokenCounts: []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20},
			want: ChunkTokenStats{
				Min:  1,
				Max:  20,
				Mean: 10.5,
				P95:  20, // 95th percentile of 20 values = index 19 = 20
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := computeTokenStats(tt.tokenCounts)
			if got.Min != tt.want.Min {
				t.Errorf("Min = %d, want %d", got.Min, tt.want.Min)
			}
			if got.Max != tt.want.Max {
				t.Errorf("Max = %d, want %d", got.Max, tt.want.Max)
			}
			if got.Mean != tt.want.Mean {
				t.Errorf("Mean = %f, want %f", got.Mean, tt.want.Mean)
			}
			if got.P95 != tt.want.P95 {
				t.Errorf("P95 = %d, want %d", got.P95, tt.want.P95)
			}
		})
	}
}

func TestGetIndexingCoverageStats_ErrorHandling(t *testing.T) {
	// Test with invalid repo types (not *storage.NoteRepo or *storage.ChunkRepo)
	pipeline := &Pipeline{
		noteRepo:  nil, // Will cause type assertion to fail
		chunkRepo: nil,
	}

	ctx := context.Background()
	_, err := pipeline.GetIndexingCoverageStats(ctx, "test-model")
	if err == nil {
		t.Error("GetIndexingCoverageStats() should return error with nil repos")
	}
}

