# pyright: reportUndefinedVariable=false, reportUnboundVariable=false
# Tiltfile for helloworld-ai
# Manages: llama.cpp server and API server

# ============================================================================
# Configuration Variables (from .env file)
# ============================================================================

# llama.cpp Server Configuration
llama_server_path = "../llama.cpp/build/bin/llama-server"

# Chat Server Configuration
llama_chat_model_path = "../llama.cpp/models/bartowski_Qwen2.5-14B-Instruct-GGUF_Qwen2.5-14B-Instruct-Q4_K_M.gguf"
llama_chat_port = 8080

# Embeddings Server Configuration
llama_embeddings_model_path = "../llama.cpp/models/ggml-org_embeddinggemma-300M-GGUF_embeddinggemma-300M-Q8_0.gguf"
# llama_embeddings_model_path = "../llama.cpp/models/bartowski_granite-embedding-278m-multilingual-GGUF_granite-embedding-278m-multilingual-Q5_K_L.gguf"
llama_embeddings_port = 8081

# API Server Configuration
api_port = 9000

# Swagger UI Configuration
swagger_port = 8082

# ============================================================================
# Qdrant Vector Database (Infrastructure Dependency)
# ============================================================================
local_resource(
    name="qdrant",
    serve_cmd=[
        "bash", "-c",
        """
        # Remove existing container if it exists
        docker rm -f qdrant 2>/dev/null || true
        
        # Start Qdrant container (runs in foreground for Tilt)
        echo "Starting Qdrant vector database on port 6333..."
        docker run --name qdrant -p 6333:6333 -p 6334:6334 -v qdrant_storage:/qdrant/storage qdrant/qdrant
        """,
    ],
    resource_deps=[],
    labels=["infra"],
    auto_init=True,
    ignore=["**"],
    readiness_probe=probe(
        exec=exec_action(["curl", "-f", "http://localhost:6333/"]),
        initial_delay_secs=5,
        timeout_secs=2,
        period_secs=3,
    ),
)

# ============================================================================
# llama.cpp Chat Server (Infrastructure Dependency)
# ============================================================================
local_resource(
    name="llama-server-chat",
    serve_cmd=[
        "bash", "-c",
        """
        if [ ! -f "%s" ]; then
            echo "Error: llama-server not found at %s"
            echo "Please build llama.cpp first: cd ../llama.cpp && make"
            exit 1
        fi
        
        if [ ! -f "%s" ]; then
            echo "Warning: Chat model file not found at %s"
            echo "Please ensure the model file exists"
            exit 1
        else
            echo "Starting llama.cpp chat server on port %d with model %s"
            %s -m %s \
              --port %d \
              --host 127.0.0.1 \
              --ctx-size 8192 \
              --threads 8 \
              --batch-size 384 \
              --ubatch-size 96
        fi
        """ % (
            llama_server_path, llama_server_path,
            llama_chat_model_path, llama_chat_model_path,
            llama_chat_port, llama_chat_model_path,
            llama_server_path, llama_chat_model_path, llama_chat_port
        )
    ],
    resource_deps=[],
    labels=["infra"],
    auto_init=True,
    ignore=["**"],
    readiness_probe=probe(
        exec=exec_action(["curl", "-f", "http://localhost:%d/props" % llama_chat_port]),
        initial_delay_secs=10,
        timeout_secs=2,
        period_secs=3,
    ),
)
# ============================================================================
# llama.cpp Embeddings Server (Infrastructure Dependency)
# ============================================================================
local_resource(
    name="llama-server-embeddings",
    serve_cmd=[
        "bash", "-c",
        """
        if [ ! -f "%s" ]; then
            echo "Error: llama-server not found at %s"
            echo "Please build llama.cpp first: cd ../llama.cpp && make"
            exit 1
        fi
        
        if [ ! -f "%s" ]; then
            echo "Warning: Embeddings model file not found at %s"
            echo "Please ensure the model file exists"
            exit 1
        else
            echo "Starting llama.cpp embeddings server on port %d with model %s"
            # EmbeddingGemma-300M supports a 2048-token context window, so --ctx-size 2048 is fine.
            %s -m %s \
              --port %d \
              --host 127.0.0.1 \
              --embedding \
              --pooling mean \
              --ubatch-size 2048 \
              --ctx-size 2048
        fi
        """ % (
            llama_server_path, llama_server_path,
            llama_embeddings_model_path, llama_embeddings_model_path,
            llama_embeddings_port, llama_embeddings_model_path,
            llama_server_path, llama_embeddings_model_path, llama_embeddings_port
        )
    ],
    resource_deps=[],
    labels=["infra"],
    auto_init=True,
    ignore=["**"],
    readiness_probe=probe(
        exec=exec_action(["curl", "-f", "http://localhost:%d/props" % llama_embeddings_port]),
        initial_delay_secs=10,
        timeout_secs=2,
        period_secs=3,
    ),
)

# ============================================================================
# API Server
# ============================================================================
local_resource(
    name="api",
    serve_cmd=[
        "bash", "-c",
        """
        # Go service automatically loads .env file via config package
        # Only set QDRANT_URL here since it's Tilt-specific (localhost)
        export QDRANT_URL="http://localhost:6333"
        exec go run ./cmd/api
        """,
    ],
    deps=[
        "./cmd/api",
        "./internal",
        "./go.mod",
        "./go.sum",
        "./cmd/api/swagger.json",
    ],
    resource_deps=["qdrant", "llama-server-chat", "llama-server-embeddings"],
    labels=["api"],
    ignore=[
        "**/bin/**",
        "**/web/**",
    ],
    readiness_probe=probe(
        exec=exec_action(["curl", "-f", "http://localhost:%d/" % api_port]),
        initial_delay_secs=5,
        timeout_secs=2,
        period_secs=3,
    ),
)

# ============================================================================
# Swagger UI (API Documentation)
# ============================================================================
local_resource(
    name="swagger-ui",
    serve_cmd=[
        "bash", "-c",
        """
        if ! command -v swagger > /dev/null; then
            echo "Error: swagger CLI not found. Install it with:"
            echo "  go install github.com/go-swagger/go-swagger/cmd/swagger@latest"
            exit 1
        fi
        echo "Starting Swagger UI on port %d..."
        echo "Swagger UI will be available at: http://localhost:%d"
        echo "API docs JSON: http://localhost:%d/api/docs/swagger.json"
        # Serve Swagger UI with the spec file
        # Note: Swagger UI will use the spec's host/scheme, or you can change it in the UI
        swagger serve -p %d -F swagger --no-open cmd/api/swagger.json
        """ % (swagger_port, swagger_port, api_port, swagger_port),
    ],
    deps=[
        "./cmd/api/swagger.json",
    ],
    resource_deps=["api"],
    labels=["api", "docs"],
    ignore=["**"],
    readiness_probe=probe(
        exec=exec_action(["curl", "-f", "http://localhost:%d/docs" % swagger_port]),
        initial_delay_secs=5,
        timeout_secs=2,
        period_secs=3,
    ),
)
