# HTTP Infrastructure - Agent Guide

Router and middleware patterns.

## Router Setup

```go
func NewRouter(deps *Deps) http.Handler {
    r := chi.NewRouter()
    
    r.Use(middleware.Recoverer)
    r.Use(RequestLogger)
    r.Use(LoggerMiddleware)
    r.Use(CORS)
    
    // Register routes
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

## Rules

- Add middleware in logical order
- Use typed context keys for logger
- Wrap ResponseWriter to capture status codes
- Handle preflight requests in CORS
