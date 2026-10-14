//go:build windows && (amd64 || arm64)

// NOTICE: the WFP structure layouts, constants and GUIDs in this file are
// adapted from the MIT-licensed WireGuard for Windows project
// (golang.zx2c4.com/wireguard/windows, package tunnel/firewall).
//
// SPDX-License-Identifier: MIT
// Copyright (C) 2019-2021 WireGuard LLC. All Rights Reserved.
//
// Only the definitions Proxity needs are kept; the public API of this
// package (see wfp_windows.go) does not expose these types.

package wfp

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	wtFwpByteBlob_Size        = 16
	wtFwpByteBlob_data_Offset = 8

	wtFwpConditionValue0_Size = 16

	wtFwpmFilter0_Size              = 200
	wtFwpmFilterCondition0_Size     = 40
	wtFwpmSession0_Size             = 72
	wtFwpmSublayer0_Size            = 72
	wtFwpProvider0_Size             = 64
	wtFwpValue0_Size                = 16
	wtFwpV4AddrAndMask_Size         = 8
	wtFwpV6AddrAndMask_Size         = 17
	wtFwpRange0_Size                = 32
	wtFwpmDisplayData0_Size         = 16
	wtFwpmAction0_Size              = 20
	wtFwpmAction0_filterType_Offset = 4
)

type wtFwpActionFlag uint32

const (
	cFWP_ACTION_FLAG_TERMINATING wtFwpActionFlag = 0x00001000
)

// FWP_ACTION_TYPE (fwptypes.h)
type wtFwpActionType uint32

const (
	cFWP_ACTION_BLOCK  wtFwpActionType = wtFwpActionType(0x00000001 | cFWP_ACTION_FLAG_TERMINATING)
	cFWP_ACTION_PERMIT wtFwpActionType = wtFwpActionType(0x00000002 | cFWP_ACTION_FLAG_TERMINATING)
)

// FWP_BYTE_BLOB (fwptypes.h)
type wtFwpByteBlob struct {
	size uint32
	data *uint8
}

// FWP_MATCH_TYPE (fwptypes.h)
type wtFwpMatchType uint32

const (
	cFWP_MATCH_EQUAL          wtFwpMatchType = 0
	cFWP_MATCH_RANGE          wtFwpMatchType = 5
	cFWP_MATCH_FLAGS_ALL_SET  wtFwpMatchType = 6
	cFWP_MATCH_FLAGS_ANY_SET  wtFwpMatchType = 7
	cFWP_MATCH_FLAGS_NONE_SET wtFwpMatchType = 8
)

// FWPM_ACTION0 (fwpmtypes.h)
type wtFwpmAction0 struct {
	_type      wtFwpActionType
	filterType windows.GUID
}

// Condition field GUIDs (fwpmu.h)

// b235ae9a-1d64-49b8-a44c-5ff3d9095045
var cFWPM_CONDITION_IP_REMOTE_ADDRESS = windows.GUID{
	Data1: 0xb235ae9a,
	Data2: 0x1d64,
	Data3: 0x49b8,
	Data4: [8]byte{0xa4, 0x4c, 0x5f, 0xf3, 0xd9, 0x09, 0x50, 0x45},
}

// 3971ef2b-623e-4f9a-8cb1-6e79b806b9a7
var cFWPM_CONDITION_IP_PROTOCOL = windows.GUID{
	Data1: 0x3971ef2b,
	Data2: 0x623e,
	Data3: 0x4f9a,
	Data4: [8]byte{0x8c, 0xb1, 0x6e, 0x79, 0xb8, 0x06, 0xb9, 0xa7},
}

// d78e1e87-8644-4ea5-9437-d809ecefc971
var cFWPM_CONDITION_ALE_APP_ID = windows.GUID{
	Data1: 0xd78e1e87,
	Data2: 0x8644,
	Data3: 0x4ea5,
	Data4: [8]byte{0x94, 0x37, 0xd8, 0x09, 0xec, 0xef, 0xc9, 0x71},
}

// 632ce23b-5167-435c-86d7-e903684aa80c
var cFWPM_CONDITION_FLAGS = windows.GUID{
	Data1: 0x632ce23b,
	Data2: 0x5167,
	Data3: 0x435c,
	Data4: [8]byte{0x86, 0xd7, 0xe9, 0x03, 0x68, 0x4a, 0xa8, 0x0c},
}

// c35a604d-d22b-4e1a-91b4-68f674ee674b — also usable inside
// FWPM_NET_EVENT_ENUM_TEMPLATE0 / FWPM_NET_EVENT_SUBSCRIPTION0.
var cFWPM_CONDITION_IP_REMOTE_PORT = windows.GUID{
	Data1: 0xc35a604d,
	Data2: 0xd22b,
	Data3: 0x4e1a,
	Data4: [8]byte{0x91, 0xb4, 0x68, 0xf6, 0x74, 0xee, 0x67, 0x4b},
}

const cFWP_CONDITION_FLAG_IS_LOOPBACK uint32 = 0x00000001

// FWPM_FILTER0 flags (fwpmtypes.h)
type wtFwpmFilterFlags uint32

