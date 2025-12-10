# internal/assets Agent Guide

This package owns every embedded frontend asset that ships with the API binary. It is intentionally tiny so the rest of the stack can treat the UI as a static filesystem served through Chi.

## Responsibilities

- **go:embed ownership:** `assets.go` declares `//go:embed static/*` and exposes `StaticFS embed.FS`. Nothing else should reference files on disk at runtime.
- **Source of truth for the UI:** The `static/` directory contains `index.html`, `app.js`, and `styles.css`. These files mirror the structure under `web/static/` (which is a convenience symlink for editors).
- **Ingress integration:** `internal/http/router.go` calls `fs.Sub(assets.StaticFS, "static")` and mounts `http.FileServer(http.FS(...))` at `/*`. Agents modifying the router must not break this handshake.

## Editing Workflow

1. **Edit via the symlink:** Open files under `web/static/` in your editor. That directory points to `internal/assets/static/`, so changes propagate automatically while keeping a nice path for frontend tooling.
2. **Keep assets self-contained:** No build step runs during `make build-api`, so avoid importing npm toolchains. Pure HTML/CSS/JS (plus CDN-hosted dependencies like `marked`) keeps the binary reproducible.
3. **Respect IDs/structure:** `app.js` relies on specific IDs (`ask-form`, `question`, `status`, `output`, etc.). Update the JS when altering markup.
4. **Testing:** Run the API (`make run-api`) and hit `/` to verify static files load. Because assets are embedded, every Go rebuild picks up your changes automatically.
5. **Swagger + static files:** When adding new frontend endpoints/assets, no swagger changes are needed. Only adjust router/tests if you change the asset mount path.

## Adding Files

- Drop new files inside `internal/assets/static/`. The embed directive automatically includes anything under `static/` at build time.
- If you add large libraries, prefer bundling the minimized version (or load via CDN) to avoid bloating the binary.
- Update `README.md` if the developer workflow changes (e.g., new build steps).

## Troubleshooting

- **Not seeing changes:** Ensure you rebuilt the Go binary after editing assets. `go run ./cmd/api` picks up edits automatically because `go build` re-embeds files.
- **404s on assets:** Check that router middleware has not added a conflicting catch-all before `r.Handle("/*", ...)`.
- **Tests failing:** `internal/http/router_test.go` makes assumptions about the embedded HTML. Update expectations if you significantly change `index.html`.

Following this guide keeps the frontend lightweight, versioned with the backend, and trivial to ship.
