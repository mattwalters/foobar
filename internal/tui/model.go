package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattwalters/foobar/internal/client"
	"github.com/mattwalters/foobar/internal/store"
)

type Model struct {
	Client     *client.Client
	Logs       []store.LogEntry
	Width      int
	Height     int
	Err        error
	SocketPath string
}

func New(socketPath string) Model {
	return Model{
		Client:     client.New(socketPath),
		SocketPath: socketPath,
	}
}

type logMsg []store.LogEntry
type errMsg error
type tickMsg time.Time

func (m Model) Init() tea.Cmd {
	return tickEvery()
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		}
	case tea.WindowSizeMsg:
		m.Width = msg.Width
		m.Height = msg.Height

	case tickMsg:
		// Fetch logs now
		return m, m.fetchLogs()

	case logMsg:
		m.Logs = msg
		// Schedule next tick
		return m, tickEvery()

	case errMsg:
		m.Err = msg
		return m, nil
	}
	return m, nil
}

func (m Model) View() string {
	if m.Err != nil {
		return fmt.Sprintf("Error: %v\nPress q to quit.", m.Err)
	}

	s := strings.Builder{}
	s.WriteString("Foobar Logs (Polling...)\n\n")

	// Very simple tail for now
	start := 0
	if len(m.Logs) > 20 {
		start = len(m.Logs) - 20
	}

	for _, l := range m.Logs[start:] {
		line := fmt.Sprintf("[%s] %s: %s", l.Timestamp.Format("15:04:05"), l.ProcessID, strings.TrimSpace(l.Raw))
		s.WriteString(line + "\n")
	}

	s.WriteString("\nPress 'q' to quit.")
	return s.String()
}

func (m Model) fetchLogs() tea.Cmd {
	return func() tea.Msg {
		logs, err := m.Client.GetLogs(context.Background())
		if err != nil {
			return errMsg(err)
		}
		return logMsg(logs)
	}
}

func tickEvery() tea.Cmd {
	return tea.Tick(500*time.Millisecond, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}