const (
	cFWPM_FILTER_FLAG_NONE       wtFwpmFilterFlags = 0x00000000
	cFWPM_FILTER_FLAG_PERSISTENT wtFwpmFilterFlags = 0x00000001
)

// Layer GUIDs (fwpmu.h)

// FWPM_LAYER_ALE_AUTH_CONNECT_V4 (c38d57d1-05a7-4c33-904f-7fbceee60e82)
var cFWPM_LAYER_ALE_AUTH_CONNECT_V4 = windows.GUID{
	Data1: 0xc38d57d1,
	Data2: 0x05a7,
	Data3: 0x4c33,
	Data4: [8]byte{0x90, 0x4f, 0x7f, 0xbc, 0xee, 0xe6, 0x0e, 0x82},
}

// FWPM_LAYER_ALE_AUTH_CONNECT_V6 (4a72393b-319f-44bc-84c3-ba54dcb3b6b4)
var cFWPM_LAYER_ALE_AUTH_CONNECT_V6 = windows.GUID{
	Data1: 0x4a72393b,
	Data2: 0x319f,
	Data3: 0x44bc,
	Data4: [8]byte{0x84, 0xc3, 0xba, 0x54, 0xdc, 0xb3, 0xb6, 0xb4},
}

// FWPM_LAYER_ALE_AUTH_RECV_ACCEPT_V4 (e1cd9fe7-f4b5-4273-96c0-592e487b8650)
// Authorizes inbound accept/first-packet delivery — the transparent
// redirect's reflected SYN lands here under the default inbound-block
// policy, so the service needs a self permit at this layer.
var cFWPM_LAYER_ALE_AUTH_RECV_ACCEPT_V4 = windows.GUID{
	Data1: 0xe1cd9fe7,
	Data2: 0xf4b5,
	Data3: 0x4273,
	Data4: [8]byte{0x96, 0xc0, 0x59, 0x2e, 0x48, 0x7b, 0x86, 0x50},
}

// FWPM_LAYER_ALE_AUTH_RECV_ACCEPT_V6 (a3b42c97-9f04-4672-b87e-cee9c483257f)
var cFWPM_LAYER_ALE_AUTH_RECV_ACCEPT_V6 = windows.GUID{
	Data1: 0xa3b42c97,
	Data2: 0x9f04,
	Data3: 0x4672,
	Data4: [8]byte{0xb8, 0x7e, 0xce, 0xe9, 0xc4, 0x83, 0x25, 0x7f},
}

// FWP_DATA_TYPE (fwptypes.h)
type wtFwpDataType uint

const (
	cFWP_EMPTY                    wtFwpDataType = 0
	cFWP_UINT8                    wtFwpDataType = cFWP_EMPTY + 1
	cFWP_UINT16                   wtFwpDataType = cFWP_UINT8 + 1
	cFWP_UINT32                   wtFwpDataType = cFWP_UINT16 + 1
	cFWP_UINT64                   wtFwpDataType = cFWP_UINT32 + 1
	cFWP_BYTE_ARRAY16_TYPE        wtFwpDataType = 10
	cFWP_BYTE_BLOB_TYPE           wtFwpDataType = 11
	cFWP_SID                      wtFwpDataType = 12
	cFWP_SECURITY_DESCRIPTOR_TYPE wtFwpDataType = 13
	cFWP_SINGLE_DATA_TYPE_MAX     wtFwpDataType = 0xff
	cFWP_V4_ADDR_MASK             wtFwpDataType = cFWP_SINGLE_DATA_TYPE_MAX + 1
	cFWP_V6_ADDR_MASK             wtFwpDataType = cFWP_V4_ADDR_MASK + 1
	cFWP_RANGE_TYPE               wtFwpDataType = cFWP_V6_ADDR_MASK + 1
)

// FWP_V4_ADDR_AND_MASK (fwptypes.h)
type wtFwpV4AddrAndMask struct {
	addr uint32
	mask uint32
}

// FWP_V6_ADDR_AND_MASK (fwptypes.h)
type wtFwpV6AddrAndMask struct {
	addr         [16]uint8
	prefixLength uint8
}

// FWP_RANGE0 (fwptypes.h): two FWP_VALUE0 low/high bounds.
type wtFwpRange0 struct {
	valueLow  wtFwpValue0
	valueHigh wtFwpValue0
}

// FWP_VALUE0 (fwptypes.h)
type wtFwpValue0 struct {
	_type wtFwpDataType
	value uintptr
}

// FWP_CONDITION_VALUE0 (fwptypes.h)
type wtFwpConditionValue0 wtFwpValue0

// FWPM_DISPLAY_DATA0 (fwpmtypes.h)
type wtFwpmDisplayData0 struct {
	name        *uint16
	description *uint16
}

// FWPM_FILTER_CONDITION0 (fwpmtypes.h)
type wtFwpmFilterCondition0 struct {
	fieldKey       windows.GUID
	matchType      wtFwpMatchType
	conditionValue wtFwpConditionValue0
}

