package store

import (
	"path/filepath"
	"testing"
	"time"
)

func TestStore_InsertAndGet(t *testing.T) {
	// Use an in-memory or temp file duckdb for tests
	dbPath := filepath.Join(t.TempDir(), "test.duckdb")
	db, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer db.Close()

	now := time.Now()

	entries := []LogEntry{
		{Timestamp: now.Add(-3 * time.Second), Process: "web", Stream: "stdout", Message: "log 1", Context: "{}"},
		{Timestamp: now.Add(-2 * time.Second), Process: "web", Stream: "stderr", Message: "log 2", Context: "{}"},
		{Timestamp: now.Add(-1 * time.Second), Process: "api", Stream: "stdout", Message: "log 3", Context: "{}"},
		{Timestamp: now, Process: "web", Stream: "stdout", Message: "log 4", Context: "{}"},
	}

	for _, e := range entries {
		if err := db.InsertLog(e); err != nil {
			t.Fatalf("failed to insert log: %v", err)
		}
	}

	// Retrieve logs for 'web' process
	logs, err := db.GetRecentLogs([]string{"web"}, 10, time.Time{}, time.Time{})
	if err != nil {
		t.Fatalf("failed to get recent logs: %v", err)
	}

	if len(logs) != 3 {
		t.Fatalf("expected 3 logs for 'web' process, got %d", len(logs))
	}

	// Verify order is chronological (oldest to newest)
	if logs[0].Message != "log 1" || logs[1].Message != "log 2" || logs[2].Message != "log 4" {
		t.Errorf("logs are not in expected chronological order: %v", logs)
	}

	// Limit test
	logsLimited, err := db.GetRecentLogs([]string{"web"}, 2, time.Time{}, time.Time{})
	if err != nil {
		t.Fatalf("failed to get recent logs with limit: %v", err)
	}

	// Should get the 2 MOST RECENT logs, ordered chronologically
	if len(logsLimited) != 2 {
		t.Fatalf("expected 2 logs for 'web' process, got %d", len(logsLimited))
	}
	if logsLimited[0].Message != "log 2" || logsLimited[1].Message != "log 4" {
		t.Errorf("limited logs are not the most recent or in chronological order: %v", logsLimited)
	}
}
