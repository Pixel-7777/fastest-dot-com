package ui

import (
	"fmt"

	"fastest-dot-com/internal/tracker"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/showwin/speedtest-go/speedtest"
)

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