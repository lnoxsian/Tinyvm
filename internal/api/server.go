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

	// Parse web templates
	pageTemplates := []string{"dashboard", "vm", "create", "console", "settings"}
	for _, page := range pageTemplates {
		tmpl, err := template.ParseFS(web.Files, "templates/layout.html", "templates/"+page+".html")
		if err != nil {
			return nil, fmt.Errorf("failed to parse template %s: %w", page, err)
		}
		s.templates[page] = tmpl
	}

	mux := http.NewServeMux()
	s.registerRoutes(mux)

	s.httpServer = &http.Server{
		Addr:           cfg.Addr(),
		Handler:        mux,
		ReadTimeout:    15 * time.Second,
		WriteTimeout:   15 * time.Second,
		IdleTimeout:    60 * time.Second,
		MaxHeaderBytes: 1 << 20, // 1 MB
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
