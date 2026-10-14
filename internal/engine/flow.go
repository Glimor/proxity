//go:build windows

package engine

import (
	"net/netip"
	"sync"
	"time"

	"github.com/Glimor/proxity/internal/domain"
	"github.com/Glimor/proxity/internal/netpkt"
)

type tupleKey struct {
	src, dst     netip.Addr
	sport, dport uint16
}

type flow struct {
	natID       uint32 // 0 for pass-through flows (no NAT address needed)
	origSrc     netip.Addr
	origSrcPort uint16
	origDst     netip.Addr
	origDstPort uint16
	ipv6        bool
	pid         int
	exeFold     string
	action      domain.Decision // DecisionProxy | DecisionDirect
	ruleID      string
	proxyID     string
	connID      int64 // stats connection id, 0 for pass flows
	lastSeen    time.Time
	createdAt   time.Time
}

type flowTable struct {
	mu      sync.RWMutex
	byID    map[uint32]*flow
	byTuple map[tupleKey]*flow
	free    []uint32
	next    uint32

	v6Pool  bool   // encode v6 natID in a pool port instead of a ULA address
	poolMax uint32 // exclusive upper bound of pool ids
	v6Next  uint32
	v6Free  []uint32
}

func newFlowTable(v6Pool bool, poolSize int) *flowTable {
	t := &flowTable{
		byID:    make(map[uint32]*flow),
		byTuple: make(map[tupleKey]*flow),
		next:    1,
		v6Pool:  v6Pool,
		poolMax: uint32(poolSize),
		v6Next:  1,
	}
	if v6Pool {
		// v4 ids live above the pool so both families share byID safely.
		t.next = uint32(poolSize) + 1
	}
	return t
}

func (t *flowTable) allocID(ipv6 bool) (uint32, bool) {
	if ipv6 && t.v6Pool {
		if n := len(t.v6Free); n > 0 {
			id := t.v6Free[n-1]
			t.v6Free = t.v6Free[:n-1]
			return id, true
		}
		if t.v6Next <= t.poolMax {
			id := t.v6Next
			t.v6Next++
			return id, true
		}
		return 0, false
	}
	if n := len(t.free); n > 0 {
		id := t.free[n-1]
		t.free = t.free[:n-1]
		return id, true
	}
	if t.next <= netpkt.MaxNATID {
		id := t.next
		t.next++
		return id, true
	}
	return 0, false
}

// add inserts f; proxied flows (wantID) get a natID. Returns false when the id
// space is exhausted.
func (t *flowTable) add(f *flow, wantID bool) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if wantID {
		id, ok := t.allocID(f.ipv6)
		if !ok {
			return false
		}
		f.natID = id
		t.byID[id] = f
	}
	f.createdAt, f.lastSeen = time.Now(), time.Now()
	t.byTuple[tupleKey{f.origSrc, f.origDst, f.origSrcPort, f.origDstPort}] = f
	return true
}

func (t *flowTable) getByTuple(k tupleKey) *flow {
	t.mu.RLock()
	f := t.byTuple[k]
	t.mu.RUnlock()
	return f
}

func (t *flowTable) getByID(id uint32) *flow {
	t.mu.RLock()
	f := t.byID[id]
	t.mu.RUnlock()
	return f
}

func (t *flowTable) remove(f *flow) {
	t.mu.Lock()
	delete(t.byTuple, tupleKey{f.origSrc, f.origDst, f.origSrcPort, f.origDstPort})
	if f.natID != 0 {
		delete(t.byID, f.natID)
		if f.ipv6 && t.v6Pool {
			t.v6Free = append(t.v6Free, f.natID)
		} else {
			t.free = append(t.free, f.natID)
		}
	}
	t.mu.Unlock()
}

// removeByLocal drops flows whose original source is (addr,port) — used on
// socket CLOSE events.
func (t *flowTable) removeByLocal(addr netip.Addr, port uint16) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for k, f := range t.byTuple {
		if f.origSrc == addr && f.origSrcPort == port {
			delete(t.byTuple, k)
			if f.natID != 0 {
				delete(t.byID, f.natID)
				if f.ipv6 && t.v6Pool {
					t.v6Free = append(t.v6Free, f.natID)
				} else {
					t.free = append(t.free, f.natID)
				}
			}
		}
	}
}

func (t *flowTable) touch(f *flow) {
	t.mu.Lock()
	f.lastSeen = time.Now()
	t.mu.Unlock()
}

const staleAfter = 2 * time.Hour

// sweep drops idle flows; returns the removed ones so callers can close their
// stats connections.
func (t *flowTable) sweep() []*flow {
	cut := time.Now().Add(-staleAfter)
	t.mu.Lock()
	defer t.mu.Unlock()
	var out []*flow
	for k, f := range t.byTuple {
		if f.lastSeen.Before(cut) {
			delete(t.byTuple, k)
			if f.natID != 0 {
				delete(t.byID, f.natID)
				if f.ipv6 && t.v6Pool {
					t.v6Free = append(t.v6Free, f.natID)
				} else {
					t.free = append(t.free, f.natID)
				}
			}
			out = append(out, f)
		}
	}
	return out
}

func (t *flowTable) len() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return len(t.byTuple)
}
