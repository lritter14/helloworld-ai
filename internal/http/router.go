package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"helloworld-ai/internal/handlers"
	"helloworld-ai/internal/indexer"
	"helloworld-ai/internal/rag"
	"helloworld-ai/internal/storage"
)

// Deps holds dependencies for the HTTP router.
type Deps struct {
	RAGEngine      rag.Engine
	VaultRepo      storage.VaultStore
	IndexerPipeline *indexer.Pipeline
	IndexHTML      string // Embedded HTML content
}

// NewRouter creates a new HTTP router with the provided dependencies.
func NewRouter(deps *Deps) http.Handler {
	r := chi.NewRouter()

	// Add chi middleware
	r.Use(middleware.Recoverer)

	// Add custom request logger (skips health checks)
	r.Use(RequestLogger)

	// Add structured logging middleware
	r.Use(LoggerMiddleware)

	// Add CORS middleware
	r.Use(CORS)

	// Create handlers
	askHandler := handlers.NewAskHandler(deps.RAGEngine, deps.VaultRepo)
	indexHandler := handlers.NewIndexHandler(deps.IndexerPipeline)

	// Register API routes
	r.Route("/api", func(r chi.Router) {
		r.Method(http.MethodPost, "/index", indexHandler) // Re-index endpoint
		r.Route("/v1", func(r chi.Router) {
			r.Method(http.MethodPost, "/ask", askHandler)
		})
	})

	// Serve HTML page at root
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(deps.IndexHTML))
	})

	return r
}
