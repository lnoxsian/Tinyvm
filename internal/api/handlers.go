package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"tinyvm/internal/host"
	"tinyvm/internal/storage"
	"tinyvm/internal/version"
	"tinyvm/internal/vm"
	"tinyvm/internal/websocket"
)

// BasePageData contains common fields for page templates.
type BasePageData struct {
	ActiveNav  string
	Version    string
	HostOnline bool
	KVMEnabled bool
}

// DashboardPageData contains stats and VM listings for the dashboard.
type DashboardPageData struct {
	BasePageData
	TotalVMs    int
	RunningVMs  int
	StoppedVMs  int
	CPUPercent  int
	CPUCores    int
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
	Arch        string
	OSType      string
	Status      string
	StatusClass string
	CPUs        int
	MemoryMB    int
	MemUsedMB   int
	DiskSize    string
	Firmware    string
	ISO         string
	PID         int
	Uptime      string
	CPUPercent  float64
	SSHPort     int
	IsRunning   bool
	IsStopping  bool
	IsPaused    bool
}

// CreateVMPageData provides context and hardware boundaries for the VM Creation Wizard.
type CreateVMPageData struct {
	BasePageData
	ISOs         []string
	HostCPUs     int
	HostMemoryMB int
	HostUEFI     bool
	NextSSHPort  int
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
	DataDir                   string
	VMsDir                    string
	ISODir                    string
	ISOs                      []ISOViewModel
	Disks                     []DiskViewModel
	DiskUsedGB                float64
	DiskTotalGB               float64
	DiskPercent               int
	TotalISOSizeFormatted     string
	TotalDiskActualFormatted  string
	TotalDiskVirtualFormatted string
	QCOWCount                 int
	RawCount                  int
}

// ISOViewModel holds formatted metadata for ISO display.
type ISOViewModel struct {
	Name          string
	Path          string
	SizeBytes     int64
	SizeFormatted string
}

// DiskViewModel holds formatted metadata for QCOW2 and RAW disk image display.
type DiskViewModel struct {
	VMID                 string
	VMName               string
	Filename             string
	Format               string
	Path                 string
	VirtualSizeBytes     int64
	ActualSizeBytes      int64
	VirtualSizeFormatted string
	ActualSizeFormatted  string
}

// VMDetailPageData provides presentation data for a specific VM view.
type VMDetailPageData struct {
	BasePageData
	VM            *vm.VM
	Metrics       *vm.VMMetrics
	AvailableISOs []string
	Snapshots     []vm.SnapshotInfo
	DiskInfo      *storage.DiskInfo
	QEMULog       string
}

// VMConsolePageData provides presentation data for the interactive VM console view.
type VMConsolePageData struct {
	BasePageData
	VM *vm.VM
}

var bufPool = sync.Pool{
	New: func() any {
		return new(bytes.Buffer)
	},
}

