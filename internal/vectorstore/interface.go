package vectorstore

//go:generate go run go.uber.org/mock/mockgen@latest -destination=mocks/mock_vector_store.go -package=mocks helloworld-ai/internal/vectorstore VectorStore

import "context"

// Point represents a vector point with metadata.
type Point struct {
	ID   string
	Vec  []float32
	Meta map[string]any
}

// SearchResult represents a search result from vector search.
type SearchResult struct {
	PointID string
	Score   float32
	Meta    map[string]any
}

// VectorStore defines the interface for vector storage operations.
type VectorStore interface {
	// Upsert inserts or updates points in the collection.
	Upsert(ctx context.Context, collection string, points []Point) error

	// Search performs a similarity search with optional filters.
	Search(ctx context.Context, collection string, query []float32, k int, filters map[string]any) ([]SearchResult, error)

	// Delete removes points by their IDs.
	Delete(ctx context.Context, collection string, ids []string) error
}

