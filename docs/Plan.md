# MiniVM

A minimal, lightweight, single-node virtual machine manager built in Go around QEMU/KVM.

The goal is to provide a small self-hosted VM management appliance with a polished but lightweight web interface, similar in concept to a very small subset of Proxmox.

MiniVM is **not** intended to reproduce Proxmox's advanced functionality.

The project should prioritize:

* Low RAM usage
* Low CPU overhead
* Minimal dependencies
* Single Go binary
* Embedded frontend
* QEMU/KVM
* Persistent VM storage
* Simple VM lifecycle management
* Browser-based serial console
* Simple networking
* Easy Docker deployment

---

# 1. Core Architecture

```text
                         Browser
                            │
                            │ HTTP
                            │ WebSocket
                            ▼
                 ┌─────────────────────┐
                 │      MiniVM          │
                 │    Go Binary         │
                 │                     │
                 │  HTTP Server        │
                 │  REST API           │
                 │  WebSocket          │
                 │  VM Manager         │
                 │  QEMU Manager       │
                 │  Storage Manager    │
                 │  Config Manager     │
                 └──────────┬──────────┘
                            │
                 ┌──────────┴──────────┐
                 │                     │
                 ▼                     ▼
              QEMU/KVM             VM Storage
                 │                     │
        ┌────────┼────────┐            │
        ▼        ▼        ▼            ▼
       VM1      VM2      VM3       QCOW2 disks
```

The Go binary is responsible for:

* HTTP server
* API
* frontend serving
* VM lifecycle
* QEMU process management
* QMP communication
* console WebSockets
* storage operations
* configuration
* basic host statistics

---

# 2. Design Principles

## 2.1 Single Binary

The final application must compile into one executable:

```text
minivm
```

The binary should contain:

```text
Go backend
HTML templates
CSS
JavaScript
SVG icons
frontend assets
```

Use Go's `embed` package:

```go
//go:embed web/*
var webFS embed.FS
```

No Node.js runtime should be required.

No frontend build system should be required for the normal build.

---

# 3. Technology Stack

## Backend

```text
Go
```

Use the Go standard library wherever practical.

Prefer:

```text
net/http
html/template
embed
encoding/json
os
os/exec
context
sync
syscall
net
io
bufio
time
log/slog
```

Avoid unnecessary frameworks.

---

# 4. External Runtime Dependencies

MiniVM itself should remain a single binary, but the host/container needs:

```text
QEMU
QEMU utilities
KVM
Linux kernel
```

Recommended base image:

```text
debian:stable-slim
```

Install only required QEMU packages.

The Docker container should not contain:

* Node.js
* npm
* Python
* nginx
* Apache
* PostgreSQL
* Redis
* systemd

unless a future feature explicitly requires them.

---

# 5. Frontend

The frontend should be modern and polished while remaining lightweight.

Use:

```text
HTML
CSS
HTMX
Vanilla JavaScript
xterm.js
```

Do not use:

```text
React
Vue
Angular
Next.js
Nuxt
Electron
Tauri
Tailwind build pipeline
large UI frameworks
```

HTMX should be used for simple dynamic interactions.

Vanilla JavaScript should handle:

* console
* keyboard interactions
* modals
* small UI state
* confirmation dialogs
* WebSocket connection
* terminal resize

---

# 6. Frontend Design

The UI should be:

* Dark-first
* Clean
* Modern
* Dense but readable
* Responsive
* Desktop-oriented
* Usable on tablets
* Minimal animations

Avoid excessive:

* gradients
* shadows
* animations
* huge cards
* rounded elements everywhere
* decorative elements

The interface should feel like a professional server-management application.

---

# 7. Main UI Layout

```text
┌──────────────────────────────────────────────────────────────┐
│ MiniVM                                      ● Host Online     │
├──────────────┬───────────────────────────────────────────────┤
│              │                                               │
│ Overview     │ Virtual Machines                 [+ Create]   │
│              │                                               │
│ VMs          │ ┌───────────────────────────────────────────┐ │
│              │ │ ● ubuntu-server              Running      │ │
│ Storage      │ │                                           │ │
│              │ │ 2 vCPU    4 GB RAM    40 GB              │ │
│              │ │                                           │ │
│ Settings     │ │ [Console] [Shutdown] [More]              │ │
│              │ └───────────────────────────────────────────┘ │
│              │                                               │
│              │ ┌───────────────────────────────────────────┐ │
│              │ │ ○ debian-test                Stopped      │ │
│              │ │                                           │ │
│              │ │ 2 vCPU    2 GB RAM    20 GB              │ │
│              │ │                                           │ │
│              │ │ [Start] [Edit] [More]                     │ │
│              │ └───────────────────────────────────────────┘ │
└──────────────┴───────────────────────────────────────────────┘
```

