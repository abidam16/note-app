package auth

import (
	"testing"
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
}
