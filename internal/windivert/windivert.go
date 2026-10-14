//go:build windows

// Package windivert is a minimal LazyDLL binding to WinDivert.dll (v2.2.x).
// Only the functions the redirect engine needs are wrapped; packet parsing
// lives in internal/netpkt.
package windivert

import (
	"encoding/binary"
	"errors"
	"net/netip"
	"os"
	"path/filepath"
	"unsafe"

	"github.com/Glimor/proxity/internal/domain"
	"golang.org/x/sys/windows"
)

type Layer uint32

const (
	LayerNetwork Layer = 0
	LayerSocket  Layer = 3
)

const (
	FlagSniff    uint64 = 0x0001
	FlagDrop     uint64 = 0x0002
	FlagRecvOnly uint64 = 0x0004
	FlagSendOnly uint64 = 0x0008
)

const (
	ParamQueueLength = 0
	ParamQueueTime   = 1
	ParamQueueSize   = 2
)

const (
	ShutdownRecv = 0x1
	ShutdownSend = 0x2
	ShutdownBoth = 0x3
)

// Socket-layer event values from windivert.h (verified against v2.2.2).
const (
	EventPacket          = 0
	EventFlowEstablished = 1
	EventFlowDeleted     = 2
	EventSocketBind      = 3
	EventSocketConnect   = 4
	EventSocketListen    = 5
	EventSocketAccept    = 6
	EventSocketClose     = 7
	EventReflectOpen     = 8
	EventReflectClose    = 9
)

const invalidHandle = ^uintptr(0)

// Address mirrors WINDIVERT_ADDRESS (80 bytes).
//
//	INT64 Timestamp @0; UINT32 bitfield @8 (Layer:8 Event:8 Sniffed:1
//	Outbound:1 Loopback:1 Impostor:1 IPv6:1 IPChecksum:1 TCPChecksum:1
//	UDPChecksum:1 Reserved:8); UINT32 Reserved @12; BYTE union[64] @16.
type Address struct {
	Timestamp int64
	Bits      uint32
	Reserved  uint32
	Data      [64]byte
}

func (a *Address) Layer() Layer      { return Layer(a.Bits & 0xff) }
func (a *Address) Event() uint32     { return (a.Bits >> 8) & 0xff }
func (a *Address) Sniffed() bool     { return a.Bits&(1<<16) != 0 }
func (a *Address) Outbound() bool    { return a.Bits&(1<<17) != 0 }
func (a *Address) Loopback() bool    { return a.Bits&(1<<18) != 0 }
func (a *Address) Impostor() bool    { return a.Bits&(1<<19) != 0 }
func (a *Address) IPv6() bool        { return a.Bits&(1<<20) != 0 }
func (a *Address) TCPChecksum() bool { return a.Bits&(1<<22) != 0 }

func (a *Address) SetOutbound(v bool) {
	if v {
		a.Bits |= 1 << 17
	} else {
		a.Bits &^= 1 << 17
	}
}

// SOCKET-layer union accessors (offsets relative to Data).
func (a *Address) ProcessID() uint32 { return binary.LittleEndian.Uint32(a.Data[16:20]) }

func (a *Address) LocalAddr() netip.Addr  { return unionAddr(a.Data[20:36]) }
func (a *Address) RemoteAddr() netip.Addr { return unionAddr(a.Data[36:52]) }

func (a *Address) LocalPort() uint16  { return binary.LittleEndian.Uint16(a.Data[52:54]) }
func (a *Address) RemotePort() uint16 { return binary.LittleEndian.Uint16(a.Data[54:56]) }
func (a *Address) Protocol() byte     { return a.Data[56] }

// unionAddr decodes the UINT32[4] address array used by the socket/flow union
// members: the address bytes are stored in network order; IPv4 uses only the
// first word.
func unionAddr(b []byte) netip.Addr {
	v6 := false
	for _, c := range b[4:16] {
		if c != 0 {
			v6 = true
			break
		}
	}
	if !v6 {
		return netip.AddrFrom4([4]byte{b[0], b[1], b[2], b[3]})
	}
	return netip.AddrFrom16([16]byte(b[:16])).Unmap()
}

type Handle struct {
	h windows.Handle
}

var (
	dll     *lazyDLL
	loadErr error
)

type lazyDLL struct{ d *windows.LazyDLL }

func (l *lazyDLL) proc(name string) *windows.LazyProc { return l.d.NewProc(name) }

func init() {
	path, err := findDLL()
	if err != nil {
		loadErr = err
		return
	}
	dll = &lazyDLL{d: windows.NewLazyDLL(path)}
	// Force the load so Load() errors surface early with context.
	if err := dll.d.Load(); err != nil {
		loadErr = domain.WrapError("windivert.load_failed",
			"WinDivert driver could not be loaded. Run as administrator and make sure WinDivert.dll and WinDivert64.sys sit next to proxity-service.exe or in third_party\\windivert.", err)
	}
}

