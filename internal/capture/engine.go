package capture

import (
	"fmt"
	"sync"
	"time"

	"fastest-dot-com/internal/processor"
	"fastest-dot-com/internal/tracker"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
	psnet "github.com/shirou/gopsutil/v3/net"
	"github.com/shirou/gopsutil/v3/process"
)

var (
	portToApp = make(map[uint32]string)
	pidCache  = make(map[int32]string)
	mapLock   sync.RWMutex
)

func StartEngine(device string, out chan tracker.PacketInfo) error {
	// Start our background app mapper!
	go refreshProcessMap()

	// (We deleted the net.InterfaceByName block here because it crashes on Windows)

	handle, err := pcap.OpenLive(device, 1600, true, pcap.BlockForever)
	if err != nil {
		return err
	}

	source := gopacket.NewPacketSource(handle, handle.LinkType())

	go func() {
		defer handle.Close()
		for packet := range source.Packets() {
			info := parsePacket(packet)
			if info != nil {
				out <- *info
			}
		}
	}()

	return nil
}

func parsePacket(packet gopacket.Packet) *tracker.PacketInfo {
	ipLayer := packet.Layer(layers.LayerTypeIPv4)
	if ipLayer == nil {
		return nil
	}
	ip, _ := ipLayer.(*layers.IPv4)

	srcIP := ip.SrcIP.String()
	dstIP := ip.DstIP.String()

	var isIncoming bool
	var remoteIP string

	if srcIP == processor.LocalIP {
		isIncoming = false
		remoteIP = dstIP
	} else {
		isIncoming = true
		remoteIP = srcIP
	}

	protocol := "Other"
	var port int
	var seq uint32
	var localPort uint32
	var payloadSize int

	// Extract Goodput payload
	if appLayer := packet.ApplicationLayer(); appLayer != nil {
		payloadSize = len(appLayer.Payload())
	}

	if tcpLayer := packet.Layer(layers.LayerTypeTCP); tcpLayer != nil {
		tcp, _ := tcpLayer.(*layers.TCP)
		protocol = "TCP"
		port = int(tcp.DstPort)
		seq = tcp.Seq
		if isIncoming {
			localPort = uint32(tcp.DstPort)
		} else {
			localPort = uint32(tcp.SrcPort)
		}
	} else if udpLayer := packet.Layer(layers.LayerTypeUDP); udpLayer != nil {
		udp, _ := udpLayer.(*layers.UDP)
		protocol = "UDP"
		port = int(udp.DstPort)
		if isIncoming {
			localPort = uint32(udp.DstPort)
		} else {
			localPort = uint32(udp.SrcPort)
		}
	}

	// Map the port to the App Name
	mapLock.RLock()
	appName := portToApp[localPort]
	mapLock.RUnlock()
	if appName == "" {
		appName = "System/Unknown"
	}

	return &tracker.PacketInfo{
		RemoteIP:    remoteIP,
		Port:        port,
		Protocol:    protocol,
		Size:        len(packet.Data()),
		SeqNum:      seq,
		IsInbound:   isIncoming,
		AppName:     appName,
		PayloadSize: payloadSize,
	}
}

func refreshProcessMap() {
	for {
		conns, err := psnet.Connections("all")
		if err != nil {
			time.Sleep(2 * time.Second)
			continue
		}

		newPortMap := make(map[uint32]string)
		for _, conn := range conns {
			if conn.Pid == 0 {
				continue
			}

			name, known := pidCache[conn.Pid]
			if !known {
				proc, err := process.NewProcess(conn.Pid)
				if err == nil {
					name, _ = proc.Name()
					pidCache[conn.Pid] = name
				} else {
					name = "System/Unknown"
				}
			}
			newPortMap[conn.Laddr.Port] = name
		}

		mapLock.Lock()
		portToApp = newPortMap
		mapLock.Unlock()

		time.Sleep(2 * time.Second)
	}
}

func FindActiveDevice() (string, error) {
	devices, err := pcap.FindAllDevs()
	if err != nil {
		return "", err
	}

	for _, device := range devices {
		for _, address := range device.Addresses {
			ip := address.IP

			if ip.To4() != nil && !ip.IsLoopback() && !ip.IsLinkLocalUnicast() {
				// NEW: Set the Local IP right here where it's safe!
				processor.LocalIP = ip.String()
				return device.Name, nil
			}
		}
	}
	return "", fmt.Errorf("could not find an active internet connection")
}

func GetKnownApps() []string {
	mapLock.RLock()
	defer mapLock.RUnlock()

	// Use a map to get unique app names
	appSet := make(map[string]struct{})
	for _, app := range portToApp {
		if app != "System/Unknown" && app != "" {
			appSet[app] = struct{}{}
		}
	}

	var apps []string
	for app := range appSet {
		apps = append(apps, app)
	}
	return apps
}
