package ipc

import (
	"encoding/json"
	"fmt"
	"time"
)

// Client communicates with the PoseaVPN background service.
type Client struct {
	timeout time.Duration
}

// NewClient creates a new IPC client.
func NewClient() *Client {
	return &Client{
		timeout: 10 * time.Second,
	}
}

// SendRequest sends a JSON request to the background service and returns the response.
func (c *Client) SendRequest(req Request) (*Response, error) {
	conn, err := dialIPC(c.timeout)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	if err := json.NewEncoder(conn).Encode(req); err != nil {
		return nil, fmt.Errorf("failed to send request to service: %w", err)
	}

	var resp Response
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		return nil, fmt.Errorf("failed to read response from service: %w", err)
	}

	return &resp, nil
}

// Connect sends a connect request to establish the specified tunnel.
func (c *Client) Connect(tunnelName, configData string) (*Response, error) {
	return c.SendRequest(Request{
		Action:     "connect",
		Name:       tunnelName,
		ConfigData: configData,
	})
}

// ConnectWithAuth sends a connect request with protocol, username and password.
func (c *Client) ConnectWithAuth(tunnelName, configData, proto, username, password string) (*Response, error) {
	return c.SendRequest(Request{
		Action:     "connect",
		Name:       tunnelName,
		ConfigData: configData,
		Proto:      proto,
		Username:   username,
		Password:   password,
	})
}

// Disconnect tears down every active tunnel.
func (c *Client) Disconnect() (*Response, error) {
	return c.SendRequest(Request{
		Action: "disconnect",
	})
}

// DisconnectTunnel tears down a single tunnel by name.
func (c *Client) DisconnectTunnel(name string) (*Response, error) {
	return c.SendRequest(Request{
		Action: "disconnect",
		Name:   name,
	})
}

// List returns all active tunnels (empty slice when none).
func (c *Client) List() ([]StatusInfo, error) {
	resp, err := c.SendRequest(Request{Action: "status"})
	if err != nil {
		return nil, err
	}
	if !resp.Success {
		return nil, fmt.Errorf("service error: %s", resp.Message)
	}
	return resp.Tunnels, nil
}

// Status returns the first active tunnel, or Active=false when none is up.
func (c *Client) Status() (*StatusInfo, error) {
	resp, err := c.SendRequest(Request{
		Action: "status",
	})
	if err != nil {
		return nil, err
	}
	if !resp.Success {
		return nil, fmt.Errorf("service error: %s", resp.Message)
	}
	return resp.Status, nil
}

// GetLog returns the log output for a tunnel.
func (c *Client) GetLog(name string) (string, error) {
	resp, err := c.SendRequest(Request{
		Action: "log",
		Name:   name,
	})
	if err != nil {
		return "", err
	}
	return resp.Log, nil
}

// StopService requests the background service to terminate gracefully.
func (c *Client) StopService() error {
	_, err := c.SendRequest(Request{
		Action: "stop_service",
	})
	return err
}
