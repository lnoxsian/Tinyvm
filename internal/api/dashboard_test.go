package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"tinyvm/internal/vm"
)

func TestDashboard_HostStatsAndRendering(t *testing.T) {
	srv := newTestServer(t)

	// Create a dummy VM to populate the dashboard
	_, err := srv.vmMgr.CreateVM(vm.VMConfig{
		ID:         "dash-vm-1",
		Name:       "Dashboard VM One",
		CPUs:       2,
		MemoryMB:   1024,
		DiskSize:   "10M",
		DiskFormat: "qcow2",
		Firmware:   "bios",
		OSType:     "Linux",
	})
	if err != nil {
		t.Fatalf("failed to create dummy VM: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on dashboard, got %d", rec.Code)
	}

	body := rec.Body.String()

	// Verify stat cards structure from Section 9
	if !strings.Contains(body, "Dashboard Overview") {
		t.Errorf("expected body to contain 'Dashboard Overview'")
	}
	if !strings.Contains(body, "Virtual Machines") {
		t.Errorf("expected body to contain 'Virtual Machines'")
	}
	if !strings.Contains(body, "Host CPU") {
		t.Errorf("expected body to contain 'Host CPU'")
	}
	if !strings.Contains(body, "Host Memory") {
		t.Errorf("expected body to contain 'Host Memory'")
	}
	if !strings.Contains(body, "Storage") {
		t.Errorf("expected body to contain 'Storage'")
	}

	// Verify VM card content from Section 10
	if !strings.Contains(body, "Dashboard VM One") {
		t.Errorf("expected body to contain 'Dashboard VM One'")
	}
	if !strings.Contains(body, "dash-vm-1") {
		t.Errorf("expected body to contain 'dash-vm-1'")
	}
	if !strings.Contains(body, "BIOS") {
		t.Errorf("expected body to contain 'BIOS'")
	}
	if !strings.Contains(body, "Linux") {
		t.Errorf("expected body to contain 'Linux' OSType badge")
	}
	if !strings.Contains(body, "1024 MB") {
		t.Errorf("expected body to contain '1024 MB'")
	}
	if !strings.Contains(body, "Edit") {
		t.Errorf("expected body to contain 'Edit' button")
	}
}

func TestDashboard_HTMXPartials(t *testing.T) {
	srv := newTestServer(t)

	// 1. GET /partials/stats
	req := httptest.NewRequest(http.MethodGet, "/partials/stats", nil)
	req.Header.Set("HX-Request", "true")
	rec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on /partials/stats, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "id=\"stats-grid\"") {
		t.Errorf("expected partial stats to contain 'id=\"stats-grid\"'")
	}

	// 2. GET /partials/vms (empty state initially)
	req = httptest.NewRequest(http.MethodGet, "/partials/vms", nil)
	req.Header.Set("HX-Request", "true")
	rec = httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on /partials/vms, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "id=\"vm-grid\"") {
		t.Errorf("expected partial vms to contain 'id=\"vm-grid\"'")
	}
	if !strings.Contains(rec.Body.String(), "No Virtual Machines") {
		t.Errorf("expected empty state message in partial vms")
	}

	// 3. Create a VM and fetch single card partial
	_, err := srv.vmMgr.CreateVM(vm.VMConfig{
		ID:         "partial-vm",
		Name:       "Partial Test VM",
		CPUs:       1,
		MemoryMB:   512,
		DiskSize:   "10M",
		DiskFormat: "qcow2",
		Firmware:   "uefi",
	})
	if err != nil {
		t.Fatalf("failed to create VM: %v", err)
	}

	req = httptest.NewRequest(http.MethodGet, "/partials/vms/partial-vm", nil)
	req.Header.Set("HX-Request", "true")
	rec = httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on /partials/vms/partial-vm, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "id=\"vm-card-partial-vm\"") {
		t.Errorf("expected partial card to contain 'id=\"vm-card-partial-vm\"'")
	}
	if !strings.Contains(body, "Partial Test VM") {
		t.Errorf("expected partial card to contain 'Partial Test VM'")
	}
	if !strings.Contains(body, "UEFI") {
		t.Errorf("expected partial card to contain 'UEFI'")
	}
}

func TestDashboard_HTMXLifecycleActions(t *testing.T) {
	srv := newTestServer(t)

	// Create VM
	body, _ := json.Marshal(CreateVMRequest{
		ID:         "htmx-lifecycle-vm",
		Name:       "HTMX Lifecycle VM",
		CPUs:       1,
		MemoryMB:   256,
		DiskSize:   "10M",
		DiskFormat: "qcow2",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/vms", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("failed to create VM: %d", rec.Code)
	}

	// 1. Start VM with HX-Request -> returns rendered VM card snippet with HX-Trigger
	req = httptest.NewRequest(http.MethodPost, "/api/v1/vms/htmx-lifecycle-vm/start", nil)
	req.Header.Set("HX-Request", "true")
	rec = httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on HTMX start, got %d: %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("HX-Trigger") != "vm-updated" {
		t.Errorf("expected HX-Trigger: vm-updated, got %s", rec.Header().Get("HX-Trigger"))
	}
	cardHTML := rec.Body.String()
	if !strings.Contains(cardHTML, "id=\"vm-card-htmx-lifecycle-vm\"") {
		t.Errorf("expected cardHTML to contain 'id=\"vm-card-htmx-lifecycle-vm\"'")
	}
	if !strings.Contains(cardHTML, "running") {
		t.Errorf("expected cardHTML to contain 'running'")
	}
	if !strings.Contains(cardHTML, "Console") {
		t.Errorf("expected cardHTML to contain 'Console' action button")
	}
	if !strings.Contains(cardHTML, "Shutdown") {
		t.Errorf("expected cardHTML to contain 'Shutdown' action button")
	}
	if !strings.Contains(cardHTML, "Force Stop") {
		t.Errorf("expected cardHTML to contain 'Force Stop' action button")
	}

	// 2. Stop VM with HX-Request -> returns rendered VM card in stopped state
	req = httptest.NewRequest(http.MethodPost, "/api/v1/vms/htmx-lifecycle-vm/stop", nil)
	req.Header.Set("HX-Request", "true")
	rec = httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on HTMX stop, got %d: %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("HX-Trigger") != "vm-updated" {
		t.Errorf("expected HX-Trigger: vm-updated on stop")
	}
	stoppedCardHTML := rec.Body.String()
	if !strings.Contains(stoppedCardHTML, "stopped") {
		t.Errorf("expected stoppedCardHTML to contain 'stopped'")
	}
	if !strings.Contains(stoppedCardHTML, "Start") {
		t.Errorf("expected stoppedCardHTML to contain 'Start' action button")
	}

	// 3. Delete VM with HX-Request -> returns 200 OK with empty body and HX-Trigger: vm-updated
	req = httptest.NewRequest(http.MethodDelete, "/api/v1/vms/htmx-lifecycle-vm", nil)
	req.Header.Set("HX-Request", "true")
	rec = httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on HTMX delete, got %d: %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("HX-Trigger") != "vm-updated" {
		t.Errorf("expected HX-Trigger: vm-updated on delete")
	}
	if rec.Body.Len() != 0 {
		t.Errorf("expected empty body on HTMX delete, got %q", rec.Body.String())
	}
}
