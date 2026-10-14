//go:build windows

// Package netwatch watches for Windows network changes (interface up/down,
// unicast address add/remove) via iphlpapi Notify*Change callbacks and
// invokes a debounced callback so consumers can re-check proxies and
// reconcile the kill switch. Pure Go, no cgo: the notification callbacks
// ride windows.NewCallback thunks.
package netwatch

import (
	"context"
	"log/slog"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	modiphlpapi = windows.NewLazySystemDLL("iphlpapi.dll")

	procNotifyIpInterfaceChange      = modiphlpapi.NewProc("NotifyIpInterfaceChange")
	procNotifyUnicastIpAddressChange = modiphlpapi.NewProc("NotifyUnicastIpAddressChange")
	procCancelMibChangeNotify2       = modiphlpapi.NewProc("CancelMibChangeNotify2")
)

const afUnspecified = 0 // ADDRESS_FAMILY AF_UNSPEC — watch both families

// Watcher debounces interface/address change notifications into cb calls.
type Watcher struct {
	log     *slog.Logger
	delay   time.Duration
	cb      func()
	handles []uintptr // notification handles for CancelMibChangeNotify2
	thunks  []uintptr // windows.NewCallback thunks, kept for liveness

	mu    sync.Mutex
	timer *time.Timer
}

// Watch registers both notifications and returns a stop function. cb is
// invoked at most once per `delay` window no matter how many notifications
// arrive. Safe to call cb from the returned watcher context.
func Watch(log *slog.Logger, delay time.Duration, cb func()) (*Watcher, error) {
	w := &Watcher{log: log, delay: delay, cb: cb}

	ifaceCB := windows.NewCallback(func(_, _, _ uintptr) uintptr {
		w.changed("interface")
		return 0
	})
	addrCB := windows.NewCallback(func(_, _, _ uintptr) uintptr {
		w.changed("address")
		return 0
	})

	var h1, h2 uintptr
	r1, _, e1 := syscall.Syscall6(procNotifyIpInterfaceChange.Addr(), 5,
		afUnspecified, ifaceCB, 0, 0, uintptr(unsafe.Pointer(&h1)), 0)
	if r1 != 0 {
		return nil, errnoErr(e1)
	}
	r2, _, e2 := syscall.Syscall6(procNotifyUnicastIpAddressChange.Addr(), 5,
		afUnspecified, addrCB, 0, 0, uintptr(unsafe.Pointer(&h2)), 0)
	if r2 != 0 {
		cancelNotify(h1)
		return nil, errnoErr(e2)
	}
	w.handles = []uintptr{h1, h2}
	w.thunks = []uintptr{ifaceCB, addrCB}
	return w, nil
}

// changed arms/re-arms the debounce timer.
func (w *Watcher) changed(what string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.log != nil {
		w.log.Debug("netwatch: change notification", slog.String("kind", what))
	}
	if w.timer == nil {
		w.timer = time.AfterFunc(w.delay, w.cb)
		return
	}
	w.timer.Reset(w.delay)
}

// Close cancels the notifications and any pending debounce.
func (w *Watcher) Close() {
	w.mu.Lock()
	if w.timer != nil {
		w.timer.Stop()
	}
	w.mu.Unlock()
	for _, h := range w.handles {
		cancelNotify(h)
	}
	w.handles = nil
}

func cancelNotify(h uintptr) {
	syscall.Syscall(procCancelMibChangeNotify2.Addr(), 1, h, 0, 0)
}

func errnoErr(e syscall.Errno) error {
	if e == 0 {
		return syscall.EINVAL
	}
	return e
}

// Run blocks until ctx is cancelled, then closes the watcher. Convenience
// wrapper for a service goroutine.
func Run(ctx context.Context, log *slog.Logger, delay time.Duration, cb func()) error {
	w, err := Watch(log, delay, cb)
	if err != nil {
		return err
	}
	<-ctx.Done()
	w.Close()
	return nil
}
