# Configuration Layer - Agent Guide

Configuration loading and validation patterns.

## Load Pattern

```go
func Load() (*Config, error) {
    llmBaseURL := getEnv("LLM_BASE_URL", "http://localhost:8080")
    llmModelName := getEnv("LLM_MODEL", "local-model")
    
    cfg := &Config{
        LLMBaseURL:        llmBaseURL,
        LLMModelName:      llmModelName,
        LLMAPIKey:         getEnv("LLM_API_KEY", "dummy-key"),
        EmbeddingBaseURL:  getEnv("EMBEDDING_BASE_URL", llmBaseURL),  // Defaults to LLM_BASE_URL
        EmbeddingModelName: getEnv("EMBEDDING_MODEL_NAME", llmModelName), // Defaults to LLM_MODEL
        DBPath:            getEnv("DB_PATH", "./data/helloworld-ai.db"),
        VaultPersonalPath: getEnv("VAULT_PERSONAL_PATH", ""),
        VaultWorkPath:     getEnv("VAULT_WORK_PATH", ""),
        QdrantURL:         getEnv("QDRANT_URL", "http://localhost:6333"),
        QdrantCollection:  getEnv("QDRANT_COLLECTION", "notes"),
        APIPort:           getEnv("API_PORT", "9000"),
    }
    
    // Validate required fields
    if cfg.VaultPersonalPath == "" {
        return nil, fmt.Errorf("VAULT_PERSONAL_PATH is required")
    }
    
    return cfg, nil
}
```

## Environment Helper

```go
func getEnv(key, defaultValue string) string {
    if value := os.Getenv(key); value != "" {
        return value
    }
    return defaultValue
}
```

## Validation

```go
// Type conversion with validation
vectorSizeStr := getEnv("QDRANT_VECTOR_SIZE", "")
if vectorSizeStr == "" {
    return nil, fmt.Errorf("QDRANT_VECTOR_SIZE is required")
}
vectorSize, err := strconv.Atoi(vectorSizeStr)
if err != nil {
    return nil, fmt.Errorf("QDRANT_VECTOR_SIZE must be a valid integer: %w", err)
}
if vectorSize <= 0 {
    return nil, fmt.Errorf("QDRANT_VECTOR_SIZE must be greater than 0")
}
```

## Configuration Fields

**LLM Configuration:**
- `LLMBaseURL` - Base URL for chat completions API
- `LLMModelName` - Model name for chat completions
- `LLMAPIKey` - API key for authentication

**Embeddings Configuration:**
- `EmbeddingBaseURL` - Base URL for embeddings API (defaults to `LLMBaseURL`)
- `EmbeddingModelName` - Model name for embeddings (defaults to `LLMModelName`)

**Vector Store Configuration:**
- `QdrantURL` - Qdrant server URL
- `QdrantCollection` - Collection name
- `QdrantVectorSize` - Required vector size (validated > 0)

**Vault Configuration:**
- `VaultPersonalPath` - Required path to personal vault
- `VaultWorkPath` - Required path to work vault

## Rules

- Validate required fields at startup
- Provide sensible defaults (embeddings default to LLM settings)
- Handle type conversion with errors
- Create necessary directories (e.g., data directory)
- Return clear error messages
