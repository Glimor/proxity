//go:build windows

package wfp

import (
	"context"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

func TestEmitReturnsWhileCallbackIsBlocked(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	callbackEntered := make(chan struct{})
	releaseCallback := make(chan struct{})
	p := newDropParser(nil, func(ev DropEvent) {
		if ev.FilterID == 1 {
			close(callbackEntered)
			<-releaseCallback
		}
	})
	workerDone := make(chan struct{})
	go func() {
		defer close(workerDone)
		p.process(ctx)
	}()
	defer func() {
		cancel()
		close(releaseCallback)
		<-workerDone
	}()

	makeEvent := func(id uint64) wtFwpmNetEvent0 {
		header := &wtFwpmNetEventHeader0{
			timeStamp:  windows.Filetime{LowDateTime: uint32(id)},
			ipProtocol: 17,
			remoteAddr: [16]byte{192, 0, 2, byte(id)},
			remotePort: uint16(id),
		}
		drop := &wtFwpmNetEventClassifyDrop0{filterId: id}
		return wtFwpmNetEvent0{
			_type:  cFWPM_NET_EVENT_TYPE_CLASSIFY_DROP,
			header: header,
			u:      unsafe.Pointer(drop),
		}
	}

	first := makeEvent(1)
	firstEmitted := make(chan struct{})
	go func() {
		p.emit(&first)
		close(firstEmitted)
	}()
	select {
	case <-callbackEntered:
	case <-time.After(time.Second):
		t.Fatal("callback did not start")
	}
	select {
	case <-firstEmitted:
	case <-time.After(time.Second):
		t.Fatal("emit waited for the callback")
	}

	for id := uint64(2); id <= dropEventQueueCapacity+2; id++ {
		event := makeEvent(id)
		p.emit(&event)
	}
	if got := len(p.events); got != dropEventQueueCapacity {
		t.Fatalf("queued %d events, want %d", got, dropEventQueueCapacity)
	}
	if got := p.dropped.Load(); got != 1 {
		t.Fatalf("dropped %d events, want 1", got)
	}
}
