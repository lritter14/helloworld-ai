.PHONY: run run-api start stop tilt-up tilt-down tilt-restart start-llama lint test build build-api deps clean help generate-mocks test-rag generate-swagger download-models

# llama.cpp server configuration
LLAMA_SERVER ?= ../llama.cpp/build/bin/llama-server
LLAMA_MODEL ?= ../llama.cpp/models/llama-3-8b-instruct-q4_k_m.gguf
LLAMA_PORT ?= 8081

# API port
API_PORT ?= 9000

help:
	@echo "Available targets:"
	@echo "  start/run     - Start services using Tilt (Qdrant, API, Swagger UI)"
	@echo "                 NOTE: llama.cpp server must be started separately with 'make start-llama'"
	@echo "  stop          - Stop all services using Tilt"
	@echo "  tilt-up       - Start services using Tilt (Qdrant, API, Swagger UI)"
	@echo "  tilt-down     - Stop all services using Tilt"
	@echo "  tilt-restart  - Restart all services using Tilt"
	@echo "  start-llama   - Start llama.cpp server only (port $(LLAMA_PORT)) - REQUIRED before 'make start'"
	@echo "  run-api       - Run the API server only (without Tilt)"
	@echo "  download-models - Download required AI models to ../llama.cpp/models/"
	@echo "  lint          - Run Go linter"
	@echo "  test          - Run Go tests"
	@echo "  build-api     - Build the API binary (outputs to bin/helloworld-ai-api)"
	@echo "  deps          - Install Go dependencies"
	@echo "  generate-mocks - Generate mock files for testing"
	@echo "  generate-swagger - Generate Swagger/OpenAPI specification from code"
	@echo "  test-rag      - Run RAG endpoint test script"
	@echo "  reindex       - Re-index all vaults via API (skips unchanged files)"
	@echo "  force-reindex - Force re-index via API (clears all data and rebuilds from scratch)"
	@echo "  clean         - Remove build artifacts"

# Default target - start all services with Tilt
run: start

# Start all services using Tilt (recommended)
start: tilt-up

# Stop all services using Tilt
stop: tilt-down

tilt-up:
	@tilt up

tilt-down:
	@tilt down

tilt-restart:
	@tilt down && tilt up

start-llama:
	@if [ ! -f "$(LLAMA_SERVER)" ]; then \
		echo "Error: llama-server not found at $(LLAMA_SERVER)"; \
		echo "Please build llama.cpp first: cd ../llama.cpp && make"; \
		exit 1; \
	fi
	@if [ ! -f "$(LLAMA_MODEL)" ]; then \
		echo "Warning: Model file not found at $(LLAMA_MODEL)"; \
		echo "Starting server with Hugging Face model download..."; \
		$(LLAMA_SERVER) -hf ggml-org/llama-3-8b-instruct-GGUF --port $(LLAMA_PORT); \
	else \
		echo "Starting llama.cpp server on port $(LLAMA_PORT) with model $(LLAMA_MODEL)"; \
		$(LLAMA_SERVER) -m $(LLAMA_MODEL) --port $(LLAMA_PORT); \
	fi

run-api:
	@go run ./cmd/api

