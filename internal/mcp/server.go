package mcp

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/mattwalters/foobar/internal/client"
)

type MCPServer struct {
	Client *client.Client
	Server *server.MCPServer
}

func New(socketPath string) *MCPServer {
	c := client.New(socketPath)
	s := server.NewMCPServer("foobar", "1.0.0")

	// Resource: Logs (all of them, roughly)
	s.AddResource(
		mcp.NewResource("logs://all", "All Logs", mcp.WithMIMEType("text/plain"), mcp.WithResourceDescription("Complete log stream")),
		func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
			logs, err := c.GetLogs(ctx)
			if err != nil {
				return nil, err
			}
			var content string
			// Simple join for now
			for _, l := range logs {
				content += fmt.Sprintf("[%s] %s: %s\n", l.Timestamp.Format("15:04:05"), l.ProcessID, l.Raw)
			}
			return []mcp.ResourceContents{
				mcp.TextResourceContents{
					URI:      request.Params.URI,
					MIMEType: "text/plain",
					Text:     content,
				},
			}, nil
		},
	)

	// Tool: read_logs
	s.AddTool(mcp.NewTool("read_logs",
		mcp.WithDescription("Read logs from the foobar development environment"),
		mcp.WithString("process_id", mcp.Description("Optional process ID to filter by")),
		mcp.WithNumber("limit", mcp.Description("Number of lines to read (default 100)")),
	), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// Fetch logs via client
		logs, err := c.GetLogs(ctx)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to fetch logs: %v", err)), nil
		}

		// Basic formatting
		var content string
		count := 0
		limit := request.GetInt("limit", 100)
		// processID := request.GetString("process_id", "") // TODO: filter by processID in client/store

		for _, l := range logs {
			if count >= limit {
				break
			}
			content += fmt.Sprintf("[%s] %s: %s\n", l.Timestamp.Format("15:04:05"), l.ProcessID, l.Raw)
			count++
		}

		return mcp.NewToolResultText(content), nil
	})

	return &MCPServer{
		Client: c,
		Server: s,
	}
}

func (s *MCPServer) Serve() error {
	// Serve over Stdio
	return server.ServeStdio(s.Server)
}
