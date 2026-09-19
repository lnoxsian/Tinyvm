package api

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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

func TestAPISOs_UploadAndDelete(t *testing.T) {
	srv := newTestServer(t)

	// 1. Upload an ISO using multipart form
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "debian-netinst.iso")
	if err != nil {
		t.Fatalf("failed to create form file: %v", err)
	}
	isoBytes := []byte("debian netinst simulated iso content")
	_, _ = part.Write(isoBytes)
	_ = writer.Close()

	uploadReq := httptest.NewRequest(http.MethodPost, "/api/v1/isos", &body)
	uploadReq.Header.Set("Content-Type", writer.FormDataContentType())
	uploadRec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(uploadRec, uploadReq)

	if uploadRec.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created on ISO upload, got %d: %s", uploadRec.Code, uploadRec.Body.String())
	}

	var resp ISOResponse
	if err := json.NewDecoder(uploadRec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode upload response: %v", err)
	}
	if resp.Name != "debian-netinst.iso" || resp.SizeBytes != int64(len(isoBytes)) {
		t.Errorf("unexpected ISO upload response: %+v", resp)
	}

	// 2. Duplicate upload should return 409 Conflict
	var dupBody bytes.Buffer
	dupWriter := multipart.NewWriter(&dupBody)
	dupPart, _ := dupWriter.CreateFormFile("file", "debian-netinst.iso")
	_, _ = dupPart.Write(isoBytes)
	_ = dupWriter.Close()

	dupReq := httptest.NewRequest(http.MethodPost, "/api/v1/isos", &dupBody)
	dupReq.Header.Set("Content-Type", dupWriter.FormDataContentType())
	dupRec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(dupRec, dupReq)

	if dupRec.Code != http.StatusConflict {
		t.Errorf("expected 409 Conflict for duplicate ISO, got %d", dupRec.Code)
	}

	// 3. Delete the uploaded ISO
	delReq := httptest.NewRequest(http.MethodDelete, "/api/v1/isos/debian-netinst.iso", nil)
	delRec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(delRec, delReq)

	if delRec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on ISO delete, got %d: %s", delRec.Code, delRec.Body.String())
	}

	// 4. Delete again should return 404
	delAgainReq := httptest.NewRequest(http.MethodDelete, "/api/v1/isos/debian-netinst.iso", nil)
	delAgainRec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(delAgainRec, delAgainReq)

	if delAgainRec.Code != http.StatusNotFound {
		t.Errorf("expected 404 Not Found on deleted ISO, got %d", delAgainRec.Code)
	}

	// 5. Upload another ISO and test HTMX deletion response (empty body + HX-Trigger)
	bodyHtmx := &bytes.Buffer{}
	writerHtmx := multipart.NewWriter(bodyHtmx)
	partHtmx, _ := writerHtmx.CreateFormFile("file", "htmx-test.iso")
	_, _ = partHtmx.Write([]byte("dummy iso contents"))
	_ = writerHtmx.Close()

	upHtmxReq := httptest.NewRequest(http.MethodPost, "/api/v1/isos", bodyHtmx)
	upHtmxReq.Header.Set("Content-Type", writerHtmx.FormDataContentType())
	upHtmxRec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(upHtmxRec, upHtmxReq)
	if upHtmxRec.Code != http.StatusCreated {
		t.Fatalf("failed uploading second ISO: %d", upHtmxRec.Code)
	}

	htmxDelReq := httptest.NewRequest(http.MethodDelete, "/api/v1/isos/htmx-test.iso", nil)
	htmxDelReq.Header.Set("HX-Request", "true")
	htmxDelRec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(htmxDelRec, htmxDelReq)

	if htmxDelRec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on HTMX ISO delete, got %d", htmxDelRec.Code)
	}
	if htmxDelRec.Body.Len() != 0 {
		t.Errorf("expected empty body for HTMX swap so row is removed, got %q", htmxDelRec.Body.String())
	}
	triggerHdr := htmxDelRec.Header().Get("HX-Trigger")
	if triggerHdr != "iso-updated" {
		t.Errorf("expected HX-Trigger: iso-updated, got %q", triggerHdr)
	}
}

func TestAPIVMs_QueryAndBodyActions(t *testing.T) {
	srv := newTestServer(t)

	// Create test VM
	body, _ := json.Marshal(CreateVMRequest{
		ID:         "action-vm",
		Name:       "Action VM",
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

	// 1. POST /start?id=action-vm (query parameter action)
	req = httptest.NewRequest(http.MethodPost, "/start?id=action-vm", nil)
	rec = httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 on /start?id=..., got %d: %s", rec.Code, rec.Body.String())
	}

	// 2. GET /status?id=action-vm (query parameter status)
	req = httptest.NewRequest(http.MethodGet, "/status?id=action-vm", nil)
	rec = httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 on /status?id=..., got %d", rec.Code)
	}

	// 3. POST /stop with JSON body {"id": "action-vm"}
	stopBody, _ := json.Marshal(VMActionRequest{ID: "action-vm"})
	req = httptest.NewRequest(http.MethodPost, "/stop", bytes.NewReader(stopBody))
	rec = httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 on /stop with body, got %d: %s", rec.Code, rec.Body.String())
	}

	// Clean up
	req = httptest.NewRequest(http.MethodDelete, "/api/v1/vms/action-vm", nil)
	rec = httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 on delete, got %d", rec.Code)
	}
}