---

# 8. Frontend Pages

Implement:

```text
/
    Dashboard

/vms
    VM list

/vms/new
    Create VM

/vms/{id}
    VM details

/vms/{id}/console
    VM console

/storage
    ISO and disk storage

/settings
    Application settings
```

---

# 9. Dashboard

Show:

```text
VM count
Running VMs
Stopped VMs
Host CPU usage
Host memory usage
Storage usage
```

Do not implement complex charts initially.

Use:

* simple numbers
* progress bars
* status indicators

---

# 10. VM Cards

Each VM card should show:

```text
VM name
OS/type
Status
vCPU count
RAM
Disk size
CPU usage
Memory usage
```

Actions:

```text
Start
Shutdown
Restart
Force Stop
Console
Edit
Delete
```

Only show actions appropriate for the VM state.

---

# 11. VM Detail Page

Display:

```text
Name
Status
Uptime
PID
CPU
Memory
Disk
Network
```

Sections:

```text
Overview
Resources
Storage
Network
Console
```

Keep the first implementation simple.

---

# 12. VM Creation Wizard

Create a four-step wizard.

```text
General
   ↓
Resources
   ↓
Storage
   ↓
Review
```

## General

Fields:

```text
VM Name
Operating System
ISO
```

## Resources

Fields:

```text
vCPU
RAM
```

## Storage

Fields:

```text
Disk size
Disk format
```

Default:

```text
QCOW2
```

## Network

Fields:

```text
Enable network
SSH port
```

Use QEMU user-mode networking initially.

---

# 13. VM Configuration

Store configuration as JSON.

Example:

```json
{
  "id": "ubuntu-server",
  "name": "ubuntu-server",
  "cpus": 2,
  "memory_mb": 4096,
  "disk": "disk.qcow2",
  "disk_format": "qcow2",
  "iso": "ubuntu-24.04.iso",
  "network": {
    "enabled": true,
    "ssh_port": 2222
  }
}
```

Configuration path:

```text
/var/lib/minivm/vms/<vm-id>/config.json
```

---

# 14. VM Storage Layout

Use:

```text
/var/lib/minivm/
│
├── config/
│
├── vms/
│   ├── ubuntu-server/
│   │   ├── config.json
│   │   ├── disk.qcow2
│   │   ├── qmp.sock
│   │   ├── console.sock
│   │   ├── vnc.sock
│   │   └── logs/
│   │
│   └── debian-test/
│       ├── config.json
│       ├── disk.qcow2
│       ├── qmp.sock
│       ├── console.sock
│       ├── vnc.sock
│       └── logs/
│
└── iso/
```

Make the storage root configurable.

Default:

```text
/var/lib/minivm
```

---

# 15. VM State

Do not rely solely on configuration files for runtime state.

Track runtime state in memory.

Example:

```go
type VMState string

const (
    StateStopped  VMState = "stopped"
    StateStarting VMState = "starting"
    StateRunning  VMState = "running"
    StateStopping VMState = "stopping"
    StateError    VMState = "error"
)
```

The manager should maintain:

```go
type VMRuntime struct {
    PID       int
    State     VMState
    StartedAt time.Time
}
```

---

# 16. VM Manager

Create a central manager:

```go
type Manager struct {
    mu  sync.RWMutex
    vms map[string]*VM
}
```

Responsibilities:

```text
Discover VMs
Create VM
Delete VM
Start VM
Stop VM
Shutdown VM
Restart VM
Get VM
List VMs
Recover running VMs
```

Important:

All lifecycle operations must be concurrency-safe.

Do not allow:

```text
start + start
start + delete
stop + delete
```

to race against each other.

---

# 17. QEMU Manager

Create a dedicated QEMU package.

Responsibilities:

```text
Build QEMU arguments
Start QEMU
Stop QEMU
Shutdown QEMU
Connect QMP
Connect serial console
Monitor process
Capture logs
```

Example:

```go
type QEMU struct {
    Cmd     *exec.Cmd
    PID     int
    QMPPath string
}
```

---

