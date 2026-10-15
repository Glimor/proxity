// Package netpkt parses and rewrites IPv4/IPv6 TCP packets captured by
// WinDivert. All multi-byte packet fields are big-endian; checksums are fully
// recomputed in Go after any address/port rewrite (the NIC may still offload
// pass-through packets, which we never touch).
package netpkt

import (
	"encoding/binary"
	"net/netip"
)

const (
	ProtoTCP = 6

	FlagFIN = 0x01
	FlagSYN = 0x02
	FlagRST = 0x04
	FlagPSH = 0x08
	FlagACK = 0x10
	FlagURG = 0x20
)

// Packet is a parsed IPv4/IPv6 TCP segment. Raw always starts at the IP
// header and covers the whole IP packet.
type Packet struct {
	Raw     []byte
	IPv6    bool
	IHL     int // IP header length in bytes
	SrcIP   netip.Addr
	DstIP   netip.Addr
	SrcPort uint16
	DstPort uint16
	Seq     uint32
	Ack     uint32
	Flags   byte
}

// Parse decodes an IPv4 or IPv6 packet carrying TCP. It returns ok=false for
// truncated packets, non-TCP payloads, IPv6 extension headers and non-first
// fragments — callers must pass those through unchanged.
func Parse(b []byte) (*Packet, bool) {
	if len(b) < 20 {
		return nil, false
	}
	p := &Packet{Raw: b}
	switch b[0] >> 4 {
	case 4:
		p.IHL = int(b[0]&0x0f) * 4
		if p.IHL < 20 || len(b) < p.IHL+20 || b[9] != ProtoTCP {
			return nil, false
		}
		if frag := binary.BigEndian.Uint16(b[6:8]); frag&0x1fff != 0 {
			return nil, false
		}
		p.SrcIP = netip.AddrFrom4([4]byte{b[12], b[13], b[14], b[15]})
		p.DstIP = netip.AddrFrom4([4]byte{b[16], b[17], b[18], b[19]})
	case 6:
		if len(b) < 60 || b[6] != ProtoTCP {
			return nil, false
		}
		p.IPv6 = true
		p.IHL = 40
		p.SrcIP = netip.AddrFrom16([16]byte(b[8:24]))
		p.DstIP = netip.AddrFrom16([16]byte(b[24:40]))
	default:
		return nil, false
	}
	t := b[p.IHL:]
	p.SrcPort = binary.BigEndian.Uint16(t[0:2])
	p.DstPort = binary.BigEndian.Uint16(t[2:4])
	p.Seq = binary.BigEndian.Uint32(t[4:8])
	p.Ack = binary.BigEndian.Uint32(t[8:12])
	p.Flags = t[13]
	if dOff := int(t[12]>>4) * 4; dOff < 20 || len(t) < dOff {
		return nil, false
	}
	return p, true
}

// IsSYN reports a connection-opening segment (SYN set, ACK clear).
func (p *Packet) IsSYN() bool { return p.Flags&(FlagSYN|FlagACK) == FlagSYN }

func (p *Packet) tcp() []byte { return p.Raw[p.IHL:] }

// SetSrc rewrites the source address and port. The caller must call
// FixChecksums before reinjection.
func (p *Packet) SetSrc(ip netip.Addr, port uint16) {
	if p.IPv6 {
		b := ip.As16()
		copy(p.Raw[8:24], b[:])
	} else {
		b := ip.As4()
		copy(p.Raw[12:16], b[:])
	}
	binary.BigEndian.PutUint16(p.tcp()[0:2], port)
	p.SrcIP, p.SrcPort = ip, port
}

// SetDst rewrites the destination address and port.
func (p *Packet) SetDst(ip netip.Addr, port uint16) {
	if p.IPv6 {
		b := ip.As16()
		copy(p.Raw[24:40], b[:])
	} else {
		b := ip.As4()
		copy(p.Raw[16:20], b[:])
	}
	binary.BigEndian.PutUint16(p.tcp()[2:4], port)
	p.DstIP, p.DstPort = ip, port
}

// FixChecksums recomputes the IPv4 header checksum and the TCP checksum
// (including the v4/v6 pseudo-header) after any field rewrite.
func (p *Packet) FixChecksums() {
	t := p.tcp()
	t[16], t[17] = 0, 0
	var sum uint32
	if p.IPv6 {
		sum = checksum(p.Raw[8:24], sum)
		sum = checksum(p.Raw[24:40], sum)
		l := uint32(len(p.Raw) - p.IHL)
		sum += (l >> 16) + (l & 0xffff)
		sum += ProtoTCP
	} else {
		p.Raw[10], p.Raw[11] = 0, 0
		ipSum := finish(checksum(p.Raw[:p.IHL], 0))
		binary.BigEndian.PutUint16(p.Raw[10:12], ipSum)
		sum = checksum(p.Raw[12:20], 0)
		sum += ProtoTCP
		sum += uint32(len(p.Raw) - p.IHL)
	}
	c := finish(checksum(t, sum))
	if c == 0 {
		c = 0xffff
	}
	binary.BigEndian.PutUint16(t[16:18], c)
}

// ValidTCPChecksum recomputes the TCP checksum and compares it with the
// stored field. Packets captured outbound may carry an offloaded (invalid)
// checksum — use only on packets known to be checksummed.
func (p *Packet) ValidTCPChecksum() bool {
	var sum uint32
	if p.IPv6 {
		sum = checksum(p.Raw[8:24], 0)
		sum = checksum(p.Raw[24:40], sum)
		l := uint32(len(p.Raw) - p.IHL)
		sum += (l >> 16) + (l & 0xffff) + ProtoTCP
	} else {
		sum = checksum(p.Raw[12:20], 0)
		sum += ProtoTCP + uint32(len(p.Raw)-p.IHL)
	}
	return finish(checksum(p.tcp(), sum)) == 0
}

func checksum(b []byte, sum uint32) uint32 {
	for len(b) >= 2 {
		sum += uint32(b[0])<<8 | uint32(b[1])
		b = b[2:]
	}
	if len(b) == 1 {
		sum += uint32(b[0]) << 8
	}
	return sum
}

func finish(sum uint32) uint16 {
	for sum>>16 != 0 {
		sum = (sum & 0xffff) + (sum >> 16)
	}
	return uint16(^sum)
}
