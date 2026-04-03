package processor

import (
	"fastest-dot-com/internal/tracker"
	"time"
)

type Session struct {
	AppName       string // NEW: Store the application name
	RemoteIP      string
	TotalBytes    int64 // This serves as Throughput
	PayloadBytes  int64 // NEW: This serves as Goodput
	InboundBytes  int64
	OutboundBytes int64

	LastPacketAt  time.Time
	AverageJitter time.Duration
	Latency       time.Duration

	WindowBytes  int64
	LastWindowAt time.Time
	CurrentRate  float64

	LastSeqNum uint32
	PacketLoss int
}

var Registry = make(map[string]*Session)
var LocalIP string

func Process(info tracker.PacketInfo) {
	s, exists := Registry[info.RemoteIP]
	if !exists {
		s = &Session{RemoteIP: info.RemoteIP, AppName: info.AppName, LastWindowAt: time.Now()}
		Registry[info.RemoteIP] = s
	}

	// Update the AppName if it was previously unknown but we found it now
	if s.AppName == "System/Unknown" && info.AppName != "System/Unknown" {
		s.AppName = info.AppName
	}

	now := time.Now()

	if info.IsInbound {
		s.InboundBytes += int64(info.Size)
	} else {
		s.OutboundBytes += int64(info.Size)
	}
	s.TotalBytes += int64(info.Size)
	s.PayloadBytes += int64(info.PayloadSize) // NEW: Tally the Goodput

	s.WindowBytes += int64(info.Size)
	duration := now.Sub(s.LastWindowAt)
	if duration >= time.Second {
		s.CurrentRate = (float64(s.WindowBytes) * 8) / (1024 * 1024) / duration.Seconds()
		s.WindowBytes = 0
		s.LastWindowAt = now
	}

	if !s.LastPacketAt.IsZero() {
		gap := now.Sub(s.LastPacketAt)
		if s.Latency != 0 {
			diff := gap - s.Latency
			if diff < 0 {
				diff = -diff
			}
			s.AverageJitter = (s.AverageJitter * 9 / 10) + (diff / 10)
		}
		s.Latency = gap
	}

	if info.Protocol == "TCP" && info.SeqNum > 0 {
		if s.LastSeqNum > 0 && info.SeqNum <= s.LastSeqNum {
			s.PacketLoss++
		}
		s.LastSeqNum = info.SeqNum
	}

	s.LastPacketAt = now
}
