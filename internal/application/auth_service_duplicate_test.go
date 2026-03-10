package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"note-app/internal/domain"
	appauth "note-app/internal/infrastructure/auth"
)

func TestAuthServiceRegisterDuplicateEmail(t *testing.T) {
	svc := NewAuthService(authUserRepoStub{getByEmailFn: func(context.Context, string) (domain.User, error) {
		return domain.User{ID: "u1", Email: "dup@example.com"}, nil
	}}, authRefreshRepoStub{}, appauth.NewPasswordManager(), appauth.NewTokenManager("super-secret-token", "note-app", 15*time.Minute), time.Hour)

	if _, err := svc.Register(context.Background(), RegisterInput{Email: "dup@example.com", Password: "Password1", FullName: "Dup"}); !errors.Is(err, domain.ErrEmailAlreadyUsed) {
		t.Fatalf("expected email already used, got %v", err)
	}
}
