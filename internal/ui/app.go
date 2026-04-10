package ui

import (
	"fmt"
	"log"
	"os"
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
	pageAppSelect
	pageDeviceSelect
)

type tickMsg time.Time
type clearStatusMsg struct{}
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
	deviceDesc string
	packetPipe chan tracker.PacketInfo
	width      int
	height     int

	appCursor  int
	appChoices []string

	devCursor  int
	devChoices []capture.NetworkDevice

	thruGraph  *Graph
	goodGraph  *Graph
	speedGraph *Graph
	lastTotal  int64
	lastGood   int64

	stStatus string
	stPing   time.Duration
	stDL     float64
	stUL     float64
	stDone   bool

	isRecording     bool
	recordFile      *os.File
	recordFilename  string
	recordStartTime time.Time
	recordStatus    string
	startTotals     map[string]int64
}

var (
	headerStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7D56F4")).MarginBottom(1)
	activeStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF00")).Bold(true)
	tableHeader = lipgloss.NewStyle().Background(lipgloss.Color("#3C3C3C")).Foreground(lipgloss.Color("#FFFFFF")).Bold(true)
	lossStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000"))

	thruColor  = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF00FF"))
	goodColor  = lipgloss.NewStyle().Foreground(lipgloss.Color("#00FFFF"))
	speedColor = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFF00"))
)

func initialModel() model {
	return model{
		state:   pageMenu,
		choices: []string{"Real-time Monitor", "Track Specific App", "Select Network Device", "Test Internet Speed", "Exit"},
		// 1. Create a massive bucket (1 Million packet buffer)
		packetPipe: make(chan tracker.PacketInfo, 10000),
		thruGraph:  NewGraph("Throughput (Mbps)", 40, 8),
		goodGraph:  NewGraph("Goodput (Mbps)", 40, 8),
		speedGraph: NewGraph("Live Bandwidth Activity (Mbps)", 85, 12),
	}
}

