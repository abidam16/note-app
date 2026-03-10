package auth

import "golang.org/x/crypto/bcrypt"

type PasswordManager struct{}

func NewPasswordManager() PasswordManager {
	return PasswordManager{}
}

func (PasswordManager) Hash(password string) (string, error) {
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hashed), nil
}

func (PasswordManager) Compare(hash, password string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}
