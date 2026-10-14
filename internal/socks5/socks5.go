// Package socks5 implements a minimal RFC 1928/1929 SOCKS5 client used to
// relay redirected TCP flows through a configured proxy profile.
package socks5

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/netip"
	"strconv"
	"time"

	"github.com/Glimor/proxity/internal/domain"
)

const defaultTimeout = 10 * time.Second

var repText = map[byte]string{
	0x01: "general failure",
	0x02: "connection not allowed by ruleset",
	0x03: "network unreachable",
	0x04: "host unreachable",
	0x05: "connection refused",
	0x06: "TTL expired",
	0x07: "command not supported",
	0x08: "address type not supported",
}

// Dial opens a TCP connection to the proxy described by profile and issues a
// CONNECT to dstHost:dstPort. dstHost may be an IP literal or a domain name
// (ATYP is chosen accordingly). password may be nil.
func Dial(ctx context.Context, profile *domain.ProxyProfile, password []byte, dstHost string, dstPort int) (net.Conn, error) {
	timeout := time.Duration(profile.TimeoutMS) * time.Millisecond
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	d := net.Dialer{Timeout: timeout}
	addr := net.JoinHostPort(profile.Host, strconv.Itoa(profile.Port))
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, domain.WrapError("socks5.dial_failed",
			fmt.Sprintf("Cannot reach proxy %s.", addr), err)
	}
	if err := handshake(conn, profile, password, dstHost, dstPort); err != nil {
		conn.Close()
		return nil, err
	}
	return conn, nil
}

func handshake(c net.Conn, profile *domain.ProxyProfile, password []byte, dstHost string, dstPort int) error {
	methods := []byte{0x00}
	if profile.Username != "" {
		methods = append(methods, 0x02)
	}
	greet := append([]byte{0x05, byte(len(methods))}, methods...)
	if _, err := c.Write(greet); err != nil {
		return err
	}
	var resp [2]byte
	if _, err := io.ReadFull(c, resp[:]); err != nil {
		return err
	}
	if resp[0] != 0x05 {
		return domain.NewError("socks5.bad_version", "Proxy is not a SOCKS5 server.", "")
	}
	switch resp[1] {
	case 0x00:
	case 0x02:
		if err := userPassAuth(c, profile.Username, password); err != nil {
			return err
		}
	case 0xff:
		return domain.NewError("socks5.auth_failed",
			"Proxy requires an authentication method we do not support.", "")
	default:
		return domain.NewError("socks5.auth_failed",
			fmt.Sprintf("Proxy selected unsupported auth method 0x%02x.", resp[1]), "")
	}
	return connect(c, dstHost, dstPort)
}

func userPassAuth(c net.Conn, user string, password []byte) error {
	if len(user) > 255 || len(password) > 255 {
		return domain.NewError("socks5.auth_failed", "Username or password too long.", "")
	}
	req := []byte{0x01, byte(len(user))}
	req = append(req, user...)
	req = append(req, byte(len(password)))
	req = append(req, password...)
	if _, err := c.Write(req); err != nil {
		return err
	}
	var resp [2]byte
	if _, err := io.ReadFull(c, resp[:]); err != nil {
		return err
	}
	if resp[1] != 0x00 {
		return domain.NewError("socks5.auth_failed",
			"Proxy rejected the username or password.", "")
	}
	return nil
}

func connect(c net.Conn, dstHost string, dstPort int) error {
	if dstPort < 1 || dstPort > 65535 {
		return domain.NewError("socks5.bad_port", "Invalid destination port.", "")
	}
	req := []byte{0x05, 0x01, 0x00}
	if ip, err := netip.ParseAddr(dstHost); err == nil {
		if ip.Is4() {
			b := ip.As4()
			req = append(req, 0x01, b[0], b[1], b[2], b[3])
		} else {
			b := ip.As16()
			req = append(req, 0x04)
			req = append(req, b[:]...)
		}
	} else {
		if len(dstHost) > 255 {
			return domain.NewError("socks5.bad_host", "Destination host name too long.", "")
		}
		req = append(req, 0x03, byte(len(dstHost)))
		req = append(req, dstHost...)
	}
	var port [2]byte
	binary.BigEndian.PutUint16(port[:], uint16(dstPort))
	req = append(req, port[:]...)
	if _, err := c.Write(req); err != nil {
		return err
	}
	var head [4]byte
	if _, err := io.ReadFull(c, head[:]); err != nil {
		return err
	}
	if head[0] != 0x05 {
		return domain.NewError("socks5.bad_version", "Malformed SOCKS5 reply.", "")
	}
	if head[1] != 0x00 {
		msg := repText[head[1]]
		if msg == "" {
			msg = fmt.Sprintf("reply code 0x%02x", head[1])
		}
		return domain.NewError("socks5.connect_failed",
			fmt.Sprintf("Proxy CONNECT failed: %s.", msg), "")
	}
	var skip int
	switch head[3] {
	case 0x01:
		skip = 4
	case 0x04:
		skip = 16
	case 0x03:
		var l [1]byte
		if _, err := io.ReadFull(c, l[:]); err != nil {
			return err
		}
		skip = int(l[0])
	default:
		return domain.NewError("socks5.bad_reply", "Malformed SOCKS5 bound address.", "")
	}
	if _, err := io.ReadFull(c, make([]byte, skip+2)); err != nil {
		return err
	}
	return nil
}