# 18. QEMU Command

Initial QEMU configuration:

```bash
qemu-system-x86_64 \
    -enable-kvm \
    -machine q35,accel=kvm \
    -cpu host \
    -smp 2 \
    -m 4096 \
    -drive file=disk.qcow2,format=qcow2,if=virtio \
    -drive file=installer.iso,media=cdrom \
    -boot order=c \
    -device virtio-net-pci,netdev=net0 \
    -netdev user,id=net0,hostfwd=tcp::2222-:22 \
    -qmp unix:qmp.sock,server=on,wait=off \
    -serial unix:console.sock,server=on,wait=off \
    -vga virtio \
    -vnc unix:vnc.sock
```

Arguments must be generated programmatically.

Never construct commands through a shell.

Use:

```go
exec.CommandContext(
    ctx,
    "qemu-system-x86_64",
    args...,
)
```

This prevents shell injection.

---

# 19. KVM Detection

At startup check:

```text
/dev/kvm
```

If unavailable, display a clear error:

```text
KVM acceleration is unavailable.

Make sure:
- virtualization is enabled in BIOS/UEFI
- /dev/kvm exists
- the container has access to /dev/kvm
```

Do not silently assume KVM works.

---

# 20. QMP

Implement the minimum QEMU Machine Protocol functionality.

Required:

```text
connect
qmp_capabilities
query-status
system_powerdown
quit
```

Optional:

```text
query-cpus
query-memory-size-summary
query-block
```

Use QMP for proper VM lifecycle control.

Do not use `kill -9` for normal shutdown.

Lifecycle:

```text
Shutdown
   ↓
QMP system_powerdown
   ↓
Wait
   ↓
Process exits
```

Force Stop:

```text
SIGTERM
   ↓
wait
   ↓
SIGKILL if necessary
```

---

# 21. QMP Implementation

Implement a small JSON protocol client.

Do not add a large QMP dependency unless absolutely necessary.

Required operations:

```go
Connect()
Execute(command string, arguments map[string]any)
QueryStatus()
Shutdown()
Quit()
```

Handle:

```text
QMP greeting
command responses
events
timeouts
connection failures
```

---

# 22. Serial Console

Use QEMU's serial socket:

```text
console.sock
```

Browser connection:

```text
Browser
   │
   │ WebSocket
   ▼
Go WebSocket handler
   │
   │ Unix socket
   ▼
QEMU
```

The backend should bridge:

```text
WebSocket → QEMU
QEMU → WebSocket
```

Do not create a shell inside the container.

The browser console should interact with the actual VM serial console.

---

# 23. WebSocket Console & Graphical Display

Endpoints:

```text
WS /api/v1/vms/{id}/console   (Text / Serial console via xterm.js)
WS /api/v1/vms/{id}/vnc       (Graphical display via embedded noVNC)
```

Implement:

```text
connect
read
write
disconnect
resize
```

### Serial Console (xterm.js)
* Bridges `WS /api/v1/vms/{id}/console` ↔ `console.sock`
* Interactive text-based serial stream.
* If using xterm.js, support terminal resizing where possible.
* Do not load xterm.js on pages that do not use the console.

### Graphical Display (Embedded noVNC)
* Bridges `WS /api/v1/vms/{id}/vnc` ↔ `vnc.sock` (RFB stream over WebSocket)
* Embed noVNC client assets into the single binary (under `web/vendor/novnc/`) with zero external runtime proxies (e.g. no external websockify daemon needed).
* Full graphical frame interaction with keyboard, mouse pointer, and resolution scaling.
* Support switching between Graphical (noVNC) and Serial (xterm.js) on `/vms/{id}/console`.

---

# 24. REST API

Prefix:

```text
/api/v1
```

Endpoints:

```text
GET    /api/v1/health

GET    /api/v1/vms
POST   /api/v1/vms

GET    /api/v1/vms/{id}
DELETE /api/v1/vms/{id}

POST   /api/v1/vms/{id}/start
POST   /api/v1/vms/{id}/shutdown
POST   /api/v1/vms/{id}/restart
POST   /api/v1/vms/{id}/stop

GET    /api/v1/vms/{id}/status

WS     /api/v1/vms/{id}/console
```

Use JSON for API responses.

Example:

```json
{
  "id": "ubuntu-server",
  "status": "running",
  "cpus": 2,
  "memory_mb": 4096
}
```

---

# 25. HTTP Server

