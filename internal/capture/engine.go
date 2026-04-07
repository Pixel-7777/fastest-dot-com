package capture

import (
	"fmt"
	"strings"
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
	go refreshProcessMap()

	// 1. Create an Inactive Handle so we can modify the raw driver settings
	inactive, err := pcap.NewInactiveHandle(device)
	if err != nil {
		return err
	}
	defer inactive.CleanUp()

	inactive.SetSnapLen(131072)
	// 2. TURN OFF PROMISCUOUS MODE (Crucial for Wi-Fi adapter stability)
	inactive.SetPromisc(false)
	inactive.SetTimeout(pcap.BlockForever)
	// 3. SET A MASSIVE KERNEL BUFFER (32MB instead of the tiny default)
	inactive.SetBufferSize(32 * 1024 * 1024)

	// 4. Activate the custom handle
	handle, err := inactive.Activate()
	if err != nil {
		return err
	}

	source := gopacket.NewPacketSource(handle, handle.LinkType())

	// 5. High-Performance Decoding (Saves massive CPU cycles)
	source.DecodeOptions.Lazy = true
	source.DecodeOptions.NoCopy = true

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
	var srcIP, dstIP string

	// Check for IPv4 first
	if ipv4Layer := packet.Layer(layers.LayerTypeIPv4); ipv4Layer != nil {
		ipv4, _ := ipv4Layer.(*layers.IPv4)
		srcIP = ipv4.SrcIP.String()
		dstIP = ipv4.DstIP.String()
	} else if ipv6Layer := packet.Layer(layers.LayerTypeIPv6); ipv6Layer != nil {
		// If not IPv4, check for IPv6
		ipv6, _ := ipv6Layer.(*layers.IPv6)
		srcIP = ipv6.SrcIP.String()
		dstIP = ipv6.DstIP.String()
	} else {
		// Not IP traffic (e.g. ARP packets), throw it away
		return nil
	}

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
		Size:        packet.Metadata().Length,
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
			time.Sleep(1 * time.Second)
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
				// Specifically target your local Wi-Fi router's subnet
				if strings.HasPrefix(ip.String(), "192.168.") {
					processor.LocalIP = ip.String()
					return device.Name, nil
				}
			}
		}
	}
	return "", fmt.Errorf("could not find an active internet connection")
}

func GetKnownApps() []string {
	mapLock.RLock()
	defer mapLock.RUnlock()

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
