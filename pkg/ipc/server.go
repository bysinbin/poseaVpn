package ipc

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/netip"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Microsoft/go-winio"

	"poseavpn/pkg/config"
	"poseavpn/pkg/ovpn"
	"poseavpn/pkg/tunnel"
)

// Server handles IPC requests from non-admin desktop clients.
type Server struct {
	listener net.Listener
	tunnels  map[string]tunnel.Instance // keyed by lower-case tunnel name
	mu       sync.Mutex
	stopCh   chan struct{}
}

// NewServer initializes the IPC server with AD Domain User accessible DACL.
func NewServer() (*Server, error) {
	cfg := &winio.PipeConfig{
		SecurityDescriptor: PipeSDDL,
		InputBufferSize:    65536,
		OutputBufferSize:   65536,
	}

	l, err := winio.ListenPipe(PipePath, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create named pipe listener at %s: %w", PipePath, err)
	}

	return &Server{
		listener: l,
		tunnels:  make(map[string]tunnel.Instance),
		stopCh:   make(chan struct{}),
	}, nil
}

// Start begins accepting IPC requests from clients.
func (s *Server) Start() {
	log.Printf("[IPC Server] Listening for client requests on %s (Domain Users allowed)", PipePath)

	for {
		conn, err := s.listener.Accept()
		if err != nil {
			select {
			case <-s.stopCh:
				return
			default:
				log.Printf("[IPC Server] Accept error: %v", err)
				continue
			}
		}

		go s.handleConnection(conn)
	}
}

// Stop shuts down the IPC server and every active tunnel.
func (s *Server) Stop() {
	close(s.stopCh)
	if s.listener != nil {
		_ = s.listener.Close()
	}

	s.mu.Lock()
	for key, t := range s.tunnels {
		_ = t.Stop()
		delete(s.tunnels, key)
	}
	s.mu.Unlock()
}

func tunnelKey(name string) string { return strings.ToLower(strings.TrimSpace(name)) }

// pruneDead drops tunnels that stopped on their own. Caller must hold s.mu.
func (s *Server) pruneDead() {
	for key, t := range s.tunnels {
		if !t.IsRunning() {
			delete(s.tunnels, key)
		}
	}
}

// conflictWith reports why cfg cannot run next to the already active tunnels ("" = ok).
// Caller must hold s.mu.
func (s *Server) conflictWith(cfg *config.Config) string {
	mine := make(map[netip.Addr]bool)
	for _, a := range cfg.Interface.Addresses {
		mine[a.Addr()] = true
	}
	for _, inst := range s.tunnels {
		if t, ok := inst.(*tunnel.Tunnel); ok && t.Config != nil {
			for _, a := range t.Config.Interface.Addresses {
				if mine[a.Addr()] {
					return fmt.Sprintf("address %s is already used by active tunnel '%s'", a.Addr(), t.Name)
				}
			}
		}
	}
	return ""
}

func statusOf(t tunnel.Instance) StatusInfo {
	return t.Status()
}

