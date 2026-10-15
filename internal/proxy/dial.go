// Package proxy opens TCP tunnels through HTTP, HTTPS, SOCKS4(a), and SOCKS5 proxies.
package proxy

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"time"

	"github.com/Glimor/proxity/internal/domain"
	"github.com/Glimor/proxity/internal/socks5"
)

// Dial selects the profile protocol. An omitted protocol means SOCKS5.
// HTTPS verifies the proxy certificate against system roots and its host name.
func Dial(ctx context.Context, p *domain.ProxyProfile, password []byte, host string, port int) (net.Conn, error) {
	return dial(ctx, p, password, host, port, nil)
}

func dial(ctx context.Context, p *domain.ProxyProfile, password []byte, host string, port int, tlsConfig *tls.Config) (net.Conn, error) {
	protocol := p.Protocol.Normalized()
	if !protocol.Valid() {
		return nil, domain.NewError("proxy.bad_protocol", "Unsupported proxy protocol", string(protocol))
	}
	if port < 1 || port > 65535 || host == "" || strings.ContainsAny(host, "\x00\r\n\t /\\") {
		return nil, domain.NewError("proxy.bad_destination", "Invalid destination host or port", "")
	}
	if protocol == domain.ProxySOCKS5 {
		return socks5.Dial(ctx, p, password, host, port)
	}
	if protocol == domain.ProxySOCKS4 && (len(password) != 0 || strings.ContainsRune(p.Username, 0)) {
		return nil, domain.NewError("proxy.bad_credentials", "SOCKS4 accepts a user ID, but no password or NUL characters", "")
	}
	if (protocol == domain.ProxyHTTP || protocol == domain.ProxyHTTPS) && strings.Contains(p.Username, ":") {
		return nil, domain.NewError("proxy.bad_credentials", "HTTP proxy username cannot contain a colon", "")
	}
	timeout := time.Duration(p.TimeoutMS) * time.Millisecond
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	d := net.Dialer{Timeout: timeout}
	raw, err := d.DialContext(ctx, "tcp", net.JoinHostPort(p.Host, strconv.Itoa(p.Port)))
	if err != nil {
		return nil, domain.WrapError("proxy.dial_failed", "Cannot reach proxy", err)
	}
	deadline, _ := ctx.Deadline()
	raw.SetDeadline(deadline)
	aborted := make(chan struct{})
	stop := context.AfterFunc(ctx, func() {
		raw.SetDeadline(time.Now())
		close(aborted)
	})
	var conn net.Conn = raw
	if protocol == domain.ProxyHTTPS {
		config := &tls.Config{ServerName: p.Host, MinVersion: tls.VersionTLS12, NextProtos: []string{"http/1.1"}}
		if tlsConfig != nil {
			config = tlsConfig.Clone()
		}
		secure := tls.Client(raw, config)
		conn = secure
		err = secure.HandshakeContext(ctx)
	}
	if err == nil {
		if protocol == domain.ProxySOCKS4 {
			err = socks4Connect(conn, p.Username, host, port)
		} else {
			conn, err = httpConnect(conn, p.Username, password, host, port)
		}
	}
	if !stop() {
		<-aborted
	}
	if err == nil {
		err = ctx.Err()
	}
	if err != nil {
		raw.Close()
		return nil, domain.WrapError("proxy.connect_failed", "Proxy tunnel failed", err)
	}
	raw.SetDeadline(time.Time{})
	return conn, nil
}

type bufferedConn struct {
	net.Conn
	reader *bufio.Reader
}

func (c *bufferedConn) Read(b []byte) (int, error) { return c.reader.Read(b) }

func (c *bufferedConn) CloseWrite() error {
	if writer, ok := c.Conn.(interface{ CloseWrite() error }); ok {
		return writer.CloseWrite()
	}
	return c.Conn.Close()
}

func httpConnect(c net.Conn, user string, password []byte, host string, port int) (net.Conn, error) {
	authority := net.JoinHostPort(host, strconv.Itoa(port))
	request := "CONNECT " + authority + " HTTP/1.1\r\nHost: " + authority + "\r\n"
	if user != "" || len(password) != 0 {
		request += "Proxy-Authorization: Basic " + base64.StdEncoding.EncodeToString(append([]byte(user+":"), password...)) + "\r\n"
	}
	if _, err := io.WriteString(c, request+"\r\n"); err != nil {
		return nil, err
	}
	reader := bufio.NewReader(c)
	// Bound all response headers, including informational replies, without
	// placing a limit on the tunnel's subsequent data.
	total := 0
	for {
		var header []byte
		continued := false
		for {
			line, err := reader.ReadSlice('\n')
			header = append(header, line...)
			total += len(line)
			if total > 64*1024 {
				return nil, fmt.Errorf("HTTP proxy response headers exceed 64 KiB")
			}
			if err == bufio.ErrBufferFull {
				continued = true
				continue
			}
			if err != nil {
				return nil, err
			}
			if !continued && (bytes.Equal(line, []byte("\r\n")) || bytes.Equal(line, []byte("\n"))) {
				break
			}
			continued = false
		}
		response, err := http.ReadResponse(bufio.NewReader(bytes.NewReader(header)), &http.Request{Method: http.MethodConnect})
		if err != nil {
			return nil, fmt.Errorf("malformed HTTP proxy response: %w", err)
		}
		if response.StatusCode >= 100 && response.StatusCode < 200 && response.StatusCode != http.StatusSwitchingProtocols {
			continue
		}
		if response.StatusCode == http.StatusProxyAuthRequired {
			return nil, fmt.Errorf("HTTP proxy rejected credentials (407); Basic authentication is supported")
		}
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			return nil, fmt.Errorf("HTTP proxy CONNECT rejected (status %d)", response.StatusCode)
		}
		// A successful CONNECT response has no HTTP body; keep prefetched tunnel bytes.
		return &bufferedConn{Conn: c, reader: reader}, nil
	}
}

func socks4Connect(c net.Conn, user, host string, port int) error {
	request := []byte{4, 1, 0, 0}
	binary.BigEndian.PutUint16(request[2:], uint16(port))
	ip, err := netip.ParseAddr(host)
	if err == nil {
		ip = ip.Unmap()
		if !ip.Is4() {
			return fmt.Errorf("SOCKS4 does not support IPv6 destinations; use HTTP, HTTPS or SOCKS5")
		}
		address := ip.As4()
		request = append(request, address[:]...)
	} else {
		// SOCKS4a resolves the name at the proxy, without a local DNS lookup.
		request = append(request, 0, 0, 0, 1)
	}
	request = append(request, user...)
	request = append(request, 0)
	if err != nil {
		request = append(request, host...)
		request = append(request, 0)
	}
	if _, err := c.Write(request); err != nil {
		return err
	}
	var reply [8]byte
	if _, err := io.ReadFull(c, reply[:]); err != nil {
		return err
	}
	if reply[0] != 0 || reply[1] != 90 {
		return fmt.Errorf("SOCKS4 CONNECT rejected or malformed (version %d, code %d)", reply[0], reply[1])
	}
	return nil
}
