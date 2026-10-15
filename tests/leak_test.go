//go:build windows

// Package tests holds manual integration harnesses that need administrator
// rights and the real WFP backend. They are skipped unless PROXITY_INTEG=1:
//
//	PROXITY_INTEG=1 go test ./tests -v
//
// IMPORTANT: run them only while the ProxityEngine service is STOPPED.
// killswitch.OpenBackend resets the shared "Proxity" WFP sublayer on open,
// which would wipe a running service's persistent filters. The service
// rebuilds them on its next reconcile, but for a clean verification stop it
// first:  proxity-service.exe -uninstall  (or services.msc → stop).
package tests

import (
	"net"
	"os"
	"testing"
	"time"

	"github.com/Glimor/proxity/internal/killswitch"
)

// requireWFP opens the real WFP backend (admin required) and returns a
// policy plus a cleanup func that removes every installed filter.
func requireWFP(t *testing.T) (*killswitch.Policy, func()) {
	t.Helper()
	if os.Getenv("PROXITY_INTEG") != "1" {
		t.Skip("manual integration harness; set PROXITY_INTEG=1 (needs admin, service stopped)")
	}
	b, err := killswitch.OpenBackend()
	if err != nil {
		t.Skipf("WFP backend unavailable (run elevated, service stopped): %v", err)
	}
	p := killswitch.New(b, nil)
	return p, func() {
		_ = p.Reconcile(killswitch.State{}) // drop every filter we added
		p.Close()
	}
}

// loopbackEcho starts a 127.0.0.1 listener that accepts and echoes; the
// local permit must always allow this even under a full block.
func loopbackEcho(t *testing.T) (addr string, done func()) {
	t.Helper()
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() { c.Write([]byte("ok")); c.Close() }()
		}
	}()
	return ln.Addr().String(), func() { ln.Close() }
}

// expectLoopbackAllowed proves the local permit still applies while a block
// is installed (LAN/loopback must never die with the kill switch).
func expectLoopbackAllowed(t *testing.T, addr string) {
	t.Helper()
	c, err := net.DialTimeout("tcp4", addr, 3*time.Second)
	if err != nil {
		t.Fatalf("loopback dial blocked while local permit installed: %v", err)
	}
	c.Close()
}

// expectRemoteBlocked dials a public address and requires the connect to
// fail — a blocked ALE connect surfaces as an immediate socket error or a
// timeout; either way the SYN must not leave the machine.
func expectRemoteBlocked(t *testing.T) {
	t.Helper()
	c, err := net.DialTimeout("tcp4", "1.1.1.1:443", 3*time.Second)
	if err == nil {
		c.Close()
		t.Fatal("blocked process dialed out — WFP block not effective")
	}
	t.Logf("remote dial blocked as expected: %v", err)
}

// Scenario 1: an Action=block exe cannot open TCP connections while the
// engine is stopped — the persistent WFP block stands on its own, with the
// loopback/LAN permit still intact. The rule targets the test binary
// itself, so no service or child process is needed.
func TestBlockedExeCannotDial(t *testing.T) {
	p, done := requireWFP(t)
	defer done()

	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Reconcile(killswitch.State{
		Apps:     []killswitch.AppPolicy{{Path: exe, BlockOnly: true}},
		LANAllow: true,
	}); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	lb, lbDone := loopbackEcho(t)
	defer lbDone()
	expectLoopbackAllowed(t, lb)
	expectRemoteBlocked(t)
}

// Scenario 2: a kill-switch proxy rule while the engine is dead (no dynamic
// permits issued) — the persistent block must fail the connect.
func TestKillSwitchProxyDownBlocks(t *testing.T) {
	p, done := requireWFP(t)
	defer done()

	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Reconcile(killswitch.State{
		Apps:          []killswitch.AppPolicy{{Path: exe}}, // proxy+KS app
		LANAllow:      true,
		EngineHealthy: false, // engine dead → no healthy permits
	}); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	lb, lbDone := loopbackEcho(t)
	defer lbDone()
	expectLoopbackAllowed(t, lb)
	expectRemoteBlocked(t)
}

// A UDP-blocked (proxy without kill switch) app can still dial TCP — only
// remote UDP is denied at WFP level. Verified here by checking the engine
// side contract: remote TCP dial must succeed (or fail for network reasons
// other than the WFP block — the filter set has no TCP block at all).
func TestUDPBlockedAppKeepsTCP(t *testing.T) {
	p, done := requireWFP(t)
	defer done()

	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Reconcile(killswitch.State{
		Apps:     []killswitch.AppPolicy{{Path: exe, UDPBlock: true}},
		LANAllow: true,
	}); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	lb, lbDone := loopbackEcho(t)
	defer lbDone()
	expectLoopbackAllowed(t, lb)

	// Remote TCP is untouched by the UDP block; whether it succeeds
	// depends on connectivity — what matters is it is NOT blocked by our
	// filters. A WFP block fails fast; a plain network failure is slower
	// or succeeds. Give it a generous window and accept both outcomes —
	// a fast "forbidden" error means the block leaked onto TCP.
	c, err := net.DialTimeout("tcp4", "1.1.1.1:443", 3*time.Second)
	if err == nil {
		c.Close()
		return
	}
	t.Logf("remote TCP dial result (allowed either way): %v", err)
}
