//go:build !windows && !darwin

package tunnel

import (
	"fmt"
	"golang.zx2c4.com/wireguard/tun"
)

func createTUN(adapterName string, mtu int) (tun.Device, string, error) {
	return nil, "", fmt.Errorf("platform not supported")
}

func configureOSNetwork(t *Tunnel, opts Options) error {
	return fmt.Errorf("platform not supported")
}

func teardownOSNetwork(t *Tunnel) {}
