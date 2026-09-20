package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"tinyvm/internal/vm"
)

func TestAPI_ProxmoxControls(t *testing.T) {
	srv := newTestServer(t)

	// Create test ISO in storage
	isoName := "proxmox-test.iso"
	isoPath := filepath.Join(srv.vmMgr.Storage().ISODir(), isoName)
	if err := os.WriteFile(isoPath, []byte("fake-iso-bytes"), 0644); err != nil {
		t.Fatalf("failed to create test iso: %v", err)
	}

	// Create VM
	createBody, _ := json.Marshal(CreateVMRequest{
		ID:         "pve-ctrl-vm",
		Name:       "PVE Control VM",
		CPUs:       1,
		MemoryMB:   256,
		DiskSize:   "10M",
		DiskFormat: "qcow2",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/vms", bytes.NewReader(createBody))
	rec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("failed to create VM: %d, %s", rec.Code, rec.Body.String())
	}

	// 1. Attach ISO via POST /vms/pve-ctrl-vm/attach-iso
	form := url.Values{}
	form.Set("iso", isoName)
	req = httptest.NewRequest(http.MethodPost, "/vms/pve-ctrl-vm/attach-iso", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK && rec.Code != http.StatusSeeOther {
		t.Fatalf("expected 200/303 on attach-iso, got %d", rec.Code)
	}

	targetVM, err := srv.vmMgr.GetVM("pve-ctrl-vm")
	if err != nil || targetVM.Config.ISO != isoName {
		t.Fatalf("expected attached ISO %s, got %s", isoName, targetVM.Config.ISO)
	}

	// 2. Start VM
	req = httptest.NewRequest(http.MethodPost, "/api/v1/vms/pve-ctrl-vm/start", nil)
	rec = httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 on start, got %d", rec.Code)
	}
	time.Sleep(100 * time.Millisecond)

	// 3. Pause VM via POST /vms/pve-ctrl-vm/pause
	req = httptest.NewRequest(http.MethodPost, "/vms/pve-ctrl-vm/pause", nil)
	rec = httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK && rec.Code != http.StatusSeeOther {
		t.Fatalf("expected 200/303 on pause, got %d", rec.Code)
	}

	targetVM, _ = srv.vmMgr.GetVM("pve-ctrl-vm")
	if targetVM.Runtime.State != vm.StatePaused {
		t.Errorf("expected state %s, got %s", vm.StatePaused, targetVM.Runtime.State)
	}

	// 4. Resume VM via POST /vms/pve-ctrl-vm/resume
	req = httptest.NewRequest(http.MethodPost, "/vms/pve-ctrl-vm/resume", nil)
	rec = httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK && rec.Code != http.StatusSeeOther {
		t.Fatalf("expected 200/303 on resume, got %d", rec.Code)
	}

	targetVM, _ = srv.vmMgr.GetVM("pve-ctrl-vm")
	if targetVM.Runtime.State != vm.StateRunning {
		t.Errorf("expected state %s, got %s", vm.StateRunning, targetVM.Runtime.State)
	}

	// 5. Reset VM via POST /vms/pve-ctrl-vm/reset
	req = httptest.NewRequest(http.MethodPost, "/vms/pve-ctrl-vm/reset", nil)
	rec = httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK && rec.Code != http.StatusSeeOther {
		t.Fatalf("expected 200/303 on reset, got %d", rec.Code)
	}

	// 6. Stop VM
	req = httptest.NewRequest(http.MethodPost, "/api/v1/vms/pve-ctrl-vm/stop", nil)
	rec = httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 on stop, got %d", rec.Code)
	}

	// 7. Update VM Config via POST /vms/pve-ctrl-vm/config
	cfgForm := url.Values{}
	cfgForm.Set("cpus", "2")
	cfgForm.Set("memory_mb", "512")
	cfgForm.Set("boot_order", "c")
	cfgForm.Set("os_type", "Linux")
	cfgForm.Set("autostart", "true")
	req = httptest.NewRequest(http.MethodPost, "/vms/pve-ctrl-vm/config", strings.NewReader(cfgForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK && rec.Code != http.StatusSeeOther {
		t.Fatalf("expected 200/303 on config update, got %d", rec.Code)
	}

	targetVM, _ = srv.vmMgr.GetVM("pve-ctrl-vm")
	if targetVM.Config.CPUs != 2 || targetVM.Config.MemoryMB != 512 || targetVM.Config.BootOrder != "c" {
		t.Errorf("unexpected updated config: %+v", targetVM.Config)
	}

	// 8. Resize Disk via POST /vms/pve-ctrl-vm/resize-disk
	resizeForm := url.Values{}
	resizeForm.Set("size", "+10M")
	req = httptest.NewRequest(http.MethodPost, "/vms/pve-ctrl-vm/resize-disk", strings.NewReader(resizeForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK && rec.Code != http.StatusSeeOther {
		t.Fatalf("expected 200/303 on resize-disk, got %d", rec.Code)
	}

	// 9. Snapshots: Create, List, Rollback, Delete
	snapForm := url.Values{}
	snapForm.Set("name", "snap-test")
	snapForm.Set("description", "Snapshot 1 description")
	req = httptest.NewRequest(http.MethodPost, "/vms/pve-ctrl-vm/snapshots", strings.NewReader(snapForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK && rec.Code != http.StatusSeeOther {
		t.Fatalf("expected 200/303 on create snapshot, got %d: %s", rec.Code, rec.Body.String())
	}

	// List snapshots via API
	req = httptest.NewRequest(http.MethodGet, "/api/v1/vms/pve-ctrl-vm/snapshots", nil)
	rec = httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 on list snapshots, got %d", rec.Code)
	}

	var snapList []vm.SnapshotInfo
	if err := json.NewDecoder(rec.Body).Decode(&snapList); err != nil {
		t.Fatalf("failed to decode snapshot list: %v", err)
	}
	if len(snapList) != 1 || snapList[0].Name != "snap-test" {
		t.Errorf("expected 1 snapshot 'snap-test', got %+v", snapList)
	}

	// Rollback snapshot
	req = httptest.NewRequest(http.MethodPost, "/vms/pve-ctrl-vm/snapshots/snap-test/rollback", nil)
	rec = httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK && rec.Code != http.StatusSeeOther {
		t.Fatalf("expected 200/303 on rollback snapshot, got %d", rec.Code)
	}

	// Delete snapshot
	req = httptest.NewRequest(http.MethodPost, "/vms/pve-ctrl-vm/snapshots/snap-test/delete", nil)
	rec = httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK && rec.Code != http.StatusSeeOther {
		t.Fatalf("expected 200/303 on delete snapshot, got %d", rec.Code)
	}

	// 10. Clone VM via POST /vms/pve-ctrl-vm/clone
	cloneForm := url.Values{}
	cloneForm.Set("new_id", "pve-cloned-vm")
	cloneForm.Set("new_name", "Cloned PVE Instance")
	req = httptest.NewRequest(http.MethodPost, "/vms/pve-ctrl-vm/clone", strings.NewReader(cloneForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK && rec.Code != http.StatusSeeOther {
		t.Fatalf("expected 200/303 on clone vm, got %d", rec.Code)
	}

	clonedVM, err := srv.vmMgr.GetVM("pve-cloned-vm")
	if err != nil || clonedVM.Config.Name != "Cloned PVE Instance" {
		t.Fatalf("expected cloned VM 'Cloned PVE Instance', got %v", clonedVM)
	}

	// 11. View Logs endpoint
	req = httptest.NewRequest(http.MethodGet, "/vms/pve-ctrl-vm/logs", nil)
	rec = httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 on get logs, got %d", rec.Code)
	}

	// Clean up
	_ = srv.vmMgr.DeleteVM("pve-ctrl-vm")
	_ = srv.vmMgr.DeleteVM("pve-cloned-vm")
}
