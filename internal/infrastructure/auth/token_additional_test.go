package auth

import (
	"testing"
	"time"
)

func TestTokenManagerParseFailures(t *testing.T) {
	now := time.Now().UTC()
	manager := NewTokenManager("super-secret-token", "note-app", time.Minute)

	token, _, err := manager.GenerateAccessToken("user-1", "user@example.com", now)
	if err != nil {
		t.Fatalf("GenerateAccessToken() error = %v", err)
	}

	other := NewTokenManager("different-secret", "note-app", time.Minute)
	if _, err := other.ParseAccessToken(token); err == nil {
		t.Fatal("expected parse to fail with different secret")
	}

	if _, err := manager.ParseAccessToken("not-a-token"); err == nil {
		t.Fatal("expected parse to fail for malformed token")
	}

	expiredManager := NewTokenManager("super-secret-token", "note-app", -time.Minute)
	expiredToken, _, err := expiredManager.GenerateAccessToken("user-1", "user@example.com", now)
	if err != nil {
		t.Fatalf("GenerateAccessToken(expired) error = %v", err)
	}
	if _, err := manager.ParseAccessToken(expiredToken); err == nil {
		t.Fatal("expected parse to fail for expired token")
	}
}

func TestTokenManagerHashRefreshTokenDeterministic(t *testing.T) {
	manager := NewTokenManager("super-secret-token", "note-app", time.Minute)
	h1 := manager.HashRefreshToken("abc")
	h2 := manager.HashRefreshToken("abc")
	h3 := manager.HashRefreshToken("xyz")
	if h1 != h2 {
		t.Fatal("expected deterministic hash for same input")
	}
	if h1 == h3 {
		t.Fatal("expected different hash for different input")
	}
}
