//go:build !windows

package ipc

import (
	"fmt"
	"net"
	"os"
)

var activeSocketPath = SocketPath

func listenIPC() (net.Listener, func(), error) {
	// Try system SocketPath first (/var/run/poseavpn.sock)
	sockPath := SocketPath
	_ = os.Remove(sockPath)

	l, err := net.Listen("unix", sockPath)
	if err != nil {
		// Fallback to /tmp/poseavpn.sock if /var/run is not writable
		sockPath = FallbackSocketPath
		_ = os.Remove(sockPath)
		l, err = net.Listen("unix", sockPath)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create unix domain socket at %s or %s: %w", SocketPath, FallbackSocketPath, err)
		}
	}

	activeSocketPath = sockPath
	// Allow non-root users (Domain Users / Standard Users) to talk to the daemon without sudo
	_ = os.Chmod(sockPath, 0o666)

	cleanup := func() {
		_ = os.Remove(sockPath)
	}

	return l, cleanup, nil
}

func ipcEndpointDesc() string {
	return activeSocketPath + " (All Users allowed)"
}
