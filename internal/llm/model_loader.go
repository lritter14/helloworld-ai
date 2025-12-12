package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// ModelLoader loads models into llama.cpp server via the /models/load endpoint.
type ModelLoader struct {
	baseURL string
	client  *http.Client
}

// NewModelLoader creates a new model loader.
func NewModelLoader(baseURL string) *ModelLoader {
	return &ModelLoader{
		baseURL: baseURL,
		client:  newHTTPClient(),
	}
}

// LoadModelRequest represents the request payload for loading a model.
type LoadModelRequest struct {
	Model     string   `json:"model"`
	ExtraArgs []string `json:"extra_args,omitempty"`
}

// LoadModelResponse represents the response from the load model endpoint.
type LoadModelResponse struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

// LoadModel loads a model into the llama.cpp server with optional extra arguments.
func (ml *ModelLoader) LoadModel(ctx context.Context, modelName string, extraArgs []string) error {
	url := fmt.Sprintf("%s/models/load", ml.baseURL)

	payload := LoadModelRequest{
		Model:     modelName,
		ExtraArgs: extraArgs,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := ml.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("bad status %d: %s", resp.StatusCode, string(raw))
	}

	var loadResp LoadModelResponse
	if err := json.NewDecoder(resp.Body).Decode(&loadResp); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	if !loadResp.Success {
		return fmt.Errorf("model load failed: %s", loadResp.Error)
	}

	return nil
}
