package api

import (
	"io/fs"
	"net/http"

	"tinyvm/web"
)

func (s *Server) registerRoutes(mux *http.ServeMux) {
	// Static & Vendor Assets embedded in web.Files
	staticFS, err := fs.Sub(web.Files, "static")
	if err == nil {
		mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))
	}

	vendorFS, err := fs.Sub(web.Files, "vendor")
	if err == nil {
		mux.Handle("GET /vendor/", http.StripPrefix("/vendor/", http.FileServer(http.FS(vendorFS))))
	}

	// Web UI Pages
	mux.HandleFunc("GET /{$}", s.handleDashboard)
	mux.HandleFunc("GET /vms", s.handleVMsList)
	mux.HandleFunc("GET /vms/new", s.handleVMCreate)
	mux.HandleFunc("GET /vms/{id}", s.handleVMDetail)
	mux.HandleFunc("GET /vms/{id}/console", s.handleVMConsole)
	mux.HandleFunc("GET /storage", s.handleStorage)
	mux.HandleFunc("GET /settings", s.handleSettings)

	// Health API
	mux.HandleFunc("GET /api/v1/health", s.handleHealth)
}
