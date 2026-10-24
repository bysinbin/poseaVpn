package tunnel

import (
	"strconv"
	"strings"
	"time"

	"golang.zx2c4.com/wireguard/conn"
)

// udpPortFree reports whether WireGuard itself could bind the UDP port right now.
// It uses WireGuard's own bind: a plain net.ListenPacket probe wrongly reports a
// port as free while a WireGuard socket already holds it.
func udpPortFree(port int) bool {
	b := conn.NewDefaultBind()
	_, _, err := b.Open(uint16(port))
	if err != nil {
		return false
	}
	_ = b.Close()
	return true
}

// Stats are live counters read from the WireGuard device.
type Stats struct {
	RxBytes       uint64
	TxBytes       uint64
	LastHandshake time.Time // zero if no handshake yet
}

// Stats returns aggregated peer counters (sum of bytes, newest handshake).
func (t *Tunnel) Stats() Stats {
	t.mu.Lock()
	dev := t.device
	t.mu.Unlock()

	var s Stats
	if dev == nil {
		return s
	}
	out, err := dev.IpcGet()
	if err != nil {
		return s
	}

	var sec int64
	for _, line := range strings.Split(out, "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		switch k {
		case "rx_bytes":
			if n, err := strconv.ParseUint(v, 10, 64); err == nil {
				s.RxBytes += n
			}
		case "tx_bytes":
			if n, err := strconv.ParseUint(v, 10, 64); err == nil {
				s.TxBytes += n
			}
		case "last_handshake_time_sec":
			if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > sec {
				sec = n
			}
		}
	}
	if sec > 0 {
		s.LastHandshake = time.Unix(sec, 0)
	}
	return s
}
