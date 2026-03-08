package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"note-app/internal/domain"
	appauth "note-app/internal/infrastructure/auth"
)

func TestAuthServiceRefreshSuccess(t *testing.T) {
	tm := appauth.NewTokenManager("super-secret-token", "note-app", 15*time.Minute)
	pm := appauth.NewPasswordManager()
	user := domain.User{ID: "u1", Email: "user@example.com", FullName: "User", PasswordHash: "hash"}
	users := authUserRepoStub{getByIDFn: func(context.Context, string) (domain.User, error) { return user, nil }}

	raw, hash, err := tm.NewRefreshToken()
	if err != nil {
		t.Fatalf("NewRefreshToken() error = %v", err)
	}
	now := time.Now().UTC()
	refresh := authRefreshRepoStub{
		getByHashFn: func(context.Context, string) (domain.RefreshToken, error) {
			return domain.RefreshToken{ID: "rt-1", UserID: "u1", TokenHash: hash, ExpiresAt: now.Add(time.Hour), CreatedAt: now}, nil
		},
		createFn: func(ctx context.Context, token domain.RefreshToken) (domain.RefreshToken, error) {
			return token, nil
		},
		revokeByIDFn: func(context.Context, string, time.Time) error { return nil },
	}

	svc := NewAuthService(users, refresh, pm, tm, 24*time.Hour)
	result, err := svc.Refresh(context.Background(), raw)
	if err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}
	if result.User.ID != "u1" || result.Tokens.AccessToken == "" || result.Tokens.RefreshToken == "" {
		t.Fatalf("unexpected refresh result: %+v", result)
	}
}

func TestWorkspaceServiceCreateWorkspaceRepoError(t *testing.T) {
	repoErr := errors.New("create workspace failed")
	svc := NewWorkspaceService(workspaceRepoStub{createWithOwnerFn: func(context.Context, domain.Workspace, domain.WorkspaceMember) (domain.Workspace, domain.WorkspaceMember, error) {
		return domain.Workspace{}, domain.WorkspaceMember{}, repoErr
	}}, authUserRepoStub{})

	_, _, err := svc.CreateWorkspace(context.Background(), "u1", CreateWorkspaceInput{Name: "Team"})
	if err == nil || err.Error() != repoErr.Error() {
		t.Fatalf("expected repo error propagation, got %v", err)
	}
}
