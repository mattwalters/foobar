# Foobar AI Agent Guidelines

Welcome, fellow agent! This document serves as a high-level guide to the `foobar` project. It outlines the core architecture, the technologies used, and our testing strategy.

## 1. Project Overview

`foobar` is a command-line process manager and log viewer designed to replace `concurrently` while providing a vastly superior, AI-ready developer experience. 

It runs as a local background daemon that manages child processes (e.g., node servers, frontend bundlers, database containers) and pipelines their `stdout`/`stderr` into an embedded DuckDB database. A separate frontend Bubbletea TUI can independently connect to this daemon to view logs and control process lifecycles.

**For a full dive into the vision, roadmap, and monetization strategy, please read the `productplan.md` in the root of the repository.**

## 2. Core Technologies

- **Language**: Go (`golang`)
- **CLI Framework**: Cobra (`github.com/spf13/cobra`)
- **TUI Framework**: Bubbletea & Bubbles (`github.com/charmbracelet/bubbletea`)
- **Persistence**: DuckDB via CGO (`github.com/marcboeker/go-duckdb`)
- **IPC Mechanism**: HTTP over local Unix Domain Sockets (e.g., `/tmp/foobar.sock`)

## 3. Architecture

The codebase is split strictly between the generic orchestrator (Server) and the viewer (Client).

*   `cmd/foobar/main.go`: The main entrypoint. It uses lazy-daemon logic (pinging the unix socket and starting a detached server process if one doesn't exist) before attaching the TUI viewer.
*   `internal/config`: Parses the `foobar.config.json` specifications for processes and directories.
*   `internal/process`: The core engine responsible for spawning `exec.Cmd`, capturing pipes, streaming output, and maintaining lifecycle state.
*   `internal/store`: The DuckDB interface for persisting logs.
*   `internal/server`: The background HTTP/Unix socket daemon. It exposes REST endpoints like `/processes`, `/logs`, and POST endpoints like `/processes/start`.
*   `internal/tui`: The frontend Bubbletea application that connects to the `server` to render the UI.

## 4. Testing Strategy

Our testing strategy prioritizes execution speed and integration confidence, split into three layers:

### A. Unit Tests (Fast & Isolated)
Lightweight tests for pure business logic, such as configuration parsing (`config_test.go`) or SQL query building.
- **Goal**: Cover all parsing edge cases and schema validations.
- **Rule**: Must not touch the filesystem or spawn actual OS processes.

### B. Integration Tests (Medium & Realistic)
Tests that exercise the core engine components acting together, without needing the HTTP server or TUI running.
- **Current Example**: `manager_test.go` spins up a real shell command (`while true; do echo "hello"...`), binds it to the `ProcessManager`, waits for output, and asserts that the log successfully pipelined into an in-memory test DuckDB store instance.
- **Goal**: Ensure that when the process manager says a process is running, the OS agrees, and logs are reliably captured.

### C. End-to-End Tests (Full System)
Tests that validate the user's ultimate experience by treating the daemon as a black box.
- **Goal**: Spin up the `foobar server` binary in test mode, open an HTTP client against its Unix socket, and programmatically assert that hitting the endpoints (e.g., `/processes/start`) correctly manipulates the underlying state and returns the expected JSON. 
- *Note:* While complex TUI behavior is manually tested via `make dev`, we do maintain a basic `teatest` pattern in `internal/tui/app_test.go` (`github.com/charmbracelet/x/exp/teatest`) to verify that the core Bubbletea `Model` renders successfully without panicking.

## 5. Developer Experience (DX)

When working on this project, use the provided `Makefile`:

- `make build`: Compiles `bin/foobar`.
- `make test`: Runs `go test -v ./...`.
- `make dev`: Kills any existing daemon, cleans old databases/sockets, builds the latest binary, and launches the TUI inside the `testdata/` directory against a dummy configuration.
  - **IMPORTANT AI RULE:** Do *not* run `make dev` programmatically! The TUI output will overwhelm your terminal processing. Always ask the human user to run `make dev` in a separate terminal and provide verbal feedback instead.
