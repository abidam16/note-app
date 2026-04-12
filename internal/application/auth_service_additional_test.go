package application

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"note-app/internal/domain"
	appauth "note-app/internal/infrastructure/auth"
)

type authUserRepoStub struct {
	createFn     func(ctx context.Context, user domain.User) (domain.User, error)
	getByEmailFn func(ctx context.Context, email string) (domain.User, error)
	getByIDFn    func(ctx context.Context, userID string) (domain.User, error)
}

func (s authUserRepoStub) Create(ctx context.Context, user domain.User) (domain.User, error) {
	if s.createFn != nil {
		return s.createFn(ctx, user)
	}
	return user, nil
}

func (s authUserRepoStub) GetByEmail(ctx context.Context, email string) (domain.User, error) {
	if s.getByEmailFn != nil {
		return s.getByEmailFn(ctx, email)
	}
	return domain.User{}, domain.ErrNotFound
}

func (s authUserRepoStub) GetByID(ctx context.Context, userID string) (domain.User, error) {
	if s.getByIDFn != nil {
		return s.getByIDFn(ctx, userID)
	}
	return domain.User{}, domain.ErrNotFound
}

type authRefreshRepoStub struct {
	createFn     func(ctx context.Context, token domain.RefreshToken) (domain.RefreshToken, error)
	getByHashFn  func(ctx context.Context, hash string) (domain.RefreshToken, error)
	revokeByIDFn func(ctx context.Context, tokenID string, revokedAt time.Time) error
}

func (s authRefreshRepoStub) Create(ctx context.Context, token domain.RefreshToken) (domain.RefreshToken, error) {
	if s.createFn != nil {
		return s.createFn(ctx, token)
	}
	return token, nil
}

func (s authRefreshRepoStub) GetByHash(ctx context.Context, hash string) (domain.RefreshToken, error) {
	if s.getByHashFn != nil {
		return s.getByHashFn(ctx, hash)
	}
	return domain.RefreshToken{}, domain.ErrNotFound
}

func (s authRefreshRepoStub) RevokeByID(ctx context.Context, tokenID string, revokedAt time.Time) error {
	if s.revokeByIDFn != nil {
		return s.revokeByIDFn(ctx, tokenID, revokedAt)
	}
	return nil
}

func TestAuthHelpers(t *testing.T) {
	email, err := normalizeEmail("  USER@Example.com ")
	if err != nil {
		t.Fatalf("normalizeEmail should succeed: %v", err)
	}
	if email != "user@example.com" {
		t.Fatalf("unexpected normalized email: %s", email)
	}

	if _, err := normalizeEmail("not-an-email"); err == nil {
		t.Fatal("normalizeEmail should fail for invalid email")
	}

	if err := validatePassword("short"); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("expected validation error for short password, got %v", err)
	}
	if err := validatePassword("allletters"); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("expected validation error for missing digit, got %v", err)
	}
	if err := validatePassword("12345678"); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("expected validation error for missing letter, got %v", err)
	}
	if err := validatePassword("password1"); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("expected validation error for missing uppercase, got %v", err)
	}
	if err := validatePassword("PASSWORD1"); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("expected validation error for missing lowercase, got %v", err)
	}
	if err := validatePassword("Passw0rd"); err != nil {
		t.Fatalf("expected valid password, got %v", err)
	}

	sanitized := sanitizeUser(domain.User{ID: "u1", Email: "u@example.com", PasswordHash: "secret"})
	if sanitized.PasswordHash != "" {
		t.Fatal("sanitizeUser should clear password hash")
	}
}