func (m model) Init() tea.Cmd {
	return tea.Tick(500*time.Millisecond, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func startCaptureIfOff(m *model) {
	if m.device == "" {
		devName, devDesc, _ := capture.FindActiveDevice()
		m.device = devName
		m.deviceDesc = devDesc
		go capture.StartEngine(m.device, m.packetPipe)
	}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			if m.isRecording {
				stopRecording(&m)
			}
			return m, tea.Quit
		case "up", "k":
			if m.state == pageMenu && m.cursor > 0 {
				m.cursor--
			} else if m.state == pageAppSelect && m.appCursor > 0 {
				m.appCursor--
			} else if m.state == pageDeviceSelect && m.devCursor > 0 { // NEW
				m.devCursor--
			}
		case "down", "j":
			if m.state == pageMenu && m.cursor < len(m.choices)-1 {
				m.cursor++
			} else if m.state == pageAppSelect && m.appCursor < len(m.appChoices)-1 {
				m.appCursor++
			} else if m.state == pageDeviceSelect && m.devCursor < len(m.devChoices)-1 { // NEW
				m.devCursor++
			}
		case "r":
			if m.state == pageMonitor {
				if !m.isRecording {
					startRecording(&m)
					return m, nil
				} else {
					cmd := stopRecording(&m)
					return m, cmd
				}
			} else if m.state == pageDeviceSelect { // NEW: Refresh device list
				m.devChoices, _ = capture.GetAllDevices()
			}
		case "enter":
			if m.state == pageMenu {
				if m.cursor == 0 { // Real-time Monitor
					processor.TargetApp = ""
					m.state = pageMonitor
					startCaptureIfOff(&m)
					return m, nil
				}
				if m.cursor == 1 { // Track Specific App
					m.state = pageAppSelect
					m.appCursor = 0
					m.appChoices = capture.GetKnownApps()
					startCaptureIfOff(&m)
					return m, nil
				}
				if m.cursor == 2 { // NEW: Select Network Interface
					m.state = pageDeviceSelect
					m.devCursor = 0
					m.devChoices, _ = capture.GetAllDevices()
					return m, nil
				}
				if m.cursor == 3 { // Test Internet Speed (Shifted to 3)
					m.state = pageSpeedTest
					m.stStatus = "Locating closest Speedtest server..."
					m.stDone = false
					m.stPing = 0
					m.stDL = 0
					m.stUL = 0

					var cmds []tea.Cmd
					cmds = append(cmds, stInit())
					startCaptureIfOff(&m)
					return m, tea.Batch(cmds...)
				}
				if m.cursor == 4 { // Exit (Shifted to 4)
					return m, tea.Quit
				}
			} else if m.state == pageAppSelect {
				if len(m.appChoices) > 0 {
					processor.TargetApp = m.appChoices[m.appCursor]
					processor.RegistryLock.Lock()
					processor.Registry = make(map[string]*processor.Session)
					processor.RegistryLock.Unlock()
					m.state = pageMonitor
				}
			} else if m.state == pageDeviceSelect { // NEW: Handle device selection
				if len(m.devChoices) > 0 {
					selected := m.devChoices[m.devCursor]
					m.device = selected.Name
					m.deviceDesc = selected.Description

					// Update IP context
					if len(selected.IPs) > 0 {
						processor.LocalIP = selected.IPs[0]
					}

					// Safely restart the engine on the new device
					capture.StopEngine()
					go capture.StartEngine(m.device, m.packetPipe)

					// Wipe old stats
					processor.RegistryLock.Lock()
					processor.Registry = make(map[string]*processor.Session)
					processor.RegistryLock.Unlock()

					m.state = pageMenu
				}
			}
		case "esc":
			if m.state == pageMonitor {
				var cmd tea.Cmd
				if m.isRecording {
					cmd = stopRecording(&m)
				}
				m.state = pageMenu
				return m, cmd
			} else if m.state == pageAppSelect || m.state == pageDeviceSelect { // UPDATED
				m.state = pageMenu
			} else if m.state == pageSpeedTest && m.stDone {
				m.state = pageMenu
			}
		}

	case clearStatusMsg:
		m.recordStatus = ""
		return m, nil

	case stStepMsg:
		if msg.err != nil {
			// Append the error so the user sees where it failed
			m.stStatus += "\nError: " + msg.err.Error()
			m.stDone = true
			return m, nil
		}

		switch msg.step {
		case "ping":
			// "Connected to" becomes the first permanent line
			m.stStatus = fmt.Sprintf("Connected to: %s (%s)\nTesting Ping...",
				msg.server.Name, msg.server.Country)
			return m, stPing(msg.server)

		case "download":
			m.stPing = msg.server.Latency
			// Use \n to keep the previous lines and add a new status line
			m.stStatus += "\nTesting Download Speed..."
			return m, stDownload(msg.server)

		case "upload":
			// Capture DL speed then move to Upload
			m.stDL = msg.server.DLSpeed.Mbps()
			m.stStatus += "\nTesting Upload Speed..."
			return m, stUpload(msg.server)

		case "done":
			m.stUL = msg.server.ULSpeed.Mbps()
			m.stStatus += "\nSpeed Test Complete!"
			m.stDone = true
			return m, nil
		}
	case tickMsg:
		if m.state == pageAppSelect {
			m.appChoices = capture.GetKnownApps()
			sort.Strings(m.appChoices)
			if m.appCursor >= len(m.appChoices) && len(m.appChoices) > 0 {
				m.appCursor = len(m.appChoices) - 1
			}
		}

		// 3. Lock the data so the UI doesn't crash the background worker
		processor.RegistryLock.RLock()

		if m.isRecording && m.recordFile != nil {
			for ip, stats := range processor.Registry {
				if stats.CurrentRate > 0.01 { // Only log active streams
					line := fmt.Sprintf("%s,%s,%s,%s,%.2f,%v,%d\n",
						time.Now().Format("15:04:05"),
						stats.AppName,
						ip,
						stats.Protocol,
						stats.CurrentRate,
						stats.Latency.Round(time.Millisecond),
						stats.PacketLoss,
					)
					m.recordFile.WriteString(line)
				}
			}
		}

		var currentTotal, currentGood int64
		for _, stats := range processor.Registry {
			currentTotal += stats.TotalBytes
			currentGood += stats.PayloadBytes
		}

		// Unlock immediately after we finish reading
		processor.RegistryLock.RUnlock()

		if m.lastTotal != 0 {
			diffTotal := currentTotal - m.lastTotal
			diffGood := currentGood - m.lastGood

			// THE FIX: If the Garbage Collector just deleted a session,
			// the diff will be negative. Clamp it to 0 so the graph doesn't break.
			if diffTotal < 0 {
				diffTotal = 0
			}
			if diffGood < 0 {
				diffGood = 0
			}

			// Multiply by 2 to convert the 500ms diff into a full 1-second rate
			thruMbps := (float64(diffTotal) * 8 / 1000000) * 2
			goodMbps := (float64(diffGood) * 8 / 1000000) * 2

			m.thruGraph.AddPoint(thruMbps)
			m.goodGraph.AddPoint(goodMbps)
			m.speedGraph.AddPoint(thruMbps)
		}

		m.lastTotal = currentTotal
		m.lastGood = currentGood

		return m, tea.Tick(500*time.Millisecond, func(t time.Time) tea.Msg {
			return tickMsg(t)
		})
	}

	return m, nil
}

func Start() error {
	f, _ := tea.LogToFile("debug.log", "network-monitor")
	defer f.Close()
	log.SetOutput(f)

	m := initialModel()

	go func() {
		for pkt := range m.packetPipe {
			processor.Process(pkt)
		}
	}()

	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}
