package dnsproxy

import (
	"bytes"
	"encoding/binary"
	"math/rand"
	"net/netip"
	"testing"
)

var (
	realNet = netip.MustParsePrefix("192.168.0.0/22")
	virtNet = netip.MustParsePrefix("198.18.0.0/22")
)

func mapA(a netip.Addr) (netip.Addr, bool) {
	if !realNet.Contains(a) {
		return a, false
	}
	b := a.As4()
	v := virtNet.Addr().As4()
	// same host part inside the /22
	host := (uint32(b[2])<<8 | uint32(b[3])) & 0x3FF
	n := uint32(v[0])<<24 | uint32(v[1])<<16 | uint32(v[2])<<8 | uint32(v[3])
	n += host
	return netip.AddrFrom4([4]byte{byte(n >> 24), byte(n >> 16), byte(n >> 8), byte(n)}), true
}

func name(parts ...string) []byte {
	var b []byte
	for _, p := range parts {
		b = append(b, byte(len(p)))
		b = append(b, p...)
	}
	return append(b, 0)
}

func rr(nameBytes []byte, typ uint16, rdata []byte) []byte {
	b := append([]byte{}, nameBytes...)
	b = binary.BigEndian.AppendUint16(b, typ)
	b = binary.BigEndian.AppendUint16(b, 1) // IN
	b = binary.BigEndian.AppendUint32(b, 60)
	b = binary.BigEndian.AppendUint16(b, uint16(len(rdata)))
	return append(b, rdata...)
}

func reply() []byte {
	m := make([]byte, 12)
	binary.BigEndian.PutUint16(m[0:], 0xBEEF)
	binary.BigEndian.PutUint16(m[2:], 0x8180)
	binary.BigEndian.PutUint16(m[4:], 1)  // questions
	binary.BigEndian.PutUint16(m[6:], 3)  // answers
	binary.BigEndian.PutUint16(m[10:], 1) // additional

	q := name("fs01", "corp", "local")
	m = append(m, q...)
	m = append(m, 0, 1, 0, 1)

	ptr := []byte{0xC0, 12}                                      // compression pointer to the question name
	m = append(m, rr(ptr, 5, name("alias", "corp", "local"))...) // CNAME: must be skipped correctly
	m = append(m, rr(ptr, 1, []byte{192, 168, 1, 5})...)         // A inside the remapped network
	m = append(m, rr(ptr, 1, []byte{192, 168, 3, 250})...)       // second A, end of the /22
	m = append(m, rr(ptr, 1, []byte{8, 8, 8, 8})...)             // additional: unrelated address
	return m
}

func TestRewriteA(t *testing.T) {
	m := reply()
	out := RewriteA(append([]byte{}, m...), mapA)

	if len(out) != len(m) {
		t.Fatal("message length changed")
	}
	want := func(b []byte, ip [4]byte) bool { return bytes.Contains(b, ip[:]) }
	if !want(out, [4]byte{198, 18, 1, 5}) {
		t.Error("192.168.1.5 was not mapped to 198.18.1.5")
	}
	if !want(out, [4]byte{198, 18, 3, 250}) {
		t.Error("192.168.3.250 was not mapped to 198.18.3.250")
	}
	if want(out, [4]byte{192, 168, 1, 5}) || want(out, [4]byte{192, 168, 3, 250}) {
		t.Error("a real address is still present")
	}
	if !want(out, [4]byte{8, 8, 8, 8}) {
		t.Error("an unrelated address was changed")
	}
	// everything except the three address fields must be identical
	diff := 0
	for i := range m {
		if m[i] != out[i] {
			diff++
		}
	}
	if diff > 6 {
		t.Errorf("%d bytes changed, expected only the mapped address bytes", diff)
	}
}

func TestRewriteANoMapper(t *testing.T) {
	m := reply()
	out := RewriteA(append([]byte{}, m...), nil)
	if !bytes.Equal(m, out) {
		t.Fatal("message changed without a mapper")
	}
}

func TestRewriteAMalformedNeverPanics(t *testing.T) {
	r := rand.New(rand.NewSource(7))
	good := reply()
	for i := 0; i < 30000; i++ {
		var b []byte
		if r.Intn(2) == 0 {
			b = append([]byte{}, good[:r.Intn(len(good)+1)]...) // truncated real message
		} else {
			b = make([]byte, r.Intn(120))
			r.Read(b)
		}
		RewriteA(b, mapA)
	}
}
