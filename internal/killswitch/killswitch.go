// Package killswitch reconciles Proxity's fail-closed WFP policy: persistent
// BLOCK filters (they survive a service crash) plus higher-precedence PERMIT
// filters that live in a dynamic WFP session and vanish the instant the
// process dies or the engine reports unhealthy.
//
// Ordering note that drives the design: WFP evaluates ALE_AUTH_CONNECT when
// the connect() call is authorized — before the SYN ever reaches the network
// layer where WinDivert redirects it. Therefore a protected app's TCP must be
// PERMITted at WFP level while the engine is healthy; the permit only lets
// the SYN reach WinDivert, which redirects it into our transparent listener
// (if the SOCKS dial then fails, the connection fails — never direct). When
// the engine is unhealthy the dynamic permits are revoked and the persistent
// blocks take over: fail closed.
//
// WFP arbitration is "highest weight wins" within a layer, so the weight
// order is: self-permit > healthy TCP permit > local (loopback/LAN) permit >
// block-all. Do not reorder.
package killswitch

import (
	"fmt"
	"log/slog"
	"net/netip"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Filter weights. Higher wins inside the same ALE layer.
const (
	WeightBlock        = 1000 // persistent catch-all: lowest precedence
	WeightPermitLocal  = 2000 // loopback (+LAN when allowed) always reachable
	WeightBlockDNS     = 2500 // secure-DNS: port-53 egress denied
	WeightPermitDNS    = 2600 // secure-DNS: service itself may hit :53
	WeightPermitHealth = 3000 // dynamic-session TCP permit while engine runs
	WeightPermitSelf   = 4000 // service itself (global kill switch)
)

// AppPolicy describes one per-app filter bundle.
type AppPolicy struct {
	Path      string // canonical exe path
	PathFold  string // casefolded Path used in filter names
	BlockOnly bool   // Action=block rules: never get the healthy permit
}

// State is the desired kill-switch posture.
type State struct {
	Apps          []AppPolicy // deduped by PathFold; BlockOnly wins on dup
	Global        bool        // global kill switch: block everything
	LANAllow      bool        // include LAN ranges in the local permit set
	EngineHealthy bool        // engine capture + listener alive
	SelfPath      string      // this service's exe (global/DNS self-permit)
	// SecureDNS installs a remote-port-53 block for everything but the
	// service itself, pinning all name resolution to the local forwarder.
	SecureDNS bool
}

// Spec is one desired filter; the backend maps it onto WFP.
type Spec struct {
	Key        uuid.UUID
	Name       string // logical name, also the WFP display name
	IPv6       bool
	Persistent bool
	Weight     uint64
	Block      bool
	AppID      []byte // nil = all processes
	TCPOnly    bool
	RemotePort uint16 // 0 = any
	Remote     []netip.Prefix
	// NoLoopback excludes loopback packets (the secure-DNS block must not
	// trap the forwarder's own 127.0.0.1:53 listener).
	NoLoopback bool
}

// DropEvent is one kill-switch classify-drop reported by the WFP net-event
// stream (internal/wfp/netevents_windows.go).
type DropEvent struct {
	ExePath string
	Dst     netip.Addr
	DstPort uint16
	Proto   uint8
	TS      time.Time
}

// Backend abstracts the actual fwpuclnt calls so the diff logic is testable.
// Add must treat "already exists" as success; Delete "not found" as success.
type Backend interface {
	// EnsureBase installs the provider/sublayer once (idempotent).
	EnsureBase() error
	// ReopenSession drops the dynamic session (revoking every dynamic
	// object instantly) and opens a fresh empty one.
	ReopenSession() error
	// Txn runs fn inside a WFP transaction.
	Txn(fn func() error) error
	Add(s Spec) error
	Delete(key uuid.UUID) error
	// AppIDOf resolves an exe path to a WFP app-id blob.
	AppIDOf(path string) ([]byte, error)
}

// filterNamespace scopes the deterministic filter GUIDs.
var filterNamespace = uuid.MustParse("2e6b1f9c-8a4d-4f5e-b7c1-9d0a3e5f7b2c")

func keyFor(name string) uuid.UUID {
	return uuid.NewSHA1(filterNamespace, []byte(name))
}

// Policy tracks the desired state and the set of installed filters, and
// reconciles them transactionally.
type Policy struct {
	b   Backend
	log *slog.Logger

	mu        sync.Mutex
	state     State
	baseReady bool
	installed map[string]Spec // logical name -> spec
}

func New(b Backend, log *slog.Logger) *Policy {
	if log == nil {
		log = slog.Default()
	}
	return &Policy{b: b, log: log, installed: make(map[string]Spec)}
}

// Reconcile swaps the desired state and applies the diff inside a single
// transaction. Idempotent: a no-change call issues no backend writes.
func (p *Policy) Reconcile(s State) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.state = s
	return p.reconcileLocked()
}

