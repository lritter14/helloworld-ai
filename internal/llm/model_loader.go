package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
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

// ModelStatus represents the status of a model from the /models endpoint.
type ModelStatus struct {
	ID      string `json:"id"`
	InCache bool   `json:"in_cache"`
	Status  struct {
		Value    string `json:"value"`
		ExitCode *int   `json:"exit_code,omitempty"`
		Failed   *bool  `json:"failed,omitempty"`
	} `json:"status"`
}

// ModelsResponse represents the response from the /models endpoint.
type ModelsResponse struct {
	Data []ModelStatus `json:"data"`
}

// IsModelLoaded checks if a model is already loaded (in cache) in the llama.cpp server.
func (ml *ModelLoader) IsModelLoaded(ctx context.Context, modelName string) (bool, error) {
	modelsURL := fmt.Sprintf("%s/models", ml.baseURL)
	statusReq, err := http.NewRequestWithContext(ctx, "GET", modelsURL, nil)
	if err != nil {
		return false, fmt.Errorf("failed to create status request: %w", err)
	}

	statusResp, err := ml.client.Do(statusReq)
	if err != nil {
		return false, fmt.Errorf("failed to check model status: %w", err)
	}
	defer func() {
		_ = statusResp.Body.Close()
	}()

	if statusResp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(statusResp.Body)
		return false, fmt.Errorf("bad status %d: %s", statusResp.StatusCode, string(raw))
	}

	var modelsResp ModelsResponse
	if err := json.NewDecoder(statusResp.Body).Decode(&modelsResp); err != nil {
		return false, fmt.Errorf("failed to decode models response: %w", err)
	}

	// Find our model
	for _, model := range modelsResp.Data {
		if model.ID == modelName {
			return model.InCache, nil
		}
	}

	// Model not found in the list
	return false, nil
}

// LoadModel loads a model into the llama.cpp server with optional extra arguments.
// It checks if the model is already loaded first, and only loads if not in cache.
// It waits for the model to actually load and verifies it's in cache before returning.
func (ml *ModelLoader) LoadModel(ctx context.Context, modelName string, extraArgs []string) error {
	// Check if model is already loaded
	loaded, err := ml.IsModelLoaded(ctx, modelName)
	if err != nil {
		// If we can't check status, proceed with loading attempt
		// (might be a transient error)
	} else if loaded {
		// Model is already loaded, no need to load again
		return nil
	}

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

	// Wait and verify the model actually loaded successfully
	// Poll the /models endpoint to check if the model is in cache
	// This is necessary because /models/load returns success immediately,
	// but the actual loading happens asynchronously and may fail
	modelsURL := fmt.Sprintf("%s/models", ml.baseURL)
	maxAttempts := 30 // Wait up to 30 seconds (1 second per attempt)
	for i := 0; i < maxAttempts; i++ {
		// Check model status
		statusReq, err := http.NewRequestWithContext(ctx, "GET", modelsURL, nil)
		if err != nil {
			return fmt.Errorf("failed to create status request: %w", err)
		}

		statusResp, err := ml.client.Do(statusReq)
		if err != nil {
			// If we can't check status, assume it's still loading and continue
			time.Sleep(time.Second)
			continue
		}

		var modelsResp ModelsResponse
		if err := json.NewDecoder(statusResp.Body).Decode(&modelsResp); err != nil {
			_ = statusResp.Body.Close()
			time.Sleep(time.Second)
			continue
		}
		_ = statusResp.Body.Close()

		// Find our model
		for _, model := range modelsResp.Data {
			if model.ID == modelName {
				// Check if model is in cache (successfully loaded)
				if model.InCache {
					return nil
				}
				// Check if model load failed
				if model.Status.Failed != nil && *model.Status.Failed {
					exitCode := 0
					if model.Status.ExitCode != nil {
						exitCode = *model.Status.ExitCode
					}
					return fmt.Errorf("model load failed with exit code %d", exitCode)
				}
				// Model is still loading, continue polling
				break
			}
		}

		// Wait before next attempt
		time.Sleep(time.Second)
	}

	// If we get here, the model didn't load within the timeout
	return fmt.Errorf("model did not load within timeout period")
}
