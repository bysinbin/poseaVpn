// Package dnsproxy is a small per-tunnel DNS forwarder.
//
// Problem it solves: two tunnels can use the same address for their DNS server
// (e.g. both VPN servers are 10.0.0.1). Windows routes by destination only, so a
// query for 10.0.0.1 always goes to one tunnel and the other tunnel's DNS is
// unreachable. The proxy listens on a private loopback address (unique per
// tunnel) and forwards queries to the real DNS server through a socket that is
// forced onto the tunnel's own adapter (IP_UNICAST_IF), ignoring the route table.
package dnsproxy

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"math/bits"
	"net"
	"net/netip"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
)

const (
	ipUnicastIf  = 31 // IP_UNICAST_IF (ws2ipdef.h)
	queryTimeout = 4 * time.Second
	tcpIdle      = 10 * time.Second
)

// Proxy forwards DNS queries received on Listen to Upstream through the adapter IfIndex.
type Proxy struct {
	Listen   netip.Addr // loopback address Windows is told to use as DNS server
	Upstream netip.Addr // address the queries are sent to (the tunnel's DNS server)
	Source   netip.Addr // tunnel interface address used as source
	IfIndex  uint32     // tunnel adapter interface index

	// MapA, when set, is applied to every A record of every reply. It turns the
	// real addresses of a remapped network into the virtual addresses Windows must use.
	MapA func(netip.Addr) (netip.Addr, bool)

	udp    *net.UDPConn
	tcp    net.Listener
	wg     sync.WaitGroup
	closed chan struct{}
	once   sync.Once
}

// Start begins listening on Listen:53 (UDP and TCP).
func Start(listen, upstream, source netip.Addr, ifIndex uint32, mapA func(netip.Addr) (netip.Addr, bool)) (*Proxy, error) {
	p := &Proxy{Listen: listen, Upstream: upstream, Source: source, IfIndex: ifIndex, MapA: mapA, closed: make(chan struct{})}

	addr := net.JoinHostPort(listen.String(), "53")
	udp, err := net.ListenPacket("udp", addr)
	if err != nil {
		return nil, fmt.Errorf("dns proxy: listen udp %s: %w", addr, err)
	}
	tcp, err := net.Listen("tcp", addr)
	if err != nil {
		udp.Close()
		return nil, fmt.Errorf("dns proxy: listen tcp %s: %w", addr, err)
	}
	p.udp = udp.(*net.UDPConn)
	p.tcp = tcp

	p.wg.Add(2)
	go p.serveUDP()
	go p.serveTCP()
	log.Printf("[DNSProxy] %s -> %s via ifIndex %d (src %s)", listen, upstream, ifIndex, source)
	return p, nil
}

// Close stops the proxy.
func (p *Proxy) Close() {
	p.once.Do(func() {
		close(p.closed)
		_ = p.udp.Close()
		_ = p.tcp.Close()
		p.wg.Wait()
	})
}

// control forces a socket onto the tunnel adapter and ignores the route table.
func (p *Proxy) control(network, address string, c syscall.RawConn) error {
	var serr error
	err := c.Control(func(fd uintptr) {
		// For IPv4 the interface index must be passed in network byte order.
		serr = windows.SetsockoptInt(windows.Handle(fd), windows.IPPROTO_IP, ipUnicastIf, int(bits.ReverseBytes32(p.IfIndex)))
	})
	if err != nil {
		return err
	}
	return serr
}

func (p *Proxy) dialer(network string) *net.Dialer {
	d := &net.Dialer{Timeout: queryTimeout, Control: p.control}
	switch network {
	case "udp":
		d.LocalAddr = &net.UDPAddr{IP: p.Source.AsSlice()}
	case "tcp":
		d.LocalAddr = &net.TCPAddr{IP: p.Source.AsSlice()}
	}
	return d
}

// ---------------------------------------------------------------- UDP

func (p *Proxy) serveUDP() {
	defer p.wg.Done()
	buf := make([]byte, 65535)
	for {
		n, client, err := p.udp.ReadFromUDP(buf)
		if err != nil {
			select {
			case <-p.closed:
				return
			default:
				continue
			}
		}
		query := append([]byte(nil), buf[:n]...)
		p.wg.Add(1)
		go func() {
			defer p.wg.Done()
			reply, err := p.forwardUDP(query)
			if err != nil {
				log.Printf("[DNSProxy] udp %s -> %s failed: %v", p.Listen, p.Upstream, err)
				return // the client retries; nothing useful to send back
			}
			_, _ = p.udp.WriteToUDP(reply, client)
		}()
	}
}

func (p *Proxy) forwardUDP(query []byte) ([]byte, error) {
	conn, err := p.dialer("udp").DialContext(context.Background(), "udp", net.JoinHostPort(p.Upstream.String(), "53"))
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(queryTimeout))
	if _, err := conn.Write(query); err != nil {
		return nil, err
	}
	buf := make([]byte, 65535)
	n, err := conn.Read(buf)
	if err != nil {
		return nil, err
	}
	return RewriteA(buf[:n], p.MapA), nil
}

// ---------------------------------------------------------------- TCP

func (p *Proxy) serveTCP() {
	defer p.wg.Done()
	for {
		c, err := p.tcp.Accept()
		if err != nil {
			select {
			case <-p.closed:
				return
			default:
				continue
			}
		}
		p.wg.Add(1)
		go func() {
			defer p.wg.Done()
			p.handleTCP(c)
		}()
	}
}

// handleTCP relays length-prefixed DNS messages (RFC 1035 4.2.2); several per connection.
func (p *Proxy) handleTCP(client net.Conn) {
	defer client.Close()
	for {
		_ = client.SetReadDeadline(time.Now().Add(tcpIdle))
		var hdr [2]byte
		if _, err := io.ReadFull(client, hdr[:]); err != nil {
			return
		}
		msg := make([]byte, binary.BigEndian.Uint16(hdr[:]))
		if _, err := io.ReadFull(client, msg); err != nil {
			return
		}
		reply, err := p.forwardTCP(msg)
		if err != nil {
			return
		}
		out := make([]byte, 2+len(reply))
		binary.BigEndian.PutUint16(out, uint16(len(reply)))
		copy(out[2:], reply)
		_ = client.SetWriteDeadline(time.Now().Add(queryTimeout))
		if _, err := client.Write(out); err != nil {
			return
		}
	}
}

var errShort = errors.New("short dns reply")

func (p *Proxy) forwardTCP(msg []byte) ([]byte, error) {
	conn, err := p.dialer("tcp").DialContext(context.Background(), "tcp", net.JoinHostPort(p.Upstream.String(), "53"))
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(queryTimeout))

	out := make([]byte, 2+len(msg))
	binary.BigEndian.PutUint16(out, uint16(len(msg)))
	copy(out[2:], msg)
	if _, err := conn.Write(out); err != nil {
		return nil, err
	}
	var hdr [2]byte
	if _, err := io.ReadFull(conn, hdr[:]); err != nil {
		return nil, err
	}
	l := int(binary.BigEndian.Uint16(hdr[:]))
	if l == 0 {
		return nil, errShort
	}
	reply := make([]byte, l)
	if _, err := io.ReadFull(conn, reply); err != nil {
		return nil, err
	}
	return RewriteA(reply, p.MapA), nil
}
