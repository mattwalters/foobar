# Product Plan: foobar

## Overview
`foobar` is a command-line process manager and log viewer tailored for development in the age of AI. It serves as a centralized hub for managing local development environments, collecting structured logs, and exposing them for both human developers and AI agents.

## Core Features

### 1. Process Management & Ownership
`foobar` acts as a full-blown process manager. It spawns, owns, and wraps development processes to hook directly into their standard output and error streams.
*   **Startup Configurations**:
    *   **Native Config**: A `foobar.config.json` file for specifying arbitrary commands and processes.
    *   **Concurrently**: Support for importing processes from a `concurrently` configuration.
    *   **Docker**: Support for analyzing a `Dockerfile` or `docker-compose` to run processes.
*   **Interaction Model**:
    *   Running `foobar` lazily spins up the background server (if not already running) and attaches the TUI to the existing session.
    *   Running `foobar ps` lists the currently active managed processes (similar to `docker ps`).

### 2. Architecture: Single Go Binary (Server & TUI Client)
Both the server and the client will be written in **Go** and distributed as a single binary.

*   **Server (The Hub)**:
    *   **Lazy Singleton**: If a server is already running for the context, new commands no-op and connect to it over local IPC (e.g., Domain Sockets or gRPC). If not, it spins up lazily.
    *   **Process Owner**: Manages the lifecycle of all spawned processes.
    *   **Storage**: Persists ingested logs to a **DuckDB** database (embedded database) for heavily optimized chronological querying and filtering.
*   **Client (Terminal UI)**:
    *   Built in **Go** using **Bubbletea** and **Bubbles**.
    *   Inspired by a **Digital Audio Workstation (DAW)**: Processes are treated as "channels". The user can solo, mute, filter, and adjust the visibility ("volume") of different process streams.
    *   **Process "Channel Strips"**: Visual representation for each process, including health status, restart controls, and a **Sparkline** graph displaying log activity/frequency to give a high-level visual overview of system behavior.
    *   **Optimized Log Viewer**: Because the DuckDB backend will hold millions of rows, the TUI log viewer must heavily utilize **virtualization**. We cannot hold all logs in memory at once. We will likely need a custom, highly-optimized viewport (or a heavily extended `bubbles/viewport`) that fetches log chunks dynamically from the DuckDB store as the user scrolls.
    *   **Process Management Native Controls**: Ability to stop, restart, and view the health of individual processes directly via keyboard shortcuts.
    *   **Environments**: Primarily views the local `dev` environment, but has the architectural capability to switch and view `CI` logs or other environments.

## Configuration & Process Integration
`foobar` needs to cleanly manage and wrap processes from various sources without creating overly complex state models. We will exclusively support JSON for now (`foobar.config.json`), with YAML planned for the future.

### 1. Native Configuration (`foobar.config.json`)
Allows defining raw processes directly. Users can specify the command, working directory, and auto-restart policies (e.g., `restart_on_fail: true`, `max_retries: 3`).

### 2. Concurrently Wrapping
If a user is already using tools like `concurrently`, `foobar` can act as a drop-in replacement. Instead of reinventing the wheel, `foobar` will parse a `concurrently` definition and spin up, wrap, and manage each of those processes natively.

### 3. Docker Compose Integration
Integrating with Docker Compose requires a seamless and stateless approach to avoid wrestling with Docker's internal state machine.
*   **The Seamless Hook**: If the user's `foobar.config.json` specifies a `docker-compose.yml` file, `foobar` checks if the compose stack is running.
    *   If **not running**: `foobar` executes `docker compose up -d` to start the stack statelessly in the background.
    *   If **already running**: `foobar` skips the startup.
*   **Log Ingestion**: Instead of trying to wrap the Docker daemon, `foobar` creates lightweight goroutines that simply run `docker compose logs -f <service>` for each service defined in the compose file. These streams are then parsed and ingested into DuckDB exactly like native native processes.
*   **Process Control**: TUI commands to "restart" a Docker process simply translate to executing `docker compose restart <service>`. This keeps all heavy lifting on Docker and keeps `foobar` stateless and robust.

### 3. Agentic Integration (MCP Server)
The background server also functions as a **Model Context Protocol (MCP) Server**.
*   This allows AI assistants and agents to read, query, and analyze the aggregated logs directly from the `foobar` server, giving them deep context into the current state of the development environment.
*   While AI analysis isn't built *directly* into the TUI right away, this deterministic foundation (DuckDB + MCP) sets the stage for powerful AI workflows.

## Log Schema & Structure
`foobar` leans heavily into **Structured Logging**, inferring JSON structures where possible and falling back to raw text.

### Normalization Heuristic
1.  **Attempt JSON Parse**: Try to parse incoming stdout/stderr lines as JSON.
2.  **Identify Core Fields**: Extract timestamp, level, and message using common fallback keys (e.g., `time`/`timestamp`, `level`/`severity`, `msg`/`message`).
3.  **Fallback to Raw**: If parsing fails, assign the current timestamp, default level (`info` or `error`), and place the raw text into the message field.

### DuckDB Schema
The core table structure in DuckDB provides rigid typed columns for primary querying, and a flexible JSON column for everything else:

*   `id` (VARCHAR / UUID)
*   `timestamp` (TIMESTAMP)
*   `process_name` (VARCHAR) - e.g., "frontend", "api"
*   `process_id` (INTEGER) - The OS-level PID of the spawned process, crucial for disambiguation and tracing.
*   `level` (VARCHAR) - `INFO`, `WARN`, `ERROR`, `DEBUG`
*   `message` (VARCHAR) - The core log text.
*   `context` (JSON) - Any extra fields present in the structured log (e.g., `reqId`, `latency_ms`).
*   `stream` (VARCHAR) - `stdout` vs `stderr`

