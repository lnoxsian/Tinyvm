package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"tinyvm/internal/host"
	"tinyvm/internal/storage"
	"tinyvm/internal/vm"
)

// CreateVMRequest represents the JSON or form payload to create a new VM.
type CreateVMRequest struct {
	ID         string           `json:"id"`
	Name       string           `json:"name"`
	OSType     string           `json:"os_type,omitempty"`
	OS         string           `json:"os,omitempty"`
	CPUs       int              `json:"cpus"`
	CPU        int              `json:"cpu,omitempty"`
	MemoryMB   int              `json:"memory_mb"`
	Memory     int              `json:"memory,omitempty"`
	RAM        int              `json:"ram,omitempty"`
	Disk       string           `json:"disk,omitempty"`
	DiskFormat string           `json:"disk_format,omitempty"`
	DiskSize   string           `json:"disk_size,omitempty"`
	ISO        string           `json:"iso,omitempty"`
	Firmware   string           `json:"firmware,omitempty"`
	Network    vm.NetworkConfig `json:"network,omitempty"`
}

// VMActionRequest represents optional payload for action endpoints.
type VMActionRequest struct {
	ID string `json:"id"`
}

// VMResponse represents a VM in API responses.
type VMResponse struct {
	ID         string           `json:"id"`
	Name       string           `json:"name"`
	OSType     string           `json:"os_type,omitempty"`
	Status     string           `json:"status"`
	CPUs       int              `json:"cpus"`
	MemoryMB   int              `json:"memory_mb"`
	Disk       string           `json:"disk,omitempty"`
	DiskSize   string           `json:"disk_size,omitempty"`
	DiskFormat string           `json:"disk_format,omitempty"`
	ISO        string           `json:"iso,omitempty"`
	Firmware   string           `json:"firmware"`
	Network    vm.NetworkConfig `json:"network"`
	PID        int              `json:"pid,omitempty"`
	StartedAt  *time.Time       `json:"started_at,omitempty"`
}

// ActionResponse represents standard lifecycle action response.
type ActionResponse struct {
	Message string `json:"message"`
	ID      string `json:"id"`
	Status  string `json:"status,omitempty"`
}

// ISOResponse represents an ISO image in the storage pool.
type ISOResponse struct {
	Name      string `json:"name"`
	SizeBytes int64  `json:"size_bytes"`
}

func toVMResponse(v *vm.VM) VMResponse {
	resp := VMResponse{
		ID:         v.Config.ID,
		Name:       v.Config.Name,
		OSType:     v.Config.OSType,
		Status:     string(v.Runtime.State),
		CPUs:       v.Config.CPUs,
		MemoryMB:   v.Config.MemoryMB,
		Disk:       v.Config.Disk,
		DiskSize:   v.Config.DiskSize,
		DiskFormat: v.Config.DiskFormat,
		ISO:        v.Config.ISO,
		Firmware:   v.Config.Firmware,
		Network:    v.Config.Network,
		PID:        v.Runtime.PID,
	}
	if !v.Runtime.StartedAt.IsZero() {
		resp.StartedAt = &v.Runtime.StartedAt
	}
	return resp
}

// extractVMID retrieves VM ID from URL path, query params, form value, or JSON body.
func (s *Server) extractVMID(r *http.Request) string {
	if id := r.PathValue("id"); id != "" {
		return id
	}
	if id := r.URL.Query().Get("id"); id != "" {
		return id
	}
	if id := r.FormValue("id"); id != "" {
		return id
	}
	return ""
}

// handleAPIVMsList handles GET /api/v1/vms
func (s *Server) handleAPIVMsList(w http.ResponseWriter, r *http.Request) {
	if s.vmMgr == nil {
		WriteJSONError(w, http.StatusInternalServerError, ErrCodeInternal, "VM manager not initialized")
		return
	}

	rawList := s.vmMgr.ListVMs()
	resp := make([]VMResponse, 0, len(rawList))
	for _, v := range rawList {
		resp = append(resp, toVMResponse(v))
	}

	writeJSON(w, http.StatusOK, resp)
}

