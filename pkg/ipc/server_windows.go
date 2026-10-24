//go:build windows

package ipc

import (
	"fmt"
	"net"

	"github.com/Microsoft/go-winio"
)

func listenIPC() (net.Listener, func(), error) {
	cfg := &winio.PipeConfig{
		SecurityDescriptor: PipeSDDL,
		InputBufferSize:    65536,
		OutputBufferSize:   65536,
	}

	l, err := winio.ListenPipe(PipePath, cfg)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create named pipe listener at %s: %w", PipePath, err)
	}

	cleanup := func() {}
	return l, cleanup, nil
}

func ipcEndpointDesc() string {
	return PipePath + " (Domain Users allowed)"
}
