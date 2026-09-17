package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"tinyvm/internal/host"
	"tinyvm/internal/storage"
	"tinyvm/internal/vm"
)

// CreateVMRequest represents the JSON payload to create a new VM.
type CreateVMRequest struct {
	ID         string           `json:"id"`
	Name       string           `json:"name"`
	CPUs       int              `json:"cpus"`
	MemoryMB   int              `json:"memory_mb"`
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

// handleAPIVMCreate handles POST /api/v1/vms
func (s *Server) handleAPIVMCreate(w http.ResponseWriter, r *http.Request) {
	if s.vmMgr == nil {
		WriteJSONError(w, http.StatusInternalServerError, ErrCodeInternal, "VM manager not initialized")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MB limit
	var req CreateVMRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteJSONError(w, http.StatusBadRequest, ErrCodeInvalidInput, "malformed request payload: "+err.Error())
		return
	}

	name := req.Name
	if name == "" {
		name = req.ID
	}

	cfg := vm.VMConfig{
		ID:         req.ID,
		Name:       name,
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

	if err := s.vmMgr.DeleteVM(id); err != nil {
		if errors.Is(err, vm.ErrVMNotFoundInMgr) {
			WriteJSONError(w, http.StatusNotFound, ErrCodeNotFound, fmt.Sprintf("VM '%s' not found", id))
			return
		}
		if errors.Is(err, vm.ErrVMAlreadyRunning) {
			WriteJSONError(w, http.StatusConflict, ErrCodeConflict, "cannot delete running VM; stop it first")
			return
		}
		s.logger.Error("Failed to delete VM", "id", id, "err", err)
		WriteJSONError(w, http.StatusInternalServerError, ErrCodeInternal, err.Error())
		return
	}

	s.logger.Info("Deleted virtual machine", "id", id)
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

// handleAPIVMStatus handles GET /api/v1/vms/{id}/status and /status
func (s *Server) handleAPIVMStatus(w http.ResponseWriter, r *http.Request) {
	if s.vmMgr == nil {
		WriteJSONError(w, http.StatusInternalServerError, ErrCodeInternal, "VM manager not initialized")
		return
	}

	id := s.extractVMID(r)
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
