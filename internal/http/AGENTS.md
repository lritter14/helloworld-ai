# HTTP Infrastructure - Agent Guide

Router and middleware patterns.

## Router Setup

```go
type Deps struct {
    RAGEngine      rag.Engine
    VaultRepo      storage.VaultStore
    IndexerPipeline *indexer.Pipeline
    IndexHTML      string
}

func NewRouter(deps *Deps) http.Handler {
    r := chi.NewRouter()
    
    r.Use(middleware.Recoverer)
    r.Use(RequestLogger)
    r.Use(LoggerMiddleware)
    r.Use(CORS)
    
    // Register routes
    r.Route("/api", func(r chi.Router) {
        r.Method(http.MethodPost, "/index", indexHandler)
        r.Route("/v1", func(r chi.Router) {
            r.Method(http.MethodPost, "/ask", askHandler)
        })
        // Serve Swagger spec at /api/docs/swagger.json
        r.Route("/docs", func(r chi.Router) {
            r.Get("/swagger.json", func(w http.ResponseWriter, req *http.Request) {
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
    
    // Serve HTML page at root
    r.Get("/", func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "text/html; charset=utf-8")
        w.WriteHeader(http.StatusOK)
        _, _ = w.Write([]byte(deps.IndexHTML))
    })
    
    return r
}
```

## Middleware Order

1. Recovery (panic handling)
2. Request Logger (HTTP logging)
3. Logger Middleware (context enrichment)
4. CORS (cross-origin headers)

## Logger Middleware

Adds logger to context:

```go
func LoggerMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        logger := slog.Default().With("method", r.Method, "path", r.URL.Path)
        ctx := context.WithValue(r.Context(), loggerKey, logger)
        next.ServeHTTP(w, r.WithContext(ctx))
    })
}
```

## Request Logger

Logs HTTP requests, skips health checks:

```go
func RequestLogger(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        start := time.Now()
        ww := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
        next.ServeHTTP(ww, r)
        
        if r.Method == http.MethodGet && r.URL.Path == "/" && ww.statusCode == http.StatusOK {
            return // Skip health checks
        }
        
        slog.Info("HTTP request", "method", r.Method, "path", r.URL.Path, 
            "status", ww.statusCode, "duration", time.Since(start))
    })
}
```

## CORS Middleware

```go
func CORS(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        origin := r.Header.Get("Origin")
        if origin != "" {
            w.Header().Set("Access-Control-Allow-Origin", origin)
        } else {
            w.Header().Set("Access-Control-Allow-Origin", "*")
        }
        
        if r.Method == http.MethodOptions {
            w.WriteHeader(http.StatusNoContent)
            return
        }
        next.ServeHTTP(w, r)
    })
}
```

## Testing

### Test Patterns

**Mock Usage:**

```go
deps := &Deps{
    RAGEngine:   mockRAGEngine,
    VaultRepo:   mockVaultRepo,
    IndexerPipeline: mockIndexerPipeline,
    IndexHTML:   "<html></html>",
}

router := NewRouter(deps)
```

**HTTP Testing:**

```go
req := httptest.NewRequest(http.MethodGet, "/", nil)
w := httptest.NewRecorder()

router.ServeHTTP(w, req)

// Check middleware applied
if w.Header().Get("Access-Control-Allow-Origin") == "" {
    t.Error("Router should apply CORS middleware")
}
```

**Error Handling:**

Properly handle error returns:

```go
_, _ = w.Write([]byte(deps.IndexHTML)) // Ignore error in handler
```

## Swagger JSON Serving

The router serves the Swagger specification at `/api/docs/swagger.json`:

- Reads `cmd/api/swagger.json` from disk (not embedded)
- Returns 404 if file not found
- Sets proper `Content-Type: application/json` header
- The spec is generated automatically during build via `make generate-swagger`

When using Tilt, Swagger UI is available at `http://localhost:8082/docs` and loads the spec from the API endpoint.

## Rules

- Add middleware in logical order
- Use typed context keys for logger
- Wrap ResponseWriter to capture status codes
- Handle preflight requests in CORS
- Handle all error returns (use `_` for intentional ignores)
- Serve Swagger JSON at `/api/docs/swagger.json` (read from disk, not embedded)