// handleAPIVMCreate handles POST /api/v1/vms and POST /vms
func (s *Server) handleAPIVMCreate(w http.ResponseWriter, r *http.Request) {
	if s.vmMgr == nil {
		WriteJSONError(w, http.StatusInternalServerError, ErrCodeInternal, "VM manager not initialized")
		return
	}

	contentType := r.Header.Get("Content-Type")
	var req CreateVMRequest

	isForm := strings.HasPrefix(contentType, "application/x-www-form-urlencoded") || strings.HasPrefix(contentType, "multipart/form-data")
	if isForm {
		if err := r.ParseForm(); err != nil {
			WriteJSONError(w, http.StatusBadRequest, ErrCodeInvalidInput, "failed to parse form data: "+err.Error())
			return
		}
		req.ID = strings.TrimSpace(r.FormValue("id"))
		req.Name = strings.TrimSpace(r.FormValue("name"))
		req.OSType = strings.TrimSpace(r.FormValue("os_type"))
		if req.OSType == "" {
			req.OSType = strings.TrimSpace(r.FormValue("os"))
		}
		if c, err := strconv.Atoi(r.FormValue("cpus")); err == nil && c > 0 {
			req.CPUs = c
		} else if c, err := strconv.Atoi(r.FormValue("cpu")); err == nil && c > 0 {
			req.CPUs = c
		}
		if m, err := strconv.Atoi(r.FormValue("memory_mb")); err == nil && m > 0 {
			req.MemoryMB = m
		} else if m, err := strconv.Atoi(r.FormValue("memory")); err == nil && m > 0 {
			req.MemoryMB = m
		} else if m, err := strconv.Atoi(r.FormValue("ram")); err == nil && m > 0 {
			req.MemoryMB = m
		}
		req.Disk = strings.TrimSpace(r.FormValue("disk"))
		req.DiskFormat = strings.TrimSpace(r.FormValue("disk_format"))
		req.DiskSize = strings.TrimSpace(r.FormValue("disk_size"))
		req.ISO = strings.TrimSpace(r.FormValue("iso"))
		req.Firmware = strings.TrimSpace(r.FormValue("firmware"))
		if p, err := strconv.Atoi(r.FormValue("ssh_port")); err == nil && p > 0 {
			req.Network.SSHPort = p
		}
		if r.Form.Has("enable_network") {
			v := r.FormValue("enable_network")
			req.Network.Enabled = (v == "true" || v == "on" || v == "1")
		} else {
			// In an HTML form submission, unchecked checkbox sends nothing
			req.Network.Enabled = false
		}
		if req.Network.Enabled {
			req.Network.Mode = "user"
		}
	} else {
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MB limit
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			WriteJSONError(w, http.StatusBadRequest, ErrCodeInvalidInput, "malformed request payload: "+err.Error())
			return
		}
		if req.OSType == "" && req.OS != "" {
			req.OSType = req.OS
		}
	}

	if req.ID == "" && req.Name != "" {
		req.ID = req.Name
	}
	if req.Name == "" {
		req.Name = req.ID
	}
	if req.CPUs == 0 && req.CPU > 0 {
		req.CPUs = req.CPU
	}
	if req.CPUs <= 0 {
		req.CPUs = 1
	}
	if req.MemoryMB == 0 {
		if req.Memory > 0 {
			req.MemoryMB = req.Memory
		} else if req.RAM > 0 {
			req.MemoryMB = req.RAM
		} else {
			req.MemoryMB = 1024
		}
	}
	if req.DiskFormat == "" {
		req.DiskFormat = "qcow2"
	}
	if req.DiskSize == "" {
		req.DiskSize = "10G"
	}
	if req.Firmware == "" {
		req.Firmware = "bios"
	}
	if !isForm && !req.Network.Enabled && req.Network.SSHPort == 0 && len(req.Network.Ports) == 0 {
		req.Network.Enabled = true
		req.Network.Mode = "user"
	}
	if req.Network.Enabled && req.Network.Mode == "" {
		req.Network.Mode = "user"
	}

	cfg := vm.VMConfig{
		ID:         req.ID,
		Name:       req.Name,
		OSType:     req.OSType,
		CPUs:       req.CPUs,
		MemoryMB:   req.MemoryMB,
		Disk:       req.Disk,
		DiskFormat: req.DiskFormat,
		DiskSize:   req.DiskSize,
		ISO:        req.ISO,
		Firmware:   req.Firmware,
		Network:    req.Network,
	}

	created, err := s.vmMgr.CreateVM(cfg)
	if err != nil {
		if errors.Is(err, vm.ErrVMAlreadyExists) {
			WriteJSONError(w, http.StatusConflict, ErrCodeConflict, err.Error())
			return
		}
		if errors.Is(err, host.ErrUEFINotSupported) {
			WriteJSONError(w, http.StatusBadRequest, ErrCodeInvalidInput, "UEFI firmware not supported or not installed on host")
			return
		}
		// Check for validation errors
		if errors.Is(err, vm.ErrInvalidVMName) ||
			errors.Is(err, vm.ErrInvalidCPUs) ||
			errors.Is(err, vm.ErrInvalidMemory) ||
			errors.Is(err, vm.ErrInvalidPort) ||
			errors.Is(err, vm.ErrDuplicatePort) ||
			errors.Is(err, vm.ErrInvalidDiskName) ||
			errors.Is(err, storage.ErrInvalidVMID) ||
			errors.Is(err, storage.ErrInvalidDiskFormat) ||
			errors.Is(err, storage.ErrInvalidDiskSize) ||
			errors.Is(err, storage.ErrInvalidISOName) {
			WriteJSONError(w, http.StatusBadRequest, ErrCodeInvalidInput, err.Error())
			return
		}
		s.logger.Error("Failed to create VM", "id", cfg.ID, "err", err)
		WriteJSONError(w, http.StatusInternalServerError, ErrCodeInternal, err.Error())
		return
	}

	s.logger.Info("Created virtual machine", "id", created.Config.ID, "name", created.Config.Name)

	// HTMX client-side redirection
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Redirect", "/vms/"+created.Config.ID)
		w.WriteHeader(http.StatusOK)
		return
	}

	// Browser form submission redirection
	if strings.Contains(r.Header.Get("Accept"), "text/html") && !strings.Contains(r.Header.Get("Accept"), "application/json") {
		http.Redirect(w, r, "/vms/"+created.Config.ID, http.StatusSeeOther)
		return
	}

	w.Header().Set("Location", fmt.Sprintf("/api/v1/vms/%s", created.Config.ID))
	writeJSON(w, http.StatusCreated, toVMResponse(created))
}

