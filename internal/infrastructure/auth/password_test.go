package auth

import (
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestPasswordManagerHashAndCompare(t *testing.T) {
	m := NewPasswordManager()
	hash, err := m.Hash("strong-password")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	if hash == "strong-password" {
		t.Fatal("hash should not equal raw password")
	}
	if err := m.Compare(hash, "strong-password"); err != nil {
		t.Fatalf("compare should succeed: %v", err)
	}
	if err := m.Compare(hash, "wrong"); err == nil {
		t.Fatal("compare should fail for wrong password")
	}

	cost, err := bcrypt.Cost([]byte(hash))
	if err != nil {
		t.Fatalf("read bcrypt cost: %v", err)
	}
	if cost != defaultBcryptCost {
		t.Fatalf("unexpected bcrypt cost: got %d want %d", cost, defaultBcryptCost)
	}
}
