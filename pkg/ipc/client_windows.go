//go:build windows

package ipc

import (
	"fmt"
	"net"
	"time"

	"github.com/Microsoft/go-winio"
)

func dialIPC(timeout time.Duration) (net.Conn, error) {
	conn, err := winio.DialPipe(PipePath, &timeout)
	if err != nil {
		return nil, fmt.Errorf("could not connect to PoseaVPN service: %w (is the service installed and running?)", err)
	}
	return conn, nil
}