// handleAPIVMGet handles GET /api/v1/vms/{id}
func (s *Server) handleAPIVMGet(w http.ResponseWriter, r *http.Request) {
	if s.vmMgr == nil {
		WriteJSONError(w, http.StatusInternalServerError, ErrCodeInternal, "VM manager not initialized")
		return
	}

	id := s.extractVMID(r)
	if id == "" {
		WriteJSONError(w, http.StatusBadRequest, ErrCodeInvalidInput, "missing VM ID parameter")
		return
	}

	v, err := s.vmMgr.GetVM(id)
	if err != nil {
		if errors.Is(err, vm.ErrVMNotFoundInMgr) {
			WriteJSONError(w, http.StatusNotFound, ErrCodeNotFound, fmt.Sprintf("VM '%s' not found", id))
			return
		}
		WriteJSONError(w, http.StatusInternalServerError, ErrCodeInternal, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, toVMResponse(v))
}

// handleAPIVMDelete handles DELETE /api/v1/vms/{id}
func (s *Server) handleAPIVMDelete(w http.ResponseWriter, r *http.Request) {
	if s.vmMgr == nil {
		WriteJSONError(w, http.StatusInternalServerError, ErrCodeInternal, "VM manager not initialized")
		return
	}

	id := s.extractVMID(r)
	if id == "" {
		WriteJSONError(w, http.StatusBadRequest, ErrCodeInvalidInput, "missing VM ID parameter")
		return
	}

	force := r.URL.Query().Get("force") == "true" || r.URL.Query().Get("force") == "1"
	if force && s.vmMgr.IsVMRunning(id) {
		s.logger.Info("Force-stopping VM before deletion", "id", id)
		if err := s.vmMgr.StopVM(id); err != nil {
			s.logger.Warn("Failed to force stop VM before deletion", "id", id, "err", err)
		}
	}

	if err := s.vmMgr.DeleteVM(id); err != nil {
		if errors.Is(err, vm.ErrVMNotFoundInMgr) {
			WriteJSONError(w, http.StatusNotFound, ErrCodeNotFound, fmt.Sprintf("VM '%s' not found", id))
			return
		}
		if errors.Is(err, vm.ErrVMAlreadyRunning) {
			WriteJSONError(w, http.StatusConflict, ErrCodeConflict, "cannot delete running VM; stop it first or pass ?force=true")
			return
		}
		s.logger.Error("Failed to delete VM", "id", id, "err", err)
		WriteJSONError(w, http.StatusInternalServerError, ErrCodeInternal, err.Error())
		return
	}

	s.logger.Info("Deleted virtual machine", "id", id)

	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Trigger", "vm-updated")
		w.WriteHeader(http.StatusOK)
		return
	}

	writeJSON(w, http.StatusOK, ActionResponse{
		Message: "VM deleted successfully",
		ID:      id,
		Status:  "deleted",
	})
}

