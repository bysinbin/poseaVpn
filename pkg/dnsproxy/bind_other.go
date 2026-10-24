//go:build !windows && !darwin && !linux

package dnsproxy

import "syscall"

func bindSocketToInterface(c syscall.RawConn, ifIndex uint32) error {
	return nil
}