func (s *Server) handleConnection(conn net.Conn) {
	defer conn.Close()

	decoder := json.NewDecoder(conn)
	encoder := json.NewEncoder(conn)

	var req Request
	if err := decoder.Decode(&req); err != nil {
		if err != io.EOF {
			_ = encoder.Encode(Response{
				Success: false,
				Message: fmt.Sprintf("invalid request JSON: %v", err),
			})
		}
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneDead()

	switch strings.ToLower(req.Action) {
	case "connect":
		name := strings.TrimSpace(req.Name)
		if name == "" {
			name = "PoseaVPN"
		}
		key := tunnelKey(name)

		if _, exists := s.tunnels[key]; exists {
			_ = encoder.Encode(Response{
				Success: false,
				Message: fmt.Sprintf("tunnel '%s' is already active", name),
			})
			return
		}
		if req.ConfigData == "" {
			_ = encoder.Encode(Response{Success: false, Message: "missing configuration data"})
			return
		}

		isOpenVPN := req.Proto == "openvpn" || ovpn.IsOpenVPN(req.ConfigData)
		if isOpenVPN {
			// Start OpenVPN instance
			ovpnTunnel, err := tunnel.StartOpenVPN(name, req.ConfigData, req.Username, req.Password)
			if err != nil {
				_ = encoder.Encode(Response{
					Success: false,
					Message: fmt.Sprintf("%v", err),
				})
				return
			}
			s.tunnels[key] = ovpnTunnel
			info := ovpnTunnel.Status()
			_ = encoder.Encode(Response{
				Success: true,
				Message: fmt.Sprintf("OpenVPN tüneli '%s' başarıyla bağlandı", name),
				Status:  &info,
			})
			return
		}

		cfg, err := config.Parse(strings.NewReader(req.ConfigData))
		if err != nil {
			_ = encoder.Encode(Response{
				Success: false,
				Message: fmt.Sprintf("failed to parse configuration: %v", err),
			})
			return
		}

		if reason := s.conflictWith(cfg); reason != "" {
			_ = encoder.Encode(Response{Success: false, Message: reason})
			return
		}
		// what the running tunnels really claim in Windows (remapped networks count as their virtual prefix)
		var otherReach []netip.Prefix
		for _, other := range s.tunnels {
			otherReach = append(otherReach, other.Reach()...)
		}
		t, err := tunnel.StartWithOptions(name, cfg, tunnel.Options{OtherReach: otherReach})
		if err != nil {
			_ = encoder.Encode(Response{
				Success: false,
				Message: fmt.Sprintf("failed to bring up tunnel: %v", err),
			})
			return
		}
		s.tunnels[key] = t

		msg := fmt.Sprintf("tunnel '%s' connected successfully", name)
		if len(t.Mappings) > 0 {
			var parts []string
			for _, m := range t.Mappings {
				parts = append(parts, m.String())
			}
			msg += "; address mapping: " + strings.Join(parts, ", ")
		}
		info := statusOf(t)
		_ = encoder.Encode(Response{Success: true, Message: msg, Status: &info})

	case "disconnect":
		if strings.TrimSpace(req.Name) == "" {
			n := len(s.tunnels)
			for key, t := range s.tunnels {
				_ = t.Stop()
				delete(s.tunnels, key)
			}
			_ = encoder.Encode(Response{
				Success: true,
				Message: fmt.Sprintf("%d tunnel(s) disconnected", n),
			})
			return
		}

		key := tunnelKey(req.Name)
		t, ok := s.tunnels[key]
		if !ok {
			_ = encoder.Encode(Response{
				Success: true,
				Message: fmt.Sprintf("tunnel '%s' is not active", req.Name),
			})
			return
		}
		_ = t.Stop()
		delete(s.tunnels, key)
		_ = encoder.Encode(Response{
			Success: true,
			Message: fmt.Sprintf("tunnel '%s' disconnected", t.TunnelName()),
		})

	case "status":
		list := make([]StatusInfo, 0, len(s.tunnels))
		for _, t := range s.tunnels {
			list = append(list, statusOf(t))
		}
		sort.Slice(list, func(i, j int) bool {
			return strings.ToLower(list[i].TunnelName) < strings.ToLower(list[j].TunnelName)
		})

		resp := Response{Success: true, Tunnels: list}
		if len(list) > 0 {
			first := list[0]
			resp.Status = &first
		} else {
			resp.Status = &StatusInfo{Active: false}
		}
		_ = encoder.Encode(resp)

	case "log":
		name := strings.TrimSpace(req.Name)
		logData := ""
		if name != "" {
			if t, ok := s.tunnels[tunnelKey(name)]; ok {
				if ov, isOvpn := t.(*tunnel.OpenVPNTunnel); isOvpn {
					logData = ov.Log()
				}
			}
			if logData == "" {
				p := tunnel.OpenVPNLogPath(name)
				if data, err := os.ReadFile(p); err == nil {
					logData = string(data)
				}
			}
		}
		_ = encoder.Encode(Response{
			Success: true,
			Log:     logData,
		})

	case "stop_service":
		_ = encoder.Encode(Response{
			Success: true,
			Message: "service stopping",
		})
		go func() {
			time.Sleep(200 * time.Millisecond)
			s.Stop()
			os.Exit(0)
		}()
		return

	default:
		_ = encoder.Encode(Response{
			Success: false,
			Message: fmt.Sprintf("unknown action '%s'", req.Action),
		})
	}
}