// handleAPIVMStart handles POST /api/v1/vms/{id}/start and /start
func (s *Server) handleAPIVMStart(w http.ResponseWriter, r *http.Request) {
	if s.vmMgr == nil {
		WriteJSONError(w, http.StatusInternalServerError, ErrCodeInternal, "VM manager not initialized")
		return
	}

	id := s.extractVMID(r)
	if id == "" && r.Body != nil {
		var act VMActionRequest
		_ = json.NewDecoder(r.Body).Decode(&act)
		id = act.ID
	}

	if id == "" {
		WriteJSONError(w, http.StatusBadRequest, ErrCodeInvalidInput, "missing VM ID parameter")
		return
	}

	if err := s.vmMgr.StartVM(id); err != nil {
		if errors.Is(err, vm.ErrVMNotFoundInMgr) {
			WriteJSONError(w, http.StatusNotFound, ErrCodeNotFound, fmt.Sprintf("VM '%s' not found", id))
			return
		}
		if errors.Is(err, vm.ErrVMAlreadyRunning) {
			WriteJSONError(w, http.StatusConflict, ErrCodeConflict, "VM is already running")
			return
		}
		if strings.Contains(err.Error(), "QEMU launcher is not configured") ||
			strings.Contains(err.Error(), "qemu-system-x86_64 not found") {
			WriteJSONError(w, http.StatusServiceUnavailable, ErrCodeKVMUnavailable, err.Error())
			return
		}
		s.logger.Error("Failed to start VM", "id", id, "err", err)
		WriteJSONError(w, http.StatusInternalServerError, ErrCodeInternal, err.Error())
		return
	}

	s.logger.Info("Started virtual machine", "id", id)

	if r.Header.Get("HX-Request") == "true" {
		if cardView, err := s.getSingleVMCardView(id); err == nil {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("HX-Trigger", "vm-updated")
			_ = s.templates["dashboard"].ExecuteTemplate(w, "vm-card", cardView)
			return
		}
	}

	writeJSON(w, http.StatusOK, ActionResponse{
		Message: "VM started successfully",
		ID:      id,
		Status:  "running",
	})
}