Use:

```go
net/http
```

Avoid a large web framework.

Configure:

```text
ReadTimeout
WriteTimeout
IdleTimeout
MaxHeaderBytes
```

Serve:

```text
frontend
API
WebSocket
```

from the same server.

---

# 26. Frontend/API Separation

The frontend must not directly execute QEMU commands.

Correct:

```text
Browser
 ↓
HTTP API
 ↓
VM Manager
 ↓
QEMU
```

Incorrect:

```text
Browser
 ↓
shell
 ↓
QEMU
```

---

# 27. HTMX

Use HTMX for lightweight dynamic operations.

Examples:

```text
Start VM
Stop VM
Refresh status
Delete VM
Load VM cards
```

For example:

```html
<button
    hx-post="/api/v1/vms/ubuntu/start"
    hx-swap="outerHTML">
    Start
</button>
```

However, keep API endpoints JSON-compatible.

It is acceptable for HTML routes to call internal manager methods separately.

---

# 28. Frontend Assets

Use:

```text
web/
├── templates/
│   ├── layout.html
│   ├── dashboard.html
│   ├── vm.html
│   ├── create.html
│   ├── console.html
│   └── settings.html
│
├── static/
│   ├── css/
│   │   ├── base.css
│   │   ├── layout.css
│   │   ├── components.css
│   │   └── console.css
│   │
│   ├── js/
│   │   ├── app.js
│   │   └── console.js
│   │
│   └── icons/
│
└── vendor/
    ├── htmx.min.js
    ├── xterm/
    └── novnc/
```

Pin frontend dependency versions.

Do not require an internet connection for the frontend.

---

# 29. Icons

Use lightweight SVG icons.

Prefer:

```text
Lucide-style SVG icons
```

Do not use icon fonts.

Avoid shipping hundreds of unused icons.

Only include icons actually used by the interface.

---

# 30. Theme

Implement CSS variables:

```css
:root {
    --bg: ...;
    --panel: ...;
    --panel-hover: ...;
    --border: ...;
    --text: ...;
    --text-muted: ...;
    --accent: ...;
    --danger: ...;
    --warning: ...;
    --success: ...;
}
```

Support:

```text
Dark
Light
System
```

The default should be dark.

---

# 31. Settings

Keep settings minimal.

Implement:

```text
Theme
Accent color
Server port
Storage path
```

Potential future settings:

```text
Default CPU
Default RAM
Default disk format
```

Do not build an extensive configuration system initially.

---

# 32. Host Monitoring

Implement basic Linux host statistics.

Display:

```text
CPU usage
Memory usage
Disk usage
KVM availability
```

Do not add Prometheus initially.

Do not run a heavyweight monitoring stack.

Read basic information from:

```text
/proc
/sys
syscall/statfs
```

where practical.

---

# 33. VM Monitoring

For running VMs, optionally retrieve:

```text
CPU state
Memory
Disk information
```

using QMP where practical.

If accurate metrics require significantly more complexity, defer them.

The VM lifecycle must not depend on monitoring.

---

# 34. ISO Management

Implement a simple ISO directory:

```text
/var/lib/minivm/iso
```

Frontend:

```text
Storage
│
├── ubuntu.iso
├── debian.iso
└── alpine.iso
```

Initially support:

```text
List ISO files
Delete ISO
Select ISO when creating VM
```

Uploading large ISO files should use streaming uploads.

Do not load ISO files into RAM.

---

# 35. Disk Creation

Use:

```text
qemu-img
```

Example:

```bash
qemu-img create -f qcow2 disk.qcow2 20G
```

The Go program should invoke:

```go
exec.Command(
    "qemu-img",
    "create",
    "-f",
    "qcow2",
    path,
    size,
)
```

Validate disk sizes before execution.

Never accept arbitrary command arguments from users.

---

# 36. Input Validation

Validate:

```text
VM ID
VM name
CPU count
RAM
disk size
ports
ISO filenames
paths
```

VM names should allow only safe characters.

Example:

```regex
^[a-zA-Z0-9][a-zA-Z0-9._-]{0,63}$
```

Never allow user-provided paths to escape:

```text
/var/lib/minivm/vms/
```

Prevent:

```text
../
absolute paths
symlink traversal
```

where applicable.

---

# 37. Security

Initial version is intended for trusted/local networks.

Still implement basic security:

