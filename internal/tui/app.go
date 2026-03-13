package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
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
	Process   string    `json:"process"`
	Stream    string    `json:"stream"`
	Message   string    `json:"message"`
}

type tickMsg time.Time
type processesMsg []Process

type logsAction int
const (
	logsReset logsAction = iota
	logsAppend
	logsPrepend
)

type logsMsg struct {
	logs   []LogEntry
	action logsAction
}

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
	
	// Phase 11: Interleaved Logs & Channel Filtering
	visibleChannels map[string]bool // Empty map means "show all"
	
	// Phase 11: Infinite Scrolling & Memory limit
	logBuffer       []LogEntry
	isFetchingOlder bool
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
		socketPath:      socketPath,
		client:          client,
		visibleChannels: make(map[string]bool),
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

func (m *model) fetchLogsCmd(action logsAction, beforeTime, afterTime time.Time) tea.Cmd {
	return func() tea.Msg {
		// Build query parameters based on visible channels
		var queryParams []string
		
		// If map is strictly empty, we show all (do not append ?channel=)
		if len(m.visibleChannels) > 0 {
			for ch, visible := range m.visibleChannels {
				if visible {
					queryParams = append(queryParams, fmt.Sprintf("channel=%s", ch))
				}
			}
			
			// If user mutated all channels off (the map has entries but all are false)
			// we must prevent it from falling back to "show all" by passing a dummy channel
			if len(queryParams) == 0 {
				queryParams = append(queryParams, "channel=__none__")
			}
		}

		if !beforeTime.IsZero() {
			queryParams = append(queryParams, fmt.Sprintf("before_time=%s", beforeTime.Format(time.RFC3339Nano)))
		}
		if !afterTime.IsZero() {
			queryParams = append(queryParams, fmt.Sprintf("after_time=%s", afterTime.Format(time.RFC3339Nano)))
		}

		// Use larger limit for infinite scroll fetches, small limit for standard newest-refresh
		limit := 200
		if action == logsPrepend && len(queryParams) > 0 {
			limit = 1000
		}
		queryParams = append(queryParams, fmt.Sprintf("limit=%d", limit))
		
		url := fmt.Sprintf("http://unix/logs?%s", strings.Join(queryParams, "&"))
		resp, err := m.client.Get(url)
		if err != nil {
			return errMsg{err}
		}
		defer resp.Body.Close()

		var logs []LogEntry
		if err := json.NewDecoder(resp.Body).Decode(&logs); err != nil {
			return errMsg{err}
		}
		return logsMsg{logs: logs, action: action}
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
			}
		case "down", "j":
			if m.selectedProcess < len(m.processes)-1 {
				m.selectedProcess++
			}
		case "m": // Mute highlighted channel
			if len(m.processes) > 0 {
				pname := m.processes[m.selectedProcess].Name
				// If first filter action, set all others to true first
				if len(m.visibleChannels) == 0 {
					for _, p := range m.processes {
						m.visibleChannels[p.Name] = true
					}
				}
				m.visibleChannels[pname] = false
				cmds = append(cmds, m.fetchLogsCmd(logsReset, time.Time{}, time.Time{}))
			}
		case "s": // Solo highlighted channel
			if len(m.processes) > 0 {
				pname := m.processes[m.selectedProcess].Name
				// Clear map and only set the selected one to true
				m.visibleChannels = make(map[string]bool)
				m.visibleChannels[pname] = true
				cmds = append(cmds, m.fetchLogsCmd(logsReset, time.Time{}, time.Time{}))
			}
		case "a": // Show all channels
			m.visibleChannels = make(map[string]bool)
			cmds = append(cmds, m.fetchLogsCmd(logsReset, time.Time{}, time.Time{}))
		case "r": // We'll move start/stop/restart to shift keys since s is taken
			if len(m.processes) > 0 {
				cmds = append(cmds, m.controlProcessCmd(m.processes[m.selectedProcess].Name, "restart"))
			}
		case "S":
			if len(m.processes) > 0 {
				cmds = append(cmds, m.controlProcessCmd(m.processes[m.selectedProcess].Name, "start"))
			}
		case "X":
			if len(m.processes) > 0 {
				cmds = append(cmds, m.controlProcessCmd(m.processes[m.selectedProcess].Name, "stop"))
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
			var afterTime time.Time
			if len(m.logBuffer) > 0 {
				afterTime = m.logBuffer[len(m.logBuffer)-1].Timestamp
			}
			cmds = append(cmds, m.fetchLogsCmd(logsAppend, time.Time{}, afterTime))
		}

	case processesMsg:
		m.processes = msg
		// Fetch logs initially
		if len(m.processes) > 0 && m.viewport.TotalLineCount() == 0 && len(m.logBuffer) == 0 {
			cmds = append(cmds, m.fetchLogsCmd(logsReset, time.Time{}, time.Time{}))
		}

	case logsMsg:
		m.isFetchingOlder = false
		if len(msg.logs) > 0 {
			switch msg.action {
			case logsReset:
				m.logBuffer = msg.logs
			case logsAppend:
				m.logBuffer = append(m.logBuffer, msg.logs...)
				if len(m.logBuffer) > 10000 {
					m.logBuffer = m.logBuffer[len(m.logBuffer)-10000:]
				}
			case logsPrepend:
				m.logBuffer = append(msg.logs, m.logBuffer...)
				if len(m.logBuffer) > 10000 {
					m.logBuffer = m.logBuffer[:10000]
				}
			}
		}

		// Rebuild viewport content from buffer only if we had new logs or reset
		if len(msg.logs) > 0 || msg.action == logsReset {
			// Determine color for process names
			channelColor := func(name string) lipgloss.Color {
				hash := fnv.New32a()
				hash.Write([]byte(name))
				colorKeys := []string{"39", "45", "51", "87", "111", "121", "141", "183", "213", "225"}
				idx := hash.Sum32() % uint32(len(colorKeys))
				return lipgloss.Color(colorKeys[idx])
			}

			var sb strings.Builder
			for _, entry := range m.logBuffer {
				timeStr := entry.Timestamp.Format("15:04:05")
				timeRendered := lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render(timeStr)
				
				procStyle := lipgloss.NewStyle().Foreground(channelColor(entry.Process)).Bold(true)
				procRendered := procStyle.Render(entry.Process)
				
				msgRendered := entry.Message
				if entry.Stream == "stderr" {
					msgRendered = lipgloss.NewStyle().Foreground(lipgloss.Color("167")).Render(msgRendered)
				}

				sb.WriteString(fmt.Sprintf("%s %s %s\n", timeRendered, procRendered, msgRendered))
			}
			
			isAtBottom := m.viewport.AtBottom()
			m.viewport.SetContent(sb.String())
			
			// Auto scroll to bottom smoothly if appending or reset
			if msg.action == logsReset || (msg.action == logsAppend && isAtBottom) {
				m.viewport.GotoBottom()
			}
		}

	case errMsg:
		m.err = msg.err
	}

	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)

	// Phase 11: Infinite Scroll
	// If the user scrolled near the top of the buffer, asynchronously fetch older logs
	if m.viewport.YOffset < 10 && len(m.logBuffer) > 0 && !m.isFetchingOlder {
		m.isFetchingOlder = true
		beforeTime := m.logBuffer[0].Timestamp
		cmds = append(cmds, m.fetchLogsCmd(logsPrepend, beforeTime, time.Time{}))
	}

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
		// Determine visibility icon
		visIcon := " "
		if len(m.visibleChannels) == 0 {
			visIcon = "●" // all visible by default
		} else if m.visibleChannels[p.Name] {
			visIcon = "●"
		} else {
			visIcon = "○"
		}
		
		statusStr := fmt.Sprintf("[%s]", p.Status)
		row := fmt.Sprintf("%-2s %-12s %s", visIcon, p.Name, statusStr)
		if i == m.selectedProcess {
			listBuilder.WriteString(selStyle.Render(">", row) + "\n")
		} else {
			listBuilder.WriteString(itemStyle.Render(row) + "\n")
		}
	}

	for i := len(m.processes); i < m.height-17; i++ {
		listBuilder.WriteString("\n")
	}
	
	// Add footer instructions
	keyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("205")).Bold(true)
	descStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("241"))

	footerText := fmt.Sprintf("\n\n Filter Views:\n %s %s\n %s %s\n %s %s\n\n Controls:\n %s %s\n %s %s\n %s %s\n %s %s",
		keyStyle.Render("s"), descStyle.Render("solo channel"),
		keyStyle.Render("m"), descStyle.Render("mute channel"),
		keyStyle.Render("a"), descStyle.Render("show all channels"),
		keyStyle.Render("S"), descStyle.Render("start"),
		keyStyle.Render("X"), descStyle.Render("stop"),
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
