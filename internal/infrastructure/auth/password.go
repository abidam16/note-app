package auth

import "golang.org/x/crypto/bcrypt"

const defaultBcryptCost = 12

type PasswordManager struct{}

func NewPasswordManager() PasswordManager {
	return PasswordManager{}
}

func (PasswordManager) Hash(password string) (string, error) {
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), defaultBcryptCost)
	if err != nil {
		return "", err
	}
	return string(hashed), nil
}

func (PasswordManager) Compare(hash, password string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}
