//go:build windows

package engine

import (
	"context"
	"log/slog"
	"net"
	"net/netip"
	"testing"
	"time"

	"github.com/Glimor/proxity/internal/domain"
)

func testLog() *slog.Logger { return slog.New(slog.NewTextHandler(discard{}, nil)) }

type discard struct{}

func (discard) Write(p []byte) (int, error) { return len(p), nil }

func tcpAddr(ip string, port int) *net.TCPAddr {
	return &net.TCPAddr{IP: net.ParseIP(ip), Port: port}
}

var flowPortCounter uint16 = 40000

func mkflow(id uint32, v6 bool) *flow {
	flowPortCounter++
	src := netip.MustParseAddr("192.168.1.5")
	dst := netip.MustParseAddr("93.184.216.34")
	if v6 {
		src = netip.MustParseAddr("2001:db8::5")
		dst = netip.MustParseAddr("2001:db8::9")
	}
	return &flow{
		natID:       id,
		origSrc:     src,
		origSrcPort: flowPortCounter,
		origDst:     dst,
		origDstPort: 443,
		ipv6:        v6,
		action:      domain.DecisionProxy,
	}
}

func TestFlowTableAllocReuse(t *testing.T) {
	ft := newFlowTable()
	f1, f2 := mkflow(0, false), mkflow(0, false)
	if !ft.add(f1, true) || !ft.add(f2, true) {
		t.Fatal("add failed")
	}
	if f1.natID == 0 || f2.natID == 0 || f1.natID == f2.natID {
		t.Fatalf("bad ids %d %d", f1.natID, f2.natID)
	}
	if got := ft.getByID(f1.natID); got != f1 {
		t.Fatal("byID miss")
	}
	k := tupleKey{f1.origSrc, f1.origDst, f1.origSrcPort, f1.origDstPort}
	if got := ft.getByTuple(k); got != f1 {
		t.Fatal("byTuple miss")
	}
	ft.remove(f1)
	f3 := mkflow(0, false)
	if !ft.add(f3, true) {
		t.Fatal("add3 failed")
	}
	if f3.natID != f1.natID {
		t.Fatalf("expected id reuse %d got %d", f1.natID, f3.natID)
	}
	if ft.len() != 2 {
		t.Fatalf("len=%d", ft.len())
	}
}

func TestFlowTableIDSpace(t *testing.T) {
	ft := newFlowTable()
	// natIDs live in [1, maxNATID] regardless of address family — the id
	// rides in the reflected source port now, not in a NAT address.
	v4, v6 := mkflow(0, false), mkflow(0, true)
	if !ft.add(v4, true) || !ft.add(v6, true) {
		t.Fatal("add failed")
	}
	if v4.natID < 1 || v4.natID > maxNATID || v6.natID < 1 || v6.natID > maxNATID {
		t.Fatalf("ids out of range: %d %d", v4.natID, v6.natID)
	}
	if v4.natID == v6.natID {
		t.Fatal("v4/v6 ids collide")
	}
	ft.remove(v4)
	f := mkflow(0, true)
	if !ft.add(f, true) || f.natID != v4.natID {
		t.Fatalf("freed id not reused: %d", f.natID)
	}
}

func TestFlowTableSweep(t *testing.T) {
	ft := newFlowTable()
	f := mkflow(0, false)
	ft.add(f, true)
	f.lastSeen = time.Now().Add(-3 * time.Hour)
	out := ft.sweep()
	if len(out) != 1 || out[0] != f {
		t.Fatal("sweep missed stale flow")
	}
	if ft.getByID(f.natID) != nil {
		t.Fatal("stale id still present")
	}
}

func TestFlowTableRemoveByLocal(t *testing.T) {
	ft := newFlowTable()
	f := mkflow(0, false)
	ft.add(f, true)
	ft.removeByLocal(f.origSrc, f.origSrcPort)
	if ft.len() != 0 || ft.getByID(f.natID) != nil {
		t.Fatal("removeByLocal failed")
	}
}

type fakeStatsStore struct {
	mu         chan struct{}
	deltas     []string
	outs       []domain.ConnStatus
	exeDaily   []domain.ExeDailyStats
	proxyDaily []domain.ProxyDailyStats
}

func (f *fakeStatsStore) AddFlowDelta(ctx context.Context, exePath, proxyID string, up, down int64) error {
	f.deltas = append(f.deltas, exePath)
	return nil
}
func (f *fakeStatsStore) RecordOutcome(ctx context.Context, s domain.ConnStatus) error {
	f.outs = append(f.outs, s)
	return nil
}
func (f *fakeStatsStore) SessionCounters(ctx context.Context) (domain.SessionCounters, error) {
	return domain.SessionCounters{}, nil
}
func (f *fakeStatsStore) UpsertExeDaily(ctx context.Context, s domain.ExeDailyStats) error {
	f.exeDaily = append(f.exeDaily, s)
	return nil
}
func (f *fakeStatsStore) UpsertProxyDaily(ctx context.Context, s domain.ProxyDailyStats) error {
	f.proxyDaily = append(f.proxyDaily, s)
	return nil
}
func (f *fakeStatsStore) ExeDailyRange(ctx context.Context, a, b string) ([]domain.ExeDailyStats, error) {
	return nil, nil
}
func (f *fakeStatsStore) ProxyDailyRange(ctx context.Context, a, b string) ([]domain.ProxyDailyStats, error) {
	return nil, nil
}
func (f *fakeStatsStore) DeleteDailyBefore(ctx context.Context, day string) (int64, error) {
	return 0, nil
}
func (f *fakeStatsStore) SaveLastSession(ctx context.Context, s domain.LastSession) error {
	return nil
}
func (f *fakeStatsStore) LastSession(ctx context.Context) (*domain.LastSession, error) {
	return nil, nil
}
func (f *fakeStatsStore) ResetExe(ctx context.Context, e string) error { return nil }
func (f *fakeStatsStore) ResetAll(ctx context.Context) error           { return nil }

