package ipc

import (
	"poseavpn/pkg/tunnel"
)

// PipePath is the Windows Named Pipe path for PoseaVPN IPC communication.
const PipePath = `\\.\pipe\poseavpn`

// PipeSDDL allows Authenticated Users (Domain Users), LocalSystem, and Builtin Administrators.
const PipeSDDL = "D:(A;;GRGW;;;AU)(A;;GA;;;BA)(A;;GA;;;SY)"

// SocketPath is the Unix Domain Socket path on macOS/Linux.
const SocketPath = "/var/run/poseavpn.sock"

// FallbackSocketPath is used when /var/run is not writable (e.g. non-root debugging/testing).
const FallbackSocketPath = "/tmp/poseavpn.sock"

// Request sent from a standard non-admin client to the SYSTEM/root service.
//
// Actions:
//
//	connect         start tunnel Name with ConfigData (several tunnels may run at once)
//	disconnect      stop tunnel Name; empty Name stops all tunnels
//	status          list every active tunnel (Response.Tunnels)
type Request struct {
	Action     string `json:"action"`
	Name       string `json:"name,omitempty"`
	ConfigData string `json:"config_data,omitempty"`
	Proto      string `json:"proto,omitempty"` // "wireguard" or "openvpn" (empty = auto)
	Username   string `json:"username,omitempty"`
	Password   string `json:"password,omitempty"`
}

// Response sent from the service back to the client.
type Response struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
	Log     string `json:"log,omitempty"`
	// Status holds the tunnel affected by connect, or the first active tunnel for
	// "status" (kept so older single-tunnel clients keep working).
	Status *StatusInfo `json:"status,omitempty"`
	// Tunnels lists all active tunnels for "status".
	Tunnels []StatusInfo `json:"tunnels,omitempty"`
}

// StatusInfo represents the current state of one tunnel.
type StatusInfo = tunnel.StatusInfo
