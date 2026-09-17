package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"tinyvm/internal/config"
)

func newTestServer(t *testing.T) *Server {
	cfg := config.DefaultConfig()
	srv, err := NewServer(cfg, nil)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}
	return srv
}

func TestHealthEndpoint(t *testing.T) {
	srv := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	rec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var resp map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode JSON response: %v", err)
	}

	if resp["status"] != "ok" {
		t.Errorf("expected status 'ok', got %v", resp["status"])
	}
}

func TestDashboardEndpoint(t *testing.T) {
	srv := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "TinyVM") {
		t.Errorf("expected body to contain 'TinyVM'")
	}
	if !strings.Contains(body, "Dashboard Overview") {
		t.Errorf("expected body to contain 'Dashboard Overview'")
	}
}

func TestStaticCSS(t *testing.T) {
	srv := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/static/css/base.css", nil)
	rec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "--bg") {
		t.Errorf("expected CSS variables in base.css response")
	}
}

func TestWriteJSONError(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteJSONError(rec, http.StatusBadRequest, ErrCodeInvalidInput, "test error message")

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}

	var errResp APIError
	if err := json.NewDecoder(rec.Body).Decode(&errResp); err != nil {
		t.Fatalf("failed to decode error json: %v", err)
	}

	if errResp.Error.Code != ErrCodeInvalidInput {
		t.Errorf("expected code %s, got %s", ErrCodeInvalidInput, errResp.Error.Code)
	}
	if errResp.Error.Message != "test error message" {
		t.Errorf("expected message 'test error message', got %s", errResp.Error.Message)
	}
}
