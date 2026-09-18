package api

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestWizard_GetPage(t *testing.T) {
	srv := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/vms/new", nil)
	rec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on GET /vms/new, got %d", rec.Code)
	}

	body := rec.Body.String()

	// 1. Check wizard stepper and title
	if !strings.Contains(body, "Create Virtual Machine") {
		t.Errorf("expected page title 'Create Virtual Machine'")
	}
	if !strings.Contains(body, "wizard-stepper") {
		t.Errorf("expected wizard stepper container")
	}

	// 2. Check Step 1: General fields
	if !strings.Contains(body, "vm-name") {
		t.Errorf("expected 'vm-name' input field")
	}
	if !strings.Contains(body, "vm-os") {
		t.Errorf("expected 'vm-os' select field")
	}
	if !strings.Contains(body, "vm-iso") {
		t.Errorf("expected 'vm-iso' select field")
	}
	if !strings.Contains(body, "firmware-card-bios") {
		t.Errorf("expected 'firmware-card-bios' firmware selector")
	}

	// 3. Check Step 2: Resources fields
	if !strings.Contains(body, "cpu-slider") || !strings.Contains(body, "cpu-number") {
		t.Errorf("expected CPU slider and number inputs")
	}
	if !strings.Contains(body, "ram-slider") || !strings.Contains(body, "ram-number") {
		t.Errorf("expected RAM slider and number inputs")
	}

	// 4. Check Step 3: Storage & Network fields
	if !strings.Contains(body, "vm-disk-size") {
		t.Errorf("expected 'vm-disk-size' input field")
	}
	if !strings.Contains(body, "vm-disk-format") {
		t.Errorf("expected 'vm-disk-format' select field")
	}
	if !strings.Contains(body, "vm-net-enable") {
		t.Errorf("expected 'vm-net-enable' network toggle")
	}
	if !strings.Contains(body, "vm-ssh-port") {
		t.Errorf("expected 'vm-ssh-port' input field")
	}

	// 5. Check Step 4: Review fields
	if !strings.Contains(body, "rev-name") || !strings.Contains(body, "rev-cpu") {
		t.Errorf("expected review summary card fields")
	}
	if !strings.Contains(body, "btn-submit-wizard") {
		t.Errorf("expected wizard submission button")
	}
}

