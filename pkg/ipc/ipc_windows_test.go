//go:build windows

package ipc

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/Microsoft/go-winio"
)

func TestNamedPipeRoundtrip(t *testing.T) {
	testPipe := `\\.\pipe\poseavpn-test-pipe`
	cfg := &winio.PipeConfig{
		SecurityDescriptor: PipeSDDL,
		InputBufferSize:    4096,
		OutputBufferSize:   4096,
	}

	l, err := winio.ListenPipe(testPipe, cfg)
	if err != nil {
		t.Fatalf("ListenPipe failed: %v", err)
	}
	defer l.Close()

	// Server handler goroutine
	go func() {
		conn, err := l.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		var req Request
		_ = json.NewDecoder(conn).Decode(&req)

		_ = json.NewEncoder(conn).Encode(Response{
			Success: true,
			Message: "Echo: " + req.Action,
		})
	}()

	// Client dials
	timeout := 2 * time.Second
	conn, err := winio.DialPipe(testPipe, &timeout)
	if err != nil {
		t.Fatalf("DialPipe failed: %v", err)
	}
	defer conn.Close()

	if err := json.NewEncoder(conn).Encode(Request{Action: "ping"}); err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	var resp Response
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		t.Fatalf("Decode failed: %v", err)
	}

	if !resp.Success || resp.Message != "Echo: ping" {
		t.Fatalf("Unexpected response: %+v", resp)
	}
}
