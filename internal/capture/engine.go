package capture

import (
	"fmt"
	"net"
	"fastest-dot-com/internal/processor"
	"fastest-dot-com/internal/tracker"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
)

func StartEngine(device string, out chan tracker.PacketInfo) error {
	// Find the local IP for direction logic
	iface, err := net.InterfaceByName(device)
	if err != nil {
		return err
	}
	addrs, err := iface.Addrs()
	if err != nil {
		return err
	}
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				processor.LocalIP = ipnet.IP.String()
				break
			}
		}
	}

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

	if tcpLayer := packet.Layer(layers.LayerTypeTCP); tcpLayer != nil {
		tcp, _ := tcpLayer.(*layers.TCP)
		protocol = "TCP"
		port = int(tcp.DstPort)
		seq = tcp.Seq
	} else if udpLayer := packet.Layer(layers.LayerTypeUDP); udpLayer != nil {
		udp, _ := udpLayer.(*layers.UDP)
		protocol = "UDP"
		port = int(udp.DstPort)
	}

	return &tracker.PacketInfo{
		RemoteIP:  remoteIP,
		Port:      port,
		Protocol:  protocol,
		Size:      len(packet.Data()),
		SeqNum:    seq,
		IsInbound: isIncoming,
	}
}

func FindActiveDevice() (string, error) {
	devices, err := pcap.FindAllDevs()
	if err != nil {
		return "", err
	}
	for _, d := range devices {
		if d.Name == "lo" || d.Name == "any" {
			continue
		}
		if len(d.Addresses) > 0 {
			return d.Name, nil
		}
	}
	return "", fmt.Errorf("no active network device found")
}
