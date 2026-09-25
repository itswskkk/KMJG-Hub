package github

import (
	"bytes"
	"crypto/rand"
	"strings"
	"testing"
)

func testKey(t *testing.T) []byte {
	t.Helper()
	key := make([]byte, TokenKeySize)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	return key
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	key := testKey(t)
	const token = "gho_exampleAccessToken123"
	ct, err := EncryptToken(token, key)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(ct, []byte(token)) {
		t.Fatal("ciphertext contains plaintext")
	}
	got, err := DecryptToken(ct, key)
	if err != nil || got != token {
		t.Fatalf("round trip: got %q, %v", got, err)
	}

	ct2, _ := EncryptToken(token, key)
	if bytes.Equal(ct, ct2) {
		t.Fatal("two encryptions of the same token must differ (random nonce)")
	}
}

func TestDecryptRejectsTamperingAndWrongKey(t *testing.T) {
	key := testKey(t)
	ct, err := EncryptToken("secret-token", key)
	if err != nil {
		t.Fatal(err)
	}
	for i := range ct {
		tampered := append([]byte(nil), ct...)
		tampered[i] ^= 0x01
		if _, err := DecryptToken(tampered, key); err == nil {
			t.Fatalf("tampered byte %d decrypted successfully", i)
		}
	}
	if _, err := DecryptToken(ct, testKey(t)); err == nil {
		t.Fatal("decrypted with the wrong key")
	}
	if _, err := DecryptToken(ct[:5], key); err == nil {
		t.Fatal("decrypted truncated ciphertext")
	}
}

func TestEncryptRejectsBadKeySize(t *testing.T) {
	if _, err := EncryptToken("x", []byte("short")); err == nil || !strings.Contains(err.Error(), "32 bytes") {
		t.Fatalf("expected key size error, got %v", err)
	}
}
