package vectorstore

import (
	"context"
	"fmt"
	"net/url"
	"strconv"

	"github.com/qdrant/go-client/qdrant"

	"helloworld-ai/internal/contextutil"
)

// QdrantStore implements VectorStore using Qdrant.
type QdrantStore struct {
	client *qdrant.Client
}

// NewQdrantStore creates a new Qdrant vector store client.
// urlStr should be in the format "http://host:port" (e.g., "http://localhost:6333").
// The gRPC port (typically 6334) will be derived from the HTTP port.
func NewQdrantStore(urlStr string) (*QdrantStore, error) {
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return nil, fmt.Errorf("invalid Qdrant URL: %w", err)
	}

	host := parsedURL.Hostname()
	if host == "" {
		host = "localhost"
	}

	// Extract port from URL, default to 6333 for HTTP
	port := 6334 // Default gRPC port
	if parsedURL.Port() != "" {
		httpPort, err := strconv.Atoi(parsedURL.Port())
		if err == nil {
			// gRPC port is typically HTTP port + 1
			port = httpPort + 1
		}
	}

	client, err := qdrant.NewClient(&qdrant.Config{
		Host: host,
		Port: port,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create Qdrant client: %w", err)
	}

	return &QdrantStore{
		client: client,
	}, nil
}

// Upsert inserts or updates points in the collection.
func (s *QdrantStore) Upsert(ctx context.Context, collection string, points []Point) error {
	logger := contextutil.LoggerFromContext(ctx)

	if len(points) == 0 {
		return nil
	}

	qdrantPoints := make([]*qdrant.PointStruct, 0, len(points))
	for _, point := range points {
		qdrantPoint := &qdrant.PointStruct{
			Id:      qdrant.NewID(point.ID),
			Vectors: qdrant.NewVectors(point.Vec...),
		}

		if len(point.Meta) > 0 {
			qdrantPoint.Payload = qdrant.NewValueMap(point.Meta)
		}

		qdrantPoints = append(qdrantPoints, qdrantPoint)
	}

	_, err := s.client.Upsert(ctx, &qdrant.UpsertPoints{
		CollectionName: collection,
		Points:         qdrantPoints,
	})
	if err != nil {
		logger.ErrorContext(ctx, "failed to upsert points", "collection", collection, "count", len(points), "error", err)
		return fmt.Errorf("failed to upsert points: %w", err)
	}

	logger.InfoContext(ctx, "upserted points", "collection", collection, "count", len(points))
	return nil
}

