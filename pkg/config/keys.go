package config

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/curve25519"
)

// KeyPair holds a matched WireGuard private and public key encoded in standard base64.
type KeyPair struct {
	PrivateKey string `json:"privateKey"`
	PublicKey  string `json:"publicKey"`
}

// clamp applies Curve25519 / WireGuard bit clamping to a 32-byte private key.
func clamp(key []byte) {
	if len(key) < 32 {
		return
	}
	key[0] &= 248
	key[31] = (key[31] & 127) | 64
}

// GenerateKeyPair generates a cryptographically secure WireGuard private/public key pair.
func GenerateKeyPair() (KeyPair, error) {
	var priv [32]byte
	if _, err := rand.Read(priv[:]); err != nil {
		return KeyPair{}, fmt.Errorf("failed to generate random bytes: %w", err)
	}

	clamp(priv[:])

	pub, err := curve25519.X25519(priv[:], curve25519.Basepoint)
	if err != nil {
		return KeyPair{}, fmt.Errorf("failed to compute public key: %w", err)
	}

	return KeyPair{
		PrivateKey: base64.StdEncoding.EncodeToString(priv[:]),
		PublicKey:  base64.StdEncoding.EncodeToString(pub),
	}, nil
}

// PublicKeyFromPrivate computes the WireGuard public key from a base64-encoded private key.
func PublicKeyFromPrivate(privKeyB64 string) (string, error) {
	privKeyB64 = strings.TrimSpace(privKeyB64)
	if privKeyB64 == "" {
		return "", errors.New("private key is empty")
	}

	data, err := base64.StdEncoding.DecodeString(privKeyB64)
	if err != nil {
		return "", fmt.Errorf("invalid base64 private key: %w", err)
	}

	if len(data) != 32 {
		return "", fmt.Errorf("private key must be exactly 32 bytes (got %d)", len(data))
	}

	clamp(data)

	pub, err := curve25519.X25519(data, curve25519.Basepoint)
	if err != nil {
		return "", fmt.Errorf("failed to compute public key: %w", err)
	}

	return base64.StdEncoding.EncodeToString(pub), nil
}

// GeneratePresharedKey generates a cryptographically secure 32-byte pre-shared key (PSK).
func GeneratePresharedKey() (string, error) {
	var psk [32]byte
	if _, err := rand.Read(psk[:]); err != nil {
		return "", fmt.Errorf("failed to generate preshared key: %w", err)
	}
	return base64.StdEncoding.EncodeToString(psk[:]), nil
}
