# Configuration Layer - Agent Guide

Configuration loading and validation patterns.

## Load Pattern

The `Load()` function automatically loads `.env` files from the project root, then reads environment variables. Environment variables take precedence over `.env` file values.

```go
func Load() (*Config, error) {
    // Automatically load .env file (ignores error if not found)
    _ = godotenv.Load() // Try current directory
    
    // Try to find project root by looking for go.mod
    wd, err := os.Getwd()
    if err == nil {
        dir := wd
        for i := 0; i < 5; i++ { // Limit search depth
            envPath := filepath.Join(dir, ".env")
            if _, err := os.Stat(envPath); err == nil {
                _ = godotenv.Load(envPath)
                break
            }
            parent := filepath.Dir(dir)
            if parent == dir {
                break // Reached filesystem root
            }
            dir = parent
        }
    }
    
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

## .env File Support

The config package automatically loads `.env` files using `github.com/joho/godotenv`:

- **Search Order:** Current directory → project root (where `go.mod` is)
- **Priority:** Environment variables > `.env` file values
- **Silent Failure:** If `.env` doesn't exist, continues with environment variables only
- **No Dependencies:** Works when running `go run ./cmd/api` directly (no Tilt required)

## Environment Helper

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

- **Automatic .env Loading:** Search for `.env` file in project root automatically
- **Environment Variable Priority:** Environment variables override `.env` file values
- **Validate Required Fields:** Fail fast at startup with clear error messages
- **Sensible Defaults:** Embeddings default to LLM settings when not specified
- **Type Conversion:** Handle type conversion with proper error messages
- **Directory Creation:** Create necessary directories (e.g., data directory)
- **Cross-platform:** Use `filepath` package for path operations
