package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"tinyvm/internal/vm"
)

func TestMetrics_HostEndpoints(t *testing.T) {
	srv := newTestServer(t)

	// 1. Initial host metrics (no VMs)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/metrics", nil)
	rec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body: %s", rec.Code, rec.Body.String())
	}

	var hostMetrics vm.HostMetrics
	if err := json.NewDecoder(rec.Body).Decode(&hostMetrics); err != nil {
		t.Fatalf("failed to decode HostMetrics JSON: %v", err)
	}

	if hostMetrics.CPUCores <= 0 {
		t.Errorf("expected CPUCores > 0, got %d", hostMetrics.CPUCores)
	}
	if hostMetrics.TotalVMs != 0 {
		t.Errorf("expected TotalVMs == 0, got %d", hostMetrics.TotalVMs)
	}

	// 2. Create a VM and check shorthand /metrics endpoint
	vmReqBody, _ := json.Marshal(CreateVMRequest{
		ID:         "mon-test-vm",
		Name:       "Monitoring Test VM",
		CPUs:       2,
		MemoryMB:   1024,
		DiskFormat: "qcow2",
		DiskSize:   "10M",
	})
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/vms", bytes.NewReader(vmReqBody))
	createRec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("failed to create VM: %d", createRec.Code)
	}

	shortReq := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	shortRec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(shortRec, shortReq)
	if shortRec.Code != http.StatusOK {
		t.Fatalf("expected status 200 for /metrics, got %d", shortRec.Code)
	}

	var shortMetrics vm.HostMetrics
	if err := json.NewDecoder(shortRec.Body).Decode(&shortMetrics); err != nil {
		t.Fatalf("failed to decode JSON from /metrics: %v", err)
	}
	if shortMetrics.TotalVMs != 1 {
		t.Errorf("expected TotalVMs == 1, got %d", shortMetrics.TotalVMs)
	}
	if shortMetrics.StoppedVMs != 1 {
		t.Errorf("expected StoppedVMs == 1, got %d", shortMetrics.StoppedVMs)
	}
	if len(shortMetrics.VMs) != 1 {
		t.Fatalf("expected 1 VM metric, got %d", len(shortMetrics.VMs))
	}
	if shortMetrics.VMs[0].ID != "mon-test-vm" {
		t.Errorf("expected VM ID 'mon-test-vm', got '%s'", shortMetrics.VMs[0].ID)
	}
}

func TestMetrics_VMEndpoints(t *testing.T) {
	srv := newTestServer(t)

	// Create test VM
	vmReqBody, _ := json.Marshal(CreateVMRequest{
		ID:         "vm-telemetry-1",
		Name:       "Telemetry VM 1",
		CPUs:       4,
		MemoryMB:   2048,
		DiskFormat: "qcow2",
		DiskSize:   "20G",
	})
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/vms", bytes.NewReader(vmReqBody))
	createRec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("failed to create VM: %d", createRec.Code)
	}

	// 1. GET /api/v1/vms/vm-telemetry-1/metrics
	req := httptest.NewRequest(http.MethodGet, "/api/v1/vms/vm-telemetry-1/metrics", nil)
	rec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body: %s", rec.Code, rec.Body.String())
	}

	var m vm.VMMetrics
	if err := json.NewDecoder(rec.Body).Decode(&m); err != nil {
		t.Fatalf("failed to decode VMMetrics: %v", err)
	}
	if m.ID != "vm-telemetry-1" {
		t.Errorf("expected ID 'vm-telemetry-1', got '%s'", m.ID)
	}
	if m.Status != vm.StateStopped {
		t.Errorf("expected status 'stopped', got '%s'", m.Status)
	}
	if m.MemoryConfigMB != 2048 {
		t.Errorf("expected MemoryConfigMB 2048, got %d", m.MemoryConfigMB)
	}
	if m.DiskVirtualBytes != 20*1024*1024*1024 {
		t.Errorf("expected DiskVirtualBytes 20GB, got %d", m.DiskVirtualBytes)
	}

	// 2. Shorthand GET /vms/vm-telemetry-1/metrics
	shortReq := httptest.NewRequest(http.MethodGet, "/vms/vm-telemetry-1/metrics", nil)
	shortRec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(shortRec, shortReq)
	if shortRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for /vms/vm-telemetry-1/metrics, got %d", shortRec.Code)
	}

	// 3. Non-existent VM ID -> 404
	errReq := httptest.NewRequest(http.MethodGet, "/api/v1/vms/does-not-exist/metrics", nil)
	errRec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(errRec, errReq)
	if errRec.Code != http.StatusNotFound {
		t.Errorf("expected 404 for non-existent VM, got %d", errRec.Code)
	}
}

