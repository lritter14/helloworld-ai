package rag

import (
	"context"
	"testing"

	"helloworld-ai/internal/storage"
	"helloworld-ai/internal/vectorstore"
)

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

func TestBuildDebugInfo(t *testing.T) {
	// Create a minimal ragEngine for testing buildDebugInfo
	engine := &ragEngine{}

	ctx := context.Background()

	// Create test data
	deduplicated := []vectorstore.SearchResult{
		{
			PointID: "chunk1",
			Score:   0.95,
			Meta: map[string]any{
				"vault_id":   float64(1),
				"vault_name": "personal",
				"rel_path":   "projects/main.md",
				"folder":     "projects",
			},
		},
		{
			PointID: "chunk2",
			Score:   0.85,
			Meta: map[string]any{
				"vault_id":   float64(1),
				"vault_name": "personal",
				"rel_path":   "docs/config.md",
				"folder":     "docs",
			},
		},
	}

	candidates := []rerankCandidate{
		{
			result: deduplicated[0],
			chunk: &storage.ChunkRecord{
				ID:          "chunk1",
				HeadingPath: "# Overview",
				Text:        "This is the main project overview.",
			},
			vaultName:    "personal",
			relPath:      "projects/main.md",
			headingPath:  "# Overview",
			chunkIndex:   0,
			vectorScore:  0.95,
			lexicalScore: 0.80,
			finalScore:   0.90,
			originalRank: 1,
		},
		{
			result: deduplicated[1],
			chunk: &storage.ChunkRecord{
				ID:          "chunk2",
				HeadingPath: "# Configuration",
				Text:        "Configuration settings for the project.",
			},
			vaultName:    "personal",
			relPath:      "docs/config.md",
			headingPath:  "# Configuration",
			chunkIndex:   0,
			vectorScore:  0.85,
			lexicalScore: 0.75,
			finalScore:   0.82,
			originalRank: 2,
		},
	}

	selectedCandidates := candidates[:1] // Only first candidate selected

	orderedFolders := []string{"1/projects", "1/docs"}
	availableFolders := []string{"1/projects", "1/docs", "1/notes"}
	vaultIDToNameMap := map[int]string{
		1: "personal",
	}

	// Build debug info
	debugInfo := engine.buildDebugInfo(
		ctx,
		deduplicated,
		candidates,
		selectedCandidates,
		orderedFolders,
		availableFolders,
		vaultIDToNameMap,
		50,   // maxDebugChunks
		10,   // folderSelectionMs
		50,   // retrievalMs
		100,  // generationMs
		160,  // totalMs
	)

	// Verify debug info structure
	if debugInfo == nil {
		t.Fatal("buildDebugInfo() returned nil")
	}

	// Check retrieved chunks
	if len(debugInfo.RetrievedChunks) != len(candidates) {
		t.Errorf("expected %d retrieved chunks, got %d", len(candidates), len(debugInfo.RetrievedChunks))
	}

	// Verify first chunk details
	if len(debugInfo.RetrievedChunks) > 0 {
		chunk := debugInfo.RetrievedChunks[0]
		if chunk.ChunkID != "chunk1" {
			t.Errorf("expected chunk ID 'chunk1', got %q", chunk.ChunkID)
		}
		if chunk.RelPath != "projects/main.md" {
			t.Errorf("expected rel_path 'projects/main.md', got %q", chunk.RelPath)
		}
		if chunk.HeadingPath != "# Overview" {
			t.Errorf("expected heading_path '# Overview', got %q", chunk.HeadingPath)
		}
		const epsilon = 0.0001
		if abs(chunk.ScoreVector-0.95) > epsilon {
			t.Errorf("expected score_vector 0.95, got %f", chunk.ScoreVector)
		}
		if abs(chunk.ScoreLexical-0.80) > epsilon {
			t.Errorf("expected score_lexical 0.80, got %f", chunk.ScoreLexical)
		}
		if abs(chunk.ScoreFinal-0.90) > epsilon {
			t.Errorf("expected score_final 0.90, got %f", chunk.ScoreFinal)
		}
		if chunk.Rank != 1 {
			t.Errorf("expected rank 1, got %d", chunk.Rank)
		}
		if chunk.Text != "This is the main project overview." {
			t.Errorf("expected text 'This is the main project overview.', got %q", chunk.Text)
		}
	}

	// Check folder selection
	if debugInfo.FolderSelection == nil {
		t.Fatal("expected folder selection info, got nil")
	}

	// Verify folder conversion (vaultID/folder -> vaultName/folder)
	expectedSelectedFolders := []string{"personal/projects", "personal/docs"}
	if len(debugInfo.FolderSelection.SelectedFolders) != len(expectedSelectedFolders) {
		t.Errorf("expected %d selected folders, got %d",
			len(expectedSelectedFolders), len(debugInfo.FolderSelection.SelectedFolders))
	} else {
		for i, expected := range expectedSelectedFolders {
			if i < len(debugInfo.FolderSelection.SelectedFolders) {
				if debugInfo.FolderSelection.SelectedFolders[i] != expected {
					t.Errorf("selected folder[%d] = %q, want %q",
						i, debugInfo.FolderSelection.SelectedFolders[i], expected)
				}
			}
		}
	}

	expectedAvailableFolders := []string{"personal/projects", "personal/docs", "personal/notes"}
	if len(debugInfo.FolderSelection.AvailableFolders) != len(expectedAvailableFolders) {
		t.Errorf("expected %d available folders, got %d",
			len(expectedAvailableFolders), len(debugInfo.FolderSelection.AvailableFolders))
	} else {
		for i, expected := range expectedAvailableFolders {
			if i < len(debugInfo.FolderSelection.AvailableFolders) {
		if debugInfo.FolderSelection.AvailableFolders[i] != expected {
			t.Errorf("available folder[%d] = %q, want %q",
				i, debugInfo.FolderSelection.AvailableFolders[i], expected)
		}
	}

	// Check latency breakdown
	if debugInfo.Latency == nil {
		t.Fatal("expected latency breakdown, got nil")
	}
	if debugInfo.Latency.FolderSelectionMs != 10 {
		t.Errorf("expected folder_selection_ms 10, got %d", debugInfo.Latency.FolderSelectionMs)
	}
	if debugInfo.Latency.RetrievalMs != 50 {
		t.Errorf("expected retrieval_ms 50, got %d", debugInfo.Latency.RetrievalMs)
	}
	if debugInfo.Latency.GenerationMs != 100 {
		t.Errorf("expected generation_ms 100, got %d", debugInfo.Latency.GenerationMs)
	}
	if debugInfo.Latency.JudgeMs != 0 {
		t.Errorf("expected judge_ms 0, got %d", debugInfo.Latency.JudgeMs)
	}
	if debugInfo.Latency.TotalMs != 160 {
		t.Errorf("expected total_ms 160, got %d", debugInfo.Latency.TotalMs)
	}
}
	}
}

