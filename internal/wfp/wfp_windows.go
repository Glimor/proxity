//go:build windows

// Package wfp is Proxity's minimal Windows Filtering Platform binding layer
// (fwpuclnt.dll, pure Go syscalls, no cgo). Struct layouts and call patterns
// are adapted from the MIT-licensed WireGuard for Windows firewall package;
// see the NOTICE headers in types_windows.go and syscall_windows.go.
//
// Lifetime model (the fail-closed property the kill switch relies on):
// filters added without the PERSISTENT flag inside a DYNAMIC session are
// revoked by the base filtering engine the instant the session (or the whole
// process) dies. Filters marked FWPM_FILTER_FLAG_PERSISTENT survive. So the
// kill switch keeps its BLOCK filters persistent and its PERMIT filters
// dynamic: if the service crashes, the permits vanish and the blocks remain.
package wfp

import (
	"encoding/binary"
	"fmt"
	"net/netip"
	"runtime"
	"sync"
	"unsafe"

	"github.com/google/uuid"
	"golang.org/x/sys/windows"
)

const txnWaitTimeout = 10000

// Fixed object GUIDs. They must never change: persistent WFP objects key off
// them and reinstall/uninstall rely on stable identity.
var (
	ProviderGUID = uuid.MustParse("7c2e8f4a-1b3d-4e5f-9a6c-8d0e2f4a6b8c")
	SublayerGUID = uuid.MustParse("a1d3e5f7-9b2c-4d6e-8f0a-1c3e5b7d9f0a")
)

// FilterSpec is the backend-agnostic description of one WFP filter. Both the
// ALE_AUTH_CONNECT_V4 and ALE_AUTH_CONNECT_V6 variants are separate Specs
// (IPv6 flag picks the layer).
type FilterSpec struct {
	Key        uuid.UUID // caller-chosen deterministic key (enables idempotent adds)
	Name       string    // display name
	IPv6       bool
	Persistent bool // survives session/process death
	Weight     uint64
	Block      bool // false = PERMIT

	AppID      []byte         // nil matches any process
	TCPOnly    bool           // restrict to TCP
	UDPOnly    bool           // restrict to UDP
	RemotePort uint16         // 0 matches any remote port
	Remote     []netip.Prefix // OR'ed remote-address conditions
	// NoLoopback excludes loopback traffic (FWPM_CONDITION_FLAGS /
	// FWP_CONDITION_FLAG_IS_LOOPBACK must be *none* set).
	NoLoopback bool
	// RecvAccept installs the filter at ALE_AUTH_RECV_ACCEPT instead of
	// ALE_AUTH_CONNECT (inbound authorize layer — used for the self permit
	// covering the transparent redirect's reflected SYNs).
	RecvAccept bool
}

// Engine wraps one WFP engine handle opened in a dynamic session.
type Engine struct {
	mu sync.Mutex
	// txnMu serializes Txn bodies. It is separate from mu because the ops a
	// transaction runs (AddFilter/DeleteFilter) lock mu themselves — a plain
	// mutex is not reentrant and would self-deadlock.
	txnMu sync.Mutex
	h     uintptr
	name  string
	// flags are OR'ed into the FWPM_SESSION0 flags at open time.
	flags wtFwpmSessionFlagsValue
	// ids maps WFP filter IDs (returned by FilterAdd) back to our
	// deterministic keys so net events can be attributed to our filters.
	ids map[uint64]uuid.UUID
}

// Open opens a WFP engine handle inside a FWPM_SESSION_FLAG_DYNAMIC session:
// every non-persistent object added through it dies with the session.
func Open(sessionName string) (*Engine, error) {
	e := &Engine{name: sessionName, flags: cFWPM_SESSION_FLAG_DYNAMIC}
	if err := e.open(); err != nil {
		return nil, err
	}
	return e, nil
}

// OpenEventEngine opens a session that collects net events
// (FWPM_SESSION_FLAG_COLLECT_NET_EVENTS) so FwpmNetEventSubscribe0 /
// FwpmNetEventEnum0 work on it. It holds no filters of its own; the
// DYNAMIC flag keeps anything it does create session-scoped.
func OpenEventEngine(sessionName string) (*Engine, error) {
	// FwpmEngineSetOption0 is rejected on dynamic sessions
	// (FWP_E_DYNAMIC_SESSION_IN_PROGRESS), and this session creates no
	// objects, so it is opened non-dynamic.
	e := &Engine{name: sessionName}
	if err := e.open(); err != nil {
		return nil, err
	}
	// Net-event collection is an engine option (FwpmEngineSetOption0), not a
	// session flag. Passing it in FWPM_SESSION0.flags fails with
	// FWP_E_INVALID_FLAGS.
	on := wtFwpValue0{_type: cFWP_UINT32, value: 1}
	if err := fwpmEngineSetOption0(e.h, cFWPM_ENGINE_COLLECT_NET_EVENTS, &on); err != nil {
		e.closeLocked()
		return nil, fmt.Errorf("enable net-event collection: %w", err)
	}
	return e, nil
}

