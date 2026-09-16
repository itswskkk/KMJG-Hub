package auth

import "testing"

func TestGenerateSessionTokenIsUniqueAndNonEmpty(t *testing.T) {
	a, err := GenerateSessionToken()
	if err != nil {
		t.Fatalf("GenerateSessionToken: %v", err)
	}
	b, err := GenerateSessionToken()
	if err != nil {
		t.Fatalf("GenerateSessionToken: %v", err)
	}
	if a == "" || b == "" {
		t.Fatal("expected non-empty tokens")
	}
	if a == b {
		t.Fatal("expected distinct tokens across calls")
	}
}

func TestHashSessionTokenIsDeterministicAndDistinct(t *testing.T) {
	if HashSessionToken("token-a") != HashSessionToken("token-a") {
		t.Fatal("expected same input to hash to the same value")
	}
	if HashSessionToken("token-a") == HashSessionToken("token-b") {
		t.Fatal("expected different inputs to hash to different values")
	}
}
