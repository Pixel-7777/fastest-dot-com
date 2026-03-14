package main

import (
	"fmt"
	"fastest-dot-com/internal/capture"
	"fastest-dot-com/internal/processor"
	"fastest-dot-com/internal/tracker"
	"os"
	"sort"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// State management
type sessionState int

const (
	pageMenu sessionState = iota
	pageMonitor
)

type tickMsg time.Time

type model struct {
	state      sessionState
	cursor     int
	choices    []string
	device     string
	packetPipe chan tracker.PacketInfo
	width      int
	height     int
}

// Styles
var (
	headerStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7D56F4")).MarginBottom(1)
	activeStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF00")).Bold(true)
	tableHeader = lipgloss.NewStyle().Background(lipgloss.Color("#3C3C3C")).Foreground(lipgloss.Color("#FFFFFF")).Bold(true)
	lossStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000"))
)

func initialModel() model {
	return model{
		state:      pageMenu,
		choices:    []string{"Real-time Monitor", "Test Internet Speed {Not Implemented}", "Exit"},
		packetPipe: make(chan tracker.PacketInfo),
	}
}

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
				// Start the engine
				go capture.StartEngine(device, m.packetPipe)
				return m, listenForPackets(m.packetPipe)
			}
			if m.cursor == 2 {
				return m, tea.Quit
			}
		case "esc":
			m.state = pageMenu
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

func listenForPackets(pipe chan tracker.PacketInfo) tea.Cmd {
	return func() tea.Msg {
		return <-pipe
	}
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

	// MONITOR PAGE
	s := headerStyle.Render(fmt.Sprintf("LIVE MONITOR [%s] - Local: %s - Press ESC for Menu", m.device, processor.LocalIP)) + "\n"

	// FIXED: Table header now matches the width of the rows below
	headerRow := fmt.Sprintf("%-18s | %-8s | %-8s | %-8s | %-10s | %-10s | %-5s",
		"Remote IP", "In MB", "Out MB", "Mbps", "Latency", "Jitter", "Loss")
	s += tableHeader.Render(headerRow) + "\n"

	keys := make([]string, 0, len(processor.Registry))
	for k := range processor.Registry {
		keys = append(keys, k)
	}
	// Sort by IP string to prevent the list from jumping around
	sort.Strings(keys)

	count := 0
	for _, ip := range keys {
		if count > 18 { // Increased limit for larger terminals
			break
		}
		stats := processor.Registry[ip]

		// Filter out dead connections to keep UI clean
		if stats.TotalBytes == 0 {
			continue
		}

		// Calculate values
		inMB := float64(stats.InboundBytes) / 1024 / 1024
		outMB := float64(stats.OutboundBytes) / 1024 / 1024
		lat := stats.Latency.Round(time.Millisecond)
		jit := stats.AverageJitter.Round(time.Microsecond)

		// Color logic for Packet Loss
		lossStr := fmt.Sprintf("%-5d", stats.PacketLoss)
		if stats.PacketLoss > 0 {
			lossStr = lossStyle.Render(lossStr)
		}

		// Build the row string
		row := fmt.Sprintf("%-18s | %-8.2f | %-8.2f | %-8.2f | %-10v | %-10v | %s\n",
			ip, inMB, outMB, stats.CurrentRate, lat, jit, lossStr)

		s += row
		count++
	}

	return s
}

func main() {
	p := tea.NewProgram(initialModel(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Alas, there's been an error: %v", err)
		os.Exit(1)
	}
}
