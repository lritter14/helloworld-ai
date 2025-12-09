package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"helloworld-ai/internal/handlers"
	"helloworld-ai/internal/service"
)

// Deps holds dependencies for the HTTP router.
type Deps struct {
	ChatService service.ChatService
	RAGEngine   interface{} // Will be typed properly in Phase 7
	IndexHTML   string      // Embedded HTML content
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

	// Create chat handler
	chatHandler := handlers.NewChatHandler(deps.ChatService)

	// Register API routes
	r.Route("/api", func(r chi.Router) {
		r.Method(http.MethodPost, "/chat", chatHandler)
	})

	// Serve HTML page at root
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(deps.IndexHTML))
	})

	return r
}
