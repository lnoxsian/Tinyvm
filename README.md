# TinyVM

A minimal, lightweight, single-node virtual machine manager built in Go around QEMU/KVM.

TinyVM provides a small self-hosted VM management appliance with a clean, dark-first web interface (similar in concept to a small, single-node subset of Proxmox).

---

## Features (MVP Goals)

* **Single Go Binary**: Completely self-contained executable with embedded frontend assets (`//go:embed`).
* **Zero Heavy Dependencies**: No Node.js runtime, no build pipelines, no external database.
* **Direct QEMU / KVM Management**: Real hardware virtualization via Linux `/dev/kvm`.
* **Graceful Lifecycle via QMP**: Uses QEMU Machine Protocol over Unix sockets.
* **Browser Serial Console**: Interactive xterm.js terminal over WebSockets.
* **Persistent Storage**: QCOW2 disk images and ISO management.
* **Lightweight Host Monitoring**: Non-invasive metrics from `/proc` and `sysfs`.

---

## Quick Start

### Build & Run

```bash
# Build the single tinyvm binary
make build

# Start the server (default on 0.0.0.0:8080)
./tinyvm serve
```

Access the dashboard at `http://localhost:8080`.

### CLI Usage

```bash
tinyvm version       # Display version and build info
tinyvm serve         # Run the web server and REST API
tinyvm list          # List managed VMs
tinyvm start <vm>    # Start a VM
tinyvm stop <vm>     # Force stop a VM
tinyvm shutdown <vm> # Gracefully ACPI shutdown a VM
tinyvm status <vm>   # Query VM state
```

### Configuration

TinyVM can be configured via flags or environment variables:

| Flag | Env Var | Default | Description |
|------|---------|---------|-------------|
| `-listen` | `TINYVM_LISTEN` | `0.0.0.0` | Bind IP address |
| `-port` | `TINYVM_PORT` | `8080` | HTTP listen port |
| `-data-dir` | `TINYVM_DATA_DIR` | `/var/lib/tinyvm` | Storage directory root |
| `-log-level` | `TINYVM_LOG_LEVEL` | `info` | Logging verbosity (`debug`, `info`, `warn`, `error`) |
| `-log-format` | `TINYVM_LOG_FORMAT` | `text` | Logger format (`text`, `json`) |

---

## Architecture

```text
               TinyVM (Go Single Binary)
                          │
       ┌──────────────────┴──────────────────┐
       │                                     │
   Web UI / HTMX                           REST API
       │                                     │
       └──────────────────┬──────────────────┘
                          │
                    Go VM Manager
                          │
       ┌──────────────────┼──────────────────┐
       ▼                  ▼                  ▼
      QMP              Storage             Console
  (Unix Socket)     (QCOW2 Disks)        (WebSocket)
       │                                     │
       ▼                                     ▼
   QEMU/KVM                              xterm.js
```

---

## License

[MIT](LICENSE)
