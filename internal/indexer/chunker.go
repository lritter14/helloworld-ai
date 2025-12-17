package indexer

import (
	"fmt"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/text"
)

const (
	minChunkSize = 50
	maxChunkSize = 700 // Max runes per chunk (targets ~450 tokens for 512-token embedding model)
)

// GoldmarkChunker chunks markdown content using goldmark AST parsing.
type GoldmarkChunker struct {
	parser goldmark.Markdown
}

// NewGoldmarkChunker creates a new goldmark chunker.
func NewGoldmarkChunker() *GoldmarkChunker {
	return &GoldmarkChunker{
		parser: goldmark.New(
			goldmark.WithExtensions(extension.Table),
		),
	}
}

// ChunkMarkdown parses markdown content and returns the title and chunks.
// The chunks are organized by heading hierarchy with size constraints.
func (c *GoldmarkChunker) ChunkMarkdown(content []byte, filename string) (title string, chunks []Chunk, err error) {
	if len(content) == 0 {
		// Empty file: use filename as title, return empty chunks
		title = extractTitleFromFilename(filename)
		return title, []Chunk{}, nil
	}

	// Parse markdown into AST
	reader := text.NewReader(content)
	doc := c.parser.Parser().Parse(reader)

	// Extract title first (needed for first chunk heading path)
	title = extractTitle(doc, content, filename)

	// Walk AST to build chunks
	chunks = c.buildChunks(doc, content, title)

	// Apply size constraints: merge tiny chunks, split oversized chunks
	chunks = c.applySizeConstraints(chunks)

	return title, chunks, nil
}

// extractTitle extracts the document title per Section 0.7:
// 1. First # Heading (level 1)
// 2. First ## Heading (level 2) if no level 1
// 3. Filename without extension (capitalize words) if no headings
func extractTitle(doc ast.Node, content []byte, filename string) string {
	var firstH1, firstH2 string

	// Walk AST to find first heading
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}

		if heading, ok := n.(*ast.Heading); ok {
			level := heading.Level
			headingText := extractTextFromNode(heading, content)

			if level == 1 && firstH1 == "" {
				firstH1 = headingText
			} else if level == 2 && firstH2 == "" && firstH1 == "" {
				firstH2 = headingText
			}

			// Stop walking once we have what we need
			if firstH1 != "" {
				return ast.WalkStop, nil
			}
		}

		return ast.WalkContinue, nil
	})

	if firstH1 != "" {
		return firstH1
	}
	if firstH2 != "" {
		return firstH2
	}

	// No headings found, use filename
	return extractTitleFromFilename(filename)
}

// extractTitleFromFilename extracts title from filename by removing extension and capitalizing words.
func extractTitleFromFilename(filename string) string {
	// Remove extension
	name := filepath.Base(filename)
	ext := filepath.Ext(name)
	if ext != "" {
		name = name[:len(name)-len(ext)]
	}

	// Capitalize first letter of each word
	words := strings.Fields(name)
	for i, word := range words {
		if len(word) > 0 {
			runes := []rune(word)
			runes[0] = unicode.ToUpper(runes[0])
			words[i] = string(runes)
		}
	}

	return strings.Join(words, " ")
}

