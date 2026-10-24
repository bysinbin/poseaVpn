//go:build darwin

package dnsproxy

import (
	"syscall"

	"golang.org/x/sys/unix"
)

func bindSocketToInterface(c syscall.RawConn, ifIndex uint32) error {
	var serr error
	err := c.Control(func(fd uintptr) {
		serr = unix.SetsockoptInt(int(fd), unix.IPPROTO_IP, unix.IP_BOUND_IF, int(ifIndex))
	})
	if err != nil {
		return err
	}
	return serr
}
