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
	"github.com/Glimor/proxity/internal/socks5"
)

// flowForRemote maps an accepted connection back to its flow: the peer port
// carries the natID and the peer address must equal the flow's original
// destination (the redirect rewrites src=D:natID — see redirect.go).
func (e *Engine) flowForRemote(ra *net.TCPAddr) *flow {
	if ra == nil {
		return nil
	}
	f := e.flows.getByID(uint32(ra.Port))
	if f == nil {
		return nil
	}
	a, err := netip.ParseAddr(ra.IP.String())
	if err != nil || a.Unmap() != f.origDst {
		return nil
	}
	return f
}

func (e *Engine) serveListener(ctx context.Context, ln net.Listener) {
	for {
		c, err := ln.Accept()
		if err != nil {
			if ctx.Err() == nil {
				// The transparent listener died while the engine is up:
				// redirected connections can no longer be served, so the
				// kill switch must fail closed.
				e.log.Error("proxy listener accept failed",
					slog.String("err", err.Error()))
				e.captureDead()
			}
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

	ra, _ := c.RemoteAddr().(*net.TCPAddr)
	f := e.flowForRemote(ra)
	if f == nil {
		remote := "<unknown>"
		if r := c.RemoteAddr(); r != nil {
			remote = r.String()
		}
		e.log.Debug("accepted conn with unknown nat id", slog.String("remote", remote))
		c.Close()
		return
	}
	cfg := e.config()
	prof, ok := cfg.profiles[f.proxyID]
	if !ok || !prof.Enabled {
		e.agg.close(f.connID, domain.ConnFailed)
		e.flows.remove(f)
		c.Close()
		return
	}
	timeout := time.Duration(prof.TimeoutMS) * time.Millisecond
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	dctx, cancel := context.WithTimeout(ctx, timeout)
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

// openListeners binds the transparent listener on both stacks.
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
	return out, nil
}
