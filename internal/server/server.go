package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"mattwalters/foobar/internal/config"
	"mattwalters/foobar/internal/logger"
	"mattwalters/foobar/internal/process"
	"mattwalters/foobar/internal/store"
)

// Server represents the local IPC server
type Server struct {
	socketPath string
	manager    *process.Manager
	db         *store.Store
	listener   net.Listener
}

// NewServer creates a new IPC Server
func NewServer(socketPath string, manager *process.Manager, db *store.Store) *Server {
	return &Server{
		socketPath: socketPath,
		manager:    manager,
		db:         db,
	}
}

// Start begins listening on the Unix domain socket
func (s *Server) Start() error {
	// Clean up stale socket if it exists
	if _, err := os.Stat(s.socketPath); err == nil {
		if err := os.Remove(s.socketPath); err != nil {
			return fmt.Errorf("failed to remove stale socket: %w", err)
		}
	}

	listener, err := net.Listen("unix", s.socketPath)
	if err != nil {
		return fmt.Errorf("failed to listen on socket %s: %w", s.socketPath, err)
	}
	s.listener = listener

	// Optional: secure the socket permissions
	if err := os.Chmod(s.socketPath, 0600); err != nil {
		// Log warning but don't strictly fail
		fmt.Fprintf(os.Stderr, "warning: failed to secure socket permission: %v\n", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/processes", s.handleProcesses)
	mux.HandleFunc("/processes/start", func(w http.ResponseWriter, r *http.Request) { s.handleProcessControl(w, r, "start") })
	mux.HandleFunc("/processes/stop", func(w http.ResponseWriter, r *http.Request) { s.handleProcessControl(w, r, "stop") })
	mux.HandleFunc("/processes/restart", func(w http.ResponseWriter, r *http.Request) { s.handleProcessControl(w, r, "restart") })
	mux.HandleFunc("/processes/add", s.handleAddProcess)
	mux.HandleFunc("/logs", s.handleLogs)

	go func() {
		// Serve will block, so we run it in a goroutine
		err := http.Serve(s.listener, mux)
		if err != nil && err != http.ErrServerClosed && !strings.Contains(err.Error(), "use of closed network connection") {
			fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		}
	}()

	return nil
}

// Stop closes the listener
func (s *Server) Stop() error {
	if s.listener != nil {
		return s.listener.Close()
	}
	return nil
}

// ProcessResponse is the JSON schema for the /processes endpoint
type ProcessResponse struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

func (s *Server) handleProcesses(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	procs := s.manager.GetAllProcesses()
	res := make([]ProcessResponse, 0, len(procs)+1)
	for _, p := range procs {
		res = append(res, ProcessResponse{
			Name:   p.Name,
			Status: p.GetStatus(),
		})
	}

	// Always append the virtual system process so the UI can fetch its logs
	res = append(res, ProcessResponse{
		Name:   logger.SystemProcessName,
		Status: "running",
	})

	sort.Slice(res, func(i, j int) bool {
		return res[i].Name < res[j].Name
	})

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}

func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	limitStr := r.URL.Query().Get("limit")
	limit := 100 // default
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	// Allow passing ?channel=web&channel=api
	// If empty, we will pass an empty slice which returns all logs
	channels := r.URL.Query()["channel"]

	// Backwards compatibility with ?process=
	if len(channels) == 0 {
		if p := r.URL.Query().Get("process"); p != "" {
			channels = append(channels, p)
		}
	}

	// For MVP, if specific channels are requested, we should ensure they exist.
	// But to keep Interleaving simple, we just pass the slice directly to DB.
	// We'll trust the DB query IN clause to just return 0 rows for bad channel names.

	// Parse time-based pagination
	var beforeTime, afterTime time.Time
	if bt := r.URL.Query().Get("before_time"); bt != "" {
		if parsed, err := time.Parse(time.RFC3339Nano, bt); err == nil {
			beforeTime = parsed
		}
	}
	if at := r.URL.Query().Get("after_time"); at != "" {
		if parsed, err := time.Parse(time.RFC3339Nano, at); err == nil {
			afterTime = parsed
		}
	}

	logs, err := s.db.GetRecentLogs(channels, limit, beforeTime, afterTime)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to query logs: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(logs)
}

func (s *Server) handleProcessControl(w http.ResponseWriter, r *http.Request, action string) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	processName := r.URL.Query().Get("process")
	if processName == "" {
		http.Error(w, "missing 'process' parameter", http.StatusBadRequest)
		return
	}

	p, ok := s.manager.GetProcess(processName)
	if !ok {
		http.Error(w, "process not found", http.StatusNotFound)
		return
	}

	var err error
	switch action {
	case "start":
		err = p.Start(context.Background())
	case "stop":
		err = p.Stop()
	case "restart":
		err = p.Stop()
		if err == nil {
			err = p.Start(context.Background())
		}
	default:
		http.Error(w, "invalid action", http.StatusBadRequest)
		return
	}

	if err != nil {
		http.Error(w, fmt.Sprintf("failed to %s process: %v", action, err), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(fmt.Sprintf("process %s %sed", processName, action)))
}

type AddProcessRequest struct {
	Name    string `json:"name"`
	Command string `json:"command"`
	Dir     string `json:"dir,omitempty"`
}

func (s *Server) handleAddProcess(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req AddProcessRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.Name == "" || req.Command == "" {
		http.Error(w, "name and command are required", http.StatusBadRequest)
		return
	}

	// Check if process already exists
	if _, exists := s.manager.GetProcess(req.Name); exists {
		http.Error(w, "process already exists", http.StatusConflict)
		return
	}

	pcfg := config.ProcessConfig{
		Command: req.Command,
		Dir:     req.Dir,
	}

	// Safely add and start the new process
	p := s.manager.Add(req.Name, pcfg)
	if err := p.Start(context.Background()); err != nil {
		http.Error(w, fmt.Sprintf("failed to start process: %v", err), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(fmt.Sprintf("process %s added and started", req.Name)))
}
