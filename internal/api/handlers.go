package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"tinyvm/internal/storage"
	"tinyvm/internal/version"
	"tinyvm/internal/vm"
)

// BasePageData contains common fields for page templates.
type BasePageData struct {
	ActiveNav  string
	Version    string
	HostOnline bool
}

// DashboardPageData contains stats and VM listings for the dashboard.
type DashboardPageData struct {
	BasePageData
	TotalVMs    int
	RunningVMs  int
	StoppedVMs  int
	CPUPercent  int
	CPUCores    int
	KVMEnabled  bool
	MemUsedGB   float64
	MemTotalGB  float64
	MemPercent  int
	DiskUsedGB  float64
	DiskTotalGB float64
	DiskPercent int
	VMs         []VMCardView
}

// VMCardView provides presentation data for VM cards.
type VMCardView struct {
	ID          string
	Name        string
	Status      string
	StatusClass string
	CPUs        int
	MemoryMB    int
	DiskSize    string
}

// SettingsPageData provides data for the settings view.
type SettingsPageData struct {
	BasePageData
	DataDir    string
	ListenAddr string
}

// StoragePageData provides presentation data for the storage pool view.
type StoragePageData struct {
	BasePageData
	DataDir string
	ISODir  string
	ISOs    []ISOViewModel
}

// ISOViewModel holds formatted metadata for ISO display.
type ISOViewModel struct {
	Name          string
	Path          string
	SizeFormatted string
}

// VMDetailPageData provides presentation data for a specific VM view.
type VMDetailPageData struct {
	BasePageData
	VM *vm.VM
}

