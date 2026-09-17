package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"os"
	"testing"

	"tinyvm/internal/config"
	"tinyvm/internal/storage"
	"tinyvm/internal/vm"
)

func newTestServer(t *testing.T) *Server {
	tmpDir, err := os.MkdirTemp("", "tinyvm-api-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(tmpDir) })

	cfg := config.DefaultConfig()
	cfg.DataDir = tmpDir

	s, err := storage.New(tmpDir)
	if err != nil {
		t.Fatalf("failed to initialize storage: %v", err)
	}

	vmMgr := vm.NewManager(s, nil)
	srv, err := NewServer(cfg, nil, vmMgr)
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

func TestWebPagesRendering(t *testing.T) {
	srv := newTestServer(t)

	// Create a dummy VM so /vms and /vms/{id} have data to render
	_, err := srv.vmMgr.CreateVM(vm.VMConfig{
		ID:         "test-web-vm",
		Name:       "Test Web VM",
		CPUs:       1,
		MemoryMB:   256,
		DiskSize:   "10M",
		DiskFormat: "qcow2",
	})
	if err != nil {
		t.Fatalf("failed to create dummy VM: %v", err)
	}

	pages := []struct {
		url          string
		expectedText string
	}{
		{"/", "Dashboard Overview"},
		{"/vms", "Virtual Machines"},
		{"/vms/new", "Create Virtual Machine"},
		{"/vms/test-web-vm", "Test Web VM"},
		{"/vms/test-web-vm/console", "Console"},
		{"/storage", "Storage Pools"},
		{"/settings", "Application Settings"},
	}

	for _, p := range pages {
		req := httptest.NewRequest(http.MethodGet, p.url, nil)
		req.Header.Set("Accept", "text/html,application/xhtml+xml")
		rec := httptest.NewRecorder()
		srv.httpServer.Handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("page %s returned status %d, expected 200. Body: %s", p.url, rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), p.expectedText) {
			t.Errorf("page %s expected to contain %q, but didn't. Body: %s", p.url, p.expectedText, rec.Body.String())
		}
	}
}
