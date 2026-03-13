package process

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"mattwalters/foobar/internal/config"
	"mattwalters/foobar/internal/store"
)

func TestManagerStartAndStop(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "foobar-test.duckdb")
	db, err := store.NewStore(dbPath)
	if err != nil {
		t.Fatalf("failed to init db: %v", err)
	}
	defer db.Close()

	m := NewManager(db)

	m.Add("test_proc", config.ProcessConfig{
		// A command that just prints "hello" every 100ms
		Command: `while true; do echo "hello"; sleep 0.1; done`,
	})

	ctx := context.Background()
	if err := m.StartAll(ctx); err != nil {
		t.Fatalf("StartAll failed: %v", err)
	}

	p, ok := m.GetProcess("test_proc")
	if !ok {
		t.Fatal("process not found")
	}

	if p.GetStatus() != "running" {
		t.Fatalf("expected status running, got %s", p.GetStatus())
	}

	// Give it time to generate some logs
	time.Sleep(300 * time.Millisecond)

	logs, err := db.GetRecentLogs("test_proc", 10)
	if err != nil {
		t.Fatalf("failed to get logs: %v", err)
	}
	if len(logs) == 0 {
		t.Fatal("expected logs, got none")
	}

	hasHello := false
	for _, l := range logs {
		if l.Message == "hello" && l.Stream == "stdout" {
			hasHello = true
			break
		}
	}

	if !hasHello {
		t.Error("expected to find 'hello' in logs")
	}

	// Test Stop
	m.StopAll()

	// Give it a tiny bit of time to transition
	time.Sleep(100 * time.Millisecond)

	if p.GetStatus() != "stopped" {
		t.Fatalf("expected status stopped, got %s", p.GetStatus())
	}
}
