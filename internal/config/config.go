package config

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

// Config holds all configuration for the application.
type Config struct {
	LLMBaseURL         string
	LLMModelName       string
	LLMAPIKey          string
	EmbeddingBaseURL   string
	EmbeddingModelName string
	DBPath             string
	VaultPersonalPath  string
	VaultWorkPath      string
	QdrantURL          string
	QdrantCollection   string
	QdrantVectorSize   int
	APIPort            string
	LogLevel           slog.Level
	LogFormat          string
}

// Load reads configuration from environment variables and returns a Config struct.
// It applies defaults for optional fields and validates required fields.
// If a .env file exists in the current directory or project root, it will be loaded automatically.
// Environment variables already set take precedence over .env file values.
func Load() (*Config, error) {
	// Try to load .env file (ignore error if it doesn't exist)
	// Check current directory first, then walk up to find project root (where go.mod is)
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

	llmBaseURL := getEnv("LLM_BASE_URL", "http://localhost:8081")
	llmModelName := getEnv("LLM_MODEL", "Llama-3.1-8B-Instruct")

	// Parse log level (case-insensitive)
	logLevelStr := strings.ToUpper(getEnv("LOG_LEVEL", "INFO"))
	var logLevel slog.Level
	switch logLevelStr {
	case "DEBUG":
		logLevel = slog.LevelDebug
	case "INFO":
		logLevel = slog.LevelInfo
	case "WARN":
		logLevel = slog.LevelWarn
	case "ERROR":
		logLevel = slog.LevelError
	default:
		return nil, fmt.Errorf("invalid LOG_LEVEL: %s (must be DEBUG, INFO, WARN, or ERROR)", logLevelStr)
	}

	// Parse log format
	logFormat := strings.ToLower(getEnv("LOG_FORMAT", "text"))
	if logFormat != "text" && logFormat != "json" {
		return nil, fmt.Errorf("invalid LOG_FORMAT: %s (must be text or json)", logFormat)
	}

	cfg := &Config{
		LLMBaseURL:         llmBaseURL,
		LLMModelName:       llmModelName,
		LLMAPIKey:          getEnv("LLM_API_KEY", "dummy-key"),
		EmbeddingBaseURL:   getEnv("EMBEDDING_BASE_URL", "http://localhost:8082"),                 // Default to embeddings server
		EmbeddingModelName: getEnv("EMBEDDING_MODEL_NAME", "granite-embedding-278m-multilingual"), // Default to granite embeddings model
		// Note: granite-embedding-278m-multilingual has n_ctx=512 tokens (hard limit enforced by model).
		// The --ctx-size flag in llama.cpp is ignored; the model enforces 512 tokens maximum.
		DBPath:            getEnv("DB_PATH", "./data/helloworld-ai.db"),
		VaultPersonalPath: getEnv("VAULT_PERSONAL_PATH", ""),
		VaultWorkPath:     getEnv("VAULT_WORK_PATH", ""),
		QdrantURL:         getEnv("QDRANT_URL", "http://localhost:6333"),
		QdrantCollection:  getEnv("QDRANT_COLLECTION", "notes"),
		APIPort:           getEnv("API_PORT", "9000"),
		LogLevel:          logLevel,
		LogFormat:         logFormat,
	}

	// Parse QDRANT_VECTOR_SIZE
	// Note: This must match the output vector size of the embeddings model.
	// For granite-embedding-278m-multilingual, this is typically 1024 dimensions.
	// Verify the actual output size by testing the model and update QDRANT_VECTOR_SIZE
	// in your .env file accordingly. If the vector size changes, the Qdrant collection
	// must be recreated.
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
	cfg.QdrantVectorSize = vectorSize

	// Validate required fields
	if cfg.VaultPersonalPath == "" {
		return nil, fmt.Errorf("VAULT_PERSONAL_PATH is required")
	}
	if cfg.VaultWorkPath == "" {
		return nil, fmt.Errorf("VAULT_WORK_PATH is required")
	}

	// Create ./data directory if it doesn't exist (for future DB file)
	dataDir := filepath.Dir(cfg.DBPath)
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create data directory: %w", err)
	}

	return cfg, nil
}

// getEnv gets an environment variable or returns a default value.
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
