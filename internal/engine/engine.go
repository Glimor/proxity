//go:build windows

// Package engine is Proxity's traffic engine: it captures outbound TCP with
// WinDivert, attributes connections to processes, applies rules and relays
// proxied flows through the selected proxy via a local transparent listener (the
// "reflect" trick — see redirect.go).
package engine

import (
	"context"
	"log/slog"
	"net"
	"net/netip"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Glimor/proxity/internal/domain"
	"github.com/Glimor/proxity/internal/ports"
	"github.com/Glimor/proxity/internal/procmon"
	"github.com/Glimor/proxity/internal/windivert"
)

// ProxyPort is the local transparent listener's port. A redirected flow's
// natID rides in the reflected packet's source port (see redirect.go);
// listener replies are recognised by srcPort==ProxyPort.
const ProxyPort = 37890

type Deps struct {
	Stats   ports.StatsStore
	Blocked ports.BlockedLogStore
	Cipher  ports.SecretCipher
	Log     *slog.Logger
	// SelfCheck overrides driver preflight for hosts/tests with a custom check.
	SelfCheck func() error

	// OnCaptureDead is invoked exactly once per start/stop cycle when the
	// engine can no longer guarantee interception: the WinDivert capture
	// loop died, a proxy listener failed, or Stop was called. The kill
	// switch wires this to SetHealthy(false) so WFP permits are revoked.
	OnCaptureDead func()

	// DomainLookup, when non-nil and returning true for a destination IP,
	// makes the proxy dial use the domain name instead of the IP literal —
	// the proxy then resolves remotely and no plaintext DNS is needed
	// locally. Wired to the secure-DNS forwarder's domain cache.
	DomainLookup func(netip.Addr) (string, bool)
}

// config is swapped atomically on UpdateConfig; new connections use the new
// view, established flows keep their original decision.
type config struct {
	rules    []domain.Rule
	profiles map[string]domain.ProxyProfile
	settings domain.Settings
	proxyEPs map[string]struct{} // "ip:port" and literal "host:port"
}

func (c *config) isProxyEndpoint(ip netip.Addr, port uint16) bool {
	_, ok := c.proxyEPs[ip.String()+":"+strconv.Itoa(int(port))]
	return ok
}

type Engine struct {
	deps    Deps
	log     *slog.Logger
	selfPID int

	cfg   atomic.Pointer[config]
	flows *flowTable
	pm    *procmon.Monitor
	agg   *aggregator
	hc    *health

	h         *windivert.Handle
	listeners []net.Listener

	// captureAlive is true while the capture loop and listeners are
	// expected to be running; it gates the kill switch's dynamic permits.
	captureAlive atomic.Bool

	connsMu sync.Mutex
	conns   map[net.Conn]struct{}

	// blockedAt dedupes blocked-flow log entries per 4-tuple so SYN
	// retransmissions don't spam the log (see dropBlocked).
	blockedMu sync.Mutex
	blockedAt map[tupleKey]time.Time

	// warnedMissing dedupes the "rule references deleted proxy" warning.
	warnedMissingMu sync.Mutex
	warnedMissing   map[string]struct{}

	lifecycleMu sync.Mutex // serialize starts/stops, including failed starts
	mu          sync.Mutex
	started     bool
	cancel      context.CancelFunc
	wg          sync.WaitGroup
}

func New(d Deps) *Engine {
	if d.Log == nil {
		d.Log = slog.Default()
	}
	if d.SelfCheck == nil {
		d.SelfCheck = SelfCheck
	}
	e := &Engine{
		deps:    d,
		log:     d.Log,
		selfPID: os.Getpid(),
	}
	e.flows = newFlowTable()
	e.conns = make(map[net.Conn]struct{})
	e.blockedAt = make(map[tupleKey]time.Time)
	e.warnedMissing = make(map[string]struct{})
	e.agg = newAggregator(d.Stats, d.Blocked, d.Log)
	e.hc = newHealth(d.Log, e.password)
	e.pm = procmon.New(d.Log)
	e.UpdateConfig(nil, nil, domain.DefaultSettings())
	return e
}

