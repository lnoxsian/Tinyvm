package api

import (
	"context"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"time"

	"tinyvm/internal/config"
	"tinyvm/internal/vm"
	"tinyvm/web"
)

// Server encapsulates the HTTP API and frontend web server.
type Server struct {
	cfg        *config.Config
	logger     *slog.Logger
	httpServer *http.Server
	templates  map[string]*template.Template
	vmMgr      *vm.Manager
}

// NewServer creates a new configured Server instance.
func NewServer(cfg *config.Config, logger *slog.Logger, vmMgr *vm.Manager) (*Server, error) {
	if logger == nil {
		logger = slog.Default()
	}

	s := &Server{
		cfg:       cfg,
		logger:    logger,
		templates: make(map[string]*template.Template),
		vmMgr:     vmMgr,
	}

	toFloat64 := func(v any) float64 {
		switch n := v.(type) {
		case int:
			return float64(n)
		case int64:
			return float64(n)
		case uint64:
			return float64(n)
		case float64:
			return n
		case float32:
			return float64(n)
		default:
			return 0
		}
	}

	funcMap := template.FuncMap{
		"float64": toFloat64,
		"div": func(a, b any) float64 {
			fb := toFloat64(b)
			if fb == 0 {
				return 0
			}
			return toFloat64(a) / fb
		},
		"mul": func(a, b any) float64 {
			return toFloat64(a) * toFloat64(b)
		},
		"add": func(a, b any) float64 {
			return toFloat64(a) + toFloat64(b)
		},
		"sub": func(a, b any) float64 {
			return toFloat64(a) - toFloat64(b)
		},
		"progressClass": func(pct int) string {
			if pct > 85 {
				return "danger"
			}
			if pct > 65 {
				return "warning"
			}
			return "accent"
		},
		"formatFloat": func(f float64) string {
			return fmt.Sprintf("%.1f", f)
		},
		"toInt": func(v any) int {
			switch n := v.(type) {
			case int:
				return n
			case int64:
				return int(n)
			case uint64:
				return int(n)
			case float64:
				return int(n)
			case float32:
				return int(n)
			default:
				return 0
			}
		},
		"calcPercent": func(part, total any) int {
			var p, t float64
			switch n := part.(type) {
			case int:
				p = float64(n)
			case int64:
				p = float64(n)
			case uint64:
				p = float64(n)
			case float64:
				p = n
			}
			switch n := total.(type) {
			case int:
				t = float64(n)
			case int64:
				t = float64(n)
			case uint64:
				t = float64(n)
			case float64:
				t = n
			}
			if t <= 0 {
				return 0
			}
			pct := int((p / t) * 100)
			if pct < 0 {
				return 0
			}
			if pct > 100 {
				return 100
			}
			return pct
		},
		"formatBytes": func(v any) string {
			var b int64
			switch n := v.(type) {
			case int:
				b = int64(n)
			case int64:
				b = n
			case uint64:
				b = int64(n)
			case float64:
				b = int64(n)
			}
			if b <= 0 {
				return "0 B"
			}
			const unit = 1024
			if b < unit {
				return fmt.Sprintf("%d B", b)
			}
			div, exp := int64(unit), 0
			for n := b / unit; n >= unit; n /= unit {
				div *= unit
				exp++
			}
			return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
		},
	}

	// Parse web templates with shared modular components
	pageTemplates := []string{"dashboard", "vm", "create", "console", "settings", "storage", "detail"}
	for _, page := range pageTemplates {
		tmpl, err := template.New("layout.html").Funcs(funcMap).ParseFS(
			web.Files,
			"templates/layout.html",
			"templates/card.html",
			"templates/stats.html",
			"templates/grid.html",
			"templates/telemetry.html",
			"templates/"+page+".html",
		)
		if err != nil {
			return nil, fmt.Errorf("failed to parse template %s: %w", page, err)
		}
		s.templates[page] = tmpl
	}

	mux := http.NewServeMux()
	s.registerRoutes(mux)

	// Middleware pipeline: Panic Recovery -> Request Logging -> Security Headers -> Auth -> Mux
	var handler http.Handler = mux
	handler = s.AuthMiddleware(handler)
	handler = s.SecurityHeadersMiddleware(handler)
	handler = s.LoggingMiddleware(handler)
	handler = s.PanicRecoveryMiddleware(handler)

	s.httpServer = &http.Server{
		Addr:              cfg.Addr(),
		Handler:           handler,
		ReadHeaderTimeout: 30 * time.Second,
		ReadTimeout:       2 * time.Hour,
		WriteTimeout:      2 * time.Hour,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20, // 1 MB
	}

	return s, nil
}

// Start begins listening and serving HTTP requests.
func (s *Server) Start() error {
	s.logger.Info("Starting TinyVM server", "addr", s.cfg.Addr(), "data_dir", s.cfg.DataDir)
	if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("http server failed: %w", err)
	}
	return nil
}

// Shutdown gracefully shuts down the server without interrupting active requests.
func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Info("Shutting down HTTP server")
	return s.httpServer.Shutdown(ctx)
}
