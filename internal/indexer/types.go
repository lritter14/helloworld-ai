package indexer

// Chunk represents a chunk of text from a markdown document.
type Chunk struct {
	Index       int    // Chunk index within note (starts at 0)
	HeadingPath string // Format: "# Heading1 > ## Heading2"
	Text        string // Chunk text content
}
