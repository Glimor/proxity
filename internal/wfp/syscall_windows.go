//go:build windows

// NOTICE: the fwpuclnt.dll syscall wrappers in this file are adapted from the
// MIT-licensed WireGuard for Windows project (tunnel/firewall/zsyscall_windows.go).
//
// SPDX-License-Identifier: MIT
// Copyright (C) 2019-2021 WireGuard LLC. All Rights Reserved.

package wfp

import (
	"fmt"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	modfwpuclnt = windows.NewLazySystemDLL("fwpuclnt.dll")
	modkernel32 = windows.NewLazySystemDLL("kernel32.dll")

	procFwpmEngineClose0               = modfwpuclnt.NewProc("FwpmEngineClose0")
	procFwpmEngineOpen0                = modfwpuclnt.NewProc("FwpmEngineOpen0")
	procFwpmEngineSetOption0           = modfwpuclnt.NewProc("FwpmEngineSetOption0")
	procFwpmFilterAdd0                 = modfwpuclnt.NewProc("FwpmFilterAdd0")
	procFwpmFilterCreateEnumHandle0    = modfwpuclnt.NewProc("FwpmFilterCreateEnumHandle0")
	procFwpmFilterDeleteByKey0         = modfwpuclnt.NewProc("FwpmFilterDeleteByKey0")
	procFwpmFilterDestroyEnumHandle0   = modfwpuclnt.NewProc("FwpmFilterDestroyEnumHandle0")
	procFwpmFilterEnum0                = modfwpuclnt.NewProc("FwpmFilterEnum0")
	procFwpmFilterGetByKey0            = modfwpuclnt.NewProc("FwpmFilterGetByKey0")
	procFwpmFreeMemory0                = modfwpuclnt.NewProc("FwpmFreeMemory0")
	procFwpmGetAppIdFromFileName0      = modfwpuclnt.NewProc("FwpmGetAppIdFromFileName0")
	procFwpmNetEventCreateEnumHandle0  = modfwpuclnt.NewProc("FwpmNetEventCreateEnumHandle0")
	procFwpmNetEventDestroyEnumHandle0 = modfwpuclnt.NewProc("FwpmNetEventDestroyEnumHandle0")
	procFwpmNetEventEnum0              = modfwpuclnt.NewProc("FwpmNetEventEnum0")
	procFwpmNetEventSubscribe0         = modfwpuclnt.NewProc("FwpmNetEventSubscribe0")
	procFwpmNetEventUnsubscribe0       = modfwpuclnt.NewProc("FwpmNetEventUnsubscribe0")
	procFwpmProviderAdd0               = modfwpuclnt.NewProc("FwpmProviderAdd0")
	procFwpmProviderDeleteByKey0       = modfwpuclnt.NewProc("FwpmProviderDeleteByKey0")
	procFwpmSubLayerAdd0               = modfwpuclnt.NewProc("FwpmSubLayerAdd0")
	procFwpmSubLayerDeleteByKey0       = modfwpuclnt.NewProc("FwpmSubLayerDeleteByKey0")
	procFwpmTransactionAbort0          = modfwpuclnt.NewProc("FwpmTransactionAbort0")
	procFwpmTransactionBegin0          = modfwpuclnt.NewProc("FwpmTransactionBegin0")
	procFwpmTransactionCommit0         = modfwpuclnt.NewProc("FwpmTransactionCommit0")
	procQueryDosDeviceW                = modkernel32.NewProc("QueryDosDeviceW")
)

// requiredProcs lists every lazy proc the engine calls. Open runs preflight
// on them so a missing export becomes an error, not a LazyProc panic.
var requiredProcs = []*windows.LazyProc{
	procFwpmEngineClose0,
	procFwpmEngineOpen0,
	procFwpmEngineSetOption0,
	procFwpmFilterAdd0,
	procFwpmFilterCreateEnumHandle0,
	procFwpmFilterDeleteByKey0,
	procFwpmFilterDestroyEnumHandle0,
	procFwpmFilterEnum0,
	procFwpmFilterGetByKey0,
	procFwpmFreeMemory0,
	procFwpmGetAppIdFromFileName0,
	procFwpmNetEventCreateEnumHandle0,
	procFwpmNetEventDestroyEnumHandle0,
	procFwpmNetEventEnum0,
	procFwpmNetEventSubscribe0,
	procFwpmNetEventUnsubscribe0,
	procFwpmProviderAdd0,
	procFwpmProviderDeleteByKey0,
	procFwpmSubLayerAdd0,
	procFwpmSubLayerDeleteByKey0,
	procFwpmTransactionAbort0,
	procFwpmTransactionBegin0,
	procFwpmTransactionCommit0,
	procQueryDosDeviceW,
}

func preflight() error {
	if err := modfwpuclnt.Load(); err != nil {
		return err
	}
	for _, p := range requiredProcs {
		if err := p.Find(); err != nil {
			return fmt.Errorf("fwpuclnt missing %s", p.Name)
		}
	}
	return nil
}

func errnoErr(e syscall.Errno) error {
	if e == 0 {
		return syscall.EINVAL
	}
	return e
}

