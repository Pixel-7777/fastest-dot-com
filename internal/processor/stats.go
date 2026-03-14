package processor

import (
	"fastest-dot-com/internal/tracker"
	"time"
)

type Session struct {
	RemoteIP      string
	TotalBytes    int64
	PayloadBytes  int64
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
		s = &Session{RemoteIP: info.RemoteIP, LastWindowAt: time.Now()}
		Registry[info.RemoteIP] = s
	}

	now := time.Now()

	if info.IsInbound {
		s.InboundBytes += int64(info.Size)
	} else {
		s.OutboundBytes += int64(info.Size)
	}
	s.TotalBytes += int64(info.Size)

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