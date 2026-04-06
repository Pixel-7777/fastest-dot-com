package ui

import (
	"fmt"
	"os"
	"sort"
	"time"

	"fastest-dot-com/internal/processor"

	tea "github.com/charmbracelet/bubbletea"
)

func startRecording(m *model) {
	os.MkdirAll("logs", os.ModePerm)
	m.recordFilename = fmt.Sprintf("logs/diag_%s.log", time.Now().Format("20060102_150405"))
	f, err := os.Create(m.recordFilename)
	if err != nil {
		m.recordStatus = "Failed to create log file"
		return
	}

	m.isRecording = true
	m.recordFile = f
	m.recordStartTime = time.Now()

	m.startTotals = make(map[string]int64)
	for _, stats := range processor.Registry {
		m.startTotals[stats.AppName] += stats.TotalBytes
	}

	f.WriteString(fmt.Sprintf("=== NETWORK DIAGNOSTIC SESSION ===\nStart Time: %s\nTarget App: %s\n\n", m.recordStartTime.Format(time.RFC1123), processor.TargetApp))
	f.WriteString("Time,App,IP,Protocol,Mbps,Latency,Loss\n")
}

func stopRecording(m *model) tea.Cmd {
	if !m.isRecording || m.recordFile == nil {
		return nil
	}

	m.isRecording = false
	duration := time.Since(m.recordStartTime)

	type appStat struct {
		name  string
		bytes int64
	}
	var endTotals []appStat
	var sessionTotal int64

	for _, stats := range processor.Registry {
		diff := stats.TotalBytes - m.startTotals[stats.AppName]
		if diff > 0 {
			found := false
			for i, a := range endTotals {
				if a.name == stats.AppName {
					endTotals[i].bytes += diff
					found = true
					break
				}
			}
			if !found {
				endTotals = append(endTotals, appStat{name: stats.AppName, bytes: diff})
			}
			sessionTotal += diff
		}
	}

	sort.Slice(endTotals, func(i, j int) bool {
		return endTotals[i].bytes > endTotals[j].bytes
	})

	m.recordFile.WriteString(fmt.Sprintf("\n=== SESSION SUMMARY ===\n"))
	m.recordFile.WriteString(fmt.Sprintf("Duration: %v\n", duration.Round(time.Second)))
	m.recordFile.WriteString(fmt.Sprintf("Total Data Transferred: %s\n", formatBytes(uint64(sessionTotal))))
	m.recordFile.WriteString("Top Applications:\n")
	for i, app := range endTotals {
		if i >= 5 {
			break
		}
		m.recordFile.WriteString(fmt.Sprintf(" - %s: %s\n", app.name, formatBytes(uint64(app.bytes))))
	}

	m.recordFile.Close()
	m.recordFile = nil
	m.recordStatus = "✅ Saved to " + m.recordFilename

	return func() tea.Msg {
		time.Sleep(4 * time.Second)
		return clearStatusMsg{}
	}
}
