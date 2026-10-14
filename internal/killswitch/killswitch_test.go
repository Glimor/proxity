package killswitch

import (
	"errors"
	"sort"
	"strings"
	"testing"

	"github.com/google/uuid"
)

type fakeBackend struct {
	added     []Spec
	deleted   []uuid.UUID
	reopens   int
	txns      int
	baseCalls int
	failAdd   string // spec name substring that triggers an Add error
	appIDs    map[string][]byte
}

func (f *fakeBackend) EnsureBase() error    { f.baseCalls++; return nil }
func (f *fakeBackend) ReopenSession() error { f.reopens++; return nil }
func (f *fakeBackend) Txn(fn func() error) error {
	f.txns++
	return fn()
}
func (f *fakeBackend) Add(s Spec) error {
	if f.failAdd != "" && s.Name == f.failAdd {
		return errors.New("add failed")
	}
	f.added = append(f.added, s)
	return nil
}
func (f *fakeBackend) Delete(k uuid.UUID) error {
	f.deleted = append(f.deleted, k)
	return nil
}
func (f *fakeBackend) AppIDOf(path string) ([]byte, error) {
	if f.appIDs != nil {
		if id, ok := f.appIDs[path]; ok {
			return id, nil
		}
		return nil, errors.New("no such file")
	}
	return []byte("appid:" + path), nil
}

func hasName(specs []Spec, substr string) bool {
	for _, s := range specs {
		if strings.Contains(s.Name, substr) {
			return true
		}
	}
	return false
}

func TestReconcileBasicKillSwitchApp(t *testing.T) {
	fb := &fakeBackend{}
	p := New(fb, nil)

	st := State{
		Apps:          []AppPolicy{{Path: `C:\Apps\foo.exe`, BlockOnly: false}},
		EngineHealthy: true,
	}
	if err := p.Reconcile(st); err != nil {
		t.Fatal(err)
	}
	if len(fb.added) != 6 {
		t.Fatalf("expected 6 filters (block+local+healthy x v4/v6), got %d: %v",
			len(fb.added), fb.added)
	}
	// Verify weights/actions/persistence.
	for _, s := range fb.added {
		switch {
		case hasName([]Spec{s}, "|block|"):
			if !s.Block || !s.Persistent || s.Weight != WeightBlock {
				t.Fatalf("bad block spec %+v", s)
			}
		case hasName([]Spec{s}, "|local|"):
			if s.Block || !s.Persistent || s.Weight != WeightPermitLocal || len(s.Remote) == 0 {
				t.Fatalf("bad local spec %+v", s)
			}
		case hasName([]Spec{s}, "|healthy|"):
			if s.Block || s.Persistent || s.Weight != WeightPermitHealth || !s.TCPOnly {
				t.Fatalf("bad healthy spec %+v", s)
			}
		}
		if s.AppID == nil {
			t.Fatalf("app filter without AppID: %+v", s)
		}
	}

	// Idempotent: same state → no new ops.
	if err := p.Reconcile(st); err != nil {
		t.Fatal(err)
	}
	if len(fb.added) != 6 || len(fb.deleted) != 0 {
		t.Fatalf("reconcile not idempotent: added=%d deleted=%d", len(fb.added), len(fb.deleted))
	}
}

