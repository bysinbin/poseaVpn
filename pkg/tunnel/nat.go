package tunnel

import (
	"encoding/binary"
	"fmt"
	"log"
	"net/netip"
	"sync"

	"golang.zx2c4.com/wireguard/tun"

	"poseavpn/pkg/config"
)

// ---------------------------------------------------------------------------
// Address mapping (1:1 NAT)
//
// Two tunnels can lead to different networks that use the same addresses
// (e.g. both sites use 192.168.0.0/22). Windows can only route a destination
// to one of them. The tunnel that connects later therefore gets a *virtual*
// prefix of the same size (e.g. 198.18.0.0/22): Windows sends traffic for the
// virtual address to this tunnel, and this layer translates it to the real
// address on the way in and back on the way out.
// ---------------------------------------------------------------------------

// Mapping ties a real prefix of a remote network to the virtual prefix that
// stands for it on this computer. Both have the same length.
type Mapping struct {
	Real    netip.Prefix
	Virtual netip.Prefix
}

func (m Mapping) String() string { return fmt.Sprintf("%s → %s", m.Real, m.Virtual) }

// virtPool is the range virtual prefixes are taken from (RFC 2544 benchmark range,
// which real networks practically never use).
var virtPool = netip.MustParsePrefix("198.18.0.0/15")

var (
	virtMu   sync.Mutex
	virtUsed []netip.Prefix
)

func addrU32(a netip.Addr) uint32 {
	b := a.As4()
	return binary.BigEndian.Uint32(b[:])
}

func u32Addr(v uint32) netip.Addr {
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], v)
	return netip.AddrFrom4(b)
}

// allocVirtual reserves an aligned free block of the given prefix length.
func allocVirtual(bits int) (netip.Prefix, func(), error) {
	if bits < 16 || bits > 32 {
		return netip.Prefix{}, nil, fmt.Errorf("cannot map a /%d prefix", bits)
	}
	size := uint64(1) << uint(32-bits)
	start := uint64(addrU32(virtPool.Addr()))
	end := start + (uint64(1) << uint(32-virtPool.Bits()))

	virtMu.Lock()
	defer virtMu.Unlock()
	for cur := start; cur+size <= end; cur += size {
		cand := netip.PrefixFrom(u32Addr(uint32(cur)), bits)
		if overlapsAny(cand, virtUsed) {
			continue
		}
		virtUsed = append(virtUsed, cand)
		return cand, func() {
			virtMu.Lock()
			defer virtMu.Unlock()
			for i, p := range virtUsed {
				if p == cand {
					virtUsed = append(virtUsed[:i], virtUsed[i+1:]...)
					return
				}
			}
		}, nil
	}
	return netip.Prefix{}, nil, fmt.Errorf("no free virtual /%d block left", bits)
}

func overlapsAny(p netip.Prefix, list []netip.Prefix) bool {
	for _, q := range list {
		if p.Overlaps(q) {
			return true
		}
	}
	return false
}

// planMappings decides which of this tunnel's networks need a virtual prefix:
// every IPv4 AllowedIPs entry that overlaps something another tunnel already
// reaches. otherReach is what the other running tunnels really claim in Windows.
func planMappings(cfg *config.Config, otherReach []netip.Prefix) ([]Mapping, []func()) {
	if len(otherReach) == 0 {
		return nil, nil
	}
	var maps []Mapping
	var releases []func()
	seen := map[netip.Prefix]bool{}

	for _, peer := range cfg.Peers {
		for _, ip := range peer.AllowedIPs {
			ip = ip.Masked()
			if !ip.Addr().Is4() || seen[ip] || !overlapsAny(ip, otherReach) {
				continue
			}
			seen[ip] = true
			virt, release, err := allocVirtual(ip.Bits())
			if err != nil {
				log.Printf("[PoseaVPN] address mapping for %s skipped: %v", ip, err)
				continue
			}
			maps = append(maps, Mapping{Real: ip, Virtual: virt})
			releases = append(releases, release)
		}
	}
	return maps, releases
}

// ---------------------------------------------------------------------------
// tun.Device wrapper
// ---------------------------------------------------------------------------

// natDevice sits between Wintun and WireGuard and translates addresses.
type natDevice struct {
	tun.Device
	maps []Mapping
}

// Read returns packets Windows sent into the tunnel: virtual destinations become real.
func (n *natDevice) Read(bufs [][]byte, sizes []int, offset int) (int, error) {
	c, err := n.Device.Read(bufs, sizes, offset)
	for i := 0; i < c; i++ {
		if sizes[i] > 0 {
			rewritePacket(bufs[i][offset:offset+sizes[i]], n.maps, true)
		}
	}
	return c, err
}

// Write hands packets from the remote network to Windows: real sources become virtual.
func (n *natDevice) Write(bufs [][]byte, offset int) (int, error) {
	for i := range bufs {
		if len(bufs[i]) > offset {
			rewritePacket(bufs[i][offset:], n.maps, false)
		}
	}
	return n.Device.Write(bufs, offset)
}

