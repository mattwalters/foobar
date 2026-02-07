package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattwalters/foobar/internal/config"
	"github.com/mattwalters/foobar/internal/daemon"
	"github.com/mattwalters/foobar/internal/mcp"
	"github.com/mattwalters/foobar/internal/tui"
)

func main() {
	daemonMode := flag.Bool("daemon", false, "Run in daemon mode")
	configPath := flag.String("config", "foobar.config.yaml", "Path to config file")
	flag.Parse()

	// Locate socket path (user scoped)
	home, _ := os.UserHomeDir()
	socketPath := filepath.Join(home, ".foobar.sock")

	if *daemonMode {
		runDaemon(*configPath, socketPath)
		return
	}

	mcpMode := flag.Bool("mcp", false, "Run in MCP server mode")
	if *mcpMode {
		runMCP(socketPath)
		return
	}

	// Client Mode
	// 1. Check if daemon is running (try to connect)
	if !isDaemonRunning(socketPath) {
		fmt.Println("Daemon not running. Starting...")
		if err := startDaemon(); err != nil {
			log.Fatalf("Failed to start daemon: %v", err)
		}
		// Wait for socket
		waitForSocket(socketPath)
	}

	fmt.Println("Connected to Foobar Daemon! Launching TUI...")
	p := tea.NewProgram(tui.New(socketPath), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Alas, there's been an error: %v", err)
		os.Exit(1)
	}
}

func runDaemon(configPath, socketPath string) {
	cfg, err := config.Load(configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	d, err := daemon.New(cfg, socketPath)
	if err != nil {
		log.Fatalf("Failed to init daemon: %v", err)
	}

	if err := d.Run(); err != nil {
		log.Fatalf("Daemon error: %v", err)
	}
}

func isDaemonRunning(socketPath string) bool {
	if _, err := os.Stat(socketPath); os.IsNotExist(err) {
		return false
	}
	// Try to dial
	// ... (TODO: ping health endpoint)
	return true
}

func startDaemon() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command(exe, "--daemon")
	cmd.Stdout = os.Stdout // For now, let's see output
	cmd.Stderr = os.Stderr
	// Detach? For now just run in background
	if err := cmd.Start(); err != nil {
		return err
	}
	// TODO: Detach properly if needed, but for "foobar up" staying attached might be okay initially?
	// Actually implementation plan says "Smart Start" -> starts background daemon.
	// exec.Command doesn't fully detach unless we do setsid, etc.
	// For V1 MVP, let's just let it run.
	fmt.Printf("Started daemon with PID %d\n", cmd.Process.Pid)
	return nil
}

func waitForSocket(path string) {
	for i := 0; i < 10; i++ {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func runMCP(socketPath string) {
	server := mcp.New(socketPath)
	if err := server.Serve(); err != nil {
		log.Fatalf("MCP Server execution error: %v", err)
	}
}
