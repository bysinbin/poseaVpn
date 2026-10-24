package dnsproxy

import (
	"encoding/binary"
	"net/netip"
)

// skipName returns the offset just after the (possibly compressed) name at i,
// or -1 when the message is malformed.
func skipName(m []byte, i int) int {
	for hops := 0; i < len(m) && hops < 128; hops++ {
		l := int(m[i])
		switch {
		case l == 0:
			return i + 1
		case l&0xC0 == 0xC0: // compression pointer: the name ends here
			if i+2 > len(m) {
				return -1
			}
			return i + 2
		case l&0xC0 != 0:
			return -1
		default:
			i += 1 + l
		}
	}
	return -1
}

// RewriteA replaces the address of every IPv4 A record in a DNS message for which
// mapA reports a replacement. The message is changed in place and returned. A
// malformed message is returned unchanged from the point where parsing stops.
func RewriteA(m []byte, mapA func(netip.Addr) (netip.Addr, bool)) []byte {
	if mapA == nil || len(m) < 12 {
		return m
	}
	qd := int(binary.BigEndian.Uint16(m[4:6]))
	rrs := int(binary.BigEndian.Uint16(m[6:8])) + int(binary.BigEndian.Uint16(m[8:10])) + int(binary.BigEndian.Uint16(m[10:12]))

	i := 12
	for k := 0; k < qd; k++ { // question section: name + type + class
		if i = skipName(m, i); i < 0 || i+4 > len(m) {
			return m
		}
		i += 4
	}
	for k := 0; k < rrs; k++ { // answer, authority and additional sections
		if i = skipName(m, i); i < 0 || i+10 > len(m) {
			return m
		}
		typ := binary.BigEndian.Uint16(m[i:])
		class := binary.BigEndian.Uint16(m[i+2:])
		rdlen := int(binary.BigEndian.Uint16(m[i+8:]))
		rdata := i + 10
		if rdata+rdlen > len(m) {
			return m
		}
		if typ == 1 && class == 1 && rdlen == 4 { // A, IN
			var a [4]byte
			copy(a[:], m[rdata:rdata+4])
			if n, ok := mapA(netip.AddrFrom4(a)); ok {
				b := n.As4()
				copy(m[rdata:rdata+4], b[:])
			}
		}
		i = rdata + rdlen
	}
	return m
}
