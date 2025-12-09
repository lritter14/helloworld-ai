# Configuration Layer - Agent Guide

Configuration loading and validation patterns.

## Load Pattern

```go
func Load() (*Config, error) {
    cfg := &Config{
        LLMBaseURL:   getEnv("LLM_BASE_URL", "http://localhost:8080"),
        LLMModelName: getEnv("LLM_MODEL", "local-model"),
        // ... more fields
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

## Rules

- Validate required fields at startup
- Provide sensible defaults
- Handle type conversion with errors
- Create necessary directories
- Return clear error messages
