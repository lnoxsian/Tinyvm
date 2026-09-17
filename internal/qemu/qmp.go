package qemu

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"sync"
	"time"
)

var (
	ErrQMPNotConnected   = errors.New("QMP client is not connected")
	ErrQMPTimeout        = errors.New("QMP request timed out")
	ErrQMPClosed         = errors.New("QMP connection closed")
	ErrQMPSocketNotFound = errors.New("QMP socket does not exist")
)

// QMPError represents an error returned by QEMU over QMP.
type QMPError struct {
	Class string `json:"class"`
	Desc  string `json:"desc"`
}

func (e *QMPError) Error() string {
	return fmt.Sprintf("QMP error (%s): %s", e.Class, e.Desc)
}

// QMPStatusResult is the payload returned by query-status.
type QMPStatusResult struct {
	Running    bool   `json:"running"`
	Singlestep bool   `json:"singlestep"`
	Status     string `json:"status"` // "running", "paused", "shutdown", "prelaunch", etc.
}

// QMPCPUInfo represents CPU details returned by query-cpus-fast or query-cpus.
type QMPCPUInfo struct {
	CPUIndex int    `json:"cpu-index"`
	QOMPath  string `json:"qom-path,omitempty"`
	ThreadID int    `json:"thread-id,omitempty"`
	Halted   bool   `json:"halted,omitempty"`
}

// QMPMemorySummary represents memory sizing details returned by query-memory-size-summary.
type QMPMemorySummary struct {
	BaseMemory    int64 `json:"base-memory"`
	PluggedMemory int64 `json:"plugged-memory,omitempty"`
}

// QMPBlockInserted details the media inserted into a block device.
type QMPBlockInserted struct {
	File      string `json:"file"`
	NodeName  string `json:"node-name,omitempty"`
	Ro        bool   `json:"ro"`
	Drv       string `json:"drv"`
	Encrypted bool   `json:"encrypted,omitempty"`
}

// QMPBlockInfo represents block device info returned by query-block.
type QMPBlockInfo struct {
	Device    string            `json:"device"`
	Qdev      string            `json:"qdev,omitempty"`
	Type      string            `json:"type"`
	Removable bool              `json:"removable"`
	Locked    bool              `json:"locked"`
	Inserted  *QMPBlockInserted `json:"inserted,omitempty"`
}


// QMPEvent represents an asynchronous event emitted by QEMU.
type QMPEvent struct {
	Event     string          `json:"event"`
	Data      json.RawMessage `json:"data,omitempty"`
	Timestamp struct {
		Seconds      int64 `json:"seconds"`
		Microseconds int64 `json:"microseconds"`
	} `json:"timestamp"`
}

// QMPGreeting contains QEMU version and capability details received upon connection.
type QMPGreeting struct {
	QMP struct {
		Version struct {
			QEMU struct {
				Major int `json:"major"`
				Minor int `json:"minor"`
				Micro int `json:"micro"`
			} `json:"qemu"`
			Package string `json:"package"`
		} `json:"version"`
		Capabilities []string `json:"capabilities"`
	} `json:"QMP"`
}

// qmpRequest represents an outgoing QMP command.
type qmpRequest struct {
	Execute   string `json:"execute"`
	Arguments any    `json:"arguments,omitempty"`
}

// qmpRawResponse captures raw responses and events received over QMP.
type qmpRawResponse struct {
	Return json.RawMessage `json:"return,omitempty"`
	Error  *QMPError       `json:"error,omitempty"`
	Event  string          `json:"event,omitempty"`
}

// QMPClient manages a connection to a QEMU instance via its QMP UNIX socket.
type QMPClient struct {
	socketPath string
	conn       net.Conn
	reader     *bufio.Reader
	mu         sync.Mutex
	closed     bool
	greeting   *QMPGreeting
	events     chan QMPEvent
	cmdTimeout time.Duration
}

// NewQMPClient creates an uninitialized QMP client for the specified UNIX domain socket.
func NewQMPClient(socketPath string) *QMPClient {
	return &QMPClient{
		socketPath: socketPath,
		events:     make(chan QMPEvent, 64),
		cmdTimeout: 5 * time.Second,
	}
}

