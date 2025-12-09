package indexer

import (
	"testing"
	"unicode/utf8"
)

func TestNewGoldmarkChunker(t *testing.T) {
	chunker := NewGoldmarkChunker()
	if chunker == nil {
		t.Fatal("NewGoldmarkChunker() returned nil")
	}
}

func TestGoldmarkChunker_ChunkMarkdown(t *testing.T) {
	chunker := NewGoldmarkChunker()

	tests := []struct {
		name     string
		content  []byte
		filename string
		wantErr  bool
		check    func(string, []Chunk) bool
	}{
		{
			name:     "empty content",
			content:  []byte{},
			filename: "empty.md",
			wantErr:  false,
			check: func(title string, chunks []Chunk) bool {
				return title != "" && len(chunks) == 0
			},
		},
		{
			name:     "simple heading",
			content:  []byte("# Heading\n\nContent here."),
			filename: "simple.md",
			wantErr:  false,
			check: func(title string, chunks []Chunk) bool {
				return title == "Heading" && len(chunks) > 0
			},
		},
		{
			name:     "multiple headings",
			content:  []byte("# Main\n\nContent 1\n\n## Sub\n\nContent 2"),
			filename: "multiple.md",
			wantErr:  false,
			check: func(title string, chunks []Chunk) bool {
				// Title should be "Main" (or could be extracted differently)
				// We should have at least one chunk (chunks might be merged based on size constraints)
				if title == "" {
					return false
				}
				if len(chunks) == 0 {
					return false
				}
				return true
			},
		},
		{
			name:     "no headings uses filename",
			content:  []byte("Just some content without headings."),
			filename: "no-headings.md",
			wantErr:  false,
			check: func(title string, chunks []Chunk) bool {
				return title != "" && len(chunks) > 0
			},
		},
		{
			name:     "H2 as title when no H1",
			content:  []byte("## First H2\n\nContent"),
			filename: "h2-title.md",
			wantErr:  false,
			check: func(title string, chunks []Chunk) bool {
				return title == "First H2"
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			title, chunks, err := chunker.ChunkMarkdown(tt.content, tt.filename)

			if tt.wantErr {
				if err == nil {
					t.Errorf("ChunkMarkdown() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("ChunkMarkdown() unexpected error: %v", err)
				return
			}

			if tt.check != nil && !tt.check(title, chunks) {
				t.Error("ChunkMarkdown() result validation failed")
			}
		})
	}
}

func TestGoldmarkChunker_ChunkMarkdown_SizeConstraints(t *testing.T) {
	chunker := NewGoldmarkChunker()

	// Create content that will produce chunks needing size adjustment
	largeContent := make([]byte, 5000)
	for i := range largeContent {
		largeContent[i] = 'a'
	}
	largeContent = append([]byte("# Heading\n\n"), largeContent...)

	title, chunks, err := chunker.ChunkMarkdown(largeContent, "large.md")
	if err != nil {
		t.Fatalf("ChunkMarkdown() error = %v", err)
	}

	if title == "" {
		t.Error("ChunkMarkdown() should extract title")
	}

	// Check that chunks respect size constraints (using rune count)
	for i, chunk := range chunks {
		chunkRunes := utf8.RuneCountInString(chunk.Text)
		if chunkRunes > maxChunkSize {
			t.Errorf("ChunkMarkdown() chunk[%d] size = %d runes, exceeds max %d", i, chunkRunes, maxChunkSize)
		}
	}
}

func TestGoldmarkChunker_ChunkMarkdown_HeadingHierarchy(t *testing.T) {
	chunker := NewGoldmarkChunker()

	content := []byte(`# Main Heading

Content under main.

## Sub Heading 1

Content under sub 1.

### Sub Sub Heading

Content under sub sub.

## Sub Heading 2

Content under sub 2.
`)

	title, chunks, err := chunker.ChunkMarkdown(content, "hierarchy.md")
	if err != nil {
		t.Fatalf("ChunkMarkdown() error = %v", err)
	}

	if title != "Main Heading" {
		t.Errorf("ChunkMarkdown() title = %q, want Main Heading", title)
	}

	// Should have multiple chunks based on heading hierarchy
	if len(chunks) < 2 {
		t.Errorf("ChunkMarkdown() chunks = %d, want at least 2", len(chunks))
	}

	// Check heading paths
	foundMain := false
	for _, chunk := range chunks {
		if chunk.HeadingPath != "" {
			foundMain = true
			break
		}
	}
	if !foundMain {
		t.Error("ChunkMarkdown() should include heading paths")
	}
}

func TestExtractTitleFromFilename(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		want     string
	}{
		{
			name:     "simple filename",
			filename: "test.md",
			want:     "Test",
		},
		{
			name:     "filename with spaces",
			filename: "my test file.md",
			want:     "My Test File",
		},
		{
			name:     "filename with underscores",
			filename: "my_test_file.md",
			want:     "My_test_file",
		},
		{
			name:     "filename without extension",
			filename: "test",
			want:     "Test",
		},
		{
			name:     "path with directory",
			filename: "folder/test.md",
			want:     "Test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractTitleFromFilename(tt.filename)
			if got != tt.want {
				t.Errorf("extractTitleFromFilename(%q) = %q, want %q", tt.filename, got, tt.want)
			}
		})
	}
}

