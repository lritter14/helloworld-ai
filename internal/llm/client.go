package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Client is a client for interacting with llama.cpp chat completions API.
type Client struct {
	BaseURL string
	APIKey  string
	Model   string
	client  *http.Client
}

// NewClient creates a new LLM client.
func NewClient(baseURL, apiKey, model string) *Client {
	return &Client{
		BaseURL: baseURL,
		APIKey:  apiKey,
		Model:   model,
		client:  http.DefaultClient,
	}
}

// ChatMessage represents a single message in a chat conversation.
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatRequest represents the request payload for chat completions.
type ChatRequest struct {
	Model    string        `json:"model"`
	Messages []ChatMessage `json:"messages"`
	Stream   bool          `json:"stream,omitempty"`
}

// ChatChoiceMessage represents the message in a chat choice.
type ChatChoiceMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatChoice represents a single choice in the chat response.
type ChatChoice struct {
	Index        int               `json:"index"`
	Message      ChatChoiceMessage `json:"message"`
	FinishReason string            `json:"finish_reason"`
}

// ChatResponse represents the response from the chat completions API.
type ChatResponse struct {
	ID      string       `json:"id"`
	Object  string       `json:"object"`
	Choices []ChatChoice `json:"choices"`
}

// Chat sends a chat completion request to the LLM API.
func (c *Client) Chat(ctx context.Context, message string) (string, error) {
	url := fmt.Sprintf("%s/v1/chat/completions", c.BaseURL)

	payload := ChatRequest{
		Model: c.Model,
		Messages: []ChatMessage{
			{
				Role:    "user",
				Content: message,
			},
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(body))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.APIKey))
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("bad status %d: %s", resp.StatusCode, string(raw))
	}

	var chatResp ChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("no choices returned")
	}

	return chatResp.Choices[0].Message.Content, nil
}

// StreamChat sends a streaming chat completion request to the LLM API.
// It reads Server-Sent Events (SSE) from the response and calls the callback for each chunk.
func (c *Client) StreamChat(ctx context.Context, message string, callback func(chunk string) error) error {
	url := fmt.Sprintf("%s/v1/chat/completions", c.BaseURL)

	payload := ChatRequest{
		Model: c.Model,
		Messages: []ChatMessage{
			{
				Role:    "user",
				Content: message,
			},
		},
		Stream: true,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.APIKey))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")

	resp, err := c.client.Do(req)
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

	// Read Server-Sent Events
	scanner := bufio.NewScanner(resp.Body)
	var dataPrefix = "data: "
	var donePrefix = "[DONE]"

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		if !strings.HasPrefix(line, dataPrefix) {
			continue
		}

		data := strings.TrimPrefix(line, dataPrefix)
		if data == donePrefix {
			break
		}

		var streamResp struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
				FinishReason string `json:"finish_reason"`
			} `json:"choices"`
		}

		if err := json.Unmarshal([]byte(data), &streamResp); err != nil {
			// Skip malformed JSON chunks
			continue
		}

		if len(streamResp.Choices) > 0 {
			chunk := streamResp.Choices[0].Delta.Content
			if chunk != "" {
				if err := callback(chunk); err != nil {
					return fmt.Errorf("callback error: %w", err)
				}
			}

			// Check if stream is finished
			if streamResp.Choices[0].FinishReason != "" {
				break
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("failed to read stream: %w", err)
	}

	return nil
}
