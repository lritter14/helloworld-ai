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
    llmModelName := getEnv("LLM_MODEL", "Llama-3.1-8B-Instruct")
    
    cfg := &Config{
        LLMBaseURL:        llmBaseURL,
        LLMModelName:      llmModelName,
        LLMAPIKey:         getEnv("LLM_API_KEY", "dummy-key"),
        EmbeddingBaseURL:  getEnv("EMBEDDING_BASE_URL", "http://localhost:8081"),  // Defaults to embeddings server
        EmbeddingModelName: getEnv("EMBEDDING_MODEL_NAME", "granite-embedding-278m-multilingual"), // Defaults to granite embeddings model
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
- `EmbeddingBaseURL` - Base URL for embeddings API (default: `http://localhost:8081`)
- `EmbeddingModelName` - Model name for embeddings (default: `granite-embedding-278m-multilingual`)
- **Note:** The embedding model has a hard context size limit of 512 tokens. The `QDRANT_VECTOR_SIZE` must match the output vector size of the embeddings model (typically 1024 for granite-embedding-278m-multilingual).

**Vector Store Configuration:**
- `QdrantURL` - Qdrant server URL
- `QdrantCollection` - Collection name
- `QdrantVectorSize` - Required vector size (validated > 0)

**Vault Configuration:**
- `VaultPersonalPath` - Required path to personal vault
- `VaultWorkPath` - Required path to work vault

## Testing

### Test Patterns

**Environment Variable Isolation:**

Tests change to temporary directories to avoid loading `.env` files:

```go
t.Run(tt.name, func(t *testing.T) {
    // Change to temp directory without .env file
    tmpDir := t.TempDir()
    originalWd, _ := os.Getwd()
    _ = os.Chdir(tmpDir)
    defer func() {
        _ = os.Chdir(originalWd) // Ignore error in cleanup
    }()
    
    // Test code here
})
```

**Helper Functions:**

Use helper functions to ignore errors in test setup:

```go
// setEnv sets an environment variable, ignoring errors (for test setup)
func setEnv(key, value string) {
    _ = os.Setenv(key, value)
}

// unsetEnv unsets an environment variable, ignoring errors (for test cleanup)
func unsetEnv(key string) {
    _ = os.Unsetenv(key)
}
```

**Temporary Directories:**

Use `t.TempDir()` for all test paths:

```go
setupEnv: func(t *testing.T) {
    setEnv("VAULT_PERSONAL_PATH", t.TempDir())
    setEnv("VAULT_WORK_PATH", t.TempDir())
    setEnv("QDRANT_VECTOR_SIZE", "768")
}
```

## Rules

- **Automatic .env Loading:** Search for `.env` file in project root automatically
- **Environment Variable Priority:** Environment variables override `.env` file values
- **Validate Required Fields:** Fail fast at startup with clear error messages
- **Separate Embedding Defaults:** Embeddings have separate defaults (different server and model)
- **Type Conversion:** Handle type conversion with proper error messages
- **Directory Creation:** Create necessary directories (e.g., data directory)
- **Cross-platform:** Use `filepath` package for path operations
- **Test Isolation:** Use temporary directories and change working directory in tests
- **Error Handling:** Use helper functions to ignore errors in test setup/cleanup
- **Vector Size Validation:** `QDRANT_VECTOR_SIZE` must match the output vector size of the embeddings model
