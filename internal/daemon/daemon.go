package daemon

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/mattwalters/foobar/internal/api"
	"github.com/mattwalters/foobar/internal/config"
	"github.com/mattwalters/foobar/internal/runner"
	"github.com/mattwalters/foobar/internal/store"
)

type Daemon struct {
	Config     *config.Config
	Store      *store.Store
	Runner     *runner.Manager
	SocketPath string
}

func New(cfg *config.Config, socketPath string) (*Daemon, error) {
	// Initialize Store (DuckDB)
	// For now, use a file in the same dir as config or temp
	dbPath := filepath.Join(os.TempDir(), "foobar.duckdb")
	s, err := store.New(dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to init store: %w", err)
	}

	r := runner.New(cfg, s)

	return &Daemon{
		Config:     cfg,
		Store:      s,
		Runner:     r,
		SocketPath: socketPath,
	}, nil
}

func (d *Daemon) Run() error {
	// 1. Setup UDS Listener
	if err := os.RemoveAll(d.SocketPath); err != nil {
		return fmt.Errorf("failed to remove old socket: %w", err)
	}

	listener, err := net.Listen("unix", d.SocketPath)
	if err != nil {
		return fmt.Errorf("failed to listen on socket %s: %w", d.SocketPath, err)
	}
	defer listener.Close()

	// 2. Setup API Server
	srv := api.New(d.Store)
	httpServer := &http.Server{
		Handler: srv.Router(),
	}

	// 3a. Start Processes
	go d.Runner.StartAll()

	// 3. Handle Signals for graceful shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	go func() {
		fmt.Printf("Daemon listening on %s\n", d.SocketPath)
		if err := httpServer.Serve(listener); err != nil && err != http.ErrServerClosed {
			fmt.Printf("HTTP server error: %v\n", err)
		}
	}()

	<-stop
	fmt.Println("Shutting down daemon...")

	// Cleanup
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := httpServer.Shutdown(ctx); err != nil {
		fmt.Printf("HTTP shutdown error: %v\n", err)
	}

	if err := d.Store.Close(); err != nil {
		fmt.Printf("Store close error: %v\n", err)
	}

	return nil
}