func (e *Engine) open() error {
	if err := preflight(); err != nil {
		return fmt.Errorf("wfp unavailable: %w", err)
	}
	dd, err := displayData(e.name, e.name+" dynamic session")
	if err != nil {
		return err
	}
	session := wtFwpmSession0{
		displayData:          *dd,
		flags:                e.flags,
		txnWaitTimeoutInMSec: txnWaitTimeout,
	}
	var h uintptr
	err = fwpmEngineOpen0(nil, cRPC_C_AUTHN_WINNT, nil, &session, unsafe.Pointer(&h))
	runtime.KeepAlive(dd)
	if err != nil {
		return err
	}
	e.h = h
	return nil
}

// Close ends the session; all non-persistent objects added through it are
// revoked by the base filtering engine.
func (e *Engine) Close() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.closeLocked()
}

func (e *Engine) closeLocked() {
	if e.h != 0 {
		fwpmEngineClose0(e.h)
		e.h = 0
	}
}

// Reopen closes the current session and opens a fresh one. This is the
// fail-closed primitive: dynamic PERMITs vanish on Close and the new session
// starts empty until filters are re-added.
func (e *Engine) Reopen() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.closeLocked()
	return e.open()
}

// Txn runs fn inside a WFP transaction, aborting on error.
func (e *Engine) Txn(fn func() error) error {
	e.txnMu.Lock()
	defer e.txnMu.Unlock()

	e.mu.Lock()
	h := e.h
	e.mu.Unlock()
	if h == 0 {
		return fmt.Errorf("wfp: engine closed")
	}

	if err := fwpmTransactionBegin0(h, 0); err != nil {
		return err
	}
	if err := fn(); err != nil {
		fwpmTransactionAbort0(h)
		return err
	}
	if err := fwpmTransactionCommit0(h); err != nil {
		fwpmTransactionAbort0(h)
		return err
	}
	return nil
}

// EnsureBase installs our persistent provider and sublayer once. Both calls
// are idempotent: an existing object with the same key is left in place.
func (e *Engine) EnsureBase(providerName, sublayerName string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.ensureBaseLocked(providerName, sublayerName)
}

func (e *Engine) ensureBaseLocked(providerName, sublayerName string) error {
	dd, err := displayData(providerName, "Proxity kill-switch provider")
	if err != nil {
		return err
	}
	provider := wtFwpmProvider0{
		providerKey: toWinGUID(ProviderGUID),
		displayData: *dd,
		flags:       cFWPM_PROVIDER_FLAG_PERSISTENT,
	}
	err = fwpmProviderAdd0(e.h, &provider, 0)
	runtime.KeepAlive(dd)
	if err != nil && !isAlreadyExistsErr(err) {
		return err
	}

	sd, err := displayData(sublayerName, "Proxity permit/block filters")
	if err != nil {
		return err
	}
	pk := toWinGUID(ProviderGUID)
	sublayer := wtFwpmSublayer0{
		subLayerKey: toWinGUID(SublayerGUID),
		displayData: *sd,
		flags:       cFWPM_SUBLAYER_FLAG_PERSISTENT,
		providerKey: &pk,
		weight:      ^uint16(0), // highest sublayer precedence
	}
	err = fwpmSubLayerAdd0(e.h, &sublayer, 0)
	runtime.KeepAlive(sd)
	if err != nil && !isAlreadyExistsErr(err) {
		return err
	}
	return nil
}

// Reset drops our sublayer (which removes every filter under it, persistent
// or not) and recreates provider+sublayer. Used at startup so leftover
// persistent filters from a previous run can't accumulate.
func (e *Engine) Reset(providerName, sublayerName string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	sk := toWinGUID(SublayerGUID)
	if err := fwpmSubLayerDeleteByKey0(e.h, &sk); err != nil && !isNotFoundErr(err) {
		return err
	}
	e.ids = nil // dropping the sublayer deleted every filter under it
	return e.ensureBaseLocked(providerName, sublayerName)
}