// handleAPIVMShutdown handles POST /api/v1/vms/{id}/shutdown and /shutdown
func (s *Server) handleAPIVMShutdown(w http.ResponseWriter, r *http.Request) {
	if s.vmMgr == nil {
		WriteJSONError(w, http.StatusInternalServerError, ErrCodeInternal, "VM manager not initialized")
		return
	}

	id := s.extractVMID(r)
	if id == "" && r.Body != nil {
		var act VMActionRequest
		_ = json.NewDecoder(r.Body).Decode(&act)
		id = act.ID
	}

	if id == "" {
		WriteJSONError(w, http.StatusBadRequest, ErrCodeInvalidInput, "missing VM ID parameter")
		return
	}

	if err := s.vmMgr.ShutdownVM(id); err != nil {
		if errors.Is(err, vm.ErrVMNotFoundInMgr) {
			WriteJSONError(w, http.StatusNotFound, ErrCodeNotFound, fmt.Sprintf("VM '%s' not found", id))
			return
		}
		if errors.Is(err, vm.ErrVMNotRunning) {
			WriteJSONError(w, http.StatusConflict, ErrCodeConflict, "VM is not running")
			return
		}
		if errors.Is(err, vm.ErrShutdownTimeout) {
			s.logger.Warn("VM shutdown timed out", "id", id)
			WriteJSONError(w, http.StatusGatewayTimeout, ErrCodeShutdownTimeout, err.Error())
			return
		}
		s.logger.Error("Failed to shut down VM", "id", id, "err", err)
		WriteJSONError(w, http.StatusInternalServerError, ErrCodeInternal, err.Error())
		return
	}

	s.logger.Info("Shut down virtual machine", "id", id)

	if r.Header.Get("HX-Request") == "true" {
		if cardView, err := s.getSingleVMCardView(id); err == nil {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("HX-Trigger", "vm-updated")
			_ = s.templates["dashboard"].ExecuteTemplate(w, "vm-card", cardView)
			return
		}
	}

	writeJSON(w, http.StatusOK, ActionResponse{
		Message: "VM shut down cleanly",
		ID:      id,
		Status:  "stopped",
	})
}

// handleAPIVMRestart handles POST /api/v1/vms/{id}/restart and /restart
func (s *Server) handleAPIVMRestart(w http.ResponseWriter, r *http.Request) {
	if s.vmMgr == nil {
		WriteJSONError(w, http.StatusInternalServerError, ErrCodeInternal, "VM manager not initialized")
		return
	}

	id := s.extractVMID(r)
	if id == "" && r.Body != nil {
		var act VMActionRequest
		_ = json.NewDecoder(r.Body).Decode(&act)
		id = act.ID
	}

	if id == "" {
		WriteJSONError(w, http.StatusBadRequest, ErrCodeInvalidInput, "missing VM ID parameter")
		return
	}

	if err := s.vmMgr.RestartVM(id); err != nil {
		if errors.Is(err, vm.ErrVMNotFoundInMgr) {
			WriteJSONError(w, http.StatusNotFound, ErrCodeNotFound, fmt.Sprintf("VM '%s' not found", id))
			return
		}
		s.logger.Error("Failed to restart VM", "id", id, "err", err)
		WriteJSONError(w, http.StatusInternalServerError, ErrCodeInternal, err.Error())
		return
	}

	s.logger.Info("Restarted virtual machine", "id", id)

	if r.Header.Get("HX-Request") == "true" {
		if cardView, err := s.getSingleVMCardView(id); err == nil {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("HX-Trigger", "vm-updated")
			_ = s.templates["dashboard"].ExecuteTemplate(w, "vm-card", cardView)
			return
		}
	}

	writeJSON(w, http.StatusOK, ActionResponse{
		Message: "VM restarted successfully",
		ID:      id,
		Status:  "running",
	})
}

// handleAPIVMStop handles POST /api/v1/vms/{id}/stop and /stop
func (s *Server) handleAPIVMStop(w http.ResponseWriter, r *http.Request) {
	if s.vmMgr == nil {
		WriteJSONError(w, http.StatusInternalServerError, ErrCodeInternal, "VM manager not initialized")
		return
	}

	id := s.extractVMID(r)
	if id == "" && r.Body != nil {
		var act VMActionRequest
		_ = json.NewDecoder(r.Body).Decode(&act)
		id = act.ID
	}

	if id == "" {
		WriteJSONError(w, http.StatusBadRequest, ErrCodeInvalidInput, "missing VM ID parameter")
		return
	}

	if err := s.vmMgr.StopVM(id); err != nil {
		if errors.Is(err, vm.ErrVMNotFoundInMgr) {
			WriteJSONError(w, http.StatusNotFound, ErrCodeNotFound, fmt.Sprintf("VM '%s' not found", id))
			return
		}
		if errors.Is(err, vm.ErrVMNotRunning) {
			WriteJSONError(w, http.StatusConflict, ErrCodeConflict, "VM is not running")
			return
		}
		s.logger.Error("Failed to stop VM", "id", id, "err", err)
		WriteJSONError(w, http.StatusInternalServerError, ErrCodeInternal, err.Error())
		return
	}

	s.logger.Info("Stopped virtual machine", "id", id)

	if r.Header.Get("HX-Request") == "true" {
		if cardView, err := s.getSingleVMCardView(id); err == nil {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("HX-Trigger", "vm-updated")
			_ = s.templates["dashboard"].ExecuteTemplate(w, "vm-card", cardView)
			return
		}
	}

	writeJSON(w, http.StatusOK, ActionResponse{
		Message: "VM stopped successfully",
		ID:      id,
		Status:  "stopped",
	})
}

