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
	"github.com/Glimor/proxity/internal/netpkt"
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
	ft := newFlowTable(false, poolSize)
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

func TestFlowTableV6PoolRange(t *testing.T) {
	ft := newFlowTable(true, 4)
	var v6 []*flow
	for i := 0; i < 4; i++ {
		f := mkflow(0, true)
		if !ft.add(f, true) {
			t.Fatalf("v6 add %d failed", i)
		}
		if f.natID < 1 || f.natID > 4 {
			t.Fatalf("v6 id out of pool: %d", f.natID)
		}
		v6 = append(v6, f)
	}
	if ok := ft.add(mkflow(0, true), true); ok {
		t.Fatal("pool exhaustion not detected")
	}
	v4 := mkflow(0, false)
	if !ft.add(v4, true) {
		t.Fatal("v4 add failed")
	}
	if v4.natID <= 4 {
		t.Fatalf("v4 id collides with pool: %d", v4.natID)
	}
	ft.remove(v6[0])
	f := mkflow(0, true)
	if !ft.add(f, true) || f.natID != v6[0].natID {
		t.Fatalf("v6 id not reused: %d", f.natID)
	}
}

func TestFlowTableSweep(t *testing.T) {
	ft := newFlowTable(false, poolSize)
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
	ft := newFlowTable(false, poolSize)
	f := mkflow(0, false)
	ft.add(f, true)
	ft.removeByLocal(f.origSrc, f.origSrcPort)
	if ft.len() != 0 || ft.getByID(f.natID) != nil {
		t.Fatal("removeByLocal failed")
	}
}

type fakeStatsStore struct {
	mu     chan struct{}
	deltas []string
	outs   []domain.ConnStatus
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
	return nil
}
func (f *fakeStatsStore) UpsertProxyDaily(ctx context.Context, s domain.ProxyDailyStats) error {
	return nil
}
func (f *fakeStatsStore) ExeDailyRange(ctx context.Context, a, b string) ([]domain.ExeDailyStats, error) {
	return nil, nil
}
func (f *fakeStatsStore) ProxyDailyRange(ctx context.Context, a, b string) ([]domain.ProxyDailyStats, error) {
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

func TestDecodeNATOnListener(t *testing.T) {
	e := New(Deps{Log: testLog()})
	// v4
	ip := netpkt.EncodeNATIPv4(42).As4()
	id, ok := e.decodeNAT(tcpAddr(net.IP(ip[:]).String(), ProxyPort))
	if !ok || id != 42 {
		t.Fatalf("v4 decode id=%d ok=%v", id, ok)
	}
	// v6 ula
	a := netpkt.EncodeNATIPv6(77)
	id, ok = e.decodeNAT(tcpAddr(a.String(), ProxyPort))
	if !ok || id != 77 {
		t.Fatalf("v6 decode id=%d ok=%v", id, ok)
	}
	// pool mode
	e.v6Pool = true
	id, ok = e.decodeNAT(tcpAddr("::1", int(poolBasePort)+4))
	if !ok || id != 5 {
		t.Fatalf("pool decode id=%d ok=%v", id, ok)
	}
	// foreign addr
	if _, ok := e.decodeNAT(tcpAddr("8.8.8.8", ProxyPort)); ok {
		t.Fatal("decoded foreign addr")
	}
}
