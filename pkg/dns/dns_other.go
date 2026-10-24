//go:build !windows && !darwin

package dns

import "net"

func ConfigureDNS(interfaceAlias string, servers []net.IP, suffixes []string) error {
	return nil
}

func CleanUpNRPT(interfaceAlias string) error {
	return nil
}
