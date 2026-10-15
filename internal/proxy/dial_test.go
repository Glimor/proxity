package proxy

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Glimor/proxity/internal/domain"
)

// Local servers only: no driver, public proxy, DNS lookup, or internet access.
func localProxy(t *testing.T, protocol domain.ProxyProtocol, handle func(net.Conn) error) (*domain.ProxyProfile, *tls.Config, <-chan error) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var config *tls.Config
	if protocol == domain.ProxyHTTPS {
		seed := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
		roots := x509.NewCertPool()
		roots.AddCert(seed.Certificate())
		config = &tls.Config{RootCAs: roots, ServerName: "example.com", MinVersion: tls.VersionTLS12}
		listener = tls.NewListener(listener, &tls.Config{Certificates: seed.TLS.Certificates, MinVersion: tls.VersionTLS12})
		seed.Close()
	}
	t.Cleanup(func() { listener.Close() })
	result := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			result <- err
			return
		}
		defer conn.Close()
		conn.SetDeadline(time.Now().Add(3 * time.Second))
		result <- handle(conn)
	}()
	address := listener.Addr().(*net.TCPAddr)
	return &domain.ProxyProfile{Protocol: protocol, Host: "127.0.0.1", Port: address.Port, TimeoutMS: 2000}, config, result
}