func TestAuthServiceAdditionalBranches(t *testing.T) {
	tm := appauth.NewTokenManager("super-secret-token", "note-app", 15*time.Minute)
	pm := appauth.NewPasswordManager()

	hash, err := pm.Hash("Password1")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	user := domain.User{ID: "user-1", Email: "user@example.com", FullName: "User", PasswordHash: hash}

	t.Run("register propagates unexpected lookup error", func(t *testing.T) {
		svc := NewAuthService(authUserRepoStub{getByEmailFn: func(context.Context, string) (domain.User, error) {
			return domain.User{}, errors.New("db")
		}}, authRefreshRepoStub{}, pm, tm, time.Hour)

		_, err := svc.Register(context.Background(), RegisterInput{Email: user.Email, Password: "Password1", FullName: "User"})
		if err == nil || !strings.Contains(err.Error(), "db") {
			t.Fatalf("expected db error propagation, got %v", err)
		}
	})

	t.Run("register rejects blank full name", func(t *testing.T) {
		svc := NewAuthService(authUserRepoStub{}, authRefreshRepoStub{}, pm, tm, time.Hour)
		_, err := svc.Register(context.Background(), RegisterInput{Email: user.Email, Password: "Password1", FullName: "   "})
		if !errors.Is(err, domain.ErrValidation) {
			t.Fatalf("expected validation error, got %v", err)
		}
	})

	t.Run("login invalid email and db error", func(t *testing.T) {
		svc := NewAuthService(authUserRepoStub{}, authRefreshRepoStub{}, pm, tm, time.Hour)
		if _, err := svc.Login(context.Background(), LoginInput{Email: "bad", Password: "x"}); !errors.Is(err, domain.ErrInvalidCredentials) {
			t.Fatalf("expected invalid credentials for bad email, got %v", err)
		}

		svc = NewAuthService(authUserRepoStub{getByEmailFn: func(context.Context, string) (domain.User, error) {
			return domain.User{}, errors.New("read error")
		}}, authRefreshRepoStub{}, pm, tm, time.Hour)
		if _, err := svc.Login(context.Background(), LoginInput{Email: user.Email, Password: "Password1"}); err == nil || !strings.Contains(err.Error(), "read error") {
			t.Fatalf("expected read error propagation, got %v", err)
		}
	})

	t.Run("refresh unauthorized revoked expired and revoke failure", func(t *testing.T) {
		svc := NewAuthService(authUserRepoStub{}, authRefreshRepoStub{}, pm, tm, time.Hour)
		if _, err := svc.Refresh(context.Background(), "missing"); !errors.Is(err, domain.ErrUnauthorized) {
			t.Fatalf("expected unauthorized for missing token, got %v", err)
		}

		raw, hash, _ := tm.NewRefreshToken()
		now := time.Now().UTC()
		revokedAt := now
		revokedToken := domain.RefreshToken{ID: "rt-1", UserID: user.ID, TokenHash: hash, ExpiresAt: now.Add(time.Hour), RevokedAt: &revokedAt, CreatedAt: now}
		svc = NewAuthService(authUserRepoStub{}, authRefreshRepoStub{getByHashFn: func(context.Context, string) (domain.RefreshToken, error) {
			return revokedToken, nil
		}}, pm, tm, time.Hour)
		if _, err := svc.Refresh(context.Background(), raw); !errors.Is(err, domain.ErrUnauthorized) {
			t.Fatalf("expected unauthorized for revoked token, got %v", err)
		}

		expiredToken := domain.RefreshToken{ID: "rt-2", UserID: user.ID, TokenHash: hash, ExpiresAt: now.Add(-time.Hour), CreatedAt: now}
		svc = NewAuthService(authUserRepoStub{}, authRefreshRepoStub{getByHashFn: func(context.Context, string) (domain.RefreshToken, error) {
			return expiredToken, nil
		}}, pm, tm, time.Hour)
		if _, err := svc.Refresh(context.Background(), raw); !errors.Is(err, domain.ErrTokenExpired) {
			t.Fatalf("expected token expired, got %v", err)
		}

		activeToken := domain.RefreshToken{ID: "rt-3", UserID: user.ID, TokenHash: hash, ExpiresAt: now.Add(time.Hour), CreatedAt: now}
		svc = NewAuthService(authUserRepoStub{getByIDFn: func(context.Context, string) (domain.User, error) {
			return user, nil
		}}, authRefreshRepoStub{
			getByHashFn:  func(context.Context, string) (domain.RefreshToken, error) { return activeToken, nil },
			revokeByIDFn: func(context.Context, string, time.Time) error { return errors.New("revoke failed") },
		}, pm, tm, time.Hour)
		if _, err := svc.Refresh(context.Background(), raw); err == nil || !strings.Contains(err.Error(), "revoke failed") {
			t.Fatalf("expected revoke failure propagation, got %v", err)
		}
	})

	t.Run("login returns token creation error when refresh create fails", func(t *testing.T) {
		svc := NewAuthService(authUserRepoStub{getByEmailFn: func(context.Context, string) (domain.User, error) {
			return user, nil
		}}, authRefreshRepoStub{createFn: func(context.Context, domain.RefreshToken) (domain.RefreshToken, error) {
			return domain.RefreshToken{}, errors.New("create refresh failed")
		}}, pm, tm, time.Hour)

		if _, err := svc.Login(context.Background(), LoginInput{Email: user.Email, Password: "Password1"}); err == nil || !strings.Contains(err.Error(), "create refresh failed") {
			t.Fatalf("expected refresh create failure, got %v", err)
		}
	})

	t.Run("logout and current user branches", func(t *testing.T) {
		now := time.Now().UTC()
		token := domain.RefreshToken{ID: "rt-4", UserID: user.ID, TokenHash: "h", ExpiresAt: now.Add(time.Hour), CreatedAt: now}

		svc := NewAuthService(authUserRepoStub{getByIDFn: func(context.Context, string) (domain.User, error) {
			return domain.User{}, errors.New("lookup failed")
		}}, authRefreshRepoStub{getByHashFn: func(context.Context, string) (domain.RefreshToken, error) {
			return token, nil
		}, revokeByIDFn: func(context.Context, string, time.Time) error {
			return errors.New("revoke failed")
		}}, pm, tm, time.Hour)

		raw, _, _ := tm.NewRefreshToken()
		if err := svc.Logout(context.Background(), raw); err == nil || !strings.Contains(err.Error(), "revoke failed") {
			t.Fatalf("expected logout revoke failure, got %v", err)
		}

		svc = NewAuthService(authUserRepoStub{}, authRefreshRepoStub{}, pm, tm, time.Hour)
		if err := svc.Logout(context.Background(), "missing"); err != nil {
			t.Fatalf("logout should ignore missing token: %v", err)
		}

		if _, err := svc.CurrentUser(context.Background(), "missing"); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("expected not found current user, got %v", err)
		}
	})
}
