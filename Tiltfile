# Tiltfile for helloworld-ai
# Manages: llama.cpp server and API server

# Configuration
llama_server_path = "../llama.cpp/build/bin/llama-server"
llama_model_path = "../llama.cpp/models/llama-3-8b-instruct-q4_k_m.gguf"
llama_port = 8080
api_port = 9000

# Environment variables
llm_base_url = "http://localhost:%d" % llama_port
llm_api_key = "dummy-key"
llm_model = "local-model"
qdrant_vector_size = "4096"  # llama-3-8b-instruct embedding size
vault_personal_path = "./vaults/personal"
vault_work_path = "./vaults/work"

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
# llama.cpp Server (Chat + Embeddings) (Infrastructure Dependency)
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
        
        if [ ! -f "%s" ]; then
            echo "Warning: Model file not found at %s"
            echo "Starting server with Hugging Face model download..."
            %s -hf ggml-org/llama-3-8b-instruct-GGUF --port %d --embedding --pooling mean
        else
            echo "Starting llama.cpp server on port %d with model %s (chat + embeddings)"
            %s -m %s --port %d --embedding --pooling mean
        fi
        """ % (
            llama_server_path, llama_server_path,
            llama_model_path, llama_model_path,
            llama_server_path, llama_port,
            llama_port, llama_model_path,
            llama_server_path, llama_model_path, llama_port
        )
    ],
    resource_deps=[],
    labels=["infra"],
    auto_init=True,
    ignore=["**"],
    readiness_probe=probe(
        exec=exec_action(["curl", "-f", "http://localhost:%d/props" % llama_port]),
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
        export LLM_BASE_URL="%s"
        export LLM_API_KEY="%s"
        export LLM_MODEL="%s"
        export EMBEDDING_BASE_URL="%s"
        export EMBEDDING_MODEL_NAME="%s"
        export VAULT_PERSONAL_PATH="%s"
        export VAULT_WORK_PATH="%s"
        export QDRANT_URL="http://localhost:6333"
        export QDRANT_COLLECTION="notes"
        export QDRANT_VECTOR_SIZE="%s"
        export API_PORT="%s"
        export DB_PATH="./data/helloworld-ai.db"
        exec go run ./cmd/api
        """ % (
            llm_base_url,
            llm_api_key,
            llm_model,
            llm_base_url,
            llm_model,
            vault_personal_path,
            vault_work_path,
            qdrant_vector_size,
            str(api_port),
        )
    ],
    deps=[
        "./cmd/api",
        "./internal",
        "./go.mod",
        "./go.sum",
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