download-models:
	@echo "Downloading required AI models..."
	@echo "Loading configuration from .env file..."
	@mkdir -p ../llama.cpp/models
	@bash -c '\
	if [ -f .env ]; then \
		export $$(grep -v "^#" .env | grep -v "^$$" | xargs); \
	fi; \
	\
	# Download embeddings model \
	EMBEDDING_MODEL_NAME=$${EMBEDDING_MODEL_NAME:-ggml-org_embeddinggemma-300M-GGUF_embeddinggemma-300M-Q8_0}; \
	EMBEDDING_FILENAME="$$EMBEDDING_MODEL_NAME.gguf"; \
	echo ""; \
	echo "Downloading embeddings model..."; \
	echo "Model name from .env: $$EMBEDDING_MODEL_NAME"; \
	if [ ! -f "../llama.cpp/models/$$EMBEDDING_FILENAME" ]; then \
		echo "Extracting HuggingFace URL from model name..."; \
		ORG=$$(echo $$EMBEDDING_MODEL_NAME | cut -d"_" -f1); \
		REPO=$$(echo $$EMBEDDING_MODEL_NAME | cut -d"_" -f2); \
		FILE_NAME=$$(echo $$EMBEDDING_MODEL_NAME | cut -d"_" -f3-); \
		HF_URL="https://huggingface.co/$$ORG/$$REPO/resolve/main/$$FILE_NAME.gguf"; \
		echo "Downloading from: $$HF_URL"; \
		if command -v wget > /dev/null; then \
			wget -q --show-progress "$$HF_URL" \
				-O "../llama.cpp/models/$$EMBEDDING_FILENAME" || (echo "Download failed. Please check the model name and HuggingFace URL." && exit 1); \
		elif command -v curl > /dev/null; then \
			curl -L --progress-bar "$$HF_URL" \
				-o "../llama.cpp/models/$$EMBEDDING_FILENAME" || (echo "Download failed. Please check the model name and HuggingFace URL." && exit 1); \
		else \
			echo "Error: Neither wget nor curl found. Please install one of them."; \
			exit 1; \
		fi; \
		echo "Embeddings model downloaded successfully: $$EMBEDDING_FILENAME"; \
	else \
		echo "Embeddings model already exists: $$EMBEDDING_FILENAME"; \
	fi; \
	\
	# Download LLM model \
	echo ""; \
	echo "Downloading LLM model..."; \
	LLM_MODEL=$${LLM_MODEL:-Qwen2.5-3B-Instruct-Q4_K_M}; \
	echo "Model name from .env: $$LLM_MODEL"; \
	\
	# Check if model name follows {org}_{repo}_{filename} pattern \
	if echo "$$LLM_MODEL" | grep -q "^[^_]*_[^_]*_"; then \
		# Pattern: org_repo_filename \
		LLM_FILENAME="$$LLM_MODEL.gguf"; \
		ORG=$$(echo $$LLM_MODEL | cut -d"_" -f1); \
		REPO=$$(echo $$LLM_MODEL | cut -d"_" -f2); \
		FILE_NAME=$$(echo $$LLM_MODEL | cut -d"_" -f3-); \
		HF_URL="https://huggingface.co/$$ORG/$$REPO/resolve/main/$$FILE_NAME.gguf"; \
	else \
		# Pattern: just filename - try to infer from common patterns \
		LLM_FILENAME="$$LLM_MODEL.gguf"; \
		# Try common repos for Qwen models \
		if echo "$$LLM_MODEL" | grep -qi "qwen"; then \
			if echo "$$LLM_MODEL" | grep -qi "14b"; then \
				ORG="bartowski"; \
				REPO="Qwen2.5-14B-Instruct-GGUF"; \
			else \
				ORG="bartowski"; \
				REPO="Qwen2.5-3B-Instruct-GGUF"; \
			fi; \
			FILE_NAME="$$LLM_MODEL"; \
			HF_URL="https://huggingface.co/$$ORG/$$REPO/resolve/main/$$FILE_NAME.gguf"; \
		else \
			echo "Warning: Cannot infer HuggingFace repo for LLM model: $$LLM_MODEL"; \
			echo "Please use format: {org}_{repo}_{filename} or set LLM_MODEL_HF_REPO in .env"; \
			LLM_FILENAME=""; \
		fi; \
	fi; \
	\
	if [ -n "$$LLM_FILENAME" ]; then \
		if [ ! -f "../llama.cpp/models/$$LLM_FILENAME" ]; then \
			echo "Downloading from: $$HF_URL"; \
			if command -v wget > /dev/null; then \
				wget -q --show-progress "$$HF_URL" \
					-O "../llama.cpp/models/$$LLM_FILENAME" || (echo "Download failed. Please check the model name and HuggingFace URL." && exit 1); \
			elif command -v curl > /dev/null; then \
				curl -L --progress-bar "$$HF_URL" \
					-o "../llama.cpp/models/$$LLM_FILENAME" || (echo "Download failed. Please check the model name and HuggingFace URL." && exit 1); \
			else \
				echo "Error: Neither wget nor curl found. Please install one of them."; \
				exit 1; \
			fi; \
			echo "LLM model downloaded successfully: $$LLM_FILENAME"; \
		else \
			echo "LLM model already exists: $$LLM_FILENAME"; \
		fi; \
	fi; \
	\
	echo ""; \
	echo "All models processed!"; \
	echo "Model files location: ../llama.cpp/models/"; \
	echo ""; \
	echo "Note: The model names in your .env file must match the filename (without .gguf extension)."'

lint:
	@golangci-lint run || go vet ./...

test:
	@go test -v ./...

generate-mocks:
	@echo "Generating mocks..."
	@go generate ./...
	@echo "Mocks generated successfully"

generate-swagger:
	@echo "Generating Swagger specification..."
	@if ! command -v swagger > /dev/null; then \
		echo "Error: swagger CLI not found. Install it with:"; \
		echo "  go install github.com/go-swagger/go-swagger/cmd/swagger@latest"; \
		exit 1; \
	fi
	@swagger generate spec -o cmd/api/swagger.json
	@echo "Swagger specification generated: cmd/api/swagger.json"
	@echo "Note: The swagger.json file is served by the API at /api/docs/swagger.json"
	@echo "      Swagger UI is available via Tilt at http://localhost:8083"


build-api:
	@mkdir -p bin
	@go build -o bin/helloworld-ai-api ./cmd/api
	@echo "Binary built: bin/helloworld-ai-api"

deps: deps-go

deps-go:
	@go mod download
	@go mod tidy

test-rag:
	@if [ -z "$(QUESTION)" ]; then \
		./scripts/test-rag.sh; \
	else \
		./scripts/test-rag.sh "$(QUESTION)"; \
	fi

reindex:
	@echo "Calling re-index API at http://127.0.0.1:$(API_PORT)/api/index"
	@curl -X POST http://127.0.0.1:$(API_PORT)/api/index \
		-H "Content-Type: application/json" \
		-s | jq '.' || echo "Indexing started. Check server logs for progress."

force-reindex:
	@echo "Force reindexing: clearing all existing data and rebuilding..."
	@echo "Calling force re-index API at http://127.0.0.1:$(API_PORT)/api/index?force=true"
	@curl -X POST "http://127.0.0.1:$(API_PORT)/api/index?force=true" \
		-H "Content-Type: application/json" \
		-s | jq '.' || echo "Force re-indexing started. Check server logs for progress."

clean:
	@rm -rf bin/
	@rm -rf .tilt/
	@rm -f tilt.log

