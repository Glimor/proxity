package socks5

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/Glimor/proxity/internal/domain"
)

// fakeServer is an in-process SOCKS5 server for tests. behavior controls the
// reply: "ok" grants, "noaccept" returns 0xff at the greeting, "badauth"
// fails RFC1929, "refused" replies rep=5, "unreachable" replies rep=4.
type fakeServer struct {
	ln           net.Listener
	behavior     string
	wantUser     string
	wantPass     string
	gotATYP      byte
	gotHost      string
	gotPort      int
	sentPayload  []byte
	closeOnWrite bool
}

func startFake(t *testing.T, behavior, user, pass string) *fakeServer {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeServer{ln: ln, behavior: behavior, wantUser: user, wantPass: pass}
	go f.serve()
	t.Cleanup(func() { ln.Close() })
	return f
}

func (f *fakeServer) serve() {
	for {
		c, err := f.ln.Accept()
		if err != nil {
			return
		}
		go f.handle(c)
	}
}

func (f *fakeServer) handle(c net.Conn) {
	defer c.Close()
	var g [2]byte
	if _, err := io.ReadFull(c, g[:]); err != nil || g[0] != 0x05 {
		return
	}
	methods := make([]byte, int(g[1]))
	if _, err := io.ReadFull(c, methods); err != nil {
		return
	}
	switch f.behavior {
	case "noaccept":
		c.Write([]byte{0x05, 0xff})
		return
	case "badauth", "auth":
		c.Write([]byte{0x05, 0x02})
		var h [2]byte
		if _, err := io.ReadFull(c, h[:]); err != nil {
			return
		}
		ub := make([]byte, int(h[1]))
		io.ReadFull(c, ub)
		var pl [1]byte
		io.ReadFull(c, pl[:])
		pb := make([]byte, int(pl[0]))
		io.ReadFull(c, pb)
		if f.behavior == "badauth" || string(ub) != f.wantUser || string(pb) != f.wantPass {
			c.Write([]byte{0x01, 0x01})
			return
		}
		c.Write([]byte{0x01, 0x00})
	default:
		c.Write([]byte{0x05, 0x00})
	}
	var req [4]byte
	if _, err := io.ReadFull(c, req[:]); err != nil {
		return
	}
	f.gotATYP = req[3]
	var host string
	switch req[3] {
	case 0x01:
		b := make([]byte, 4)
		io.ReadFull(c, b)
		host = net.IP(b).String()
	case 0x04:
		b := make([]byte, 16)
		io.ReadFull(c, b)
		host = net.IP(b).String()
	case 0x03:
		var l [1]byte
		io.ReadFull(c, l[:])
		b := make([]byte, int(l[0]))
		io.ReadFull(c, b)
		host = string(b)
	}
	var p [2]byte
	io.ReadFull(c, p[:])
	f.gotHost, f.gotPort = host, int(binary.BigEndian.Uint16(p[:]))
	rep := byte(0x00)
	switch f.behavior {
	case "refused":
		rep = 0x05
	case "unreachable":
		rep = 0x04
	}
	c.Write([]byte{0x05, rep, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
	if rep != 0 {
		return
	}
	// Echo server: reflect whatever the client sends.
	if f.sentPayload != nil {
		c.Write(f.sentPayload)
	}
	io.Copy(c, c)
}

func profileFor(t *testing.T, f *fakeServer, user string) *domain.ProxyProfile {
	host, port, _ := net.SplitHostPort(f.ln.Addr().String())
	p := &domain.ProxyProfile{ID: "p1", Host: host, TimeoutMS: 3000}
	p.Port = atoi(t, port)
	p.Username = user
	return p
}

func atoi(t *testing.T, s string) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func TestDialNoAuth(t *testing.T) {
	f := startFake(t, "ok", "", "")
	f.sentPayload = []byte("hello")
	conn, err := Dial(context.Background(), profileFor(t, f, ""), nil, "93.184.216.34", 443)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if f.gotATYP != 0x01 || f.gotHost != "93.184.216.34" || f.gotPort != 443 {
		t.Fatalf("bad connect req: atyp=%x host=%s port=%d", f.gotATYP, f.gotHost, f.gotPort)
	}
	buf := make([]byte, 5)
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := io.ReadFull(conn, buf); err != nil || string(buf) != "hello" {
		t.Fatalf("payload: %v %q", err, buf)
	}
}

func TestDialUserPassOK(t *testing.T) {
	f := startFake(t, "auth", "alice", "s3cret")
	conn, err := Dial(context.Background(), profileFor(t, f, "alice"), []byte("s3cret"), "example.com", 80)
	if err != nil {
		t.Fatal(err)
	}
	conn.Close()
	if f.gotATYP != 0x03 || f.gotHost != "example.com" {
		t.Fatalf("bad domain req: atyp=%x host=%s", f.gotATYP, f.gotHost)
	}
}

func TestDialUserPassBad(t *testing.T) {
	f := startFake(t, "auth", "alice", "s3cret")
	_, err := Dial(context.Background(), profileFor(t, f, "alice"), []byte("wrong"), "1.1.1.1", 80)
	var ae *domain.AppError
	if !errors.As(err, &ae) || ae.Code != "socks5.auth_failed" {
		t.Fatalf("want auth_failed, got %v", err)
	}
}

func TestDialNoAcceptableMethods(t *testing.T) {
	f := startFake(t, "noaccept", "", "")
	_, err := Dial(context.Background(), profileFor(t, f, ""), nil, "1.1.1.1", 80)
	var ae *domain.AppError
	if !errors.As(err, &ae) || ae.Code != "socks5.auth_failed" {
		t.Fatalf("want auth_failed, got %v", err)
	}
}

func TestDialRefusedAndUnreachable(t *testing.T) {
	for _, b := range []string{"refused", "unreachable"} {
		f := startFake(t, b, "", "")
		_, err := Dial(context.Background(), profileFor(t, f, ""), nil, "1.1.1.1", 80)
		var ae *domain.AppError
		if !errors.As(err, &ae) || ae.Code != "socks5.connect_failed" {
			t.Fatalf("%s: want connect_failed, got %v", b, err)
		}
	}
}

func TestDialIPv6ATYP(t *testing.T) {
	f := startFake(t, "ok", "", "")
	conn, err := Dial(context.Background(), profileFor(t, f, ""), nil, "2606:4700:4700::1111", 443)
	if err != nil {
		t.Fatal(err)
	}
	conn.Close()
	if f.gotATYP != 0x04 {
		t.Fatalf("want atyp 4, got %x", f.gotATYP)
	}
}

func TestDialProxyDown(t *testing.T) {
	p := &domain.ProxyProfile{ID: "x", Host: "127.0.0.1", Port: 1, TimeoutMS: 500}
	_, err := Dial(context.Background(), p, nil, "1.1.1.1", 80)
	var ae *domain.AppError
	if !errors.As(err, &ae) || ae.Code != "socks5.dial_failed" {
		t.Fatalf("want dial_failed, got %v", err)
	}
}

func TestStalledHandshakeStopsOnTimeoutOrCancellation(t *testing.T) {
	for _, cancelEarly := range []bool{false, true} {
		t.Run(strconv.FormatBool(cancelEarly), func(t *testing.T) {
			ln, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer ln.Close()
			accepted := make(chan struct{})
			go func() {
				conn, err := ln.Accept()
				if err != nil {
					return
				}
				defer conn.Close()
				close(accepted)
				io.Copy(io.Discard, conn) // never answer the SOCKS greeting
			}()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			timeout := 100
			if cancelEarly {
				timeout = 5000
				go func() { <-accepted; cancel() }()
			}
			addr := ln.Addr().(*net.TCPAddr)
			profile := &domain.ProxyProfile{Host: "127.0.0.1", Port: addr.Port, TimeoutMS: timeout}
			done := make(chan error, 1)
			go func() {
				conn, err := Dial(ctx, profile, nil, "example.com", 443)
				if conn != nil {
					conn.Close()
				}
				done <- err
			}()
			select {
			case err := <-done:
				if err == nil {
					t.Fatal("stalled handshake succeeded")
				}
			case <-time.After(2 * time.Second):
				cancel()
				t.Fatal("handshake ignored timeout/cancellation")
			}
		})
	}
}

func TestHandshakeDeadlineDoesNotExpireEstablishedTunnel(t *testing.T) {
	f := startFake(t, "ok", "", "")
	p := profileFor(t, f, "")
	p.TimeoutMS = 200
	conn, err := Dial(context.Background(), p, nil, "example.com", 80)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	time.Sleep(250 * time.Millisecond)
	conn.SetReadDeadline(time.Now().Add(time.Second))
	if _, err := conn.Write([]byte("hello")); err != nil {
		t.Fatalf("handshake deadline leaked to tunnel: %v", err)
	}
	buf := make([]byte, 5)
	if _, err := io.ReadFull(conn, buf); err != nil || string(buf) != "hello" {
		t.Fatalf("echo: %q %v", buf, err)
	}
}