// FilterIDs returns a snapshot of the BFE-assigned filter ID -> our key
// map, used to attribute FWPM_CLASSIFY_DROP net events to our filters.
func (e *Engine) FilterIDs() map[uint64]uuid.UUID {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make(map[uint64]uuid.UUID, len(e.ids))
	for id, k := range e.ids {
		out[id] = k
	}
	return out
}

// AddFilter installs one filter. An existing filter with the same key is
// treated as success so reconcile is idempotent.
func (e *Engine) AddFilter(s FilterSpec) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	var keep []any
	defer func() { runtime.KeepAlive(keep) }()

	dd, err := displayData(s.Name, "Proxity kill-switch filter")
	if err != nil {
		return err
	}
	keep = append(keep, dd)

	conds, err := e.conditions(s, &keep)
	if err != nil {
		return err
	}

	f := wtFwpmFilter0{
		filterKey:   toWinGUID(s.Key),
		displayData: *dd,
		providerKey: ptr(toWinGUID(ProviderGUID)),
		subLayerKey: toWinGUID(SublayerGUID),
		weight: wtFwpValue0{
			_type: cFWP_UINT64,
			value: uintptr(s.Weight),
		},
		numFilterConditions: uint32(len(conds)),
		action:              wtFwpmAction0{_type: cFWP_ACTION_PERMIT},
	}
	switch {
	case s.RecvAccept && s.IPv6:
		f.layerKey = cFWPM_LAYER_ALE_AUTH_RECV_ACCEPT_V6
	case s.RecvAccept:
		f.layerKey = cFWPM_LAYER_ALE_AUTH_RECV_ACCEPT_V4
	case s.IPv6:
		f.layerKey = cFWPM_LAYER_ALE_AUTH_CONNECT_V6
	default:
		f.layerKey = cFWPM_LAYER_ALE_AUTH_CONNECT_V4
	}
	if s.Persistent {
		f.flags = cFWPM_FILTER_FLAG_PERSISTENT
	}
	if s.Block {
		f.action._type = cFWP_ACTION_BLOCK
	}
	if len(conds) > 0 {
		f.filterCondition = &conds[0]
	}

	var id uint64
	err = fwpmFilterAdd0(e.h, &f, 0, &id)
	runtime.KeepAlive(conds)
	if err != nil && isAlreadyExistsErr(err) {
		// Fetch the ID the BFE already assigned so event attribution works.
		id, _ = e.filterIDLocked(s.Key)
		err = nil
	}
	if err != nil {
		return err
	}
	if id != 0 {
		if e.ids == nil {
			e.ids = make(map[uint64]uuid.UUID)
		}
		e.ids[id] = s.Key
	}
	return nil
}

// filterIDLocked resolves the runtime ID of an installed filter. The
// returned *wtFwpmFilter0 is freed before returning.
func (e *Engine) filterIDLocked(key uuid.UUID) (uint64, error) {
	k := toWinGUID(key)
	var fp *wtFwpmFilter0
	if err := fwpmFilterGetByKey0(e.h, &k, &fp); err != nil {
		return 0, err
	}
	defer fwpmFreeMemory0(unsafe.Pointer(fp))
	return fp.filterID, nil
}

