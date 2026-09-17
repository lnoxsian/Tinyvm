package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"tinyvm/internal/api"
	"tinyvm/internal/config"
	"tinyvm/internal/storage"
	"tinyvm/internal/version"
	"tinyvm/internal/vm"
)

func printUsage() {
	fmt.Printf(`TinyVM - Lightweight Single-Node Virtual Machine Manager

Usage:
  tinyvm <command> [arguments]

Commands:
  serve       Start the TinyVM web server and REST API (default)
  version     Show TinyVM version and build information
  list        List all virtual machines
  create      Create a new virtual machine
  start       Start a virtual machine
  shutdown    Gracefully shut down a virtual machine
  stop        Force stop a virtual machine
  restart     Restart a virtual machine
  delete      Delete a virtual machine and its storage
  status      Show the status of a virtual machine

Flags for 'serve':
  -listen string
        IP address to listen on (default: 0.0.0.0)
  -port int
        HTTP port to listen on (default: 8080)
  -data-dir string
        Data directory for VM storage and configs (default: /var/lib/tinyvm)
  -log-level string
        Logging level: debug, info, warn, error (default: info)
  -log-format string
        Logging format: text, json (default: text)

Environment Variables:
  TINYVM_LISTEN, TINYVM_PORT, TINYVM_DATA_DIR, TINYVM_LOG_LEVEL, TINYVM_LOG_FORMAT
`)
}

func main() {
	args := os.Args[1:]

	if len(args) == 0 {
		runServe(nil)
		return
	}

	cmd := args[0]
	switch cmd {
	case "version", "-v", "--version":
		info := version.Get()
		fmt.Printf("TinyVM v%s\n", info.Version)
		fmt.Printf("  Commit:    %s\n", info.Commit)
		fmt.Printf("  Built:     %s\n", info.BuildDate)
		fmt.Printf("  Go:        %s\n", info.GoVersion)
		fmt.Printf("  Platform:  %s\n", info.Platform)
		return

	case "help", "-h", "--help":
		printUsage()
		return

	case "serve":
		runServe(args[1:])
		return

	case "list":
		runList(args[1:])
		return

	case "create":
		runCreate(args[1:])
		return

	case "start":
		runStart(args[1:])
		return

	case "shutdown":
		runShutdown(args[1:])
		return

	case "stop":
		runStop(args[1:])
		return

	case "restart":
		runRestart(args[1:])
		return

	case "delete":
		runDelete(args[1:])
		return

	case "status":
		runStatus(args[1:])
		return

	default:
		// If argument begins with '-', treat as flag to 'serve'
		if len(cmd) > 0 && cmd[0] == '-' {
			runServe(args)
			return
		}
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", cmd)
		printUsage()
		os.Exit(1)
	}
}

func parseVMIDAndDataDir(cmdName string, args []string) (string, string) {
	var vmID string
	var dataDir string

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if (arg == "-data-dir" || arg == "--data-dir") && i+1 < len(args) {
			dataDir = args[i+1]
			i++
		} else if strings.HasPrefix(arg, "-data-dir=") {
			dataDir = strings.TrimPrefix(arg, "-data-dir=")
		} else if strings.HasPrefix(arg, "--data-dir=") {
			dataDir = strings.TrimPrefix(arg, "--data-dir=")
		} else if strings.HasPrefix(arg, "-") {
			// ignore other flags
		} else if vmID == "" {
			vmID = arg
		}
	}

	if vmID == "" {
		fmt.Fprintf(os.Stderr, "Error: missing VM ID\nUsage: tinyvm %s <vm-id> [-data-dir path]\n", cmdName)
		os.Exit(1)
	}
	return vmID, dataDir
}

func getManager(args []string) (*vm.Manager, error) {
	cfg, _, err := config.Load(args)
	if err != nil {
		return nil, fmt.Errorf("error loading configuration: %w", err)
	}

	store, err := storage.New(cfg.DataDir)
	if err != nil {
		return nil, fmt.Errorf("error accessing storage: %w", err)
	}

	mgr := vm.NewManager(store, nil)
	_ = mgr.RecoverAll()
	return mgr, nil
}