```text
No shell execution
Input validation
Path validation
Safe QEMU arguments
HTTP request limits
Upload limits
WebSocket connection limits
```

Do not expose privileged APIs unnecessarily.

QEMU should ideally run with reduced privileges where feasible.

Do not automatically grant the container unrestricted host access beyond what is required.

---

# 38. Docker

Create:

```text
Dockerfile
docker-compose.yml
```

Example runtime:

```bash
docker run \
    --device=/dev/kvm \
    -p 8080:8080 \
    -v minivm-data:/var/lib/minivm \
    minivm
```

The container needs:

```text
/dev/kvm
```

Networking capabilities should only be added if actually required.

Do not use:

```text
--privileged
```

by default.

If a feature genuinely requires additional capabilities, document exactly why.

---

# 39. Docker Image

Keep the final image small.

Multi-stage build:

```text
Stage 1
Go compiler
     ↓
minivm binary

Stage 2
Debian slim
     ↓
QEMU runtime
     ↓
minivm binary
```

Do not ship:

```text
Go compiler
Git
npm
Node.js
build tools
source code
```

in the final image.

---

# 40. Entrypoint

Create:

```text
/docker/entrypoint.sh
```

Responsibilities:

```text
Check /dev/kvm
Create required directories
Validate configuration
Start minivm
```

Do not use systemd.

Example:

```text
container
   ↓
entrypoint.sh
   ↓
minivm
```

MiniVM should remain PID 1 if practical.

Handle:

```text
SIGTERM
SIGINT
```

and gracefully stop managed VMs.

---

# 41. Shutdown Behavior

When the container receives SIGTERM:

```text
MiniVM
  ↓
Stop accepting new requests
  ↓
Request VM shutdown
  ↓
Wait for VMs
  ↓
Force stop remaining VMs
  ↓
Exit
```

Do not leave orphaned QEMU processes.

---

# 42. VM Recovery

When MiniVM starts:

```text
Scan /var/lib/minivm/vms/
```

For each VM:

```text
Read config
Check runtime artifacts
Check whether QEMU is actually running
Clean stale PID/socket files
```

Do not automatically restart VMs unless an explicit configuration option is implemented.

Initial default:

```text
VMs remain stopped after MiniVM restart.
```

---

# 43. Logging

Use:

```go
log/slog
```

Log:

```text
server startup
VM creation
VM start
VM shutdown
VM stop
QEMU errors
QMP errors
console errors
storage errors
```

Support:

```text
text
json
```

logging formats if easy.

Do not log:

```text
passwords
private configuration
VM console contents
```

---

# 44. Error Handling

API errors should have consistent JSON:

```json
{
  "error": {
    "code": "VM_NOT_FOUND",
    "message": "VM does not exist"
  }
}
```

Create internal error types where useful.

HTTP status examples:

```text
400 invalid input
404 VM not found
409 VM state conflict
500 internal error
503 QEMU/KVM unavailable
```

---

# 45. Concurrency

The following operations must be safe:

```text
Start VM
Stop VM
Delete VM
Console connections
Status polling
VM discovery
```

Use:

```go
sync.RWMutex
context.Context
channels
```

only where needed.

Avoid unnecessary goroutines.

Every goroutine must have a clear lifecycle.

---

# 46. Process Management

When launching QEMU:

```text
Create context
Create exec.Cmd
Configure stdin/stdout/stderr
Create QMP socket
Create serial socket
Start process
Record PID
Monitor process
```

When QEMU exits:

```text
Update VM state
Close sockets
Clean runtime state
Record exit status
```

The process monitor must not leak goroutines.

---

# 47. API Authentication

For the initial local/trusted deployment, authentication can be optional.

However, structure the HTTP server so authentication can be added later.

Do not hard-code authentication logic throughout handlers.

Create middleware:

```text
Request
 ↓
Auth middleware
 ↓
API handler
```

Future support:

```text
password
API token
reverse proxy authentication
```

---

# 48. Networking

Initial networking:

```text
QEMU user-mode networking
```

Example:

```text
VM
 ↓
QEMU NAT
 ↓
Host port forwarding
```

Support:

```text
SSH port
custom TCP ports
```

Do not implement Linux bridges initially.

Future:

```text
tap
bridge
VLAN
```

are out of scope.

---

# 49. VM Networking Configuration

Example:

```json
{
  "network": {
    "enabled": true,
    "mode": "user",
    "ports": [
      {
        "host": 2222,
        "guest": 22,
        "protocol": "tcp"
      }
    ]
  }
}
```

