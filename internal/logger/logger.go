package logger

import (
	"context"
	"io"
	"log/slog"
	"os"

	"mattwalters/foobar/internal/store"
)

// SystemProcessName is the reserved name for foobar's internal logs
const SystemProcessName = "foobar_system"

// logHandler implements slog.Handler to multiplex logs to both a file and DuckDB
type logHandler struct {
	fileHandler slog.Handler
	db          *store.Store
	level       slog.Level
}

func (h *logHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return level >= h.level
}

func (h *logHandler) Handle(ctx context.Context, r slog.Record) error {
	// 1. Always write to the file handler
	if err := h.fileHandler.Handle(ctx, r); err != nil {
		return err
	}

	// 2. Write to DuckDB, but strictly filter out highly verbose Debug logs
	if h.db != nil && r.Level >= slog.LevelInfo {
		entry := store.LogEntry{
			Timestamp: r.Time,
			Process:   SystemProcessName,
			Stream:    r.Level.String(),
			Message:   r.Message,
			Context:   "{}",
		}
		
		// Fire and forget; if the DB fails to log a DB error we can't do much
		go h.db.InsertLog(entry)
	}

	return nil
}

func (h *logHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &logHandler{
		fileHandler: h.fileHandler.WithAttrs(attrs),
		db:          h.db,
		level:       h.level,
	}
}

func (h *logHandler) WithGroup(name string) slog.Handler {
	return &logHandler{
		fileHandler: h.fileHandler.WithGroup(name),
		db:          h.db,
		level:       h.level,
	}
}

// Init configures the global slog instance
func Init(logFilePath string, db *store.Store, level slog.Level) error {
	var fileWriter io.Writer
	
	if logFilePath != "" {
		f, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
		if err != nil {
			return err
		}
		fileWriter = f
	} else {
		// Fallback to stderr if no file specified
		fileWriter = os.Stderr
	}

	// Create JSON file handler
	fileOptions := &slog.HandlerOptions{
		Level: level,
	}
	fileHandler := slog.NewJSONHandler(fileWriter, fileOptions)

	// Wrap with our multiplexer
	h := &logHandler{
		fileHandler: fileHandler,
		db:          db,
		level:       level,
	}

	logger := slog.New(h)
	slog.SetDefault(logger)
	
	return nil
}
