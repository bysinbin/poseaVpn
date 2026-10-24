package transport

import (
	"bytes"
	"net"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestFallbackRoundtrip(t *testing.T) {
	// 1. Mock remote WireGuard UDP server
	mockWgAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("ResolveUDPAddr failed: %v", err)
	}
	mockWgConn, err := net.ListenUDP("udp", mockWgAddr)
	if err != nil {
		t.Fatalf("ListenUDP failed: %v", err)
	}
	defer mockWgConn.Close()

	actualWgAddr := mockWgConn.LocalAddr().String()

	// 2. Start HTTP/WS Fallback Server using httptest
	server := NewFallbackServer("", actualWgAddr)
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/wg"

	// 3. Start Fallback Client
	client, err := NewFallbackClient(wsURL, true)
	if err != nil {
		t.Fatalf("NewFallbackClient failed: %v", err)
	}
	defer client.Close()

	if err := client.Start(); err != nil {
		t.Fatalf("client.Start failed: %v", err)
	}

	// 4. Test UDP client sending to Fallback Client local endpoint
	clientLocalUDP, err := net.ResolveUDPAddr("udp", client.LocalEndpoint())
	if err != nil {
		t.Fatalf("Resolve client endpoint failed: %v", err)
	}

	testSender, err := net.DialUDP("udp", nil, clientLocalUDP)
	if err != nil {
		t.Fatalf("DialUDP to fallback client failed: %v", err)
	}
	defer testSender.Close()

	testPayload := []byte("PoseaVPN-Fallback-Ping")
	if _, err := testSender.Write(testPayload); err != nil {
		t.Fatalf("Write test payload failed: %v", err)
	}

	// 5. Mock WG server reads the packet
	recvBuf := make([]byte, 1024)
	_ = mockWgConn.SetReadDeadline(time.Now().Add(3 * time.Second))
	n, clientFromWgAddr, err := mockWgConn.ReadFromUDP(recvBuf)
	if err != nil {
		t.Fatalf("Mock WG server failed to receive payload: %v", err)
	}

	if !bytes.Equal(recvBuf[:n], testPayload) {
		t.Fatalf("Payload mismatch: got %s, want %s", string(recvBuf[:n]), string(testPayload))
	}

	// 6. Mock WG server responds
	replyPayload := []byte("PoseaVPN-Fallback-Pong")
	if _, err := mockWgConn.WriteToUDP(replyPayload, clientFromWgAddr); err != nil {
		t.Fatalf("Mock WG server reply failed: %v", err)
	}

	// 7. Test sender receives reply
	replyBuf := make([]byte, 1024)
	_ = testSender.SetReadDeadline(time.Now().Add(3 * time.Second))
	rn, err := testSender.Read(replyBuf)
	if err != nil {
		t.Fatalf("Test sender failed to receive reply: %v", err)
	}

	if !bytes.Equal(replyBuf[:rn], replyPayload) {
		t.Fatalf("Reply mismatch: got %s, want %s", string(replyBuf[:rn]), string(replyPayload))
	}
}
