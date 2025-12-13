.PHONY: run run-api start stop tilt-up tilt-down tilt-restart start-llama lint test build build-api deps clean help generate-mocks test-rag generate-swagger

# llama.cpp server configuration
LLAMA_SERVER ?= ../llama.cpp/build/bin/llama-server
LLAMA_MODEL ?= ../llama.cpp/models/llama-3-8b-instruct-q4_k_m.gguf
LLAMA_PORT ?= 8081

# API port
API_PORT ?= 9000

help:
	@echo "Available targets:"
	@echo "  start/run     - Start all services using Tilt (llama-server, API)"
	@echo "  stop          - Stop all services using Tilt"
	@echo "  tilt-up       - Start all services using Tilt"
	@echo "  tilt-down     - Stop all services using Tilt"
	@echo "  tilt-restart  - Restart all services using Tilt"
	@echo "  start-llama   - Start llama.cpp server only (port $(LLAMA_PORT))"
	@echo "  run-api       - Run the API server only (without Tilt)"
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
	@echo "Calling re-index API at http://localhost:$(API_PORT)/api/index"
	@curl -X POST http://localhost:$(API_PORT)/api/index \
		-H "Content-Type: application/json" \
		-s | jq '.' || echo "Indexing started. Check server logs for progress."

force-reindex:
	@echo "Force reindexing: clearing all existing data and rebuilding..."
	@echo "Calling force re-index API at http://localhost:$(API_PORT)/api/index?force=true"
	@curl -X POST "http://localhost:$(API_PORT)/api/index?force=true" \
		-H "Content-Type: application/json" \
		-s | jq '.' || echo "Force re-indexing started. Check server logs for progress."

clean:
	@rm -rf bin/
	@rm -rf .tilt/
	@rm -f tilt.log

