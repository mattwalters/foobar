package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/marcboeker/go-duckdb"
)

type Store struct {
	db *sql.DB
}

type LogEntry struct {
	ID        string    `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	SessionID string    `json:"session_id"`
	ProcessID string    `json:"process_id"`
	Level     string    `json:"level"`
	Content   string    `json:"content"` // JSON string
	Raw       string    `json:"raw"`
}

func New(path string) (*Store, error) {
	// If path is empty, use in-memory DB
	dsn := path
	if dsn == "" {
		dsn = "?access_mode=READ_WRITE"
	}

	db, err := sql.Open("duckdb", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to check duckdb: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to open duckdb: %w", err)
	}

	s := &Store{db: db}
	if err := s.initSchema(); err != nil {
		return nil, err
	}

	return s, nil
}

func (s *Store) initSchema() error {
	query := `
	CREATE SEQUENCE IF NOT EXISTS log_id_seq;
	CREATE TABLE IF NOT EXISTS logs (
		id UUID DEFAULT uuid(),
		timestamp TIMESTAMP,
		session_id UUID,
		process_id VARCHAR,
		level VARCHAR,
		content JSON,
		raw VARCHAR
	);
	`
	_, err := s.db.Exec(query)
	return err
}

func (s *Store) Ingest(ctx context.Context, entry LogEntry) error {
	query := `
	INSERT INTO logs (timestamp, session_id, process_id, level, content, raw)
	VALUES (?, ?, ?, ?, ?, ?)
	`
	// Handle JSON content safely
	if entry.Content == "" {
		entry.Content = "{}"
	}

	_, err := s.db.ExecContext(ctx, query,
		entry.Timestamp,
		entry.SessionID,
		entry.ProcessID,
		entry.Level,
		entry.Content,
		entry.Raw,
	)
	return err
}

type LogFilter struct {
	SessionID string
	ProcessID string
	Limit     int
	Offset    int
	Search    string
}

func (s *Store) Query(ctx context.Context, filter LogFilter) ([]LogEntry, error) {
	query := `SELECT timestamp, session_id, process_id, level, CAST(content AS VARCHAR), raw FROM logs WHERE 1=1`
	var args []interface{}

	if filter.SessionID != "" {
		query += " AND session_id = ?"
		args = append(args, filter.SessionID)
	}
	if filter.ProcessID != "" {
		query += " AND process_id = ?"
		args = append(args, filter.ProcessID)
	}
	if filter.Search != "" {
		query += " AND (raw ILIKE ? OR process_id ILIKE ?)"
		pattern := "%" + filter.Search + "%"
		args = append(args, pattern, pattern)
	}

	query += " ORDER BY timestamp DESC"

	if filter.Limit > 0 {
		query += " LIMIT ?"
		args = append(args, filter.Limit)
	}
	if filter.Offset > 0 {
		query += " OFFSET ?"
		args = append(args, filter.Offset)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []LogEntry
	for rows.Next() {
		var l LogEntry
		if err := rows.Scan(&l.Timestamp, &l.SessionID, &l.ProcessID, &l.Level, &l.Content, &l.Raw); err != nil {
			return nil, err
		}
		logs = append(logs, l)
	}
	return logs, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}
