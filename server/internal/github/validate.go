package github

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"strconv"
	"strings"
	"time"
)

// OAuthStateTTL bounds how long a user has to complete the GitHub
// authorization after starting it.
const OAuthStateTTL = 10 * time.Minute

// The OAuth state is self-contained and HMAC-signed, so no server-side
// storage is needed:
//
//	base64url(userID "." unixExpiry "." randomNonce) "." hex(HMAC-SHA256)
//
// It binds the callback to the KMJG Hub user who started the flow (GitHub's
// browser redirect carries no KMJG Hub session) and expires after
// OAuthStateTTL. The random nonce makes every state unique.

func newOAuthState(key []byte, userID string, now time.Time) (string, error) {
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	body := userID + "." + strconv.FormatInt(now.Add(OAuthStateTTL).Unix(), 10) + "." + hex.EncodeToString(nonce)
	encoded := base64.RawURLEncoding.EncodeToString([]byte(body))
	return encoded + "." + signState(key, encoded), nil
}

// validateOAuthState verifies state's signature and expiry and returns the
// user ID it was issued to, or ErrInvalidOAuthState.
func validateOAuthState(key []byte, state string, now time.Time) (string, error) {
	if state == "" || len(key) == 0 {
		return "", ErrInvalidOAuthState
	}
	encoded, sig, ok := strings.Cut(state, ".")
	if !ok || !hmac.Equal([]byte(sig), []byte(signState(key, encoded))) {
		return "", ErrInvalidOAuthState
	}
	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return "", ErrInvalidOAuthState
	}
	parts := strings.Split(string(raw), ".")
	if len(parts) != 3 || parts[0] == "" {
		return "", ErrInvalidOAuthState
	}
	expiry, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || !now.Before(time.Unix(expiry, 0)) {
		return "", ErrInvalidOAuthState
	}
	return parts[0], nil
}

func signState(key []byte, encoded string) string {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte("kmjg-github-oauth-state:" + encoded))
	return hex.EncodeToString(mac.Sum(nil))
}

// VerifySignature reports whether signatureHeader (GitHub's
// X-Hub-Signature-256 value, "sha256=<hex>") is the HMAC-SHA256 of payload
// under secret. An empty secret never verifies.
func VerifySignature(secret string, payload []byte, signatureHeader string) bool {
	if secret == "" {
		return false
	}
	hexSig, ok := strings.CutPrefix(signatureHeader, "sha256=")
	if !ok {
		return false
	}
	got, err := hex.DecodeString(hexSig)
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return hmac.Equal(got, mac.Sum(nil))
}

// SignPayload returns the X-Hub-Signature-256 header value GitHub would
// send for payload under secret (used by tests and tooling).
func SignPayload(secret string, payload []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}