Validate ports:

```text
1-65535
```

Prevent duplicate host ports.

---

# 50. File Structure

Use:

```text
minivm/
│
├── cmd/
│   └── minivm/
│       └── main.go
│
├── internal/
│   ├── api/
│   │   ├── server.go
│   │   ├── routes.go
│   │   ├── handlers.go
│   │   └── errors.go
│   │
│   ├── vm/
│   │   ├── manager.go
│   │   ├── model.go
│   │   ├── lifecycle.go
│   │   └── recovery.go
│   │
│   ├── qemu/
│   │   ├── qemu.go
│   │   ├── args.go
│   │   ├── process.go
│   │   ├── qmp.go
│   │   └── console.go
│   │
│   ├── storage/
│   │   ├── storage.go
│   │   ├── disk.go
│   │   └── iso.go
│   │
│   ├── host/
│   │   ├── cpu.go
│   │   ├── memory.go
│   │   ├── disk.go
│   │   └── kvm.go
│   │
│   ├── config/
│   │   └── config.go
│   │
│   └── websocket/
│       └── console.go
│
├── web/
│   ├── templates/
│   ├── static/
│   └── embed.go
│
├── Dockerfile
├── docker-compose.yml
├── Makefile
├── README.md
├── LICENSE
└── plan.md
```

---

# 51. Go Packages

Keep package boundaries meaningful.

Avoid:

```text
internal/utils
internal/helpers
internal/common
```

unless genuinely necessary.

Prefer packages based on actual responsibilities:

```text
vm
qemu
storage
api
host
config
```

---

# 52. CLI

Even though the primary interface is web-based, provide a tiny CLI.

Examples:

```bash
minivm version
minivm serve
minivm list
minivm start ubuntu
minivm stop ubuntu
minivm shutdown ubuntu
minivm status ubuntu
```

The CLI must use the same internal VM manager.

Do not implement separate VM logic for CLI and API.

---

# 53. Server Configuration

Support flags:

```text
--listen
--port
--data-dir
--log-level
```

Example:

```bash
minivm serve \
    --listen 0.0.0.0 \
    --port 8080 \
    --data-dir /var/lib/minivm
```

Environment variables may also be supported:

```text
MINIVM_LISTEN
MINIVM_PORT
MINIVM_DATA_DIR
MINIVM_LOG_LEVEL
```

---

# 54. Testing

Implement unit tests for:

```text
VM config validation
VM name validation
disk size validation
port validation
QEMU argument generation
QMP parsing
storage paths
API handlers
VM state transitions
```

Test QEMU argument generation independently:

```text
config
   ↓
[]string
```

This makes it easy to test without starting QEMU.

---

# 55. Integration Tests

When KVM is available:

```text
Create VM
Create disk
Start VM
Query status
Connect console
Shutdown VM
Verify process exited
```

Do not require KVM for ordinary unit tests.

Tests that need KVM should be explicitly marked/skipped when unavailable.

---

# 56. Security Tests

Test:

```text
../ path traversal
absolute path injection
invalid VM IDs
invalid ports
invalid memory
invalid CPU counts
duplicate VM creation
duplicate ports
concurrent start requests
delete running VM
```

---

# 57. Performance Goals

The application should remain lightweight.

Target:

```text
Single Go process
Minimal idle CPU
Low memory overhead
No background polling more frequently than necessary
No unnecessary goroutines
No database
No Node runtime
```

The VM's RAM usage is separate from MiniVM's own memory footprint.

Avoid polling QEMU every few milliseconds.

Use reasonable intervals such as:

```text
1–5 seconds
```

for dashboard metrics.

---

# 58. No Database

Do not use:

```text
SQLite
PostgreSQL
MySQL
Redis
```

for the initial version.

VM configuration is stored in JSON files.

Runtime state stays in memory.

This keeps the system:

```text
portable
debuggable
backup-friendly
simple
```

---

# 59. No Background Daemon

MiniVM itself should be the service.

Do not require:

```text
systemd
supervisord
pm2
```

Docker manages the process lifecycle.

---

# 60. Build

Standard build:

```bash
go build -trimpath -ldflags="-s -w" -o minivm ./cmd/minivm
```

The result:

```text
minivm
```

must be a self-contained Go executable containing the frontend.

---

# 61. Version Information

Embed:

```text
version
commit
build date
```

