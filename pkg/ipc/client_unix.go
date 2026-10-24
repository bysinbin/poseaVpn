//go:build !windows

package ipc

import (
	"fmt"
	"net"
	"time"
)

func dialIPC(timeout time.Duration) (net.Conn, error) {
	conn, err := net.DialTimeout("unix", SocketPath, timeout)
	if err == nil {
		return conn, nil
	}
	// Try fallback user socket path
	conn, err2 := net.DialTimeout("unix", FallbackSocketPath, timeout)
	if err2 == nil {
		return conn, nil
	}
	return nil, fmt.Errorf("could not connect to PoseaVPN service: %w (is the service installed and running?)", err)
}