func TestMetrics_HTMXPartialAndDetailPage(t *testing.T) {
	srv := newTestServer(t)

	// Create test VM
	vmReqBody, _ := json.Marshal(CreateVMRequest{
		ID:         "vm-partial-test",
		Name:       "Partial Test VM",
		CPUs:       1,
		MemoryMB:   512,
		DiskFormat: "qcow2",
		DiskSize:   "10G",
	})
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/vms", bytes.NewReader(vmReqBody))
	createRec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("failed to create VM: %d", createRec.Code)
	}

	// 1. GET /partials/vms/vm-partial-test/metrics
	req := httptest.NewRequest(http.MethodGet, "/partials/vms/vm-partial-test/metrics", nil)
	rec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `id="vm-telemetry"`) {
		t.Errorf("expected partial to contain id=\"vm-telemetry\", got: %s", body)
	}
	if !strings.Contains(body, `hx-get="/partials/vms/vm-partial-test/metrics"`) {
		t.Errorf("expected partial to have hx-get trigger for VM metrics")
	}
	if !strings.Contains(body, "Live Telemetry") {
		t.Errorf("expected partial to contain 'Live Telemetry'")
	}
	if !strings.Contains(body, "CPU Usage") || !strings.Contains(body, "RAM RSS") {
		t.Errorf("expected partial to show CPU and RAM metrics")
	}
	if strings.Contains(body, "Process PID") {
		t.Errorf("expected partial to NOT contain 'Process PID'")
	}

	// 2. Partial for non-existent VM -> 404
	badReq := httptest.NewRequest(http.MethodGet, "/partials/vms/unknown-ghost/metrics", nil)
	badRec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(badRec, badReq)
	if badRec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", badRec.Code)
	}

	// 3. GET /vms/vm-partial-test (Detail Page) contains the telemetry container
	detailReq := httptest.NewRequest(http.MethodGet, "/vms/vm-partial-test", nil)
	detailReq.Header.Set("Accept", "text/html")
	detailRec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(detailRec, detailReq)

	if detailRec.Code != http.StatusOK {
		t.Fatalf("expected status 200 for detail page, got %d", detailRec.Code)
	}
	detailBody := detailRec.Body.String()
	if !strings.Contains(detailBody, `id="vm-telemetry"`) {
		t.Errorf("expected detail page to include #vm-telemetry container")
	}
}

func TestMetrics_SimulatedRunningVM(t *testing.T) {
	srv := newTestServer(t)

	vmReqBody, _ := json.Marshal(CreateVMRequest{
		ID:         "vm-running-mon",
		Name:       "Running Mon VM",
		CPUs:       2,
		MemoryMB:   1024,
		DiskFormat: "qcow2",
		DiskSize:   "5G",
	})
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/vms", bytes.NewReader(vmReqBody))
	createRec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("failed to create VM: %d", createRec.Code)
	}

	// Simulate running VM using the test process PID
	targetVM, err := srv.vmMgr.GetVM("vm-running-mon")
	if err != nil {
		t.Fatalf("failed to retrieve VM: %v", err)
	}

	targetVM.Runtime.State = vm.StateRunning
	targetVM.Runtime.PID = os.Getpid()
	targetVM.Runtime.StartedAt = time.Now().Add(-75 * time.Second)

	// Test JSON metrics
	req := httptest.NewRequest(http.MethodGet, "/api/v1/vms/vm-running-mon/metrics", nil)
	rec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var m vm.VMMetrics
	if err := json.NewDecoder(rec.Body).Decode(&m); err != nil {
		t.Fatalf("failed to decode VMMetrics: %v", err)
	}
	if m.Status != vm.StateRunning {
		t.Errorf("expected StateRunning, got %s", m.Status)
	}
	if m.PID != os.Getpid() {
		t.Errorf("expected PID %d, got %d", os.Getpid(), m.PID)
	}
	if m.UptimeSeconds < 70 {
		t.Errorf("expected uptime >= 70s, got %d", m.UptimeSeconds)
	}
	if m.Uptime == "" {
		t.Errorf("expected non-empty formatted uptime")
	}

	// Test HTMX Partial for running VM
	partReq := httptest.NewRequest(http.MethodGet, "/partials/vms/vm-running-mon/metrics", nil)
	partRec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(partRec, partReq)

	if partRec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", partRec.Code)
	}
	partBody := partRec.Body.String()
	if !strings.Contains(partBody, "Live") {
		t.Errorf("expected partial to indicate 'Live' status for running VM")
	}
	if !strings.Contains(partBody, "Uptime:") {
		t.Errorf("expected partial to show 'Uptime:' label")
	}

	// Test Dashboard card view includes CPU percent
	cardReq := httptest.NewRequest(http.MethodGet, "/partials/vms/vm-running-mon", nil)
	cardRec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(cardRec, cardReq)

	if cardRec.Code != http.StatusOK {
		t.Fatalf("expected status 200 for card partial, got %d", cardRec.Code)
	}
	cardBody := cardRec.Body.String()
	if !strings.Contains(cardBody, "CPU:") {
		t.Errorf("expected VM card to show 'CPU:' when running, got: %s", cardBody)
	}
}
