package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	foobarServer "mattwalters/foobar/internal/server"
	"mattwalters/foobar/internal/store"
)

// mockTransport lets us intercept HTTP requests before they hit the real unix socket
type mockTransport struct {
	responder func(req *http.Request) (*http.Response, error)
}

func (m *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return m.responder(req)
}

func TestHandler_HandleGetProcesses(t *testing.T) {
	h := NewHandler("fake.sock")

	// Intercept the client transport
	h.httpClient.Transport = &mockTransport{
		responder: func(req *http.Request) (*http.Response, error) {
			if req.URL.Path != "/processes" {
				t.Fatalf("unexpected path: %s", req.URL.Path)
			}

			// Mock a response with 1 running process
			procs := []foobarServer.ProcessResponse{
				{Name: "web", Status: "running"},
			}
			body, _ := json.Marshal(procs)

			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewReader(body)),
			}, nil
		},
	}

	res, err := h.handleGetProcesses(context.Background(), mcp.CallToolRequest{})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if res.IsError {
		t.Fatalf("expected IsError false")
	}

	content := res.Content[0].(mcp.TextContent).Text
	if !strings.Contains(content, "web: running") {
		t.Errorf("expected output to contain web: running, got %s", content)
	}
}

func TestHandler_HandleGetProcesses_Error(t *testing.T) {
	h := NewHandler("fake.sock")

	h.httpClient.Transport = &mockTransport{
		responder: func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusInternalServerError,
				Body:       io.NopCloser(strings.NewReader("internal server error")),
			}, nil
		},
	}

	res, err := h.handleGetProcesses(context.Background(), mcp.CallToolRequest{})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !res.IsError {
		t.Fatalf("expected IsError true")
	}
	
	content := res.Content[0].(mcp.TextContent).Text
	if !strings.Contains(content, "internal server error") {
		t.Errorf("expected error message in text content, got %s", content)
	}
}


func TestHandler_HandleQueryLogs(t *testing.T) {
	h := NewHandler("fake.sock")

	h.httpClient.Transport = &mockTransport{
		responder: func(req *http.Request) (*http.Response, error) {
			if req.URL.Path != "/logs" {
				t.Fatalf("unexpected path: %s", req.URL.Path)
			}

			// Verify URL params
			if req.URL.Query().Get("channel") != "api" {
				t.Errorf("expected channel 'api', got '%s'", req.URL.Query().Get("channel"))
			}
			if req.URL.Query().Get("limit") != "50" {
				t.Errorf("expected limit '50', got '%s'", req.URL.Query().Get("limit"))
			}

			logs := []store.LogEntry{
				{
					Process:   "api",
					Message:   "Starting database connection...",
					Timestamp: time.Date(2026, 3, 14, 12, 0, 0, 0, time.UTC),
				},
			}
			body, _ := json.Marshal(logs)

			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewReader(body)),
			}, nil
		},
	}

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"process": "api",
		"limit":   float64(50),
	}

	res, err := h.handleQueryLogs(context.Background(), req)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if res.IsError {
		t.Fatalf("expected IsError false")
	}

	content := res.Content[0].(mcp.TextContent).Text
	if !strings.Contains(content, "[api] Starting database connection...") {
		t.Errorf("expected output to contain log message, got %s", content)
	}
}

func TestHandler_HandleQueryLogs_Error(t *testing.T) {
	h := NewHandler("fake.sock")

	h.httpClient.Transport = &mockTransport{
		responder: func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusInternalServerError,
				Body:       io.NopCloser(strings.NewReader("database offline")),
			}, nil
		},
	}

	req := mcp.CallToolRequest{}
	res, err := h.handleQueryLogs(context.Background(), req)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !res.IsError {
		t.Fatalf("expected IsError true")
	}

	content := res.Content[0].(mcp.TextContent).Text
	if !strings.Contains(content, "database offline") {
		t.Errorf("expected error message in text content, got %s", content)
	}
}

