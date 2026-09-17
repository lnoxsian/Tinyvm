package api

import (
	"fmt"
	"net/http"
	"runtime/debug"
	"strings"
	"time"
)

// statusRecorder captures the response status code for logging.
type statusRecorder struct {
	http.ResponseWriter
	statusCode  int
	wroteHeader bool
}

func (r *statusRecorder) WriteHeader(code int) {
	if !r.wroteHeader {
		r.statusCode = code
		r.wroteHeader = true
		r.ResponseWriter.WriteHeader(code)
	}
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if !r.wroteHeader {
		r.WriteHeader(http.StatusOK)
	}
	return r.ResponseWriter.Write(b)
}

// LoggingMiddleware logs incoming HTTP requests and response metrics.
func (s *Server) LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, statusCode: http.StatusOK}

		next.ServeHTTP(rec, r)

		duration := time.Since(start)
		s.logger.Info("HTTP request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.statusCode,
			"duration_ms", duration.Milliseconds(),
			"remote_addr", r.RemoteAddr,
		)
	})
}

// PanicRecoveryMiddleware recovers from panics in request handlers and returns 500 JSON.
func (s *Server) PanicRecoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				s.logger.Error("Panic recovered in HTTP handler",
					"error", fmt.Sprintf("%v", rec),
					"stack", string(debug.Stack()),
					"path", r.URL.Path,
				)
				WriteJSONError(w, http.StatusInternalServerError, ErrCodeInternal, "Internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// SecurityHeadersMiddleware attaches standard defensive HTTP headers.
func (s *Server) SecurityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "SAMEORIGIN")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		next.ServeHTTP(w, r)
	})
}

// AuthMiddleware implements Section 47 API token authentication.
// When an API token is configured in s.cfg.APIToken, requests to /api/v1/... (except /api/v1/health)
// require either 'Authorization: Bearer <token>' or query parameter '?token=<token>'.
// When no token is configured, requests proceed unhindered.
func (s *Server) AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := ""
		if s.cfg != nil {
			token = strings.TrimSpace(s.cfg.APIToken)
		}

		// If no token is configured, authentication is disabled (local trusted mode)
		if token == "" {
			next.ServeHTTP(w, r)
			return
		}

		// Whitelist health check and static assets
		if r.URL.Path == "/api/v1/health" || strings.HasPrefix(r.URL.Path, "/static/") || strings.HasPrefix(r.URL.Path, "/vendor/") {
			next.ServeHTTP(w, r)
			return
		}

		// Only enforce on API endpoints
		if strings.HasPrefix(r.URL.Path, "/api/") || r.Header.Get("Accept") == "application/json" {
			authHeader := r.Header.Get("Authorization")
			queryToken := r.URL.Query().Get("token")

			provided := ""
			if strings.HasPrefix(authHeader, "Bearer ") {
				provided = strings.TrimPrefix(authHeader, "Bearer ")
			} else if queryToken != "" {
				provided = queryToken
			}

			if provided != token {
				w.Header().Set("WWW-Authenticate", `Bearer realm="TinyVM API"`)
				WriteJSONError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Valid API token required")
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}