// WFP error space (fwperrors.h): FWP_E_*_NOT_FOUND runs 0x80320001-0x80320008;
// FWP_E_ALREADY_EXISTS is 0x80320009.
func isNotFoundErr(err error) bool {
	e, ok := err.(syscall.Errno)
	return ok && e >= 0x80320001 && e <= 0x80320008
}

func isAlreadyExistsErr(err error) bool {
	e, ok := err.(syscall.Errno)
	return ok && e == 0x80320009
}

func fwpmEngineClose0(engineHandle uintptr) (err error) {
	r1, _, _ := syscall.Syscall(procFwpmEngineClose0.Addr(), 1, engineHandle, 0, 0)
	if r1 != 0 {
		err = syscall.Errno(r1)
	}
	return
}

func fwpmEngineOpen0(serverName *uint16, authnService wtRpcCAuthN, authIdentity *uintptr, session *wtFwpmSession0, engineHandle unsafe.Pointer) (err error) {
	r1, _, _ := syscall.Syscall6(procFwpmEngineOpen0.Addr(), 5,
		uintptr(unsafe.Pointer(serverName)), uintptr(authnService),
		uintptr(unsafe.Pointer(authIdentity)), uintptr(unsafe.Pointer(session)),
		uintptr(engineHandle), 0)
	if r1 != 0 {
		err = syscall.Errno(r1)
	}
	return
}

func fwpmEngineSetOption0(engineHandle uintptr, option uint32, value *wtFwpValue0) (err error) {
	r1, _, _ := syscall.Syscall(procFwpmEngineSetOption0.Addr(), 3,
		engineHandle, uintptr(option), uintptr(unsafe.Pointer(value)))
	if r1 != 0 {
		err = syscall.Errno(r1)
	}
	return
}

func fwpmFilterAdd0(engineHandle uintptr, filter *wtFwpmFilter0, sd uintptr, id *uint64) (err error) {
	r1, _, _ := syscall.Syscall6(procFwpmFilterAdd0.Addr(), 4,
		engineHandle, uintptr(unsafe.Pointer(filter)), sd,
		uintptr(unsafe.Pointer(id)), 0, 0)
	if r1 != 0 {
		err = syscall.Errno(r1)
	}
	return
}

func fwpmFilterCreateEnumHandle0(engineHandle uintptr, enumHandle *uintptr) (err error) {
	r1, _, _ := syscall.Syscall(procFwpmFilterCreateEnumHandle0.Addr(), 3,
		engineHandle, 0, uintptr(unsafe.Pointer(enumHandle)))
	if r1 != 0 {
		err = syscall.Errno(r1)
	}
	return
}

func fwpmFilterDeleteByKey0(engineHandle uintptr, key *windows.GUID) (err error) {
	r1, _, _ := syscall.Syscall(procFwpmFilterDeleteByKey0.Addr(), 2,
		engineHandle, uintptr(unsafe.Pointer(key)), 0)
	if r1 != 0 {
		err = syscall.Errno(r1)
	}
	return
}

func fwpmFilterDestroyEnumHandle0(engineHandle, enumHandle uintptr) (err error) {
	r1, _, _ := syscall.Syscall(procFwpmFilterDestroyEnumHandle0.Addr(), 2,
		engineHandle, enumHandle, 0)
	if r1 != 0 {
		err = syscall.Errno(r1)
	}
	return
}

func fwpmFilterEnum0(engineHandle, enumHandle uintptr, numRequested uint32, entries unsafe.Pointer, numReturned *uint32) (err error) {
	r1, _, _ := syscall.Syscall6(procFwpmFilterEnum0.Addr(), 5,
		engineHandle, enumHandle, uintptr(numRequested),
		uintptr(entries), uintptr(unsafe.Pointer(numReturned)), 0)
	if r1 != 0 {
		err = syscall.Errno(r1)
	}
	return
}

func fwpmFilterGetByKey0(engineHandle uintptr, key *windows.GUID, filter **wtFwpmFilter0) (err error) {
	r1, _, _ := syscall.Syscall(procFwpmFilterGetByKey0.Addr(), 3,
		engineHandle, uintptr(unsafe.Pointer(key)), uintptr(unsafe.Pointer(filter)))
	if r1 != 0 {
		err = syscall.Errno(r1)
	}
	return
}

func fwpmNetEventCreateEnumHandle0(engineHandle uintptr, enumTemplate *wtFwpmNetEventEnumTemplate0, enumHandle unsafe.Pointer) (err error) {
	r1, _, _ := syscall.Syscall(procFwpmNetEventCreateEnumHandle0.Addr(), 3,
		engineHandle, uintptr(unsafe.Pointer(enumTemplate)), uintptr(enumHandle))
	if r1 != 0 {
		err = syscall.Errno(r1)
	}
	return
}

func fwpmNetEventDestroyEnumHandle0(engineHandle, enumHandle uintptr) (err error) {
	r1, _, _ := syscall.Syscall(procFwpmNetEventDestroyEnumHandle0.Addr(), 2,
		engineHandle, enumHandle, 0)
	if r1 != 0 {
		err = syscall.Errno(r1)
	}
	return
}

