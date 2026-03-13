package server

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"mattwalters/foobar/internal/config"
	"mattwalters/foobar/internal/process"
	"mattwalters/foobar/internal/store"
)

func TestServerE2E(t *testing.T) {
	// 1. Setup Data Store
	dbPath := filepath.Join(t.TempDir(), "foobar-e2e.duckdb")
	db, err := store.NewStore(dbPath)
	if err != nil {
		t.Fatalf("failed to init db: %v", err)
	}
	defer db.Close()

	// 2. Setup Process Manager with dummy config
	manager := process.NewManager(db)
	manager.Add("dummy", config.ProcessConfig{
		Command: "while true; do echo 'alive'; sleep 0.1; done",
	})
	
	// Start processes so we get logs
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := manager.StartAll(ctx); err != nil {
		t.Fatalf("StartAll failed: %v", err)
	}
	defer manager.StopAll()

	// 3. Setup Server
	socketPath := filepath.Join(t.TempDir(), "foobar.sock")
	srv := NewServer(socketPath, manager, db)
	if err := srv.Start(); err != nil {
		t.Fatalf("Server start failed: %v", err)
	}
	defer srv.Stop()

	// 4. Create HTTP Client configured to use the Unix Socket
	client := &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, addr string) (net.Conn, error) {
				return net.Dial("unix", socketPath)
			},
		},
		Timeout: 5 * time.Second,
	}

	// Wait for process to print "alive" a few times
	time.Sleep(300 * time.Millisecond)

	// --- A. Test GET /processes
	t.Run("GET /processes", func(t *testing.T) {
		resp, err := client.Get("http://unix/processes")
		if err != nil {
			t.Fatalf("failed GET /processes: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200 OK, got %d", resp.StatusCode)
		}

		var procs []ProcessResponse
		if err := json.NewDecoder(resp.Body).Decode(&procs); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if len(procs) != 1 || procs[0].Name != "dummy" || procs[0].Status != "running" {
			t.Errorf("unexpected processes response: %+v", procs)
		}
	})

	// --- B. Test GET /logs
	t.Run("GET /logs", func(t *testing.T) {
		resp, err := client.Get("http://unix/logs?process=dummy&limit=5")
		if err != nil {
			t.Fatalf("failed GET /logs: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			b, _ := io.ReadAll(resp.Body)
			t.Errorf("expected 200 OK, got %d. Body: %s", resp.StatusCode, string(b))
		}

		var logs []store.LogEntry
		if err := json.NewDecoder(resp.Body).Decode(&logs); err != nil {
			t.Fatalf("failed to decode logs: %v", err)
		}

		if len(logs) == 0 {
			t.Fatal("expected logs, got none")
		}

		if logs[0].Message != "alive" {
			t.Errorf("expected log message 'alive', got '%s'", logs[0].Message)
		}
	})

	// --- C. Test POST /processes/stop
	t.Run("POST /processes/stop", func(t *testing.T) {
		resp, err := client.Post("http://unix/processes/stop?process=dummy", "application/json", nil)
		if err != nil {
			t.Fatalf("failed POST /processes/stop: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200 OK, got %d", resp.StatusCode)
		}

		// Verify via Manager
		time.Sleep(50 * time.Millisecond) // buffer for OS kill
		p, _ := manager.GetProcess("dummy")
		if p.GetStatus() != "stopped" {
			t.Errorf("expected process status stopped, got %s", p.GetStatus())
		}
	})
	
	// --- D. Test POST /processes/start
	t.Run("POST /processes/start", func(t *testing.T) {
		resp, err := client.Post("http://unix/processes/start?process=dummy", "application/json", nil)
		if err != nil {
			t.Fatalf("failed POST /processes/start: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200 OK, got %d", resp.StatusCode)
		}

		p, _ := manager.GetProcess("dummy")
		if p.GetStatus() != "running" {
			t.Errorf("expected process status running, got %s", p.GetStatus())
		}
	})
}
