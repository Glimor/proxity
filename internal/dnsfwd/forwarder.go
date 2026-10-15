package dnsfwd

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/netip"
	"sync"
	"time"
)

// upstreamTimeout bounds the whole proxied DNS-over-TCP exchange.
const upstreamTimeout = 10 * time.Second

// maxUDPQueries caps in-flight UDP relays so a flood can't spawn unbounded
// goroutines; extra queries are simply dropped (the client will retry).
const maxUDPQueries = 128

// DialFunc opens a stream to the upstream resolver (typically a proxy
// CONNECT already bound to the upstream address by the caller).
type DialFunc func(ctx context.Context) (net.Conn, error)

// Config configures a Forwarder.
type Config struct {
	// Addrs are UDP/TCP listen addresses; default is 127.0.0.1:53 only.
	Addrs []string
	// Dial connects to the upstream resolver. Nil → every query SERVFAILs.
	Dial DialFunc
	// Log, when set, receives error/debug output.
	Log *slog.Logger
}

// Forwarder serves DNS on loopback addresses, relaying each query as
// DNS-over-TCP through Dial. Responses' A/AAAA records feed the domain
// cache so the engine can reconnect flows by name.
type Forwarder struct {
	cfg   Config
	log   *slog.Logger
	cache *domainCache

	mu       sync.Mutex
	udp      []net.PacketConn
	tcp      []net.Listener
	done     chan struct{}
	wg       sync.WaitGroup
	inFlight chan struct{}
	running  bool
}

func NewForwarder(cfg Config) *Forwarder {
	if cfg.Log == nil {
		cfg.Log = slog.Default()
	}
	addrs := cfg.Addrs
	if len(addrs) == 0 {
		addrs = []string{"127.0.0.1:53"}
	}
	cfg.Addrs = addrs
	return &Forwarder{cfg: cfg, log: cfg.Log, cache: newDomainCache()}
}

// LookupDomain exposes the domain cache (IP → queried name).
func (f *Forwarder) LookupDomain(ip netip.Addr) (string, bool) {
	return f.cache.Lookup(ip)
}

// Running reports whether listeners are up.
func (f *Forwarder) Running() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.running
}

// Addrs returns the bound UDP listen addresses (in config order).
func (f *Forwarder) Addrs() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, 0, len(f.udp))
	for _, pc := range f.udp {
		out = append(out, pc.LocalAddr().String())
	}
	return out
}

// Start binds all configured addresses (UDP and TCP each) and starts
// serving. If any bind fails everything is torn down and an error returned.
func (f *Forwarder) Start() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.running {
		return nil
	}
	f.done = make(chan struct{})
	f.inFlight = make(chan struct{}, maxUDPQueries)
	var bound bool
	for _, a := range f.cfg.Addrs {
		pc, err := net.ListenPacket("udp", a)
		if err != nil {
			f.closeLocked()
			return err
		}
		f.udp = append(f.udp, pc)
		ln, err := net.Listen("tcp", a)
		if err != nil {
			f.closeLocked()
			return err
		}
		f.tcp = append(f.tcp, ln)
		bound = true
	}
	if !bound {
		return errors.New("dnsfwd: no listen address")
	}
	f.running = true
	for _, pc := range f.udp {
		pc := pc
		f.wg.Add(1)
		go func() { defer f.wg.Done(); f.serveUDP(pc) }()
	}
	for _, ln := range f.tcp {
		ln := ln
		f.wg.Add(1)
		go func() { defer f.wg.Done(); f.serveTCP(ln) }()
	}
	f.log.Info("dns forwarder listening", slog.Any("addrs", f.cfg.Addrs))
	return nil
}

// Close stops the forwarder and waits for handlers to drain.
func (f *Forwarder) Close() {
	f.mu.Lock()
	if !f.running {
		f.mu.Unlock()
		return
	}
	f.running = false
	f.closeLocked()
	f.mu.Unlock()
	f.wg.Wait()
}

