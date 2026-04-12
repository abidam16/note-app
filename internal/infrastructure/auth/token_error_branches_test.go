package auth

import (
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestPasswordManagerHashTooLongReturnsError(t *testing.T) {
	m := NewPasswordManager()
	tooLong := strings.Repeat("a", 73)
	if _, err := m.Hash(tooLong); err == nil {
		t.Fatal("expected hash error for password length > 72 bytes")
	}
}

func TestTokenManagerNewRefreshTokenReadFailure(t *testing.T) {
	manager := NewTokenManager("secret", "note-app", time.Minute)
	manager.readRandom = func(_ []byte) (int, error) {
		return 0, errors.New("forced rand failure")
	}

	if _, _, err := manager.NewRefreshToken(); err == nil {
		t.Fatal("expected error when rand reader fails")
	}
}

func TestTokenManagerGenerateAccessTokenSignerFailure(t *testing.T) {
	manager := NewTokenManager("secret", "note-app", time.Minute)
	manager.sign = func(_ *jwt.Token, _ []byte) (string, error) {
		return "", errors.New("forced sign failure")
	}

	if _, _, err := manager.GenerateAccessToken("user-1", "user@example.com", time.Now().UTC()); err == nil {
		t.Fatal("expected error when signer fails")
	}
}

func TestTokenManagerParseUnexpectedSigningMethod(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}

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

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	signed, err := token.SignedString(key)
	if err != nil {
		t.Fatalf("sign rs256 token: %v", err)
	}

	manager := NewTokenManager("secret", "note-app", time.Minute)
	if _, err := manager.ParseAccessToken(signed); err == nil || !strings.Contains(err.Error(), "signing method RS256 is invalid") {
		t.Fatalf("expected invalid signing method error, got %v", err)
	}
}
