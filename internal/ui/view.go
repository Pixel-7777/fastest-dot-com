package ui

import (
	"fmt"
	"sort"
	"time"

	"fastest-dot-com/internal/processor"

	"github.com/charmbracelet/lipgloss"
)

func (m model) View() string {
	switch m.state {
	case pageMenu:
		return m.renderMenu()
	case pageAppSelect:
		return m.renderAppSelect()
	case pageSpeedTest:
		return m.renderSpeedTest()
	case pageMonitor:
		return m.renderMonitor()
	}
	return ""
}

func (m model) renderMenu() string {
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

func (m model) renderAppSelect() string {
	s := headerStyle.Render("─── SELECT APP TO TRACK ───") + "\n\n"
	if len(m.appChoices) == 0 {
		s += "Scanning for active applications... Please wait.\n"
	} else {
		for i, app := range m.appChoices {
			cursor := " "
			if m.appCursor == i {
				cursor = ">"
				s += activeStyle.Render(fmt.Sprintf("%s %s", cursor, app)) + "\n"
			} else {
				s += fmt.Sprintf("%s %s\n", cursor, app)
			}
		}
	}
	s += "\n(Use arrows to navigate, Enter to select, ESC to return)"
	return s
}

func (m model) renderSpeedTest() string {
	s := headerStyle.Render("─── ACTIVE INTERNET SPEED TEST ───") + "\n\n"
	s += speedColor.Render(m.speedGraph.View()) + "\n\n"
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

func (m model) renderMonitor() string {
	processor.RegistryLock.RLock()
	defer processor.RegistryLock.RUnlock()
	targetInfo := ""
	if processor.TargetApp != "" {
		targetInfo = fmt.Sprintf(" [Tracking: %s]", processor.TargetApp)
	}

	recIndicator := ""
	if m.isRecording {
		recIndicator = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000")).Render(" [🔴 RECORDING (Press 'r' to stop)]")
	} else if m.recordStatus != "" {
		recIndicator = lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF00")).Render(" [" + m.recordStatus + "]")
	}

	s := headerStyle.Render(fmt.Sprintf("LIVE MONITOR [%s] - Local: %s%s", m.device, processor.LocalIP, targetInfo)) +
		recIndicator + "\n(Press ESC for Menu, 'r' to Record)\n"

	graphs := lipgloss.JoinHorizontal(lipgloss.Top,
		lipgloss.NewStyle().MarginRight(4).Render(thruColor.Render(m.thruGraph.View())),
		goodColor.Render(m.goodGraph.View()),
	)
	s += graphs + "\n\n"

	// Proportion Bar
	appTotals := make(map[string]int64)
	var grandTotal int64 = 0
	for _, stats := range processor.Registry {
		appTotals[stats.AppName] += stats.TotalBytes
		grandTotal += stats.TotalBytes
	}

	type appStat struct {
		name  string
		bytes int64
	}
	var sortedApps []appStat
	for name, bytes := range appTotals {
		sortedApps = append(sortedApps, appStat{name, bytes})
	}
	sort.Slice(sortedApps, func(i, j int) bool {
		return sortedApps[i].bytes > sortedApps[j].bytes
	})

	if grandTotal > 0 {
		barWidth := 84
		pieChartStr := "Bandwidth Share: \n"
		legendStr := ""
		colors := []lipgloss.TerminalColor{
			lipgloss.Color("#00FFFF"), lipgloss.Color("#FF00FF"), lipgloss.Color("#FFFF00"),
			lipgloss.Color("#00FF00"), lipgloss.Color("#555555"),
		}

		var remainingWidth = barWidth
		for i, app := range sortedApps {
			if i >= 4 {
				break
			}
			percent := float64(app.bytes) / float64(grandTotal)
			blockWidth := int(percent * float64(barWidth))
			if blockWidth == 0 && percent > 0 {
				blockWidth = 1
			}
			if blockWidth > remainingWidth {
				blockWidth = remainingWidth
			}

			blockStyle := lipgloss.NewStyle().Foreground(colors[i])
			blocks := ""
			for b := 0; b < blockWidth; b++ {
				blocks += "█"
			}
			pieChartStr += blockStyle.Render(blocks)
			legendStr += blockStyle.Render("■ ") + fmt.Sprintf("%s (%.1f%%)  ", app.name, percent*100)
			remainingWidth -= blockWidth
		}

		if remainingWidth > 0 {
			otherStyle := lipgloss.NewStyle().Foreground(colors[4])
			blocks := ""
			for b := 0; b < remainingWidth; b++ {
				blocks += "█"
			}
			pieChartStr += otherStyle.Render(blocks)
			legendStr += otherStyle.Render("■ ") + "Other"
		}
		s += pieChartStr + "\n" + legendStr + "\n\n"
	}

	// Table
	headerRow := fmt.Sprintf("%-18s | %-16s | %-6s | %-5s | %-10s | %-9s | %-6s | %-6s | %-7s | %-8s | %-4s",
		"Application", "Remote IP", "Port", "Protocol", "Throughput", "Goodput", "In MB", "Out MB", "Mbps", "Latency", "Loss")
	s += tableHeader.Render(headerRow) + "\n"

	type sessionItem struct {
		ip    string
		stats *processor.Session
	}
	var items []sessionItem
	for k, v := range processor.Registry {
		items = append(items, sessionItem{ip: k, stats: v})
	}
	// Sort by Total Throughput (highest to lowest)
	sort.Slice(items, func(i, j int) bool {
		return items[i].stats.TotalBytes > items[j].stats.TotalBytes
	})

	count := 0
	for _, item := range items {
		if count > 18 {
			break
		}
		ip := item.ip
		stats := item.stats

		inMB := float64(stats.InboundBytes) / 1024 / 1024
		outMB := float64(stats.OutboundBytes) / 1024 / 1024
		lat := stats.Latency.Round(time.Millisecond)

		lossStr := fmt.Sprintf("%-4d", stats.PacketLoss)
		if stats.PacketLoss > 0 {
			lossStr = lossStyle.Render(lossStr)
		}

		thruStr := formatBytes(uint64(stats.TotalBytes))
		goodStr := formatBytes(uint64(stats.PayloadBytes))

		row := fmt.Sprintf("%-18.18s | %-16.16s | %-6d | %-5s | %-10s | %-9s | %-6.2f | %-6.2f | %-7.2f | %-8v | %s\n",
			stats.AppName, ip, stats.Port, stats.Protocol, thruStr, goodStr, inMB, outMB, stats.CurrentRate, lat, lossStr)
		s += row
		count++
	}

	return s
}
