package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"note-app/internal/domain"
	appauth "note-app/internal/infrastructure/auth"
)

func TestAuthServiceRegisterCreateAndRefreshUserLookupErrors(t *testing.T) {
	tm := appauth.NewTokenManager("super-secret-token", "note-app", 15*time.Minute)
	pm := appauth.NewPasswordManager()

	svc := NewAuthService(authUserRepoStub{
		getByEmailFn: func(context.Context, string) (domain.User, error) { return domain.User{}, domain.ErrNotFound },
		createFn: func(context.Context, domain.User) (domain.User, error) {
			return domain.User{}, errors.New("create failed")
		},
	}, authRefreshRepoStub{}, pm, tm, time.Hour)

	if _, err := svc.Register(context.Background(), RegisterInput{Email: "user@example.com", Password: "Password1", FullName: "User"}); err == nil || err.Error() != "create failed" {
		t.Fatalf("expected create failure, got %v", err)
	}

	raw, hash, _ := tm.NewRefreshToken()
	now := time.Now().UTC()
	svc = NewAuthService(authUserRepoStub{getByIDFn: func(context.Context, string) (domain.User, error) {
		return domain.User{}, errors.New("user lookup failed")
	}}, authRefreshRepoStub{
		getByHashFn: func(context.Context, string) (domain.RefreshToken, error) {
			return domain.RefreshToken{ID: "rt-1", UserID: "u1", TokenHash: hash, ExpiresAt: now.Add(time.Hour), CreatedAt: now}, nil
		},
	}, pm, tm, time.Hour)

	if _, err := svc.Refresh(context.Background(), raw); err == nil || err.Error() != "user lookup failed" {
		t.Fatalf("expected user lookup failure from refresh, got %v", err)
	}
}
