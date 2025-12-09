package llm

// Message represents a single message in a chat conversation.
// This type is used by the RAG engine and other structured message consumers.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatParams holds parameters for chat completion requests.
type ChatParams struct {
	// Model specifies the model to use. If empty, the client's default model is used.
	Model string

	// MaxTokens specifies the maximum number of tokens to generate.
	// If 0, no limit is applied.
	MaxTokens int

	// Temperature controls the randomness of the output.
	// Default is 0.7 if not specified.
	Temperature float32
}
