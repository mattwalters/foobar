package process

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"mattwalters/foobar/internal/config"
	"mattwalters/foobar/internal/store"
)

// Process represents a managed command
type Process struct {
	Name     string
	Config   config.ProcessConfig
	Cmd      *exec.Cmd
	DB       *store.Store
	Status   string        // e.g. "stopped", "running", "failed"
	waitDone chan struct{} // Closed when the process finishes waiting
	mu       sync.RWMutex
}

// Manager handles the lifecycle of multiple processes
type Manager struct {
	processes map[string]*Process
	db        *store.Store
	mu        sync.RWMutex
}

// NewManager creates a new Process Manager
func NewManager(db *store.Store) *Manager {
	return &Manager{
		processes: make(map[string]*Process),
		db:        db,
	}
}

// Add adds a new process based on the config but does not start it.
func (m *Manager) Add(name string, cfg config.ProcessConfig) *Process {
	m.mu.Lock()
	defer m.mu.Unlock()

	p := &Process{
		Name:   name,
		Config: cfg,
		DB:     m.db,
		Status: "stopped",
	}
	m.processes[name] = p
	return p
}

// StartAll starts all managed processes that are currently stopped.
func (m *Manager) StartAll(ctx context.Context) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var errs []error
	for _, p := range m.processes {
		if p.Status == "stopped" || p.Status == "failed" {
			if err := p.Start(ctx); err != nil {
				errs = append(errs, fmt.Errorf("failed to start %s: %w", p.Name, err))
			}
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("some processes failed to start: %v", errs)
	}
	return nil
}

// StopAll sends a kill signal to all running processes
func (m *Manager) StopAll() {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, p := range m.processes {
		_ = p.Stop()
	}
}

// GetProcess returns a process by name
func (m *Manager) GetProcess(name string) (*Process, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	p, ok := m.processes[name]
	return p, ok
}

// GetAllProcesses returns a list of all managed processes
func (m *Manager) GetAllProcesses() []*Process {
	m.mu.RLock()
	defer m.mu.RUnlock()

	list := make([]*Process, 0, len(m.processes))
	for _, p := range m.processes {
		list = append(list, p)
	}
	return list
}

// Start begins the process execution and hooks up log streaming
func (p *Process) Start(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.Status == "running" {
		return nil
	}

	p.Cmd = exec.CommandContext(ctx, "sh", "-c", p.Config.Command)
	p.Cmd.Dir = p.Config.Dir

	// Setup pipes
	stdout, err := p.Cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := p.Cmd.StderrPipe()
	if err != nil {
		return err
	}

	// Start command
	if err := p.Cmd.Start(); err != nil {
		p.Status = "failed"
		return err
	}

	p.Status = "running"

	// Stream logs in background
	go p.streamLogs(stdout, "stdout")
	go p.streamLogs(stderr, "stderr")

	// Monitor completion
	currentCmd := p.Cmd
	// Create a channel that will be closed when Wait() returns
	done := make(chan struct{})

	go func() {
		defer close(done)
		err := currentCmd.Wait()
		p.mu.Lock()
		defer p.mu.Unlock()

		// Only update status if this goroutine belongs to the current active command
		if p.Cmd == currentCmd {
			if p.Status == "stopping" {
				p.Status = "stopped"
			} else if p.Status != "stopped" {
				if err != nil {
					p.Status = "failed"
				} else {
					p.Status = "stopped"
				}
			}
		}
	}()

	// Store the done channel on the process object so Stop() can use it
	// We need to add this field to the Process struct
	p.waitDone = done

	return nil
}

// Stop terminates the process
func (p *Process) Stop() error {
	p.mu.Lock()
	if p.Status != "running" || p.Cmd == nil || p.Cmd.Process == nil {
		p.mu.Unlock()
		return nil
	}

	p.Status = "stopping"
	cmd := p.Cmd
	done := p.waitDone
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		_ = cmd.Process.Kill() // fallback to immediate kill
		p.mu.Unlock()
		return err
	}
	p.mu.Unlock()

	// Wait for process to exit or timeout
	select {
	case <-done:
		// Process exited gracefully
	case <-time.After(5 * time.Second):
		// Force kill if it hasn't exited
		p.mu.Lock()
		if p.Status == "stopping" && p.Cmd == cmd {
			_ = cmd.Process.Kill()
		}
		p.mu.Unlock()
	}

	return nil
}

// GetStatus returns the current status of the process in a thread-safe manner
func (p *Process) GetStatus() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.Status
}

func (p *Process) streamLogs(pipe io.Reader, streamName string) {
	scanner := bufio.NewScanner(pipe)
	for scanner.Scan() {
		text := scanner.Text()

		entry := store.LogEntry{
			Timestamp: time.Now(),
			Process:   p.Name,
			Stream:    streamName,
			Message:   text,
			Context:   "{}",
		}

		if err := p.DB.InsertLog(entry); err != nil {
			// Avoid printing directly to stdout as it may corrupt TUI output
			slog.Error("failed to insert log into DuckDB", "process", p.Name, "stream", streamName, "error", err)
		}
	}
}
