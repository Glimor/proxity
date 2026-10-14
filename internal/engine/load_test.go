//go:build windows

package engine

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/netip"
	"os"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/Glimor/proxity/internal/domain"
)

// Load/stress harness for the transparent-proxy path — no WinDivert needed.
//
// Redirected packets never appear; instead flows are injected into the flow
// table directly and each client binds its source port to the flow's natID
// (the reflected packet's addressing: remote port = natID, remote addr =
// origDst — see listener.flowForRemote). What IS exercised for real: the
// listener accept loop, flow resolution, the SOCKS5 client dial, and the
// bidirectional relay.
//
// Manual test: skipped unless PROXITY_INTEG=1 (it binds 200 source ports
// and port 37890):
//
//	PROXITY_INTEG=1 go test ./internal/engine -run TestProxyPathLoad -v

func skipUnlessInteg(t *testing.T) {
	t.Helper()
	if os.Getenv("PROXITY_INTEG") != "1" {
		t.Skip("manual integration test; set PROXITY_INTEG=1")
	}
}

// echoServer reflects whatever it receives. Returns the bound port.
func echoServer(t *testing.T) (port int, done func()) {
	t.Helper()
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() { io.Copy(c, c); c.Close() }()
		}
	}()
	return ln.Addr().(*net.TCPAddr).Port, func() { ln.Close() }
}

// miniSocks is the smallest possible no-auth SOCKS5 CONNECT server: it
// honors the requested destination and then pipes both ways.
func miniSocks(t *testing.T) (port int, done func()) {
	t.Helper()
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go serveSocks(c)
		}
	}()
	return ln.Addr().(*net.TCPAddr).Port, func() { ln.Close() }
}

func serveSocks(c net.Conn) {
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(30 * time.Second))

	// greeting
	var head [2]byte
	if _, err := io.ReadFull(c, head[:]); err != nil || head[0] != 0x05 {
		return
	}
	if _, err := io.ReadFull(c, make([]byte, int(head[1]))); err != nil {
		return
	}
	if _, err := c.Write([]byte{0x05, 0x00}); err != nil { // no auth
		return
	}

	// connect request: ver cmd rsv atyp addr port
	if _, err := io.ReadFull(c, head[:]); err != nil || head[0] != 0x05 {
		return
	}
	var atyp [1]byte
	if _, err := io.ReadFull(c, atyp[:]); err != nil {
		return
	}
	var host string
	switch atyp[0] {
	case 0x01:
		var b [4]byte
		if _, err := io.ReadFull(c, b[:]); err != nil {
			return
		}
		host = net.IP(b[:]).String()
	case 0x04:
		var b [16]byte
		if _, err := io.ReadFull(c, b[:]); err != nil {
			return
		}
		host = net.IP(b[:]).String()
	case 0x03:
		var l [1]byte
		if _, err := io.ReadFull(c, l[:]); err != nil {
			return
		}
		name := make([]byte, int(l[0]))
		if _, err := io.ReadFull(c, name); err != nil {
			return
		}
		host = string(name)
	default:
		return
	}
	var pb [2]byte
	if _, err := io.ReadFull(c, pb[:]); err != nil {
		return
	}

	up, err := net.DialTimeout("tcp", net.JoinHostPort(host, fmt.Sprint(binary.BigEndian.Uint16(pb[:]))), 10*time.Second)
	if err != nil {
		c.Write([]byte{0x05, 0x05, 0x00, 0x01, 0, 0, 0, 0, 0, 0}) // conn refused
		return
	}
	defer up.Close()
	// success reply: bind 127.0.0.1:0
	c.Write([]byte{0x05, 0x00, 0x00, 0x01, 127, 0, 0, 1, 0, 0})

	_ = c.SetDeadline(time.Time{})
	_ = up.(*net.TCPConn).SetDeadline(time.Time{})

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		io.Copy(up, c)
		if t, ok := up.(*net.TCPConn); ok {
			_ = t.CloseWrite()
		}
	}()
	io.Copy(c, up)
	wg.Wait()
}