// Search performs a similarity search with optional filters.
func (s *QdrantStore) Search(ctx context.Context, collection string, query []float32, k int, filters map[string]any) ([]SearchResult, error) {
	logger := contextutil.LoggerFromContext(ctx)

	if k <= 0 {
		return nil, fmt.Errorf("k must be greater than 0")
	}

	// Build filter conditions
	var qdrantFilter *qdrant.Filter
	if len(filters) > 0 {
		mustConditions := make([]*qdrant.Condition, 0)

		// Handle vault_id filter (must be integer to match stored type)
		if vaultID, ok := filters["vault_id"]; ok {
			// Convert to int64 for Qdrant (vault_id is stored as integer)
			var vaultIDInt int64
			switch v := vaultID.(type) {
			case int:
				vaultIDInt = int64(v)
			case int32:
				vaultIDInt = int64(v)
			case int64:
				vaultIDInt = v
			default:
				// Try to convert via string parsing as fallback
				if str, ok := v.(string); ok {
					if parsed, err := strconv.ParseInt(str, 10, 64); err == nil {
						vaultIDInt = parsed
					} else {
						logger.WarnContext(ctx, "invalid vault_id type, skipping filter", "vault_id", vaultID, "type", fmt.Sprintf("%T", vaultID))
						// Skip this filter condition - vaultIDInt will be 0 which is invalid
						vaultIDInt = 0
					}
				} else {
					logger.WarnContext(ctx, "invalid vault_id type, skipping filter", "vault_id", vaultID, "type", fmt.Sprintf("%T", vaultID))
					// Skip this filter condition - vaultIDInt will be 0 which is invalid
					vaultIDInt = 0
				}
			}
			// Use NewMatchInt for integer matching (vault_id is stored as integer)
			if vaultIDInt != 0 {
				mustConditions = append(mustConditions, qdrant.NewMatchInt("vault_id", vaultIDInt))
			}
		}

		// Handle folder filter (prefix matching)
		if folder, ok := filters["folder"]; ok {
			folderStr := fmt.Sprintf("%v", folder)
			if folderStr != "" {
				// Use match with text for prefix matching
				// Qdrant supports prefix matching via match with text
				mustConditions = append(mustConditions, qdrant.NewMatchText("folder", folderStr))
			} else {
				// Empty string means root-level files only
				mustConditions = append(mustConditions, qdrant.NewMatch("folder", ""))
			}
		}

		if len(mustConditions) > 0 {
			qdrantFilter = &qdrant.Filter{
				Must: mustConditions,
			}
		}
	}

	limit := uint64(k)
	queryReq := &qdrant.QueryPoints{
		CollectionName: collection,
		Query:          qdrant.NewQuery(query...),
		Limit:          &limit,
		WithPayload:    qdrant.NewWithPayload(true),
	}
	if qdrantFilter != nil {
		queryReq.Filter = qdrantFilter
	}

	scoredPoints, err := s.client.Query(ctx, queryReq)
	if err != nil {
		logger.ErrorContext(ctx, "failed to search points", "collection", collection, "k", k, "error", err)
		return nil, fmt.Errorf("failed to search points: %w", err)
	}

	results := make([]SearchResult, 0, len(scoredPoints))
	for _, result := range scoredPoints {
		pointID := ""
		if result.Id != nil {
			pointID = result.Id.GetUuid()
		}

		score := result.Score

		meta := make(map[string]any)
		if result.Payload != nil {
			meta = convertPayloadToMap(result.Payload)
		}

		results = append(results, SearchResult{
			PointID: pointID,
			Score:   score,
			Meta:    meta,
		})
	}

	logger.InfoContext(ctx, "search completed", "collection", collection, "k", k, "results", len(results))
	return results, nil
}

// Delete removes points by their IDs.
func (s *QdrantStore) Delete(ctx context.Context, collection string, ids []string) error {
	logger := contextutil.LoggerFromContext(ctx)

	if len(ids) == 0 {
		return nil
	}

	qdrantIDs := make([]*qdrant.PointId, 0, len(ids))
	for _, id := range ids {
		qdrantIDs = append(qdrantIDs, qdrant.NewID(id))
	}

	_, err := s.client.Delete(ctx, &qdrant.DeletePoints{
		CollectionName: collection,
		Points:         qdrant.NewPointsSelector(qdrantIDs...),
	})
	if err != nil {
		logger.ErrorContext(ctx, "failed to delete points", "collection", collection, "count", len(ids), "error", err)
		return fmt.Errorf("failed to delete points: %w", err)
	}

	logger.InfoContext(ctx, "deleted points", "collection", collection, "count", len(ids))
	return nil
}

// CollectionExists checks if a collection exists.
func (s *QdrantStore) CollectionExists(ctx context.Context, collection string) (bool, error) {
	exists, err := s.client.CollectionExists(ctx, collection)
	if err != nil {
		return false, fmt.Errorf("failed to check collection existence: %w", err)
	}
	return exists, nil
}

