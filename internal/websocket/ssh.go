package websocket

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"strconv"
)

// SSHOptions configures the backend PTY session for SSH or shell access.
type SSHOptions struct {
	VMID    string
	VMName  string
	VMDir   string
	SSHPort int
	User    string
	Mode    string // "ssh" or "shell"
	Logger  *slog.Logger
}

// HandleSSH upgrades an HTTP request to a WebSocket and connects it to an interactive PTY session
// running an SSH client to the VM or dropping to a host management shell.
func HandleSSH(w http.ResponseWriter, r *http.Request, opts SSHOptions) error {
	ws, err := Upgrade(w, r, nil)
	if err != nil {
		return err
	}

	var cmd *exec.Cmd

	if opts.Mode == "shell" || opts.SSHPort <= 0 {
		shell := os.Getenv("SHELL")
		if shell == "" {
			shell = "/bin/bash"
			if _, err := os.Stat(shell); err != nil {
				shell = "/bin/sh"
			}
		}

		cmd = exec.Command(shell)
		cmd.Dir = opts.VMDir
		cmd.Env = append(os.Environ(),
			"TERM=xterm-256color",
			"VM_ID="+opts.VMID,
			"VM_NAME="+opts.VMName,
			"VM_DIR="+opts.VMDir,
			fmt.Sprintf("VM_SSH_PORT=%d", opts.SSHPort),
		)
	} else {
		user := opts.User
		if user == "" {
			user = "root"
		}
		target := fmt.Sprintf("%s@127.0.0.1", user)
		cmd = exec.Command("ssh",
			"-o", "StrictHostKeyChecking=no",
			"-o", "UserKnownHostsFile=/dev/null",
			"-o", "LogLevel=ERROR",
			"-p", strconv.Itoa(opts.SSHPort),
			target,
		)
		cmd.Dir = opts.VMDir
		cmd.Env = append(os.Environ(), "TERM=xterm-256color")
	}

	session, err := StartPTY(cmd, 24, 80)
	if err != nil {
		errMsg := fmt.Sprintf("\r\n\x1b[31m[TinyVM Error] Failed to start terminal session: %v\x1b[0m\r\n", err)
		_ = ws.WriteMessage(OpcodeText, []byte(errMsg))
		_ = ws.Close()
		return err
	}

	// Send welcoming / guidance banner to the WebSocket client
	if opts.Mode == "shell" {
		banner := fmt.Sprintf("\r\n\x1b[1;32m[TinyVM Management Shell]\x1b[0m Scoped to VM \x1b[1m%s\x1b[0m (ID: %s)\r\n", opts.VMName, opts.VMID)
		if opts.SSHPort > 0 {
			user := opts.User
			if user == "" {
				user = "root"
			}
			banner += fmt.Sprintf("\x1b[90mTip: To SSH into guest directly, run: ssh -p %d %s@127.0.0.1\x1b[0m\r\n\r\n", opts.SSHPort, user)
		} else {
			banner += "\x1b[33mNote: SSH port is not forwarded for this VM. Working in host shell.\x1b[0m\r\n\r\n"
		}
		_ = ws.WriteMessage(OpcodeText, []byte(banner))
	} else if opts.SSHPort <= 0 {
		notice := fmt.Sprintf("\r\n\x1b[33m[TinyVM] No SSH port forwarding configured for VM '%s'.\x1b[0m\r\n\x1b[36mDropped into backend host shell instead.\x1b[0m\r\n\r\n", opts.VMName)
		_ = ws.WriteMessage(OpcodeText, []byte(notice))
	}

	return BridgePTY(ws, session, opts.Logger)
}
