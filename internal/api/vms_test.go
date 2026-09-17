package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestAPIVMs_CRUD(t *testing.T) {
	srv := newTestServer(t)

	// 1. GET /api/v1/vms (initially empty)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/vms", nil)
	rec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var vms []VMResponse
	if err := json.NewDecoder(rec.Body).Decode(&vms); err != nil {
		t.Fatalf("failed to decode JSON: %v", err)
	}
	if len(vms) != 0 {
		t.Errorf("expected 0 VMs, got %d", len(vms))
	}

	// 2. POST /api/v1/vms with malformed JSON
	req = httptest.NewRequest(http.MethodPost, "/api/v1/vms", bytes.NewReader([]byte("not-json")))
	rec = httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for malformed json, got %d", rec.Code)
	}

	// 3. POST /api/v1/vms with invalid VM ID
	invalidBody, _ := json.Marshal(CreateVMRequest{
		ID:       "invalid id with spaces",
		CPUs:     1,
		MemoryMB: 512,
	})
	req = httptest.NewRequest(http.MethodPost, "/api/v1/vms", bytes.NewReader(invalidBody))
	rec = httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid ID, got %d", rec.Code)
	}

	// 4. POST /api/v1/vms valid creation
	validBody, _ := json.Marshal(CreateVMRequest{
		ID:         "test-vm-1",
		Name:       "Test VM One",
		CPUs:       2,
		MemoryMB:   512,
		DiskSize:   "10M",
		DiskFormat: "qcow2",
		Firmware:   "bios",
	})
	req = httptest.NewRequest(http.MethodPost, "/api/v1/vms", bytes.NewReader(validBody))
	rec = httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d, body: %s", rec.Code, rec.Body.String())
	}
	loc := rec.Header().Get("Location")
	if loc != "/api/v1/vms/test-vm-1" {
		t.Errorf("unexpected Location header: %s", loc)
	}
	var created VMResponse
	if err := json.NewDecoder(rec.Body).Decode(&created); err != nil {
		t.Fatalf("failed to decode created VM response: %v", err)
	}
	if created.ID != "test-vm-1" || created.CPUs != 2 || created.MemoryMB != 512 {
		t.Errorf("unexpected created VM response: %+v", created)
	}

	// 5. POST /api/v1/vms duplicate creation -> 409 Conflict
	req = httptest.NewRequest(http.MethodPost, "/api/v1/vms", bytes.NewReader(validBody))
	rec = httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Errorf("expected 409 Conflict for duplicate VM, got %d", rec.Code)
	}

	// 6. GET /api/v1/vms now returns 1 VM
	req = httptest.NewRequest(http.MethodGet, "/api/v1/vms", nil)
	rec = httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)
	if err := json.NewDecoder(rec.Body).Decode(&vms); err != nil {
		t.Fatalf("failed to decode vms list: %v", err)
	}
	if len(vms) != 1 || vms[0].ID != "test-vm-1" {
		t.Errorf("expected 1 VM 'test-vm-1', got %+v", vms)
	}

	// 7. GET /api/v1/vms/{id} existing VM
	req = httptest.NewRequest(http.MethodGet, "/api/v1/vms/test-vm-1", nil)
	rec = httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	var single VMResponse
	if err := json.NewDecoder(rec.Body).Decode(&single); err != nil {
		t.Fatalf("failed to decode single VM: %v", err)
	}
	if single.ID != "test-vm-1" {
		t.Errorf("expected ID 'test-vm-1', got %s", single.ID)
	}

	// 8. GET /api/v1/vms/{id} non-existent VM -> 404
	req = httptest.NewRequest(http.MethodGet, "/api/v1/vms/does-not-exist", nil)
	rec = httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}

	// 9. DELETE /api/v1/vms/{id} existing VM -> 200
	req = httptest.NewRequest(http.MethodDelete, "/api/v1/vms/test-vm-1", nil)
	rec = httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 on delete, got %d", rec.Code)
	}

	// 10. DELETE /api/v1/vms/{id} non-existent VM -> 404
	req = httptest.NewRequest(http.MethodDelete, "/api/v1/vms/test-vm-1", nil)
	rec = httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404 on deleting already deleted VM, got %d", rec.Code)
	}
}