// UpdateConfig atomically replaces rules/profiles/settings. Only new
// connections see the change (per product requirement).
func (e *Engine) UpdateConfig(profiles []domain.ProxyProfile, rules []domain.Rule, s domain.Settings) {
	c := &config{
		rules:    rules,
		profiles: make(map[string]domain.ProxyProfile),
		settings: s,
		proxyEPs: make(map[string]struct{}),
	}
	for _, p := range profiles {
		c.profiles[p.ID] = p
		if p.Host == "" || p.Port <= 0 {
			continue
		}
		port := strconv.Itoa(p.Port)
		c.proxyEPs[p.Host+":"+port] = struct{}{}
		if ip, err := netip.ParseAddr(p.Host); err == nil {
			c.proxyEPs[ip.Unmap().String()+":"+port] = struct{}{}
			continue
		}
		// Resolve hostname proxies so our own dial-outs to them bypass capture.
		resolveCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		ips, err := net.DefaultResolver.LookupIPAddr(resolveCtx, p.Host)
		cancel()
		if err == nil {
			for _, lip := range ips {
				if a, ok := netip.AddrFromSlice(lip.IP); ok {
					c.proxyEPs[a.Unmap().String()+":"+port] = struct{}{}
				}
			}
		}
	}
	e.cfg.Store(c)
	e.hc.reconcile(c.profiles)

	// Rules pointing at a now-deleted proxy keep established flows alive but
	// new dials fail — log that once per (rule, proxy) pair.
	for _, r := range rules {
		if r.Action != domain.RuleActionProxy || r.ProxyID == "" {
			continue
		}
		if _, ok := c.profiles[r.ProxyID]; ok {
			continue
		}
		k := r.ID + "|" + r.ProxyID
		e.warnedMissingMu.Lock()
		_, warned := e.warnedMissing[k]
		e.warnedMissing[k] = struct{}{}
		e.warnedMissingMu.Unlock()
		if !warned {
			e.log.Warn("rule references missing proxy profile; new connections will fail",
				slog.String("rule", r.Name), slog.String("proxy_id", r.ProxyID))
		}
	}
}

func (e *Engine) config() *config {
	c := e.cfg.Load()
	if c == nil {
		return &config{proxyEPs: map[string]struct{}{}}
	}
	return c
}

// Start opens the listener(s), procmon and the WinDivert capture handle and
// launches all goroutines. Requires administrator privileges.
func (e *Engine) Start(ctx context.Context) (startErr error) {
	e.lifecycleMu.Lock()
	defer e.lifecycleMu.Unlock()
	e.mu.Lock()
	if e.started {
		e.mu.Unlock()
		return nil
	}
	e.started = true
	e.mu.Unlock()
	defer func() {
		if startErr != nil {
			e.mu.Lock()
			e.started = false
			e.mu.Unlock()
		}
	}()

	if err := e.deps.SelfCheck(); err != nil {
		return err
	}
	lns, err := e.openListeners(ProxyPort)
	if err != nil {
		return err
	}
	e.mu.Lock()
	e.listeners = lns
	e.mu.Unlock()

	e.pm.OnClose = func(addr netip.Addr, port uint16) {
		e.flows.removeByLocal(addr.Unmap(), port)
	}
	if err := e.pm.Start(ctx); err != nil {
		e.log.Warn("procmon degraded", slog.String("err", err.Error()))
	}

	h, err := windivert.Open(captureFilter, windivert.LayerNetwork, 0, 0)
	if err != nil {
		e.closeListeners()
		e.pm.Close()
		return err
	}
	e.h = h
	h.SetParam(windivert.ParamQueueLength, 4096)
	h.SetParam(windivert.ParamQueueTime, 2000)
	h.SetParam(windivert.ParamQueueSize, 8<<20)

	e.captureAlive.Store(true)
	ctx, e.cancel = context.WithCancel(ctx)
	e.hc.start()
	e.hc.reconcile(e.config().profiles)
	e.wg.Add(3)
	go func() { defer e.wg.Done(); e.captureLoop(ctx) }()
	go func() { defer e.wg.Done(); e.agg.run(ctx) }()
	go func() { defer e.wg.Done(); e.sweepLoop(ctx) }()
	for _, ln := range lns {
		ln := ln
		e.wg.Add(1)
		go func() { defer e.wg.Done(); e.serveListener(ctx, ln) }()
	}
	e.log.Info("traffic engine started", slog.Int("proxy_port", ProxyPort))
	return nil
}

