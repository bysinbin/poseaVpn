package transport

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// FallbackClient encapsulates WireGuard UDP packets over WebSocket / TLS (port 443).
// This bypasses corporate firewalls, Next-Gen firewalls, and DPI that block raw UDP traffic.
type FallbackClient struct {
	remoteURL  string
	insecure   bool
	localConn  *net.UDPConn
	localAddr  *net.UDPAddr
	remotePeer *net.UDPAddr
	wsConn     *websocket.Conn
	wsMu       sync.Mutex
	ctx        context.Context
	cancel     context.CancelFunc
	running    bool
	mu         sync.Mutex
}

// NewFallbackClient creates a client that exposes a local UDP endpoint and forwards
// all WireGuard packets to/from a remote WebSocket tunnel.
func NewFallbackClient(remoteWSSURL string, insecureTLS bool) (*FallbackClient, error) {
	u, err := url.Parse(remoteWSSURL)
	if err != nil {
		return nil, fmt.Errorf("invalid websocket URL '%s': %w", remoteWSSURL, err)
	}
	if u.Scheme != "ws" && u.Scheme != "wss" {
		return nil, fmt.Errorf("URL scheme must be ws:// or wss://, got: %s", u.Scheme)
	}

	// Listen on an ephemeral local UDP port (127.0.0.1:0)
	laddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("failed to resolve local address: %w", err)
	}

	udpConn, err := net.ListenUDP("udp", laddr)
	if err != nil {
		return nil, fmt.Errorf("failed to bind local UDP socket: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &FallbackClient{
		remoteURL: remoteWSSURL,
		insecure:  insecureTLS,
		localConn: udpConn,
		localAddr: udpConn.LocalAddr().(*net.UDPAddr),
		ctx:       ctx,
		cancel:    cancel,
	}, nil
}

// LocalEndpoint returns the local 127.0.0.1:port string that WireGuard should point to.
func (c *FallbackClient) LocalEndpoint() string {
	return c.localAddr.String()
}

// Start initiates the WebSocket connection and starts bidirectional forwarding.
func (c *FallbackClient) Start() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.running {
		return nil
	}

	dialer := websocket.Dialer{
		Proxy:            http.ProxyFromEnvironment, // Honors HTTP_PROXY / HTTPS_PROXY
		HandshakeTimeout: 10 * time.Second,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: c.insecure,
		},
	}

	ws, resp, err := dialer.DialContext(c.ctx, c.remoteURL, nil)
	if err != nil {
		status := "no response"
		if resp != nil {
			status = resp.Status
		}
		return fmt.Errorf("failed to dial WebSocket '%s' (%s): %w", c.remoteURL, status, err)
	}

	c.wsConn = ws
	c.running = true

	// Goroutine 1: Read UDP from WireGuard, send to WebSocket
	go c.forwardUDPToWS()

	// Goroutine 2: Read WebSocket from remote gateway, send to WireGuard via UDP
	go c.forwardWSToUDP()

	return nil
}

func (c *FallbackClient) forwardUDPToWS() {
	buf := make([]byte, 65535)
	for {
		select {
		case <-c.ctx.Done():
			return
		default:
		}

		n, srcAddr, err := c.localConn.ReadFromUDP(buf)
		if err != nil {
			if c.ctx.Err() != nil {
				return
			}
			continue
		}

		// Store WireGuard client address to send replies back
		c.mu.Lock()
		c.remotePeer = srcAddr
		c.mu.Unlock()

		c.wsMu.Lock()
		if c.wsConn != nil {
			_ = c.wsConn.SetWriteDeadline(time.Now().Add(5 * time.Second))
			_ = c.wsConn.WriteMessage(websocket.BinaryMessage, buf[:n])
		}
		c.wsMu.Unlock()
	}
}

