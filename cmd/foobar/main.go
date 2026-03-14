package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"time" // Keep time because time.Sleep is used

	"mattwalters/foobar/internal/config"
	"mattwalters/foobar/internal/logger"
	"mattwalters/foobar/internal/process"
	"mattwalters/foobar/internal/server"
	"mattwalters/foobar/internal/store"
	"mattwalters/foobar/internal/tui"

	"github.com/spf13/cobra"
)

var socketPath = "/tmp/foobar.sock"

var rootCmd = &cobra.Command{
	Use:   "foobar",
	Short: "foobar is a command-line process manager and log viewer",
	Long:  `foobar is a local development hub that manages background processes, captures structured logs, and displays them via a terminal UI.`,
	Run: func(cmd *cobra.Command, args []string) {
		// Initialize System Logger for the launcher (without DB hook)
		if err := logger.Init("foobar-system.log", nil, slog.LevelInfo); err != nil {
			fmt.Fprintf(os.Stderr, "failed to init logger: %v\n", err)
			os.Exit(1)
		}

		// 3. Check if server is running by dialing the socket
		conn, err := net.Dial("unix", socketPath)
		if err != nil {
			// Server not running, spin it up in the background
			slog.Info("Starting background daemon...")
			serverCmd := exec.Command(os.Args[0], "server")
			// Pass stderr so we can see why it crashes on startup before slog is active
			serverCmd.Stderr = os.Stderr
			// Detach from current process group (simple backgrounding)
			if err := serverCmd.Start(); err != nil {
				slog.Error("failed to start background daemon", "error", err)
				os.Exit(1)
			}

			// Wait for socket to become available
			for i := 0; i < 10; i++ {
				conn, err := net.Dial("unix", socketPath)
				if err == nil {
					conn.Close()
					break
				}
				time.Sleep(500 * time.Millisecond)
			}
		} else {
			conn.Close()
		}

		// 4. Attach TUI
		if err := tui.Start(socketPath); err != nil {
			slog.Error("TUI error", "error", err)
			os.Exit(1)
		}
	},
}

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Starts the foobar background server",
	Long:  `Starts the foobar background server that manages processes and IPC without launching the UI.`,
	Run: func(cmd *cobra.Command, args []string) {
		// 1. Initialize DB first so the logger can wire into it
		dbPath := os.Getenv("FOOBAR_DB_PATH")
		if dbPath == "" {
			dbPath = filepath.Join(os.TempDir(), "foobar.duckdb")
		}
		db, err := store.NewStore(dbPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to init db: %v\n", err)
			os.Exit(1)
		}
		defer db.Close()

		// 2. Initialize System Logger
		if err := logger.Init("foobar-system.log", db, slog.LevelInfo); err != nil {
			fmt.Fprintf(os.Stderr, "failed to init logger: %v\n", err)
			os.Exit(1)
		}
		slog.Info("Starting foobar background daemon...")

		// 3. Load config
		cfg, err := config.Load("foobar.config.json")
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				slog.Warn("foobar.config.json not found, starting with empty configuration")
				cfg = &config.FoobarConfig{Processes: make(map[string]config.ProcessConfig)}
			} else {
				fmt.Fprintf(os.Stderr, "Configuration error: %v\n", err)
				os.Exit(1)
			}
		}

		// 3b. Load Concurrently scripts if configured
		if cfg.Concurrently != nil {
			procs, err := config.ExtractConcurrentlyProcesses(cfg.Concurrently.File, cfg.Concurrently.Script)
			if err != nil {
				slog.Error("failed to extract concurrently processes", "error", err)
			} else {
				for name, pcfg := range procs {
					cfg.Processes[name] = pcfg
				}
			}
		}

		// 3c. Load Docker Compose if configured
		if cfg.DockerCompose != "" {
			slog.Info("booting docker compose stack", "file", cfg.DockerCompose)
			procs, err := config.SetupDockerCompose(cfg.DockerCompose)
			if err != nil {
				slog.Error("failed to setup docker compose", "error", err)
			} else {
				for name, pcfg := range procs {
					cfg.Processes[name] = pcfg
				}
			}
		}

		// 4. Setup process manager
		manager := process.NewManager(db)
		for name, pcfg := range cfg.Processes {
			manager.Add(name, pcfg)
		}
		_ = manager.StartAll(context.Background())
		defer manager.StopAll()

		// 5. Setup RPC Server
		srv := server.NewServer(socketPath, manager, db)
		if err := srv.Start(); err != nil {
			slog.Error("server failed to start", "error", err)
			os.Exit(1)
		}
		defer func() { _ = srv.Stop() }() // Ensure server is stopped gracefully

		// 6. Block forever (or hook up signal handlers to shutdown gracefully)
		select {}
	},
}

var debugCmd = &cobra.Command{
	Use:   "debug",
	Short: "Tails the foobar-system.log file",
	Long:  `Runs tail -f foobar-system.log to view the background daemon logs in real-time.`,
	Run: func(cmd *cobra.Command, args []string) {
		tailCmd := exec.Command("tail", "-f", "foobar-system.log")
		tailCmd.Stdout = os.Stdout
		tailCmd.Stderr = os.Stderr

		fmt.Println("Tailing foobar-system.log (Ctrl+C to exit)...")
		if err := tailCmd.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "failed to run tail: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(serverCmd)
	rootCmd.AddCommand(debugCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
