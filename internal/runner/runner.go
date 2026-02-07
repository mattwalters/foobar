package runner

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/mattwalters/foobar/internal/config"
	"github.com/mattwalters/foobar/internal/store"
)

type Manager struct {
	Store     *store.Store
	Config    *config.Config
	processes map[string]*exec.Cmd
	mu        sync.Mutex
}

func New(cfg *config.Config, s *store.Store) *Manager {
	return &Manager{
		Store:     s,
		Config:    cfg,
		processes: make(map[string]*exec.Cmd),
	}
}

func (m *Manager) StartAll() {
	var sessionID = uuid.New().String() // Generate a session ID for this "up" run

	for _, p := range m.Config.Processes {
		go m.startProcess(p, sessionID)
	}
}

func (m *Manager) startProcess(p config.Process, sessionID string) {
	// Handle Docker Compose types
	if p.Type == "docker-compose" {
		go m.startDockerCompose(p, sessionID)
		return
	}

	// Use shell to execute so we handle pipes/quotes correctly
	cmd := exec.Command("/bin/sh", "-c", p.Command)
	if p.Cwd != "" {
		cmd.Dir = p.Cwd
	}

	cmd.Env = os.Environ()
	for k, v := range p.Env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()

	if err := cmd.Start(); err != nil {
		log.Printf("Failed to start %s: %v", p.Name, err)
		return
	}

	m.mu.Lock()
	m.processes[p.Name] = cmd
	m.mu.Unlock()

	// Stream logs
	go m.streamLog(stdout, p.Name, sessionID, "INFO")
	go m.streamLog(stderr, p.Name, sessionID, "ERROR")

	cmd.Wait()
}

func (m *Manager) startDockerCompose(p config.Process, sessionID string) {
	// 1. Ensure it's up
	// We use "docker compose up -d" to ensure it's running in background
	// If it's already running, this is effectively a no-op or update
	cmdArgs := []string{"compose"}
	if p.ConfigFile != "" {
		cmdArgs = append(cmdArgs, "-f", p.ConfigFile)
	}
	cmdArgs = append(cmdArgs, "up", "-d")

	upCmd := exec.Command("docker", cmdArgs...)
	if p.Cwd != "" {
		upCmd.Dir = p.Cwd
	}

	if output, err := upCmd.CombinedOutput(); err != nil {
		log.Printf("Failed to start docker compose for %s: %v\nOutput: %s", p.Name, err, string(output))
		// Log this error to the store too?
		return
	}

	// 2. Stream logs
	// "docker compose logs -f"
	logArgs := []string{"compose"}
	if p.ConfigFile != "" {
		logArgs = append(logArgs, "-f", p.ConfigFile)
	}
	logArgs = append(logArgs, "logs", "-f", "--no-log-prefix") // We might want prefix to identify service?
	// Actually, docker compose logs mixes all services.
	// Ideally we want to identify which service line belongs to.
	// "--no-log-prefix" removes the "service-1 | " part.
	// If we keep it, we can parse it.
	// Let's keep the prefix (default) and parse it? Or just treat "db" as the process_id for all?
	// For V1, let's just treat the group as the process name for simplicity,
	// or maybe we stream logs for the whole group and let the raw log contain the prefix.

	logCmd := exec.Command("docker", logArgs...)
	if p.Cwd != "" {
		logCmd.Dir = p.Cwd
	}

	stdout, _ := logCmd.StdoutPipe()
	stderr, _ := logCmd.StderrPipe()

	if err := logCmd.Start(); err != nil {
		log.Printf("Failed to start docker logs for %s: %v", p.Name, err)
		return
	}

	m.mu.Lock()
	m.processes[p.Name] = logCmd // We track the log streamer key
	m.mu.Unlock()

	go m.streamLog(stdout, p.Name, sessionID, "INFO")
	go m.streamLog(stderr, p.Name, sessionID, "ERROR")

	logCmd.Wait()
}

func (m *Manager) streamLog(r io.Reader, processID, sessionID, level string) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		text := scanner.Text()
		entry := store.LogEntry{
			Timestamp: time.Now(),
			SessionID: sessionID,
			ProcessID: processID,
			Level:     level,
			Raw:       text,
			Content:   "{}", // TODO: Parse JSON if applicable
		}
		// Fire and forget ingestion? Or buffer?
		// For now, sync insert
		m.Store.Ingest(context.Background(), entry)
	}
}
