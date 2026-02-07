package runner

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/mattwalters/foobar/internal/config"
	"github.com/mattwalters/foobar/internal/store"
)

func TestRunner(t *testing.T) {
	// 1. Setup Dependencies
	s, err := store.New("") // In-memory
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	cfg := &config.Config{
		Processes: []config.Process{
			{
				Name:    "echo-test",
				Command: "echo 'hello world'",
			},
		},
	}

	m := New(cfg, s)

	// 2. Start Processes
	m.StartAll()

	// 3. Wait for process to finish and logs to flush
	// Since we don't have a "Wait" API exposed on Manager yet for specific processes,
	// we'll just sleep a bit. Real integration tests handles this better with retries.
	time.Sleep(500 * time.Millisecond)

	// 4. Verify Logs in Store
	ctx := context.Background()
	logs, err := s.Query(ctx, store.LogFilter{ProcessID: "echo-test"})
	if err != nil {
		t.Fatalf("failed to query logs: %v", err)
	}

	found := false
	for _, l := range logs {
		if strings.Contains(l.Raw, "hello world") {
			found = true
			break
		}
	}

	if !found {
		t.Errorf("did not find 'hello world' in logs. Got %d logs", len(logs))
		for _, l := range logs {
			t.Logf("Log: %s", l.Raw)
		}
	}
}