// EnsureCollection ensures a collection exists with the specified vector size.
// If the collection exists, validates that the vector size matches.
// If it doesn't exist, creates it with the specified vector size.
func (s *QdrantStore) EnsureCollection(ctx context.Context, collection string, vectorSize int) error {
	logger := contextutil.LoggerFromContext(ctx)

	exists, err := s.CollectionExists(ctx, collection)
	if err != nil {
		return fmt.Errorf("failed to check collection existence: %w", err)
	}

	if !exists {
		logger.InfoContext(ctx, "creating collection", "collection", collection, "vector_size", vectorSize)
		err := s.client.CreateCollection(ctx, &qdrant.CreateCollection{
			CollectionName: collection,
			VectorsConfig: qdrant.NewVectorsConfig(&qdrant.VectorParams{
				Size:     uint64(vectorSize),
				Distance: qdrant.Distance_Cosine,
			}),
		})
		if err != nil {
			return fmt.Errorf("failed to create collection: %w", err)
		}
		logger.InfoContext(ctx, "collection created", "collection", collection, "vector_size", vectorSize)
		return nil
	}

	// Collection exists, validate vector size
	info, err := s.client.GetCollectionInfo(ctx, collection)
	if err != nil {
		return fmt.Errorf("failed to get collection info: %w", err)
	}

	// Extract vector size from collection config
	config := info.Config
	if config == nil || config.Params == nil {
		return fmt.Errorf("collection config is invalid")
	}

	vectorsConfig := config.Params.GetVectorsConfig()
	if vectorsConfig == nil {
		return fmt.Errorf("collection vectors config is invalid")
	}

	params := vectorsConfig.GetParams()
	if params == nil {
		return fmt.Errorf("collection vector params are invalid")
	}

	actualSize := params.Size
	if actualSize == 0 {
		return fmt.Errorf("could not determine collection vector size")
	}

	if int(actualSize) != vectorSize {
		return fmt.Errorf("collection vector size mismatch: expected %d, got %d", vectorSize, actualSize)
	}

	logger.InfoContext(ctx, "collection validated", "collection", collection, "vector_size", vectorSize)
	return nil
}

// GetCollectionInfo returns information about a collection including point count.
func (s *QdrantStore) GetCollectionInfo(ctx context.Context, collection string) (*CollectionInfo, error) {
	info, err := s.client.GetCollectionInfo(ctx, collection)
	if err != nil {
		return nil, fmt.Errorf("failed to get collection info: %w", err)
	}

	// Extract vector size
	var vectorSize int
	if config := info.Config; config != nil && config.Params != nil {
		if vectorsConfig := config.Params.GetVectorsConfig(); vectorsConfig != nil {
			if params := vectorsConfig.GetParams(); params != nil {
				vectorSize = int(params.Size)
			}
		}
	}

	// Extract point count (PointsCount is a pointer to uint64)
	var pointsCount int
	if info.PointsCount != nil {
		pointsCount = int(*info.PointsCount)
	}

	// Extract status (Status is an enum, not a pointer)
	status := "unknown"
	if info.Status != 0 {
		status = info.Status.String()
	}

	return &CollectionInfo{
		VectorSize:  vectorSize,
		PointsCount: pointsCount,
		Status:      status,
	}, nil
}

// CollectionInfo contains information about a Qdrant collection.
type CollectionInfo struct {
	VectorSize  int
	PointsCount int
	Status      string
}

// convertPayloadToMap converts Qdrant payload to map[string]any.
func convertPayloadToMap(payload map[string]*qdrant.Value) map[string]any {
	result := make(map[string]any, len(payload))
	for k, v := range payload {
		if v == nil {
			continue
		}
		result[k] = convertValue(v)
	}
	return result
}

// convertValue converts a Qdrant Value to Go any type.
func convertValue(v *qdrant.Value) any {
	switch val := v.Kind.(type) {
	case *qdrant.Value_BoolValue:
		return val.BoolValue
	case *qdrant.Value_IntegerValue:
		return val.IntegerValue
	case *qdrant.Value_DoubleValue:
		return val.DoubleValue
	case *qdrant.Value_StringValue:
		return val.StringValue
	case *qdrant.Value_ListValue:
		list := make([]any, len(val.ListValue.Values))
		for i, item := range val.ListValue.Values {
			list[i] = convertValue(item)
		}
		return list
	case *qdrant.Value_StructValue:
		return convertPayloadToMap(val.StructValue.Fields)
	default:
		return nil
	}
}