using Go linker flags.

CLI:

```bash
minivm version
```

Web UI:

```text
MiniVM v0.x.x
```

---

# 62. Makefile

Provide:

```text
make build
make test
make run
make docker
make clean
```

Optional:

```text
make fmt
make vet
make lint
```

Do not make external linters mandatory for the basic build.

---

# 63. Docker Compose

Provide a simple example:

```yaml
services:
  minivm:
    image: minivm:latest
    devices:
      - /dev/kvm:/dev/kvm
    ports:
      - "8080:8080"
    volumes:
      - minivm-data:/var/lib/minivm

volumes:
  minivm-data:
```

Do not use:

```yaml
privileged: true
```

unless a feature specifically requires it.

---

# 64. README

README should explain:

```text
What MiniVM is
Architecture
Requirements
KVM requirement
Docker installation
Native installation
Creating a VM
Opening console
Storage
Networking
Security
Limitations
Development
```

Clearly state that MiniVM is not a full Proxmox replacement.

---

# 65. Implementation Order

Follow this order.

## Phase 1 — Project foundation

Implement:

```text
Go module
Directory structure
Configuration
Logging
CLI
Embedded frontend
Basic HTTP server
```

Verify:

```bash
minivm serve
```

opens the dashboard.

---

## Phase 2 — Storage

Implement:

```text
data directory
VM directories
config.json
ISO directory
QCOW2 creation
```

Test creating a VM without starting QEMU.

---

## Phase 3 — QEMU

Implement:

```text
QEMU argument builder
KVM detection
QEMU process launcher
PID tracking
process monitoring
```

Start a real VM.

---

## Phase 4 — VM Lifecycle

Implement:

```text
Create
Start
Shutdown
Stop
Restart
Delete
Status
```

All operations should work through the CLI first.

---

## Phase 5 — QMP

Implement:

```text
QMP socket
handshake
qmp_capabilities
query-status
system_powerdown
quit
```

Replace unsafe/process-based lifecycle operations with QMP where appropriate.

---

## Phase 6 — REST API

Implement:

```text
GET /vms
POST /vms
GET /vms/{id}
DELETE /vms/{id}

POST /start
POST /shutdown
POST /restart
POST /stop
GET /status
```

---

## Phase 7 — Dashboard

Build:

```text
sidebar
dashboard
VM cards
status indicators
resource display
actions
```

Use HTMX where useful.

---

## Phase 8 — VM Creation UI

Implement:

```text
Create VM wizard
CPU
RAM
disk
ISO
network
review
```

---

## Phase 9 — Browser Console & Graphical Display

Implement:

```text
QEMU serial socket (console.sock)
QEMU VNC socket (vnc.sock via -vnc unix:vnc.sock)
WebSocket bridge (Serial console & VNC RFB streams in pure Go)
Embedded noVNC into binary (web/vendor/novnc/ via web/embed.go)
xterm.js integration (web/vendor/xterm/)
Dual-mode console page (/vms/{id}/console with Graphical VNC & Serial xterm)
Fullscreen toggle
Connection state & auto-reconnect
```

### Architecture & Implementation Steps:

1. **QEMU VNC UNIX Domain Socket**:
   - Add `-vga virtio -vnc unix:<vm-storage-dir>/vnc.sock` to QEMU argument builder.
   - Clean, secure local socket communication without opening unauthenticated host TCP ports.

2. **Native Go WebSocket Bridge (Zero External Dependencies)**:
   - Implement WebSocket upgrade and RFC 6455 framing in `internal/websocket`.
   - `WS /api/v1/vms/{id}/vnc`: Bi-directional bridge between WebSocket binary messages and QEMU's `vnc.sock` (RFB protocol over WebSocket), replacing the need for external tools like `websockify`.
   - `WS /api/v1/vms/{id}/console`: Bi-directional bridge between WebSocket text/binary messages and QEMU's `console.sock` for serial text console.

3. **Embedded noVNC Client**:
   - Place noVNC web assets under `web/vendor/novnc/`.
   - Embed into the single Go binary using Go 1.16+ `//go:embed` in `web/embed.go`.
   - 100% offline and self-contained; no CDN or external internet required.

4. **Interactive Console UI (`/vms/{id}/console`)**:
   - Switchable view tabs: **Graphical Console (noVNC)** and **Serial Console (xterm.js)**.
   - Fullscreen mode, keyboard/mouse capture, status indicator (`Connected`, `Connecting`, `Disconnected`), and disconnect/reconnect controls.

