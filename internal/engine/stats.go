//go:build windows

package engine

import (
	"context"
	"log/slog"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Glimor/proxity/internal/domain"
	"github.com/Glimor/proxity/internal/ports"
)

const histLen = 60

// flushEvery is the daily-stats rollup cadence: pending per-(exe,proxy)
// deltas are upserted into exe_daily_stats / proxy_daily_stats once a
// minute (and once more on shutdown).
const flushEvery = time.Minute

type connEntry struct {
	conn             domain.Connection
	up, down         atomic.Int64
	lastUp, lastDown int64
}

// dayDelta accumulates bytes/conns/seconds between daily flushes.
type dayDelta struct {
	up, down, conns, secs int64
}

type dayKey struct {
	exe   string
	proxy string
}

type aggregator struct {
	mu     sync.Mutex
	conns  map[int64]*connEntry
	nextID atomic.Int64

	nBlocked   atomic.Int64
	nFailed    atomic.Int64
	nSucceeded atomic.Int64

	snap domain.LiveSnapshot
	hist []domain.LiveSnapshot // last histLen snapshots for graphs

	pending map[dayKey]*dayDelta // not yet flushed to daily tables

	store ports.StatsStore
	blog  ports.BlockedLogStore
	log   *slog.Logger
	ctx   context.Context
}

func newAggregator(store ports.StatsStore, blocked ports.BlockedLogStore, log *slog.Logger) *aggregator {
	return &aggregator{
		conns:   make(map[int64]*connEntry),
		pending: make(map[dayKey]*dayDelta),
		store:   store,
		blog:    blocked,
		log:     log,
	}
}

func (a *aggregator) run(ctx context.Context) {
	a.ctx = ctx
	t := time.NewTicker(time.Second)
	flush := time.NewTicker(flushEvery)
	defer t.Stop()
	defer flush.Stop()
	for {
		select {
		case <-ctx.Done():
			a.tick()
			a.flushDaily(time.Now())
			return
		case <-t.C:
			a.tick()
		case <-flush.C:
			a.flushDaily(time.Now())
		}
	}
}

func (a *aggregator) register(c domain.Connection) int64 {
	id := a.nextID.Add(1)
	c.ID = id
	c.Status = domain.ConnActive
	c.StartedAt = time.Now()
	e := &connEntry{conn: c}
	a.mu.Lock()
	a.conns[id] = e
	a.mu.Unlock()
	return id
}

func (a *aggregator) add(id int64, up, down int64) {
	a.mu.Lock()
	e, ok := a.conns[id]
	a.mu.Unlock()
	if !ok {
		return
	}
	if up > 0 {
		e.up.Add(up)
	}
	if down > 0 {
		e.down.Add(down)
	}
}

func (a *aggregator) close(id int64, st domain.ConnStatus) {
	a.mu.Lock()
	e, ok := a.conns[id]
	if ok {
		delete(a.conns, id)
	}
	if ok {
		// Conn lifetime rolls into the daily tables at the next flush.
		d := a.pending[dayKey{e.conn.ExePath, e.conn.ProxyID}]
		if d == nil {
			d = &dayDelta{}
			a.pending[dayKey{e.conn.ExePath, e.conn.ProxyID}] = d
		}
		d.conns++
		if secs := int64(math.Ceil(time.Since(e.conn.StartedAt).Seconds())); secs > 0 {
			d.secs += secs
		}
	}
	a.mu.Unlock()
	if !ok {
		return
	}
	a.outcome(st)
}

func (a *aggregator) outcome(st domain.ConnStatus) {
	switch st {
	case domain.ConnBlocked:
		a.nBlocked.Add(1)
	case domain.ConnFailed:
		a.nFailed.Add(1)
	case domain.ConnClosed:
		a.nSucceeded.Add(1)
	}
	if a.store != nil && a.ctx != nil {
		ctx, cancel := context.WithTimeout(a.ctx, 2*time.Second)
		a.store.RecordOutcome(ctx, st)
		cancel()
	}
}

func (a *aggregator) recordBlocked(e domain.BlockedEntry) {
	a.outcome(domain.ConnBlocked)
	if a.blog != nil && a.ctx != nil {
		ctx, cancel := context.WithTimeout(a.ctx, 2*time.Second)
		a.blog.Log(ctx, e)
		cancel()
	}
}