func TestSecureDNSFilters(t *testing.T) {
	fb := &fakeBackend{}
	p := New(fb, nil)

	st := State{
		SecureDNS: true,
		SelfPath:  `C:\Proxity\proxity-service.exe`,
	}
	if err := p.Reconcile(st); err != nil {
		t.Fatal(err)
	}
	var blocks, selfPermits int
	for _, s := range fb.added {
		switch {
		case strings.HasPrefix(s.Name, "dns|block"):
			blocks++
			if !s.Block || !s.Persistent || s.Weight != WeightBlockDNS || s.RemotePort != 53 {
				t.Fatalf("bad dns block spec %+v", s)
			}
			if !s.NoLoopback {
				t.Fatalf("dns block must exclude loopback (forwarder lives there): %+v", s)
			}
			if s.AppID != nil {
				t.Fatalf("dns block must cover all processes: %+v", s)
			}
		case strings.HasPrefix(s.Name, "dns|self"):
			selfPermits++
			if s.Block || s.Weight != WeightPermitDNS || s.RemotePort != 53 || s.AppID == nil {
				t.Fatalf("bad dns self permit %+v", s)
			}
		}
	}
	if blocks != 2 || selfPermits != 2 {
		t.Fatalf("expected 2+2 dns filters (v4+v6), got block=%d self=%d", blocks, selfPermits)
	}

	// Weight ordering: the self permit must beat the block.
	if WeightPermitDNS <= WeightBlockDNS {
		t.Fatal("dns permit must outweigh dns block")
	}

	// Toggling off removes the dns filters plus the always-on self
	// recv-accept pair.
	if err := p.Reconcile(State{}); err != nil {
		t.Fatal(err)
	}
	if len(fb.deleted) != 6 {
		t.Fatalf("expected 6 deletes, got %d", len(fb.deleted))
	}
}

// Without a self path the block still installs but the permit is skipped.
func TestSecureDNSNoSelf(t *testing.T) {
	fb := &fakeBackend{}
	p := New(fb, nil)
	if err := p.Reconcile(State{SecureDNS: true}); err != nil {
		t.Fatal(err)
	}
	var n int
	for _, s := range fb.added {
		if strings.HasPrefix(s.Name, "dns|block") {
			n++
		}
	}
	if n != 2 {
		t.Fatalf("expected 2 dns block filters, got %d", n)
	}
}

func TestReconcileDeletesStale(t *testing.T) {
	fb := &fakeBackend{}
	p := New(fb, nil)
	app := AppPolicy{Path: `C:\Apps\foo.exe`}
	st := State{Apps: []AppPolicy{app}}
	if err := p.Reconcile(st); err != nil {
		t.Fatal(err)
	}
	if err := p.Reconcile(State{}); err != nil {
		t.Fatal(err)
	}
	if len(fb.deleted) != 4 {
		t.Fatalf("expected 4 deletes (block+local x2), got %d", len(fb.deleted))
	}
	if len(p.InstalledNames()) != 0 {
		t.Fatalf("installed not empty: %v", p.InstalledNames())
	}
}

func TestSetHealthyRevokesPermits(t *testing.T) {
	fb := &fakeBackend{}
	p := New(fb, nil)
	st := State{Apps: []AppPolicy{{Path: `C:\Apps\foo.exe`}}, EngineHealthy: true}
	if err := p.Reconcile(st); err != nil {
		t.Fatal(err)
	}
	if len(fb.added) != 6 {
		t.Fatalf("got %d adds", len(fb.added))
	}

	// Unhealthy: session reopen revokes dynamic permits; the healthy filters
	// are dropped from installed without any WFP delete calls (already gone).
	if err := p.SetHealthy(false); err != nil {
		t.Fatal(err)
	}
	if fb.reopens != 1 {
		t.Fatalf("expected 1 reopen, got %d", fb.reopens)
	}
	for _, n := range p.InstalledNames() {
		if hasName([]Spec{{Name: n}}, "healthy") {
			t.Fatalf("healthy filter still installed: %s", n)
		}
	}

	// Healthy again: permits are re-added to the fresh session.
	if err := p.SetHealthy(true); err != nil {
		t.Fatal(err)
	}
	if fb.reopens != 2 {
		t.Fatalf("expected 2 reopens, got %d", fb.reopens)
	}
	names := p.InstalledNames()
	var healthy int
	for _, n := range names {
		if hasName([]Spec{{Name: n}}, "|healthy|") {
			healthy++
		}
	}
	if healthy != 2 {
		t.Fatalf("expected 2 healthy filters re-added, got %d (%v)", healthy, names)
	}
}

func TestBlockOnlyAppNeverGetsPermit(t *testing.T) {
	fb := &fakeBackend{}
	p := New(fb, nil)
	st := State{
		Apps:          []AppPolicy{{Path: `C:\Apps\foo.exe`, BlockOnly: true}},
		EngineHealthy: true,
	}
	if err := p.Reconcile(st); err != nil {
		t.Fatal(err)
	}
	for _, s := range fb.added {
		if hasName([]Spec{s}, "|healthy|") {
			t.Fatalf("block-only app got a healthy permit: %+v", s)
		}
	}
	if len(fb.added) != 4 {
		t.Fatalf("expected 4 filters, got %d", len(fb.added))
	}
}

