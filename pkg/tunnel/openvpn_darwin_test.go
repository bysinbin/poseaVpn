//go:build darwin

package tunnel

import (
	"testing"
)

func TestFindOpenVPN(t *testing.T) {
	p, err := FindOpenVPNBinary()
	if err != nil {
		t.Fatalf("FindOpenVPNBinary failed: %v", err)
	}
	t.Logf("Found OpenVPN binary at: %s", p)
}