// ---------------------------------------------------------------------------
// Packet rewriting
// ---------------------------------------------------------------------------

// translate maps one address. outbound: virtual → real; inbound: real → virtual.
func translate(a [4]byte, maps []Mapping, outbound bool) ([4]byte, bool) {
	addr := netip.AddrFrom4(a)
	for _, m := range maps {
		from, to := m.Real, m.Virtual
		if outbound {
			from, to = m.Virtual, m.Real
		}
		if from.Contains(addr) {
			host := addrU32(addr) - addrU32(from.Addr())
			return u32Addr(addrU32(to.Addr()) + host).As4(), true
		}
	}
	return a, false
}

// csumAdjust updates an Internet checksum after a 4-byte field changed (RFC 1624).
func csumAdjust(sum uint16, old, new [4]byte) uint16 {
	s := uint32(^sum) & 0xFFFF
	for i := 0; i < 4; i += 2 {
		o := uint16(old[i])<<8 | uint16(old[i+1])
		n := uint16(new[i])<<8 | uint16(new[i+1])
		s += uint32(^o) & 0xFFFF
		s += uint32(n)
	}
	for s>>16 != 0 {
		s = (s & 0xFFFF) + (s >> 16)
	}
	return ^uint16(s)
}

// csum16 computes the Internet checksum of b.
func csum16(b []byte) uint16 {
	var s uint32
	for i := 0; i+1 < len(b); i += 2 {
		s += uint32(b[i])<<8 | uint32(b[i+1])
	}
	if len(b)%2 == 1 {
		s += uint32(b[len(b)-1]) << 8
	}
	for s>>16 != 0 {
		s = (s & 0xFFFF) + (s >> 16)
	}
	return ^uint16(s)
}

// rewritePacket translates one IPv4 packet in place and fixes every checksum
// that covers the changed address. Anything that is not IPv4 is left alone.
func rewritePacket(pkt []byte, maps []Mapping, outbound bool) {
	if len(maps) == 0 || len(pkt) < 20 || pkt[0]>>4 != 4 {
		return
	}
	ihl := int(pkt[0]&0x0F) * 4
	total := int(binary.BigEndian.Uint16(pkt[2:4]))
	if ihl < 20 || len(pkt) < ihl || total < ihl || total > len(pkt) {
		return
	}

	// outbound packets carry the virtual address in the destination, inbound in the source
	off := 12 // source
	if outbound {
		off = 16 // destination
	}
	var oldA [4]byte
	copy(oldA[:], pkt[off:off+4])
	newA, ok := translate(oldA, maps, outbound)
	if !ok {
		return
	}
	copy(pkt[off:off+4], newA[:])

	// IPv4 header checksum
	ipsum := binary.BigEndian.Uint16(pkt[10:12])
	binary.BigEndian.PutUint16(pkt[10:12], csumAdjust(ipsum, oldA, newA))

	// Transport checksum (pseudo header contains the address). Only the first
	// fragment carries the transport header.
	if binary.BigEndian.Uint16(pkt[6:8])&0x1FFF != 0 {
		return
	}
	l4 := pkt[ihl:total]
	switch pkt[9] {
	case 6: // TCP
		if len(l4) >= 18 {
			s := binary.BigEndian.Uint16(l4[16:18])
			binary.BigEndian.PutUint16(l4[16:18], csumAdjust(s, oldA, newA))
		}
	case 17: // UDP
		if len(l4) >= 8 {
			s := binary.BigEndian.Uint16(l4[6:8])
			if s != 0 { // 0 = checksum not used
				ns := csumAdjust(s, oldA, newA)
				if ns == 0 {
					ns = 0xFFFF
				}
				binary.BigEndian.PutUint16(l4[6:8], ns)
			}
		}
	case 1: // ICMP: errors quote the original IP header, which holds mapped addresses
		rewriteICMPError(l4, maps, outbound)
	}
}

// rewriteICMPError translates the quoted packet inside an ICMP error message
// (destination unreachable, time exceeded, ...) and recomputes the checksums.
func rewriteICMPError(icmp []byte, maps []Mapping, outbound bool) {
	if len(icmp) < 8+20 {
		return
	}
	switch icmp[0] {
	case 3, 4, 11, 12:
	default:
		return // echo etc.: the ICMP checksum does not cover the IP header
	}
	inner := icmp[8:]
	if inner[0]>>4 != 4 {
		return
	}
	iihl := int(inner[0]&0x0F) * 4
	if iihl < 20 || len(inner) < iihl {
		return
	}

	changed := false
	for _, off := range []int{12, 16} { // quoted source and destination
		var a [4]byte
		copy(a[:], inner[off:off+4])
		if n, ok := translate(a, maps, outbound); ok {
			copy(inner[off:off+4], n[:])
			changed = true
		}
	}
	if !changed {
		return
	}
	inner[10], inner[11] = 0, 0
	binary.BigEndian.PutUint16(inner[10:12], csum16(inner[:iihl]))
	icmp[2], icmp[3] = 0, 0
	binary.BigEndian.PutUint16(icmp[2:4], csum16(icmp))
}
