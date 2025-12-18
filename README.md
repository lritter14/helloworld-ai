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

## Model Downloads

Before starting the llama.cpp servers, you need to download the required model files. The project uses two models:

- **Chat Model:** `bartowski_Qwen2.5-14B-Instruct-GGUF_Qwen2.5-14B-Instruct-Q4_K_M.gguf` (for chat completions)
- **Embeddings Model:** `ggml-org_embeddinggemma-300M-GGUF_embeddinggemma-300M-Q8_0.gguf` (for embeddings generation)

### Quick Start

The easiest way to download all required models is using the Makefile target:

```bash
make download-models
```

This will automatically download both models to `../llama.cpp/models/` and skip any models that already exist.

**Important:** After downloading, ensure your `.env` file uses model names that match the downloaded filenames (without the `.gguf` extension). See the [Configuration](#configuration) section for details.

### Download Chat Model

```bash
# Create models directory if it doesn't exist
mkdir -p ../llama.cpp/models

# Download the Qwen2.5-14B-Instruct model (Q4_K_M quantization, ~8.99 GB)
cd ../llama.cpp/models
wget https://huggingface.co/bartowski/Qwen2.5-14B-Instruct-GGUF/resolve/main/Qwen2.5-14B-Instruct-Q4_K_M.gguf \
  -O bartowski_Qwen2.5-14B-Instruct-GGUF_Qwen2.5-14B-Instruct-Q4_K_M.gguf
```

Or using curl:

```bash
mkdir -p ../llama.cpp/models
curl -L https://huggingface.co/bartowski/Qwen2.5-14B-Instruct-GGUF/resolve/main/Qwen2.5-14B-Instruct-Q4_K_M.gguf \
  -o ../llama.cpp/models/bartowski_Qwen2.5-14B-Instruct-GGUF_Qwen2.5-14B-Instruct-Q4_K_M.gguf
```

### Download Embeddings Model

```bash
# Create models directory if it doesn't exist
mkdir -p ../llama.cpp/models

# Download the EmbeddingGemma-300M model (Q8_0 quantization)
cd ../llama.cpp/models
wget https://huggingface.co/ggml-org/embeddinggemma-300M-GGUF/resolve/main/embeddinggemma-300M-Q8_0.gguf \
  -O ggml-org_embeddinggemma-300M-GGUF_embeddinggemma-300M-Q8_0.gguf
```

Or using curl:

```bash
mkdir -p ../llama.cpp/models
curl -L https://huggingface.co/ggml-org/embeddinggemma-300M-GGUF/resolve/main/embeddinggemma-300M-Q8_0.gguf \
  -o ../llama.cpp/models/ggml-org_embeddinggemma-300M-GGUF_embeddinggemma-300M-Q8_0.gguf
```

**Note:** Ensure you have sufficient disk space for both models. The chat model is approximately 8.99 GB, and the embeddings model is smaller. The models will be downloaded to `../llama.cpp/models/` relative to the project root.

### Model Naming Convention

When configuring your `.env` file, the model names must match the downloaded filenames **without the `.gguf` extension**. For example:

- If the file is `bartowski_Qwen2.5-14B-Instruct-GGUF_Qwen2.5-14B-Instruct-Q4_K_M.gguf`, use:
  ```bash
  LLM_MODEL=Qwen2.5-14B-Instruct-Q4_K_M
  ```

- If the file is `ggml-org_embeddinggemma-300M-GGUF_embeddinggemma-300M-Q8_0.gguf`, use:
  ```bash
  EMBEDDING_MODEL_NAME=ggml-org_embeddinggemma-300M-GGUF_embeddinggemma-300M-Q8_0
  ```

The llama.cpp server in router mode will automatically discover models in the `../llama.cpp/models/` directory and load them by name when requested.

## Configuration

### Environment File (.env)

The project automatically loads configuration from a `.env` file in the project root. The Go service reads `.env` files automatically, so you can run `go run ./cmd/api` directly without manually exporting environment variables. Create a `.env` file in the project root with the following variables:

```bash
# API Server Configuration
API_PORT=9000

# LLM Configuration (Chat Completions)
LLM_BASE_URL=http://127.0.0.1:8081
LLM_API_KEY=dummy-key
LLM_MODEL=Llama-3.1-8B-Instruct

# Embeddings Configuration
EMBEDDING_BASE_URL=http://127.0.0.1:8081
EMBEDDING_MODEL_NAME=granite-embedding-278m-multilingual

# Qdrant Configuration
# Note: Must match the output vector size of the embeddings model
# For granite-embedding-278m-multilingual, this is typically 1024 dimensions
QDRANT_VECTOR_SIZE=1024

# Vault Configuration
VAULT_PERSONAL_PATH=./vaults/personal
VAULT_WORK_PATH=./vaults/work
```

A `.env` file with default values is included in the repository. Modify it according to your local setup.

## Quick Start

### Option 1: Using Tilt (Recommended for Development)

**Important:** Before starting Tilt, you must start the llama.cpp server manually in a separate terminal session. See [Starting llama.cpp Server](#starting-llamacpp-server) below.

Tilt manages Qdrant, the API server, and Swagger UI. It reads configuration from the `.env` file:

```bash
# In terminal 1: Start llama.cpp server (required before Tilt)
make start-llama

# In terminal 2: Start Tilt
tilt up
# or
make start
```

This will start:

- Qdrant (vector database) on port 6333
- API server on port 9000 - serves both API and web UI
- Swagger UI on port 8083 - interactive API documentation
- Watch for file changes and auto-reload
- Provide a web UI at `http://localhost:10350` to view logs and status

Access the application at `http://localhost:9000`
Access API documentation at `http://localhost:8083/docs`

To stop all services:

```bash
tilt down
# or
make stop
```

**Note:** The llama.cpp server must be stopped separately (Ctrl+C in the terminal where it's running).

### Starting llama.cpp Server

The llama.cpp server must be started manually before running the API server (whether using Tilt or running directly). This is because llama.cpp requires direct hardware access and is typically built for the host architecture.

**Prerequisites:**

1. Build llama.cpp (if not already built):
   ```bash
   cd /path/to/llama.cpp
   make
   ```

2. Download models (see [Model Downloads](#model-downloads) section):
   ```bash
   make download-models
   ```

**Start llama.cpp Server in Router Mode:**

The project uses llama.cpp in router mode, which allows a single server to handle both chat and embeddings models. Start the server with:

```bash
# Set the path to your llama-server binary
LLAMA_SERVER=/opt/homebrew/bin/llama-server  # or /path/to/llama.cpp/build/bin/llama-server

# Set the path to your models directory (use absolute path)
MODELS_DIR=$(realpath ../llama.cpp/models)

# Start server in router mode
LLAMA_ARG_MODELS_ALLOW_EXTRA_ARGS=true $LLAMA_SERVER \
  --models-dir "$MODELS_DIR" \
  --port 8081 \
  --host 127.0.0.1 \
  --models-max 4 \
  --embeddings


LLAMA_ARG_MODELS_ALLOW_EXTRA_ARGS=true /Users/loganritter/projects/llama.cpp/build/bin/llama-server \
  --models-dir "/Users/loganritter/projects/llama.cpp/models" \
  --port 8081 \
  --host 127.0.0.1 \
  --models-max 4 \
  --embeddings
```




Or use the Makefile target:

```bash
make start-llama
```

**Important Configuration:**

- `--host 127.0.0.1` - For local development (use `0.0.0.0` for Docker container access)
- `--port 8081` - Port must match `LLM_BASE_URL` in your `.env` file
- `--models-dir` - **Use absolute path** to avoid relative path resolution issues when llama.cpp spawns subprocesses
- `--embeddings` - Enables embeddings endpoint
- `--models-max 4` - Maximum number of models to keep loaded in memory
- `LLAMA_ARG_MODELS_ALLOW_EXTRA_ARGS=true` - Allows model-specific parameters via `/models/load` endpoint

**Verify llama.cpp is Running:**

```bash
curl http://127.0.0.1:8081/models
```

You should see a JSON response listing available models.

### Option 2: Using Make (Without Tilt)

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

### Frontend Assets

- Source files live in `web/static` (`index.html`, `app.js`, `styles.css`). That directory is a symlink to `internal/assets/static`, so edits automatically update the embedded assets.
- `internal/assets` uses `go:embed` to bundle the static files directly into the API binary. No separate npm build is required.
- The router serves those assets at `/`, so rebuilding the Go binary is all that’s needed to ship frontend updates.

## Running

### API Server

```bash
make run-api
# Or with custom port:
API_PORT=9000 go run ./cmd/api
```

The API server serves:

- Web UI at `http://localhost:9000/`
- RAG API endpoint at `http://localhost:9000/api/v1/ask` (question-answering over indexed notes with intelligent folder selection + lexical reranking)
  - Supports `?debug=true` query parameter for detailed retrieval information (useful for evaluation frameworks)
- Index API endpoint at `http://localhost:9000/api/index` (trigger re-indexing)
- Swagger JSON spec at `http://localhost:9000/api/docs/swagger.json`

The web UI includes vault/folder filters plus an **Answer Detail** control (Brief / Normal / Detailed) that hints how much supporting context to retrieve for each answer.

When using Tilt, Swagger UI is available at `http://localhost:8082/docs` for interactive API documentation.

### Indexing

On startup, the API server automatically indexes all markdown files from both vaults:

- Scans all `.md` files in personal and work vaults
- Chunks files by heading hierarchy (min 50 runes, max 1000 runes per chunk)
- Generates stable, deterministic chunk IDs based on content (vault_id, rel_path, heading_path, chunk_text)
- Generates embeddings for each chunk with automatic batch size reduction on errors
- Skips chunks that exceed the embedding model's context size limit (512 tokens) with warnings
- Stores metadata in SQLite and vectors in Qdrant
- Uses hash-based change detection to skip unchanged files
- Validates embedding vector size at startup (fail-fast if mismatch)

Indexing runs synchronously at startup. Errors for individual files are logged but don't prevent the server from starting. The indexer automatically handles embedding batch size errors by splitting batches in half and retrying. Chunks that are too large for the embedding model (exceeding 512 tokens) are skipped with warnings rather than causing failures. Check logs for indexing progress and any errors.

### API Server Environment Variables

When running the API server directly (not via Tilt), you can set these environment variables:

**Required:**

- `VAULT_PERSONAL_PATH` - Path to personal vault directory
- `VAULT_WORK_PATH` - Path to work vault directory
- `QDRANT_VECTOR_SIZE` - Vector size for embeddings (must be > 0)

**Optional (with defaults):**

- `LLM_BASE_URL` - Base URL for llama.cpp chat server (default: `http://127.0.0.1:8081`)
- `LLM_API_KEY` - API key for llama.cpp (default: `dummy-key`)
- `LLM_MODEL` - Model name for chat completions (default: `Llama-3.1-8B-Instruct`)
- `EMBEDDING_BASE_URL` - Base URL for embeddings API (default: `http://127.0.0.1:8081`)
- `EMBEDDING_MODEL_NAME` - Model name for embeddings (default: `granite-embedding-278m-multilingual`)
- `DB_PATH` - Path to SQLite database (default: `./data/helloworld-ai.db`)
- `QDRANT_URL` - Qdrant server URL (default: `http://127.0.0.1:6333`)
- `QDRANT_COLLECTION` - Qdrant collection name (default: `notes`)
- `API_PORT` - Port for API server (default: `9000`)

**Note:** The embedding model (`granite-embedding-278m-multilingual`) has a hard context size limit of 512 tokens. Chunks exceeding this limit are automatically skipped during indexing. The `QDRANT_VECTOR_SIZE` must match the output vector size of your embeddings model (typically 1024 for granite-embedding-278m-multilingual).

**Note:** The Go service automatically loads `.env` files from the project root. Environment variables take precedence over `.env` file values if both are set. When using Tilt, the `.env` file is automatically loaded by the Go service.

## Building

Build both binaries:

```bash
make build
```

Build the API binary:

```bash
make build-api      # Builds bin/helloworld-ai-api (automatically generates Swagger spec)
```

The build process automatically generates the Swagger specification from code annotations before building the binary.

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
make lint            # Run Go linter
make test            # Run Go tests
make generate-mocks  # Generate mock files for testing
make generate-swagger # Generate Swagger/OpenAPI specification from code
make deps            # Install Go dependencies
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

### Automated CI/CD Deployment (Recommended)

The project includes automated deployment via GitHub Actions and Docker. See [DEPLOYMENT.md](DEPLOYMENT.md) for complete setup instructions.

**Quick Overview:**

- GitHub Actions builds and tests on every push
- Docker image is pushed to GitHub Container Registry (GHCR)
- Deployment triggered via TailNet SSH to your home PC
- Graceful rolling updates with health checks

**Prerequisites:**

- Docker and Docker Compose on home PC
- TailNet/Tailscale configured
- GitHub repository secrets configured

### Manual Home Server Deployment

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
QDRANT_VECTOR_SIZE=1024 \
LLM_BASE_URL=http://127.0.0.1:8081 \
LLM_API_KEY=dummy-key \
LLM_MODEL=Llama-3.1-8B-Instruct \
EMBEDDING_BASE_URL=http://127.0.0.1:8081 \
EMBEDDING_MODEL_NAME=granite-embedding-278m-multilingual \
API_PORT=9000 \
./helloworld-ai-api
```

The server will serve both the API and web UI on the same port.

### Docker Deployment

The project includes `Dockerfile` and `docker-compose.yml` for containerized deployment. See [DEPLOYMENT.md](DEPLOYMENT.md) for details.

#### Starting llama.cpp Server for Docker Compose

The `docker-compose.yml` file runs the API and Qdrant containers, but **llama.cpp must be started separately** on the host machine (not in Docker). This is because llama.cpp requires direct hardware access and is typically built for the host architecture.

**Prerequisites:**

1. Build llama.cpp (if not already built):
   ```bash
   cd /path/to/llama.cpp
   make
   ```

2. Download models (see [Model Downloads](#model-downloads) section):
   ```bash
   make download-models
   ```

**Start llama.cpp Server in Router Mode:**

The project uses llama.cpp in router mode, which allows a single server to handle both chat and embeddings models. Start the server with:

```bash
# Set the path to your llama-server binary
LLAMA_SERVER=/path/to/llama.cpp/build/bin/llama-server

# Set the path to your models directory
MODELS_DIR=/path/to/llama.cpp/models

# Start server in router mode (accessible from Docker containers)
LLAMA_ARG_MODELS_ALLOW_EXTRA_ARGS=true $LLAMA_SERVER \
  --models-dir $MODELS_DIR \
  --port 8081 \
  --host 0.0.0.0 \
  --models-max 4 \
  --embeddings
```

**Important Configuration:**

- `--host 0.0.0.0` - **Required** for Docker containers to access the server (not `localhost` or `127.0.0.1`)
- `--port 8081` - Port must match `LLM_BASE_URL` in your `.env` file
- `--models-dir` - Points to directory containing your downloaded `.gguf` model files
- `--embeddings` - Enables embeddings endpoint
- `--models-max 4` - Maximum number of models to keep loaded in memory
- `LLAMA_ARG_MODELS_ALLOW_EXTRA_ARGS=true` - Allows model-specific parameters via `/models/load` endpoint

**Docker Container Access:**

The API container in `docker-compose.yml` connects to llama.cpp using:

- **Linux**: Use `host.docker.internal:8081` in your `.env` file (Docker automatically resolves this)
- **Alternative (Linux)**: Use the host's IP address or `172.17.0.1` (default Docker bridge gateway)

Example `.env` configuration:

```bash
LLM_BASE_URL=http://host.docker.internal:8081
EMBEDDING_BASE_URL=http://host.docker.internal:8081
```

**Verify llama.cpp is Running:**

```bash
# Check if server is responding
curl http://127.0.0.1:8081/models

# Check from inside a Docker container
docker exec helloworld-ai-api curl http://host.docker.internal:8081/models
```

**Running as a Service (Optional):**

For production, you may want to run llama.cpp as a systemd service. Create `/etc/systemd/system/llama-server.service`:

```ini
[Unit]
Description=llama.cpp Server for helloworld-ai
After=network.target

[Service]
Type=simple
User=your-user
WorkingDirectory=/path/to/llama.cpp
Environment="LLAMA_ARG_MODELS_ALLOW_EXTRA_ARGS=true"
ExecStart=/path/to/llama.cpp/build/bin/llama-server \
  --models-dir /path/to/llama.cpp/models \
  --port 8081 \
  --host 0.0.0.0 \
  --models-max 4 \
  --embeddings
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
```

Then enable and start:

```bash
sudo systemctl enable llama-server
sudo systemctl start llama-server
sudo systemctl status llama-server
```

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

## API Documentation

The API is fully documented using Swagger/OpenAPI 2.0 annotations. The specification is generated from code annotations and served by the API server.

### Accessing API Documentation

**Swagger UI (Interactive):**

- When using Tilt: `http://localhost:8082/docs`
- Or run manually: `swagger serve cmd/api/swagger.json`

**Swagger JSON (Raw):**

- `http://localhost:9000/api/docs/swagger.json`

### Generating Swagger Spec

The Swagger specification is automatically generated during the build process. You can also generate it manually:

```bash
make generate-swagger
```

This requires the `swagger` CLI tool. Install it with:

```bash
go install github.com/go-swagger/go-swagger/cmd/swagger@latest
```

### Adding API Documentation

All API endpoints are documented using go-swagger annotations. See `internal/handlers/AGENTS.md` for documentation patterns.

## Architecture Layers

- **Configuration Layer** (`internal/config`) - Environment variable and `.env` file loading
- **Ingress Layer** (`internal/handlers`) - HTTP request/response handling with Swagger documentation
- **HTTP Infrastructure Layer** (`internal/http`) - Router, middleware, and Swagger JSON serving
- **Service Layer** (`internal/service`) - Business logic and domain models
- **RAG Layer** (`internal/rag`) - RAG engine for question-answering over indexed notes (vector search + lexical rerank for higher precision)
- **Storage Layer** (`internal/storage`) - Database operations and repositories (SQLite)
- **Vector Store Layer** (`internal/vectorstore`) - Vector database operations (Qdrant)
- **Vault Layer** (`internal/vault`) - Vault management and file scanning
- **Indexer Layer** (`internal/indexer`) - Markdown chunking and indexing pipeline
- **External Service Layer** (`internal/llm`) - llama.cpp API clients (chat and embeddings)

See `AGENTS.md` for detailed architecture guidelines and coding standards.

## Evaluation Framework

The project includes a comprehensive Python-based evaluation framework for measuring chatbot effectiveness over time. The framework tracks core metrics (Retrieval Recall@K, MRR, Scope Miss Rate, Groundedness, Correctness, Abstention) and enables data-driven decisions about model changes, hardware upgrades, and embedding strategy improvements.

### Evaluation Overview

The evaluation framework is located in the `eval/` directory and includes:

- **Test Suite**: JSONL format test cases (`eval_set.jsonl`) with anchor-based gold supports
- **Core Metrics**: Defined in `eval/EVAL.md` - retrieval metrics, answer quality metrics, and abstention metrics
- **Labeling Workflow**: Interactive script (`eval/scripts/label_eval.py`) for marking gold supports
- **Results Storage**: Structured storage format for evaluation runs with latency/cost tracking

### Evaluation Quick Start

**Label Test Cases** (create ground truth):

```bash
python eval/scripts/label_eval.py --eval-set eval/eval_set.jsonl --api-url http://localhost:9000
```

**Run Evaluation** (when `run_eval.py` is implemented):

```bash
python eval/scripts/run_eval.py --eval-set eval/eval_set.jsonl --k 5
```

### Evaluation Documentation

- **Core Metrics**: See `eval/EVAL.md` for detailed metric definitions
- **Scripts Documentation**: See `eval/scripts/README.md` for script usage
- **Setup Guide**: See `eval/scripts/SETUP.md` for environment setup
- **Patterns and Guidelines**: See `eval/AGENTS.md` for evaluation framework patterns

### Current Status

**Completed:**

- ✅ Stable chunk IDs (32-char hash) in Go API
- ✅ Debug mode (`debug=true` query parameter) in `/api/v1/ask` endpoint
- ✅ Core metrics documentation (`eval/EVAL.md`)
- ✅ Initial eval set (`eval/eval_set.jsonl` with 50 test cases)
- ✅ Results storage module (`eval/scripts/storage.py`)
- ✅ Labeling workflow (`eval/scripts/label_eval.py`)

**In Progress:**

- 🔄 Evaluation runner (`run_eval.py`)
- 🔄 Retrieval metrics calculator (`score_retrieval.py`)
- 🔄 Answer quality judges (`judge_answers.py`)
- 🔄 Abstention metrics calculator (`score_abstention.py`)
- 🔄 Run comparison tool (`compare_runs.py`)

See `.cursor/plans/chatbot_evaluation_framework_a914ccc3.plan.md` for the complete implementation plan.
