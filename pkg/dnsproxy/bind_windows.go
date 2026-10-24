//go:build windows

package dnsproxy

import (
	"math/bits"
	"syscall"

	"golang.org/x/sys/windows"
)

const ipUnicastIf = 31 // IP_UNICAST_IF (ws2ipdef.h)

func bindSocketToInterface(c syscall.RawConn, ifIndex uint32) error {
	var serr error
	err := c.Control(func(fd uintptr) {
		// For IPv4 the interface index must be passed in network byte order.
		serr = windows.SetsockoptInt(windows.Handle(fd), windows.IPPROTO_IP, ipUnicastIf, int(bits.ReverseBytes32(ifIndex)))
	})
	if err != nil {
		return err
	}
	return serr
}