func (e *Engine) Stop() {
	e.lifecycleMu.Lock()
	defer e.lifecycleMu.Unlock()
	e.mu.Lock()
	if !e.started {
		e.mu.Unlock()
		return
	}
	e.started = false
	cancel := e.cancel
	e.mu.Unlock()

	e.captureDead() // revoke kill-switch permits before tearing down
	e.hc.stop()
	if cancel != nil {
		cancel()
	}
	if e.h != nil {
		e.h.Shutdown(windivert.ShutdownBoth)
		e.h.Close()
	}
	e.pm.Close()
	e.closeListeners()
	e.connsMu.Lock()
	for c := range e.conns {
		c.Close()
	}
	e.connsMu.Unlock()
	e.wg.Wait()
	e.log.Info("traffic engine stopped")
}

func (e *Engine) closeListeners() {
	e.mu.Lock()
	listeners := e.listeners
	e.listeners = nil
	e.mu.Unlock()
	for _, ln := range listeners {
		ln.Close()
	}
}

func (e *Engine) sweepLoop(ctx context.Context) {
	t := time.NewTicker(5 * time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			for _, f := range e.flows.sweep() {
				if f.connID != 0 {
					e.agg.close(f.connID, domain.ConnClosed)
				}
			}
		}
	}
}

// captureDead flips captureAlive off exactly once and runs the kill-switch
// fail-closed hook.
func (e *Engine) captureDead() {
	if e.captureAlive.Swap(false) && e.deps.OnCaptureDead != nil {
		e.deps.OnCaptureDead()
	}
}

// Running reports whether Start completed and Stop has not run.
func (e *Engine) Running() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.started
}

// CaptureAlive reports whether the capture loop is expected to be running.
// False means the kill switch should hold its dynamic permits revoked.
func (e *Engine) CaptureAlive() bool { return e.captureAlive.Load() }

// Listeners returns the number of bound transparent listeners (v4 + v6).
func (e *Engine) Listeners() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.listeners)
}

// RecheckProxies forces an immediate re-run of all proxy health checks
// (e.g. after resume from sleep or a network change).
func (e *Engine) RecheckProxies() {
	e.hc.recheck(e.config().profiles)
}

// Status returns the latest per-profile proxy statuses.
func (e *Engine) Status() []domain.ProxyStatus { return e.hc.snapshot() }

func (e *Engine) Snapshot() domain.LiveSnapshot { return e.agg.Snapshot() }

func (e *Engine) History() []domain.LiveSnapshot { return e.agg.History() }

func (e *Engine) ActiveConnections() []domain.Connection {
	return e.agg.ActiveConnections()
}

// password decrypts a profile's password via the configured cipher.
func (e *Engine) password(p *domain.ProxyProfile) []byte {
	if e.deps.Cipher == nil || len(p.PasswordCiphertext) == 0 {
		return nil
	}
	pw, err := e.deps.Cipher.Unprotect(p.PasswordCiphertext)
	if err != nil {
		e.log.Warn("profile password decrypt failed",
			slog.String("profile", p.ID), slog.String("err", err.Error()))
		return nil
	}
	return pw
}