// handleAPIVMQuit handles POST /api/v1/vms/{id}/quit and /quit
func (s *Server) handleAPIVMQuit(w http.ResponseWriter, r *http.Request) {
	if s.vmMgr == nil {
		WriteJSONError(w, http.StatusInternalServerError, ErrCodeInternal, "VM manager not initialized")
		return
	}

	id := s.extractVMID(r)
	if id == "" && r.Body != nil {
		var act VMActionRequest
		_ = json.NewDecoder(r.Body).Decode(&act)
		id = act.ID
	}

	if id == "" {
		WriteJSONError(w, http.StatusBadRequest, ErrCodeInvalidInput, "missing VM ID parameter")
		return
	}

	if err := s.vmMgr.QuitVM(id); err != nil {
		if errors.Is(err, vm.ErrVMNotFoundInMgr) {
			WriteJSONError(w, http.StatusNotFound, ErrCodeNotFound, fmt.Sprintf("VM '%s' not found", id))
			return
		}
		if errors.Is(err, vm.ErrVMNotRunning) {
			WriteJSONError(w, http.StatusConflict, ErrCodeConflict, "VM is not running")
			return
		}
		s.logger.Error("Failed to quit VM", "id", id, "err", err)
		WriteJSONError(w, http.StatusInternalServerError, ErrCodeInternal, err.Error())
		return
	}

	s.logger.Info("Quit virtual machine via QMP", "id", id)
	writeJSON(w, http.StatusOK, ActionResponse{
		Message: "VM quit cleanly via QMP",
		ID:      id,
		Status:  "stopped",
	})
}