### Indexing Strategy & Out-of-Order Logs
DuckDB is a columnar analytical (OLAP) database. Instead of traditional B-Trees, it automatically creates **Zone Maps (Min-Max indexes)** for chunks of rows (row groups).

## Business Model & Monetization
The core local tool is open-source and free, optimized for individual developer velocity. Given the context of a solo developer looking for high executability and low operational overhead, the monetization strategies are focused on high-margin, low-maintenance models:

### 1. The "Obsidian Sync" Model (E2E Encrypted Sharing & Backup)
*   **Concept**: The core App is 100% free and local. You sell a lightweight SaaS subscription (e.g., $8/month) that provides simple, encrypted syncing of the DuckDB log database between a developer's machines.
*   **Key Feature - "Share this Trace"**: Allows a developer to click a button in the TUI, upload a specific time-slice of logs to an S3 bucket, and generate a secure, hosted web URL to share an error trace with a coworker.
*   **Why it works for a Solo Dev**: Extremely cheap infrastructure. You aren't processing millions of logs in a massive cluster; you are just storing small encrypted DuckDB chunks or static text blobs in Object Storage. High perceived value for the user, negligible maintenance for you.

### 2. Managed `foobar` Cloud (Agentic Log Pipeline)
*   **Concept**: A centralized cloud backend (e.g., using ClickHouse) where local `foobar` clients optionally forward logs for long-term retention and team-wide access.
*   **Value Prop**: Provides entire teams and their remote AI agents instant access to historical logs across *all* developer machines and CI/CD pipelines. This maximizes log access and visibility across the organization.
*   **Why it works for a Solo Dev**: While heavier on infrastructure than the Sync model, charging a premium enterprise rate (e.g., $50/user/mo or usage-based tiering) ensures the margins cover the ClickHouse hosting costs. It scales revenue directly with the size of the engineering teams adopting it.

## Edge Cases & Architectural Considerations

### 1. The Startup/Shutdown Race Condition
What happens if the TUI is forcefully closed (e.g., `kill -9` or a terminal crash)?
*   **The Daemon Approach**: The background Go server *must* detach and ignore `SIGHUP` from the terminal. If the TUI dies, the processes keep running.
*   **Orphan Management**: We need a deliberate shutdown command (`foobar stop`) that gracefully winds down the processes and the DuckDB connection. If the lockfile exists but the daemon process PID is dead, `foobar` must clean up the stale state on next run.

### 2. Log Rotation & Disk Usage
DuckDB is efficient, but appending millions of logs locally indefinitely will fill developers' hard drives.
*   **Retention Policies**: `foobar` needs a default retention policy (e.g., "delete logs older than 7 days" or "max DB size 5GB").
*   **Vacuuming**: We need a background job in the server that occasionally prunes old row groups to reclaim disk space automatically.

### 3. IPC IPC Security Model
Because the `foobar` server is a singleton running on the local machine exposing an MCP server:
*   **Socket Permissions**: If using a Unix Domain Socket, it must be restricted to the current user payload. We do not want malicious local scripts intercepting dev environment logs.
*   **MCP Authentication**: If exposing an HTTP/SSE endpoints for agents, it should bind exclusively to `localhost` or `127.0.0.1`.

### 4. Zero-Friction Telemetry / Tracing
While parsing stdout/stderr is the primary hook, modern apps use OTel (OpenTelemetry).
*   **OTel Receiver**: `foobar`'s background server should eventually act as a lightweight, local OTel Collector. If a developer's local app emits OTel traces/spans to `localhost:4317`, `foobar` intercepts them. This elevates the tool from just a "log viewer" to a full "local observability platform."

## Go-To-Market (GTM) Strategy & Risk Assessment

### The Core Bet
The fundamental GTM motion is **bottom-up, product-led growth driven by the AI tailwind.** Developers must *love* the local tool so much that they drag it into their company's environments.

### Core GTM Strategies
1.  **"The AI Debugger's Best Friend" (Positioning)**
    *   **The Pitch**: Stop pasting scattered logs into ChatGPT. Run `foobar` and give Cursor/Claude desktop native access to your entire dev environment's structured history via MCP.
    *   **The Motion**: Aggressively market the MCP server functionality. Create quick 30-second videos showing a developer running `foobar`, triggering a gnarly microservice bug, and having Cursor instantly identify the root cause across three different Docker containers because it has SQL access to DuckDB via MCP.
2.  **The Drop-In Replacement (Frictionless Adoption)**
    *   **The Pitch**: "A better `concurrently` that actually lets you read your logs."
    *   **The Motion**: Target users fatigued by messy, interleaved terminal output. By accepting a `concurrently` config or a `docker-compose.yml` natively without requiring any configuration rewrites, the time-to-value drops to zero.

### Advisor Assessment: The Biggest Risks
1.  **The "Habit" Risk (Status Quo Bias)**
    *   Developers have deep muscle memory for `docker compose up` and `npm run dev`. Getting them to type `foobar` instead requires the UI (the "DAW" experience) to be an order of magnitude better than their standard terminal emulator. If the TUI is clunky or slow, they will revert back instantly.
2.  **The Phantom Process Issue (Reliability Risk)**
    *   If a developer rage-quits the TUI and the background daemon fails to clean up properly, leaving zombie Node or Go processes listening on port 3000, they will burn the tool to the ground. Process management must be bulletproof on macOS, Linux, and Windows from day one.
3.  **The "Why Not Just X?" Risk**
    *   People will ask: "Why not just use Docker Desktop's log viewer?" or "Why not just pipe to `jq`?" The product must strongly index on the *AI enablement* (MCP) and the *cross-environment* capability (Native + Docker combined).
