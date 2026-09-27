package secretbox

import (
	"encoding/hex"
	"strings"
	"testing"
)

func TestSealOpen(t *testing.T) {
	key := hex.EncodeToString([]byte(strings.Repeat("k", 32)))
	box, err := NewFromHex(key)
	if err != nil {
		t.Fatalf("NewFromHex: %v", err)
	}

	ciphertext, nonce, err := box.Seal([]byte("secret-token"))
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	if string(ciphertext) == "secret-token" {
		t.Fatal("ciphertext contains plaintext")
	}

	plaintext, err := box.Open(ciphertext, nonce)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if got, want := string(plaintext), "secret-token"; got != want {
		t.Fatalf("plaintext = %q, want %q", got, want)
	}
}

func TestInvalidKeyLength(t *testing.T) {
	if _, err := NewFromHex("abcd"); err == nil {
		t.Fatal("expected invalid key length error")
	}
}