func (e *Engine) conditions(s FilterSpec, keep *[]any) ([]wtFwpmFilterCondition0, error) {
	var conds []wtFwpmFilterCondition0

	if len(s.AppID) > 0 {
		blob := &wtFwpByteBlob{size: uint32(len(s.AppID)), data: &s.AppID[0]}
		*keep = append(*keep, blob)
		conds = append(conds, wtFwpmFilterCondition0{
			fieldKey:  cFWPM_CONDITION_ALE_APP_ID,
			matchType: cFWP_MATCH_EQUAL,
			conditionValue: wtFwpConditionValue0{
				_type: cFWP_BYTE_BLOB_TYPE,
				value: uintptr(unsafe.Pointer(blob)),
			},
		})
	}

	if s.TCPOnly || s.UDPOnly {
		proto := uintptr(cIPPROTO_TCP)
		if s.UDPOnly {
			proto = cIPPROTO_UDP
		}
		conds = append(conds, wtFwpmFilterCondition0{
			fieldKey:  cFWPM_CONDITION_IP_PROTOCOL,
			matchType: cFWP_MATCH_EQUAL,
			conditionValue: wtFwpConditionValue0{
				_type: cFWP_UINT8,
				value: proto,
			},
		})
	}

	if s.RemotePort != 0 {
		conds = append(conds, wtFwpmFilterCondition0{
			fieldKey:  cFWPM_CONDITION_IP_REMOTE_PORT,
			matchType: cFWP_MATCH_EQUAL,
			conditionValue: wtFwpConditionValue0{
				_type: cFWP_UINT16,
				value: uintptr(s.RemotePort),
			},
		})
	}

	if s.NoLoopback {
		conds = append(conds, wtFwpmFilterCondition0{
			fieldKey:  cFWPM_CONDITION_FLAGS,
			matchType: cFWP_MATCH_FLAGS_NONE_SET,
			conditionValue: wtFwpConditionValue0{
				_type: cFWP_UINT32,
				value: uintptr(cFWP_CONDITION_FLAG_IS_LOOPBACK),
			},
		})
	}

	for _, p := range s.Remote {
		a := p.Masked().Addr().Unmap()
		// A V4_ADDR_MASK on the v6 layer (and vice versa) is rejected;
		// prefixes are split per family.
		if a.Is4() == s.IPv6 {
			continue
		}
		bits := p.Bits()
		if p.Addr().Is4In6() {
			bits -= 96 // mapped prefix length counts the 96-bit v6 prefix
		}
		conds = append(conds, prefixCondition(a, bits, keep))
	}
	return conds, nil
}

func prefixCondition(a netip.Addr, bits int, keep *[]any) wtFwpmFilterCondition0 {
	c := wtFwpmFilterCondition0{
		fieldKey:  cFWPM_CONDITION_IP_REMOTE_ADDRESS,
		matchType: cFWP_MATCH_EQUAL,
	}
	if a.Is4() {
		a4 := a.As4()
		m := &wtFwpV4AddrAndMask{
			addr: binary.BigEndian.Uint32(a4[:]),
			mask: prefixMask4(bits),
		}
		*keep = append(*keep, m)
		c.conditionValue = wtFwpConditionValue0{
			_type: cFWP_V4_ADDR_MASK,
			value: uintptr(unsafe.Pointer(m)),
		}
		return c
	}
	m := &wtFwpV6AddrAndMask{addr: a.As16(), prefixLength: uint8(bits)}
	*keep = append(*keep, m)
	c.conditionValue = wtFwpConditionValue0{
		_type: cFWP_V6_ADDR_MASK,
		value: uintptr(unsafe.Pointer(m)),
	}
	return c
}

func prefixMask4(bits int) uint32 {
	if bits <= 0 {
		return 0
	}
	if bits >= 32 {
		return 0xffffffff
	}
	return 0xffffffff << (32 - bits)
}

// DeleteFilter removes a filter by key; not-found is success (idempotent).
func (e *Engine) DeleteFilter(key uuid.UUID) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	k := toWinGUID(key)
	err := fwpmFilterDeleteByKey0(e.h, &k)
	if err != nil && isNotFoundErr(err) {
		return nil
	}
	if err == nil {
		for id, kk := range e.ids {
			if kk == key {
				delete(e.ids, id)
			}
		}
	}
	return err
}

// RemoveProvider deletes our sublayer (cascading to its filters) and provider.
// Opens its own short-lived dynamic session; used by -uninstall.
func RemoveProvider() error {
	eng, err := Open("Proxity uninstall")
	if err != nil {
		return err
	}
	defer eng.Close()

	return eng.Txn(func() error {
		sk := toWinGUID(SublayerGUID)
		if err := fwpmSubLayerDeleteByKey0(eng.h, &sk); err != nil && !isNotFoundErr(err) {
			return err
		}
		pk := toWinGUID(ProviderGUID)
		if err := fwpmProviderDeleteByKey0(eng.h, &pk); err != nil && !isNotFoundErr(err) {
			return err
		}
		return nil
	})
}

func displayData(name, desc string) (*wtFwpmDisplayData0, error) {
	n, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return nil, err
	}
	d, err := windows.UTF16PtrFromString(desc)
	if err != nil {
		return nil, err
	}
	return &wtFwpmDisplayData0{name: n, description: d}, nil
}

func ptr[T any](v T) *T { return &v }

func toWinGUID(u uuid.UUID) windows.GUID {
	return windows.GUID{
		Data1: binary.LittleEndian.Uint32(u[0:4]),
		Data2: binary.LittleEndian.Uint16(u[4:6]),
		Data3: binary.LittleEndian.Uint16(u[6:8]),
		Data4: [8]byte(u[8:16]),
	}
}