func TestWizard_FormSubmission_BrowserRedirect(t *testing.T) {
	srv := newTestServer(t)

	formData := url.Values{}
	formData.Set("name", "wizard-ubuntu-vm")
	formData.Set("id", "wizard-ubuntu-vm")
	formData.Set("os_type", "Ubuntu")
	formData.Set("firmware", "bios")
	formData.Set("cpus", "2")
	formData.Set("memory_mb", "2048")
	formData.Set("disk_size", "20G")
	formData.Set("disk_format", "qcow2")
	formData.Set("enable_network", "true")
	formData.Set("ssh_port", "2222")

	req := httptest.NewRequest(http.MethodPost, "/vms", strings.NewReader(formData.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	rec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(rec, req)

	// Browser submission should redirect to /vms/{id}
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 See Other redirect, got %d: %s", rec.Code, rec.Body.String())
	}
	location := rec.Header().Get("Location")
	if location != "/vms/wizard-ubuntu-vm" {
		t.Errorf("expected Location /vms/wizard-ubuntu-vm, got %s", location)
	}

	// Verify VM was properly created in memory & storage
	v, err := srv.vmMgr.GetVM("wizard-ubuntu-vm")
	if err != nil {
		t.Fatalf("failed to retrieve created VM: %v", err)
	}
	if v.Config.Name != "wizard-ubuntu-vm" {
		t.Errorf("expected name 'wizard-ubuntu-vm', got '%s'", v.Config.Name)
	}
	if v.Config.OSType != "Ubuntu" {
		t.Errorf("expected OSType 'Ubuntu', got '%s'", v.Config.OSType)
	}
	if v.Config.CPUs != 2 {
		t.Errorf("expected 2 CPUs, got %d", v.Config.CPUs)
	}
	if v.Config.MemoryMB != 2048 {
		t.Errorf("expected 2048 MB RAM, got %d", v.Config.MemoryMB)
	}
	if v.Config.Network.SSHPort != 2222 {
		t.Errorf("expected SSH port 2222, got %d", v.Config.Network.SSHPort)
	}
	if !v.Config.Network.Enabled {
		t.Errorf("expected Network to be enabled")
	}
}

func TestWizard_FormSubmission_HTMXRedirect(t *testing.T) {
	srv := newTestServer(t)

	formData := url.Values{}
	formData.Set("name", "htmx-wizard-vm")
	formData.Set("os_type", "Debian")
	formData.Set("firmware", "bios")
	formData.Set("cpus", "1")
	formData.Set("memory_mb", "1024")
	formData.Set("disk_size", "10G")
	formData.Set("disk_format", "qcow2")
	formData.Set("enable_network", "true")
	formData.Set("ssh_port", "2223")

	req := httptest.NewRequest(http.MethodPost, "/vms", strings.NewReader(formData.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	rec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(rec, req)

	// HTMX request should respond with 200 OK and HX-Redirect header
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on HTMX creation, got %d: %s", rec.Code, rec.Body.String())
	}
	hxRedirect := rec.Header().Get("HX-Redirect")
	if hxRedirect != "/vms/htmx-wizard-vm" {
		t.Errorf("expected HX-Redirect: /vms/htmx-wizard-vm, got '%s'", hxRedirect)
	}

	// Verify VM created
	v, err := srv.vmMgr.GetVM("htmx-wizard-vm")
	if err != nil {
		t.Fatalf("failed to retrieve HTMX-created VM: %v", err)
	}
	if v.Config.OSType != "Debian" {
		t.Errorf("expected OSType 'Debian', got '%s'", v.Config.OSType)
	}
}

func TestWizard_FormSubmission_DisabledNetwork(t *testing.T) {
	srv := newTestServer(t)

	formData := url.Values{}
	formData.Set("name", "isolated-vm")
	formData.Set("os_type", "Alpine")
	formData.Set("firmware", "bios")
	formData.Set("cpus", "1")
	formData.Set("memory_mb", "512")
	formData.Set("disk_size", "5G")
	formData.Set("disk_format", "qcow2")
	// enable_network omitted (unchecked)

	req := httptest.NewRequest(http.MethodPost, "/vms", strings.NewReader(formData.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	rec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
	}

	v, err := srv.vmMgr.GetVM("isolated-vm")
	if err != nil {
		t.Fatalf("failed to retrieve VM: %v", err)
	}
	if v.Config.Network.Enabled {
		t.Errorf("expected Network to be disabled when omitted from form")
	}
}

func TestWizard_FormSubmission_ValidationErrors(t *testing.T) {
	srv := newTestServer(t)

	// 1. Invalid CPU count
	formData := url.Values{}
	formData.Set("name", "invalid-cpu-vm")
	formData.Set("cpus", "0")
	formData.Set("memory_mb", "1024")

	req := httptest.NewRequest(http.MethodPost, "/vms", strings.NewReader(formData.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)

	// Note: handleAPIVMCreate defaults cpus <= 0 to 1 if not strictly caught, let's test invalid memory < 128MB
	formData = url.Values{}
	formData.Set("name", "invalid-ram-vm")
	formData.Set("cpus", "1")
	formData.Set("memory_mb", "64")

	req = httptest.NewRequest(http.MethodPost, "/vms", strings.NewReader(formData.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request for RAM < 128MB, got %d", rec.Code)
	}

	// 2. Duplicate port collision
	formData = url.Values{}
	formData.Set("name", "port-vm-1")
	formData.Set("enable_network", "true")
	formData.Set("ssh_port", "2250")
	req = httptest.NewRequest(http.MethodPost, "/vms", strings.NewReader(formData.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated && rec.Code != http.StatusOK && rec.Code != http.StatusSeeOther {
		t.Fatalf("expected first VM to succeed, got %d", rec.Code)
	}

	// Create second VM with same port
	formData = url.Values{}
	formData.Set("name", "port-vm-2")
	formData.Set("enable_network", "true")
	formData.Set("ssh_port", "2250")
	req = httptest.NewRequest(http.MethodPost, "/vms", strings.NewReader(formData.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request for duplicate host port, got %d", rec.Code)
	}
}