func (c *FallbackClient) forwardWSToUDP() {
	for {
		select {
		case <-c.ctx.Done():
			return
		default:
		}

		msgType, data, err := c.wsConn.ReadMessage()
		if err != nil {
			if c.ctx.Err() != nil {
				return
			}
			// Reconnect on disconnect
			c.reconnect()
			continue
		}

		if msgType == websocket.BinaryMessage && len(data) > 0 {
			c.mu.Lock()
			peer := c.remotePeer
			c.mu.Unlock()

			if peer != nil {
				_, _ = c.localConn.WriteToUDP(data, peer)
			}
		}
	}
}

func (c *FallbackClient) reconnect() {
	c.wsMu.Lock()
	if c.wsConn != nil {
		_ = c.wsConn.Close()
		c.wsConn = nil
	}
	c.wsMu.Unlock()

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-time.After(2 * time.Second):
		}

		dialer := websocket.Dialer{
			Proxy:            http.ProxyFromEnvironment,
			HandshakeTimeout: 10 * time.Second,
			TLSClientConfig:  &tls.Config{InsecureSkipVerify: c.insecure},
		}

		ws, _, err := dialer.DialContext(c.ctx, c.remoteURL, nil)
		if err == nil {
			c.wsMu.Lock()
			c.wsConn = ws
			c.wsMu.Unlock()
			return
		}
	}
}

// Close stops the fallback client and frees ports.
func (c *FallbackClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.running {
		return nil
	}
	c.running = false
	c.cancel()

	if c.localConn != nil {
		_ = c.localConn.Close()
	}

	c.wsMu.Lock()
	if c.wsConn != nil {
		_ = c.wsConn.Close()
	}
	c.wsMu.Unlock()

	return nil
}

// ==============================================================================
// FallbackServer: Deployed on the WireGuard Gateway to unpack WebSocket frames
// and forward them to the local WireGuard UDP port.
// ==============================================================================

type FallbackServer struct {
	listenAddr    string
	wireguardAddr string
	upgrader      websocket.Upgrader
	httpServer    *http.Server
}

// NewFallbackServer creates a server proxying WebSocket clients to a WireGuard server.
func NewFallbackServer(listenAddr, wireguardUDPAddr string) *FallbackServer {
	return &FallbackServer{
		listenAddr:    listenAddr,
		wireguardAddr: wireguardUDPAddr,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
	}
}

// Handler returns an http.Handler implementing the /wg WebSocket endpoint.
func (s *FallbackServer) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/wg", s.handleWebSocket)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})
	return mux
}

// ListenAndServe starts the HTTP/WebSocket fallback gateway server.
func (s *FallbackServer) ListenAndServe() error {
	s.httpServer = &http.Server{
		Addr:    s.listenAddr,
		Handler: s.Handler(),
	}

	return s.httpServer.ListenAndServe()
}

func (s *FallbackServer) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	ws, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WS upgrade error: %v", err)
		return
	}
	defer ws.Close()

	wgUDP, err := net.ResolveUDPAddr("udp", s.wireguardAddr)
	if err != nil {
		log.Printf("Resolve WG UDP error: %v", err)
		return
	}

	udpConn, err := net.DialUDP("udp", nil, wgUDP)
	if err != nil {
		log.Printf("Dial WG UDP error: %v", err)
		return
	}
	defer udpConn.Close()

	done := make(chan struct{})

	// Read WS -> Write WG UDP
	go func() {
		defer close(done)
		for {
			msgType, data, err := ws.ReadMessage()
			if err != nil {
				return
			}
			if msgType == websocket.BinaryMessage {
				if _, err := udpConn.Write(data); err != nil {
					return
				}
			}
		}
	}()

	// Read WG UDP -> Write WS
	buf := make([]byte, 65535)
	for {
		select {
		case <-done:
			return
		default:
		}

		_ = udpConn.SetReadDeadline(time.Now().Add(10 * time.Second))
		n, err := udpConn.Read(buf)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			return
		}

		if err := ws.WriteMessage(websocket.BinaryMessage, buf[:n]); err != nil {
			return
		}
	}
}
