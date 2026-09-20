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
		vendorHandler := http.StripPrefix("/vendor/", http.FileServer(http.FS(vendorFS)))
		// Per noVNC EMBEDDING.md: tell browsers to revalidate vendor assets using conditional requests to avoid cache mismatch
		mux.Handle("GET /vendor/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "no-cache")
			vendorHandler.ServeHTTP(w, r)
		}))
	}

	// Web UI Pages
	mux.HandleFunc("GET /{$}", s.handleDashboard)
	mux.HandleFunc("GET /vms", s.handleVMsList)
	mux.HandleFunc("GET /vms/new", s.handleVMCreate)
	mux.HandleFunc("GET /vms/{id}", s.handleVMDetail)
	mux.HandleFunc("GET /vms/{id}/console", s.handleVMConsole)
	mux.HandleFunc("GET /vms/{id}/novnc", s.handleVMNoVNCApp)
	mux.HandleFunc("GET /storage", s.handleStorage)
	mux.HandleFunc("POST /storage/upload", s.handleStorageUpload)
	mux.HandleFunc("GET /settings", s.handleSettings)

	// HTMX Partials
	mux.HandleFunc("GET /partials/stats", s.handlePartialStats)
	mux.HandleFunc("GET /partials/vms", s.handlePartialVMs)
	mux.HandleFunc("GET /partials/vms/{id}", s.handlePartialVMCard)
	mux.HandleFunc("GET /partials/vms/{id}/metrics", s.handlePartialVMMetrics)

	// Health API
	mux.HandleFunc("GET /api/v1/health", s.handleHealth)

	// REST API v1
	mux.HandleFunc("GET /api/v1/metrics", s.handleAPIMetrics)
	mux.HandleFunc("GET /api/v1/vms", s.handleAPIVMsList)
	mux.HandleFunc("POST /api/v1/vms", s.handleAPIVMCreate)
	mux.HandleFunc("GET /api/v1/vms/{id}", s.handleAPIVMGet)
	mux.HandleFunc("DELETE /api/v1/vms/{id}", s.handleAPIVMDelete)
	mux.HandleFunc("POST /api/v1/vms/{id}/start", s.handleAPIVMStart)
	mux.HandleFunc("POST /api/v1/vms/{id}/shutdown", s.handleAPIVMShutdown)
	mux.HandleFunc("POST /api/v1/vms/{id}/restart", s.handleAPIVMRestart)
	mux.HandleFunc("POST /api/v1/vms/{id}/stop", s.handleAPIVMStop)
	mux.HandleFunc("POST /api/v1/vms/{id}/quit", s.handleAPIVMQuit)
	mux.HandleFunc("POST /api/v1/vms/{id}/eject-iso", s.handleAPIVMEjectISO)
	mux.HandleFunc("POST /api/v1/vms/{id}/attach-iso", s.handleAPIVMAttachISO)
	mux.HandleFunc("POST /api/v1/vms/{id}/boot", s.handleAPIVMBoot)
	mux.HandleFunc("POST /api/v1/vms/{id}/reset", s.handleAPIVMReset)
	mux.HandleFunc("POST /api/v1/vms/{id}/pause", s.handleAPIVMPause)
	mux.HandleFunc("POST /api/v1/vms/{id}/resume", s.handleAPIVMResume)
	mux.HandleFunc("PUT /api/v1/vms/{id}", s.handleAPIVMUpdateConfig)
	mux.HandleFunc("POST /api/v1/vms/{id}/config", s.handleAPIVMUpdateConfig)
	mux.HandleFunc("POST /api/v1/vms/{id}/resize-disk", s.handleAPIVMResizeDisk)
	mux.HandleFunc("GET /api/v1/vms/{id}/snapshots", s.handleAPIVMSnapshotsList)
	mux.HandleFunc("POST /api/v1/vms/{id}/snapshots", s.handleAPIVMSnapshotCreate)
	mux.HandleFunc("POST /api/v1/vms/{id}/snapshots/{name}/rollback", s.handleAPIVMSnapshotRollback)
	mux.HandleFunc("DELETE /api/v1/vms/{id}/snapshots/{name}", s.handleAPIVMSnapshotDelete)
	mux.HandleFunc("POST /api/v1/vms/{id}/clone", s.handleAPIVMClone)
	mux.HandleFunc("GET /api/v1/vms/{id}/logs", s.handleAPIVMLogs)
	mux.HandleFunc("GET /api/v1/vms/{id}/status", s.handleAPIVMStatus)
	mux.HandleFunc("GET /api/v1/vms/{id}/metrics", s.handleAPIVMMetrics)
	mux.HandleFunc("GET /api/v1/isos", s.handleAPIISOsList)
	mux.HandleFunc("POST /api/v1/isos", s.handleAPIISOUpload)
	mux.HandleFunc("POST /api/v1/isos/download", s.handleAPIISODownload)
	mux.HandleFunc("DELETE /api/v1/isos/{name}", s.handleAPIISODelete)

	// WebSockets (Serial Console & Graphical VNC RFB stream & SSH/Shell PTY)
	mux.HandleFunc("GET /api/v1/vms/{id}/console", s.handleAPIVMConsoleWS)
	mux.HandleFunc("GET /api/v1/vnc", s.handleAPIVMVncWS)
	mux.HandleFunc("GET /api/v1/vms/{id}/vnc", s.handleAPIVMVncWS)
	mux.HandleFunc("GET /api/v1/vms/{id}/ssh", s.handleAPIVMSSHWS)
	mux.HandleFunc("GET /vms/{id}/ws/console", s.handleAPIVMConsoleWS)
	mux.HandleFunc("GET /vms/{id}/ws/vnc", s.handleAPIVMVncWS)
	mux.HandleFunc("GET /vms/{id}/ws/ssh", s.handleAPIVMSSHWS)
	mux.HandleFunc("GET /vms/{id}/vnc", s.handleAPIVMVncWS)

	// Direct REST API Shorthands
	mux.HandleFunc("GET /metrics", s.handleAPIMetrics)
	mux.HandleFunc("GET /vms/{id}/metrics", s.handleAPIVMMetrics)
	mux.HandleFunc("POST /vms", s.handleAPIVMCreate)
	mux.HandleFunc("DELETE /vms/{id}", s.handleAPIVMDelete)
	mux.HandleFunc("POST /vms/{id}/start", s.handleAPIVMStart)
	mux.HandleFunc("POST /vms/{id}/shutdown", s.handleAPIVMShutdown)
	mux.HandleFunc("POST /vms/{id}/restart", s.handleAPIVMRestart)
	mux.HandleFunc("POST /vms/{id}/stop", s.handleAPIVMStop)
	mux.HandleFunc("POST /vms/{id}/quit", s.handleAPIVMQuit)
	mux.HandleFunc("POST /vms/{id}/reset", s.handleAPIVMReset)
	mux.HandleFunc("POST /vms/{id}/pause", s.handleAPIVMPause)
	mux.HandleFunc("POST /vms/{id}/resume", s.handleAPIVMResume)
	mux.HandleFunc("POST /vms/{id}/eject-iso", s.handleAPIVMEjectISO)
	mux.HandleFunc("POST /vms/{id}/attach-iso", s.handleAPIVMAttachISO)
	mux.HandleFunc("POST /vms/{id}/boot", s.handleAPIVMBoot)
	mux.HandleFunc("POST /vms/{id}/config", s.handleAPIVMUpdateConfig)
	mux.HandleFunc("POST /vms/{id}/resize-disk", s.handleAPIVMResizeDisk)
	mux.HandleFunc("GET /vms/{id}/snapshots", s.handleAPIVMSnapshotsList)
	mux.HandleFunc("POST /vms/{id}/snapshots", s.handleAPIVMSnapshotCreate)
	mux.HandleFunc("POST /vms/{id}/snapshots/{name}/rollback", s.handleAPIVMSnapshotRollback)
	mux.HandleFunc("POST /vms/{id}/snapshots/{name}/delete", s.handleAPIVMSnapshotDelete)
	mux.HandleFunc("POST /vms/{id}/clone", s.handleAPIVMClone)
	mux.HandleFunc("GET /vms/{id}/logs", s.handleAPIVMLogs)
	mux.HandleFunc("GET /vms/{id}/status", s.handleAPIVMStatus)
	mux.HandleFunc("GET /isos", s.handleAPIISOsList)
	mux.HandleFunc("POST /isos", s.handleAPIISOUpload)
	mux.HandleFunc("POST /isos/download", s.handleAPIISODownload)
	mux.HandleFunc("POST /storage/download", s.handleAPIISODownload)
	mux.HandleFunc("DELETE /isos/{name}", s.handleAPIISODelete)

	// Action endpoints supporting query string (?id=...) and JSON body ({"id": "..."})
	mux.HandleFunc("POST /api/v1/start", s.handleAPIVMStart)
	mux.HandleFunc("POST /api/v1/shutdown", s.handleAPIVMShutdown)
	mux.HandleFunc("POST /api/v1/restart", s.handleAPIVMRestart)
	mux.HandleFunc("POST /api/v1/stop", s.handleAPIVMStop)
	mux.HandleFunc("POST /api/v1/quit", s.handleAPIVMQuit)
	mux.HandleFunc("POST /api/v1/reset", s.handleAPIVMReset)
	mux.HandleFunc("POST /api/v1/pause", s.handleAPIVMPause)
	mux.HandleFunc("POST /api/v1/resume", s.handleAPIVMResume)
	mux.HandleFunc("POST /api/v1/eject-iso", s.handleAPIVMEjectISO)
	mux.HandleFunc("POST /api/v1/attach-iso", s.handleAPIVMAttachISO)
	mux.HandleFunc("POST /api/v1/boot", s.handleAPIVMBoot)
	mux.HandleFunc("GET /api/v1/status", s.handleAPIVMStatus)
	mux.HandleFunc("POST /api/v1/status", s.handleAPIVMStatus)

	mux.HandleFunc("POST /start", s.handleAPIVMStart)
	mux.HandleFunc("POST /shutdown", s.handleAPIVMShutdown)
	mux.HandleFunc("POST /restart", s.handleAPIVMRestart)
	mux.HandleFunc("POST /stop", s.handleAPIVMStop)
	mux.HandleFunc("POST /quit", s.handleAPIVMQuit)
	mux.HandleFunc("POST /reset", s.handleAPIVMReset)
	mux.HandleFunc("POST /pause", s.handleAPIVMPause)
	mux.HandleFunc("POST /resume", s.handleAPIVMResume)
	mux.HandleFunc("POST /eject-iso", s.handleAPIVMEjectISO)
	mux.HandleFunc("POST /attach-iso", s.handleAPIVMAttachISO)
	mux.HandleFunc("POST /boot", s.handleAPIVMBoot)
	mux.HandleFunc("GET /status", s.handleAPIVMStatus)
	mux.HandleFunc("POST /status", s.handleAPIVMStatus)
}