func TestAPI_AuthMiddleware(t *testing.T) {
	srv := newTestServer(t)
	srv.cfg.APIToken = "secret-token-123"

	// 1. Health check works without token
	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	rec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("health check should bypass auth, got %d", rec.Code)
	}

	// 2. /api/v1/vms without token -> 401 Unauthorized
	req = httptest.NewRequest(http.MethodGet, "/api/v1/vms", nil)
	rec = httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 Unauthorized, got %d", rec.Code)
	}

	// 3. /api/v1/vms with invalid Bearer token -> 401 Unauthorized
	req = httptest.NewRequest(http.MethodGet, "/api/v1/vms", nil)
	req.Header.Set("Authorization", "Bearer wrong-token")
	rec = httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for wrong token, got %d", rec.Code)
	}

	// 4. /api/v1/vms with valid Bearer token -> 200 OK
	req = httptest.NewRequest(http.MethodGet, "/api/v1/vms", nil)
	req.Header.Set("Authorization", "Bearer secret-token-123")
	rec = httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 with valid bearer token, got %d", rec.Code)
	}

	// 5. /api/v1/vms with valid query parameter token -> 200 OK
	req = httptest.NewRequest(http.MethodGet, "/api/v1/vms?token=secret-token-123", nil)
	rec = httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 with query token, got %d", rec.Code)
	}
}

func TestAPI_SecurityHeaders(t *testing.T) {
	srv := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	rec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("expected X-Content-Type-Options: nosniff")
	}
	if rec.Header().Get("X-Frame-Options") != "SAMEORIGIN" {
		t.Errorf("expected X-Frame-Options: SAMEORIGIN")
	}
}

func TestAPIVMs_FormURLEncodedCreate(t *testing.T) {
	srv := newTestServer(t)

	formData := "name=form-vm&cpu=2&ram=1024&disk_size=10M&disk_format=qcow2&firmware=bios"
	req := httptest.NewRequest(http.MethodPost, "/api/v1/vms", strings.NewReader(formData))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created on form POST, got %d: %s", rec.Code, rec.Body.String())
	}

	var created VMResponse
	if err := json.NewDecoder(rec.Body).Decode(&created); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if created.ID != "form-vm" || created.CPUs != 2 || created.MemoryMB != 1024 {
		t.Errorf("unexpected created VM from form: %+v", created)
	}
}

func TestAPIVMs_ForceDelete(t *testing.T) {
	srv := newTestServer(t)

	// Create and start VM
	body, _ := json.Marshal(CreateVMRequest{
		ID:         "force-del-vm",
		Name:       "Force Del VM",
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

	req = httptest.NewRequest(http.MethodPost, "/api/v1/vms/force-del-vm/start", nil)
	rec = httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("failed to start VM: %d", rec.Code)
	}

	// Normal delete fails with 409 Conflict
	req = httptest.NewRequest(http.MethodDelete, "/api/v1/vms/force-del-vm", nil)
	rec = httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Errorf("expected 409 Conflict when deleting running VM without force, got %d", rec.Code)
	}

	// Force delete succeeds with 200 OK
	req = httptest.NewRequest(http.MethodDelete, "/api/v1/vms/force-del-vm?force=true", nil)
	rec = httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 OK on force delete, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestAPIVMs_StatusJSONBodyAndTopLevelFields(t *testing.T) {
	srv := newTestServer(t)

	body, _ := json.Marshal(CreateVMRequest{
		ID:         "status-test-vm",
		Name:       "Status Test VM",
		CPUs:       2,
		MemoryMB:   512,
		DiskSize:   "10M",
		DiskFormat: "qcow2",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/vms", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("failed to create VM: %d", rec.Code)
	}

	// Query status via POST /status with JSON body
	queryBody, _ := json.Marshal(VMActionRequest{ID: "status-test-vm"})
	req = httptest.NewRequest(http.MethodPost, "/status", bytes.NewReader(queryBody))
	rec = httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 on POST /status with JSON body, got %d: %s", rec.Code, rec.Body.String())
	}

	var statusResp map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&statusResp); err != nil {
		t.Fatalf("failed to decode status: %v", err)
	}

	// Verify top-level Section 24 fields
	if statusResp["id"] != "status-test-vm" {
		t.Errorf("expected top-level id 'status-test-vm', got %v", statusResp["id"])
	}
	if statusResp["status"] != "stopped" {
		t.Errorf("expected top-level status 'stopped', got %v", statusResp["status"])
	}
	if statusResp["cpus"] != float64(2) {
		t.Errorf("expected top-level cpus 2, got %v", statusResp["cpus"])
	}
	if statusResp["memory_mb"] != float64(512) {
		t.Errorf("expected top-level memory_mb 512, got %v", statusResp["memory_mb"])
	}
}

