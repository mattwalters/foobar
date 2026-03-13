package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type Process struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

type LogEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Stream    string    `json:"stream"`
	Message   string    `json:"message"`
}

type tickMsg time.Time
type processesMsg []Process
type logsMsg []LogEntry
type errMsg struct{ err error }

func (e errMsg) Error() string { return e.err.Error() }

type model struct {
	socketPath      string
	client          *http.Client
	processes       []Process
	selectedProcess int
	viewport        viewport.Model
	ready           bool
	err             error
	width           int
	height          int
}

func New(socketPath string) *model {
	client := &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, addr string) (net.Conn, error) {
				return net.Dial("unix", socketPath)
			},
		},
		Timeout: 5 * time.Second,
	}

	return &model{
		socketPath: socketPath,
		client:     client,
	}
}

func (m *model) Init() tea.Cmd {
	return tea.Batch(
		m.fetchProcessesCmd(),
		tickCmd(),
	)
}

func tickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m *model) fetchProcessesCmd() tea.Cmd {
	return func() tea.Msg {
		// Uses dummy host since transport forces unix socket
		resp, err := m.client.Get("http://unix/processes")
		if err != nil {
			return errMsg{err}
		}
		defer resp.Body.Close()

		var procs []Process
		if err := json.NewDecoder(resp.Body).Decode(&procs); err != nil {
			return errMsg{err}
		}
		return processesMsg(procs)
	}
}

func (m *model) fetchLogsCmd(processName string) tea.Cmd {
	return func() tea.Msg {
		resp, err := m.client.Get(fmt.Sprintf("http://unix/logs?process=%s&limit=200", processName))
		if err != nil {
			return errMsg{err}
		}
		defer resp.Body.Close()

		var logs []LogEntry
		if err := json.NewDecoder(resp.Body).Decode(&logs); err != nil {
			return errMsg{err}
		}
		return logsMsg(logs)
	}
}

func (m *model) controlProcessCmd(processName, action string) tea.Cmd {
	return func() tea.Msg {
		url := fmt.Sprintf("http://unix/processes/%s?process=%s", action, processName)
		resp, err := m.client.Post(url, "application/json", nil)
		if err != nil {
			return errMsg{err}
		}
		defer resp.Body.Close()
		// Return a tick immediately to refresh the process list status
		return tickMsg(time.Now())
	}
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var (
		cmd  tea.Cmd
		cmds []tea.Cmd
	)

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "up", "k":
			if m.selectedProcess > 0 {
				m.selectedProcess--
				if len(m.processes) > 0 {
					cmds = append(cmds, m.fetchLogsCmd(m.processes[m.selectedProcess].Name))
				}
			}
		case "down", "j":
			if m.selectedProcess < len(m.processes)-1 {
				m.selectedProcess++
				if len(m.processes) > 0 {
					cmds = append(cmds, m.fetchLogsCmd(m.processes[m.selectedProcess].Name))
				}
			}
		case "s":
			if len(m.processes) > 0 {
				cmds = append(cmds, m.controlProcessCmd(m.processes[m.selectedProcess].Name, "start"))
			}
		case "x":
			if len(m.processes) > 0 {
				cmds = append(cmds, m.controlProcessCmd(m.processes[m.selectedProcess].Name, "stop"))
			}
		case "r":
			if len(m.processes) > 0 {
				cmds = append(cmds, m.controlProcessCmd(m.processes[m.selectedProcess].Name, "restart"))
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		listWidth := 30
		if !m.ready {
			m.viewport = viewport.New(msg.Width-listWidth-2, msg.Height-2)
			m.ready = true
		} else {
			m.viewport.Width = msg.Width - listWidth - 2
			m.viewport.Height = msg.Height - 2
		}

	case tickMsg:
		cmds = append(cmds, tickCmd(), m.fetchProcessesCmd())
		if len(m.processes) > 0 {
			cmds = append(cmds, m.fetchLogsCmd(m.processes[m.selectedProcess].Name))
		}

	case processesMsg:
		m.processes = msg
		// Fetch logs for the selected process initially if it's there
		if len(m.processes) > 0 && m.viewport.TotalLineCount() == 0 {
			cmds = append(cmds, m.fetchLogsCmd(m.processes[m.selectedProcess].Name))
		}

	case logsMsg:
		var sb strings.Builder
		for _, entry := range msg {
			timeStr := entry.Timestamp.Format("15:04:05")
			sb.WriteString(fmt.Sprintf("[%s] %s | %s\n", timeStr, entry.Stream, entry.Message))
		}
		
		isAtBottom := m.viewport.AtBottom()
		m.viewport.SetContent(sb.String())
		if isAtBottom || m.viewport.TotalLineCount() == 0 {
			m.viewport.GotoBottom()
		}

	case errMsg:
		m.err = msg.err
	}

	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

var (
	titleStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("205")).Bold(true)
	listStyle  = lipgloss.NewStyle().Border(lipgloss.NormalBorder()).Width(30)
	vpStyle    = lipgloss.NewStyle().Border(lipgloss.NormalBorder())
	itemStyle  = lipgloss.NewStyle().PaddingLeft(2)
	selStyle   = lipgloss.NewStyle().PaddingLeft(1).Foreground(lipgloss.Color("170")).Bold(true)
)

func (m *model) View() string {
	if !m.ready {
		return "Initializing foobar TUI...\n"
	}

	// Render Process List
	var listBuilder strings.Builder
	listBuilder.WriteString(titleStyle.Render(" Processes ") + "\n\n")
	
	if len(m.processes) == 0 {
		listBuilder.WriteString(itemStyle.Render("No processes found."))
	}

	for i, p := range m.processes {
		statusStr := fmt.Sprintf("[%s]", p.Status)
		row := fmt.Sprintf("%-15s %s", p.Name, statusStr)
		if i == m.selectedProcess {
			listBuilder.WriteString(selStyle.Render(">", row) + "\n")
		} else {
			listBuilder.WriteString(itemStyle.Render(row) + "\n")
		}
	}

	for i := len(m.processes); i < m.height-10; i++ {
		listBuilder.WriteString("\n")
	}
	
	// Add footer instructions
	keyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("205")).Bold(true)
	descStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("241"))

	footerText := fmt.Sprintf("\n\n %s %s\n %s %s\n %s %s\n %s %s",
		keyStyle.Render("s"), descStyle.Render("start"),
		keyStyle.Render("x"), descStyle.Render("stop"),
		keyStyle.Render("r"), descStyle.Render("restart"),
		keyStyle.Render("q"), descStyle.Render("quit"),
	)
	listBuilder.WriteString(footerText)
	
	listPane := listStyle.Height(m.height - 2).Render(listBuilder.String())
	vpPane := vpStyle.Width(m.width - 32).Height(m.height - 2).Render(m.viewport.View())

	return lipgloss.JoinHorizontal(lipgloss.Top, listPane, vpPane)
}

// Run starts the Bubbletea application
func Start(socketPath string) error {
	p := tea.NewProgram(New(socketPath), tea.WithAltScreen())
	_, err := p.Run()
	return err
}