func (s *Server) render(w http.ResponseWriter, name string, data any) {
	tmpl, ok := s.templates[name]
	if !ok {
		http.Error(w, "Template not found: "+name, http.StatusInternalServerError)
		return
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		s.logger.Error("Failed to render template", "template", name, "err", err)
		http.Error(w, "Template execution error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = buf.WriteTo(w)
}

func checkKVM() bool {
	f, err := os.OpenFile("/dev/kvm", os.O_RDWR, 0)
	if err != nil {
		return false
	}
	_ = f.Close()
	return true
}

func (s *Server) getVMCardViews() ([]VMCardView, int, int) {
	if s.vmMgr == nil {
		return nil, 0, 0
	}

	rawVMs := s.vmMgr.ListVMs()
	views := make([]VMCardView, 0, len(rawVMs))
	runningCount := 0
	stoppedCount := 0

	for _, v := range rawVMs {
		st := string(v.Runtime.State)
		if st == "running" {
			runningCount++
		} else {
			stoppedCount++
		}

		diskSize := v.Config.DiskSize
		if diskSize == "" {
			diskSize = "Standard"
		}

		views = append(views, VMCardView{
			ID:          v.Config.ID,
			Name:        v.Config.Name,
			Status:      st,
			StatusClass: st,
			CPUs:        v.Config.CPUs,
			MemoryMB:    v.Config.MemoryMB,
			DiskSize:    diskSize,
		})
	}

	return views, runningCount, stoppedCount
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	views, running, stopped := s.getVMCardViews()

	data := DashboardPageData{
		BasePageData: BasePageData{
			ActiveNav:  "dashboard",
			Version:    version.Version,
			HostOnline: true,
		},
		TotalVMs:    len(views),
		RunningVMs:  running,
		StoppedVMs:  stopped,
		CPUPercent:  0,
		CPUCores:    runtime.NumCPU(),
		KVMEnabled:  checkKVM(),
		MemUsedGB:   0.0,
		MemTotalGB:  0.0,
		MemPercent:  0,
		DiskUsedGB:  0.0,
		DiskTotalGB: 0.0,
		DiskPercent: 0,
		VMs:         views,
	}

	s.render(w, "dashboard", data)
}

func (s *Server) handleVMsList(w http.ResponseWriter, r *http.Request) {
	accept := r.Header.Get("Accept")
	if strings.Contains(accept, "application/json") || (!strings.Contains(accept, "text/html") && r.Header.Get("HX-Request") == "") {
		s.handleAPIVMsList(w, r)
		return
	}

	views, _, _ := s.getVMCardViews()

	data := DashboardPageData{
		BasePageData: BasePageData{
			ActiveNav:  "vms",
			Version:    version.Version,
			HostOnline: true,
		},
		TotalVMs: len(views),
		VMs:      views,
	}
	s.render(w, "vm", data)
}

func (s *Server) handleVMCreate(w http.ResponseWriter, r *http.Request) {
	data := BasePageData{
		ActiveNav:  "vms",
		Version:    version.Version,
		HostOnline: true,
	}
	s.render(w, "create", data)
}

func (s *Server) handleVMDetail(w http.ResponseWriter, r *http.Request) {
	accept := r.Header.Get("Accept")
	if strings.Contains(accept, "application/json") || (!strings.Contains(accept, "text/html") && r.Header.Get("HX-Request") == "") {
		s.handleAPIVMGet(w, r)
		return
	}

	id := s.extractVMID(r)
	if s.vmMgr == nil {
		http.Error(w, "VM manager unavailable", http.StatusInternalServerError)
		return
	}

	targetVM, err := s.vmMgr.GetVM(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	data := VMDetailPageData{
		BasePageData: BasePageData{
			ActiveNav:  "vms",
			Version:    version.Version,
			HostOnline: true,
		},
		VM: targetVM,
	}
	s.render(w, "detail", data)
}

func (s *Server) handleVMConsole(w http.ResponseWriter, r *http.Request) {
	data := BasePageData{
		ActiveNav:  "vms",
		Version:    version.Version,
		HostOnline: true,
	}
	s.render(w, "console", data)
}

func (s *Server) handleStorage(w http.ResponseWriter, r *http.Request) {
	var isoModels []ISOViewModel
	var isoDir string
	if s.vmMgr != nil && s.vmMgr.Storage() != nil {
		isoDir = s.vmMgr.Storage().ISODir()
		if rawISOs, err := s.vmMgr.Storage().ListISOs(); err == nil {
			for _, item := range rawISOs {
				sizeStr := fmt.Sprintf("%.1f MB", float64(item.SizeBytes)/(1024*1024))
				if item.SizeBytes >= 1024*1024*1024 {
					sizeStr = fmt.Sprintf("%.2f GB", float64(item.SizeBytes)/(1024*1024*1024))
				}
				isoModels = append(isoModels, ISOViewModel{
					Name:          item.Name,
					Path:          item.Path,
					SizeFormatted: sizeStr,
				})
			}
		}
	}

	data := StoragePageData{
		BasePageData: BasePageData{
			ActiveNav:  "storage",
			Version:    version.Version,
			HostOnline: true,
		},
		DataDir: s.cfg.DataDir,
		ISODir:  isoDir,
		ISOs:    isoModels,
	}
	s.render(w, "storage", data)
}

func (s *Server) handleStorageUpload(w http.ResponseWriter, r *http.Request) {
	if s.vmMgr == nil || s.vmMgr.Storage() == nil {
		http.Error(w, "Storage manager not initialized", http.StatusInternalServerError)
		return
	}

	mr, err := r.MultipartReader()
	if err != nil {
		http.Error(w, "Invalid multipart upload: "+err.Error(), http.StatusBadRequest)
		return
	}

	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			http.Error(w, "Error reading upload: "+err.Error(), http.StatusBadRequest)
			return
		}

		if part.FileName() != "" || part.FormName() == "file" {
			fileName := filepath.Base(part.FileName())
			if err := storage.ValidateISOName(fileName); err != nil {
				_ = part.Close()
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}

			_, err := s.vmMgr.Storage().SaveISO(fileName, part)
			_ = part.Close()
			if err != nil {
				http.Error(w, "Failed to save ISO: "+err.Error(), http.StatusInternalServerError)
				return
			}
			break
		}
		_ = part.Close()
	}

	http.Redirect(w, r, "/storage", http.StatusSeeOther)
}

func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	data := SettingsPageData{
		BasePageData: BasePageData{
			ActiveNav:  "settings",
			Version:    version.Version,
			HostOnline: true,
		},
		DataDir:    s.cfg.DataDir,
		ListenAddr: s.cfg.Addr(),
	}
	s.render(w, "settings", data)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	resp := map[string]any{
		"status":        "ok",
		"version":       version.Get(),
		"kvm_available": checkKVM(),
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(resp)
}