// FWPM_FILTER0 (fwpmtypes.h), 64-bit layout.
type wtFwpmFilter0 struct {
	filterKey           windows.GUID
	displayData         wtFwpmDisplayData0
	flags               wtFwpmFilterFlags
	providerKey         *windows.GUID
	providerData        wtFwpByteBlob
	layerKey            windows.GUID
	subLayerKey         windows.GUID
	weight              wtFwpValue0
	numFilterConditions uint32
	filterCondition     *wtFwpmFilterCondition0
	action              wtFwpmAction0
	offset1             [4]byte // layout correction padding
	providerContextKey  windows.GUID
	reserved            *windows.GUID
	filterID            uint64
	effectiveWeight     wtFwpValue0
}

// FWPM_SESSION0 flags
// FWPM_ENGINE_OPTION values used with FwpmEngineSetOption0.
const cFWPM_ENGINE_COLLECT_NET_EVENTS uint32 = 0

type wtFwpmSessionFlagsValue uint32

const (
	cFWPM_SESSION_FLAG_DYNAMIC wtFwpmSessionFlagsValue = 0x00000001
)

// FWPM_SESSION0 (fwpmtypes.h)
type wtFwpmSession0 struct {
	sessionKey           windows.GUID
	displayData          wtFwpmDisplayData0
	flags                wtFwpmSessionFlagsValue
	txnWaitTimeoutInMSec uint32
	processId            uint32
	sid                  *windows.SID
	username             *uint16
	kernelMode           int32
}

// FWPM_SUBLAYER0 flags
type wtFwpmSublayerFlags uint32

const cFWPM_SUBLAYER_FLAG_PERSISTENT wtFwpmSublayerFlags = 0x00000001

// FWPM_SUBLAYER0 (fwpmtypes.h)
type wtFwpmSublayer0 struct {
	subLayerKey  windows.GUID
	displayData  wtFwpmDisplayData0
	flags        wtFwpmSublayerFlags
	providerKey  *windows.GUID
	providerData wtFwpByteBlob
	weight       uint16
}

// FWPM_PROVIDER0 flags
const cFWPM_PROVIDER_FLAG_PERSISTENT uint32 = 0x00000001

// FWPM_PROVIDER0 (fwpmtypes.h)
type wtFwpmProvider0 struct {
	providerKey  windows.GUID
	displayData  wtFwpmDisplayData0
	flags        uint32
	providerData wtFwpByteBlob
	serviceName  *uint16
}

// RPC authN constants (rpcdce.h)
type wtRpcCAuthN uint32

const cRPC_C_AUTHN_WINNT wtRpcCAuthN = 10

// IP protocol numbers used in conditions.
const (
	cIPPROTO_ICMPV6 = 58
	cIPPROTO_TCP    = 6
	cIPPROTO_UDP    = 17
)

// FWPM_NET_EVENT_TYPE (fwpmtypes.h). Only the value we match is defined.
const cFWPM_NET_EVENT_TYPE_CLASSIFY_DROP uint32 = 3

// FWP_IP_VERSION (fwptypes.h).
const cFWP_IP_VERSION_V6 uint32 = 1

// FWPM_NET_EVENT_ENUM_TEMPLATE0 (fwpmtypes.h). startTime/endTime are
// FILETIMEs bounding the event timestamps to return.
type wtFwpmNetEventEnumTemplate0 struct {
	startTime           windows.Filetime
	endTime             windows.Filetime
	numFilterConditions uint32
	_pad0               uint32
	filterCondition     *wtFwpmFilterCondition0
}

// FWPM_NET_EVENT_SUBSCRIPTION0 (fwpmtypes.h).
type wtFwpmNetEventSubscription0 struct {
	enumTemplate *wtFwpmNetEventEnumTemplate0
	flags        uint32
	_pad0        uint32
	sessionKey   windows.GUID
	startTime    *windows.Filetime
	endTime      *windows.Filetime
}

// FWPM_NET_EVENT_HEADER0 (fwpmtypes.h), x64 layout. The trailing
// addressFamily/reauthReason union is irrelevant — every field Proxity
// reads sits before it.
type wtFwpmNetEventHeader0 struct {
	timeStamp  windows.Filetime
	flags      uint32
	ipVersion  uint32
	ipProtocol uint8
	_pad0      [3]byte
	localAddr  [16]byte // v4 addr in the first 4 bytes when ipVersion==0
	remoteAddr [16]byte
	localPort  uint16
	remotePort uint16
	scopeId    uint32
	_pad1      uint32
	appId      wtFwpByteBlob
	userId     *windows.SID
}

// FWPM_NET_EVENT0 (fwpmtypes.h): type tag + header pointer + payload
// union pointer (we only ever read classifyDrop).
type wtFwpmNetEvent0 struct {
	_type  uint32
	_pad0  uint32
	header *wtFwpmNetEventHeader0
	u      unsafe.Pointer
}

// FWPM_NET_EVENT_CLASSIFY_DROP0 (fwpmtypes.h) — only the leading
// filterId is needed to attribute the drop to one of our filters.
type wtFwpmNetEventClassifyDrop0 struct {
	filterId uint64
}