// buildChunks walks the AST and builds chunks based on heading hierarchy.
func (c *GoldmarkChunker) buildChunks(doc ast.Node, content []byte, docTitle string) []Chunk {
	var chunks []Chunk
	var currentChunk *Chunk
	headingStack := []headingInfo{} // Stack to track heading hierarchy
	chunkIndex := 0

	// Track if we've seen any heading yet
	seenFirstHeading := false

	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			// When exiting a heading, we've finished collecting its content
			if _, ok := n.(*ast.Heading); ok {
				return ast.WalkContinue, nil
			}
			return ast.WalkContinue, nil
		}

		switch node := n.(type) {
		case *ast.Heading:
			// Found a heading - start a new chunk
			seenFirstHeading = true

			// Update heading stack: remove headings of equal or higher level
			level := node.Level
			for len(headingStack) > 0 && headingStack[len(headingStack)-1].level >= level {
				headingStack = headingStack[:len(headingStack)-1]
			}

			// Add current heading to stack
			headingText := extractTextFromNode(node, content)
			headingStack = append(headingStack, headingInfo{
				level: level,
				text:  headingText,
			})

			// Finalize previous chunk if exists
			if currentChunk != nil && len(currentChunk.Text) > 0 {
				chunks = append(chunks, *currentChunk)
				chunkIndex++
			}

			// Start new chunk
			headingPath := buildHeadingPath(headingStack)
			currentChunk = &Chunk{
				Index:       chunkIndex,
				HeadingPath: headingPath,
				Text:        "",
			}

			return ast.WalkContinue, nil

		case *ast.Text:
			// Collect text content
			if currentChunk == nil {
				// Text before any heading - use document title as heading path
				if !seenFirstHeading {
					currentChunk = &Chunk{
						Index:       chunkIndex,
						HeadingPath: "# " + docTitle,
						Text:        "",
					}
				}
			}
			if currentChunk != nil {
				segment := node.Segment
				text := string(segment.Value(content))
				currentChunk.Text += text
			}
			return ast.WalkContinue, nil

		case *ast.String:
			// Collect string content
			if currentChunk != nil {
				currentChunk.Text += string(node.Value)
			}
			return ast.WalkContinue, nil

		case *ast.CodeSpan:
			// Collect code span content by walking its children
			if currentChunk != nil {
				codeText := extractTextFromNode(node, content)
				currentChunk.Text += codeText
			}
			return ast.WalkContinue, nil

		case *ast.CodeBlock:
			// Collect code block content
			if currentChunk != nil {
				lines := node.Lines()
				for i := 0; i < lines.Len(); i++ {
					line := lines.At(i)
					currentChunk.Text += string(line.Value(content))
				}
			}
			return ast.WalkContinue, nil

		case *ast.Paragraph:
			// Add newline before paragraph content
			if currentChunk != nil && len(currentChunk.Text) > 0 && !strings.HasSuffix(currentChunk.Text, "\n") {
				currentChunk.Text += "\n"
			}
			return ast.WalkContinue, nil

		case *ast.List:
			// Add newline before list
			if currentChunk != nil && len(currentChunk.Text) > 0 && !strings.HasSuffix(currentChunk.Text, "\n") {
				currentChunk.Text += "\n"
			}
			return ast.WalkContinue, nil

		case *ast.ListItem:
			// Add newline before list item
			if currentChunk != nil && len(currentChunk.Text) > 0 && !strings.HasSuffix(currentChunk.Text, "\n") {
				currentChunk.Text += "\n"
			}
			return ast.WalkContinue, nil

		default:
			// Check if this is a table-related node by checking the node kind name
			// Table extension nodes will have kind names containing "Table"
			kindName := n.Kind().String()
			if strings.Contains(kindName, "Table") {
				if currentChunk == nil {
					return ast.WalkContinue, nil
				}

				// Add newline before table
				if strings.Contains(kindName, "Table") && !strings.Contains(kindName, "TableRow") && !strings.Contains(kindName, "TableCell") && !strings.Contains(kindName, "TableHeader") {
					if len(currentChunk.Text) > 0 && !strings.HasSuffix(currentChunk.Text, "\n") {
						currentChunk.Text += "\n"
					}
				}

				// Handle table rows - add newline before each row and extract cell content
				if strings.Contains(kindName, "TableRow") || strings.Contains(kindName, "TableHeader") {
					if len(currentChunk.Text) > 0 && !strings.HasSuffix(currentChunk.Text, "\n") {
						currentChunk.Text += "\n"
					}
					// Extract row content by walking children (cells)
					rowText := extractTableRowText(n, content)
					currentChunk.Text += rowText
					currentChunk.Text += "\n"
					return ast.WalkSkipChildren, nil // We've already extracted the row content
				}

				// Skip TableCell nodes - they're handled by extractTableRowText when processing rows
				if strings.Contains(kindName, "TableCell") {
					return ast.WalkSkipChildren, nil
				}
			}
			// For other nodes, continue walking to collect text content
			return ast.WalkContinue, nil
		}
	})

	// Finalize last chunk if exists
	if currentChunk != nil && len(currentChunk.Text) > 0 {
		chunks = append(chunks, *currentChunk)
	}

	// If no chunks were created (no headings and no content), create one with title
	if len(chunks) == 0 {
		chunks = append(chunks, Chunk{
			Index:       0,
			HeadingPath: "# " + docTitle,
			Text:        string(content),
		})
	}

	return chunks
}

// headingInfo tracks heading level and text for building heading paths.
type headingInfo struct {
	level int
	text  string
}

// buildHeadingPath builds a heading path string from the heading stack.
// Format: "# Heading1 > ## Heading2 > ### Heading3"
func buildHeadingPath(stack []headingInfo) string {
	if len(stack) == 0 {
		return ""
	}

	parts := make([]string, len(stack))
	for i, h := range stack {
		hashes := strings.Repeat("#", h.level)
		parts[i] = fmt.Sprintf("%s %s", hashes, h.text)
	}

	return strings.Join(parts, " > ")
}

// extractTextFromNode extracts text content from a node and its children.
func extractTextFromNode(n ast.Node, content []byte) string {
	var textBuilder strings.Builder

	_ = ast.Walk(n, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}

		switch v := node.(type) {
		case *ast.Text:
			segment := v.Segment
			textBuilder.Write(segment.Value(content))
		case *ast.String:
			textBuilder.Write(v.Value)
		case *ast.CodeSpan:
			// CodeSpan content is in child text nodes, already handled by *ast.Text case
			// Just continue walking
		}
		return ast.WalkContinue, nil
	})

	return strings.TrimSpace(textBuilder.String())
}

