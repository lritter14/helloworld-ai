# pyright: reportUndefinedVariable=false, reportUnboundVariable=false
# Tiltfile for helloworld-ai
# Manages: llama.cpp server and API server

# ============================================================================
# Configuration Variables (from .env file)
# ============================================================================

# llama.cpp Server Configuration
llama_server_path = "../llama.cpp/build/bin/llama-server"

# Chat Server Configuration
llama_chat_model_path = "../llama.cpp/models/bartowski_Meta-Llama-3.1-8B-Instruct-GGUF_Meta-Llama-3.1-8B-Instruct-Q4_K_M.gguf"
llama_chat_port = 8080

# Embeddings Server Configuration
llama_embeddings_model_path = "../llama.cpp/models/bartowski_granite-embedding-278m-multilingual-GGUF_granite-embedding-278m-multilingual-Q5_K_L.gguf"
llama_embeddings_port = 8081

# API Server Configuration
api_port = 9000

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
            %s -m %s --port %d
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
            # Note: --ctx-size 2048 is set but granite-embedding-278m-multilingual enforces n_ctx=512 tokens (hard limit).
            # The model will reject inputs exceeding 512 tokens regardless of this flag.
            %s -m %s --port %d --embedding --pooling mean --ubatch-size 2048 --ctx-size 2048
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