func TestGlobalKillSwitch(t *testing.T) {
	fb := &fakeBackend{}
	p := New(fb, nil)
	st := State{Global: true, LANAllow: true, EngineHealthy: true, SelfPath: `C:\Proxity\proxity-service.exe`}
	if err := p.Reconcile(st); err != nil {
		t.Fatal(err)
	}
	// global self, local, block, healthy x v4/v6 = 8, plus the always-on
	// self recv-accept pair = 10.
	if len(fb.added) != 10 {
		t.Fatalf("expected 10 filters, got %d", len(fb.added))
	}
	for _, s := range fb.added {
		switch {
		case hasName([]Spec{s}, "global|self"):
			if !s.Persistent || s.Weight != WeightPermitSelf || s.AppID == nil || s.Block {
				t.Fatalf("bad self permit %+v", s)
			}
		case hasName([]Spec{s}, "global|local"):
			if !s.Persistent || s.AppID != nil || len(s.Remote) != 11 {
				t.Fatalf("bad global local permit %+v", s)
			}
		case hasName([]Spec{s}, "global|block"):
			if !s.Persistent || !s.Block || s.AppID != nil {
				t.Fatalf("bad global block %+v", s)
			}
		case hasName([]Spec{s}, "global|healthy"):
			if s.Persistent || s.Block || !s.TCPOnly {
				t.Fatalf("bad global healthy permit %+v", s)
			}
		}
	}
}

func TestLANToggleRereconciles(t *testing.T) {
	fb := &fakeBackend{}
	p := New(fb, nil)
	st := State{Apps: []AppPolicy{{Path: `C:\Apps\foo.exe`}}, LANAllow: false}
	if err := p.Reconcile(st); err != nil {
		t.Fatal(err)
	}
	var remote int
	for _, s := range fb.added {
		if hasName([]Spec{s}, "|local|") && !s.IPv6 {
			remote = len(s.Remote)
		}
	}
	if remote != 2 { // loopback v4+v6; LAN ranges added only when LANAllow
		t.Fatalf("expected 2 local prefixes without LAN, got %d", remote)
	}

	st.LANAllow = true
	if err := p.Reconcile(st); err != nil {
		t.Fatal(err)
	}
	// Changed spec under same key: delete + add.
	if len(fb.deleted) != 2 || len(fb.added) != 6 {
		t.Fatalf("expected 2 deletes + 6 adds total, got del=%d add=%d", len(fb.deleted), len(fb.added))
	}
}

func TestAddFailureKeepsInstalledConsistent(t *testing.T) {
	fb := &fakeBackend{failAdd: "app|C:\\APPS\\FOO.EXE|local|v4"}
	p := New(fb, nil)
	st := State{Apps: []AppPolicy{{Path: `C:\Apps\foo.exe`}}}
	if err := p.Reconcile(st); err == nil {
		t.Fatal("expected error")
	}
	// Transaction aborted: installed must remain empty so next reconcile
	// retries everything.
	if len(p.InstalledNames()) != 0 {
		t.Fatalf("installed leaked: %v", p.InstalledNames())
	}
	fb.failAdd = ""
	if err := p.Reconcile(st); err != nil {
		t.Fatal(err)
	}
	if len(p.InstalledNames()) != 4 {
		t.Fatalf("expected 4 installed, got %v", p.InstalledNames())
	}
}

