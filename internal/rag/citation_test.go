package rag

import (
	"context"
	"testing"
)

func TestNormalizePath(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"simple path", "file.md", "file.md"},
		{"path with directory", "folder/file.md", "folder/file.md"},
		{"path with trailing slash", "folder/file.md/", "folder/file.md"},
		{"uppercase", "FILE.MD", "file.md"},
		{"mixed case", "Folder/File.md", "folder/file.md"},
		{"with spaces", " folder/file.md ", "folder/file.md"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizePath(tt.input)
			if result != tt.expected {
				t.Errorf("normalizePath(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestMatchFilePath(t *testing.T) {
	tests := []struct {
		name      string
		citedPath string
		chunkPath string
		expected  bool
	}{
		{"exact match", "file.md", "file.md", true},
		{"exact match with path", "folder/file.md", "folder/file.md", true},
		{"case insensitive", "FILE.MD", "file.md", true},
		{"basename match", "file.md", "some/path/file.md", true},
		{"basename match reverse", "some/path/file.md", "file.md", true},
		{"path component match", "folder/file.md", "parent/folder/file.md", true},
		{"suffix match", "file.md", "path/to/file.md", true},
		{"no match", "file1.md", "file2.md", false},
		{"no match different dir", "folder1/file.md", "folder2/file.md", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := matchFilePath(tt.citedPath, tt.chunkPath)
			if result != tt.expected {
				t.Errorf("matchFilePath(%q, %q) = %v, want %v", tt.citedPath, tt.chunkPath, result, tt.expected)
			}
		})
	}
}

func TestNormalizeSection(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"simple section", "Section Name", "section name"},
		{"with markdown", "# Section Name", "section name"},
		{"with multiple hashes", "## Section Name", "section name"},
		{"with three hashes", "### Section Name", "section name"},
		{"with spaces", "  Section Name  ", "section name"},
		{"mixed case", "Section NAME", "section name"},
		{"with special chars", "Section-Name", "section-name"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeSection(tt.input)
			if result != tt.expected {
				t.Errorf("normalizeSection(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestMatchSection(t *testing.T) {
	tests := []struct {
		name          string
		citedSection  string
		headingPath   string
		expected      bool
	}{
		{"exact match", "Section Name", "Section Name", true},
		{"exact match normalized", "section name", "Section Name", true},
		{"with markdown", "# Section Name", "## Section Name", true},
		{"contains match", "Section", "Section Name Here", true},
		{"contains match reverse", "Section Name Here", "Section", true},
		{"token match", "Space Time tradeoff", "Space-Time tradeoff", true},
		{"token match multiple", "Hash Tables Overview", "Hash Tables and Maps Overview", true},
		{"notion-id heading", "Section Name", "## notion-id: abc123", false}, // Can't match notion-id by name
		{"no match", "Section One", "Section Two", false},
		{"heading path with separator", "Section Name", "# Main > ## Section Name", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := matchSection(tt.citedSection, tt.headingPath)
			if result != tt.expected {
				t.Errorf("matchSection(%q, %q) = %v, want %v", tt.citedSection, tt.headingPath, result, tt.expected)
			}
		})
	}
}

func TestExtractCitationsFromAnswer(t *testing.T) {
	ctx := context.Background()
	engine := &ragEngine{}

	tests := []struct {
		name        string
		answer     string
		chunks     []chunkData
		expected   int
		description string
	}{
		{
			name: "single citation exact match",
			answer: "Answer text here.\n[File: file.md, Section: Section Name]",
			chunks: []chunkData{
				{relPath: "file.md", headingPath: "Section Name"},
			},
			expected: 1,
			description: "Should match exact file and section",
		},
		{
			name: "multiple citations",
			answer: "Answer text.\n[File: file1.md, Section: Section 1]\n[File: file2.md, Section: Section 2]",
			chunks: []chunkData{
				{relPath: "file1.md", headingPath: "Section 1"},
				{relPath: "file2.md", headingPath: "Section 2"},
			},
			expected: 2,
			description: "Should match multiple citations",
		},
		{
			name: "path variation match",
			answer: "Answer text.\n[File: file.md, Section: Section Name]",
			chunks: []chunkData{
				{relPath: "folder/file.md", headingPath: "Section Name"},
			},
			expected: 1,
			description: "Should match with path variation",
		},
		{
			name: "section variation match",
			answer: "Answer text.\n[File: file.md, Section: Section Name]",
			chunks: []chunkData{
				{relPath: "file.md", headingPath: "## Section Name"},
			},
			expected: 1,
			description: "Should match with markdown heading variation",
		},
		{
			name: "token-based section match",
			answer: "Answer text.\n[File: file.md, Section: Space-Time tradeoff]",
			chunks: []chunkData{
				{relPath: "file.md", headingPath: "Space Time tradeoff"},
			},
			expected: 1,
			description: "Should match with token-based matching",
		},
		{
			name: "no citations",
			answer: "Answer text with no citations.",
			chunks: []chunkData{
				{relPath: "file.md", headingPath: "Section Name"},
			},
			expected: 0,
			description: "Should return empty when no citations",
		},
		{
			name: "citation format variation",
			answer: "Answer text.\n[file: FILE.MD, section: section name]",
			chunks: []chunkData{
				{relPath: "file.md", headingPath: "Section Name"},
			},
			expected: 1,
			description: "Should handle case variations",
		},
		{
			name: "multiple citations per line",
			answer: "Answer text.\n[File: file1.md, Section: Section 1] and [File: file2.md, Section: Section 2]",
			chunks: []chunkData{
				{relPath: "file1.md", headingPath: "Section 1"},
				{relPath: "file2.md", headingPath: "Section 2"},
			},
			expected: 2,
			description: "Should handle multiple citations in one line",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := engine.extractCitationsFromAnswer(ctx, tt.answer, tt.chunks)
			if len(result) != tt.expected {
				t.Errorf("extractCitationsFromAnswer() returned %d references, want %d. %s", len(result), tt.expected, tt.description)
			}
		})
	}
}

func TestTokenizeSection(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected map[string]bool
	}{
		{"simple", "Section Name", map[string]bool{"section": true, "name": true}},
		{"with separator", "Section-Name", map[string]bool{"section": true, "name": true}},
		{"with heading", "# Section Name", map[string]bool{"section": true, "name": true}},
		{"with path separator", "Main > Section", map[string]bool{"main": true, "section": true}},
		{"multiple words", "Hash Tables Overview", map[string]bool{"hash": true, "tables": true, "overview": true}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tokenizeSection(tt.input)
			if len(result) != len(tt.expected) {
				t.Errorf("tokenizeSection(%q) returned %d tokens, want %d", tt.input, len(result), len(tt.expected))
			}
			for token := range tt.expected {
				if !result[token] {
					t.Errorf("tokenizeSection(%q) missing token %q", tt.input, token)
				}
			}
		})
	}
}

