package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"time"

	"mattwalters/foobar/internal/config"
	"mattwalters/foobar/internal/process"
	"mattwalters/foobar/internal/server"
	"mattwalters/foobar/internal/tui"

	"github.com/spf13/cobra"
)

var socketPath = "/tmp/foobar.sock"

var rootCmd = &cobra.Command{
	Use:   "foobar",
	Short: "foobar is a command-line process manager and log viewer",
	Long:  `foobar is a local development hub that manages background processes, captures structured logs, and displays them via a terminal UI.`,
	Run: func(cmd *cobra.Command, args []string) {
		// 1. Check if server is running by dialing the socket
		conn, err := net.Dial("unix", socketPath)
		if err != nil {
			// Server not running, spin it up in the background
			fmt.Println("Starting background daemon...")
			serverCmd := exec.Command(os.Args[0], "server")
			// Detach from current process group (simple backgrounding)
			if err := serverCmd.Start(); err != nil {
				fmt.Fprintf(os.Stderr, "failed to start background daemon: %v\n", err)
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

		// 2. Attach TUI
		if err := tui.Start(socketPath); err != nil {
			fmt.Fprintf(os.Stderr, "TUI error: %v\n", err)
			os.Exit(1)
		}
	},
}

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Starts the foobar background server",
	Long:  `Starts the foobar background server that manages processes and IPC without launching the UI.`,
	Run: func(cmd *cobra.Command, args []string) {
		// 1. Load configuration
		// For MVP, assume it's in the current working directory named foobar.config.json
		cfgPath := "foobar.config.json"
		cfg, err := config.Load(cfgPath)
		if err != nil {
			// If missing, we can run empty, but let's log it
			fmt.Fprintf(os.Stderr, "Config warning: %v\n", err)
			cfg = &config.FoobarConfig{Processes: make(map[string]config.ProcessConfig)}
		}

		// 2. Initialize process manager
		manager := process.NewManager()
		for name, pcfg := range cfg.Processes {
			manager.Add(name, pcfg)
		}

		// 3. Start processes
		ctx := context.Background()
		if err := manager.StartAll(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to start processes: %v\n", err)
		}
		
		// Ensure processes are cleaned up on exit
		defer manager.StopAll()

		// 4. Start IPC Server
		srv := server.NewServer(socketPath, manager)
		if err := srv.Start(); err != nil {
			fmt.Fprintf(os.Stderr, "Server start failed: %v\n", err)
			os.Exit(1)
		}
		
		// 5. Block forever (or hook up signal handlers to shutdown gracefully)
		select {} 
	},
}

func init() {
	rootCmd.AddCommand(serverCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
