package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
)

// tokenByteLen is the amount of randomness in a generated session token.
// docs/ARCHITECTURE.md "Session Security" requires sufficient entropy to
// prevent practical guessing; 32 bytes (256 bits) is well beyond that bar.
const tokenByteLen = 32

// GenerateSessionToken returns a new cryptographically secure, URL-safe
// opaque session token to hand to the Client.
func GenerateSessionToken() (string, error) {
	buf := make([]byte, tokenByteLen)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate session token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// HashSessionToken returns the value the Server persists and looks sessions
// up by. The Server never stores the raw bearer token, mirroring how
// passwords are hashed rather than stored in plain text.
func HashSessionToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
