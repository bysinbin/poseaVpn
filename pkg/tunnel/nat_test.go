package tunnel

import (
	"bytes"
	"encoding/binary"
	"math/rand"
	"net/netip"
	"testing"

	"poseavpn/pkg/config"
)

var (
	real22   = netip.MustParsePrefix("192.168.0.0/22")
	virt22   = netip.MustParsePrefix("198.18.0.0/22")
	testMaps = []Mapping{{Real: real22, Virtual: virt22}}

	me     = [4]byte{20, 0, 0, 2}      // this computer's tunnel address
	vHost  = [4]byte{198, 18, 1, 1}    // 192.168.1.1 as seen on this computer
	rHost  = [4]byte{192, 168, 1, 1}   // the real host
	rOther = [4]byte{192, 168, 3, 250} // another real host
)

// ---- packet builders (checksums computed from scratch, independent of the code under test)

func ip4(proto byte, src, dst [4]byte, payload []byte, fragOff uint16) []byte {
	h := make([]byte, 20)
	h[0] = 0x45
	binary.BigEndian.PutUint16(h[2:], uint16(20+len(payload)))
	binary.BigEndian.PutUint16(h[4:], 0x1234)
	binary.BigEndian.PutUint16(h[6:], fragOff)
	h[8] = 64
	h[9] = proto
	copy(h[12:], src[:])
	copy(h[16:], dst[:])
	binary.BigEndian.PutUint16(h[10:], csum16(h))
	return append(h, payload...)
}

func pseudo(src, dst [4]byte, proto byte, l4 []byte) []byte {
	p := append([]byte{}, src[:]...)
	p = append(p, dst[:]...)
	p = append(p, 0, proto, byte(len(l4)>>8), byte(len(l4)))
	return append(p, l4...)
}

func tcp(src, dst [4]byte, payload []byte) []byte {
	seg := make([]byte, 20)
	binary.BigEndian.PutUint16(seg[0:], 49152)
	binary.BigEndian.PutUint16(seg[2:], 445)
	seg[12] = 0x50
	seg[13] = 0x02 // SYN
	seg = append(seg, payload...)
	binary.BigEndian.PutUint16(seg[16:], csum16(pseudo(src, dst, 6, seg)))
	return ip4(6, src, dst, seg, 0)
}

func udp(src, dst [4]byte, payload []byte, withChecksum bool) []byte {
	d := make([]byte, 8)
	binary.BigEndian.PutUint16(d[0:], 53000)
	binary.BigEndian.PutUint16(d[2:], 53)
	binary.BigEndian.PutUint16(d[4:], uint16(8+len(payload)))
	d = append(d, payload...)
	if withChecksum {
		c := csum16(pseudo(src, dst, 17, d))
		if c == 0 {
			c = 0xFFFF
		}
		binary.BigEndian.PutUint16(d[6:], c)
	}
	return ip4(17, src, dst, d, 0)
}

func icmpEcho(src, dst [4]byte) []byte {
	m := []byte{8, 0, 0, 0, 0, 1, 0, 1, 'p', 'i', 'n', 'g'}
	binary.BigEndian.PutUint16(m[2:], csum16(m))
	return ip4(1, src, dst, m, 0)
}

// ICMP destination unreachable that quotes `orig` (header + 8 bytes)
func icmpUnreach(src, dst [4]byte, orig []byte) []byte {
	q := orig[:28]
	m := append([]byte{3, 3, 0, 0, 0, 0, 0, 0}, q...)
	binary.BigEndian.PutUint16(m[2:], csum16(m))
	return ip4(1, src, dst, m, 0)
}

// ---- verification

func ipOK(t *testing.T, pkt []byte, what string) {
	t.Helper()
	if csum16(pkt[:20]) != 0 {
		t.Errorf("%s: IPv4 header checksum is wrong", what)
	}
}

func l4OK(t *testing.T, pkt []byte, what string) {
	t.Helper()
	var s, d [4]byte
	copy(s[:], pkt[12:16])
	copy(d[:], pkt[16:20])
	l4 := pkt[20:]
	if csum16(pseudo(s, d, pkt[9], l4)) != 0 {
		t.Errorf("%s: transport checksum is wrong", what)
	}
}

