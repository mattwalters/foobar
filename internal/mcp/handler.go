package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	foobarServer "mattwalters/foobar/internal/server"
	"mattwalters/foobar/internal/store"
)

// Handler wraps the MCP server and HTTP client
type Handler struct {
	server     *server.MCPServer
	httpClient *http.Client
}

// NewHandler creates a new MCP handler configured to talk to the foobar daemon
func NewHandler(socketPath string) *Handler {
	// Create the MCP server
	s := server.NewMCPServer(
		"foobar-logs",
		"1.0.0",
	)

	// Create an HTTP client that dials the local Unix socket
	client := &http.Client{
		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", socketPath)
			},
		},
	}

	h := &Handler{
		server:     s,
		httpClient: client,
	}

	h.registerTools()

	return h
}

// Start begins serving MCP JSON-RPC over Stdio
func (h *Handler) Start() error {
	return server.ServeStdio(h.server)
}

func (h *Handler) registerTools() {
	// Tool 1: get_active_processes
	getProcessesTool := mcp.NewTool("get_active_processes",
		mcp.WithDescription("Returns a list of all processes currently being managed by Foobar and their status (running, stopped, failed)."),
	)
	h.server.AddTool(getProcessesTool, h.handleGetProcesses)

	// Tool 2: query_logs
	queryLogsTool := mcp.NewTool("query_logs",
		mcp.WithDescription("Search the structured logs of managed processes to debug failures or read state."),
		mcp.WithString("process", mcp.Description("Optional. Filter by a specific process name (e.g., 'web', 'api'). If omitted, returns logs for all processes.")),
		mcp.WithNumber("limit", mcp.Description("Optional. Max number of logs to return. Default is 100.")),
	)
	h.server.AddTool(queryLogsTool, h.handleQueryLogs)
}

func (h *Handler) handleGetProcesses(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	resp, err := h.httpClient.Get("http://unix/processes")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to query daemon: %v", err)), nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return mcp.NewToolResultError(fmt.Sprintf("Daemon returned status %d: %s", resp.StatusCode, string(body))), nil
	}

	var procs []foobarServer.ProcessResponse
	if err := json.NewDecoder(resp.Body).Decode(&procs); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to decode response: %v", err)), nil
	}

	// Format nicely for the LLM
	var buf bytes.Buffer
	buf.WriteString("Active Processes:\n")
	for _, p := range procs {
		buf.WriteString(fmt.Sprintf("- %s: %s\n", p.Name, p.Status))
	}

	if len(procs) == 0 {
		buf.WriteString("(none)")
	}

	return mcp.NewToolResultText(buf.String()), nil
}

func (h *Handler) handleQueryLogs(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	params := url.Values{}

	// Parse arguments
	if processName := request.GetString("process", ""); processName != "" {
		params.Add("channel", processName)
	}

	if limitVal := request.GetFloat("limit", 0); limitVal > 0 {
		params.Add("limit", strconv.Itoa(int(limitVal)))
	} else {
		params.Add("limit", "100") // default
	}

	reqURL := "http://unix/logs"
	if len(params) > 0 {
		reqURL = reqURL + "?" + params.Encode()
	}

	resp, err := h.httpClient.Get(reqURL)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to query daemon logs: %v", err)), nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return mcp.NewToolResultError(fmt.Sprintf("Daemon returned status %d: %s", resp.StatusCode, string(body))), nil
	}

	var logs []store.LogEntry
	if err := json.NewDecoder(resp.Body).Decode(&logs); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to decode log response: %v", err)), nil
	}

	if len(logs) == 0 {
		return mcp.NewToolResultText("No logs found for the given criteria."), nil
	}

	// Format logs nicely for the LLM.
	// The DB returns logs ordered sequentially, newest last (or however GetRecentLogs was implemented, let's assume standard format).
	var buf bytes.Buffer
	for _, l := range logs {
		buf.WriteString(fmt.Sprintf("[%s] [%s] %s\n", l.Timestamp.Format("2006-01-02 15:04:05.000"), l.Process, l.Message))
	}

	return mcp.NewToolResultText(buf.String()), nil
}
