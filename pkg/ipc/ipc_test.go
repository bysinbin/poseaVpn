package ipc

import (
	"testing"
	"time"
)

func TestIPCRoundtrip(t *testing.T) {
	server, err := NewServer()
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}
	defer server.Stop()

	go server.Start()
	time.Sleep(100 * time.Millisecond)

	client := NewClient()
	resp, err := client.SendRequest(Request{Action: "status"})
	if err != nil {
		t.Fatalf("client SendRequest failed: %v", err)
	}

	if !resp.Success {
		t.Fatalf("expected success response, got: %+v", resp)
	}
}
