package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"tinyvm/internal/vm"
)

func TestConsole_PageRendering(t *testing.T) {
	srv := newTestServer(t)

	// Create a test VM
	createdVM, err := srv.vmMgr.CreateVM(vm.VMConfig{
		ID:         "test-console-vm",
		Name:       "Console Test VM",
		CPUs:       1,
		MemoryMB:   512,
		DiskSize:   "10M",
		DiskFormat: "qcow2",
		Firmware:   "uefi",
		OSType:     "Linux",
	})
	if err != nil {
		t.Fatalf("failed to create test VM: %v", err)
	}

	// 1. Stopped VM Console view
	req := httptest.NewRequest(http.MethodGet, "/vms/test-console-vm/console", nil)
	rec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for stopped VM console page, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "Console Test VM") {
		t.Errorf("expected body to contain VM Name")
	}
	if !strings.Contains(body, "Virtual Machine is Stopped") {
		t.Errorf("expected body to warn that VM is stopped")
	}

	// 2. Non-existent VM Console view
	reqNotFound := httptest.NewRequest(http.MethodGet, "/vms/non-existent-vm/console", nil)
	recNotFound := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(recNotFound, reqNotFound)

	if recNotFound.Code != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found for missing VM console, got %d", recNotFound.Code)
	}

	// 3. Mark VM as running in-memory and test active console view
	createdVM.Runtime.State = vm.StateRunning
	createdVM.Runtime.PID = 12345

	reqRunning := httptest.NewRequest(http.MethodGet, "/vms/test-console-vm/console", nil)
	recRunning := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(recRunning, reqRunning)

	if recRunning.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for running VM console page, got %d", recRunning.Code)
	}

	runningBody := recRunning.Body.String()
	if !strings.Contains(runningBody, "Graphical (noVNC)") {
		t.Errorf("expected body to contain noVNC tab")
	}
	if !strings.Contains(runningBody, "Terminal (SSH / Shell)") {
		t.Errorf("expected body to contain xterm Terminal (SSH / Shell) tab")
	}
	if !strings.Contains(runningBody, "Ctrl+Alt+Del") {
		t.Errorf("expected body to contain Ctrl+Alt+Del button")
	}
	if !strings.Contains(runningBody, "/vendor/xterm/xterm.js") {
		t.Errorf("expected running console to load xterm.js")
	}
}

func TestConsole_APIWebSocketEndpoints(t *testing.T) {
	srv := newTestServer(t)

	_, err := srv.vmMgr.CreateVM(vm.VMConfig{
		ID:         "ws-test-vm",
		Name:       "WS Test VM",
		CPUs:       1,
		MemoryMB:   512,
		DiskSize:   "10M",
		DiskFormat: "qcow2",
	})
	if err != nil {
		t.Fatalf("failed to create VM: %v", err)
	}

	// 1. Serial console endpoint when VM is stopped -> should return 409 Conflict
	reqConsole := httptest.NewRequest(http.MethodGet, "/api/v1/vms/ws-test-vm/console", nil)
	recConsole := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(recConsole, reqConsole)
	if recConsole.Code != http.StatusConflict {
		t.Errorf("expected 409 Conflict for stopped VM serial console, got %d", recConsole.Code)
	}

	// 2. VNC endpoint when VM is stopped -> should return 409 Conflict
	reqVNC := httptest.NewRequest(http.MethodGet, "/api/v1/vms/ws-test-vm/vnc", nil)
	recVNC := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(recVNC, reqVNC)
	if recVNC.Code != http.StatusConflict {
		t.Errorf("expected 409 Conflict for stopped VM VNC, got %d", recVNC.Code)
	}

	// 3. SSH endpoint when VM is stopped -> should return 409 Conflict
	reqSSH := httptest.NewRequest(http.MethodGet, "/api/v1/vms/ws-test-vm/ssh", nil)
	recSSH := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(recSSH, reqSSH)
	if recSSH.Code != http.StatusConflict {
		t.Errorf("expected 409 Conflict for stopped VM SSH, got %d", recSSH.Code)
	}

	// 4. Endpoints for non-existent VM -> should return 404
	reqMissing := httptest.NewRequest(http.MethodGet, "/api/v1/vms/ghost-vm/vnc", nil)
	recMissing := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(recMissing, reqMissing)
	if recMissing.Code != http.StatusNotFound {
		t.Errorf("expected 404 Not Found for non-existent VM, got %d", recMissing.Code)
	}

	reqMissingSSH := httptest.NewRequest(http.MethodGet, "/api/v1/vms/ghost-vm/ssh", nil)
	recMissingSSH := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(recMissingSSH, reqMissingSSH)
	if recMissingSSH.Code != http.StatusNotFound {
		t.Errorf("expected 404 Not Found for non-existent VM SSH, got %d", recMissingSSH.Code)
	}
}

func TestConsole_EmbeddedVendorAssets(t *testing.T) {
	srv := newTestServer(t)

	assets := []string{
		"/vendor/xterm/xterm.js",
		"/vendor/xterm/xterm.css",
		"/vendor/xterm/xterm-addon-fit.js",
		"/vendor/novnc/vnc.html",
		"/vendor/novnc/core/rfb.js",
		"/static/js/console.js",
		"/static/css/console.css",
	}

	for _, path := range assets {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		srv.httpServer.Handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("expected 200 OK for embedded asset %s, got %d", path, rec.Code)
		}
		if rec.Body.Len() == 0 {
			t.Errorf("expected non-empty body for embedded asset %s", path)
		}
		if strings.HasPrefix(path, "/vendor/") {
			if cc := rec.Header().Get("Cache-Control"); cc != "no-cache" {
				t.Errorf("expected Cache-Control: no-cache for vendor asset %s, got %q", path, cc)
			}
		}
	}

	// 4. Test standalone /vms/{id}/novnc redirect
	_, _ = srv.vmMgr.CreateVM(vm.VMConfig{ID: "novnc-vm", Name: "NoVNC VM", CPUs: 1, MemoryMB: 512})
	reqNovnc := httptest.NewRequest(http.MethodGet, "/vms/novnc-vm/novnc", nil)
	recNovnc := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(recNovnc, reqNovnc)
	if recNovnc.Code != http.StatusTemporaryRedirect {
		t.Fatalf("expected 307 Temporary Redirect for /vms/{id}/novnc, got %d", recNovnc.Code)
	}
	loc := recNovnc.Header().Get("Location")
	if !strings.Contains(loc, "vnc.html") || !strings.Contains(loc, "autoconnect=true") || !strings.Contains(loc, "path=/api/v1/vms/novnc-vm/vnc") {
		t.Errorf("unexpected redirect location: %s", loc)
	}
}
