package desync

import (
	"context"
	"fmt"
	"log"
	"net"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"golang.org/x/net/ipv4"
)

type DesyncEngine struct {
	Config *EvasionConfig
	ctx    context.Context
	cancel context.CancelFunc
}

func NewDesyncEngine(cfg *EvasionConfig) *DesyncEngine {
	ctx, cancel := context.WithCancel(context.Background())
	return &DesyncEngine{
		Config: cfg,
		ctx:    ctx,
		cancel: cancel,
	}
}

// CheckRawSocketAccess verifies if the process has privileges (root/cap_net_raw) to open raw sockets.
func CheckRawSocketAccess() error {
	conn, err := net.ListenPacket("ip4:tcp", "0.0.0.0")
	if err != nil {
		return fmt.Errorf("insufficient privileges: %v", err)
	}
	conn.Close()
	return nil
}

// InjectFakePacket crafts a raw TCP packet with a low TTL and Bad Checksum
// to poison the DPI state machine before the real V2Ray packet arrives.
func (e *DesyncEngine) InjectFakePacket(srcIP, dstIP net.IP, srcPort, dstPort uint16, seq uint32) error {
	if !e.Config.Enabled {
		return nil
	}

	// 1. Create Raw Socket (Requires Root/Admin)
	conn, err := net.ListenPacket("ip4:tcp", "0.0.0.0")
	if err != nil {
		return fmt.Errorf("raw socket failed (needs root): %v", err)
	}
	defer conn.Close()

	rawConn, err := ipv4.NewRawConn(conn)
	if err != nil {
		return err
	}

	// 2. Build the Fake IPv4 Header with our low TTL
	ipLayer := &layers.IPv4{
		SrcIP:    srcIP,
		DstIP:    dstIP,
		Version:  4,
		TTL:      e.Config.FakeTTL, // e.g., 5 - Dies at the Iran Firewall
		Protocol: layers.IPProtocolTCP,
	}

	// 3. Build the Fake TCP Header
	tcpLayer := &layers.TCP{
		SrcPort: layers.TCPPort(srcPort),
		DstPort: layers.TCPPort(dstPort),
		Seq:     seq, 
		PSH:     true,
		ACK:     true,
	}

	// If BadSeq is enabled, we corrupt the sequence number to desync the DPI
	if e.Config.InjectBadSeq {
		tcpLayer.Seq = seq + 10000 
	}

	tcpLayer.SetNetworkLayerForChecksum(ipLayer)

	// 4. Serialize the packet (only TCP + Payload, since ipv4.RawConn will prepend the IP header)
	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{
		ComputeChecksums: true, // Let gopacket calculate the correct sum first
		FixLengths:       true,
	}

	// Fake Payload (e.g., a dummy HTTP GET or invalid TLS SNI)
	fakePayload := gopacket.Payload([]byte("GET / HTTP/1.1\r\nHost: blockedsite.com\r\n\r\n"))

	err = gopacket.SerializeLayers(buf, opts, tcpLayer, fakePayload)
	if err != nil {
		return err
	}

	packetBytes := buf.Bytes()

	// 5. Corrupt the Checksum (BadSum Technique)
	if e.Config.InjectBadSum {
		// The TCP checksum is at bytes 16-17 of the TCP header. 
		// We manually flip bits to invalidate it.
		// Since we only serialized the TCP header, the offset is 0
		tcpHeaderOffset := 0
		if len(packetBytes) > tcpHeaderOffset+17 {
			packetBytes[tcpHeaderOffset+16] = ^packetBytes[tcpHeaderOffset+16]
			packetBytes[tcpHeaderOffset+17] = ^packetBytes[tcpHeaderOffset+17]
		}
	}

	ipv4Header := &ipv4.Header{
		Version:  4,
		Len:      20,
		TotalLen: 20 + len(packetBytes),
		TTL:      int(e.Config.FakeTTL),
		Protocol: 6, // TCP
		Dst:      dstIP,
		Src:      srcIP,
	}

	// 6. Blast the packet onto the network
	err = rawConn.WriteTo(ipv4Header, packetBytes, nil)
	if err != nil {
		return err
	}

	log.Printf("[Desync] Injected fake packet to %s:%d (TTL: %d, BadSum: %v)", 
		dstIP.String(), dstPort, e.Config.FakeTTL, e.Config.InjectBadSum)
	
	return nil
}
