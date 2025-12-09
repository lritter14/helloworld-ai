# HelloWorld AI

A Go-based API server with embedded web UI for interacting with local LLMs via llama.cpp. The project implements a RAG (Retrieval-Augmented Generation) system that indexes markdown notes from vaults and enables question-answering over the indexed content.

## Architecture

The application consists of a single binary:

- **API Server** (`cmd/api`) - Provides chat endpoints and serves the web UI at `/`
- The web UI is a simple HTML page embedded in the Go binary

### Technology Stack

- **Language:** Go 1.25.3+
- **UI:** Single embedded HTML page served by Go
- **Model Runtime:** llama.cpp with OpenAI-compatible HTTP API
- **Vector DB:** Qdrant (Docker)
- **Metadata DB:** SQLite
- **Vaults:** 2 vaults (personal + work)

## Prerequisites

- Go 1.25.3 or later
- llama.cpp server running (see `start-llama` target)
- Qdrant running (Docker)
- (Optional) Tilt for unified development workflow

## Configuration

### Environment File (.env)

The project automatically loads configuration from a `.env` file in the project root. The Go service reads `.env` files automatically, so you can run `go run ./cmd/api` directly without manually exporting environment variables. Create a `.env` file in the project root with the following variables:

```bash
# llama.cpp Server Configuration
LLAMA_SERVER_PATH=../llama.cpp/build/bin/llama-server
LLAMA_MODEL_PATH=../llama.cpp/models/llama-3-8b-instruct-q4_k_m.gguf
LLAMA_PORT=8080

# API Server Configuration
API_PORT=9000

# LLM Configuration
LLM_BASE_URL=http://localhost:8080
LLM_API_KEY=dummy-key
LLM_MODEL=local-model

# Qdrant Configuration
QDRANT_VECTOR_SIZE=4096

# Vault Configuration
VAULT_PERSONAL_PATH=./vaults/personal
VAULT_WORK_PATH=./vaults/work
```

A `.env` file with default values is included in the repository. Modify it according to your local setup.

## Quick Start

### Option 1: Using Tilt (Recommended for Development)

Tilt manages all services and dependencies automatically. It reads configuration from the `.env` file:

```bash
tilt up
```

This will:

- Start llama.cpp server (port 8080)
- Start Qdrant (port 6333)
- Start API server (port 9000) - serves both API and web UI
- Watch for file changes and auto-reload
- Provide a web UI at `http://localhost:10350` to view logs and status

Access the application at `http://localhost:9000`

To stop all services:

```bash
tilt down
```

### Option 2: Using Make

#### 1. Start llama.cpp server

```bash
make start-llama
```

#### 2. Start Qdrant

```bash
docker run -d -p 6333:6333 qdrant/qdrant
```

#### 3. Run API server

```bash
make run-api
```

This starts:

- API server on `http://localhost:9000` (serves both API and web UI)

#### 4. Access the UI

Open `http://localhost:9000` in your browser.

## Running

### API Server

```bash
make run-api
# Or with custom port:
API_PORT=9000 go run ./cmd/api
```

The API server serves:

- Web UI at `http://localhost:9000/`
- Chat API endpoint at `http://localhost:9000/api/chat` (basic LLM chat)
- RAG API endpoint at `http://localhost:9000/api/v1/ask` (question-answering over indexed notes with intelligent folder selection)

### Indexing

On startup, the API server automatically indexes all markdown files from both vaults:

- Scans all `.md` files in personal and work vaults
- Chunks files by heading hierarchy (min 50 chars, max 2000 chars per chunk)
- Generates embeddings for each chunk with automatic batch size reduction on errors
- Stores metadata in SQLite and vectors in Qdrant
- Uses hash-based change detection to skip unchanged files
- Validates embedding vector size at startup (fail-fast if mismatch)

Indexing runs synchronously at startup. Errors for individual files are logged but don't prevent the server from starting. The indexer automatically handles embedding batch size errors by splitting batches in half and retrying. Check logs for indexing progress and any errors.

### API Server Environment Variables

When running the API server directly (not via Tilt), you can set these environment variables:

**Required:**

- `VAULT_PERSONAL_PATH` - Path to personal vault directory
- `VAULT_WORK_PATH` - Path to work vault directory
- `QDRANT_VECTOR_SIZE` - Vector size for embeddings (must be > 0)

**Optional (with defaults):**

