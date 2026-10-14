package netpkt

import (
	"encoding/binary"
	"net/netip"
)

// NAT IP encoding — each redirected flow gets a natID in [1, MaxNATID] that is
// embedded into the destination address seen by the local transparent
// listener, so an accepted connection can be mapped back to its flow.
//
//	IPv4: 127.(64 + natID>>16).(natID>>8).natID  inside 127.64.0.0/10
//	      (all of 127.0.0.0/8 is local on Windows, so the wildcard v4
//	      listener accepts these addresses without extra config)
//	IPv6: fd00:7078:7879:0:0:0:<natID hi16>:<natID lo16>  (ULA /48; the low
//	      32 bits carry the natID). Requires the address range to be routed
//	      to the local stack — see engine for the port-pool fallback.
const MaxNATID = (1 << 22) - 1

var natPrefix6 = [6]byte{0xfd, 0x00, 0x70, 0x78, 0x78, 0x79}

func EncodeNATIPv4(natID uint32) netip.Addr {
	return netip.AddrFrom4([4]byte{
		127,
		64 + byte(natID>>16),
		byte(natID >> 8),
		byte(natID),
	})
}

// DecodeNATIPv4 recovers a natID from a 127.64.0.0/10 address.
func DecodeNATIPv4(a netip.Addr) (uint32, bool) {
	if !a.Is4() {
		return 0, false
	}
	b := a.As4()
	if b[0] != 127 || b[1] < 64 || b[1] >= 128 {
		return 0, false
	}
	id := uint32(b[1]-64)<<16 | uint32(b[2])<<8 | uint32(b[3])
	return id, id >= 1 && id <= MaxNATID
}

func EncodeNATIPv6(natID uint32) netip.Addr {
	var b [16]byte
	copy(b[:6], natPrefix6[:])
	binary.BigEndian.PutUint32(b[12:16], natID)
	return netip.AddrFrom16(b)
}

// DecodeNATIPv6 recovers a natID from an fd00:7078:7879::/48 address.
func DecodeNATIPv6(a netip.Addr) (uint32, bool) {
	if !a.Is6() || a.Is4In6() {
		return 0, false
	}
	b := a.As16()
	for i := 0; i < 6; i++ {
		if b[i] != natPrefix6[i] {
			return 0, false
		}
	}
	for i := 6; i < 12; i++ {
		if b[i] != 0 {
			return 0, false
		}
	}
	id := binary.BigEndian.Uint32(b[12:16])
	return id, id >= 1 && id <= MaxNATID
}
