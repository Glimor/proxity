//go:build windows

package wfp

import (
	"context"
	"log/slog"
	"net/netip"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// DropEvent is a parsed FWPM_NET_EVENT_TYPE_CLASSIFY_DROP record. AppID is
// the raw WFP application-ID blob (UTF-16 device path); use AppIDToPath to
// render it as a Win32 path.
type DropEvent struct {
	FilterID uint64
	AppID    []byte
	Dst      netip.Addr
	DstPort  uint16
	Proto    uint8
	TS       time.Time
}

const dropEventQueueCapacity = 256

// WatchDrops reports classify-drop events until ctx is cancelled or an
// unrecoverable error occurs. It opens a dedicated net-event session,
// first tries a real FwpmNetEventSubscribe0 subscription and falls back to
// polling FwpmNetEventEnum0 every `interval` if the subscription fails.
// owned(filterID) decides whether an event is attributed to Proxity —
// pass a lookup over Engine.FilterIDs() of the filter session.
//
// Events for other software are silently dropped. The function logs a
// warning and returns nil when neither mechanism is available so callers
// degrade gracefully instead of crashing the service.
func WatchDrops(ctx context.Context, log *slog.Logger, interval time.Duration,
	owned func(uint64) bool, cb func(DropEvent)) error {

	eng, err := OpenEventEngine("Proxity net events")
	if err != nil {
		if log != nil {
			log.Warn("net-event watch unavailable", slog.String("err", err.Error()))
		}
		return nil
	}
	defer eng.Close()

	p := newDropParser(owned, cb)
	workerCtx, cancelWorker := context.WithCancel(ctx)
	workerDone := make(chan struct{})
	go func() {
		defer close(workerDone)
		p.process(workerCtx)
	}()
	defer func() {
		cancelWorker()
		<-workerDone
	}()

	if unsub, err := p.subscribe(eng.h); err == nil {
		defer unsub()
		<-ctx.Done()
		return nil
	} else if log != nil {
		log.Warn("FwpmNetEventSubscribe0 failed, falling back to enum polling",
			slog.String("err", err.Error()))
	}

	return p.poll(ctx, eng.h, interval, log)
}

// dropParser carries the dedup state shared by the subscribe and enum paths.
type dropParser struct {
	owned   func(uint64) bool
	cb      func(DropEvent)
	events  chan DropEvent
	dropped atomic.Uint64
	mu      sync.Mutex
	lastTS  windows.Filetime
}

func newDropParser(owned func(uint64) bool, cb func(DropEvent)) *dropParser {
	return &dropParser{
		owned:  owned,
		cb:     cb,
		events: make(chan DropEvent, dropEventQueueCapacity),
	}
}

func (p *dropParser) process(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case ev := <-p.events:
			if p.owned != nil && !p.owned(ev.FilterID) {
				continue
			}
			if p.cb != nil {
				p.cb(ev)
			}
		}
	}
}

// filetimeGreater reports whether a sorts strictly after b (64-bit compare).
func filetimeGreater(a, b windows.Filetime) bool {
	return a.HighDateTime > b.HighDateTime ||
		(a.HighDateTime == b.HighDateTime && a.LowDateTime > b.LowDateTime)
}