func TestBuildDebugInfo_EmptyCandidates(t *testing.T) {
	engine := &ragEngine{}
	ctx := context.Background()

	debugInfo := engine.buildDebugInfo(
		ctx,
		[]vectorstore.SearchResult{},
		[]rerankCandidate{},
		[]rerankCandidate{},
		[]string{},
		[]string{},
		map[int]string{},
		50,  // maxDebugChunks
		5,   // folderSelectionMs
		20,  // retrievalMs
		0,   // generationMs
		25,  // totalMs
	)

	if debugInfo == nil {
		t.Fatal("buildDebugInfo() returned nil")
	}

	if len(debugInfo.RetrievedChunks) != 0 {
		t.Errorf("expected 0 retrieved chunks, got %d", len(debugInfo.RetrievedChunks))
	}

	if debugInfo.FolderSelection == nil {
		t.Fatal("expected folder selection info even when empty, got nil")
	}

	if len(debugInfo.FolderSelection.SelectedFolders) != 0 {
		t.Errorf("expected 0 selected folders, got %d", len(debugInfo.FolderSelection.SelectedFolders))
	}

	// Check latency breakdown
	if debugInfo.Latency == nil {
		t.Fatal("expected latency breakdown, got nil")
	}
	if debugInfo.Latency.JudgeMs != 0 {
		t.Errorf("expected judge_ms 0, got %d", debugInfo.Latency.JudgeMs)
	}
}

func TestBuildDebugInfo_FolderConversion(t *testing.T) {
	engine := &ragEngine{}
	ctx := context.Background()

	// Test folder conversion with multiple vaults
	orderedFolders := []string{"1/projects", "2/work", "1/docs"}
	availableFolders := []string{"1/projects", "1/docs", "2/work", "2/meetings"}
	vaultIDToNameMap := map[int]string{
		1: "personal",
		2: "work",
	}

	debugInfo := engine.buildDebugInfo(
		ctx,
		[]vectorstore.SearchResult{},
		[]rerankCandidate{},
		[]rerankCandidate{},
		orderedFolders,
		availableFolders,
		vaultIDToNameMap,
		50,  // maxDebugChunks
		8,   // folderSelectionMs
		30,  // retrievalMs
		0,   // generationMs
		38,  // totalMs
	)

	if debugInfo == nil || debugInfo.FolderSelection == nil {
		t.Fatal("buildDebugInfo() returned nil or folder selection is nil")
	}

	expectedSelected := []string{"personal/projects", "work/work", "personal/docs"}
	if len(debugInfo.FolderSelection.SelectedFolders) != len(expectedSelected) {
		t.Errorf("expected %d selected folders, got %d",
			len(expectedSelected), len(debugInfo.FolderSelection.SelectedFolders))
	}

	for i, expected := range expectedSelected {
		if i < len(debugInfo.FolderSelection.SelectedFolders) {
			actual := debugInfo.FolderSelection.SelectedFolders[i]
			if actual != expected {
				t.Errorf("selected folder[%d] = %q, want %q", i, actual, expected)
			}
		}
	}
}

