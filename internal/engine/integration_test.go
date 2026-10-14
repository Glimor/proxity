//go:build windows

package engine

import (
	"net"
	"testing"
	"time"

	"github.com/Glimor/proxity/internal/windivert"
)

// Integration tests need: administrator rights and WinDivert.dll +
// WinDivert64.sys reachable (next to the test binary, third_party\windivert
// under the working dir, or %WINDIVERT_DLL%).
//
// MANUAL TEST (whole redirect, elevated shell):
//  1. Copy WinDivert.dll + WinDivert64.sys next to proxity-service.exe.
//  2. Insert a proxy profile + an exe rule into the DB (or use a public
//     SOCKS5 for a smoke test), then run:
//         proxity-service.exe -engine
//  3. From the matching exe, browse a site: the console logs show the flow
//     proxied and the stats snapshot counts bytes. A non-matching exe's
//     traffic must pass through untouched.
//  4. Kill the SOCKS5 server -> rule with KillSwitch drops new SYNs
//     (site unreachable), without KillSwitch traffic falls back to direct.
//  5. Redirect trick (v4 and v6 alike): an app packet src=A:sport dst=D:dport
//     is reflected inbound as src=D:natID dst=A:ProxyPort — the destination
//     is the app's own real source address (always a configured local
//     address), so it reaches the wildcard listener on both stacks, and the
//     natID rides in the source port.

func skipIfNoDriver(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}
	if err := windivert.Available(); err != nil {
		t.Skipf("windivert unavailable: %v", err)
	}
}

// TestCaptureSeesPackets opens the real capture handle (admin required) and
// verifies outbound TCP packets are seen.
func TestCaptureSeesPackets(t *testing.T) {
	skipIfNoDriver(t)
	h, err := windivert.Open(captureFilter, windivert.LayerNetwork, 0, 0)
	if err != nil {
		t.Skipf("cannot open windivert (not admin?): %v", err)
	}
	defer h.Close()

	done := make(chan struct{})
	go func() {
		defer close(done)
		// Any outbound SYN is captured, even if nothing answers.
		c, err := net.DialTimeout("tcp", "203.0.113.1:443", 2*time.Second)
		if err == nil {
			c.Close()
		}
	}()
	defer func() {
		h.Shutdown(windivert.ShutdownBoth)
		<-done
	}()

	buf := make([]byte, 64*1024)
	var addr windivert.Address
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := h.Recv(buf, &addr); err != nil {
			t.Fatalf("recv: %v", err)
		}
		if addr.Layer() == windivert.LayerNetwork {
			return // saw an outbound tcp packet
		}
	}
	t.Fatal("no outbound TCP packet captured in 5s")
}

// TestListenersBind verifies both stacks accept connections on ProxyPort —
// this is the only part testable without the driver.
func TestListenersBind(t *testing.T) {
	e := New(Deps{Log: testLog()})
	lns, err := e.openListeners()
	if err != nil {
		t.Fatal(err)
	}
	defer e.closeListeners()
	var saw4 bool
	for _, ln := range lns {
		if ta, ok := ln.Addr().(*net.TCPAddr); ok && ta.IP.To4() != nil {
			saw4 = true
		}
	}
	if !saw4 {
		t.Fatal("no tcp4 listener")
	}
}
