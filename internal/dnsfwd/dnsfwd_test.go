package dnsfwd

import (
	"context"
	"encoding/binary"
	"io"
	"net"
	"net/netip"
	"strconv"
	"testing"
	"time"
)

// --- canned packets ---------------------------------------------------------

func encodeName(name string) []byte {
	var out []byte
	for _, p := range splitDots(name) {
		out = append(out, byte(len(p)))
		out = append(out, p...)
	}
	return append(out, 0)
}

func splitDots(s string) []string {
	var out []string
	cur := ""
	for _, c := range s {
		if c == '.' {
			out = append(out, cur)
			cur = ""
		} else {
			cur += string(c)
		}
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}

func buildQuery(id uint16, name string, qtype uint16) []byte {
	msg := []byte{byte(id >> 8), byte(id), 0x01, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	msg = append(msg, encodeName(name)...)
	msg = append(msg, byte(qtype>>8), byte(qtype), 0x00, 0x01)
	return msg
}

func rrA(namePtr uint16, ttl uint32, ip [4]byte) []byte {
	out := []byte{0xc0, byte(namePtr)}
	out = append(out, 0x00, 0x01, 0x00, 0x01)
	var tb [4]byte
	binary.BigEndian.PutUint32(tb[:], ttl)
	out = append(out, tb[:]...)
	out = append(out, 0x00, 0x04)
	return append(out, ip[:]...)
}

func rrAAAA(namePtr uint16, ttl uint32, ip [16]byte) []byte {
	out := []byte{0xc0, byte(namePtr)}
	out = append(out, 0x00, 0x1c, 0x00, 0x01)
	var tb [4]byte
	binary.BigEndian.PutUint32(tb[:], ttl)
	out = append(out, tb[:]...)
	out = append(out, 0x00, 0x10)
	return append(out, ip[:]...)
}

// cannedResponse builds a valid response echoing the query's id and question.
func cannedResponse(query []byte, a [4]byte, ttl uint32) []byte {
	resp := append([]byte(nil), query...)
	resp[2], resp[3] = 0x81, 0x80 // QR|RD, noerror
	binary.BigEndian.PutUint16(resp[6:8], 1)
	return append(resp, rrA(0x0c, ttl, a)...)
}

func TestParseAnswers(t *testing.T) {
	q := buildQuery(0x1234, "www.example.com", 1)
	resp := append([]byte(nil), q...)
	resp[2], resp[3] = 0x81, 0x80
	binary.BigEndian.PutUint16(resp[6:8], 3)
	// CNAME answer (skipped), then A + AAAA.
	cnameRD := encodeName("cdn.example.net")
	cname := []byte{0xc0, 0x0c, 0x00, 0x05, 0x00, 0x01, 0, 0, 0, 60, byte(len(cnameRD) >> 8), byte(len(cnameRD))}
	cname = append(cname, cnameRD...)
	resp = append(resp, cname...)
	resp = append(resp, rrA(0x0c, 300, [4]byte{93, 184, 216, 34})...)
	resp = append(resp, rrAAAA(0x0c, 60, [16]byte{0x26, 0x06, 0x28, 0x00, 0x02, 0x20, 0x00, 0x01, 0x02, 0x48, 0x18, 0x93, 0x25, 0xc8, 0x19, 0x46})...)

	name, ans, ok := ParseAnswers(resp)
	if !ok || name != "www.example.com" {
		t.Fatalf("qname=%q ok=%v", name, ok)
	}
	if len(ans) != 2 {
		t.Fatalf("answers=%d", len(ans))
	}
	if ans[0].IP != netip.MustParseAddr("93.184.216.34") || ans[0].TTL != 300 {
		t.Fatalf("a: %+v", ans[0])
	}
	if ans[1].IP != netip.MustParseAddr("2606:2800:220:1:248:1893:25c8:1946") || ans[1].TTL != 60 {
		t.Fatalf("aaaa: %+v", ans[1])
	}

	// Query (QR=0) and error rcodes are rejected.
	if _, _, ok := ParseAnswers(q); ok {
		t.Fatal("query accepted as response")
	}
	resp[3] = 0x83 // NXDOMAIN
	if _, _, ok := ParseAnswers(resp); ok {
		t.Fatal("nxdomain accepted")
	}
	resp[3] = 0x80
	// Truncated mid-answer → still returns harvested prefix, not hard fail.
	short := resp[:len(resp)-3]
	name, ans, ok = ParseAnswers(short)
	if len(ans) != 1 {
		t.Fatalf("truncated: n=%d", len(ans))
	}
	_ = name
	_ = ok
}

func TestBuildServFail(t *testing.T) {
	q := buildQuery(0xbeef, "test.local", 1)
	sf := BuildServFail(q)
	if len(sf) != len(q) {
		t.Fatalf("len=%d want question-only %d", len(sf), len(q))
	}
	flags := binary.BigEndian.Uint16(sf[2:4])
	if flags&flagQR == 0 || flags&rcodeMask != rcodeServ || flags&rdBit == 0 {
		t.Fatalf("flags=%04x", flags)
	}
	for _, off := range [3]int{6, 8, 10} {
		if binary.BigEndian.Uint16(sf[off:off+2]) != 0 {
			t.Fatalf("count at %d not zero", off)
		}
	}
	if sf[0] != 0xbe || sf[1] != 0xef {
		t.Fatal("id not preserved")
	}
	if BuildServFail([]byte{1, 2}) != nil {
		t.Fatal("short query should produce nil")
	}
}

func TestQName(t *testing.T) {
	if QName(buildQuery(1, "a.b.example.org", 28)) != "a.b.example.org" {
		t.Fatal("qname")
	}
	if QName([]byte{0}) != "" {
		t.Fatal("short msg")
	}
}

// --- forwarder ---------------------------------------------------------------

// fakeUpstream is a minimal DNS-over-TCP server returning cannedResponse.
func fakeUpstream(t *testing.T) (addr string, stop func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				for {
					var lb [2]byte
					if _, err := io.ReadFull(c, lb[:]); err != nil {
						return
					}
					n := int(binary.BigEndian.Uint16(lb[:]))
					q := make([]byte, n)
					if _, err := io.ReadFull(c, q); err != nil {
						return
					}
					resp := cannedResponse(q, [4]byte{203, 0, 113, 7}, 120)
					var ob [2]byte
					binary.BigEndian.PutUint16(ob[:], uint16(len(resp)))
					if _, err := c.Write(append(ob[:], resp...)); err != nil {
						return
					}
				}
			}(c)
		}
	}()
	return ln.Addr().String(), func() { ln.Close() }
}

