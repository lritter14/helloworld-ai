package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"

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

	llmBaseURL := getEnv("LLM_BASE_URL", "http://localhost:8080")
	llmModelName := getEnv("LLM_MODEL", "local-model")

	cfg := &Config{
		LLMBaseURL:         llmBaseURL,
		LLMModelName:       llmModelName,
		LLMAPIKey:          getEnv("LLM_API_KEY", "dummy-key"),
		EmbeddingBaseURL:   getEnv("EMBEDDING_BASE_URL", llmBaseURL),     // Default to same as LLM_BASE_URL
		EmbeddingModelName: getEnv("EMBEDDING_MODEL_NAME", llmModelName), // Default to same as LLM_MODEL
		DBPath:             getEnv("DB_PATH", "./data/helloworld-ai.db"),
		VaultPersonalPath:  getEnv("VAULT_PERSONAL_PATH", ""),
		VaultWorkPath:      getEnv("VAULT_WORK_PATH", ""),
		QdrantURL:          getEnv("QDRANT_URL", "http://localhost:6333"),
		QdrantCollection:   getEnv("QDRANT_COLLECTION", "notes"),
		APIPort:            getEnv("API_PORT", "9000"),
	}

	// Parse QDRANT_VECTOR_SIZE
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