func TestAPIVMs_Lifecycle(t *testing.T) {
	srv := newTestServer(t)

	// Create test VM
	body, _ := json.Marshal(CreateVMRequest{
		ID:         "lifecycle-vm",
		Name:       "Lifecycle VM",
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

	// 1. POST /api/v1/vms/lifecycle-vm/start
	req = httptest.NewRequest(http.MethodPost, "/api/v1/vms/lifecycle-vm/start", nil)
	rec = httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for start, got %d: %s", rec.Code, rec.Body.String())
	}

	// 2. Start again -> 409 Conflict
	req = httptest.NewRequest(http.MethodPost, "/api/v1/vms/lifecycle-vm/start", nil)
	rec = httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Errorf("expected 409 for starting already running VM, got %d", rec.Code)
	}

	// 3. DELETE while running -> 409 Conflict
	req = httptest.NewRequest(http.MethodDelete, "/api/v1/vms/lifecycle-vm", nil)
	rec = httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Errorf("expected 409 when deleting running VM, got %d", rec.Code)
	}

	// 4. GET /api/v1/vms/lifecycle-vm/status
	req = httptest.NewRequest(http.MethodGet, "/api/v1/vms/lifecycle-vm/status", nil)
	rec = httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 for status, got %d", rec.Code)
	}
	var statusResp map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&statusResp); err != nil {
		t.Fatalf("failed to decode status: %v", err)
	}
	runtimeMap, ok := statusResp["runtime"].(map[string]any)
	if !ok || runtimeMap["state"] != "running" {
		t.Errorf("expected status state running, got %v", statusResp["runtime"])
	}

	// 5. POST /api/v1/vms/lifecycle-vm/restart
	req = httptest.NewRequest(http.MethodPost, "/api/v1/vms/lifecycle-vm/restart", nil)
	rec = httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 for restart, got %d", rec.Code)
	}

	// 6. POST /api/v1/vms/lifecycle-vm/quit (QMP clean quit)
	req = httptest.NewRequest(http.MethodPost, "/api/v1/vms/lifecycle-vm/quit", nil)
	rec = httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 for quit, got %d: %s", rec.Code, rec.Body.String())
	}

	// 7. Stop stopped VM -> 409 Conflict
	req = httptest.NewRequest(http.MethodPost, "/api/v1/vms/lifecycle-vm/stop", nil)
	rec = httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Errorf("expected 409 stopping already stopped VM, got %d", rec.Code)
	}

	// 8. Start and force Stop
	req = httptest.NewRequest(http.MethodPost, "/api/v1/vms/lifecycle-vm/start", nil)
	rec = httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for start, got %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "/api/v1/vms/lifecycle-vm/stop", nil)
	rec = httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 for stop, got %d", rec.Code)
	}

	// Clean up VM
	req = httptest.NewRequest(http.MethodDelete, "/api/v1/vms/lifecycle-vm", nil)
	rec = httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 on cleanup delete, got %d", rec.Code)
	}
}

func TestAPIVMs_DirectShorthandsAndContentNegotiation(t *testing.T) {
	srv := newTestServer(t)

	// Direct shorthand POST /vms
	body, _ := json.Marshal(CreateVMRequest{
		ID:         "shorthand-vm",
		Name:       "Shorthand VM",
		CPUs:       1,
		MemoryMB:   256,
		DiskSize:   "10M",
		DiskFormat: "qcow2",
	})
	req := httptest.NewRequest(http.MethodPost, "/vms", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created on POST /vms, got %d", rec.Code)
	}

	// Content negotiation on GET /vms with Accept: application/json
	req = httptest.NewRequest(http.MethodGet, "/vms", nil)
	req.Header.Set("Accept", "application/json")
	rec = httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var list []VMResponse
	if err := json.NewDecoder(rec.Body).Decode(&list); err != nil {
		t.Fatalf("expected JSON response for Accept: application/json: %v", err)
	}
	if len(list) != 1 || list[0].ID != "shorthand-vm" {
		t.Errorf("unexpected list response: %+v", list)
	}

	// Direct shorthand GET /vms/{id}/status
	req = httptest.NewRequest(http.MethodGet, "/vms/shorthand-vm/status", nil)
	rec = httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 on GET /vms/shorthand-vm/status, got %d", rec.Code)
	}

	// Direct shorthand DELETE /vms/{id}
	req = httptest.NewRequest(http.MethodDelete, "/vms/shorthand-vm", nil)
	rec = httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 on DELETE /vms/shorthand-vm, got %d", rec.Code)
	}
}

func TestAPISOs_List(t *testing.T) {
	srv := newTestServer(t)

	// Create a dummy ISO file in the storage ISO directory
	isoDir := srv.vmMgr.Storage().ISODir()
	dummyISO := filepath.Join(isoDir, "test-distro.iso")
	if err := os.WriteFile(dummyISO, []byte("fake-iso-content-bytes"), 0644); err != nil {
		t.Fatalf("failed to create dummy ISO: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/isos", nil)
	rec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for ISO list, got %d", rec.Code)
	}

	var isos []ISOResponse
	if err := json.NewDecoder(rec.Body).Decode(&isos); err != nil {
		t.Fatalf("failed to decode ISO list: %v", err)
	}

	if len(isos) != 1 || isos[0].Name != "test-distro.iso" {
		t.Errorf("unexpected ISO list: %+v", isos)
	}
	if isos[0].SizeBytes <= 0 {
		t.Errorf("expected positive ISO size, got %d", isos[0].SizeBytes)
	}
}
