//go:build linux

package dnsproxy

import (
	"net"
	"syscall"

	"golang.org/x/sys/unix"
)

func bindSocketToInterface(c syscall.RawConn, ifIndex uint32) error {
	iface, err := net.InterfaceByIndex(int(ifIndex))
	if err != nil {
		return err
	}
	var serr error
	err = c.Control(func(fd uintptr) {
		serr = unix.BindToDevice(int(fd), iface.Name)
	})
	if err != nil {
		return err
	}
	return serr
}
