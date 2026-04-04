package ui

import (
	"fmt"
	"sort"
	"time"

	"fastest-dot-com/internal/capture"
	"fastest-dot-com/internal/processor"
	"fastest-dot-com/internal/tracker"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/showwin/speedtest-go/speedtest"
)

type sessionState int

const (
	pageMenu sessionState = iota
	pageMonitor
	pageSpeedTest
)

type tickMsg time.Time
type stStepMsg struct {
	step   string
	server *speedtest.Server
	err    error
}

type model struct {
	state      sessionState
	cursor     int
	choices    []string
	device     string
	packetPipe chan tracker.PacketInfo
	width      int
	height     int

	// Speed Test Variables
	stStatus string
	stPing   time.Duration
	stDL     float64
	stUL     float64
	stDone   bool
}

// styles
var (
	headerStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7D56F4")).MarginBottom(1)
	activeStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF00")).Bold(true)
	tableHeader = lipgloss.NewStyle().Background(lipgloss.Color("#3C3C3C")).Foreground(lipgloss.Color("#FFFFFF")).Bold(true)
	lossStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000"))
)

func initialModel() model {
	return model{
		state:      pageMenu,
		choices:    []string{"Real-time Monitor", "Test Internet Speed", "Exit"},
		packetPipe: make(chan tracker.PacketInfo),
	}
}

// bubble tea interface
func (m model) Init() tea.Cmd {
	return tea.Tick(time.Millisecond*800, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.choices)-1 {
				m.cursor++
			}
		case "enter":
			if m.cursor == 0 {
				m.state = pageMonitor
				device, _ := capture.FindActiveDevice()
				m.device = device
				go capture.StartEngine(device, m.packetPipe)
				return m, listenForPackets(m.packetPipe)
			}
			if m.cursor == 1 {
				m.state = pageSpeedTest
				m.stStatus = "Locating closest Speedtest server..."
				m.stDone = false
				m.stPing = 0
				m.stDL = 0
				m.stUL = 0
				return m, stInit()
			}
			if m.cursor == 2 {
				return m, tea.Quit
			}
		case "esc":
			if m.state == pageMonitor {
				m.state = pageMenu
			} else if m.state == pageSpeedTest && m.stDone {
				m.state = pageMenu
			}
		}

	case stStepMsg:
		if msg.err != nil {
			m.stStatus = "Error: " + msg.err.Error()
			m.stDone = true
			return m, nil
		}
		if msg.step == "ping" {
			m.stStatus = fmt.Sprintf("Connected to: %s\nTesting Ping...", msg.server.Name)
			return m, stPing(msg.server)
		} else if msg.step == "download" {
			m.stPing = msg.server.Latency
			m.stStatus = "Testing Download Speed..."
			return m, stDownload(msg.server)
		} else if msg.step == "upload" {
			m.stDL = msg.server.DLSpeed.Mbps()
			m.stStatus = "Testing Upload Speed..."
			return m, stUpload(msg.server)
		} else if msg.step == "done" {
			m.stUL = msg.server.ULSpeed.Mbps()
			m.stStatus = "🚀 Speed Test Complete!"
			m.stDone = true
			return m, nil
		}

	case tracker.PacketInfo:
		processor.Process(msg)
		return m, listenForPackets(m.packetPipe)

	case tickMsg:
		return m, tea.Tick(time.Millisecond*800, func(t time.Time) tea.Msg {
			return tickMsg(t)
		})
	}

	return m, nil
}

func (m model) View() string {
	if m.state == pageMenu {
		s := headerStyle.Render("─── NETWORK MONITOR PRO ───") + "\n\n"
		for i, choice := range m.choices {
			cursor := " "
			if m.cursor == i {
				cursor = ">"
				s += activeStyle.Render(fmt.Sprintf("%s %s", cursor, choice)) + "\n"
			} else {
				s += fmt.Sprintf("%s %s\n", cursor, choice)
			}
		}
		s += "\n(Use arrows to navigate, Enter to select)"
		return s
	}

	if m.state == pageSpeedTest {
		s := headerStyle.Render("─── ACTIVE INTERNET SPEED TEST ───") + "\n\n"
		s += m.stStatus + "\n\n"

		if m.stPing > 0 {
			s += fmt.Sprintf("Ping:     %v\n", m.stPing)
		}
		if m.stDL > 0 {
			s += fmt.Sprintf("Download: %.2f Mbps\n", m.stDL)
		}
		if m.stUL > 0 {
			s += fmt.Sprintf("Upload:   %.2f Mbps\n", m.stUL)
		}

		if m.stDone {
			s += "\n\nPress ESC to return to Menu"
		}
		return s
	}

	//monitor view
	s := headerStyle.Render(fmt.Sprintf("LIVE MONITOR [%s] - Local: %s - Press ESC for Menu", m.device, processor.LocalIP)) + "\n"

	headerRow := fmt.Sprintf("%-18s | %-16s | %-10s | %-9s | %-6s | %-6s | %-7s | %-8s | %-4s",
		"Application", "Remote IP", "Throughput", "Goodput", "In MB", "Out MB", "Mbps", "Latency", "Loss")
	s += tableHeader.Render(headerRow) + "\n"

	keys := make([]string, 0, len(processor.Registry))
	for k := range processor.Registry {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	count := 0
	for _, ip := range keys {
		if count > 18 {
			break
		}
		stats := processor.Registry[ip]

		if stats.TotalBytes == 0 {
			continue
		}

		inMB := float64(stats.InboundBytes) / 1024 / 1024
		outMB := float64(stats.OutboundBytes) / 1024 / 1024
		lat := stats.Latency.Round(time.Millisecond)

		lossStr := fmt.Sprintf("%-4d", stats.PacketLoss)
		if stats.PacketLoss > 0 {
			lossStr = lossStyle.Render(lossStr)
		}

		thruStr := formatBytes(uint64(stats.TotalBytes))
		goodStr := formatBytes(uint64(stats.PayloadBytes))

		row := fmt.Sprintf("%-18.18s | %-16.16s | %-10s | %-9s | %-6.2f | %-6.2f | %-7.2f | %-8v | %s\n",
			stats.AppName, ip, thruStr, goodStr, inMB, outMB, stats.CurrentRate, lat, lossStr)

		s += row
		count++
	}

	return s
}

// helpers & engine commands
func listenForPackets(pipe chan tracker.PacketInfo) tea.Cmd {
	return func() tea.Msg {
		return <-pipe
	}
}

func formatBytes(b uint64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

func stInit() tea.Cmd {
	return func() tea.Msg {
		client := speedtest.New()
		serverList, err := client.FetchServers()
		if err != nil {
			return stStepMsg{err: err}
		}
		targets, err := serverList.FindServer([]int{})
		if err != nil || len(targets) == 0 {
			return stStepMsg{err: fmt.Errorf("no server found")}
		}
		return stStepMsg{step: "ping", server: targets[0]}
	}
}

func stPing(s *speedtest.Server) tea.Cmd {
	return func() tea.Msg {
		s.PingTest(nil)
		return stStepMsg{step: "download", server: s}
	}
}

func stDownload(s *speedtest.Server) tea.Cmd {
	return func() tea.Msg {
		s.DownloadTest()
		return stStepMsg{step: "upload", server: s}
	}
}

func stUpload(s *speedtest.Server) tea.Cmd {
	return func() tea.Msg {
		s.UploadTest()
		return stStepMsg{step: "done", server: s}
	}
}

// Start function initializes the UI engine from the main function
func Start() error {
	p := tea.NewProgram(initialModel(), tea.WithAltScreen())
	_, err := p.Run()
	return err
}