func TestProtocolTunnels(t *testing.T) {
	cases := []struct {
		name                 string
		protocol             domain.ProxyProtocol
		host, user, password string
	}{
		{"http", domain.ProxyHTTP, "target.invalid", "", ""},
		{"http-auth-ipv6", domain.ProxyHTTP, "2001:db8::1", "alice", "secret"},
		{"https", domain.ProxyHTTPS, "target.invalid", "", ""},
		{"https-auth", domain.ProxyHTTPS, "target.invalid", "alice", "secret"},
		{"socks4-ipv4", domain.ProxySOCKS4, "203.0.113.4", "", ""},
		{"socks4a-user-id", domain.ProxySOCKS4, "target.invalid", "alice", ""},
		{"socks5", domain.ProxySOCKS5, "target.invalid", "", ""},
		{"socks5-auth", domain.ProxySOCKS5, "target.invalid", "alice", "secret"},
		{"legacy-default", "", "target.invalid", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, config, done := localProxy(t, tc.protocol, func(c net.Conn) error {
				r := bufio.NewReader(c)
				switch tc.protocol.Normalized() {
				case domain.ProxyHTTP, domain.ProxyHTTPS:
					req, err := http.ReadRequest(r)
					if err != nil {
						return err
					}
					if req.Method != "CONNECT" || req.Host != net.JoinHostPort(tc.host, "443") {
						return fmt.Errorf("bad CONNECT: %s %s", req.Method, req.Host)
					}
					auth := req.Header.Get("Proxy-Authorization")
					if tc.user == "" && auth != "" {
						return fmt.Errorf("unexpected credentials")
					}
					if tc.user != "" && auth != "Basic YWxpY2U6c2VjcmV0" {
						return fmt.Errorf("bad Basic authentication")
					}
					// Include data with the headers to exercise buffered tunnel bytes.
					if _, err := io.WriteString(c, "HTTP/1.1 200 Connection established\r\n\r\nready"); err != nil {
						return err
					}
				case domain.ProxySOCKS4:
					var head [8]byte
					if _, err := io.ReadFull(r, head[:]); err != nil {
						return err
					}
					if head[0] != 4 || head[1] != 1 || binary.BigEndian.Uint16(head[2:4]) != 443 {
						return fmt.Errorf("bad SOCKS4 request")
					}
					user, err := r.ReadString(0)
					if err != nil || user != tc.user+"\x00" {
						return fmt.Errorf("bad SOCKS4 user ID")
					}
					if tc.host == "target.invalid" {
						if !bytes.Equal(head[4:], []byte{0, 0, 0, 1}) {
							return fmt.Errorf("bad SOCKS4a marker")
						}
						host, err := r.ReadString(0)
						if err != nil || host != tc.host+"\x00" {
							return fmt.Errorf("bad SOCKS4a domain")
						}
					} else if !bytes.Equal(head[4:], []byte{203, 0, 113, 4}) {
						return fmt.Errorf("bad SOCKS4 IPv4")
					}
					if _, err := c.Write(append([]byte{0, 90, 0, 0, 0, 0, 0, 0}, []byte("ready")...)); err != nil {
						return err
					}
				case domain.ProxySOCKS5:
					var greeting [2]byte
					if _, err := io.ReadFull(r, greeting[:]); err != nil {
						return err
					}
					methods := make([]byte, greeting[1])
					if _, err := io.ReadFull(r, methods); err != nil {
						return err
					}
					method := byte(0)
					if tc.user != "" {
						method = 2
					}
					if _, err := c.Write([]byte{5, method}); err != nil {
						return err
					}
					if method == 2 {
						var auth [2]byte
						if _, err := io.ReadFull(r, auth[:]); err != nil {
							return err
						}
						user := make([]byte, auth[1])
						if _, err := io.ReadFull(r, user); err != nil {
							return err
						}
						n, err := r.ReadByte()
						if err != nil {
							return err
						}
						password := make([]byte, n)
						if _, err := io.ReadFull(r, password); err != nil {
							return err
						}
						if auth[0] != 1 || string(user) != tc.user || string(password) != tc.password {
							return fmt.Errorf("bad SOCKS5 credentials")
						}
						if _, err := c.Write([]byte{1, 0}); err != nil {
							return err
						}
					}
					var head [5]byte
					if _, err := io.ReadFull(r, head[:]); err != nil {
						return err
					}
					dst := make([]byte, int(head[4])+2)
					if _, err := io.ReadFull(r, dst); err != nil {
						return err
					}
					if !bytes.Equal(head[:4], []byte{5, 1, 0, 3}) || string(dst[:len(dst)-2]) != tc.host || binary.BigEndian.Uint16(dst[len(dst)-2:]) != 443 {
						return fmt.Errorf("bad SOCKS5 destination")
					}
					if _, err := c.Write(append([]byte{5, 0, 0, 1, 0, 0, 0, 0, 0, 0}, []byte("ready")...)); err != nil {
						return err
					}
				}
				data, err := io.ReadAll(r)
				if err != nil {
					return err
				}
				if string(data) != "ping" {
					return fmt.Errorf("bad tunnel payload %q", data)
				}
				_, err = c.Write([]byte("pong"))
				return err
			})
			p.Username = tc.user
			conn, err := dial(context.Background(), p, []byte(tc.password), tc.host, 443, config)
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			conn.SetDeadline(time.Now().Add(3 * time.Second))
			ready := make([]byte, 5)
			if _, err := io.ReadFull(conn, ready); err != nil || string(ready) != "ready" {
				t.Fatalf("greeting %q: %v", ready, err)
			}
			if _, err := io.WriteString(conn, "ping"); err != nil {
				t.Fatal(err)
			}
			if err := conn.(interface{ CloseWrite() error }).CloseWrite(); err != nil {
				t.Fatal(err)
			}
			pong, err := io.ReadAll(conn)
			if err != nil || string(pong) != "pong" {
				t.Fatalf("half-closed tunnel reply %q: %v", pong, err)
			}
			if err := <-done; err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestProxyRejections(t *testing.T) {
	for _, tc := range []struct {
		name        string
		protocol    domain.ProxyProtocol
		reply, want string
	}{
		{"auth", domain.ProxyHTTP, "HTTP/1.1 407 Proxy Authentication Required\r\n\r\n", "407"},
		{"blocked", domain.ProxyHTTP, "HTTP/1.1 403 Forbidden\r\n\r\n", "403"},
		{"malformed", domain.ProxyHTTP, "garbage\r\n\r\n", "malformed"},
		{"headers-limit", domain.ProxyHTTP, "HTTP/1.1 200 OK\r\nX-Huge: " + strings.Repeat("a", 65536) + "\r\n\r\n", "64 KiB"},
		{"socks4-denied", domain.ProxySOCKS4, string([]byte{0, 91, 0, 0, 0, 0, 0, 0}), "code 91"},
		{"socks4-malformed", domain.ProxySOCKS4, string([]byte{4, 90, 0, 0, 0, 0, 0, 0}), "malformed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, _, done := localProxy(t, tc.protocol, func(c net.Conn) error {
				r := bufio.NewReader(c)
				if tc.protocol == domain.ProxyHTTP {
					if _, err := http.ReadRequest(r); err != nil {
						return err
					}
				} else {
					if _, err := io.CopyN(io.Discard, r, 8); err != nil {
						return err
					}
					if _, err := r.ReadString(0); err != nil {
						return err
					}
				}
				_, err := io.WriteString(c, tc.reply)
				return err
			})
			c, err := Dial(context.Background(), p, nil, "203.0.113.4", 443)
			if c != nil {
				c.Close()
				t.Fatal("unexpected tunnel")
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want %q, got %v", tc.want, err)
			}
			<-done
		})
	}
}

func TestHTTPSCertificateVerification(t *testing.T) {
	for _, wrongHost := range []bool{false, true} {
		t.Run(strconv.FormatBool(wrongHost), func(t *testing.T) {
			p, config, done := localProxy(t, domain.ProxyHTTPS, func(c net.Conn) error { return c.(*tls.Conn).Handshake() })
			if wrongHost {
				config.ServerName = "wrong.invalid"
			} else {
				config = nil
			}
			c, err := dial(context.Background(), p, nil, "target.invalid", 443, config)
			if c != nil {
				c.Close()
			}
			if err == nil {
				t.Fatal("untrusted or mismatched certificate accepted")
			}
			<-done
		})
	}
}