func fwpmNetEventEnum0(engineHandle, enumHandle uintptr, numRequested uint32, entries unsafe.Pointer, numReturned *uint32) (err error) {
	r1, _, _ := syscall.Syscall6(procFwpmNetEventEnum0.Addr(), 5,
		engineHandle, enumHandle, uintptr(numRequested),
		uintptr(entries), uintptr(unsafe.Pointer(numReturned)), 0)
	if r1 != 0 {
		err = syscall.Errno(r1)
	}
	return
}

// callback is a thunk produced by windows.NewCallback matching
// FWPM_NET_EVENT_CALLBACK0: void (*)(void *context, const FWPM_NET_EVENT0 *event).
func fwpmNetEventSubscribe0(engineHandle uintptr, subscription *wtFwpmNetEventSubscription0, callback uintptr, ctx, eventsHandle unsafe.Pointer) (err error) {
	r1, _, _ := syscall.Syscall6(procFwpmNetEventSubscribe0.Addr(), 5,
		engineHandle, uintptr(unsafe.Pointer(subscription)), callback,
		uintptr(ctx), uintptr(eventsHandle), 0)
	if r1 != 0 {
		err = syscall.Errno(r1)
	}
	return
}

func fwpmNetEventUnsubscribe0(engineHandle, eventsHandle uintptr) (err error) {
	r1, _, _ := syscall.Syscall(procFwpmNetEventUnsubscribe0.Addr(), 2,
		engineHandle, eventsHandle, 0)
	if r1 != 0 {
		err = syscall.Errno(r1)
	}
	return
}

func fwpmFreeMemory0[T any](p **T) {
	syscall.Syscall(procFwpmFreeMemory0.Addr(), 1, uintptr(unsafe.Pointer(p)), 0, 0)
}

func fwpmGetAppIdFromFileName0(fileName *uint16, appID unsafe.Pointer) (err error) {
	r1, _, _ := syscall.Syscall(procFwpmGetAppIdFromFileName0.Addr(), 2,
		uintptr(unsafe.Pointer(fileName)), uintptr(appID), 0)
	if r1 != 0 {
		err = syscall.Errno(r1)
	}
	return
}

func fwpmProviderAdd0(engineHandle uintptr, provider *wtFwpmProvider0, sd uintptr) (err error) {
	r1, _, _ := syscall.Syscall(procFwpmProviderAdd0.Addr(), 3,
		engineHandle, uintptr(unsafe.Pointer(provider)), sd)
	if r1 != 0 {
		err = syscall.Errno(r1)
	}
	return
}

func fwpmProviderDeleteByKey0(engineHandle uintptr, key *windows.GUID) (err error) {
	r1, _, _ := syscall.Syscall(procFwpmProviderDeleteByKey0.Addr(), 2,
		engineHandle, uintptr(unsafe.Pointer(key)), 0)
	if r1 != 0 {
		err = syscall.Errno(r1)
	}
	return
}

func fwpmSubLayerAdd0(engineHandle uintptr, subLayer *wtFwpmSublayer0, sd uintptr) (err error) {
	r1, _, _ := syscall.Syscall(procFwpmSubLayerAdd0.Addr(), 3,
		engineHandle, uintptr(unsafe.Pointer(subLayer)), sd)
	if r1 != 0 {
		err = syscall.Errno(r1)
	}
	return
}

func fwpmSubLayerDeleteByKey0(engineHandle uintptr, key *windows.GUID) (err error) {
	r1, _, _ := syscall.Syscall(procFwpmSubLayerDeleteByKey0.Addr(), 2,
		engineHandle, uintptr(unsafe.Pointer(key)), 0)
	if r1 != 0 {
		err = syscall.Errno(r1)
	}
	return
}

func fwpmTransactionAbort0(engineHandle uintptr) (err error) {
	r1, _, _ := syscall.Syscall(procFwpmTransactionAbort0.Addr(), 1, engineHandle, 0, 0)
	if r1 != 0 {
		err = syscall.Errno(r1)
	}
	return
}

func fwpmTransactionBegin0(engineHandle uintptr, flags uint32) (err error) {
	r1, _, _ := syscall.Syscall(procFwpmTransactionBegin0.Addr(), 2, engineHandle, uintptr(flags), 0)
	if r1 != 0 {
		err = syscall.Errno(r1)
	}
	return
}

func fwpmTransactionCommit0(engineHandle uintptr) (err error) {
	r1, _, _ := syscall.Syscall(procFwpmTransactionCommit0.Addr(), 1, engineHandle, 0, 0)
	if r1 != 0 {
		err = syscall.Errno(r1)
	}
	return
}

// queryDosDeviceW wraps QueryDosDeviceW; returns the number of TCHARs written.
func queryDosDeviceW(name *uint16, buf *uint16, bufLen uint32) (uint32, error) {
	r1, _, e1 := syscall.Syscall(procQueryDosDeviceW.Addr(), 3,
		uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(buf)), uintptr(bufLen))
	if r1 == 0 {
		return 0, errnoErr(e1)
	}
	return uint32(r1), nil
}
