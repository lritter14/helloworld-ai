package config

import (
	"os"
	"path/filepath"
	"testing"
)

// setEnv sets an environment variable, ignoring errors (for test setup)
func setEnv(key, value string) {
	_ = os.Setenv(key, value)
}

// unsetEnv unsets an environment variable, ignoring errors (for test cleanup)
func unsetEnv(key string) {
	_ = os.Unsetenv(key)
}

func TestLoad(t *testing.T) {
	// Save original env vars
	originalEnv := make(map[string]string)
	envVars := []string{
		"VAULT_PERSONAL_PATH", "VAULT_WORK_PATH", "QDRANT_VECTOR_SIZE",
		"LLM_BASE_URL", "LLM_API_KEY", "LLM_MODEL",
		"EMBEDDING_BASE_URL", "EMBEDDING_MODEL_NAME",
		"DB_PATH", "QDRANT_URL", "QDRANT_COLLECTION", "API_PORT",
	}
	for _, key := range envVars {
		originalEnv[key] = os.Getenv(key)
		unsetEnv(key)
	}
	defer func() {
		for key, value := range originalEnv {
			if value != "" {
				setEnv(key, value)
			} else {
				unsetEnv(key)
			}
		}
	}()

	tests := []struct {
		name        string
		setupEnv    func(*testing.T)
		wantErr     bool
		checkConfig func(*Config) bool
	}{
		{
			name: "valid config with all required fields",
			setupEnv: func(t *testing.T) {
				personalPath := t.TempDir()
				workPath := t.TempDir()
				setEnv("VAULT_PERSONAL_PATH", personalPath)
				setEnv("VAULT_WORK_PATH", workPath)
				setEnv("QDRANT_VECTOR_SIZE", "768")
			},
			wantErr: false,
			checkConfig: func(cfg *Config) bool {
				return cfg.VaultPersonalPath != "" &&
					cfg.VaultWorkPath != "" &&
					cfg.QdrantVectorSize == 768
			},
		},
		{
			name: "missing VAULT_PERSONAL_PATH",
			setupEnv: func(t *testing.T) {
				setEnv("VAULT_WORK_PATH", t.TempDir())
				setEnv("QDRANT_VECTOR_SIZE", "768")
			},
			wantErr: true,
		},
		{
			name: "missing VAULT_WORK_PATH",
			setupEnv: func(t *testing.T) {
				setEnv("VAULT_PERSONAL_PATH", t.TempDir())
				setEnv("QDRANT_VECTOR_SIZE", "768")
			},
			wantErr: true,
		},
		{
			name: "missing QDRANT_VECTOR_SIZE",
			setupEnv: func(t *testing.T) {
				setEnv("VAULT_PERSONAL_PATH", t.TempDir())
				setEnv("VAULT_WORK_PATH", t.TempDir())
			},
			wantErr: true,
		},
		{
			name: "invalid QDRANT_VECTOR_SIZE",
			setupEnv: func(t *testing.T) {
				setEnv("VAULT_PERSONAL_PATH", t.TempDir())
				setEnv("VAULT_WORK_PATH", t.TempDir())
				setEnv("QDRANT_VECTOR_SIZE", "invalid")
			},
			wantErr: true,
		},
		{
			name: "zero QDRANT_VECTOR_SIZE",
			setupEnv: func(t *testing.T) {
				setEnv("VAULT_PERSONAL_PATH", t.TempDir())
				setEnv("VAULT_WORK_PATH", t.TempDir())
				setEnv("QDRANT_VECTOR_SIZE", "0")
			},
			wantErr: true,
		},
		{
			name: "negative QDRANT_VECTOR_SIZE",
			setupEnv: func(t *testing.T) {
				setEnv("VAULT_PERSONAL_PATH", t.TempDir())
				setEnv("VAULT_WORK_PATH", t.TempDir())
				setEnv("QDRANT_VECTOR_SIZE", "-1")
			},
			wantErr: true,
		},
		{
			name: "default values for optional fields",
			setupEnv: func(t *testing.T) {
				setEnv("VAULT_PERSONAL_PATH", t.TempDir())
				setEnv("VAULT_WORK_PATH", t.TempDir())
				setEnv("QDRANT_VECTOR_SIZE", "768")
			},
			wantErr: false,
			checkConfig: func(cfg *Config) bool {
				return cfg.LLMBaseURL == "http://localhost:8081" &&
					cfg.LLMModelName == "Llama-3.1-8B-Instruct" &&
					cfg.LLMAPIKey == "dummy-key" &&
					cfg.EmbeddingBaseURL == "http://localhost:8082" &&
					cfg.EmbeddingModelName == "granite-embedding-278m-multilingual" &&
					cfg.DBPath == "./data/helloworld-ai.db" &&
					cfg.QdrantURL == "http://localhost:6333" &&
					cfg.QdrantCollection == "notes" &&
					cfg.APIPort == "9000"
			},
		},
		{
			name: "custom optional values",
			setupEnv: func(t *testing.T) {
				tmpDir := t.TempDir()
				setEnv("VAULT_PERSONAL_PATH", t.TempDir())
				setEnv("VAULT_WORK_PATH", t.TempDir())
				setEnv("QDRANT_VECTOR_SIZE", "768")
				setEnv("LLM_BASE_URL", "http://custom:9090")
				setEnv("LLM_MODEL", "custom-model")
				customDBPath := filepath.Join(tmpDir, "custom", "db.db")
				setEnv("DB_PATH", customDBPath)
			},
			wantErr: false,
			checkConfig: func(cfg *Config) bool {
				return cfg.LLMBaseURL == "http://custom:9090" &&
					cfg.LLMModelName == "custom-model" &&
					filepath.Base(cfg.DBPath) == "db.db" // Just check filename, path will vary with temp dir
			},
		},
		{
			name: "embedding has separate defaults from LLM",
			setupEnv: func(t *testing.T) {
				setEnv("VAULT_PERSONAL_PATH", t.TempDir())
				setEnv("VAULT_WORK_PATH", t.TempDir())
				setEnv("QDRANT_VECTOR_SIZE", "768")
				setEnv("LLM_BASE_URL", "http://custom:9090")
				setEnv("LLM_MODEL", "custom-model")
			},
			wantErr: false,
			checkConfig: func(cfg *Config) bool {
				// Embeddings should have their own defaults, not inherit from LLM
				return cfg.LLMBaseURL == "http://custom:9090" &&
					cfg.LLMModelName == "custom-model" &&
					cfg.EmbeddingBaseURL == "http://localhost:8082" &&
					cfg.EmbeddingModelName == "granite-embedding-278m-multilingual"
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Change to a temp directory without .env file to avoid loading it
			tmpDir := t.TempDir()
			originalWd, _ := os.Getwd()
			_ = os.Chdir(tmpDir) // Ignore error - test will fail if this doesn't work
			defer func() {
				_ = os.Chdir(originalWd) // Ignore error in cleanup
			}()

			// Clean up env vars before each test
			for _, key := range envVars {
				unsetEnv(key)
			}
			// Restore original values after test
			defer func() {
				for key, value := range originalEnv {
					if value != "" {
						setEnv(key, value)
					} else {
						unsetEnv(key)
					}
				}
			}()

			tt.setupEnv(t)

			// Verify env vars are set/unset as expected for this test
			// This helps catch issues early

			cfg, err := Load()

			if tt.wantErr {
				if err == nil {
					t.Errorf("Load() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("Load() unexpected error: %v", err)
				return
			}

			if cfg == nil {
				t.Fatal("Load() returned nil config")
			}

			if tt.checkConfig != nil && !tt.checkConfig(cfg) {
				t.Errorf("Load() config validation failed")
			}
		})
	}
}

func TestLoad_CreatesDataDirectory(t *testing.T) {
	// Save original env vars
	originalEnv := make(map[string]string)
	envVars := []string{"VAULT_PERSONAL_PATH", "VAULT_WORK_PATH", "QDRANT_VECTOR_SIZE", "DB_PATH"}
	for _, key := range envVars {
		originalEnv[key] = os.Getenv(key)
		unsetEnv(key)
	}
	defer func() {
		for key, value := range originalEnv {
			if value != "" {
				setEnv(key, value)
			} else {
				unsetEnv(key)
			}
		}
	}()

	// Use a temporary directory for testing
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test", "db.db")

	setEnv("VAULT_PERSONAL_PATH", "/tmp/personal")
	setEnv("VAULT_WORK_PATH", "/tmp/work")
	setEnv("QDRANT_VECTOR_SIZE", "768")
	setEnv("DB_PATH", dbPath)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Check that directory was created
	dir := filepath.Dir(dbPath)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Errorf("Load() should create data directory: %v", err)
	}

	if cfg.DBPath != dbPath {
		t.Errorf("Load() DBPath = %v, want %v", cfg.DBPath, dbPath)
	}
}

func TestGetEnv(t *testing.T) {
	originalValue := os.Getenv("TEST_ENV_VAR")
	defer func() {
		if originalValue != "" {
			setEnv("TEST_ENV_VAR", originalValue)
		} else {
			unsetEnv("TEST_ENV_VAR")
		}
	}()

	tests := []struct {
		name         string
		setupEnv     func()
		key          string
		defaultValue string
		want         string
	}{
		{
			name: "env var set",
			setupEnv: func() {
				setEnv("TEST_ENV_VAR", "set-value")
			},
			key:          "TEST_ENV_VAR",
			defaultValue: "default",
			want:         "set-value",
		},
		{
			name: "env var not set",
			setupEnv: func() {
				unsetEnv("TEST_ENV_VAR")
			},
			key:          "TEST_ENV_VAR",
			defaultValue: "default",
			want:         "default",
		},
		{
			name: "empty env var uses default",
			setupEnv: func() {
				setEnv("TEST_ENV_VAR", "")
			},
			key:          "TEST_ENV_VAR",
			defaultValue: "default",
			want:         "default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupEnv()
			got := getEnv(tt.key, tt.defaultValue)
			if got != tt.want {
				t.Errorf("getEnv(%q, %q) = %q, want %q", tt.key, tt.defaultValue, got, tt.want)
			}
		})
	}
}