func (s *Server) render(w http.ResponseWriter, name string, data any) {
	tmpl, ok := s.templates[name]
	if !ok {
		http.Error(w, "Template not found: "+name, http.StatusInternalServerError)
		return
	}

	buf := bufPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer bufPool.Put(buf)

	if err := tmpl.Execute(buf, data); err != nil {
		s.logger.Error("Failed to render template", "template", name, "err", err)
		http.Error(w, "Template execution error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = buf.WriteTo(w)
}

var (
	kvmCacheMu    sync.RWMutex
	kvmCachedVal  bool
	kvmCacheUntil time.Time
)

func checkKVM() bool {
	kvmCacheMu.RLock()
	if time.Now().Before(kvmCacheUntil) {
		val := kvmCachedVal
		kvmCacheMu.RUnlock()
		return val
	}
	kvmCacheMu.RUnlock()

	kvmCacheMu.Lock()
	defer kvmCacheMu.Unlock()
	if time.Now().Before(kvmCacheUntil) {
		return kvmCachedVal
	}

	f, err := os.OpenFile("/dev/kvm", os.O_RDWR, 0)
	if err != nil {
		kvmCachedVal = false
	} else {
		_ = f.Close()
		kvmCachedVal = true
	}
	kvmCacheUntil = time.Now().Add(5 * time.Second)
	return kvmCachedVal
}

func toVMCardView(v *vm.VM) VMCardView {
	st := string(v.Runtime.State)
	diskSize := v.Config.DiskSize
	if diskSize == "" {
		diskSize = "Standard"
	}
	firmware := strings.ToUpper(v.Config.Firmware)
	if firmware == "" {
		firmware = "BIOS"
	}

	var uptimeStr string
	if v.Runtime.State == vm.StateRunning && !v.Runtime.StartedAt.IsZero() {
		uptimeStr = vm.FormatDuration(time.Since(v.Runtime.StartedAt))
	}

	var memUsedMB int
	var cpuPercent float64
	if v.Runtime.State == vm.StateRunning && v.Runtime.PID > 0 {
		rss := host.GetProcessRSSBytes(v.Runtime.PID)
		memUsedMB = int(rss / (1024 * 1024))
		cpuPercent = host.GetVMProcessCPUPercent(v.Runtime.PID, v.Config.CPUs)
	}

	return VMCardView{
		ID:          v.Config.ID,
		Name:        v.Config.Name,
		Arch:        v.Config.Arch,
		OSType:      v.Config.OSType,
		Status:      st,
		StatusClass: st,
		CPUs:        v.Config.CPUs,
		MemoryMB:    v.Config.MemoryMB,
		MemUsedMB:   memUsedMB,
		DiskSize:    diskSize,
		Firmware:    firmware,
		ISO:         v.Config.ISO,
		PID:         v.Runtime.PID,
		Uptime:      uptimeStr,
		CPUPercent:  cpuPercent,
		SSHPort:     v.Config.Network.SSHPort,
		IsRunning:   (st == "running"),
		IsStopping:  (st == "stopping"),
		IsPaused:    (st == "paused"),
	}
}

func (s *Server) getSingleVMCardView(id string) (*VMCardView, error) {
	if s.vmMgr == nil {
		return nil, fmt.Errorf("VM manager unavailable")
	}
	targetVM, err := s.vmMgr.GetVM(id)
	if err != nil {
		return nil, err
	}
	view := toVMCardView(targetVM)
	return &view, nil
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
		view := toVMCardView(v)
		if view.IsRunning {
			runningCount++
		} else {
			stoppedCount++
		}
		views = append(views, view)
	}

	return views, runningCount, stoppedCount
}

func (s *Server) getDashboardData() DashboardPageData {
	views, running, stopped := s.getVMCardViews()

	cpuInfo := host.GetCPUInfo()
	memInfo := host.GetMemoryInfo()

	var diskUsedGB, diskTotalGB float64
	var diskPercent int
	if s.cfg != nil {
		if diskInfo, err := host.GetDiskInfo(s.cfg.DataDir); err == nil && diskInfo.TotalBytes > 0 {
			diskUsedGB = float64(diskInfo.UsedBytes) / (1024 * 1024 * 1024)
			diskTotalGB = float64(diskInfo.TotalBytes) / (1024 * 1024 * 1024)
			diskPercent = int((float64(diskInfo.UsedBytes) / float64(diskInfo.TotalBytes)) * 100)
		}
	}

	var memUsedGB, memTotalGB float64
	var memPercent int
	if memInfo.TotalBytes > 0 {
		memUsedGB = float64(memInfo.UsedBytes) / (1024 * 1024 * 1024)
		memTotalGB = float64(memInfo.TotalBytes) / (1024 * 1024 * 1024)
		memPercent = int((float64(memInfo.UsedBytes) / float64(memInfo.TotalBytes)) * 100)
	}

	kvmOk := checkKVM()

	return DashboardPageData{
		BasePageData: BasePageData{
			ActiveNav:  "dashboard",
			Version:    version.Version,
			HostOnline: true,
			KVMEnabled: kvmOk,
		},
		TotalVMs:    len(views),
		RunningVMs:  running,
		StoppedVMs:  stopped,
		CPUPercent:  cpuInfo.UsagePercent,
		CPUCores:    cpuInfo.Count,
		MemUsedGB:   memUsedGB,
		MemTotalGB:  memTotalGB,
		MemPercent:  memPercent,
		DiskUsedGB:  diskUsedGB,
		DiskTotalGB: diskTotalGB,
		DiskPercent: diskPercent,
		VMs:         views,
	}
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	data := s.getDashboardData()
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
			KVMEnabled: checkKVM(),
		},
		TotalVMs: len(views),
		VMs:      views,
	}
	s.render(w, "vm", data)
}