func (f *Forwarder) closeLocked() {
	if f.done != nil {
		close(f.done)
	}
	for _, pc := range f.udp {
		pc.Close()
	}
	for _, ln := range f.tcp {
		ln.Close()
	}
	f.udp, f.tcp = nil, nil
}

// ---------------------------------------------------------------------------
// UDP
// ---------------------------------------------------------------------------

func (f *Forwarder) serveUDP(pc net.PacketConn) {
	buf := make([]byte, 64*1024)
	for {
		n, src, err := pc.ReadFrom(buf)
		if err != nil {
			return
		}
		q := append([]byte(nil), buf[:n]...)
		select {
		case f.inFlight <- struct{}{}:
		default:
			continue // drop; client retries
		}
		f.wg.Add(1)
		go func() {
			defer f.wg.Done()
			defer func() { <-f.inFlight }()
			resp := f.resolve(q)
			if resp == nil {
				return
			}
			pc.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if _, err := pc.WriteTo(resp, src); err != nil {
				f.log.Debug("dnsfwd udp write", slog.String("err", err.Error()))
			}
		}()
	}
}

// ---------------------------------------------------------------------------
// TCP
// ---------------------------------------------------------------------------

func (f *Forwarder) serveTCP(ln net.Listener) {
	for {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		f.wg.Add(1)
		go func() {
			defer f.wg.Done()
			defer c.Close()
			f.serveConn(c)
		}()
	}
}

func (f *Forwarder) serveConn(c net.Conn) {
	for {
		c.SetReadDeadline(time.Now().Add(30 * time.Second))
		var lb [2]byte
		if _, err := io.ReadFull(c, lb[:]); err != nil {
			return
		}
		l := int(binary.BigEndian.Uint16(lb[:]))
		if l == 0 {
			return
		}
		q := make([]byte, l)
		if _, err := io.ReadFull(c, q); err != nil {
			return
		}
		resp := f.resolve(q)
		if resp == nil {
			return
		}
		c.SetWriteDeadline(time.Now().Add(10 * time.Second))
		var ob [2]byte
		binary.BigEndian.PutUint16(ob[:], uint16(len(resp)))
		if _, err := c.Write(append(ob[:], resp...)); err != nil {
			return
		}
	}
}

// ---------------------------------------------------------------------------
// upstream relay
// ---------------------------------------------------------------------------

// resolve relays one wire-format query over TCP through the dialer and
// returns the wire-format response (or a SERVFAIL fallback). A nil Dial
// fails closed with SERVFAIL — no plaintext DNS ever leaves the machine.
func (f *Forwarder) resolve(query []byte) []byte {
	if !IsQuery(query) {
		return BuildServFail(query)
	}
	if f.cfg.Dial == nil {
		return BuildServFail(query)
	}
	ctx, cancel := context.WithTimeout(context.Background(), upstreamTimeout)
	defer cancel()
	up, err := f.cfg.Dial(ctx)
	if err != nil {
		f.log.Debug("dnsfwd upstream dial", slog.String("err", err.Error()))
		return BuildServFail(query)
	}
	defer up.Close()
	up.SetDeadline(time.Now().Add(upstreamTimeout))
	var lb [2]byte
	binary.BigEndian.PutUint16(lb[:], uint16(len(query)))
	if _, err := up.Write(append(lb[:], query...)); err != nil {
		return BuildServFail(query)
	}
	if _, err := io.ReadFull(up, lb[:]); err != nil {
		return BuildServFail(query)
	}
	n := int(binary.BigEndian.Uint16(lb[:]))
	if n <= 0 || n > 65535 {
		return BuildServFail(query)
	}
	resp := make([]byte, n)
	if _, err := io.ReadFull(up, resp); err != nil {
		return BuildServFail(query)
	}
	if name, answers, ok := ParseAnswers(resp); ok {
		for _, a := range answers {
			f.cache.put(a.IP, name, time.Duration(a.TTL)*time.Second)
		}
	}
	return resp
}
