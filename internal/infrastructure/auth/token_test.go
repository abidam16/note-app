package auth

import (
	"testing"
	"time"
)

func TestTokenManagerAccessTokenRoundTrip(t *testing.T) {
	manager := NewTokenManager("super-secret-token", "note-app", 15*time.Minute)

	token, _, err := manager.GenerateAccessToken("user-1", "user@example.com", time.Now().UTC())
	if err != nil {
		t.Fatalf("GenerateAccessToken() error = %v", err)
	}

	claims, err := manager.ParseAccessToken(token)
	if err != nil {
		t.Fatalf("ParseAccessToken() error = %v", err)
	}

	if claims.Subject != "user-1" {
		t.Fatalf("expected subject user-1, got %s", claims.Subject)
	}
}

func TestTokenManagerRefreshTokenHashing(t *testing.T) {
	manager := NewTokenManager("super-secret-token", "note-app", 15*time.Minute)

	raw, hash, err := manager.NewRefreshToken()
	if err != nil {
		t.Fatalf("NewRefreshToken() error = %v", err)
	}

	if raw == "" || hash == "" {
		t.Fatal("expected refresh token and hash to be populated")
	}

	if manager.HashRefreshToken(raw) != hash {
		t.Fatal("refresh token hash mismatch")
	}
}
