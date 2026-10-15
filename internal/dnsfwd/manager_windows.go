//go:build windows

package dnsfwd

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"sync"

	"github.com/Glimor/proxity/internal/domain"
	"github.com/Glimor/proxity/internal/ports"
	"github.com/Glimor/proxity/internal/proxy"
)

// backupKey is the settings-store key holding the pre-secure-DNS adapter
// configuration (survives service restarts so a crash can't strand DNS).
const backupKey = "dns_backup"

func RestoreFromStore(ctx context.Context, store ports.SettingsStore) []error {
	if store == nil {
		return nil
	}
	var backup DNSBackup
	found, err := store.Get(ctx, backupKey, &backup)
	if err != nil {
		return []error{fmt.Errorf("read DNS backup: %w", err)}
	}
	if !found {
		return nil
	}
	errs := RestoreDNS(&backup)
	if err := store.Delete(ctx, backupKey); err != nil {
		errs = append(errs, fmt.Errorf("delete DNS backup: %w", err))
	}
	return errs
}

// defaultUpstream is used when settings.DNSUpstream is empty.
const defaultUpstream = "1.1.1.1:53"

// listenAddrs are the forwarder's loopback endpoints.
var listenAddrs = []string{"127.0.0.1:53", "[::1]:53"}

// Manager drives the secure-DNS feature: the loopback forwarder plus the
// adapter DNS takeover. Sync is called on every config refresh; it is
// idempotent and converges actual state to settings.SecureDNS.
type Manager struct {
	log    *slog.Logger
	cipher ports.SecretCipher
	store  ports.SettingsStore // for the dns_backup key; may be nil

	mu      sync.Mutex
	fwd     *Forwarder
	adapter bool   // loopback DNS currently set
	cfgKey  string // (upstream|proxyID) the running forwarder uses
}

func NewManager(log *slog.Logger, cipher ports.SecretCipher, store ports.SettingsStore) *Manager {
	if log == nil {
		log = slog.Default()
	}
	return &Manager{log: log, cipher: cipher, store: store}
}

// LookupDomain resolves an IP to the name it was last seen answering for —
// only while the forwarder is running. Misses return false.
func (m *Manager) LookupDomain(ip netip.Addr) (string, bool) {
	m.mu.Lock()
	f := m.fwd
	m.mu.Unlock()
	if f == nil {
		return "", false
	}
	return f.LookupDomain(ip)
}

// Sync converges the DNS subsystem to the desired settings.
func (m *Manager) Sync(ctx context.Context, s domain.Settings, profiles []domain.ProxyProfile) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !s.SecureDNS {
		m.stopLocked(ctx)
		return
	}
	m.startLocked(ctx, s, profiles)
}

// Close stops everything and restores adapter DNS.
func (m *Manager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopLocked(context.Background())
}

func (m *Manager) startLocked(ctx context.Context, s domain.Settings, profiles []domain.ProxyProfile) {
	upstream := strings.TrimSpace(s.DNSUpstream)
	if upstream == "" {
		upstream = defaultUpstream
	}
	key := upstream + "|" + s.DNSViaProxyID
	if m.fwd == nil || m.cfgKey != key {
		if m.fwd != nil {
			m.fwd.Close()
			m.fwd = nil
		}
		fwd := NewForwarder(Config{
			Addrs: listenAddrs,
			Dial:  m.dialer(s, profiles, upstream),
		})
		fwd.log = m.log
		if err := fwd.Start(); err != nil {
			m.log.Error("secure-dns: forwarder start failed", slog.String("err", err.Error()))
			return
		}
		m.fwd = fwd
		m.cfgKey = key
	}
	if m.adapter {
		return
	}
	// Load (do not overwrite) any previous backup: if we crashed while
	// enabled, the current adapter state is already ours.
	var b DNSBackup
	found := false
	if m.store != nil {
		if ok, err := m.store.Get(ctx, backupKey, &b); err != nil {
			m.log.Warn("secure-dns: read dns_backup", slog.String("err", err.Error()))
		} else {
			found = ok
		}
	}
	if !found {
		nb, err := CaptureDNSBackup(func(name string, err error) {
			m.log.Warn("secure-dns: backup skipped interface",
				slog.String("iface", name), slog.String("err", err.Error()))
		})
		if err != nil {
			m.log.Warn("secure-dns: adapter backup failed", slog.String("err", err.Error()))
		} else {
			b = *nb
			if m.store != nil {
				if err := m.store.Set(ctx, backupKey, &b); err != nil {
					m.log.Warn("secure-dns: persist dns_backup", slog.String("err", err.Error()))
				}
			}
		}
	}
	for _, err := range SetLoopbackDNS() {
		m.log.Warn("secure-dns: set loopback DNS", slog.String("err", err.Error()))
	}
	m.adapter = true
}

func (m *Manager) stopLocked(ctx context.Context) {
	if m.fwd != nil {
		m.fwd.Close()
		m.fwd = nil
		m.cfgKey = ""
	}
	for _, err := range RestoreFromStore(ctx, m.store) {
		m.log.Warn("secure-dns: restore DNS", slog.String("err", err.Error()))
	}
	m.adapter = false
}

// dialer builds the upstream connector: TCP tunnel through the configured
// profile (or the first enabled one). nil → the forwarder SERVFAILs.
func (m *Manager) dialer(s domain.Settings, profiles []domain.ProxyProfile, upstream string) DialFunc {
	host, portStr, err := net.SplitHostPort(upstream)
	if err != nil {
		// Bare host given — append :53.
		host, portStr = upstream, "53"
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || port < 1 || port > 65535 {
		m.log.Warn("secure-dns: bad upstream", slog.String("upstream", upstream))
		return nil
	}
	prof := pickProfile(s.DNSViaProxyID, profiles)
	if prof == nil {
		m.log.Warn("secure-dns: no enabled proxy profile; queries will fail closed")
		return nil
	}
	p := *prof
	return func(ctx context.Context) (net.Conn, error) {
		return proxy.Dial(ctx, &p, m.password(&p), host, port)
	}
}

func pickProfile(id string, profiles []domain.ProxyProfile) *domain.ProxyProfile {
	if id != "" {
		for i := range profiles {
			if profiles[i].ID == id {
				if !profiles[i].Enabled {
					return nil
				}
				return &profiles[i]
			}
		}
		// Configured profile vanished: fall through to the first enabled
		// rather than hard-failing (it was probably deleted).
	}
	for i := range profiles {
		if profiles[i].Enabled {
			return &profiles[i]
		}
	}
	return nil
}

func (m *Manager) password(p *domain.ProxyProfile) []byte {
	if m.cipher == nil || len(p.PasswordCiphertext) == 0 {
		return nil
	}
	pw, err := m.cipher.Unprotect(p.PasswordCiphertext)
	if err != nil {
		m.log.Warn("secure-dns: password decrypt failed",
			slog.String("profile", p.ID), slog.String("err", err.Error()))
		return nil
	}
	return pw
}