// Connect dials the QMP socket, reads the greeting, and negotiates capabilities.
func (c *QMPClient) Connect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn != nil && !c.closed {
		return nil // already connected
	}

	deadline, hasDeadline := ctx.Deadline()
	if !hasDeadline {
		deadline = time.Now().Add(5 * time.Second)
	}

	var conn net.Conn
	var dialErr error

	// Retry loop to allow QEMU a moment to create and bind the UNIX socket
	for {
		if time.Now().After(deadline) {
			if _, err := os.Stat(c.socketPath); err != nil {
				return ErrQMPSocketNotFound
			}
			if dialErr != nil {
				return fmt.Errorf("%w: %v", ErrQMPTimeout, dialErr)
			}
			return ErrQMPTimeout
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		conn, dialErr = net.DialTimeout("unix", c.socketPath, 100*time.Millisecond)
		if dialErr == nil {
			break
		}

		time.Sleep(30 * time.Millisecond)
	}

	c.conn = conn
	c.reader = bufio.NewReader(conn)
	c.closed = false

	// 1. Read QMP Greeting
	_ = c.conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	greetingLine, err := c.reader.ReadBytes('\n')
	if err != nil {
		_ = c.conn.Close()
		c.conn = nil
		return fmt.Errorf("failed to read QMP greeting: %w", err)
	}

	var greeting QMPGreeting
	if err := json.Unmarshal(greetingLine, &greeting); err != nil {
		_ = c.conn.Close()
		c.conn = nil
		return fmt.Errorf("invalid QMP greeting format: %w", err)
	}
	c.greeting = &greeting

	// 2. Perform Handshake: Send qmp_capabilities
	req := qmpRequest{Execute: "qmp_capabilities"}
	reqBytes, _ := json.Marshal(req)
	reqBytes = append(reqBytes, '\n')

	_ = c.conn.SetWriteDeadline(time.Now().Add(3 * time.Second))
	if _, err := c.conn.Write(reqBytes); err != nil {
		_ = c.conn.Close()
		c.conn = nil
		return fmt.Errorf("failed to send qmp_capabilities: %w", err)
	}

	// 3. Read response for qmp_capabilities
	resp, err := c.readCommandResponseLocked(3 * time.Second)
	if err != nil {
		_ = c.conn.Close()
		c.conn = nil
		return fmt.Errorf("handshake failed: %w", err)
	}
	if resp.Error != nil {
		_ = c.conn.Close()
		c.conn = nil
		return resp.Error
	}

	return nil
}

// Execute sends a command and waits for its return payload, buffering asynchronous events.
func (c *QMPClient) Execute(command string, arguments any) (json.RawMessage, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn == nil || c.closed {
		return nil, ErrQMPNotConnected
	}

	req := qmpRequest{
		Execute:   command,
		Arguments: arguments,
	}
	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to encode QMP command: %w", err)
	}
	data = append(data, '\n')

	_ = c.conn.SetWriteDeadline(time.Now().Add(c.cmdTimeout))
	if _, err := c.conn.Write(data); err != nil {
		return nil, fmt.Errorf("failed to write QMP command: %w", err)
	}

	resp, err := c.readCommandResponseLocked(c.cmdTimeout)
	if err != nil {
		return nil, err
	}
	if resp.Error != nil {
		return nil, resp.Error
	}

	return resp.Return, nil
}

// readCommandResponseLocked reads lines until a command response (return or error) is received.
func (c *QMPClient) readCommandResponseLocked(timeout time.Duration) (*qmpRawResponse, error) {
	deadline := time.Now().Add(timeout)

	for {
		_ = c.conn.SetReadDeadline(deadline)
		line, err := c.reader.ReadBytes('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				c.closed = true
				return nil, ErrQMPClosed
			}
			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Timeout() {
				return nil, ErrQMPTimeout
			}
			return nil, fmt.Errorf("error reading QMP response: %w", err)
		}

		var raw qmpRawResponse
		if err := json.Unmarshal(line, &raw); err != nil {
			continue // ignore unparseable line
		}

		// Asynchronous event: dispatch and continue waiting for command response
		if raw.Event != "" {
			var event QMPEvent
			if err := json.Unmarshal(line, &event); err == nil {
				select {
				case c.events <- event:
				default:
					// Event channel full, drop oldest or overflow
				}
			}
			continue
		}

		// Found command response
		return &raw, nil
	}
}

// QueryStatus queries the current execution state of the QEMU guest.
func (c *QMPClient) QueryStatus() (*QMPStatusResult, error) {
	raw, err := c.Execute("query-status", nil)
	if err != nil {
		return nil, err
	}

	var status QMPStatusResult
	if err := json.Unmarshal(raw, &status); err != nil {
		return nil, fmt.Errorf("failed to parse query-status result: %w", err)
	}
	return &status, nil
}

