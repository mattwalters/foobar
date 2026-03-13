package process

import (
	"sync"
	"time"
)

// LogEntry represents a single parsed log line
type LogEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Stream    string    `json:"stream"`  // "stdout" or "stderr"
	Message   string    `json:"message"` // Default to raw string, MVP won't do full structured parsing yet
}

// LogBuffer is a thread-safe ring buffer for process logs
type LogBuffer struct {
	mu      sync.RWMutex
	entries []LogEntry
	maxSize int
	head    int // current write position
	count   int // total items in buffer
}

// NewLogBuffer creates a new LogBuffer with the given maximum capacity
func NewLogBuffer(maxSize int) *LogBuffer {
	if maxSize <= 0 {
		maxSize = 1000 // default MVP size
	}
	return &LogBuffer{
		entries: make([]LogEntry, maxSize),
		maxSize: maxSize,
	}
}

// Append adds a new log entry to the buffer
func (b *LogBuffer) Append(entry LogEntry) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.entries[b.head] = entry
	b.head = (b.head + 1) % b.maxSize

	if b.count < b.maxSize {
		b.count++
	}
}

// GetRecent returns the most recent logs, up to 'limit'. 
// If limit is 0, it returns all available logs.
func (b *LogBuffer) GetRecent(limit int) []LogEntry {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.count == 0 {
		return []LogEntry{}
	}

	if limit <= 0 || limit > b.count {
		limit = b.count
	}

	res := make([]LogEntry, limit)

	// Since it's a ring buffer, the oldest entry conceptually is at index (head - count) mod maxSize.
	// But we're getting the *most recent* 'limit' items.
	// The most recent item is at (head - 1).
	
	// Start reading from: (head - limit) mod maxSize
	startIdx := (b.head - limit + b.maxSize) % b.maxSize

	for i := 0; i < limit; i++ {
		idx := (startIdx + i) % b.maxSize
		res[i] = b.entries[idx]
	}

	return res
}
