//go:build windows

package proxy

import (
	"bytes"
	"testing"

	"github.com/google/uuid"
)

func TestWindowsProxyProtectionRoundTripsAndRejectsSwappedOrDamagedCiphertext(t *testing.T) {
	ref := uuid.NewString()
	plain := []byte(`{"username":"SYNTHETIC_DPAPI_USER","password":"SYNTHETIC_DPAPI_PASSWORD"}`)
	protected, err := Protect(ref, plain)
	if err != nil || bytes.Contains(protected, []byte("SYNTHETIC_DPAPI")) {
		t.Fatal("Windows protection unavailable or credentials left in plaintext")
	}
	restored, err := Unprotect(ref, protected)
	if err != nil || !bytes.Equal(restored, plain) {
		t.Fatal("same-user DPAPI round trip changed credentials")
	}
	Wipe(restored)
	if _, err := Unprotect(uuid.NewString(), protected); err == nil {
		t.Fatal("protected ref was swappable")
	}
	damaged := append([]byte(nil), protected...)
	damaged[len(damaged)-1] ^= 0x40
	if _, err := Unprotect(ref, damaged); err == nil {
		t.Fatal("tampered ciphertext decrypted successfully")
	}
}
