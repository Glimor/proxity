//go:build windows

package engine

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/netip"
	"strconv"
	"sync"
	"time"

	"github.com/Glimor/proxity/internal/domain"
	"github.com/Glimor/proxity/internal/netpkt"
	"github.com/Glimor/proxity/internal/socks5"
)

// decodeNAT maps an accepted connection's local address back to a natID.
func (e *Engine) decodeNAT(la *net.TCPAddr) (uint32, bool) {
	a, err := netip.ParseAddr(la.IP.String())
	if err != nil {
		return 0, false
	}
	if e.v6Pool && a == netip.IPv6Loopback() &&
		la.Port >= int(poolBasePort) && la.Port < int(poolBasePort)+e.poolSz {
		return uint32(la.Port-int(poolBasePort)) + 1, true
	}
	if la.Port != ProxyPort {
		return 0, false
	}
	if a.Is4() {
		return netpkt.DecodeNATIPv4(a)
	}
	return netpkt.DecodeNATIPv6(a)
}

func (e *Engine) serveListener(ctx context.Context, ln net.Listener) {
	for {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		go e.handleConn(ctx, c)
	}
}

// handleConn resolves the flow behind an accepted connection, dials the
// original destination through its proxy and relays both ways.
func (e *Engine) handleConn(ctx context.Context, c net.Conn) {
	e.connsMu.Lock()
	e.conns[c] = struct{}{}
	e.connsMu.Unlock()
	defer func() {
		e.connsMu.Lock()
		delete(e.conns, c)
		e.connsMu.Unlock()
	}()

	la, _ := c.LocalAddr().(*net.TCPAddr)
	id, ok := e.decodeNAT(la)
	var f *flow
	if ok {
		f = e.flows.getByID(id)
	}
	if f == nil {
		e.log.Debug("accepted conn with unknown nat id", slog.String("local", la.String()))
		c.Close()
		return
	}
	cfg := e.config()
	prof, ok := cfg.profiles[f.proxyID]
	if !ok {
		e.agg.close(f.connID, domain.ConnFailed)
		e.flows.remove(f)
		c.Close()
		return
	}
	dctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	upstream, err := socks5.Dial(dctx, &prof, e.password(&prof),
		f.origDst.String(), int(f.origDstPort))
	cancel()
	if err != nil {
		e.agg.close(f.connID, domain.ConnFailed)
		e.flows.remove(f)
		c.Close()
		return
	}
	e.relay(f, c, upstream)
}

// relay copies both directions until either side closes, counting bytes into
// stats per chunk so bps reflects live traffic. Then it reaps the flow.
func (e *Engine) relay(f *flow, a, b net.Conn) {
	var wg sync.WaitGroup
	wg.Add(2)
	copyDir := func(dst, src net.Conn, up bool) {
		defer wg.Done()
		w := counterFunc(func(p []byte) (int, error) {
			if up {
				e.agg.add(f.connID, int64(len(p)), 0)
			} else {
				e.agg.add(f.connID, 0, int64(len(p)))
			}
			return len(p), nil
		})
		io.Copy(dst, io.TeeReader(src, w))
		closeWrite(dst)
		closeRead(src)
	}
	// up = app -> proxy, down = proxy -> app
	go copyDir(b, a, true)
	go copyDir(a, b, false)
	wg.Wait()
	a.Close()
	b.Close()
	e.flows.remove(f)
	e.agg.close(f.connID, domain.ConnClosed)
}

type counterFunc func([]byte) (int, error)

func (f counterFunc) Write(p []byte) (int, error) { return f(p) }

type closeWriter interface{ CloseWrite() error }
type closeReader interface{ CloseRead() error }

func closeWrite(c net.Conn) {
	if w, ok := c.(closeWriter); ok {
		w.CloseWrite()
		return
	}
	c.Close()
}

func closeRead(c net.Conn) {
	if r, ok := c.(closeReader); ok {
		r.CloseRead()
	}
}

// openListeners binds the transparent listener on both stacks (and the v6
// port pool when enabled).
func (e *Engine) openListeners() ([]net.Listener, error) {
	var out []net.Listener
	l4, err := net.Listen("tcp4", net.JoinHostPort("0.0.0.0", strconv.Itoa(ProxyPort)))
	if err != nil {
		return nil, domain.WrapError("engine.listen_failed",
			"Cannot open the transparent proxy listener.", err)
	}
	out = append(out, l4)
	if l6, err := net.Listen("tcp6", net.JoinHostPort("::", strconv.Itoa(ProxyPort))); err == nil {
		out = append(out, l6)
	} else {
		e.log.Warn("no IPv6 listener", slog.String("err", err.Error()))
	}
	if e.v6Pool {
		for i := 0; i < e.poolSz; i++ {
			l, err := net.Listen("tcp6",
				net.JoinHostPort("::1", strconv.Itoa(int(poolBasePort)+i)))
			if err != nil {
				continue
			}
			out = append(out, l)
		}
	}
	return out, nil
}