func addrs(pkt []byte) (src, dst [4]byte) {
	copy(src[:], pkt[12:16])
	copy(dst[:], pkt[16:20])
	return
}

// ---- tests

func TestOutboundTCP(t *testing.T) {
	p := tcp(me, vHost, []byte("hello smb"))
	rewritePacket(p, testMaps, true)
	_, dst := addrs(p)
	if dst != rHost {
		t.Fatalf("dst = %v, want %v", dst, rHost)
	}
	ipOK(t, p, "tcp out")
	l4OK(t, p, "tcp out")
}

func TestInboundTCP(t *testing.T) {
	p := tcp(rHost, me, bytes.Repeat([]byte{0xAB}, 1000))
	rewritePacket(p, testMaps, false)
	src, _ := addrs(p)
	if src != vHost {
		t.Fatalf("src = %v, want %v", src, vHost)
	}
	ipOK(t, p, "tcp in")
	l4OK(t, p, "tcp in")
}

func TestUDPAndZeroChecksum(t *testing.T) {
	p := udp(me, [4]byte{198, 18, 1, 254}, []byte("dns query"), true)
	rewritePacket(p, testMaps, true)
	if _, dst := addrs(p); dst != [4]byte{192, 168, 1, 254} {
		t.Fatalf("udp dst = %v", dst)
	}
	ipOK(t, p, "udp")
	l4OK(t, p, "udp")

	// UDP checksum 0 means "none": it must stay 0
	z := udp(me, vHost, []byte("x"), false)
	rewritePacket(z, testMaps, true)
	if binary.BigEndian.Uint16(z[26:28]) != 0 {
		t.Fatal("zero UDP checksum was changed")
	}
	ipOK(t, z, "udp zero")
}

func TestICMPEcho(t *testing.T) {
	p := icmpEcho(me, vHost)
	rewritePacket(p, testMaps, true)
	ipOK(t, p, "echo")
	if csum16(p[20:]) != 0 {
		t.Fatal("echo ICMP checksum broke")
	}
	if _, dst := addrs(p); dst != rHost {
		t.Fatalf("echo dst = %v", dst)
	}
}

func TestICMPErrorQuotedHeader(t *testing.T) {
	// we sent a packet to the virtual host; the remote answers "port unreachable"
	// quoting what it received: src=me, dst=real host
	orig := udp(me, rHost, []byte("probe"), true)
	p := icmpUnreach(rHost, me, orig)

	rewritePacket(p, testMaps, false) // inbound
	src, _ := addrs(p)
	if src != vHost {
		t.Fatalf("outer src = %v, want %v", src, vHost)
	}
	ipOK(t, p, "icmp err outer")
	if csum16(p[20:]) != 0 {
		t.Fatal("ICMP checksum wrong after rewriting the quoted packet")
	}
	inner := p[28:]
	if csum16(inner[:20]) != 0 {
		t.Fatal("quoted IP header checksum wrong")
	}
	var qd [4]byte
	copy(qd[:], inner[16:20])
	if qd != vHost {
		t.Fatalf("quoted dst = %v, want the virtual address %v", qd, vHost)
	}
}

func TestUntouchedTraffic(t *testing.T) {
	cases := []struct {
		name     string
		pkt      []byte
		outbound bool
	}{
		{"other network, outbound", tcp(me, [4]byte{8, 8, 8, 8}, []byte("x")), true},
		// the real address belongs to the other tunnel, not to us: must not be rewritten
		{"real address, outbound", tcp(me, rHost, []byte("x")), true},
		{"virtual address, inbound", tcp(vHost, me, []byte("x")), false},
	}
	for _, c := range cases {
		before := append([]byte{}, c.pkt...)
		rewritePacket(c.pkt, testMaps, c.outbound)
		if !bytes.Equal(c.pkt, before) {
			t.Errorf("%s: packet was modified", c.name)
		}
	}
}

func TestNonFirstFragment(t *testing.T) {
	// payload bytes that look like a TCP header must not be touched
	body := bytes.Repeat([]byte{0x11}, 40)
	p := ip4(6, me, vHost, body, 0x0010) // fragment offset 16
	rewritePacket(p, testMaps, true)
	if _, dst := addrs(p); dst != rHost {
		t.Fatalf("fragment dst = %v", dst)
	}
	ipOK(t, p, "fragment")
	if !bytes.Equal(p[20:], body) {
		t.Fatal("non-first fragment payload was modified")
	}
}