func (a *aggregator) tick() {
	a.mu.Lock()
	defer a.mu.Unlock()
	snap := domain.LiveSnapshot{
		PerExe:   make(map[string]domain.FlowStats),
		PerProxy: make(map[string]domain.FlowStats),
	}
	for _, e := range a.conns {
		up, down := e.up.Load(), e.down.Load()
		dUp, dDown := up-e.lastUp, down-e.lastDown
		e.lastUp, e.lastDown = up, down
		e.conn.BytesUp, e.conn.BytesDown = up, down
		e.conn.DurationMS = time.Since(e.conn.StartedAt).Milliseconds()
		snap.ActiveConns++
		snap.TotalUpBps += dUp
		snap.TotalDownBps += dDown
		add := func(m map[string]domain.FlowStats, k string) {
			s := m[k]
			s.UpBps += dUp
			s.DownBps += dDown
			s.TotalUp += up
			s.TotalDown += down
			s.Conns++
			m[k] = s
		}
		add(snap.PerExe, e.conn.ExePath)
		add(snap.PerProxy, e.conn.ProxyID)
		if dUp > 0 || dDown > 0 {
			k := dayKey{e.conn.ExePath, e.conn.ProxyID}
			d := a.pending[k]
			if d == nil {
				d = &dayDelta{}
				a.pending[k] = d
			}
			d.up += dUp
			d.down += dDown
			if a.store != nil && a.ctx != nil {
				ctx, cancel := context.WithTimeout(a.ctx, 2*time.Second)
				a.store.AddFlowDelta(ctx, e.conn.ExePath, e.conn.ProxyID, dUp, dDown)
				cancel()
			}
		}
	}
	snap.BlockedCount = a.nBlocked.Load()
	snap.FailedCount = a.nFailed.Load()
	snap.SucceededCount = a.nSucceeded.Load()
	a.snap = snap
	a.hist = append(a.hist, snap)
	if len(a.hist) > histLen {
		a.hist = a.hist[len(a.hist)-histLen:]
	}
}

// flushDaily upserts the pending per-(exe,proxy) deltas into the daily
// tables with day = local YYYY-MM-DD of now.
func (a *aggregator) flushDaily(now time.Time) {
	a.mu.Lock()
	pending := a.pending
	a.pending = make(map[dayKey]*dayDelta)
	a.mu.Unlock()
	if len(pending) == 0 || a.store == nil {
		return
	}
	day := now.Format("2006-01-02")
	// Keys the DB rejected get their deltas back so nothing is lost; flushed
	// keys are deleted as we go.
	for k, d := range pending {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		err := a.store.UpsertExeDaily(ctx, domain.ExeDailyStats{
			Day: day, ExePath: k.exe, ProxyID: k.proxy,
			Down: d.down, Up: d.up, Conns: d.conns, Seconds: d.secs,
		})
		if err == nil && k.proxy != "" {
			err = a.store.UpsertProxyDaily(ctx, domain.ProxyDailyStats{
				Day: day, ProxyID: k.proxy,
				Down: d.down, Up: d.up, Conns: d.conns, Seconds: d.secs,
			})
		}
		cancel()
		if err == nil {
			delete(pending, k)
			continue
		}
		a.log.Warn("daily stats flush failed", slog.String("err", err.Error()))
	}
	if len(pending) > 0 {
		a.mu.Lock()
		for k, d := range pending {
			cur := a.pending[k]
			if cur == nil {
				a.pending[k] = d
			} else {
				cur.up += d.up
				cur.down += d.down
				cur.conns += d.conns
				cur.secs += d.secs
			}
		}
		a.mu.Unlock()
	}
}

func (a *aggregator) Snapshot() domain.LiveSnapshot {
	a.mu.Lock()
	defer a.mu.Unlock()
	s := a.snap
	s.PerExe = cloneStats(s.PerExe)
	s.PerProxy = cloneStats(s.PerProxy)
	return s
}

func (a *aggregator) History() []domain.LiveSnapshot {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]domain.LiveSnapshot(nil), a.hist...)
}

func (a *aggregator) ActiveConnections() []domain.Connection {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]domain.Connection, 0, len(a.conns))
	for _, e := range a.conns {
		c := e.conn
		c.BytesUp, c.BytesDown = e.up.Load(), e.down.Load()
		c.DurationMS = time.Since(c.StartedAt).Milliseconds()
		out = append(out, c)
	}
	return out
}

func cloneStats(m map[string]domain.FlowStats) map[string]domain.FlowStats {
	out := make(map[string]domain.FlowStats, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
