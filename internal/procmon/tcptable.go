//go:build windows

package procmon

import (
	"encoding/binary"
	"net/netip"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var iphlpapi = windows.NewLazySystemDLL("iphlpapi.dll")

const (
	afInet           = 2
	afInet6          = 23
	tcpTableOwnerPID = 5 // TCP_TABLE_OWNER_PID_ALL
	tcpStateSynSent  = 3
	tcpStateEstab    = 5
	errInsufficient  = 122 // ERROR_INSUFFICIENT_BUFFER
	tcpTableCacheTTL = 300 * time.Millisecond
)

var (
	tcpMu      sync.Mutex
	tcpCacheAt time.Time
	tcpBySock  map[sockKey]uint32
)

// lookupTCPTable resolves (addr,port)->pid via GetExtendedTcpTable. The table
// is rebuilt at most every tcpTableCacheTTL so rapid SYN bursts stay cheap.
func lookupTCPTable(addr netip.Addr, port uint16) (uint32, bool) {
	tcpMu.Lock()
	defer tcpMu.Unlock()
	if time.Since(tcpCacheAt) > tcpTableCacheTTL || tcpBySock == nil {
		tcpBySock = readTCPTable(afInet)
		for k, v := range readTCPTable(afInet6) {
			tcpBySock[k] = v
		}
		tcpCacheAt = time.Now()
	}
	if pid, ok := tcpBySock[sockKey{addr, port}]; ok {
		return pid, true
	}
	// Wildcard row: socket bound to 0.0.0.0/:: — the port still identifies it.
	for _, a := range []netip.Addr{netip.IPv4Unspecified(), netip.IPv6Unspecified()} {
		if pid, ok := tcpBySock[sockKey{a.Unmap(), port}]; ok {
			return pid, true
		}
	}
	return 0, false
}

func readTCPTable(af uint32) map[sockKey]uint32 {
	out := make(map[sockKey]uint32)
	proc := iphlpapi.NewProc("GetExtendedTcpTable")
	var size uint32
	r, _, _ := proc.Call(0, uintptr(unsafe.Pointer(&size)), 1, uintptr(af), tcpTableOwnerPID, 0)
	if r != errInsufficient || size == 0 {
		return out
	}
	buf := make([]byte, size)
	r, _, _ = proc.Call(
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(unsafe.Pointer(&size)),
		1, uintptr(af), tcpTableOwnerPID, 0)
	if r != 0 {
		return out
	}
	n := binary.LittleEndian.Uint32(buf[0:4])
	if af == afInet {
		// MIB_TCPROW_OWNER_PID: state, localAddr, localPort, remoteAddr, remotePort, pid
		const row = 24
		for i, off := uint32(0), 4; i < n && off+row <= len(buf); i, off = i+1, off+row {
			st := binary.LittleEndian.Uint32(buf[off:])
			if st != tcpStateSynSent && st != tcpStateEstab {
				continue
			}
			a := netip.AddrFrom4([4]byte{buf[off+4], buf[off+5], buf[off+6], buf[off+7]})
			port := nboPort(binary.LittleEndian.Uint32(buf[off+8:]))
			pid := binary.LittleEndian.Uint32(buf[off+20:])
			out[sockKey{a, port}] = pid
		}
		return out
	}
	// MIB_TCP6ROW_OWNER_PID: localAddr[16], remoteAddr[16], state, localPort, remotePort, pid
	const row6 = 52
	for i, off := uint32(0), 4; i < n && off+row6 <= len(buf); i, off = i+1, off+row6 {
		st := binary.LittleEndian.Uint32(buf[off+32:])
		if st != tcpStateSynSent && st != tcpStateEstab {
			continue
		}
		a := netip.AddrFrom16([16]byte(buf[off : off+16])).Unmap()
		port := nboPort(binary.LittleEndian.Uint32(buf[off+36:]))
		pid := binary.LittleEndian.Uint32(buf[off+48:])
		out[sockKey{a, port}] = pid
	}
	return out
}

// nboPort converts a port stored as the low bytes of a DWORD in network byte
// order to host order.
func nboPort(dw uint32) uint16 {
	p := uint16(dw)
	return p<<8 | p>>8
}
