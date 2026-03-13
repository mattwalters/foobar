package store

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/marcboeker/go-duckdb"
)

// Store represents a connection to the DuckDB log database
type Store struct {
	db *sql.DB
}

// LogEntry is the database representation of a log
type LogEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Process   string    `json:"process"`
	Stream    string    `json:"stream"`
	Message   string    `json:"message"`
	Context   string    `json:"context"` // JSON string
}

// NewStore initializes a new DuckDB database and ensures the schema exists
func NewStore(dbPath string) (*Store, error) {
	// Open the database using the duckdb driver
	db, err := sql.Open("duckdb", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open duckdb: %w", err)
	}

	// Create the core logs table
	schema := `
	CREATE TABLE IF NOT EXISTS logs (
		id UUID DEFAULT uuid(),
		timestamp TIMESTAMP,
		process VARCHAR,
		stream VARCHAR,
		message VARCHAR,
		context JSON
	);
	
	-- Indices are not strictly necessary for duckdb min-max zone maps on append-only time series,
	-- but if we want fast lookups by process:
	-- DuckDB 0.10+ supports indexing reasonably well, but we'll stick to raw columnar scans for MVP
	`

	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to create schema: %w", err)
	}

	return &Store{db: db}, nil
}

// Close cleanly shuts down the database connection
func (s *Store) Close() error {
	return s.db.Close()
}

// InsertLog appends a new log entry to the DuckDB store
func (s *Store) InsertLog(entry LogEntry) error {
	_, err := s.db.Exec(`
		INSERT INTO logs (timestamp, process, stream, message, context)
		VALUES (?, ?, ?, ?, ?)
	`, entry.Timestamp, entry.Process, entry.Stream, entry.Message, entry.Context)
	return err
}

// GetRecentLogs retrieves the N most recent logs for a given set of processes.
// If processes is empty, it returns logs across all managed processes.
func (s *Store) GetRecentLogs(processes []string, limit int, beforeTime, afterTime time.Time) ([]LogEntry, error) {
	if limit <= 0 {
		limit = 100
	}

	whereClause := ""
	var args []interface{}

	// Dynamically build the IN clause if specific processes are requested
	if len(processes) > 0 {
		whereClause = "WHERE process IN ("
		for i, p := range processes {
			if i > 0 {
				whereClause += ", "
			}
			whereClause += "?"
			args = append(args, p)
		}
		whereClause += ")"
	}

	if !beforeTime.IsZero() {
		if whereClause == "" {
			whereClause = "WHERE timestamp < ?"
		} else {
			whereClause += " AND timestamp < ?"
		}
		args = append(args, beforeTime)
	}

	if !afterTime.IsZero() {
		if whereClause == "" {
			whereClause = "WHERE timestamp > ?"
		} else {
			whereClause += " AND timestamp > ?"
		}
		args = append(args, afterTime)
	}

	args = append(args, limit)

	// Subquery gets the N most recent logs (ordered by newest first)
	// Outer query reverses them so the client receives them sequentially (oldest to newest)
	query := fmt.Sprintf(`
		SELECT timestamp, process, stream, message, COALESCE(context::VARCHAR, '{}') as context
		FROM (
			SELECT * FROM logs
			%s
			ORDER BY timestamp DESC
			LIMIT ?
		)
		ORDER BY timestamp ASC
	`, whereClause)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []LogEntry
	for rows.Next() {
		var l LogEntry
		if err := rows.Scan(&l.Timestamp, &l.Process, &l.Stream, &l.Message, &l.Context); err != nil {
			return nil, err
		}
		logs = append(logs, l)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return logs, nil
}
