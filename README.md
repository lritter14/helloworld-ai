# HelloWorld AI

A Go-based API server with embedded web UI for interacting with local LLMs via llama.cpp.

## Architecture

The application consists of a single binary:

- **API Server** (`cmd/api`) - Provides the `/api/chat` endpoint and serves the web UI at `/`
- The web UI is a simple HTML page embedded in the Go binary

## Prerequisites

- Go 1.25.3 or later
- llama.cpp server running (see `start-llama` target)
- (Optional) Tilt for unified development workflow

## Quick Start

### Option 1: Using Tilt (Recommended for Development)

Tilt manages all services and dependencies automatically:

```bash
tilt up
```

This will:
- Start llama.cpp server (port 8080)
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

#### 2. Run API server

```bash
make run-api
```

This starts:
- API server on `http://localhost:9000` (serves both API and web UI)

#### 3. Access the UI

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

## Configuration

### API Server Environment Variables

- `LLM_BASE_URL` - Base URL for llama.cpp server (default: `http://localhost:8080`)
- `LLM_API_KEY` - API key for llama.cpp (default: `dummy-key`)
- `LLM_MODEL` - Model name to use (default: `local-model`)
- `API_PORT` - Port for API server (default: `9000`)

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

2. Copy binary to your server:

```bash
scp bin/helloworld-ai-api user@server:~/helloworld-ai/
```

3. Run on server:

```bash
LLM_BASE_URL=http://localhost:8080 \
LLM_API_KEY=dummy-key \
LLM_MODEL=local-model \
API_PORT=9000 \
./helloworld-ai-api
```

The server will serve both the API and web UI on the same port.

## Project Structure

```
helloworld-ai/
├── cmd/
│   └── api/          # API server binary (serves API and web UI)
├── internal/
│   ├── handlers/     # HTTP handlers (ingress layer)
│   ├── service/      # Business logic (service layer)
│   └── llm/          # LLM client (external service layer)
├── index.html        # Web UI (embedded in binary)
└── Makefile
```

## Architecture Layers

- **Ingress Layer** (`internal/handlers`) - HTTP request/response handling
- **Service Layer** (`internal/service`) - Business logic and domain models
- **External Service Layer** (`internal/llm`) - llama.cpp API client

See `.cursor/commands/go-standards.md` for detailed architecture guidelines.


