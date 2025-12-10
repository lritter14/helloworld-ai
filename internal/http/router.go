package http

import (
	"io/fs"
	"net/http"
	"os"
	"path/filepath"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"helloworld-ai/internal/assets"
	"helloworld-ai/internal/handlers"
	"helloworld-ai/internal/indexer"
	"helloworld-ai/internal/rag"
	"helloworld-ai/internal/storage"
)

// Deps holds dependencies for the HTTP router.
type Deps struct {
	RAGEngine       rag.Engine
	VaultRepo       storage.VaultStore
	IndexerPipeline *indexer.Pipeline
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
		// Serve Swagger spec at /api/docs/swagger.json
		r.Route("/docs", func(r chi.Router) {
			r.Get("/swagger.json", func(w http.ResponseWriter, req *http.Request) {
				// Get the swagger.json file path relative to cmd/api
				swaggerPath := filepath.Join("cmd", "api", "swagger.json")
				data, err := os.ReadFile(swaggerPath)
				if err != nil {
					http.Error(w, "Swagger spec not found", http.StatusNotFound)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write(data)
			})
		})
	})

	// Serve embedded static assets (index.html, JS, CSS) at /
	staticFS, err := fs.Sub(assets.StaticFS, "static")
	if err != nil {
		panic("static assets missing: " + err.Error())
	}
	fileServer := http.FileServer(http.FS(staticFS))
	r.Handle("/*", http.StripPrefix("/", fileServer))

	return r
}