func runCreate(args []string) {
	fs := flag.NewFlagSet("tinyvm create", flag.ExitOnError)
	id := fs.String("id", "", "Unique VM identifier (required)")
	name := fs.String("name", "", "Friendly name for the VM")
	cpus := fs.Int("cpus", 1, "Number of vCPUs")
	ram := fs.Int("ram", 1024, "RAM in MB")
	disk := fs.String("disk", "disk.qcow2", "Disk file name")
	diskFormat := fs.String("disk-format", "qcow2", "Disk format (qcow2 or raw)")
	diskSize := fs.String("disk-size", "10G", "Virtual disk size (e.g. 20G, 500M)")
	iso := fs.String("iso", "", "Optional boot ISO file from storage")
	firmware := fs.String("firmware", "bios", "Boot firmware: 'bios' (default) or 'uefi'")
	sshPort := fs.Int("ssh-port", 0, "Optional host port to forward to guest SSH (port 22)")
	dataDir := fs.String("data-dir", "", "Custom data directory")

	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}

	if *id == "" {
		fmt.Fprintln(os.Stderr, "Error: -id is required")
		fs.Usage()
		os.Exit(1)
	}

	vmName := *name
	if vmName == "" {
		vmName = *id
	}

	cfg := vm.VMConfig{
		ID:         *id,
		Name:       vmName,
		CPUs:       *cpus,
		MemoryMB:   *ram,
		Disk:       *disk,
		DiskFormat: *diskFormat,
		DiskSize:   *diskSize,
		ISO:        *iso,
		Firmware:   *firmware,
		Network: vm.NetworkConfig{
			Enabled: true,
			Mode:    "user",
			SSHPort: *sshPort,
		},
	}

	var mgrArgs []string
	if *dataDir != "" {
		mgrArgs = append(mgrArgs, "-data-dir", *dataDir)
	}

	mgr, err := getManager(mgrArgs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Creating virtual machine '%s' (%s, %d vCPU, %d MB RAM, %s disk, %s firmware)...\n", *id, vmName, *cpus, *ram, *diskSize, *firmware)
	createdVM, err := mgr.CreateVM(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Virtual machine '%s' created successfully (state: %s).\n", createdVM.Config.ID, createdVM.Runtime.State)
}

func getManagerForDir(dataDir string) (*vm.Manager, error) {
	var args []string
	if dataDir != "" {
		args = []string{"-data-dir", dataDir}
	}
	return getManager(args)
}

func runStart(args []string) {
	vmID, dataDir := parseVMIDAndDataDir("start", args)
	mgr, err := getManagerForDir(dataDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Starting virtual machine '%s'...\n", vmID)
	if err := mgr.StartVM(vmID); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	v, _ := mgr.GetVM(vmID)
	fmt.Printf("VM '%s' started successfully (PID: %d)\n", vmID, v.Runtime.PID)
}

func runShutdown(args []string) {
	vmID, dataDir := parseVMIDAndDataDir("shutdown", args)
	mgr, err := getManagerForDir(dataDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Requesting shutdown for virtual machine '%s'...\n", vmID)
	if err := mgr.ShutdownVM(vmID); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("VM '%s' shut down successfully.\n", vmID)
}

func runStop(args []string) {
	vmID, dataDir := parseVMIDAndDataDir("stop", args)
	mgr, err := getManagerForDir(dataDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Stopping virtual machine '%s'...\n", vmID)
	if err := mgr.StopVM(vmID); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("VM '%s' stopped successfully.\n", vmID)
}

func runRestart(args []string) {
	vmID, dataDir := parseVMIDAndDataDir("restart", args)
	mgr, err := getManagerForDir(dataDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Restarting virtual machine '%s'...\n", vmID)
	if err := mgr.RestartVM(vmID); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	v, _ := mgr.GetVM(vmID)
	fmt.Printf("VM '%s' restarted successfully (PID: %d)\n", vmID, v.Runtime.PID)
}

func runDelete(args []string) {
	vmID, dataDir := parseVMIDAndDataDir("delete", args)
	mgr, err := getManagerForDir(dataDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Deleting virtual machine '%s'...\n", vmID)
	if err := mgr.DeleteVM(vmID); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("VM '%s' deleted successfully.\n", vmID)
}

func runStatus(args []string) {
	vmID, dataDir := parseVMIDAndDataDir("status", args)
	mgr, err := getManagerForDir(dataDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}

	st, err := mgr.StatusVM(vmID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("VM Name:      %s\n", st.Config.Name)
	fmt.Printf("VM ID:        %s\n", st.Config.ID)
	fmt.Printf("Status:       %s\n", st.Runtime.State)
	if st.Runtime.PID > 0 {
		fmt.Printf("PID:          %d\n", st.Runtime.PID)
		fmt.Printf("Uptime:       %s\n", st.Uptime)
	}
	fmt.Printf("vCPUs:        %d\n", st.Config.CPUs)
	fmt.Printf("Memory:       %d MB\n", st.Config.MemoryMB)
	fmt.Printf("Firmware:     %s\n", st.Config.Firmware)
	if st.EFIVarsPath != "" {
		fmt.Printf("EFI NVRAM:    %s\n", st.EFIVarsPath)
	}
	fmt.Printf("Disk:         %s (%s, virtual: %s, on-disk: %.2f MB)\n",
		st.Config.Disk, st.Config.DiskFormat, st.Config.DiskSize, float64(st.DiskActualBytes)/(1024*1024))
	if st.Config.ISO != "" {
		fmt.Printf("ISO:          %s\n", st.Config.ISO)
	}
	if st.Config.Network.Enabled {
		portsStr := "None"
		if st.Config.Network.SSHPort > 0 {
			portsStr = fmt.Sprintf("host:%d -> guest:22", st.Config.Network.SSHPort)
		}
		for _, p := range st.Config.Network.Ports {
			portsStr += fmt.Sprintf(", host:%d -> guest:%d (%s)", p.Host, p.Guest, p.Protocol)
		}
		fmt.Printf("Networking:   enabled (user-mode, %s)\n", portsStr)
	} else {
		fmt.Println("Networking:   disabled")
	}
	fmt.Printf("Log File:     %s\n", st.LogPath)
	fmt.Printf("QMP Socket:   %s\n", st.QMPSockPath)
	fmt.Printf("Console Sock: %s\n", st.ConsoleSockPath)
}

func runList(args []string) {
	fs := flag.NewFlagSet("tinyvm list", flag.ExitOnError)
	dataDir := fs.String("data-dir", "", "Custom data directory")
	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}

	mgr, err := getManagerForDir(*dataDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}

	vms := mgr.ListVMs()
	if len(vms) == 0 {
		fmt.Println("No virtual machines found.")
		return
	}

	fmt.Printf("%-18s %-20s %-6s %-10s %-10s %-10s %s\n", "ID", "NAME", "CPUS", "RAM", "DISK", "FIRMWARE", "STATUS")
	for _, v := range vms {
		diskSize := v.Config.DiskSize
		if diskSize == "" {
			diskSize = "Standard"
		}
		fw := v.Config.Firmware
		if fw == "" {
			fw = "bios"
		}
		fmt.Printf("%-18s %-20s %-6d %-10s %-10s %-10s %s\n",
			v.Config.ID,
			v.Config.Name,
			v.Config.CPUs,
			fmt.Sprintf("%d MB", v.Config.MemoryMB),
			diskSize,
			fw,
			v.Runtime.State,
		)
	}
}

func runServe(args []string) {
	cfg, _, err := config.Load(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading configuration: %v\n", err)
		os.Exit(1)
	}

	logger := cfg.SetupLogger()

	store, err := storage.New(cfg.DataDir)
	if err != nil {
		logger.Error("Failed to initialize storage", "data_dir", cfg.DataDir, "err", err)
		os.Exit(1)
	}

	vmMgr := vm.NewManager(store, nil)
	if err := vmMgr.RecoverAll(); err != nil {
		logger.Warn("Error reconciling VMs on startup", "err", err)
	}

	srv, err := api.NewServer(cfg, logger, vmMgr)
	if err != nil {
		logger.Error("Failed to initialize server", "err", err)
		os.Exit(1)
	}

	// Trap termination signals for graceful shutdown
	shutdownSig := make(chan os.Signal, 1)
	signal.Notify(shutdownSig, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)

	serverErr := make(chan error, 1)
	go func() {
		serverErr <- srv.Start()
	}()

	select {
	case sig := <-shutdownSig:
		logger.Info("Received termination signal, initiating shutdown", "signal", sig.String())
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := srv.Shutdown(ctx); err != nil {
			logger.Error("Error during server shutdown", "err", err)
			os.Exit(1)
		}
		logger.Info("TinyVM stopped cleanly")

	case err := <-serverErr:
		if err != nil {
			logger.Error("Server encountered fatal error", "err", err)
			os.Exit(1)
		}
	}
}
