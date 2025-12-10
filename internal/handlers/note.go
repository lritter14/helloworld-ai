package handlers

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	ghhtml "github.com/yuin/goldmark/renderer/html"

	"helloworld-ai/internal/vault"
)

// NoteHandler serves markdown notes as rendered HTML pages.
type NoteHandler struct {
	vaults   *vault.Manager
	parser   goldmark.Markdown
	template *template.Template
	logger   *slog.Logger
}

// notePageData holds template data for rendered note pages.
type notePageData struct {
	Title   string
	Vault   string
	RelPath string
	Content template.HTML
}

// NewNoteHandler creates a new handler for serving note files.
func NewNoteHandler(vaults *vault.Manager) *NoteHandler {
	tmpl := template.Must(template.New("note").Parse(`<!DOCTYPE html>
<html>
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>{{.Title}} — {{.Vault}} vault</title>
  <style>
    :root {
      color-scheme: dark;
    }
    body {
      font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif;
      margin: 0 auto;
      padding: 2rem;
      max-width: 900px;
      line-height: 1.7;
      background: #050b18;
      color: #e4ecff;
    }
    header {
      margin-bottom: 2rem;
      border-bottom: 1px solid rgba(148, 163, 184, 0.2);
      padding-bottom: 1.5rem;
    }
    h1 {
      margin-top: 0;
      color: #fff;
      font-size: 2rem;
    }
    article {
      background: rgba(12, 19, 35, 0.85);
      border: 1px solid rgba(99, 102, 241, 0.2);
      border-radius: 16px;
      padding: 2rem;
      box-shadow: 0 15px 35px rgba(2, 6, 23, 0.8);
    }
    article h2, article h3, article h4 {
      color: #c7d2fe;
      margin-top: 1.5rem;
    }
    article p {
      color: #cbd5f5;
    }
    pre {
      background: #0f172a;
      padding: 1rem;
      overflow-x: auto;
      border-radius: 10px;
      border: 1px solid rgba(99, 102, 241, 0.2);
    }
    code {
      font-family: 'SFMono-Regular', Consolas, 'Liberation Mono', Menlo, monospace;
      background: rgba(99, 102, 241, 0.18);
      padding: 2px 5px;
      border-radius: 6px;
      color: #cbd5ff;
    }
    pre code {
      background: transparent;
      padding: 0;
    }
    blockquote {
      border-left: 4px solid rgba(96, 165, 250, 0.6);
      padding-left: 1rem;
      margin-left: 0;
      color: #93c5fd;
      background: rgba(59, 130, 246, 0.08);
      border-radius: 6px;
    }
    a {
      color: #60a5fa;
      text-decoration: none;
    }
    a:hover {
      text-decoration: underline;
    }
    .meta {
      color: #94a3b8;
      font-size: 0.95rem;
      margin-top: 0.5rem;
    }
    @media (max-width: 640px) {
      body {
        padding: 1rem;
      }
      article {
        padding: 1.25rem;
      }
    }
  </style>
</head>
<body>
  <header>
    <h1>{{.Title}}</h1>
    <p class="meta">Vault: {{.Vault}} &middot; Path: {{.RelPath}}</p>
  </header>
  <article>{{.Content}}</article>
</body>
</html>`))

	return &NoteHandler{
		vaults: vaults,
		parser: goldmark.New(
			goldmark.WithExtensions(
				extension.GFM,
				extension.Table,
				extension.TaskList,
				extension.Strikethrough,
				extension.Linkify,
				extension.Typographer,
			),
			goldmark.WithRendererOptions(
				ghhtml.WithUnsafe(),
			),
			goldmark.WithParserOptions(
				parser.WithAutoHeadingID(),
			),
		),
		template: tmpl,
		logger:   slog.Default(),
	}
}

// ServeHTTP renders the requested note file as HTML.
func (h *NoteHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := h.getLogger(ctx)

	rawVault := strings.TrimSpace(chi.URLParam(r, "vault"))
	vaultName, err := url.PathUnescape(rawVault)
	if err != nil {
		http.Error(w, "invalid vault name", http.StatusBadRequest)
		return
	}
	if vaultName == "" {
		http.Error(w, "vault is required", http.StatusBadRequest)
		return
	}

	rawRelPath := chi.URLParam(r, "*")
	decodedRelPath, err := url.PathUnescape(rawRelPath)
	if err != nil {
		http.Error(w, "invalid path encoding", http.StatusBadRequest)
		return
	}

	relPath, err := cleanRelPath(decodedRelPath)
	if err != nil {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}

	vaultRecord, err := h.vaults.VaultByName(vaultName)
	if err != nil {
		logger.WarnContext(ctx, "unknown vault requested", "vault", vaultName, "error", err)
		http.Error(w, "vault not found", http.StatusNotFound)
		return
	}

	absPath, err := buildAbsPath(vaultRecord.RootPath, relPath)
	if err != nil {
		logger.WarnContext(ctx, "invalid note path", "vault", vaultName, "rel_path", relPath, "error", err)
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			http.Error(w, "note not found", http.StatusNotFound)
			return
		}
		logger.ErrorContext(ctx, "failed to read note", "path", absPath, "error", err)
		http.Error(w, "failed to read note", http.StatusInternalServerError)
		return
	}

	htmlContent, err := h.renderMarkdown(data)
	if err != nil {
		logger.ErrorContext(ctx, "failed to render markdown", "path", absPath, "error", err)
		http.Error(w, "failed to render note", http.StatusInternalServerError)
		return
	}

	pageData := notePageData{
		Title:   inferTitle(relPath),
		Vault:   vaultRecord.Name,
		RelPath: relPath,
		Content: template.HTML(htmlContent),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.template.Execute(w, pageData); err != nil {
		logger.ErrorContext(ctx, "failed to execute note template", "path", absPath, "error", err)
		http.Error(w, "failed to render note", http.StatusInternalServerError)
		return
	}
}

func (h *NoteHandler) renderMarkdown(content []byte) (string, error) {
	var buf bytes.Buffer
	if err := h.parser.Convert(content, &buf); err != nil {
		return "", fmt.Errorf("convert markdown: %w", err)
	}
	return buf.String(), nil
}

func cleanRelPath(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", errors.New("empty path")
	}

	cleaned := path.Clean("/" + trimmed)
	cleaned = strings.TrimPrefix(cleaned, "/")
	if cleaned == "" || cleaned == "." {
		return "", errors.New("invalid path")
	}

	for _, segment := range strings.Split(cleaned, "/") {
		if segment == ".." {
			return "", errors.New("path traversal detected")
		}
	}

	return cleaned, nil
}

func buildAbsPath(root, rel string) (string, error) {
	root = filepath.Clean(root)
	relFS := filepath.FromSlash(rel)
	abs := filepath.Join(root, relFS)

	if !strings.HasPrefix(abs, root+string(os.PathSeparator)) && abs != root {
		return "", errors.New("path escapes vault root")
	}
	return abs, nil
}

func inferTitle(rel string) string {
	base := filepath.Base(rel)
	if base == "." || base == "" {
		return "Note"
	}
	return strings.TrimSuffix(base, filepath.Ext(base))
}

func (h *NoteHandler) getLogger(ctx context.Context) *slog.Logger {
	type loggerKey string
	const key loggerKey = "logger"
	if ctxLogger := ctx.Value(key); ctxLogger != nil {
		if l, ok := ctxLogger.(*slog.Logger); ok {
			return l
		}
	}
	return h.logger
}
