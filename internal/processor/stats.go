package processor

import (
	"fastest-dot-com/internal/tracker"
	"sync"
	"time"
)

type Session struct {
	AppName string
	RemoteIP string
	Port int
	Protocol string
	TotalBytes int64
	PayloadBytes int64
	InboundBytes int64
	OutboundBytes int64

	LastPacketAt time.Time
	AverageJitter time.Duration
	Latency time.Duration

	WindowBytes int64
	LastWindowAt time.Time
	CurrentRate float64

	LastInboundSeq uint32
	LastOutboundSeq uint32
	PacketLoss int
}

var Registry = make(map[string]*Session)
var RegistryLock sync.RWMutex
var LocalIP string
var TargetApp string

func init() {
	go func() {
		for {
			// Run the cleaner every 30 seconds
			time.Sleep(30 * time.Second)

			RegistryLock.Lock()
			now := time.Now()
			for ip, session := range Registry {
				// If we haven't seen a packet from this IP in 60 seconds, delete it
				if now.Sub(session.LastPacketAt) > 60*time.Second {
					delete(Registry, ip)
				}
			}
			RegistryLock.Unlock()
		}
	}()
}

func Process(info tracker.PacketInfo) {
	RegistryLock.Lock()
	defer RegistryLock.Unlock()
	if TargetApp != "" && info.AppName != TargetApp {
		return
	}

	s, exists := Registry[info.RemoteIP]
	if !exists {
		s = &Session{RemoteIP: info.RemoteIP, AppName: info.AppName, Port: info.Port, LastWindowAt: time.Now()}
		Registry[info.RemoteIP] = s
	}

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
	s.PayloadBytes += int64(info.PayloadSize)
	s.Protocol = info.Protocol

	s.WindowBytes += int64(info.Size)
	duration := now.Sub(s.LastWindowAt)
	if duration >= 500*time.Millisecond {
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
		if info.IsInbound {
			// Track incoming sequence numbers (Server -> You)
			if s.LastInboundSeq > 0 && info.SeqNum <= s.LastInboundSeq {
				if info.SeqNum == s.LastInboundSeq {
					// An exact duplicate means the server thought a packet dropped and re-sent it
					s.PacketLoss++
				}
			} else {
				s.LastInboundSeq = info.SeqNum
			}
		} else {
			// Track outgoing sequence numbers (You -> Server)
			if s.LastOutboundSeq > 0 && info.SeqNum <= s.LastOutboundSeq {
				if info.SeqNum == s.LastOutboundSeq {
					s.PacketLoss++
				}
			} else {
				s.LastOutboundSeq = info.SeqNum
			}
		}
	}

	s.LastPacketAt = now
}
