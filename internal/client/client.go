package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"

	"github.com/mattwalters/foobar/internal/store"
)

type Client struct {
	httpClient *http.Client
	baseURL    string
}

func New(socketPath string) *Client {
	// Custom transport to dial unix socket
	transport := &http.Transport{
		DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
			return net.Dial("unix", socketPath)
		},
	}

	return &Client{
		httpClient: &http.Client{Transport: transport},
		baseURL:    "http://unix",
	}
}

func (c *Client) GetLogs(ctx context.Context) ([]store.LogEntry, error) {
	resp, err := c.httpClient.Get(c.baseURL + "/api/v1/logs")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %s", resp.Status)
	}

	var logs []store.LogEntry
	if err := json.NewDecoder(resp.Body).Decode(&logs); err != nil {
		return nil, err
	}
	return logs, nil
}
