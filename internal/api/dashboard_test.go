package api

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
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
	// Per overview requirements: BIOS/UEFI and OS name are omitted from the overview card
	if strings.Contains(body, ">BIOS<") {
		t.Errorf("expected overview card NOT to contain 'BIOS' badge")
	}
	if strings.Contains(body, ">Linux<") {
		t.Errorf("expected overview card NOT to contain 'Linux' badge")
	}
	if !strings.Contains(body, "Delete") {
		t.Errorf("expected body to contain 'Delete' button")
	}

	// Verify detail page still displays BIOS and Linux
	detailReq := httptest.NewRequest(http.MethodGet, "/vms/dash-vm-1", nil)
	detailReq.Header.Set("Accept", "text/html")
	detailRec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(detailRec, detailReq)
	if detailRec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on /vms/dash-vm-1, got %d", detailRec.Code)
	}
	detailBody := detailRec.Body.String()
	if !strings.Contains(detailBody, "BIOS") {
		t.Errorf("expected detail body to contain 'BIOS'")
	}
	if !strings.Contains(detailBody, "Linux") {
		t.Errorf("expected detail body to contain 'Linux'")
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
	if strings.Contains(body, ">UEFI<") {
		t.Errorf("expected partial card NOT to contain 'UEFI' badge")
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
	if !strings.Contains(cardHTML, "status-running") {
		t.Errorf("expected cardHTML to contain 'status-running'")
	}
	if !strings.Contains(cardHTML, "Delete") {
		t.Errorf("expected cardHTML to contain 'Delete' action button")
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
	if !strings.Contains(stoppedCardHTML, "status-stopped") {
		t.Errorf("expected stoppedCardHTML to contain 'status-stopped'")
	}
	if !strings.Contains(stoppedCardHTML, "Delete") {
		t.Errorf("expected stoppedCardHTML to contain 'Delete' action button")
	}
	if strings.Contains(stoppedCardHTML, "PID:") {
		t.Errorf("expected stoppedCardHTML NOT to contain runtime stats when stopped")
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

func TestVMList_CreateButtonVisibility(t *testing.T) {
	srv := newTestServer(t)

	// 1. When 0 VMs exist:
	req := httptest.NewRequest(http.MethodGet, "/vms", nil)
	req.Header.Set("Accept", "text/html")
	rec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on /vms, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, `id="btn-create-vm" style="display: none;"`) {
		t.Errorf("expected header Create VM button to be hidden (style='display: none;') when 0 VMs exist, body: %s", body)
	}
	if !strings.Contains(body, "No Virtual Machines Configured") {
		t.Errorf("expected empty state when 0 VMs exist")
	}

	// 2. Create 1 VM:
	_, err := srv.vmMgr.CreateVM(vm.VMConfig{
		ID:         "test-vm-vis",
		Name:       "Visibility Test VM",
		CPUs:       1,
		MemoryMB:   512,
		DiskSize:   "10M",
		DiskFormat: "qcow2",
		Firmware:   "bios",
	})
	if err != nil {
		t.Fatalf("failed to create test VM: %v", err)
	}

	// 3. When 1 VM exists:
	req = httptest.NewRequest(http.MethodGet, "/vms", nil)
	req.Header.Set("Accept", "text/html")
	rec = httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on /vms, got %d", rec.Code)
	}

	bodyWithVM := rec.Body.String()
	if strings.Contains(bodyWithVM, `id="btn-create-vm" style="display: none;"`) {
		t.Errorf("expected header Create VM button to be visible when 1 VM exists, got: %s", bodyWithVM)
	}
	if !strings.Contains(bodyWithVM, `id="btn-create-vm"`) {
		t.Errorf("expected header Create VM button to be present when 1 VM exists")
	}
	if !strings.Contains(bodyWithVM, "Visibility Test VM") {
		t.Errorf("expected VM name to be in list")
	}
}

func TestStorage_PageRenderingWithDisksAndISOs(t *testing.T) {
	srv := newTestServer(t)

	// 1. Create a VM with a QCOW2 disk
	_, err := srv.vmMgr.CreateVM(vm.VMConfig{
		ID:         "storage-test-vm",
		Name:       "Storage Test VM",
		CPUs:       1,
		MemoryMB:   512,
		Disk:       "disk.qcow2",
		DiskSize:   "50M",
		DiskFormat: "qcow2",
		Firmware:   "bios",
	})
	if err != nil {
		t.Fatalf("failed to create test VM: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/storage", nil)
	rec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on /storage, got %d", rec.Code)
	}

	body := rec.Body.String()

	// Verify overview card rows
	if !strings.Contains(body, "Storage Root") {
		t.Errorf("expected overview card to contain 'Storage Root'")
	}
	if !strings.Contains(body, "Pool Usage") {
		t.Errorf("expected overview card to contain 'Pool Usage'")
	}
	if !strings.Contains(body, "ISO Images") {
		t.Errorf("expected overview card to contain 'ISO Images'")
	}
	if !strings.Contains(body, "Virtual Disks") {
		t.Errorf("expected overview card to contain 'Virtual Disks'")
	}

	// Verify Virtual Disks table headers and content
	if !strings.Contains(body, "Virtual Disks (QCOW2 &amp; RAW)") {
		t.Errorf("expected section title 'Virtual Disks (QCOW2 & RAW)'")
	}
	if !strings.Contains(body, "disk.qcow2") {
		t.Errorf("expected body to contain disk filename 'disk.qcow2'")
	}
	if !strings.Contains(body, "Storage Test VM") {
		t.Errorf("expected body to contain VM name 'Storage Test VM'")
	}
	if !strings.Contains(body, "qcow2") {
		t.Errorf("expected body to contain format 'qcow2'")
	}
	if !strings.Contains(body, "50 MB") {
		t.Errorf("expected body to contain virtual size '50 MB', got: %s", body)
	}

	// Verify Discovered ISO Images section
	if !strings.Contains(body, "Discovered ISO Images") {
		t.Errorf("expected section title 'Discovered ISO Images'")
	}
}

func TestStorage_UploadAndDownloadJSON(t *testing.T) {
	srv := newTestServer(t)

	// Test 1: Uploading a valid ISO via multipart with Accept: application/json
	var b bytes.Buffer
	w := multipart.NewWriter(&b)
	part, err := w.CreateFormFile("file", "upload-test.iso")
	if err != nil {
		t.Fatalf("failed to create form file: %v", err)
	}
	_, _ = part.Write([]byte("dummy-iso-content-header"))
	_ = w.Close()

	req := httptest.NewRequest(http.MethodPost, "/storage/upload", &b)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Accept", "application/json")
	rec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created on json upload, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	var uploadResp map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&uploadResp); err != nil {
		t.Fatalf("failed to decode upload response: %v", err)
	}
	if uploadResp["status"] != "ok" || uploadResp["name"] != "upload-test.iso" {
		t.Errorf("unexpected upload response: %v", uploadResp)
	}

	// Verify the file was saved
	if !srv.vmMgr.Storage().ISOExists("upload-test.iso") {
		t.Errorf("expected upload-test.iso to exist in storage pool")
	}

	// Test 2: Uploading an invalid file format (non-.iso) with Accept: application/json
	b.Reset()
	w = multipart.NewWriter(&b)
	part, err = w.CreateFormFile("file", "malicious.sh")
	if err != nil {
		t.Fatalf("failed to create form file: %v", err)
	}
	_, _ = part.Write([]byte("#!/bin/bash\necho hello"))
	_ = w.Close()

	req = httptest.NewRequest(http.MethodPost, "/storage/upload", &b)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Accept", "application/json")
	rec = httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request on invalid file extension, got %d", rec.Code)
	}

	var errResp map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&errResp); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}
	// Test 3: Uploading duplicate ISO returns 409 Conflict
	b.Reset()
	w = multipart.NewWriter(&b)
	part, err = w.CreateFormFile("file", "upload-test.iso")
	if err != nil {
		t.Fatalf("failed to create form file: %v", err)
	}
	_, _ = part.Write([]byte("duplicate-content"))
	_ = w.Close()

	req = httptest.NewRequest(http.MethodPost, "/storage/upload", &b)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Accept", "application/json")
	rec = httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 Conflict on duplicate ISO upload, got %d (body: %s)", rec.Code, rec.Body.String())
	}
}


