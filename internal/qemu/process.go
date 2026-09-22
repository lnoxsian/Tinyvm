package qemu

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

var (
	ErrProcessAlreadyRunning = errors.New("QEMU process is already running")
	ErrProcessNotRunning     = errors.New("QEMU process is not running")
)

// ExitResult captures the exit status of a QEMU process.
type ExitResult struct {
	PID      int
	ExitCode int
	Err      error
}

// Process manages an active QEMU subprocess.
type Process struct {
	cmd      *exec.Cmd
	pid      int
	vmID     string
	paths    QEMUPaths
	logFile  *os.File
	exitCh   chan ExitResult
	cancelFn context.CancelFunc

	mu        sync.RWMutex
	running   bool
	exited    bool
	exitCode  int
	exitError error
}

// CleanStaleArtifacts removes lingering socket and PID files before process launch.
func CleanStaleArtifacts(paths QEMUPaths) {
	_ = os.Remove(paths.QMPSock)
	_ = os.Remove(paths.ConsoleSock)
	if paths.VNCSock != "" {
		_ = os.Remove(paths.VNCSock)
	}
	_ = os.Remove(paths.PIDFile)
}

// StartProcess launches the QEMU binary with arguments, redirecting output to logs/qemu.log.
func StartProcess(binary string, args []string, paths QEMUPaths, vmID string) (*Process, error) {
	CleanStaleArtifacts(paths)

	logPath := paths.LogFile
	if logPath == "" && paths.VMDir != "" {
		logPath = filepath.Join(paths.VMDir, "logs", "qemu.log")
	}
	if logPath == "" {
		return nil, fmt.Errorf("no log path or VM directory configured")
	}

	logsDir := filepath.Dir(logPath)
	if err := os.MkdirAll(logsDir, 0750); err != nil {
		return nil, fmt.Errorf("failed to create logs directory: %w", err)
	}

	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0640)
	if err != nil {
		return nil, fmt.Errorf("failed to open QEMU log file: %w", err)
	}

	now := time.Now().Format("2006-01-02 15:04:05")
	var formattedArgs strings.Builder
	for i, arg := range args {
		if i > 0 {
			formattedArgs.WriteString(" \\\n    ")
		}
		if strings.ContainsAny(arg, " \t\n") {
			formattedArgs.WriteString(fmt.Sprintf("%q", arg))
		} else {
			formattedArgs.WriteString(arg)
		}
	}

	banner := fmt.Sprintf("================================================================================\n"+
		"[%s] [LAUNCH] Starting Virtual Machine: %s\n"+
		"Binary: %s\n"+
		"Command:\n  %s \\\n    %s\n"+
		"================================================================================\n",
		now, vmID, binary, binary, formattedArgs.String())
	_, _ = logFile.WriteString(banner)
	_ = logFile.Sync()

	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid: true,
	}
	cmd.Stdout = logFile
	cmd.Stderr = logFile

	if err := cmd.Start(); err != nil {
		cancel()
		errTime := time.Now().Format("2006-01-02 15:04:05")
		_, _ = fmt.Fprintf(logFile, "[%s] [ERROR] Failed to start QEMU: %v\n================================================================================\n", errTime, err)
		_ = logFile.Sync()
		_ = logFile.Close()
		return nil, fmt.Errorf("failed to start QEMU: %w", err)
	}

	pid := cmd.Process.Pid
	_, _ = fmt.Fprintf(logFile, "[%s] [RUNNING] Process started successfully with PID %d\n", time.Now().Format("2006-01-02 15:04:05"), pid)
	_ = logFile.Sync()

	// Write PID file
	_ = os.WriteFile(paths.PIDFile, []byte(fmt.Sprintf("%d\n", pid)), 0640)

	p := &Process{
		cmd:      cmd,
		pid:      pid,
		vmID:     vmID,
		paths:    paths,
		logFile:  logFile,
		exitCh:   make(chan ExitResult, 1),
		cancelFn: cancel,
		running:  true,
	}

	// Spawn monitor goroutine
	go p.monitor()

	return p, nil
}

// monitor waits for the QEMU process to exit and cleans up resources.
func (p *Process) monitor() {
	waitErr := p.cmd.Wait()

	exitCode := 0
	if waitErr != nil {
		var exitErr *exec.ExitError
		if errors.As(waitErr, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
		}
	}

	p.mu.Lock()
	p.running = false
	p.exited = true
	p.exitCode = exitCode
	p.exitError = waitErr
	p.mu.Unlock()

	// Write termination record to log before closing
	if p.logFile != nil {
		timestamp := time.Now().Format("2006-01-02 15:04:05")
		if waitErr != nil {
			_, _ = fmt.Fprintf(p.logFile, "\n[%s] [TERMINATED] Process %d exited with code %d: %v\n================================================================================\n",
				timestamp, p.pid, exitCode, waitErr)
		} else {
			_, _ = fmt.Fprintf(p.logFile, "\n[%s] [TERMINATED] Process %d exited normally (exit code 0)\n================================================================================\n",
				timestamp, p.pid)
		}
		_ = p.logFile.Sync()
		_ = p.logFile.Close()
	}

	// Clean sockets and PID file on termination
	CleanStaleArtifacts(p.paths)

	p.cancelFn()

	p.exitCh <- ExitResult{
		PID:      p.pid,
		ExitCode: exitCode,
		Err:      waitErr,
	}
	close(p.exitCh)
}

// PID returns the OS process ID.
func (p *Process) PID() int {
	return p.pid
}

// IsRunning returns true if the QEMU process is currently active.
func (p *Process) IsRunning() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if !p.running {
		return false
	}

	// Double-check with OS
	err := syscall.Kill(p.pid, 0)
	return err == nil
}

// Terminate sends SIGTERM and waits up to timeout before sending SIGKILL.
func (p *Process) Terminate(timeout time.Duration) error {
	p.mu.RLock()
	if !p.running {
		p.mu.RUnlock()
		return nil
	}
	p.mu.RUnlock()

	// Send SIGTERM
	if err := syscall.Kill(p.pid, syscall.SIGTERM); err != nil {
		if errors.Is(err, syscall.ESRCH) {
			return nil
		}
		return fmt.Errorf("failed to send SIGTERM: %w", err)
	}

	select {
	case <-p.exitCh:
		return nil
	case <-time.After(timeout):
		// Escalate to SIGKILL
		return p.Kill()
	}
}

// Kill forces process termination via SIGKILL.
func (p *Process) Kill() error {
	p.mu.RLock()
	if !p.running {
		p.mu.RUnlock()
		return nil
	}
	p.mu.RUnlock()

	err := syscall.Kill(p.pid, syscall.SIGKILL)
	if err != nil && !errors.Is(err, syscall.ESRCH) {
		return fmt.Errorf("failed to send SIGKILL: %w", err)
	}

	<-p.exitCh
	return nil
}

// Signal sends an OS signal to the running QEMU process.
func (p *Process) Signal(sig os.Signal) error {
	p.mu.RLock()
	if !p.running {
		p.mu.RUnlock()
		return ErrProcessNotRunning
	}
	p.mu.RUnlock()

	return p.cmd.Process.Signal(sig)
}

// ExitChan returns a channel that receives the ExitResult when the process terminates.
func (p *Process) ExitChan() <-chan ExitResult {
	return p.exitCh
}