func TestAggregatorMath(t *testing.T) {
	store := &fakeStatsStore{}
	a := newAggregator(store, nil, testLog())
	a.ctx = context.Background()
	id1 := a.register(domain.Connection{ExePath: "C:\\APP.EXE", ProxyID: "p1"})
	id2 := a.register(domain.Connection{ExePath: "C:\\APP.EXE", ProxyID: "p2"})
	a.add(id1, 1000, 2000)
	a.add(id2, 500, 0)
	a.tick()
	snap := a.Snapshot()
	if snap.TotalUpBps != 1500 || snap.TotalDownBps != 2000 {
		t.Fatalf("bps: %+v", snap)
	}
	if snap.ActiveConns != 2 {
		t.Fatalf("conns: %+v", snap)
	}
	if snap.PerExe["C:\\APP.EXE"].UpBps != 1500 || snap.PerProxy["p1"].DownBps != 2000 {
		t.Fatalf("per-exe/proxy: %+v", snap)
	}
	// Second tick with no new bytes -> zero bps, totals retained.
	a.tick()
	snap = a.Snapshot()
	if snap.TotalUpBps != 0 || snap.PerExe["C:\\APP.EXE"].TotalUp != 1500 {
		t.Fatalf("tick2: %+v", snap)
	}
	a.close(id2, domain.ConnClosed)
	a.recordBlocked(domain.BlockedEntry{})
	a.tick()
	snap = a.Snapshot()
	if snap.SucceededCount != 1 || snap.BlockedCount != 1 {
		t.Fatalf("counters: %+v", snap)
	}
	if len(a.ActiveConnections()) != 1 {
		t.Fatal("active list not updated")
	}
	if len(store.outs) != 2 {
		t.Fatalf("outcomes not persisted: %v", store.outs)
	}
}

// flushDaily upserts accumulated byte deltas plus conn count and duration
// into the daily tables, keyed by (exe, proxy).
func TestAggregatorDailyFlush(t *testing.T) {
	store := &fakeStatsStore{}
	a := newAggregator(store, nil, testLog())
	a.ctx = context.Background()

	id := a.register(domain.Connection{ExePath: `C:\app.exe`, ProxyID: "p1"})
	// register() stamps StartedAt itself — backdate the entry so the close
	// contributes a non-zero duration.
	a.mu.Lock()
	a.conns[id].conn.StartedAt = time.Now().Add(-5 * time.Second)
	a.mu.Unlock()
	a.add(id, 1000, 2000)
	a.tick()
	a.close(id, domain.ConnClosed) // ~5s lifetime → seconds counted

	a.flushDaily(time.Now())
	if len(store.exeDaily) != 1 || len(store.proxyDaily) != 1 {
		t.Fatalf("expected 1+1 daily upserts, got %v %v", store.exeDaily, store.proxyDaily)
	}
	e := store.exeDaily[0]
	if e.Day != time.Now().Format("2006-01-02") || e.ExePath != `C:\app.exe` || e.ProxyID != "p1" {
		t.Fatalf("bad exe row: %+v", e)
	}
	if e.Up != 1000 || e.Down != 2000 || e.Conns != 1 || e.Seconds < 4 {
		t.Fatalf("bad exe delta: %+v", e)
	}
	p := store.proxyDaily[0]
	if p.ProxyID != "p1" || p.Up != 1000 || p.Down != 2000 || p.Conns != 1 {
		t.Fatalf("bad proxy row: %+v", p)
	}

	// Pending must be empty after a successful flush: a second flush writes nothing.
	a.flushDaily(time.Now())
	if len(store.exeDaily) != 1 {
		t.Fatal("flush did not drain pending deltas")
	}
}

// The listener recovers the flow from the peer's source port (natID) and
// verifies the peer address equals the flow's original destination.
func TestFlowForRemote(t *testing.T) {
	e := New(Deps{Log: testLog()})
	for _, v6 := range []bool{false, true} {
		f := mkflow(0, v6)
		if !e.flows.add(f, true) {
			t.Fatal("add failed")
		}
		got := e.flowForRemote(tcpAddr(f.origDst.String(), int(f.natID)))
		if got != f {
			t.Fatalf("v6=%v: flow not recovered", v6)
		}
		// Wrong peer address must be rejected (sanity check).
		bogus := "203.0.113.9"
		if v6 {
			bogus = "2001:db8::dead"
		}
		if e.flowForRemote(tcpAddr(bogus, int(f.natID))) != nil {
			t.Fatalf("v6=%v: accepted wrong peer addr", v6)
		}
		e.flows.remove(f)
	}
	// Unknown natID and nil addr are rejected.
	if e.flowForRemote(tcpAddr("93.184.216.34", 65000)) != nil {
		t.Fatal("resolved unknown nat id")
	}
	if e.flowForRemote(nil) != nil {
		t.Fatal("resolved nil remote addr")
	}
}
