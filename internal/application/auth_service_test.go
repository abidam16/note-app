package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"note-app/internal/domain"
	appauth "note-app/internal/infrastructure/auth"
)

type fakeUserRepo struct {
	byEmail map[string]domain.User
	byID    map[string]domain.User
}

func (r *fakeUserRepo) Create(_ context.Context, user domain.User) (domain.User, error) {
	if _, exists := r.byEmail[user.Email]; exists {
		return domain.User{}, domain.ErrEmailAlreadyUsed
	}
	r.byEmail[user.Email] = user
	r.byID[user.ID] = user
	return user, nil
}

func (r *fakeUserRepo) GetByEmail(_ context.Context, email string) (domain.User, error) {
	user, ok := r.byEmail[email]
	if !ok {
		return domain.User{}, domain.ErrNotFound
	}
	return user, nil
}

func (r *fakeUserRepo) GetByID(_ context.Context, userID string) (domain.User, error) {
	user, ok := r.byID[userID]
	if !ok {
		return domain.User{}, domain.ErrNotFound
	}
	return user, nil
}

type fakeRefreshTokenRepo struct {
	tokens map[string]domain.RefreshToken
}

func (r *fakeRefreshTokenRepo) Create(_ context.Context, token domain.RefreshToken) (domain.RefreshToken, error) {
	r.tokens[token.TokenHash] = token
	return token, nil
}

func (r *fakeRefreshTokenRepo) GetByHash(_ context.Context, hash string) (domain.RefreshToken, error) {
	token, ok := r.tokens[hash]
	if !ok {
		return domain.RefreshToken{}, domain.ErrNotFound
	}
	return token, nil
}

func (r *fakeRefreshTokenRepo) RevokeByID(_ context.Context, tokenID string, revokedAt time.Time) error {
	for hash, token := range r.tokens {
		if token.ID == tokenID {
			token.RevokedAt = &revokedAt
			r.tokens[hash] = token
			return nil
		}
	}
	return domain.ErrNotFound
}

func TestAuthServiceRegisterAndLogin(t *testing.T) {
	users := &fakeUserRepo{byEmail: map[string]domain.User{}, byID: map[string]domain.User{}}
	refreshTokens := &fakeRefreshTokenRepo{tokens: map[string]domain.RefreshToken{}}
	service := NewAuthService(users, refreshTokens, appauth.NewPasswordManager(), appauth.NewTokenManager("super-secret-token", "note-app", 15*time.Minute), 24*time.Hour)

	created, err := service.Register(context.Background(), RegisterInput{
		Email:    "user@example.com",
		Password: "Password1",
		FullName: "Test User",
	})
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	if created.PasswordHash != "" {
		t.Fatal("expected sanitized user response")
	}

	result, err := service.Login(context.Background(), LoginInput{Email: "user@example.com", Password: "Password1"})
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}

	if result.Tokens.AccessToken == "" || result.Tokens.RefreshToken == "" {
		t.Fatal("expected tokens to be returned")
	}
}

func TestAuthServiceRejectsWeakPassword(t *testing.T) {
	users := &fakeUserRepo{byEmail: map[string]domain.User{}, byID: map[string]domain.User{}}
	refreshTokens := &fakeRefreshTokenRepo{tokens: map[string]domain.RefreshToken{}}
	service := NewAuthService(users, refreshTokens, appauth.NewPasswordManager(), appauth.NewTokenManager("super-secret-token", "note-app", 15*time.Minute), 24*time.Hour)

	_, err := service.Register(context.Background(), RegisterInput{
		Email:    "user@example.com",
		Password: "weak",
		FullName: "Test User",
	})
	if !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("expected validation error, got %v", err)
	}
}
