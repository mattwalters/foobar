package process

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"time"

	"mattwalters/foobar/internal/config"
)

// Process represents a managed command
type Process struct {
	Name      string
	Config    config.ProcessConfig
	Cmd       *exec.Cmd
	LogBuffer *LogBuffer
	Status    string // e.g. "stopped", "running", "failed"
	mu        sync.RWMutex
}

// Manager handles the lifecycle of multiple processes
type Manager struct {
	processes map[string]*Process
	mu        sync.RWMutex
}

// NewManager creates a new Process Manager
func NewManager() *Manager {
	return &Manager{
		processes: make(map[string]*Process),
	}
}

// Add adds a new process based on the config but does not start it.
func (m *Manager) Add(name string, cfg config.ProcessConfig) *Process {
	m.mu.Lock()
	defer m.mu.Unlock()

	p := &Process{
		Name:      name,
		Config:    cfg,
		LogBuffer: NewLogBuffer(1000), // Max 1000 logs in memory for MVP
		Status:    "stopped",
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
		p.Stop()
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
	go func() {
		err := p.Cmd.Wait()
		p.mu.Lock()
		defer p.mu.Unlock()
		
		// If explicitly stopped by Stop(), preserve that status
		if p.Status != "stopped" {
			if err != nil {
				p.Status = "failed"
			} else {
				p.Status = "stopped"
			}
		}
	}()

	return nil
}

// Stop terminates the process
func (p *Process) Stop() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.Status != "running" || p.Cmd == nil || p.Cmd.Process == nil {
		return nil
	}

	if err := p.Cmd.Process.Kill(); err != nil {
		return err
	}
	p.Status = "stopped"
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
		
		entry := LogEntry{
			Timestamp: time.Now(),
			Stream:    streamName,
			Message:   text,
		}
		
		p.LogBuffer.Append(entry)
	}
}