// TestProxyPathLoadNoLeaks pushes N concurrent connections through the real
// listener→SOCKS5→upstream path and verifies bytes flow and no goroutine or
// tracked-connection leaks remain afterwards.
func TestProxyPathLoadNoLeaks(t *testing.T) {
	skipUnlessInteg(t)

	const N = 200

	echoPort, echoDone := echoServer(t)
	defer echoDone()
	socksPort, socksDone := miniSocks(t)
	defer socksDone()

	e := New(Deps{Log: testLog()})
	e.UpdateConfig([]domain.ProxyProfile{{
		ID: "p1", Name: "local", Host: "127.0.0.1", Port: socksPort, Enabled: true,
	}}, nil, domain.DefaultSettings())

	// natIDs double as client source ports — park the allocator well above
	// the echo/socks ports and below the typical ephemeral range is fine;
	// 41000+ avoids accidental collisions with the 0-port listeners.
	e.flows.mu.Lock()
	e.flows.next = 41000
	e.flows.mu.Unlock()

	lns, err := e.openListeners(0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		for _, ln := range lns {
			ln.Close()
		}
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	for _, ln := range lns {
		go e.serveListener(ctx, ln)
	}
	// The test only dials the v4 listener.
	dst := &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: lns[0].Addr().(*net.TCPAddr).Port}
	self := netip.MustParseAddr("127.0.0.1")

	base := runtime.NumGoroutine()

	var wg sync.WaitGroup
	errCh := make(chan error, N)
	payload := []byte("proxity load test payload — echo me back")

	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			f := &flow{
				origSrc: self, origSrcPort: uint16(20000 + i),
				origDst: self, origDstPort: uint16(echoPort),
				pid:    os.Getpid(),
				action: domain.DecisionProxy, proxyID: "p1",
			}
			f.connID = e.agg.register(domain.Connection{
				PID: os.Getpid(), ExePath: "load.test", DstIP: "127.0.0.1",
				DstPort: echoPort, Action: domain.DecisionProxy, ProxyID: "p1",
			})
			if !e.flows.add(f, true) {
				errCh <- fmt.Errorf("flow %d: nat id exhausted", i)
				return
			}
			la := &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: int(f.natID)}
			c, err := net.DialTCP("tcp4", la, dst)
			if err != nil {
				e.flows.remove(f)
				e.agg.close(f.connID, domain.ConnFailed)
				errCh <- fmt.Errorf("flow %d dial: %w", i, err)
				return
			}
			defer c.Close()
			_ = c.SetDeadline(time.Now().Add(30 * time.Second))
			if _, err := c.Write(payload); err != nil {
				errCh <- fmt.Errorf("flow %d write: %w", i, err)
				return
			}
			got := make([]byte, len(payload))
			if _, err := io.ReadFull(c, got); err != nil {
				errCh <- fmt.Errorf("flow %d read: %w", i, err)
				return
			}
			if string(got) != string(payload) {
				errCh <- fmt.Errorf("flow %d echo mismatch: %q", i, got)
			}
		}(i)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Error(err)
	}

	// Everything reaped: the engine's tracked conns and the flow table must
	// drain once relays finish.
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		e.connsMu.Lock()
		open := len(e.conns)
		e.connsMu.Unlock()
		e.flows.mu.RLock()
		nf := len(e.flows.byID)
		e.flows.mu.RUnlock()
		if open == 0 && nf == 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	e.connsMu.Lock()
	left := len(e.conns)
	e.connsMu.Unlock()
	e.flows.mu.RLock()
	flowsLeft := len(e.flows.byID)
	e.flows.mu.RUnlock()
	if left != 0 || flowsLeft != 0 {
		t.Fatalf("leaked state: %d tracked conns, %d flows", left, flowsLeft)
	}

	e.closeListeners()
	cancel()
	// Let relay/accept goroutines exit, then compare goroutine counts.
	runtime.GC()
	time.Sleep(2 * time.Second)
	if g := runtime.NumGoroutine(); g > base+5 {
		t.Fatalf("goroutine leak: before=%d after=%d", base, g)
	}
	t.Logf("load ok: %d conns through SOCKS5, goroutines before=%d after=%d",
		N, base, runtime.NumGoroutine())
}