// The recv-accept self permit is always installed when SelfPath is known —
// it exists so the reflected SYNs our transparent redirect sends to our own
// listener survive the default inbound-block policy. Not gated on health.
func TestSelfRecvAcceptPermitAlwaysOn(t *testing.T) {
	fb := &fakeBackend{}
	p := New(fb, nil)

	st := State{SelfPath: `C:\Proxity\proxity-service.exe`} // EngineHealthy: false
	if err := p.Reconcile(st); err != nil {
		t.Fatal(err)
	}
	var n int
	for _, s := range fb.added {
		if !strings.HasPrefix(s.Name, "self|recvaccept") {
			continue
		}
		n++
		if s.Block || !s.Persistent || !s.RecvAccept {
			t.Fatalf("bad recv-accept permit %+v", s)
		}
		if s.Weight != WeightPermitSelf || s.AppID == nil {
			t.Fatalf("bad recv-accept permit %+v", s)
		}
		if s.TCPOnly || s.UDPOnly || s.RemotePort != 0 || len(s.Remote) != 0 {
			t.Fatalf("recv-accept permit must match appID only: %+v", s)
		}
	}
	if n != 2 {
		t.Fatalf("expected v4+v6 recv-accept permits, got %d", n)
	}

	// No SelfPath → no permit.
	fb2 := &fakeBackend{}
	p2 := New(fb2, nil)
	if err := p2.Reconcile(State{Global: true}); err != nil {
		t.Fatal(err)
	}
	for _, s := range fb2.added {
		if strings.HasPrefix(s.Name, "self|") {
			t.Fatalf("recv-accept permit without SelfPath: %+v", s)
		}
	}
}

// A proxy rule without kill switch gets a remote-UDP block (UDP can never
// be proxied, so it must not leak direct) plus the usual local permit.
// TCP stays unfiltered: no all-proto block, no healthy permit.
func TestUDPBlockApp(t *testing.T) {
	fb := &fakeBackend{}
	p := New(fb, nil)
	st := State{
		Apps:          []AppPolicy{{Path: `C:\Apps\foo.exe`, UDPBlock: true}},
		EngineHealthy: true,
	}
	if err := p.Reconcile(st); err != nil {
		t.Fatal(err)
	}
	if len(fb.added) != 4 {
		t.Fatalf("expected 4 filters (udpblock+local x v4/v6), got %d: %v",
			len(fb.added), fb.added)
	}
	for _, s := range fb.added {
		switch {
		case hasName([]Spec{s}, "|udpblock|"):
			if !s.Block || !s.Persistent || !s.UDPOnly || s.TCPOnly ||
				s.Weight != WeightBlock || s.AppID == nil {
				t.Fatalf("bad udpblock spec %+v", s)
			}
		case hasName([]Spec{s}, "|local|"):
			if s.Block || !s.Persistent || s.Weight != WeightPermitLocal || len(s.Remote) == 0 {
				t.Fatalf("bad local spec %+v", s)
			}
		default:
			t.Fatalf("unexpected filter for udp-blocked app: %+v", s)
		}
	}
}

// When the same path is claimed by both a block rule and a no-KS proxy
// rule, BlockOnly wins: the all-proto block already covers UDP, so no
// extra udpblock filter is emitted.
func TestUDPBlockYieldsToBlockOnly(t *testing.T) {
	fb := &fakeBackend{}
	p := New(fb, nil)
	st := State{Apps: []AppPolicy{{Path: `C:\Apps\foo.exe`, BlockOnly: true, UDPBlock: true}}}
	if err := p.Reconcile(st); err != nil {
		t.Fatal(err)
	}
	if len(fb.added) != 4 {
		t.Fatalf("expected 4 filters (block+local x2), got %d", len(fb.added))
	}
	for _, s := range fb.added {
		if hasName([]Spec{s}, "udpblock") {
			t.Fatalf("redundant udpblock filter alongside all-proto block: %+v", s)
		}
	}
}

func TestDeterministicKeys(t *testing.T) {
	a := keyFor("app|X|block|v4")
	b := keyFor("app|X|block|v4")
	c := keyFor("app|X|block|v6")
	if a != b || a == c {
		t.Fatal("keys not deterministic or colliding")
	}
}

func TestSortedInstalledNames(t *testing.T) {
	fb := &fakeBackend{}
	p := New(fb, nil)
	p.Reconcile(State{Apps: []AppPolicy{{Path: `C:\a\b.exe`}, {Path: `C:\a\c.exe`}}})
	names := p.InstalledNames()
	if !sort.StringsAreSorted(names) {
		t.Fatal("not sorted")
	}
	if len(names) != 8 {
		t.Fatalf("expected 8, got %v", names)
	}
}
