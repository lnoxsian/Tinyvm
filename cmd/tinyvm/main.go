package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
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
  start       Start a virtual machine
  stop        Force stop a virtual machine
  shutdown    Gracefully shut down a virtual machine
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

	case "start":
		if len(args) < 2 {
			fmt.Println("Usage: tinyvm start <vm-id>")
			os.Exit(1)
		}
		runStart(args[1])
		return

	case "stop":
		if len(args) < 2 {
			fmt.Println("Usage: tinyvm stop <vm-id>")
			os.Exit(1)
		}
		runStop(args[1])
		return

	case "status":
		if len(args) < 2 {
			fmt.Println("Usage: tinyvm status <vm-id>")
			os.Exit(1)
		}
		runStatus(args[1])
		return

	case "shutdown":
		fmt.Println("tinyvm shutdown: QMP graceful ACPI shutdown will be connected in Phase 5. Use 'tinyvm stop' for force stop.")
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

func getManager(args []string) (*vm.Manager, error) {
	cfg, _, err := config.Load(args)
	if err != nil {
		return nil, fmt.Errorf("error loading configuration: %w", err)
	}

	store, err := storage.New(cfg.DataDir)
	if err != nil {
		return nil, fmt.Errorf("error accessing storage: %w", err)
	}

	return vm.NewManager(store, nil), nil
}

func runStart(vmID string) {
	mgr, err := getManager(nil)
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

func runStop(vmID string) {
	mgr, err := getManager(nil)
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

func runStatus(vmID string) {
	mgr, err := getManager(nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}

	v, err := mgr.GetVM(vmID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("VM:      %s\n", v.Config.Name)
	fmt.Printf("ID:      %s\n", v.Config.ID)
	fmt.Printf("Status:  %s\n", v.Runtime.State)
	if v.Runtime.PID > 0 {
		fmt.Printf("PID:     %d\n", v.Runtime.PID)
		fmt.Printf("Uptime:  %s\n", time.Since(v.Runtime.StartedAt).Round(time.Second))
	}
	fmt.Printf("CPUs:    %d\n", v.Config.CPUs)
	fmt.Printf("Memory:  %d MB\n", v.Config.MemoryMB)
	fmt.Printf("Disk:    %s\n", v.Config.Disk)
}

func runList(args []string) {
	mgr, err := getManager(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}

	vms := mgr.ListVMs()
	if len(vms) == 0 {
		fmt.Println("No virtual machines found.")
		return
	}

	fmt.Printf("%-18s %-20s %-6s %-10s %-10s %s\n", "ID", "NAME", "CPUS", "RAM", "DISK", "STATUS")
	for _, v := range vms {
		diskSize := v.Config.DiskSize
		if diskSize == "" {
			diskSize = "Standard"
		}
		fmt.Printf("%-18s %-20s %-6d %-10s %-10s %s\n",
			v.Config.ID,
			v.Config.Name,
			v.Config.CPUs,
			fmt.Sprintf("%d MB", v.Config.MemoryMB),
			diskSize,
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