// handleAPIVMStatus handles GET/POST /api/v1/vms/{id}/status and /status
func (s *Server) handleAPIVMStatus(w http.ResponseWriter, r *http.Request) {
	if s.vmMgr == nil {
		WriteJSONError(w, http.StatusInternalServerError, ErrCodeInternal, "VM manager not initialized")
		return
	}

	id := s.extractVMID(r)
	if id == "" && r.Body != nil {
		var act VMActionRequest
		_ = json.NewDecoder(r.Body).Decode(&act)
		id = act.ID
	}

	if id == "" {
		WriteJSONError(w, http.StatusBadRequest, ErrCodeInvalidInput, "missing VM ID parameter")
		return
	}

	st, err := s.vmMgr.StatusVM(id)
	if err != nil {
		if errors.Is(err, vm.ErrVMNotFoundInMgr) {
			WriteJSONError(w, http.StatusNotFound, ErrCodeNotFound, fmt.Sprintf("VM '%s' not found", id))
			return
		}
		WriteJSONError(w, http.StatusInternalServerError, ErrCodeInternal, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, st)
}

// handleAPIISOsList handles GET /api/v1/isos
func (s *Server) handleAPIISOsList(w http.ResponseWriter, r *http.Request) {
	if s.vmMgr == nil || s.vmMgr.Storage() == nil {
		WriteJSONError(w, http.StatusInternalServerError, ErrCodeInternal, "Storage manager not initialized")
		return
	}

	isoInfos, err := s.vmMgr.Storage().ListISOs()
	if err != nil {
		WriteJSONError(w, http.StatusInternalServerError, ErrCodeInternal, "failed to list ISOs: "+err.Error())
		return
	}

	res := make([]ISOResponse, 0, len(isoInfos))
	for _, item := range isoInfos {
		res = append(res, ISOResponse{
			Name:      item.Name,
			SizeBytes: item.SizeBytes,
		})
	}

	writeJSON(w, http.StatusOK, res)
}

// handleAPIISOUpload handles POST /api/v1/isos using direct streaming (no memory buffering).
func (s *Server) handleAPIISOUpload(w http.ResponseWriter, r *http.Request) {
	if s.vmMgr == nil || s.vmMgr.Storage() == nil {
		WriteJSONError(w, http.StatusInternalServerError, ErrCodeInternal, "Storage manager not initialized")
		return
	}

	contentType := r.Header.Get("Content-Type")
	var fileName string
	var reader io.Reader

	if strings.HasPrefix(contentType, "multipart/form-data") {
		mr, err := r.MultipartReader()
		if err != nil {
			WriteJSONError(w, http.StatusBadRequest, ErrCodeInvalidInput, "failed to parse multipart body: "+err.Error())
			return
		}

		for {
			part, err := mr.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				WriteJSONError(w, http.StatusBadRequest, ErrCodeInvalidInput, "failed to read part: "+err.Error())
				return
			}

			if part.FileName() != "" || part.FormName() == "file" {
				fileName = filepath.Base(part.FileName())
				reader = part
				break
			}
			_ = part.Close()
		}
	} else {
		// Support direct raw streaming upload via ?name=filename.iso
		fileName = strings.TrimSpace(r.URL.Query().Get("name"))
		reader = r.Body
	}

	if fileName == "" {
		WriteJSONError(w, http.StatusBadRequest, ErrCodeInvalidInput, "ISO filename must be provided in multipart header or 'name' query parameter")
		return
	}

	if err := storage.ValidateISOName(fileName); err != nil {
		WriteJSONError(w, http.StatusBadRequest, ErrCodeInvalidInput, err.Error())
		return
	}

	info, err := s.vmMgr.Storage().SaveISO(fileName, reader)
	if err != nil {
		if errors.Is(err, storage.ErrISOExists) {
			WriteJSONError(w, http.StatusConflict, ErrCodeConflict, fmt.Sprintf("ISO '%s' already exists", fileName))
			return
		}
		WriteJSONError(w, http.StatusInternalServerError, ErrCodeInternal, "failed to save ISO: "+err.Error())
		return
	}

	s.logger.Info("Uploaded ISO image", "name", info.Name, "size_bytes", info.SizeBytes)
	writeJSON(w, http.StatusCreated, ISOResponse{
		Name:      info.Name,
		SizeBytes: info.SizeBytes,
	})
}

// handleAPIISODelete handles DELETE /api/v1/isos/{name}
func (s *Server) handleAPIISODelete(w http.ResponseWriter, r *http.Request) {
	if s.vmMgr == nil || s.vmMgr.Storage() == nil {
		WriteJSONError(w, http.StatusInternalServerError, ErrCodeInternal, "Storage manager not initialized")
		return
	}

	name := r.PathValue("name")
	if name == "" {
		name = r.URL.Query().Get("name")
	}
	if name == "" {
		WriteJSONError(w, http.StatusBadRequest, ErrCodeInvalidInput, "ISO name is required")
		return
	}

	// Safety check: ensure no running VM is using this ISO
	for _, v := range s.vmMgr.ListVMs() {
		if v.Config.ISO == name && v.Runtime.State != vm.StateStopped {
			WriteJSONError(w, http.StatusConflict, ErrCodeConflict, fmt.Sprintf("ISO '%s' is in use by running VM '%s'", name, v.Config.ID))
			return
		}
	}

	if err := s.vmMgr.Storage().DeleteISO(name); err != nil {
		if errors.Is(err, storage.ErrISONotFound) {
			WriteJSONError(w, http.StatusNotFound, ErrCodeNotFound, fmt.Sprintf("ISO '%s' not found", name))
			return
		}
		if errors.Is(err, storage.ErrInvalidISOName) || errors.Is(err, storage.ErrPathEscapesRoot) {
			WriteJSONError(w, http.StatusBadRequest, ErrCodeInvalidInput, err.Error())
			return
		}
		WriteJSONError(w, http.StatusInternalServerError, ErrCodeInternal, "failed to delete ISO: "+err.Error())
		return
	}

	s.logger.Info("Deleted ISO image", "name", name)
	writeJSON(w, http.StatusOK, map[string]string{
		"message": fmt.Sprintf("ISO '%s' deleted successfully", name),
	})
}