- `LLM_BASE_URL` - Base URL for llama.cpp server (default: `http://localhost:8080`)
- `LLM_API_KEY` - API key for llama.cpp (default: `dummy-key`)
- `LLM_MODEL` - Model name to use (default: `local-model`)
- `EMBEDDING_BASE_URL` - Base URL for embeddings (default: same as `LLM_BASE_URL`)
- `EMBEDDING_MODEL_NAME` - Model name for embeddings (default: same as `LLM_MODEL`)
- `DB_PATH` - Path to SQLite database (default: `./data/helloworld-ai.db`)
- `QDRANT_URL` - Qdrant server URL (default: `http://localhost:6333`)
- `QDRANT_COLLECTION` - Qdrant collection name (default: `notes`)
- `API_PORT` - Port for API server (default: `9000`)

**Note:** The Go service automatically loads `.env` files from the project root. Environment variables take precedence over `.env` file values if both are set. When using Tilt, the `.env` file is automatically loaded by the Go service.

## Building

Build both binaries:

```bash
make build
```

Build the API binary:

```bash
make build-api      # Builds bin/helloworld-ai-api
```

## Development

### Using Tilt (Recommended)

Tilt provides the best development experience:

- Automatic rebuilds on file changes (Go)
- Unified log viewing for all services
- Dependency management (llama server starts first)
- Port forwarding handled automatically

```bash
make start    # Start all services with Tilt
make stop     # Stop all services
```

### Using Make

```bash
make lint          # Run Go linter
make test          # Run Go tests
make generate-mocks # Generate mock files for testing
make deps          # Install Go dependencies
```

### Testing

The project has comprehensive unit tests for all packages and uses [gomock](https://github.com/uber-go/mock) for generating mocks.

#### Generating Mocks

Mocks are automatically generated using `//go:generate` directives in source files:

```bash
make generate-mocks
# Or use go generate directly:
go generate ./...
```

Mocks are generated in `mocks/` subdirectories within each package (e.g., `internal/service/mocks/`).

#### Running Tests

Run all tests:

```bash
make test
# Or:
go test ./...
```

Run tests for a specific package:

```bash
go test ./internal/service -v
```

Run tests with coverage:

```bash
go test ./... -cover
```

#### Test Patterns

- **Mock Generation:** Interfaces have `//go:generate` directives for automatic mock generation
- **External Test Packages:** Some test files use `_test` packages (e.g., `service_test`) to avoid import cycles when using mocks
- **Test Isolation:** Each test uses temporary directories and properly cleans up resources
- **Log Suppression:** Test files suppress log output for cleaner test runs

#### Linting

Run the linter:

```bash
make lint
```

The project follows Go best practices:

- All error returns are properly handled
- No unused variables or imports
- Proper error wrapping with context

## Deployment

### Home Server Deployment

1. Build binary for your server architecture:

```bash
GOOS=linux GOARCH=amd64 make build-api
```

1. Copy binary to your server:

```bash
scp bin/helloworld-ai-api user@server:~/helloworld-ai/
```

1. Run on server:

```bash
VAULT_PERSONAL_PATH=/path/to/personal \
VAULT_WORK_PATH=/path/to/work \
QDRANT_VECTOR_SIZE=768 \
LLM_BASE_URL=http://localhost:8080 \
LLM_API_KEY=dummy-key \
LLM_MODEL=local-model \
API_PORT=9000 \
./helloworld-ai-api
```

The server will serve both the API and web UI on the same port.

## Project Structure

```text
helloworld-ai/
├── cmd/
│   └── api/          # API server binary (serves API and web UI)
├── internal/
│   ├── config/       # Configuration loading (.env support)
│   ├── handlers/     # HTTP handlers (ingress layer)
│   ├── service/      # Business logic (service layer)
│   ├── storage/      # Database operations (storage layer)
│   ├── vectorstore/  # Vector database operations (Qdrant)
│   ├── vault/        # Vault manager and file scanner
│   ├── indexer/      # Markdown chunking and indexing pipeline
│   ├── rag/          # RAG engine for question-answering
│   └── llm/          # LLM and embeddings clients (external service layer)
├── index.html        # Web UI (embedded in binary)
└── Makefile
```

## Architecture Layers

- **Configuration Layer** (`internal/config`) - Environment variable and `.env` file loading
- **Ingress Layer** (`internal/handlers`) - HTTP request/response handling
- **Service Layer** (`internal/service`) - Business logic and domain models
- **RAG Layer** (`internal/rag`) - RAG engine for question-answering over indexed notes
- **Storage Layer** (`internal/storage`) - Database operations and repositories (SQLite)
- **Vector Store Layer** (`internal/vectorstore`) - Vector database operations (Qdrant)
- **Vault Layer** (`internal/vault`) - Vault management and file scanning
- **Indexer Layer** (`internal/indexer`) - Markdown chunking and indexing pipeline
- **External Service Layer** (`internal/llm`) - llama.cpp API clients (chat and embeddings)

See `AGENTS.md` for detailed architecture guidelines and coding standards.
