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
	"tinyvm/internal/version"
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

	case "list", "start", "stop", "shutdown", "status":
		fmt.Printf("tinyvm %s: VM lifecycle manager will be connected in Phase 4.\n", cmd)
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

func runServe(args []string) {
	cfg, _, err := config.Load(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading configuration: %v\n", err)
		os.Exit(1)
	}

	logger := cfg.SetupLogger()

	if err := cfg.EnsureDirs(); err != nil {
		logger.Warn("Could not create all default data directories (permission issues may require sudo or custom -data-dir)", "data_dir", cfg.DataDir, "err", err)
	}

	srv, err := api.NewServer(cfg, logger)
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
