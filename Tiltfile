# pyright: reportUndefinedVariable=false, reportUnboundVariable=false
# Tiltfile for helloworld-ai
# Manages: Qdrant, API server, and Swagger UI
#
# NOTE: llama.cpp server must be started manually in a separate terminal before running Tilt.
# See README.md for instructions on starting the llama.cpp server.

# ============================================================================
# Configuration Variables
# ============================================================================

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
        exec=exec_action(["curl", "-f", "http://127.0.0.1:6333/"]),
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
        # Only set QDRANT_URL here since it's Tilt-specific (127.0.0.1)
        export QDRANT_URL="http://127.0.0.1:6333"
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
    resource_deps=["qdrant"],
    labels=["api"],
    ignore=[
        "**/bin/**",
        "**/web/**",
    ],
    readiness_probe=probe(
        exec=exec_action(["curl", "-f", "http://127.0.0.1:%d/" % api_port]),
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
        echo "Swagger UI will be available at: http://127.0.0.1:%d"
        echo "API docs JSON: http://127.0.0.1:%d/api/docs/swagger.json"
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
        exec=exec_action(["curl", "-f", "http://127.0.0.1:%d/docs" % swagger_port]),
        initial_delay_secs=5,
        timeout_secs=2,
        period_secs=3,
    ),
)
