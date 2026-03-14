package tracker

import "time"

type Connection struct {
	RemoteIP string
	Port int
	Protocol string
	IsInbound bool
	Bytes int64
	LastSeen time.Time
}

type PacketInfo struct {
	RemoteIP string
	Port int
	Protocol string
	Size int
	IsInbound bool
	SeqNum uint32
	AckNum uint32
}
