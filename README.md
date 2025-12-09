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

The project uses a `.env` file for configuration when using Tilt. Create a `.env` file in the project root with the following variables:

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
- API endpoint at `http://localhost:9000/api/chat`

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

**Note:** When using Tilt, configuration is read from the `.env` file instead of environment variables.

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
make lint     # Run Go linter
make test     # Run Go tests
make deps     # Install Go dependencies
```

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
│   ├── handlers/     # HTTP handlers (ingress layer)
│   ├── service/      # Business logic (service layer)
│   ├── storage/      # Database operations (storage layer)
│   ├── vectorstore/  # Vector database operations (Qdrant)
│   └── llm/          # LLM and embeddings clients (external service layer)
├── index.html        # Web UI (embedded in binary)
└── Makefile
```

## Architecture Layers

- **Ingress Layer** (`internal/handlers`) - HTTP request/response handling
- **Service Layer** (`internal/service`) - Business logic and domain models
- **Storage Layer** (`internal/storage`) - Database operations and repositories (SQLite)
- **Vector Store Layer** (`internal/vectorstore`) - Vector database operations (Qdrant)
- **External Service Layer** (`internal/llm`) - llama.cpp API clients (chat and embeddings)

See `AGENTS.md` for detailed architecture guidelines and coding standards.
