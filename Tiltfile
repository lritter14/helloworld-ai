# pyright: reportUndefinedVariable=false, reportUnboundVariable=false
# Tiltfile for helloworld-ai
# Manages: llama.cpp server and API server

# ============================================================================
# Configuration Variables (from .env file)
# ============================================================================

# llama.cpp Server Configuration
llama_server_path = "/opt/homebrew/bin/llama-server"

# Models Directory (router mode auto-discovers models from here)
llama_models_dir = "../llama.cpp/models"

# Single Server Configuration (router mode - handles both chat and embeddings)
llama_server_port = 8081

# API Server Configuration
api_port = 9000

# Swagger UI Configuration
swagger_port = 8083

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
# llama.cpp Server (Router Mode - Single Server for Chat and Embeddings)
# ============================================================================
local_resource(
    name="llama-server",
    serve_cmd=[
        "bash", "-c",
        """
        if [ ! -f "%s" ]; then
            echo "Error: llama-server not found at %s"
            echo "Please build llama.cpp first: cd ../llama.cpp && make"
            exit 1
        fi
        
        if [ ! -d "%s" ]; then
            echo "Warning: Models directory not found at %s"
            echo "Please ensure the models directory exists"
            exit 1
        else
            echo "Starting llama.cpp server in router mode on port %d"
            echo "Auto-discovering models from: %s"
            echo "Models will be loaded on-demand when requested via /models/load endpoint"
            echo "Use scripts/load-models.sh to load models with their specific parameters"
            # Router mode: minimal configuration, model-specific params set via /models/load
            LLAMA_ARG_MODELS_ALLOW_EXTRA_ARGS=true %s \
              --models-dir %s \
              --port %d \
              --host localhost \
              --models-max 4 \
              --embeddings
        fi
        """ % (
            llama_server_path, llama_server_path,
            llama_models_dir, llama_models_dir,
            llama_server_port, llama_models_dir,
            llama_server_path, llama_models_dir, llama_server_port
        )
    ],
    resource_deps=[],
    labels=["infra"],
    auto_init=True,
    ignore=["**"],
    readiness_probe=probe(
        exec=exec_action(["curl", "-f", "http://localhost:%d/models" % llama_server_port]),
        initial_delay_secs=5,
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
    resource_deps=["qdrant", "llama-server"],
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
# Model Loader (Optional - models are also loaded during API startup)
# ============================================================================
# Uncomment this resource if you prefer to load models via Tilt instead of API startup
# local_resource(
#     name="load-models",
#     serve_cmd=[
#         "bash", "-c",
#         """
#         echo "Waiting for llama server to be ready..."
#         sleep 5
#         echo "Loading models into llama.cpp server..."
#         ./scripts/load-models.sh http://localhost:%d
#         echo "Models loaded. This job will exit."
#         """ % llama_server_port,
#     ],
#     resource_deps=["llama-server"],
#     labels=["infra"],
#     auto_init=False,  # Set to True to auto-load models on Tilt start
#     ignore=["**"],
# )

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
