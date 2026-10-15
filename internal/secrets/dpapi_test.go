package secrets

import (
	"bytes"
	"testing"
)

func TestDPAPIRoundTripUserScope(t *testing.T) {
	c := NewCipher(false)
	secret := []byte("s3cret-proxy-password")
	ct, err := c.Protect(secret)
	if err != nil {
		t.Skipf("DPAPI unavailable: %v", err)
	}
	if bytes.Equal(ct, secret) {
		t.Fatal("ciphertext equals plaintext")
	}
	pt, err := c.Unprotect(ct)
	if err != nil {
		t.Fatalf("unprotect: %v", err)
	}
	if !bytes.Equal(pt, secret) {
		t.Fatalf("round trip mismatch: %q", pt)
	}
}

func TestDPAPIRoundTripMachineScope(t *testing.T) {
	c := NewCipher(true)
	secret := []byte("machine-scoped-secret")
	ct, err := c.Protect(secret)
	if err != nil {
		t.Skipf("DPAPI unavailable: %v", err)
	}
	pt, err := c.Unprotect(ct)
	if err != nil {
		t.Fatalf("unprotect: %v", err)
	}
	if !bytes.Equal(pt, secret) {
		t.Fatalf("round trip mismatch: %q", pt)
	}
}