// emit parses and queues classify-drop events. Events are deduplicated by
// timestamp: only events strictly newer than lastTS are queued.
func (p *dropParser) emit(ev *wtFwpmNetEvent0) {
	if ev == nil || ev._type != cFWPM_NET_EVENT_TYPE_CLASSIFY_DROP ||
		ev.header == nil || ev.u == nil {
		return
	}
	h := ev.header
	drop := (*wtFwpmNetEventClassifyDrop0)(ev.u)

	p.mu.Lock()
	if !filetimeGreater(h.timeStamp, p.lastTS) {
		p.mu.Unlock()
		return
	}
	p.lastTS = h.timeStamp
	p.mu.Unlock()

	var dst netip.Addr
	if h.ipVersion == cFWP_IP_VERSION_V6 {
		dst = netip.AddrFrom16(h.remoteAddr)
	} else {
		dst = netip.AddrFrom4([4]byte{h.remoteAddr[0], h.remoteAddr[1], h.remoteAddr[2], h.remoteAddr[3]})
	}
	var appID []byte
	if h.appId.data != nil && h.appId.size > 0 {
		appID = make([]byte, h.appId.size)
		copy(appID, unsafe.Slice(h.appId.data, h.appId.size))
	}
	parsed := DropEvent{
		FilterID: drop.filterId,
		AppID:    appID,
		Dst:      dst,
		DstPort:  h.remotePort,
		Proto:    h.ipProtocol,
		TS:       filetimeToTime(h.timeStamp),
	}
	select {
	case p.events <- parsed:
	default:
		p.dropped.Add(1)
	}
}

func filetimeToTime(ft windows.Filetime) time.Time {
	const ticksPerSec = 10_000_000
	const epochDelta = 11644473600 // FILETIME epoch (1601) -> Unix epoch
	ticks := uint64(ft.HighDateTime)<<32 | uint64(ft.LowDateTime)
	return time.Unix(int64(ticks/ticksPerSec)-epochDelta,
		int64(ticks%ticksPerSec)*100)
}

// subscribe attempts the push-based path. Returns an unsubscribe function.
func (p *dropParser) subscribe(engineHandle uintptr) (func(), error) {
	// Restrict the stream to drops whose remote port is 53? No — subscribe
	// with a minimal template (all net events): classification-drop events
	// are rare enough that parsing them in-process is cheap.
	sub := wtFwpmNetEventSubscription0{}

	cbThunk := windows.NewCallback(func(_ uintptr, ev *wtFwpmNetEvent0) uintptr {
		p.emit(ev)
		return 0
	})

	var eventsHandle uintptr
	err := fwpmNetEventSubscribe0(engineHandle, &sub, cbThunk, nil, unsafe.Pointer(&eventsHandle))
	if err != nil {
		return nil, err
	}
	return func() {
		fwpmNetEventUnsubscribe0(engineHandle, eventsHandle)
	}, nil
}

// poll enumerates net events every `interval` until ctx ends.
func (p *dropParser) poll(ctx context.Context, engineHandle uintptr,
	interval time.Duration, log *slog.Logger) error {

	// Only events from the last few seconds matter; using a startTime lets
	// the enum skip the historical backlog on the first pass.
	start := windows.Filetime{}
	tmpl := wtFwpmNetEventEnumTemplate0{
		startTime: start,
		endTime:   windows.Filetime{LowDateTime: 0xFFFFFFFF, HighDateTime: 0x7FFFFFFF}, // "far future"
	}
	var eh uintptr
	if err := fwpmNetEventCreateEnumHandle0(engineHandle, &tmpl, unsafe.Pointer(&eh)); err != nil {
		if log != nil {
			log.Warn("net-event enum handle failed; drop logging disabled",
				slog.String("err", err.Error()))
		}
		return nil
	}
	defer fwpmNetEventDestroyEnumHandle0(engineHandle, eh)

	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-t.C:
			p.enumOnce(engineHandle, eh, log)
		}
	}
}

func (p *dropParser) enumOnce(engineHandle, enumHandle uintptr, log *slog.Logger) {
	var entries *wtFwpmNetEvent0
	var n uint32
	if err := fwpmNetEventEnum0(engineHandle, enumHandle, 256,
		unsafe.Pointer(&entries), &n); err != nil {
		if log != nil {
			log.Warn("net-event enum failed", slog.String("err", err.Error()))
		}
		return
	}
	if n == 0 || entries == nil {
		return
	}
	defer fwpmFreeMemory0(&entries)
	for _, ev := range unsafe.Slice(entries, n) {
		p.emit(&ev)
	}
}
