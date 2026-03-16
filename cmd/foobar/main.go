package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"time" // Keep time because time.Sleep is used

	"mattwalters/foobar/internal/config"
	"mattwalters/foobar/internal/logger"
	"mattwalters/foobar/internal/mcp"
	"mattwalters/foobar/internal/process"
	"mattwalters/foobar/internal/server"
	"mattwalters/foobar/internal/store"
	"mattwalters/foobar/internal/tui"

	"github.com/spf13/cobra"
)

var socketPath = "/tmp/foobar.sock"

func ensureServerRunning() {
	// Check if server is running by dialing the socket
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		// Server not running, spin it up in the background
		slog.Info("Starting background daemon...")
		serverCmdExec := exec.Command(os.Args[0], "server")
		// Pass stderr so we can see why it crashes on startup before slog is active
		serverCmdExec.Stderr = os.Stderr
		// Detach from current process group (simple backgrounding)
		if err := serverCmdExec.Start(); err != nil {
			slog.Error("failed to start background daemon", "error", err)
			os.Exit(1)
		}

		// Wait for socket to become available
		for i := 0; i < 10; i++ {
			c, err := net.Dial("unix", socketPath)
			if err == nil {
				c.Close()
				return
			}
			time.Sleep(500 * time.Millisecond)
		}
		fmt.Fprintln(os.Stderr, "timeout waiting for background daemon")
		os.Exit(1)
	} else {
		conn.Close()
	}
}

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

		if len(args) > 0 {
			// Implicit run command
			runCmd.Run(cmd, args)
			return
		}

		ensureServerRunning()

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

var runName string
var runFg bool

var runCmd = &cobra.Command{
	Use:   "run [command...]",
	Short: "Run an ad-hoc command",
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		ensureServerRunning()

		commandStr := strings.Join(args, " ")
		name := runName
		originalName := ""
		if name == "" {
			name = filepath.Base(args[0])
			originalName = name
		}

		client := &http.Client{
			Transport: &http.Transport{
				DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
					return net.Dial("unix", socketPath)
				},
			},
		}

		cwd, _ := os.Getwd()

		for i := 1; i <= 100; i++ {
			reqBody, _ := json.Marshal(server.AddProcessRequest{
				Name:    name,
				Command: commandStr,
				Dir:     cwd,
			})

			resp, err := client.Post("http://unix/processes/add", "application/json", bytes.NewReader(reqBody))
			if err != nil {
				fmt.Fprintf(os.Stderr, "failed to add process: %v\n", err)
				os.Exit(1)
			}

			if resp.StatusCode == http.StatusOK {
				resp.Body.Close()
				break
			}

			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()

			if resp.StatusCode == http.StatusConflict && originalName != "" {
				name = fmt.Sprintf("%s-%d", originalName, i)
				continue
			}

			fmt.Fprintf(os.Stderr, "server rejected process: %s\n", string(body))
			os.Exit(1)
		}

		if runFg {
			fmt.Printf("Started '%s' in foreground mode.\n(Logs are captured by foobar, but not streamed here yet)\nPress Ctrl+C to stop it.\n", name)
			c := make(chan os.Signal, 1)
			signal.Notify(c, os.Interrupt)
			<-c
			fmt.Printf("\nStopping %s...\n", name)
			_, _ = client.Post(fmt.Sprintf("http://unix/processes/stop?process=%s", name), "application/json", nil)
		} else {
			// Background mode: launch TUI
			if err := tui.Start(socketPath); err != nil {
				slog.Error("TUI error", "error", err)
				os.Exit(1)
			}
		}
	},
}

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Start the foobar MCP server",
	Long:  `Starts a Model Context Protocol (MCP) server over Standard I/O for AI clients like Claude and Cursor.`,
	Run: func(cmd *cobra.Command, args []string) {
		ensureServerRunning()
		handler := mcp.NewHandler(socketPath)
		if err := handler.Start(); err != nil {
			fmt.Fprintf(os.Stderr, "MCP server error: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	runCmd.Flags().StringVarP(&runName, "name", "n", "", "Assign a name to the process")
	runCmd.Flags().BoolVar(&runFg, "foreground", false, "Run in foreground (block terminal)")

	rootCmd.AddCommand(serverCmd)
	rootCmd.AddCommand(debugCmd)
	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(mcpCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