// extractTableRowText extracts text from a table row, formatting cells with pipe separators.
func extractTableRowText(row ast.Node, content []byte) string {
	var rowBuilder strings.Builder
	cellCount := 0

	_ = ast.Walk(row, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}

		kindName := node.Kind().String()
		if strings.Contains(kindName, "TableCell") {
			cellText := extractTextFromNode(node, content)
			cellText = strings.TrimSpace(cellText)
			if cellCount > 0 {
				rowBuilder.WriteString(" | ")
			}
			rowBuilder.WriteString(cellText)
			cellCount++
			return ast.WalkSkipChildren, nil // Already extracted cell content
		}
		return ast.WalkContinue, nil
	})

	return rowBuilder.String()
}

// applySizeConstraints applies min/max size constraints to chunks.
// - Merge chunks smaller than minChunkSize with the next chunk
// - Merge chunks with the same heading path (helps with content before headings)
// - Split chunks larger than maxChunkSize (prefer heading boundaries, but split if needed)
// Size is measured in runes (not bytes) for consistency with embedding token estimation.
func (c *GoldmarkChunker) applySizeConstraints(chunks []Chunk) []Chunk {
	if len(chunks) == 0 {
		return chunks
	}

	result := []Chunk{}
	i := 0

	for i < len(chunks) {
		current := chunks[i]
		currentRunes := utf8.RuneCountInString(current.Text)

		// First, try to merge chunks with the same heading path
		// This helps when content appears before a heading (like descriptions before tables)
		if i+1 < len(chunks) {
			next := chunks[i+1]
			if current.HeadingPath == next.HeadingPath && current.HeadingPath != "" {
				// Same heading path - merge them
				merged := Chunk{
					Index:       current.Index,
					HeadingPath: current.HeadingPath,
					Text:        current.Text + "\n\n" + next.Text,
				}

				// If merged chunk is still reasonable, use it
				if utf8.RuneCountInString(merged.Text) <= maxChunkSize {
					current = merged
					currentRunes = utf8.RuneCountInString(current.Text)
					i++ // Skip next chunk since we merged it
				}
			}
		}

		// If chunk is too small, try to merge with next
		if currentRunes < minChunkSize && i+1 < len(chunks) {
			next := chunks[i+1]
			merged := Chunk{
				Index:       current.Index,
				HeadingPath: current.HeadingPath,
				Text:        current.Text + "\n\n" + next.Text,
			}

			// If merged chunk is still reasonable, use it
			if utf8.RuneCountInString(merged.Text) <= maxChunkSize {
				current = merged
				currentRunes = utf8.RuneCountInString(current.Text)
				i++ // Skip next chunk since we merged it
			}
		}

		// If chunk is too large, split it
		if currentRunes > maxChunkSize {
			splitChunks := c.splitChunk(current)
			result = append(result, splitChunks...)
		} else {
			result = append(result, current)
		}

		i++
	}

	// Re-index chunks
	for i := range result {
		result[i].Index = i
	}

	return result
}

// splitChunk splits a chunk that exceeds maxChunkSize.
// Tries to split at paragraph boundaries, otherwise splits at sentence boundaries, otherwise hard split.
// Size is measured in runes (not bytes) for consistency with embedding token estimation.
func (c *GoldmarkChunker) splitChunk(chunk Chunk) []Chunk {
	chunkRunes := utf8.RuneCountInString(chunk.Text)
	if chunkRunes <= maxChunkSize {
		return []Chunk{chunk}
	}

	var splits []Chunk
	text := chunk.Text
	textRunes := []rune(text) // Convert to runes for proper indexing
	start := 0
	splitIndex := 0

	for start < len(textRunes) {
		end := start + maxChunkSize

		if end >= len(textRunes) {
			// Last chunk
			splits = append(splits, Chunk{
				Index:       chunk.Index + splitIndex,
				HeadingPath: chunk.HeadingPath,
				Text:        string(textRunes[start:]),
			})
			break
		}

		// Try to find a good split point (paragraph boundary)
		// Search in the rune slice, but convert back to string for boundary detection
		searchText := string(textRunes[start:end])
		splitPoint := end
		if paragraphBoundary := strings.LastIndex(searchText, "\n\n"); paragraphBoundary != -1 {
			splitPoint = start + paragraphBoundary + 2
		} else if newlineBoundary := strings.LastIndex(searchText, "\n"); newlineBoundary != -1 {
			splitPoint = start + newlineBoundary + 1
		} else if sentenceBoundary := strings.LastIndex(searchText, ". "); sentenceBoundary != -1 {
			splitPoint = start + sentenceBoundary + 2
		}

		splits = append(splits, Chunk{
			Index:       chunk.Index + splitIndex,
			HeadingPath: chunk.HeadingPath,
			Text:        string(textRunes[start:splitPoint]),
		})

		start = splitPoint
		splitIndex++
	}

	return splits
}