func TestHandshakeCancellationAndTimeout(t *testing.T) {
	for _, protocol := range []domain.ProxyProtocol{domain.ProxyHTTP, domain.ProxyHTTPS, domain.ProxySOCKS4, domain.ProxySOCKS5} {
		for _, cancelEarly := range []bool{false, true} {
			t.Run(string(protocol)+"/cancel="+strconv.FormatBool(cancelEarly), func(t *testing.T) {
				accepted := make(chan struct{})
				p, config, done := localProxy(t, protocol, func(c net.Conn) error { close(accepted); _, err := io.Copy(io.Discard, c); return err })
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				p.TimeoutMS = 80
				if cancelEarly {
					p.TimeoutMS = 2000
					go func() { <-accepted; cancel() }()
				}
				start := time.Now()
				c, err := dial(ctx, p, nil, "target.invalid", 443, config)
				if c != nil {
					c.Close()
				}
				if err == nil || time.Since(start) > time.Second {
					t.Fatalf("handshake did not stop promptly: %v", err)
				}
				<-done
			})
		}
	}
}

func TestInvalidProfilesAndDestinations(t *testing.T) {
	for _, tc := range []struct {
		protocol domain.ProxyProtocol
		host     string
		port     int
		password string
	}{
		{"ftp", "target.invalid", 443, ""}, {domain.ProxyHTTP, "bad\r\nhost", 443, ""},
		{domain.ProxySOCKS5, "target.invalid", 0, ""}, {domain.ProxySOCKS4, "target.invalid", 443, "secret"},
	} {
		p := &domain.ProxyProfile{Protocol: tc.protocol, Host: "127.0.0.1", Port: 1}
		if c, err := Dial(context.Background(), p, []byte(tc.password), tc.host, tc.port); err == nil {
			c.Close()
			t.Fatal("invalid input accepted")
		}
	}
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()
	if err := socks4Connect(a, "", "2001:db8::1", 443); err == nil || !strings.Contains(err.Error(), "IPv6") {
		t.Fatalf("IPv6: %v", err)
	}
}

func TestHTTPInformationalResponseAndLongHeader(t *testing.T) {
	p, _, done := localProxy(t, domain.ProxyHTTP, func(c net.Conn) error {
		if _, err := http.ReadRequest(bufio.NewReader(c)); err != nil {
			return err
		}
		// The long header ends exactly at a reader fragment boundary.
		_, err := io.WriteString(c, "HTTP/1.1 100 Continue\r\n\r\nHTTP/1.1 200 OK\r\nX: "+strings.Repeat("a", 8189)+"\r\n\r\nready")
		return err
	})
	c, err := Dial(context.Background(), p, nil, "target.invalid", 443)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	data, err := io.ReadAll(c)
	if err != nil || string(data) != "ready" {
		t.Fatalf("data=%q, err=%v", data, err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestHandshakeDeadlineCleared(t *testing.T) {
	for _, protocol := range []domain.ProxyProtocol{domain.ProxyHTTP, domain.ProxyHTTPS, domain.ProxySOCKS4} {
		t.Run(string(protocol), func(t *testing.T) {
			release := make(chan struct{})
			defer close(release)
			p, config, done := localProxy(t, protocol, func(c net.Conn) error {
				r := bufio.NewReader(c)
				if protocol == domain.ProxySOCKS4 {
					if _, err := io.CopyN(io.Discard, r, 8); err != nil {
						return err
					}
					if _, err := r.ReadString(0); err != nil {
						return err
					}
					if _, err := c.Write([]byte{0, 90, 0, 0, 0, 0, 0, 0}); err != nil {
						return err
					}
				} else {
					if _, err := http.ReadRequest(r); err != nil {
						return err
					}
					if _, err := io.WriteString(c, "HTTP/1.1 200 OK\r\n\r\n"); err != nil {
						return err
					}
				}
				select {
				case <-time.After(150 * time.Millisecond):
				case <-release:
					return nil
				}
				_, err := io.WriteString(c, "still open")
				return err
			})
			p.TimeoutMS = 100
			c, err := dial(context.Background(), p, nil, "203.0.113.4", 443, config)
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			data, err := io.ReadAll(c)
			if err != nil || string(data) != "still open" {
				t.Fatalf("established tunnel expired: %q %v", data, err)
			}
			if err := <-done; err != nil {
				t.Fatal(err)
			}
		})
	}
}