func testForwarder(t *testing.T, dial DialFunc) *Forwarder {
	t.Helper()
	// Grab a free port so the UDP and TCP listeners share it.
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	f := NewForwarder(Config{Addrs: []string{addr}, Dial: dial})
	if err := f.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(f.Close)
	return f
}

func TestForwarderUDPandTCP(t *testing.T) {
	up, stop := fakeUpstream(t)
	defer stop()
	dial := func(ctx context.Context) (net.Conn, error) {
		var d net.Dialer
		return d.DialContext(ctx, "tcp", up)
	}
	f := testForwarder(t, dial)
	addr := f.Addrs()[0]

	// UDP round trip.
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer pc.Close()
	q := buildQuery(0x42, "secure.example.com", 1)
	if _, err := pc.WriteTo(q, mustAddr(t, addr)); err != nil {
		t.Fatal(err)
	}
	pc.SetReadDeadline(time.Now().Add(3 * time.Second))
	buf := make([]byte, 4096)
	n, _, err := pc.ReadFrom(buf)
	if err != nil {
		t.Fatal(err)
	}
	resp := buf[:n]
	if resp[0] != 0x00 || resp[1] != 0x42 {
		t.Fatal("id not echoed")
	}
	name, ans, ok := ParseAnswers(resp)
	if !ok || name != "secure.example.com" || len(ans) != 1 ||
		ans[0].IP != netip.MustParseAddr("203.0.113.7") {
		t.Fatalf("bad response parse name=%q ans=%v ok=%v", name, ans, ok)
	}
	// The answer fed the domain cache.
	if got, ok := f.LookupDomain(netip.MustParseAddr("203.0.113.7")); !ok || got != "secure.example.com" {
		t.Fatalf("cache lookup: %q %v", got, ok)
	}

	// TCP round trip.
	tc, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer tc.Close()
	q2 := buildQuery(0x43, "tcp.example.com", 1)
	var lb [2]byte
	binary.BigEndian.PutUint16(lb[:], uint16(len(q2)))
	if _, err := tc.Write(append(lb[:], q2...)); err != nil {
		t.Fatal(err)
	}
	tc.SetReadDeadline(time.Now().Add(3 * time.Second))
	if _, err := io.ReadFull(tc, lb[:]); err != nil {
		t.Fatal(err)
	}
	rn := int(binary.BigEndian.Uint16(lb[:]))
	rbuf := make([]byte, rn)
	if _, err := io.ReadFull(tc, rbuf); err != nil {
		t.Fatal(err)
	}
	if name, _, ok := ParseAnswers(rbuf); !ok || name != "tcp.example.com" {
		t.Fatalf("tcp response: %q %v", name, ok)
	}
}

func mustAddr(t *testing.T, s string) net.Addr {
	a, err := net.ResolveUDPAddr("udp", s)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func TestForwarderServFailNoProxy(t *testing.T) {
	f := testForwarder(t, nil)
	addr := f.Addrs()[0]
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer pc.Close()
	q := buildQuery(0x99, "x.example", 1)
	if _, err := pc.WriteTo(q, mustAddr(t, addr)); err != nil {
		t.Fatal(err)
	}
	pc.SetReadDeadline(time.Now().Add(3 * time.Second))
	buf := make([]byte, 512)
	n, _, err := pc.ReadFrom(buf)
	if err != nil {
		t.Fatal(err)
	}
	flags := binary.BigEndian.Uint16(buf[2:4])
	if flags&rcodeMask != rcodeServ {
		t.Fatalf("expected SERVFAIL, flags=%04x n=%d", flags, n)
	}
}

func TestDomainCacheTTL(t *testing.T) {
	c := newDomainCache()
	now := time.Now()
	c.nowFunc = func() time.Time { return now }
	ip := netip.MustParseAddr("192.0.2.1")
	c.put(ip, "ttl.example", 30*time.Second)
	if got, ok := c.Lookup(ip); !ok || got != "ttl.example" {
		t.Fatal("miss")
	}
	now = now.Add(31 * time.Second)
	if _, ok := c.Lookup(ip); ok {
		t.Fatal("expired entry still hits")
	}
	// TTL cap: >1h is clamped.
	now = time.Now()
	c.put(ip, "cap.example", 48*time.Hour)
	now = now.Add(2 * time.Hour)
	if _, ok := c.Lookup(ip); ok {
		t.Fatal("cap not applied")
	}
}
