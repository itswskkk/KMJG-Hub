package github

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
)

// TokenKeySize is the required encryption key length (AES-256).
const TokenKeySize = 32

var errCiphertextTooShort = errors.New("github: ciphertext too short")

// EncryptToken encrypts plaintext with AES-256-GCM under key. The random
// nonce is prepended to the returned ciphertext.
func EncryptToken(plaintext string, key []byte) ([]byte, error) {
	gcm, err := newGCM(key)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("github: generate nonce: %w", err)
	}
	return gcm.Seal(nonce, nonce, []byte(plaintext), nil), nil
}

// DecryptToken reverses EncryptToken. It fails if the ciphertext was
// tampered with or encrypted under a different key.
func DecryptToken(ciphertext []byte, key []byte) (string, error) {
	gcm, err := newGCM(key)
	if err != nil {
		return "", err
	}
	if len(ciphertext) < gcm.NonceSize() {
		return "", errCiphertextTooShort
	}
	nonce, sealed := ciphertext[:gcm.NonceSize()], ciphertext[gcm.NonceSize():]
	plaintext, err := gcm.Open(nil, nonce, sealed, nil)
	if err != nil {
		return "", fmt.Errorf("github: decrypt token: %w", err)
	}
	return string(plaintext), nil
}

func newGCM(key []byte) (cipher.AEAD, error) {
	if len(key) != TokenKeySize {
		return nil, fmt.Errorf("github: encryption key must be %d bytes, got %d", TokenKeySize, len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}
