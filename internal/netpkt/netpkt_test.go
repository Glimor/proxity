package netpkt

import (
	"encoding/binary"
	"net/netip"
	"testing"
)

func buildV4(src, dst netip.Addr, sport, dport uint16, flags byte, payload []byte) []byte {
	ihl, tl := 20, 20
	b := make([]byte, ihl+tl+len(payload))
	b[0] = 0x45
	b[1] = 0
	binary.BigEndian.PutUint16(b[2:4], uint16(len(b)))
	binary.BigEndian.PutUint16(b[4:6], 0x1234)
	binary.BigEndian.PutUint16(b[6:8], 0x4000)
	b[8] = 64
	b[9] = ProtoTCP
	s, d := src.As4(), dst.As4()
	copy(b[12:16], s[:])
	copy(b[16:20], d[:])
	t := b[ihl:]
	binary.BigEndian.PutUint16(t[0:2], sport)
	binary.BigEndian.PutUint16(t[2:4], dport)
	binary.BigEndian.PutUint32(t[4:8], 0xdeadbeef)
	t[12] = byte(tl/4) << 4
	t[13] = flags
	binary.BigEndian.PutUint16(t[14:16], 65535)
	copy(b[ihl+tl:], payload)
	return b
}

func buildV6(src, dst netip.Addr, sport, dport uint16, flags byte) []byte {
	b := make([]byte, 40+20)
	b[0] = 0x60
	binary.BigEndian.PutUint16(b[4:6], 20)
	b[6] = ProtoTCP
	b[7] = 64
	s, d := src.As16(), dst.As16()
	copy(b[8:24], s[:])
	copy(b[24:40], d[:])
	t := b[40:]
	binary.BigEndian.PutUint16(t[0:2], sport)
	binary.BigEndian.PutUint16(t[2:4], dport)
	t[12] = 5 << 4
	t[13] = flags
	return b
}

func TestParseV4RoundTrip(t *testing.T) {
	src := netip.MustParseAddr("192.168.1.10")
	dst := netip.MustParseAddr("93.184.216.34")
	b := buildV4(src, dst, 49152, 443, FlagSYN, nil)
	p, ok := Parse(b)
	if !ok {
		t.Fatal("parse failed")
	}
	if p.IPv6 || p.SrcIP != src || p.DstIP != dst || p.SrcPort != 49152 || p.DstPort != 443 {
		t.Fatalf("bad fields: %+v", p)
	}
	if !p.IsSYN() {
		t.Fatal("expected SYN")
	}
	p.FixChecksums()
	if !p.ValidTCPChecksum() {
		t.Fatal("tcp checksum invalid after fix")
	}
	if finish(checksum(b[:20], 0)) != 0 {
		t.Fatal("ip checksum invalid after fix")
	}
}

func TestParseV6RoundTrip(t *testing.T) {
	src := netip.MustParseAddr("2001:db8::1")
	dst := netip.MustParseAddr("2606:4700:4700::1111")
	b := buildV6(src, dst, 1024, 80, FlagSYN)
	p, ok := Parse(b)
	if !ok {
		t.Fatal("parse failed")
	}
	if !p.IPv6 || p.SrcIP != src || p.DstIP != dst {
		t.Fatalf("bad fields: %+v", p)
	}
	p.FixChecksums()
	if !p.ValidTCPChecksum() {
		t.Fatal("v6 tcp checksum invalid")
	}
}

// TestRewriteV4 mirrors the engine's redirect: src=A:sport dst=D:dport
// becomes src=D:natID dst=A:ProxyPort, with the natID in the source port.
func TestRewriteV4(t *testing.T) {
	src := netip.MustParseAddr("10.0.0.5")
	dst := netip.MustParseAddr("8.8.8.8")
	b := buildV4(src, dst, 5000, 53, FlagSYN|FlagACK, []byte{1, 2, 3})
	p, ok := Parse(b)
	if !ok {
		t.Fatal("parse failed")
	}
	if p.IsSYN() {
		t.Fatal("SYN|ACK must not be IsSYN")
	}
	p.SetSrc(dst, 300)
	p.SetDst(src, 37890)
	p.FixChecksums()
	if p.SrcIP != dst || p.DstIP != src || p.SrcPort != 300 || p.DstPort != 37890 {
		t.Fatalf("rewrite failed: %+v", p)
	}
	if !p.ValidTCPChecksum() {
		t.Fatal("checksum invalid after rewrite")
	}
}

func TestRewriteV6(t *testing.T) {
	src := netip.MustParseAddr("2001:db8::5")
	dst := netip.MustParseAddr("2001:db8::9")
	b := buildV6(src, dst, 5000, 443, FlagSYN)
	p, ok := Parse(b)
	if !ok {
		t.Fatal("parse failed")
	}
	p.SetSrc(dst, 77)
	p.SetDst(src, 37890)
	p.FixChecksums()
	if p.SrcPort != 77 || p.DstPort != 37890 || p.SrcIP != dst || p.DstIP != src {
		t.Fatalf("v6 rewrite failed: %+v", p)
	}
	if !p.ValidTCPChecksum() {
		t.Fatal("v6 checksum invalid after rewrite")
	}
}

func TestParseRejects(t *testing.T) {
	if _, ok := Parse([]byte{0x45}); ok {
		t.Fatal("short packet accepted")
	}
	b := buildV4(netip.MustParseAddr("1.1.1.1"), netip.MustParseAddr("2.2.2.2"), 1, 2, FlagSYN, nil)
	b[9] = 17 // UDP
	if _, ok := Parse(b); ok {
		t.Fatal("udp accepted")
	}
	b = buildV4(netip.MustParseAddr("1.1.1.1"), netip.MustParseAddr("2.2.2.2"), 1, 2, FlagSYN, nil)
	binary.BigEndian.PutUint16(b[6:8], 0x2001) // non-first fragment
	if _, ok := Parse(b); ok {
		t.Fatal("non-first fragment accepted")
	}
}