// findDLL locates WinDivert.dll: $WINDIVERT_DLL, next to the executable,
// <exe>\..\third_party\windivert, <cwd>\third_party\windivert, then PATH.
func findDLL() (string, error) {
	if p := os.Getenv("WINDIVERT_DLL"); p != "" {
		return p, nil
	}
	var candidates []string
	if exe, err := os.Executable(); err == nil {
		ed := filepath.Dir(exe)
		candidates = append(candidates, filepath.Join(ed, "WinDivert.dll"),
			filepath.Join(ed, "third_party", "windivert", "WinDivert.dll"))
	}
	// Walk up from the working dir so repo-rooted third_party\windivert is
	// found from any package dir (tests, tools).
	if cwd, err := os.Getwd(); err == nil {
		for d, i := cwd, 0; i < 5; i++ {
			candidates = append(candidates,
				filepath.Join(d, "third_party", "windivert", "WinDivert.dll"),
				filepath.Join(d, "WinDivert.dll"))
			nd := filepath.Dir(d)
			if nd == d {
				break
			}
			d = nd
		}
	}
	for _, p := range candidates {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p, nil
		}
	}
	return "", domain.NewError("windivert.not_found",
		"WinDivert.dll not found. Place WinDivert.dll and WinDivert64.sys next to proxity-service.exe or in third_party\\windivert.", "")
}

// Available reports whether the DLL could be located and loaded.
func Available() error {
	if loadErr != nil {
		return loadErr
	}
	if dll == nil {
		return domain.NewError("windivert.not_found", "WinDivert.dll not found", "")
	}
	return dll.d.Load()
}

// Open wraps WinDivertOpen. Common failures are mapped to friendly AppErrors.
func Open(filter string, layer Layer, priority int16, flags uint64) (*Handle, error) {
	if err := Available(); err != nil {
		return nil, err
	}
	fs, err := windows.BytePtrFromString(filter)
	if err != nil {
		return nil, err
	}
	r, _, e := dll.proc("WinDivertOpen").Call(
		uintptr(unsafe.Pointer(fs)),
		uintptr(uint32(layer)),
		uintptr(uint16(priority)),
		uintptr(flags),
	)
	if r == 0 || r == invalidHandle {
		return nil, openError(e)
	}
	return &Handle{h: windows.Handle(r)}, nil
}

func openError(err error) error {
	var errno windows.Errno
	if errors.As(err, &errno) {
		switch errno {
		case windows.ERROR_FILE_NOT_FOUND, windows.ERROR_PATH_NOT_FOUND:
			return domain.NewError("windivert.driver_missing",
				"WinDivert driver files missing. Put WinDivert64.sys next to WinDivert.dll.", errno.Error())
		case windows.ERROR_ACCESS_DENIED:
			return domain.NewError("windivert.access_denied",
				"WinDivert needs administrator rights. Run proxity-service elevated.", errno.Error())
		case windows.ERROR_INVALID_PARAMETER:
			return domain.NewError("windivert.bad_filter",
				"WinDivert rejected the capture filter.", errno.Error())
		}
	}
	return domain.WrapError("windivert.open_failed",
		"WinDivert driver could not be opened (admin rights required).", err)
}

func (h *Handle) SetParam(param int, value uint64) error {
	r, _, e := dll.proc("WinDivertSetParam").Call(uintptr(h.h), uintptr(param), uintptr(value))
	if r == 0 {
		return e
	}
	return nil
}

// Recv receives a single packet/event into buf and fills addr.
func (h *Handle) Recv(buf []byte, addr *Address) (int, error) {
	var n uint32
	r, _, e := dll.proc("WinDivertRecv").Call(
		uintptr(h.h),
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(len(buf)),
		uintptr(unsafe.Pointer(&n)),
		uintptr(unsafe.Pointer(addr)),
	)
	if r == 0 {
		return 0, e
	}
	return int(n), nil
}

// Send injects buf back at the direction encoded in addr.
func (h *Handle) Send(buf []byte, addr *Address) (int, error) {
	var n uint32
	r, _, e := dll.proc("WinDivertSend").Call(
		uintptr(h.h),
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(len(buf)),
		uintptr(unsafe.Pointer(&n)),
		uintptr(unsafe.Pointer(addr)),
	)
	if r == 0 {
		return 0, e
	}
	return int(n), nil
}

// Shutdown unblocks a pending Recv (how = ShutdownBoth).
func (h *Handle) Shutdown(how int) error {
	r, _, e := dll.proc("WinDivertShutdown").Call(uintptr(h.h), uintptr(how))
	if r == 0 {
		return e
	}
	return nil
}

func (h *Handle) Close() error {
	r, _, e := dll.proc("WinDivertClose").Call(uintptr(h.h))
	if r == 0 {
		return e
	}
	return nil
}