func TestFirstFragmentKeepsChecksumValid(t *testing.T) {
	// A fragmented TCP segment: the checksum covers the whole segment, but only the
	// first fragment holds the field. Verify by reassembling after rewriting.
	seg := make([]byte, 20+3000)
	binary.BigEndian.PutUint16(seg[0:], 49152)
	binary.BigEndian.PutUint16(seg[2:], 445)
	seg[12] = 0x50
	for i := 20; i < len(seg); i++ {
		seg[i] = byte(i)
	}
	binary.BigEndian.PutUint16(seg[16:], csum16(pseudo(me, vHost, 6, seg)))

	first := ip4(6, me, vHost, seg[:1480], 0x2000) // MF
	second := ip4(6, me, vHost, seg[1480:], 185)   // offset 1480/8
	rewritePacket(first, testMaps, true)
	rewritePacket(second, testMaps, true)

	s, d := addrs(first)
	whole := append(append([]byte{}, first[20:]...), second[20:]...)
	if csum16(pseudo(s, d, 6, whole)) != 0 {
		t.Fatal("reassembled fragmented segment has a wrong checksum")
	}
}

func TestGarbageNeverPanics(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	for i := 0; i < 20000; i++ {
		b := make([]byte, r.Intn(80))
		r.Read(b)
		if len(b) > 0 && r.Intn(2) == 0 {
			b[0] = 0x40 | byte(r.Intn(16)) // look like IPv4 with a random header length
		}
		rewritePacket(b, testMaps, r.Intn(2) == 0)
	}
}

func TestAllocVirtual(t *testing.T) {
	a, freeA, err := allocVirtual(22)
	if err != nil {
		t.Fatal(err)
	}
	b, freeB, err := allocVirtual(22)
	if err != nil {
		t.Fatal(err)
	}
	c, freeC, err := allocVirtual(24)
	if err != nil {
		t.Fatal(err)
	}
	if a.Overlaps(b) || a.Overlaps(c) || b.Overlaps(c) {
		t.Fatalf("overlapping blocks: %s %s %s", a, b, c)
	}
	if !virtPool.Contains(a.Addr()) || a.Masked() != a {
		t.Fatalf("block %s is not aligned inside the pool", a)
	}
	freeA()
	again, freeAgain, _ := allocVirtual(22)
	if again != a {
		t.Fatalf("freed block %s was not reused (got %s)", a, again)
	}
	freeAgain()
	freeB()
	freeC()

	if _, _, err := allocVirtual(8); err == nil {
		t.Fatal("a /8 must not be mappable")
	}
}

func TestPlanMappings(t *testing.T) {
	tunnelAReach := reachPrefixes(cfgOf("10.0.0.3/24", "10.0.0.0/24", "192.168.0.0/22"))
	tunnelB := cfgOf("20.0.0.2/24", "20.0.0.0/24", "192.168.0.0/22")

	maps, release := planMappings(tunnelB, tunnelAReach)
	defer func() {
		for _, f := range release {
			f()
		}
	}()
	if len(maps) != 1 || maps[0].Real != real22 {
		t.Fatalf("expected one mapping for 192.168.0.0/22, got %v", maps)
	}
	if maps[0].Virtual.Bits() != 22 || !virtPool.Contains(maps[0].Virtual.Addr()) {
		t.Fatalf("bad virtual prefix %s", maps[0].Virtual)
	}

	// nothing running -> nothing to map
	if m, _ := planMappings(tunnelB, nil); len(m) != 0 {
		t.Fatalf("mapped without any other tunnel: %v", m)
	}

	// a default route cannot be mapped and must be ignored without failing
	full := &config.Config{Peers: []config.PeerConfig{{AllowedIPs: []netip.Prefix{netip.MustParsePrefix("0.0.0.0/0")}}}}
	if m, _ := planMappings(full, tunnelAReach); len(m) != 0 {
		t.Fatalf("default route was mapped: %v", m)
	}
}
