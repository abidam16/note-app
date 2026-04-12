package auth

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
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

func TestTokenManagerRejectsWrongIssuer(t *testing.T) {
	now := time.Now().UTC()
	manager := NewTokenManager("super-secret-token", "note-app", time.Minute)
	otherIssuer := NewTokenManager("super-secret-token", "other-app", time.Minute)

	token, _, err := otherIssuer.GenerateAccessToken("user-1", "user@example.com", now)
	if err != nil {
		t.Fatalf("GenerateAccessToken() error = %v", err)
	}

	if _, err := manager.ParseAccessToken(token); err == nil {
		t.Fatal("expected parse to fail for wrong issuer")
	}
}

func TestTokenManagerRejectsDifferentHMACAlgorithm(t *testing.T) {
	now := time.Now().UTC()
	claims := Claims{
		Email: "user@example.com",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "user-1",
			Issuer:    "note-app",
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Minute)),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS384, claims)
	signed, err := token.SignedString([]byte("super-secret-token"))
	if err != nil {
		t.Fatalf("sign hs384 token: %v", err)
	}

	manager := NewTokenManager("super-secret-token", "note-app", time.Minute)
	if _, err := manager.ParseAccessToken(signed); err == nil {
		t.Fatal("expected parse to fail for non-HS256 HMAC token")
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