---

## Phase 10 — Monitoring

Add:

```text
host CPU
host memory
disk usage
VM uptime
basic VM metrics
```

Do not compromise VM stability for monitoring.

---

## Phase 11 — Docker

Create:

```text
multi-stage Dockerfile
entrypoint
compose file
KVM checks
persistent storage
```

Test:

```bash
docker run --device=/dev/kvm ...
```

---

## Phase 12 — Recovery and Hardening

Implement:

```text
startup VM discovery
stale state cleanup
signal handling
graceful shutdown
input validation
security checks
concurrency checks
```

---

# 66. MVP Feature Set

The first usable release must support:

```text
✓ Single Go binary
✓ Embedded frontend
✓ Docker image
✓ QEMU/KVM
✓ Create VM
✓ Delete VM
✓ Start VM
✓ Shutdown VM
✓ Force stop VM
✓ Restart VM
✓ VM status
✓ Persistent QCOW2 disks
✓ ISO selection
✓ Basic user-mode networking
✓ Port forwarding
✓ Browser serial console
✓ QMP
✓ Basic host monitoring
✓ Dark/light theme
```

---

# 67. Explicitly Out of Scope

Do NOT implement during MVP:

```text
✗ Clustering
✗ HA
✗ Live migration
✗ Ceph
✗ ZFS management
✗ Distributed storage
✗ VM snapshots
✗ Backup system
✗ VLAN management
✗ Linux bridge management
✗ SDN
✗ GPU passthrough UI
✗ PCI passthrough UI
✗ USB passthrough UI
✗ Docker/LXC management
✗ Kubernetes
✗ User accounts
✗ Multi-tenancy
✗ Billing
✗ Plugin system
✗ Marketplace
```

These may be considered later but should not influence the MVP architecture.

---

# 68. Important Engineering Rules

Antigravity must follow these rules throughout implementation.

### Rule 1

Prefer the Go standard library.

### Rule 2

Do not introduce a dependency merely for convenience.

### Rule 3

Never execute user-provided strings through a shell.

### Rule 4

Never trust VM configuration input.

### Rule 5

Do not run QEMU through `sh -c`.

### Rule 6

Do not require Node.js.

### Rule 7

Do not create a separate frontend application.

### Rule 8

Do not introduce a database.

### Rule 9

Keep frontend JavaScript minimal.

### Rule 10

Do not implement advanced virtualization features until the MVP is stable.

### Rule 11

Every goroutine must have a clear termination path.

### Rule 12

Every QEMU process must be tracked.

### Rule 13

MiniVM must cleanly handle QEMU crashes.

### Rule 14

MiniVM must never silently claim a VM is running when QEMU is not running.

### Rule 15

Graceful shutdown must be preferred over forced termination.

---

# 69. Definition of Done

The MVP is complete when the following workflow works:

```text
Start Docker container
        ↓
Open http://localhost:8080
        ↓
Dashboard loads
        ↓
Click "Create VM"
        ↓
Select ISO
        ↓
Set CPU/RAM/disk
        ↓
Create VM
        ↓
VM appears as "Stopped"
        ↓
Click Start
        ↓
QEMU starts using KVM
        ↓
VM becomes "Running"
        ↓
Click Console
        ↓
Browser serial console opens
        ↓
Interact with VM
        ↓
Return to dashboard
        ↓
Click Shutdown
        ↓
QMP requests guest shutdown
        ↓
VM becomes "Stopped"
```

The entire system should run as:

```text
Docker
  ↓
minivm
  ↓
QEMU/KVM
```

with no other application services required.

---

# 70. Final Target

The finished project should feel like:

```text
              MiniVM
                │
       ┌────────┴─────────┐
       │                  │
    Web UI              CLI
       │                  │
       └────────┬─────────┘
                │
           Go VM Manager
                │
       ┌────────┼─────────┐
       │        │         │
      QMP     Storage   Console
       │                  │
       ▼                  ▼
     QEMU              WebSocket
       │                  │
       ▼                  ▼
      KVM              xterm.js
       │
       ▼
      VM
```

The core philosophy is:

**Small binary. Small image. Small dependency tree. Simple architecture. Real QEMU/KVM. Useful web UI.**

Do not attempt to recreate Proxmox internally. Build the smallest system that provides the core VM-hosting workflow cleanly and reliably.
