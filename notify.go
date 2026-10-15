//go:build windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/gen2brain/beeep"
)

func init() {
	beeep.AppName = appName
}

// notifier gates toast notifications by the effective settings and rate-limits
// them so a burst of blocked connections doesn't spam the desktop.
type notifier struct {
	mu          sync.Mutex
	enabled     bool
	proxyEvts   bool
	blockedEvts bool
	lastToast   map[string]time.Time
	iconPath    string
}

func (n *notifier) initialize() {
	base, err := os.UserCacheDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Proxity notification cache:", err)
		return
	}
	icon, err := writeNotificationLogo(filepath.Join(base, appName, "branding"))
	if err != nil {
		fmt.Fprintln(os.Stderr, "Proxity notification logo:", err)
		return
	}
	n.iconPath = icon
	if err := registerNotificationBranding(icon); err != nil {
		fmt.Fprintln(os.Stderr, "Proxity notification identity:", err)
	}
}

func newNotifier() notifier {
	return notifier{enabled: true, proxyEvts: true, blockedEvts: true, lastToast: make(map[string]time.Time)}
}

func (n *notifier) configure(enabled, proxyEvts, blockedEvts bool) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.enabled, n.proxyEvts, n.blockedEvts = enabled, proxyEvts, blockedEvts
}

func (n *notifier) proxyEvents() bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.enabled && n.proxyEvts
}

func (n *notifier) blocked() bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.enabled && n.blockedEvts
}

// toast shows a desktop notification, at most one per title per 5 seconds.
func (n *notifier) toast(title, msg string) {
	n.mu.Lock()
	if !n.enabled {
		n.mu.Unlock()
		return
	}
	if t, ok := n.lastToast[title]; ok && time.Since(t) < 5*time.Second {
		n.mu.Unlock()
		return
	}
	n.lastToast[title] = time.Now()
	n.mu.Unlock()

	go func() {
		// beeep is best-effort; a toast failure must never kill the GUI.
		_ = beeep.Notify(title, msg, n.iconPath)
	}()
}