// SetHealthy flips the healthy bit. Closing and reopening the dynamic
// session revokes all dynamic PERMIT filters instantly — the fail-closed
// path. Called with false when the engine dies and on shutdown.
func (p *Policy) SetHealthy(healthy bool) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.state.EngineHealthy == healthy {
		return nil
	}
	p.state.EngineHealthy = healthy
	if err := p.b.ReopenSession(); err != nil {
		// Still reconcile: worst case the stale session lingers, but on
		// unhealthy the new state adds no permits so the practical effect
		// is still fail-closed once the session actually dies.
		p.log.Error("killswitch: session reopen failed", slog.String("err", err.Error()))
	} else {
		// The reopened session revoked every dynamic object: purge them
		// from the installed set so the diff below re-adds them when the
		// new state wants permits again.
		for name, spec := range p.installed {
			if !spec.Persistent {
				delete(p.installed, name)
			}
		}
	}
	return p.reconcileLocked()
}

// Close releases the backend if it supports it. Persistent filters stay
// installed — that is the fail-closed property.
func (p *Policy) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if c, ok := p.b.(interface{ Close() }); ok {
		c.Close()
	}
}

// InstalledNames returns the sorted logical names of installed filters
// (diagnostics/tests).
func (p *Policy) InstalledNames() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]string, 0, len(p.installed))
	for n := range p.installed {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

func (p *Policy) reconcileLocked() error {
	desired, warns := computeDesired(p.state, p.b.AppIDOf)
	for _, w := range warns {
		p.log.Warn("killswitch: skipped filter", slog.String("err", w.Error()))
	}

	toAdd := make([]Spec, 0, len(desired))
	var toDelete []uuid.UUID
	for name, spec := range desired {
		old, ok := p.installed[name]
		if !ok {
			toAdd = append(toAdd, spec)
			continue
		}
		if !old.same(spec) {
			// Same name => same deterministic key; the old filter must be
			// deleted first or the add would hit "already exists".
			toDelete = append(toDelete, old.Key)
			toAdd = append(toAdd, spec)
		}
	}
	for name, spec := range p.installed {
		if _, ok := desired[name]; !ok {
			toDelete = append(toDelete, spec.Key)
		}
	}
	// Stable order keeps transactions deterministic.
	sort.Slice(toAdd, func(i, j int) bool { return toAdd[i].Name < toAdd[j].Name })
	sort.Slice(toDelete, func(i, j int) bool { return toDelete[i].String() < toDelete[j].String() })

	if !p.baseReady {
		if err := p.b.EnsureBase(); err != nil {
			return fmt.Errorf("ensure base objects: %w", err)
		}
		p.baseReady = true
	}
	if len(toAdd) == 0 && len(toDelete) == 0 {
		return nil
	}

	err := p.b.Txn(func() error {
		for _, k := range toDelete {
			if err := p.b.Delete(k); err != nil {
				return fmt.Errorf("delete filter %s: %w", k, err)
			}
		}
		for _, s := range toAdd {
			if err := p.b.Add(s); err != nil {
				return fmt.Errorf("add filter %q: %w", s.Name, err)
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	// Installed state now equals exactly the desired set.
	p.installed = desired
	p.log.Debug("killswitch reconciled",
		slog.Int("added", len(toAdd)), slog.Int("deleted", len(toDelete)),
		slog.Int("installed", len(p.installed)))
	return nil
}

// same reports whether two specs install the identical filter.
func (s Spec) same(o Spec) bool {
	if s.IPv6 != o.IPv6 || s.Persistent != o.Persistent || s.Weight != o.Weight ||
		s.Block != o.Block || s.TCPOnly != o.TCPOnly || s.RemotePort != o.RemotePort ||
		s.NoLoopback != o.NoLoopback {
		return false
	}
	if string(s.AppID) != string(o.AppID) || len(s.Remote) != len(o.Remote) {
		return false
	}
	for i := range s.Remote {
		if s.Remote[i] != o.Remote[i] {
			return false
		}
	}
	return true
}

// computeDesired derives the full filter set for a state. appIDOf resolves
// exe paths; failures are returned as warnings and the affected app is
// skipped (a missing exe cannot be running, so nothing needs blocking).
func computeDesired(s State, appIDOf func(string) ([]byte, error)) (map[string]Spec, []error) {
	out := make(map[string]Spec)
	var warns []error
	local := localPrefixes(s.LANAllow)

	add := func(name string, ipv6, persistent bool, weight uint64, block bool, appID []byte, tcpOnly bool, remote []netip.Prefix, remotePort uint16) {
		full := name
		if ipv6 {
			full += "|v6"
		} else {
			full += "|v4"
		}
		out[full] = Spec{
			Key:        keyFor(full),
			Name:       full,
			IPv6:       ipv6,
			Persistent: persistent,
			Weight:     weight,
			Block:      block,
			AppID:      appID,
			TCPOnly:    tcpOnly,
			Remote:     remote,
			RemotePort: remotePort,
		}
	}
	pair := func(name string, persistent bool, weight uint64, block bool, appID []byte, tcpOnly bool, remote []netip.Prefix, remotePort uint16) {
		add(name, false, persistent, weight, block, appID, tcpOnly, remote, remotePort)
		add(name, true, persistent, weight, block, appID, tcpOnly, remote, remotePort)
	}

	seen := make(map[string]bool)
	for _, app := range s.Apps {
		fold := app.PathFold
		if fold == "" {
			fold = strings.ToUpper(app.Path)
		}
		if seen[fold] {
			continue
		}
		seen[fold] = true
		appID, err := appIDOf(app.Path)
		if err != nil {
			warns = append(warns, fmt.Errorf("app %q: %w", app.Path, err))
			continue
		}
		base := "app|" + fold + "|"
		pair(base+"block", true, WeightBlock, true, appID, false, nil, 0)
		pair(base+"local", true, WeightPermitLocal, false, appID, false, local, 0)
		if !app.BlockOnly && s.EngineHealthy {
			pair(base+"healthy", false, WeightPermitHealth, false, appID, true, nil, 0)
		}
	}

	// Self app-id is shared between the global self-permit and the
	// secure-DNS port-53 permit; resolve once when either needs it.
	var selfID []byte
	if (s.Global || s.SecureDNS) && s.SelfPath != "" {
		var err error
		selfID, err = appIDOf(s.SelfPath)
		if err != nil {
			warns = append(warns, fmt.Errorf("self %q: %w", s.SelfPath, err))
		}
	}

	if s.Global {
		// Self permit is all-protocols and persistent: the service must be
		// able to reach its proxies (health checks) even when the engine is
		// down — otherwise it could never recover.
		if selfID != nil {
			pair("global|self", true, WeightPermitSelf, false, selfID, false, nil, 0)
		}
		pair("global|local", true, WeightPermitLocal, false, nil, false, local, 0)
		pair("global|block", true, WeightBlock, true, nil, false, nil, 0)
		if s.EngineHealthy {
			pair("global|healthy", false, WeightPermitHealth, false, nil, true, nil, 0)
		}
	}

	if s.SecureDNS {
		// Trap all plaintext DNS egress on remote port 53 (UDP and TCP:
		// no protocol condition covers both). The block is persistent like
		// the kill-switch blocks — failing closed on DNS mirrors the rest
		// of the model. Loopback is excluded so the forwarder's own
		// 127.0.0.1:53 listener stays reachable.
		pair("dns|block", true, WeightBlockDNS, true, nil, false, nil, 53)
		for _, sfx := range []string{"|v4", "|v6"} {
			sp := out["dns|block"+sfx]
			sp.NoLoopback = true
			out["dns|block"+sfx] = sp
		}
		if selfID != nil {
			// The forwarder itself may still emit queries (e.g. bootstrap
			// resolution of a proxy hostname).
			pair("dns|self", true, WeightPermitDNS, false, selfID, false, nil, 53)
		}
	}
	return out, warns
}

// localPrefixes returns the always-allowed remote set: loopback, plus LAN,
// link-local, multicast and broadcast ranges when LAN access is enabled.
// The broadcast and multicast entries keep DHCP/NDP alive while the global
// kill switch is on.
func localPrefixes(lan bool) []netip.Prefix {
	out := []netip.Prefix{
		netip.MustParsePrefix("127.0.0.0/8"),
		netip.MustParsePrefix("::1/128"),
	}
	if !lan {
		return out
	}
	return append(out,
		netip.MustParsePrefix("10.0.0.0/8"),
		netip.MustParsePrefix("172.16.0.0/12"),
		netip.MustParsePrefix("192.168.0.0/16"),
		netip.MustParsePrefix("169.254.0.0/16"),
		netip.MustParsePrefix("224.0.0.0/4"),
		netip.MustParsePrefix("255.255.255.255/32"),
		netip.MustParsePrefix("fe80::/10"),
		netip.MustParsePrefix("fc00::/7"),
		netip.MustParsePrefix("ff00::/8"),
	)
}
