package api

import (
	"encoding/json"
	"net/http"

	"github.com/mattwalters/foobar/internal/store"
)

type Server struct {
	Store *store.Store
}

func New(s *store.Store) *Server {
	return &Server{Store: s}
}

func (s *Server) Router() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/ingest", s.handleIngest)
	mux.HandleFunc("GET /api/v1/logs", s.handleQueryLogs)
	mux.HandleFunc("GET /health", s.handleHealth)
	return mux
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

func (s *Server) handleIngest(w http.ResponseWriter, r *http.Request) {
	var entry store.LogEntry
	if err := json.NewDecoder(r.Body).Decode(&entry); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if err := s.Store.Ingest(r.Context(), entry); err != nil {
		http.Error(w, "Failed to ingest log", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusAccepted)
}

func (s *Server) handleQueryLogs(w http.ResponseWriter, r *http.Request) {
	// Parse basic query params
	// TODO: Add full filtering logic mapping
	filter := store.LogFilter{
		Limit: 100,
	}

	logs, err := s.Store.Query(r.Context(), filter)
	if err != nil {
		http.Error(w, "Failed to query logs", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(logs)
}
