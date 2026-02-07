package store

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestStore(t *testing.T) {
	// 1. Initialize in-memory store
	s, err := New("")
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	// 2. Test Ingest
	ctx := context.Background()
	logEntry := LogEntry{
		ID:        uuid.New().String(),
		Timestamp: time.Now(),
		SessionID: uuid.New().String(),
		ProcessID: "test-process",
		Level:     "INFO",
		Raw:       "This is a test log message",
	}

	if err := s.Ingest(ctx, logEntry); err != nil {
		t.Fatalf("Ingest failed: %v", err)
	}

	// 3. Test Query
	// Allow a small delay for async ingestion if configured (duckdb is usually fast enough for sync ingest in tests)

	// Query all logs
	filter := LogFilter{
		Limit: 10,
	}
	logs, err := s.Query(ctx, filter)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	if len(logs) != 1 {
		t.Errorf("expected 1 log, got %d", len(logs))
	} else {
		got := logs[0]
		if got.ProcessID != logEntry.ProcessID {
			t.Errorf("expected ProcessID %s, got %s", logEntry.ProcessID, got.ProcessID)
		}
		if got.Raw != logEntry.Raw {
			t.Errorf("expected Raw %s, got %s", logEntry.Raw, got.Raw)
		}
	}

	// 4. Test Filter by ProcessID
	filter.ProcessID = "non-existent"
	logs, err = s.Query(ctx, filter)
	if err != nil {
		t.Fatalf("Query with filter failed: %v", err)
	}
	if len(logs) != 0 {
		t.Errorf("expected 0 logs for non-existent process, got %d", len(logs))
	}

	filter.ProcessID = "test-process"
	logs, err = s.Query(ctx, filter)
	if err != nil {
		t.Fatalf("Query with valid filter failed: %v", err)
	}
	if len(logs) != 1 {
		t.Errorf("expected 1 log for valid process, got %d", len(logs))
	}
}
