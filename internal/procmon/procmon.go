//go:build windows

// Package procmon attributes outbound TCP connections to a PID and exe path.
// A WinDivert SOCKET-layer sniff handle tracks BIND/CONNECT/CLOSE events into
// a (localAddr, localPort) -> pid table; GetExtendedTcpTable is the lazy
// fallback for events missed during startup or races.
package procmon

import (
	"context"
	"log/slog"
	"net/netip"
	"os"
	"sync"
	"unsafe"

	"github.com/Glimor/proxity/internal/domain"
	"github.com/Glimor/proxity/internal/windivert"
	"golang.org/x/sys/windows"
)

type sockKey struct {
	addr netip.Addr
	port uint16
}

// Monitor keeps the (addr,port)->pid table and a pid->exe path cache.
// The zero value is usable: Lookup falls back to the TCP table even when the
// sniff handle failed to open.
type Monitor struct {
	mu      sync.RWMutex
	bySock  map[sockKey]uint32
	byPort  map[uint16]uint32
	paths   map[uint32]string // pid -> casefolded canonical exe path
	pathErr map[uint32]bool

	h   *windivert.Handle
	log *slog.Logger

	// OnClose is invoked (on the sniff goroutine) for SOCKET_CLOSE events so
	// the engine can reap flows early.
	OnClose func(addr netip.Addr, port uint16)
}

func New(log *slog.Logger) *Monitor {
	return &Monitor{
		bySock:  make(map[sockKey]uint32),
		byPort:  make(map[uint16]uint32),
		paths:   make(map[uint32]string),
		pathErr: make(map[uint32]bool),
		log:     log,
	}
}

// Start seeds the process cache and opens the SOCKET-layer sniff handle. A
// nil error still leaves the TCP-table fallback functional; a sniff failure is
// logged but not fatal.
func (m *Monitor) Start(ctx context.Context) error {
	m.seedProcesses()
	h, err := windivert.Open("tcp", windivert.LayerSocket, 0,
		windivert.FlagSniff|windivert.FlagRecvOnly)
	if err != nil {
		m.log.Warn("procmon sniff handle failed, using TCP table fallback",
			slog.String("err", err.Error()))
		return err
	}
	m.h = h
	go m.sniffLoop(ctx)
	return nil
}

func (m *Monitor) Close() {
	if m.h != nil {
		m.h.Shutdown(windivert.ShutdownBoth)
		m.h.Close()
	}
}

func (m *Monitor) sniffLoop(ctx context.Context) {
	var addr windivert.Address
	buf := make([]byte, 1)
	for {
		if ctx.Err() != nil {
			return
		}
		if _, err := m.h.Recv(buf, &addr); err != nil {
			if ctx.Err() == nil {
				m.log.Error("procmon recv failed", slog.String("err", err.Error()))
			}
			return
		}
		pid := addr.ProcessID()
		local := addr.LocalAddr()
		port := addr.LocalPort()
		switch addr.Event() {
		case windivert.EventSocketBind, windivert.EventSocketConnect:
			if port != 0 && pid != 0 {
				m.put(sockKey{local, port}, port, pid)
			}
		case windivert.EventSocketClose:
			m.del(local, port)
			if m.OnClose != nil {
				m.OnClose(local, port)
			}
		}
	}
}

func (m *Monitor) put(k sockKey, port uint16, pid uint32) {
	m.mu.Lock()
	m.bySock[k] = pid
	m.byPort[port] = pid
	m.mu.Unlock()
}

func (m *Monitor) del(addr netip.Addr, port uint16) {
	m.mu.Lock()
	delete(m.bySock, sockKey{addr.Unmap(), port})
	delete(m.byPort, port)
	m.mu.Unlock()
}

// Lookup resolves (localAddr, localPort) to (pid, folded exe path). A port-only
// lookup is the second chance (wildcard binds), the extended TCP table the
// third.
func (m *Monitor) Lookup(addr netip.Addr, port uint16) (int, string, bool) {
	addr = addr.Unmap()
	m.mu.RLock()
	pid, ok := m.bySock[sockKey{addr, port}]
	if !ok {
		pid, ok = m.byPort[port]
	}
	m.mu.RUnlock()
	if !ok {
		pid, ok = lookupTCPTable(addr, port)
		if ok {
			m.put(sockKey{addr, port}, port, pid)
		}
	}
	if !ok {
		return 0, "", false
	}
	path := m.ProcessPath(int(pid))
	return int(pid), path, true
}

// ProcessPath returns the casefolded canonical exe path for pid, cached.
// Returns "" when the process cannot be opened (exited, protected, etc.).
func (m *Monitor) ProcessPath(pid int) string {
	if pid <= 0 {
		return ""
	}
	k := uint32(pid)
	m.mu.RLock()
	if p, ok := m.paths[k]; ok {
		m.mu.RUnlock()
		return p
	}
	if m.pathErr[k] {
		m.mu.RUnlock()
		return ""
	}
	m.mu.RUnlock()

	path := queryProcessImage(k)
	if path == "" {
		m.mu.Lock()
		m.pathErr[k] = true
		m.mu.Unlock()
		return ""
	}
	if c, err := domain.CanonicalPath(path); err == nil {
		path = c
	}
	fold := domain.FoldPath(path)
	m.mu.Lock()
	m.paths[k] = fold
	m.mu.Unlock()
	return fold
}

func queryProcessImage(pid uint32) string {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(h)
	buf := make([]uint16, 32768)
	n := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(h, 0, &buf[0], &n); err != nil {
		return ""
	}
	return windows.UTF16ToString(buf[:n])
}

// seedProcesses snapshots running PIDs once and resolves their exe paths in
// the background, so early SYNs hit a warm cache.
func (m *Monitor) seedProcesses() {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return
	}
	var e windows.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	var pids []uint32
	if err := windows.Process32First(snap, &e); err == nil {
		for {
			pids = append(pids, e.ProcessID)
			if err := windows.Process32Next(snap, &e); err != nil {
				break
			}
		}
	}
	windows.CloseHandle(snap)
	m.ProcessPath(os.Getpid())
	go func() {
		for _, pid := range pids {
			m.ProcessPath(int(pid))
		}
	}()
}
