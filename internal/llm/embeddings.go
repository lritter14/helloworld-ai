package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// EmbeddingsClient is a client for interacting with llama.cpp embeddings API.
type EmbeddingsClient struct {
	BaseURL      string
	APIKey       string
	Model        string
	ExpectedSize int // Expected vector size for validation
	client       *http.Client
}

// NewEmbeddingsClient creates a new embeddings client.
// expectedSize is the expected vector size (from QDRANT_VECTOR_SIZE config).
// All embeddings returned by EmbedTexts will be validated against this size.
func NewEmbeddingsClient(baseURL, apiKey, model string, expectedSize int) *EmbeddingsClient {
	return &EmbeddingsClient{
		BaseURL:      baseURL,
		APIKey:       apiKey,
		Model:        model,
		ExpectedSize: expectedSize,
		client:       http.DefaultClient,
	}
}

// EmbeddingsRequest represents the request payload for embeddings API.
type EmbeddingsRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

// EmbeddingData represents a single embedding in the response.
type EmbeddingData struct {
	Embedding []float64 `json:"embedding"`
}

// EmbeddingsResponse represents the response from the embeddings API.
type EmbeddingsResponse struct {
	Data []EmbeddingData `json:"data"`
}

// EmbedTexts generates embeddings for the given texts.
// Returns a slice of float32 vectors, one per input text.
// Validates that all returned vectors match the expected size.
func (c *EmbeddingsClient) EmbedTexts(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, fmt.Errorf("empty input array")
	}

	url := fmt.Sprintf("%s/v1/embeddings", c.BaseURL)

	payload := EmbeddingsRequest{
		Model: c.Model,
		Input: texts,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.APIKey))
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("bad status %d: %s", resp.StatusCode, string(raw))
	}

	var embeddingsResp EmbeddingsResponse
	if err := json.NewDecoder(resp.Body).Decode(&embeddingsResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if len(embeddingsResp.Data) != len(texts) {
		return nil, fmt.Errorf("expected %d embeddings, got %d", len(texts), len(embeddingsResp.Data))
	}

	// Convert []float64 to []float32 and validate size
	result := make([][]float32, len(embeddingsResp.Data))
	for i, data := range embeddingsResp.Data {
		if len(data.Embedding) != c.ExpectedSize {
			return nil, fmt.Errorf("embedding %d has size %d, expected %d", i, len(data.Embedding), c.ExpectedSize)
		}

		// Convert []float64 to []float32
		vec := make([]float32, len(data.Embedding))
		for j, v := range data.Embedding {
			vec[j] = float32(v)
		}
		result[i] = vec
	}

	return result, nil
}

