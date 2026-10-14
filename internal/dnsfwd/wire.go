// Package dnsfwd implements the secure-DNS forwarder: a UDP+TCP listener on
// the loopback resolver address that relays every query as DNS-over-TCP
// through a SOCKS5 proxy, plus a TTL'd IP→domain cache so redirected flows
// can be re-connected by name (remote resolution, no DNS leak).
//
// wire.go is a minimal DNS message codec — just enough to copy the question
// into a SERVFAIL answer and to harvest A/AAAA answers for the domain cache.
package dnsfwd

import (
	"encoding/binary"
	"errors"
	"net/netip"
	"strings"
)

var errShort = errors.New("dnsfwd: truncated message")

const (
	dnsHdrLen  = 12
	rcodeServ  = 2
	rcodeNoErr = 0
	flagQR     = 0x8000
	rcodeMask  = 0x000f
	rdBit      = 0x0100
	opCodeMask = 0x7800
	tcBit      = 0x0200
)

// Answer is one harvested A/AAAA record.
type Answer struct {
	IP  netip.Addr
	TTL uint32
}

// QName extracts the first question name. Returns "" if the message is
// malformed or has no question.
func QName(msg []byte) string {
	name, _, err := readName(msg, dnsHdrLen)
	if err != nil {
		return ""
	}
	return name
}

// readName decodes a possibly-compressed DNS name at off; returns the name in
// dotted form (no trailing dot) and the offset right after the name at its
// original position (compression pointers end the walk).
func readName(msg []byte, off int) (string, int, error) {
	var parts []string
	pos := off
	end := -1
	seen := 0
	for {
		if pos >= len(msg) {
			return "", 0, errShort
		}
		l := int(msg[pos])
		switch {
		case l == 0:
			if end < 0 {
				end = pos + 1
			}
			return strings.Join(parts, "."), end, nil
		case l&0xc0 == 0xc0:
			if pos+1 >= len(msg) {
				return "", 0, errShort
			}
			if end < 0 {
				end = pos + 2
			}
			pos = int(binary.BigEndian.Uint16(msg[pos:pos+2]) & 0x3fff)
			seen++
			if seen > 16 {
				return "", 0, errors.New("dnsfwd: compression loop")
			}
		case l&0xc0 != 0:
			return "", 0, errors.New("dnsfwd: bad label")
		default:
			pos++
			if pos+l > len(msg) {
				return "", 0, errShort
			}
			parts = append(parts, string(msg[pos:pos+l]))
			pos += l
		}
	}
}

// questionEnd returns the offset just past the first question section.
func questionEnd(msg []byte) (int, error) {
	if len(msg) < dnsHdrLen {
		return 0, errShort
	}
	_, end, err := readName(msg, dnsHdrLen)
	if err != nil {
		return 0, err
	}
	if end+4 > len(msg) {
		return 0, errShort
	}
	return end + 4, nil
}

// ParseAnswers extracts the A/AAAA answers of a response message. The qname
// of the first question is also returned (used as the cache label).
func ParseAnswers(msg []byte) (qname string, out []Answer, ok bool) {
	if len(msg) < dnsHdrLen {
		return "", nil, false
	}
	flags := binary.BigEndian.Uint16(msg[2:4])
	if flags&flagQR == 0 || flags&rcodeMask != rcodeNoErr {
		return "", nil, false
	}
	qd := int(binary.BigEndian.Uint16(msg[4:6]))
	an := int(binary.BigEndian.Uint16(msg[6:8]))

	off := dnsHdrLen
	for i := 0; i < qd; i++ {
		name, end, err := readName(msg, off)
		if err != nil {
			return "", nil, false
		}
		if i == 0 {
			qname = name
		}
		off = end + 4 // QTYPE + QCLASS
	}
	for i := 0; i < an; i++ {
		_, end, err := readName(msg, off)
		if err != nil {
			return qname, out, qname != "" || len(out) > 0
		}
		if end+10 > len(msg) {
			return qname, out, qname != "" || len(out) > 0
		}
		typ := binary.BigEndian.Uint16(msg[end : end+2])
		class := binary.BigEndian.Uint16(msg[end+2 : end+4])
		ttl := binary.BigEndian.Uint32(msg[end+4 : end+8])
		rdlen := int(binary.BigEndian.Uint16(msg[end+8 : end+10]))
		rd := end + 10
		if rd+rdlen > len(msg) {
			return qname, out, qname != "" || len(out) > 0
		}
		if class == 1 { // IN
			switch typ {
			case 1: // A
				if rdlen == 4 {
					var b [4]byte
					copy(b[:], msg[rd:rd+4])
					out = append(out, Answer{IP: netip.AddrFrom4(b), TTL: ttl})
				}
			case 28: // AAAA
				if rdlen == 16 {
					var b [16]byte
					copy(b[:], msg[rd:rd+16])
					out = append(out, Answer{IP: netip.AddrFrom16(b), TTL: ttl})
				}
			}
		}
		off = rd + rdlen
	}
	return qname, out, qname != "" || len(out) > 0
}

// BuildServFail returns a response copy of query with RCODE=SERVFAIL and all
// answer sections removed.
func BuildServFail(query []byte) []byte {
	if len(query) < dnsHdrLen {
		return nil
	}
	end := len(query)
	if qe, err := questionEnd(query); err == nil {
		end = qe
	}
	out := make([]byte, end)
	copy(out, query)
	// QR=1, opcode preserved, TC=0, RD preserved, RA=0, RCODE=2.
	flags := binary.BigEndian.Uint16(out[2:4]) & (opCodeMask | rdBit)
	flags |= flagQR | rcodeServ
	binary.BigEndian.PutUint16(out[2:4], flags)
	// Zero all counts but QDCOUNT.
	for _, off := range [3]int{6, 8, 10} {
		binary.BigEndian.PutUint16(out[off:off+2], 0)
	}
	return out
}

// IsQuery reports whether msg looks like a well-formed query we can relay.
func IsQuery(msg []byte) bool {
	return len(msg) >= dnsHdrLen && len(msg) <= 65535 &&
		binary.BigEndian.Uint16(msg[2:4])&flagQR == 0 &&
		binary.BigEndian.Uint16(msg[4:6]) > 0
}