// QueryCPUs retrieves the virtual CPUs configured in the running QEMU guest.
func (c *QMPClient) QueryCPUs() ([]QMPCPUInfo, error) {
	raw, err := c.Execute("query-cpus-fast", nil)
	if err != nil {
		raw, err = c.Execute("query-cpus", nil)
		if err != nil {
			return nil, err
		}
	}

	var cpus []QMPCPUInfo
	if err := json.Unmarshal(raw, &cpus); err != nil {
		return nil, fmt.Errorf("failed to parse CPU query response: %w", err)
	}
	return cpus, nil
}

// QueryMemorySizeSummary queries the guest memory sizing details.
func (c *QMPClient) QueryMemorySizeSummary() (*QMPMemorySummary, error) {
	raw, err := c.Execute("query-memory-size-summary", nil)
	if err != nil {
		return nil, err
	}

	var mem QMPMemorySummary
	if err := json.Unmarshal(raw, &mem); err != nil {
		return nil, fmt.Errorf("failed to parse memory summary response: %w", err)
	}
	return &mem, nil
}

// QueryBlock retrieves the list of block devices and attached disk images.
func (c *QMPClient) QueryBlock() ([]QMPBlockInfo, error) {
	raw, err := c.Execute("query-block", nil)
	if err != nil {
		return nil, err
	}

	var blocks []QMPBlockInfo
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return nil, fmt.Errorf("failed to parse block device response: %w", err)
	}
	return blocks, nil
}


// SystemPowerdown triggers an ACPI powerdown event in the guest.
func (c *QMPClient) SystemPowerdown() error {
	_, err := c.Execute("system_powerdown", nil)
	return err
}

// Quit requests QEMU to terminate cleanly.
func (c *QMPClient) Quit() error {
	_, err := c.Execute("quit", nil)
	return err
}

// Stop pauses guest execution.
func (c *QMPClient) Stop() error {
	_, err := c.Execute("stop", nil)
	return err
}

// Cont resumes guest execution from paused state.
func (c *QMPClient) Cont() error {
	_, err := c.Execute("cont", nil)
	return err
}

// SystemReset hard-resets the virtual machine.
func (c *QMPClient) SystemReset() error {
	_, err := c.Execute("system_reset", nil)
	return err
}

// Greeting returns the QEMU greeting details received during handshake.
func (c *QMPClient) Greeting() *QMPGreeting {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.greeting
}

// Events returns a channel that receives asynchronous events.
func (c *QMPClient) Events() <-chan QMPEvent {
	return c.events
}

// Close disconnects from the QMP socket.
func (c *QMPClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed || c.conn == nil {
		return nil
	}

	c.closed = true
	err := c.conn.Close()
	c.conn = nil
	return err
}

// --- High-level One-shot Helper Functions ---

// QMPExecute executes a single QMP command on a socket, then cleanly closes the connection.
func QMPExecute(socketPath string, command string, arguments any, timeout time.Duration) (json.RawMessage, error) {
	client := NewQMPClient(socketPath)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		return nil, err
	}
	defer client.Close()

	return client.Execute(command, arguments)
}

// QMPSystemPowerdown sends an ACPI powerdown signal via QMP on the specified socket.
func QMPSystemPowerdown(socketPath string, timeout time.Duration) error {
	client := NewQMPClient(socketPath)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		return err
	}
	defer client.Close()

	return client.SystemPowerdown()
}

// QMPQueryStatus queries the VM execution status via QMP on the specified socket.
func QMPQueryStatus(socketPath string, timeout time.Duration) (*QMPStatusResult, error) {
	client := NewQMPClient(socketPath)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		return nil, err
	}
	defer client.Close()

	return client.QueryStatus()
}

// QMPQuit instructs QEMU to immediately quit via QMP.
func QMPQuit(socketPath string, timeout time.Duration) error {
	client := NewQMPClient(socketPath)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		return err
	}
	defer client.Close()

	return client.Quit()
}

// QMPQueryCPUs executes query-cpus on the given socket.
func QMPQueryCPUs(socketPath string, timeout time.Duration) ([]QMPCPUInfo, error) {
	client := NewQMPClient(socketPath)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		return nil, err
	}
	defer client.Close()

	return client.QueryCPUs()
}

// QMPQueryMemory executes query-memory-size-summary on the given socket.
func QMPQueryMemory(socketPath string, timeout time.Duration) (*QMPMemorySummary, error) {
	client := NewQMPClient(socketPath)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		return nil, err
	}
	defer client.Close()

	return client.QueryMemorySizeSummary()
}

// QMPQueryBlock executes query-block on the given socket.
func QMPQueryBlock(socketPath string, timeout time.Duration) ([]QMPBlockInfo, error) {
	client := NewQMPClient(socketPath)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		return nil, err
	}
	defer client.Close()

	return client.QueryBlock()
}

