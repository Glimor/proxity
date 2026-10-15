//go:build windows

package engine

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Glimor/proxity/internal/domain"
	"github.com/Glimor/proxity/internal/proxy"
)

const (
	testConnectHost = "www.gstatic.com"
	testConnectPort = 443
	testIPURL       = "api.ipify.org"
)

// health tracks per-profile connectivity: it continuously performs a proxy
// CONNECT through each enabled profile and publishes ProxyStatus.
type health struct {
	log *slog.Logger
	pw  func(*domain.ProxyProfile) []byte

	mu       sync.Mutex
	status   map[string]domain.ProxyStatus
	cancel   map[string]context.CancelFunc
	profiles map[string]domain.ProxyProfile
	running  bool
	wg       sync.WaitGroup
}

func newHealth(log *slog.Logger, pw func(*domain.ProxyProfile) []byte) *health {
	return &health{
		log:      log,
		pw:       pw,
		status:   make(map[string]domain.ProxyStatus),
		cancel:   make(map[string]context.CancelFunc),
		profiles: make(map[string]domain.ProxyProfile),
	}
}

func (h *health) start() {
	h.mu.Lock()
	h.running = true
	h.mu.Unlock()
}

func (h *health) stop() {
	h.mu.Lock()
	for id, c := range h.cancel {
		c()
		delete(h.cancel, id)
		delete(h.profiles, id)
	}
	h.running = false
	h.status = make(map[string]domain.ProxyStatus)
	h.mu.Unlock()
	h.wg.Wait()
}

// recheck restarts all check loops so they probe immediately instead of
// waiting out their backoff (resume from sleep / network change).
func (h *health) recheck(profiles map[string]domain.ProxyProfile) {
	h.mu.Lock()
	for id, c := range h.cancel {
		c()
		delete(h.cancel, id)
		delete(h.profiles, id)
	}
	h.mu.Unlock()
	h.reconcile(profiles)
}

// reconcile starts/stops per-profile check loops to match the enabled set.
func (h *health) reconcile(profiles map[string]domain.ProxyProfile) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if !h.running {
		return
	}
	for id, c := range h.cancel {
		p, ok := profiles[id]
		if !ok || !p.Enabled || !sameCheckProfile(h.profiles[id], p) {
			c()
			delete(h.cancel, id)
			delete(h.profiles, id)
			delete(h.status, id)
		}
	}
	for id, p := range profiles {
		if !p.Enabled {
			continue
		}
		if _, ok := h.cancel[id]; ok {
			continue
		}
		ctx, cancel := context.WithCancel(context.Background())
		h.cancel[id] = cancel
		h.profiles[id] = p
		pp := p
		h.wg.Add(1)
		go func() {
			defer h.wg.Done()
			h.loop(ctx, &pp)
		}()
	}
}

func sameCheckProfile(a, b domain.ProxyProfile) bool {
	return a.Protocol.Normalized() == b.Protocol.Normalized() && a.Host == b.Host && a.Port == b.Port && a.Username == b.Username &&
		bytes.Equal(a.PasswordCiphertext, b.PasswordCiphertext) &&
		a.TimeoutMS == b.TimeoutMS && a.RetryIntervalMS == b.RetryIntervalMS && a.MaxRetries == b.MaxRetries
}

func (h *health) healthy(profileID string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	s, ok := h.status[profileID]
	return ok && s.State == domain.ProxyConnected
}

func (h *health) snapshot() []domain.ProxyStatus {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]domain.ProxyStatus, 0, len(h.status))
	for _, s := range h.status {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ProfileID < out[j].ProfileID })
	return out
}

func (h *health) set(ctx context.Context, s domain.ProxyStatus) {
	s.Since = time.Now()
	h.mu.Lock()
	defer h.mu.Unlock()
	// A replaced/cancelled probe must not overwrite its successor's status.
	if ctx.Err() != nil {
		return
	}
	h.status[s.ProfileID] = s
}

func (h *health) loop(ctx context.Context, p *domain.ProxyProfile) {
	failures := 0
	maxRetries := p.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 3
	}
	base := time.Duration(p.RetryIntervalMS) * time.Millisecond
	if base <= 0 {
		base = 5 * time.Second
	}
	for {
		latency, ip, err := h.check(ctx, p)
		if ctx.Err() != nil {
			return
		}
		if err == nil {
			failures = 0
			h.set(ctx, domain.ProxyStatus{ProfileID: p.ID, State: domain.ProxyConnected,
				LatencyMS: latency, PublicIP: ip})
			if !sleep(ctx, base) {
				return
			}
			continue
		}
		failures++
		st := domain.ProxyConnecting
		if failures >= maxRetries {
			st = domain.ProxyError
		}
		h.set(ctx, domain.ProxyStatus{ProfileID: p.ID, State: st, LastError: err.Error()})
		delay := base << min(failures-1, 4)
		if !sleep(ctx, delay) {
			return
		}
	}
}

func (h *health) check(ctx context.Context, p *domain.ProxyProfile) (int64, string, error) {
	return checkProxy(ctx, p, h.pw(p))
}

// TestProxy performs a one-shot profile check for the GUI "test" button:
// a timed proxy CONNECT to the test host plus a best-effort public-IP
// fetch. Shared by the health loop and the app service.
func TestProxy(ctx context.Context, p *domain.ProxyProfile, password []byte) (latencyMS int64, publicIP string, err error) {
	return checkProxy(ctx, p, password)
}

// checkProxy performs a timed CONNECT to the test host, then a best-effort
// public IP fetch through the tunnel.
func checkProxy(ctx context.Context, p *domain.ProxyProfile, pw []byte) (int64, string, error) {
	start := time.Now()
	c, err := proxy.Dial(ctx, p, pw, testConnectHost, testConnectPort)
	if err != nil {
		return 0, "", err
	}
	latency := time.Since(start).Milliseconds()
	c.Close()
	ip := publicIP(ctx, p, pw)
	return latency, ip, nil
}

func publicIP(ctx context.Context, p *domain.ProxyProfile, pw []byte) string {
	cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	c, err := proxy.Dial(cctx, p, pw, testIPURL, 80)
	if err != nil {
		return ""
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(5 * time.Second))
	fmt.Fprintf(c, "GET / HTTP/1.1\r\nHost: %s\r\nConnection: close\r\n\r\n", testIPURL)
	var b strings.Builder
	if _, err := io.CopyN(&b, c, 4096); err != nil && b.Len() == 0 {
		return ""
	}
	body := b.String()
	if i := strings.Index(body, "\r\n\r\n"); i >= 0 {
		body = body[i+4:]
	}
	ip := strings.TrimSpace(body)
	if net.ParseIP(ip) == nil {
		return ""
	}
	return ip
}

func sleep(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}
