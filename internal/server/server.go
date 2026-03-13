package server

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"

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
	json.NewEncoder(w).Encode(res)
}

func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	processName := r.URL.Query().Get("process")
	if processName == "" {
		http.Error(w, "missing 'process' parameter", http.StatusBadRequest)
		return
	}

	limitStr := r.URL.Query().Get("limit")
	limit := 100 // default
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil {
			limit = l
		}
	}

	if processName != logger.SystemProcessName {
		_, ok := s.manager.GetProcess(processName)
		if !ok {
			http.Error(w, "process not found", http.StatusNotFound)
			return
		}
	}

	logs, err := s.db.GetRecentLogs(processName, limit)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to query logs: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(logs)
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
		err = p.Start(r.Context())
	case "stop":
		err = p.Stop()
	case "restart":
		p.Stop()
		err = p.Start(r.Context())
	default:
		http.Error(w, "invalid action", http.StatusBadRequest)
		return
	}

	if err != nil {
		http.Error(w, fmt.Sprintf("failed to %s process: %v", action, err), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(fmt.Sprintf("process %s %sed", processName, action)))
}