func (s *Server) handlePartialStats(w http.ResponseWriter, r *http.Request) {
	data := s.getDashboardData()
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = s.templates["dashboard"].ExecuteTemplate(w, "stats-grid", data)
}

func (s *Server) handlePartialVMs(w http.ResponseWriter, r *http.Request) {
	views, _, _ := s.getVMCardViews()
	data := DashboardPageData{
		VMs: views,
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = s.templates["dashboard"].ExecuteTemplate(w, "vm-grid", data)
}

func (s *Server) handlePartialVMCard(w http.ResponseWriter, r *http.Request) {
	id := s.extractVMID(r)
	if id == "" {
		http.Error(w, "Missing VM ID", http.StatusBadRequest)
		return
	}
	cardView, err := s.getSingleVMCardView(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = s.templates["dashboard"].ExecuteTemplate(w, "vm-card", cardView)
}

func (s *Server) handlePartialVMMetrics(w http.ResponseWriter, r *http.Request) {
	id := s.extractVMID(r)
	if id == "" {
		http.Error(w, "Missing VM ID", http.StatusBadRequest)
		return
	}
	if s.vmMgr == nil {
		http.Error(w, "VM manager unavailable", http.StatusInternalServerError)
		return
	}
	metrics, err := s.vmMgr.GetVMMetrics(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = s.templates["detail"].ExecuteTemplate(w, "vm-telemetry", metrics)
}

func (s *Server) handleVMCreate(w http.ResponseWriter, r *http.Request) {
	var isoNames []string
	if s.vmMgr != nil && s.vmMgr.Storage() != nil {
		if rawISOs, err := s.vmMgr.Storage().ListISOs(); err == nil {
			for _, item := range rawISOs {
				isoNames = append(isoNames, item.Name)
			}
		}
	}

	hostCPUs := host.GetCPUInfo().Count
	if hostCPUs <= 0 {
		hostCPUs = 1
	}

	hostMemInfo := host.GetMemoryInfo()
	hostMemMB := int(hostMemInfo.TotalBytes / (1024 * 1024))
	if hostMemMB <= 0 {
		hostMemMB = 2048
	}

	uefiStatus := host.DetectUEFIFirmware()

	// Calculate next available SSH port starting from 2222
	nextPort := 2222
	if s.vmMgr != nil {
		usedPorts := make(map[int]bool)
		for _, v := range s.vmMgr.ListVMs() {
			if v.Config.Network.SSHPort > 0 {
				usedPorts[v.Config.Network.SSHPort] = true
			}
			for _, p := range v.Config.Network.Ports {
				if p.Host > 0 {
					usedPorts[p.Host] = true
				}
			}
		}
		for usedPorts[nextPort] {
			nextPort++
		}
	}

	data := CreateVMPageData{
		BasePageData: BasePageData{
			ActiveNav:  "vms",
			Version:    version.Version,
			HostOnline: true,
			KVMEnabled: checkKVM(),
		},
		ISOs:         isoNames,
		HostCPUs:     hostCPUs,
		HostMemoryMB: hostMemMB,
		HostUEFI:     uefiStatus.Available,
		NextSSHPort:  nextPort,
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

	metrics, _ := s.vmMgr.GetVMMetrics(id)

	var isoNames []string
	if s.vmMgr.Storage() != nil {
		if rawISOs, err := s.vmMgr.Storage().ListISOs(); err == nil {
			for _, item := range rawISOs {
				isoNames = append(isoNames, item.Name)
			}
		}
	}

	var snapshots []vm.SnapshotInfo
	snapshots, _ = s.vmMgr.ListVMSnapshots(id)

	var diskInfo *storage.DiskInfo
	if s.vmMgr.Storage() != nil {
		if vmDir, err := s.vmMgr.Storage().VMDir(id); err == nil {
			diskName := targetVM.Config.Disk
			if diskName == "" {
				diskName = "disk.qcow2"
			}
			diskPath := filepath.Join(vmDir, diskName)
			diskInfo, _ = storage.InspectDisk(diskPath)
		}
	}

	var qemuLog string
	if s.vmMgr.Storage() != nil {
		if vmDir, err := s.vmMgr.Storage().VMDir(id); err == nil {
			logPath := filepath.Join(vmDir, "logs", "qemu.log")
			if data, err := os.ReadFile(logPath); err == nil {
				if len(data) > 65536 {
					qemuLog = string(data[len(data)-65536:])
				} else {
					qemuLog = string(data)
				}
			}
		}
	}

	data := VMDetailPageData{
		BasePageData: BasePageData{
			ActiveNav:  "vms",
			Version:    version.Version,
			HostOnline: true,
			KVMEnabled: checkKVM(),
		},
		VM:            targetVM,
		Metrics:       metrics,
		AvailableISOs: isoNames,
		Snapshots:     snapshots,
		DiskInfo:      diskInfo,
		QEMULog:       qemuLog,
	}
	s.render(w, "detail", data)
}

func (s *Server) handleVMConsole(w http.ResponseWriter, r *http.Request) {
	// If the request requests a WebSocket upgrade, bridge to the serial console
	if strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		s.handleAPIVMConsoleWS(w, r)
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

	data := VMConsolePageData{
		BasePageData: BasePageData{
			ActiveNav:  "vms",
			Version:    version.Version,
			HostOnline: true,
			KVMEnabled: checkKVM(),
		},
		VM: targetVM,
	}
	s.render(w, "console", data)
}

func (s *Server) handleAPIVMConsoleWS(w http.ResponseWriter, r *http.Request) {
	id := s.extractVMID(r)
	if id == "" {
		http.Error(w, "Missing VM ID", http.StatusBadRequest)
		return
	}
	if s.vmMgr == nil {
		http.Error(w, "VM manager unavailable", http.StatusInternalServerError)
		return
	}

	targetVM, err := s.vmMgr.GetVM(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	if targetVM.Runtime.State != vm.StateRunning && targetVM.Runtime.State != vm.StateStarting {
		http.Error(w, "VM is not running", http.StatusConflict)
		return
	}

	vmDir, err := s.vmMgr.Storage().VMDir(id)
	if err != nil {
		http.Error(w, "Invalid VM directory", http.StatusBadRequest)
		return
	}
	_ = websocket.HandleConsole(w, r, vmDir, s.logger)
}

func (s *Server) handleAPIVMVncWS(w http.ResponseWriter, r *http.Request) {
	id := s.extractVMID(r)
	if id == "" {
		http.Error(w, "Missing VM ID", http.StatusBadRequest)
		return
	}
	if s.vmMgr == nil {
		http.Error(w, "VM manager unavailable", http.StatusInternalServerError)
		return
	}

	targetVM, err := s.vmMgr.GetVM(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	if targetVM.Runtime.State != vm.StateRunning && targetVM.Runtime.State != vm.StateStarting {
		http.Error(w, "VM is not running", http.StatusConflict)
		return
	}

	vmDir, err := s.vmMgr.Storage().VMDir(id)
	if err != nil {
		http.Error(w, "Invalid VM directory", http.StatusBadRequest)
		return
	}
	_ = websocket.HandleVNC(w, r, vmDir, s.logger)
}

func (s *Server) handleAPIVMSSHWS(w http.ResponseWriter, r *http.Request) {
	id := s.extractVMID(r)
	if id == "" {
		http.Error(w, "Missing VM ID", http.StatusBadRequest)
		return
	}
	if s.vmMgr == nil {
		http.Error(w, "VM manager unavailable", http.StatusInternalServerError)
		return
	}

	targetVM, err := s.vmMgr.GetVM(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	if targetVM.Runtime.State != vm.StateRunning && targetVM.Runtime.State != vm.StateStarting {
		http.Error(w, "VM is not running", http.StatusConflict)
		return
	}

	vmDir, err := s.vmMgr.Storage().VMDir(id)
	if err != nil {
		http.Error(w, "Invalid VM directory", http.StatusBadRequest)
		return
	}

	sshPort := targetVM.Config.Network.SSHPort
	if sshPort <= 0 {
		for _, pf := range targetVM.Config.Network.Ports {
			if pf.Guest == 22 && (pf.Protocol == "" || strings.EqualFold(pf.Protocol, "tcp")) {
				sshPort = pf.Host
				break
			}
		}
	}

	user := r.URL.Query().Get("user")
	mode := r.URL.Query().Get("mode")

	_ = websocket.HandleSSH(w, r, websocket.SSHOptions{
		VMID:    id,
		VMName:  targetVM.Config.Name,
		VMDir:   vmDir,
		SSHPort: sshPort,
		User:    user,
		Mode:    mode,
		Logger:  s.logger,
	})
}

func (s *Server) handleVMNoVNCApp(w http.ResponseWriter, r *http.Request) {
	id := s.extractVMID(r)
	if id == "" {
		http.Error(w, "Missing VM ID", http.StatusBadRequest)
		return
	}
	if s.vmMgr == nil {
		http.Error(w, "VM manager unavailable", http.StatusInternalServerError)
		return
	}
	if _, err := s.vmMgr.GetVM(id); err != nil {
		http.NotFound(w, r)
		return
	}

	targetURL := fmt.Sprintf("/vendor/novnc/vnc.html?autoconnect=true&reconnect=true&resize=scale&path=/api/v1/vms/%s/vnc", url.PathEscape(id))
	http.Redirect(w, r, targetURL, http.StatusTemporaryRedirect)
}

func (s *Server) handleStorage(w http.ResponseWriter, r *http.Request) {
	var isoModels []ISOViewModel
	var isoDir string
	var totalISOBytes int64
	if s.vmMgr != nil && s.vmMgr.Storage() != nil {
		isoDir = s.vmMgr.Storage().ISODir()
		if rawISOs, err := s.vmMgr.Storage().ListISOs(); err == nil {
			for _, item := range rawISOs {
				totalISOBytes += item.SizeBytes
				isoModels = append(isoModels, ISOViewModel{
					Name:          item.Name,
					Path:          item.Path,
					SizeBytes:     item.SizeBytes,
					SizeFormatted: storage.FormatBytes(item.SizeBytes),
				})
			}
		}
	}

	var vmsDir string
	if s.vmMgr != nil && s.vmMgr.Storage() != nil {
		vmsDir = s.vmMgr.Storage().VMsDir()
	} else if s.cfg != nil {
		vmsDir = s.cfg.VMsDir()
	}

	// Discover all VM disks (qcow2, raw, img)
	var diskModels []DiskViewModel
	var totalDiskActualBytes, totalDiskVirtualBytes int64
	var qcowCount, rawCount int

	vmMap := make(map[string]*vm.VM)
	if s.vmMgr != nil {
		for _, v := range s.vmMgr.ListVMs() {
			vmMap[v.Config.ID] = v
		}
	}

	seenPaths := make(map[string]bool)

	if vmsDir != "" {
		if entries, err := os.ReadDir(vmsDir); err == nil {
			for _, entry := range entries {
				if !entry.IsDir() {
					continue
				}
				vmID := entry.Name()
				vmDir := filepath.Join(vmsDir, vmID)

				vmName := vmID
				if targetVM, ok := vmMap[vmID]; ok && targetVM.Config.Name != "" {
					vmName = targetVM.Config.Name
				}

				files, err := os.ReadDir(vmDir)
				if err != nil {
					continue
				}

				for _, f := range files {
					if f.IsDir() {
						continue
					}
					fname := f.Name()
					ext := strings.ToLower(filepath.Ext(fname))
					isConfigDisk := false
					if targetVM, ok := vmMap[vmID]; ok && targetVM.Config.Disk == fname {
						isConfigDisk = true
					}

					if ext != ".qcow2" && ext != ".raw" && ext != ".img" && !isConfigDisk {
						continue
					}

					diskPath := filepath.Join(vmDir, fname)
					if seenPaths[diskPath] {
						continue
					}
					seenPaths[diskPath] = true

					var format string
					var virtualSize, actualSize int64

					info, inspectErr := storage.InspectDisk(diskPath)
					if inspectErr == nil && info != nil {
						format = strings.ToLower(info.Format)
						virtualSize = info.VirtualSize
						actualSize = info.ActualSize
					} else {
						if fi, statErr := f.Info(); statErr == nil {
							actualSize = fi.Size()
						}
						if targetVM, ok := vmMap[vmID]; ok && targetVM.Config.DiskFormat != "" {
							format = strings.ToLower(targetVM.Config.DiskFormat)
						} else if ext == ".qcow2" {
							format = "qcow2"
						} else {
							format = "raw"
						}
						virtualSize = actualSize
					}

					if format == "qcow2" {
						qcowCount++
					} else {
						rawCount++
					}

					totalDiskActualBytes += actualSize
					totalDiskVirtualBytes += virtualSize

					diskModels = append(diskModels, DiskViewModel{
						VMID:                 vmID,
						VMName:               vmName,
						Filename:             fname,
						Format:               format,
						Path:                 diskPath,
						VirtualSizeBytes:     virtualSize,
						ActualSizeBytes:      actualSize,
						VirtualSizeFormatted: storage.FormatBytes(virtualSize),
						ActualSizeFormatted:  storage.FormatBytes(actualSize),
					})
				}
			}
		}
	}

	var diskUsedGB, diskTotalGB float64
	var diskPercent int
	if s.cfg != nil {
		if diskInfo, err := host.GetDiskInfo(s.cfg.DataDir); err == nil && diskInfo.TotalBytes > 0 {
			diskUsedGB = float64(diskInfo.UsedBytes) / (1024 * 1024 * 1024)
			diskTotalGB = float64(diskInfo.TotalBytes) / (1024 * 1024 * 1024)
			diskPercent = int((float64(diskInfo.UsedBytes) / float64(diskInfo.TotalBytes)) * 100)
		}
	}

	data := StoragePageData{
		BasePageData: BasePageData{
			ActiveNav:  "storage",
			Version:    version.Version,
			HostOnline: true,
			KVMEnabled: checkKVM(),
		},
		DataDir:                   s.cfg.DataDir,
		VMsDir:                    vmsDir,
		ISODir:                    isoDir,
		ISOs:                      isoModels,
		Disks:                     diskModels,
		DiskUsedGB:                diskUsedGB,
		DiskTotalGB:               diskTotalGB,
		DiskPercent:               diskPercent,
		TotalISOSizeFormatted:     storage.FormatBytes(totalISOBytes),
		TotalDiskActualFormatted:  storage.FormatBytes(totalDiskActualBytes),
		TotalDiskVirtualFormatted: storage.FormatBytes(totalDiskVirtualBytes),
		QCOWCount:                 qcowCount,
		RawCount:                  rawCount,
	}
	s.render(w, "storage", data)
}

func (s *Server) handleStorageUpload(w http.ResponseWriter, r *http.Request) {
	rc := http.NewResponseController(w)
	_ = rc.SetReadDeadline(time.Time{})
	_ = rc.SetWriteDeadline(time.Time{})

	isJSON := strings.Contains(r.Header.Get("Accept"), "application/json") || strings.Contains(r.Header.Get("X-Requested-With"), "XMLHttpRequest")

	if s.vmMgr == nil || s.vmMgr.Storage() == nil {
		if isJSON {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": "Storage manager not initialized"})
			return
		}
		http.Error(w, "Storage manager not initialized", http.StatusInternalServerError)
		return
	}

	mr, err := r.MultipartReader()
	if err != nil {
		if isJSON {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": "Invalid multipart upload: " + err.Error()})
			return
		}
		http.Error(w, "Invalid multipart upload: "+err.Error(), http.StatusBadRequest)
		return
	}

	var savedName string
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			s.logger.Error("Storage upload part reading failed", "error", err)
			if isJSON {
				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]any{"error": "Error reading upload: " + err.Error()})
				return
			}
			http.Error(w, "Error reading upload: "+err.Error(), http.StatusBadRequest)
			return
		}

		if part.FileName() != "" || part.FormName() == "file" {
			fileName := filepath.Base(part.FileName())
			if err := storage.ValidateISOName(fileName); err != nil {
				_ = part.Close()
				s.logger.Warn("Storage upload invalid ISO filename", "filename", fileName, "error", err)
				if isJSON {
					w.Header().Set("Content-Type", "application/json; charset=utf-8")
					w.WriteHeader(http.StatusBadRequest)
					_ = json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
					return
				}
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}

			info, err := s.vmMgr.Storage().SaveISO(fileName, part)
			_ = part.Close()
			if err != nil {
				s.logger.Error("Storage upload failed", "filename", fileName, "error", err)
				status := http.StatusInternalServerError
				if errors.Is(err, storage.ErrISOExists) {
					status = http.StatusConflict
				} else if errors.Is(err, storage.ErrInvalidISOName) {
					status = http.StatusBadRequest
				}

				if isJSON {
					w.Header().Set("Content-Type", "application/json; charset=utf-8")
					w.WriteHeader(status)
					_ = json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
					return
				}
				http.Error(w, "Failed to save ISO: "+err.Error(), status)
				return
			}
			s.logger.Info("Uploaded ISO image successfully", "name", info.Name, "size_bytes", info.SizeBytes)
			savedName = fileName
			break
		}
		_ = part.Close()
	}

	if savedName == "" {
		if isJSON {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": "No valid ISO file received"})
			return
		}
		http.Error(w, "No file uploaded", http.StatusBadRequest)
		return
	}

	if isJSON {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "ok",
			"name":   savedName,
		})
		return
	}

	http.Redirect(w, r, "/storage", http.StatusSeeOther)
}

func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	data := SettingsPageData{
		BasePageData: BasePageData{
			ActiveNav:  "settings",
			Version:    version.Version,
			HostOnline: true,
			KVMEnabled: checkKVM(),
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

func (s *Server) handleAPIMetrics(w http.ResponseWriter, r *http.Request) {
	if s.vmMgr == nil {
		WriteJSONError(w, http.StatusInternalServerError, ErrCodeInternal, "VM manager unavailable")
		return
	}
	metrics := s.vmMgr.GetHostMetrics()
	writeJSON(w, http.StatusOK, metrics)
}

func (s *Server) handleAPIVMMetrics(w http.ResponseWriter, r *http.Request) {
	id := s.extractVMID(r)
	if id == "" {
		WriteJSONError(w, http.StatusBadRequest, ErrCodeInvalidInput, "missing VM id")
		return
	}
	if s.vmMgr == nil {
		WriteJSONError(w, http.StatusInternalServerError, ErrCodeInternal, "VM manager unavailable")
		return
	}
	metrics, err := s.vmMgr.GetVMMetrics(id)
	if err != nil {
		WriteJSONError(w, http.StatusNotFound, ErrCodeNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, metrics)
}

