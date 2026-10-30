package config

import (
	"encoding/base64"
	"testing"
)

func TestGenerateKeyPair(t *testing.T) {
	kp, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}

	if len(kp.PrivateKey) != 44 {
		t.Errorf("expected 44-char base64 private key, got %d chars: %s", len(kp.PrivateKey), kp.PrivateKey)
	}
	if len(kp.PublicKey) != 44 {
		t.Errorf("expected 44-char base64 public key, got %d chars: %s", len(kp.PublicKey), kp.PublicKey)
	}

	// Verify that deriving public key from private key matches
	pub, err := PublicKeyFromPrivate(kp.PrivateKey)
	if err != nil {
		t.Fatalf("PublicKeyFromPrivate failed: %v", err)
	}
	if pub != kp.PublicKey {
		t.Errorf("public key mismatch: got %s, expected %s", pub, kp.PublicKey)
	}
}

func TestWireGuardTestVector(t *testing.T) {
	// Exact vector from official WireGuard Windows client screenshot
	priv := "IH5IlwVR8+kNiMX/jY0/WCFiymasetxwKF94/pAhqG4="
	expectedPub := "SzMh2nu8zhvONTjew8RWMfmjZfq6rNaQznS43DEylSI="

	pub, err := PublicKeyFromPrivate(priv)
	if err != nil {
		t.Fatalf("PublicKeyFromPrivate failed: %v", err)
	}
	if pub != expectedPub {
		t.Errorf("public key mismatch: got %s, expected %s", pub, expectedPub)
	}
}

func TestInvalidKeys(t *testing.T) {
	if _, err := PublicKeyFromPrivate(""); err == nil {
		t.Error("expected error for empty key")
	}
	if _, err := PublicKeyFromPrivate("not-base64!!!"); err == nil {
		t.Error("expected error for non-base64 string")
	}
	short := base64.StdEncoding.EncodeToString([]byte("too short"))
	if _, err := PublicKeyFromPrivate(short); err == nil {
		t.Error("expected error for short key")
	}
}

func TestGeneratePresharedKey(t *testing.T) {
	psk, err := GeneratePresharedKey()
	if err != nil {
		t.Fatalf("GeneratePresharedKey failed: %v", err)
	}
	if len(psk) != 44 {
		t.Errorf("expected 44-char base64 preshared key, got %d chars: %s", len(psk), psk)
	}
}
