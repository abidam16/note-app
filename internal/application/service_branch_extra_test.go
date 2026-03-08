package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"note-app/internal/domain"
	appauth "note-app/internal/infrastructure/auth"
)

type failingNotificationPublisher struct {
	err error
}

func (p failingNotificationPublisher) NotifyInvitationCreated(context.Context, domain.WorkspaceInvitation) error {
	return p.err
}

func (p failingNotificationPublisher) NotifyCommentCreated(context.Context, domain.Page, domain.PageComment) error {
	return p.err
}

func TestFolderServiceListFoldersMembershipError(t *testing.T) {
	svc := NewFolderService(&fakeFolderRepo{byID: map[string]domain.Folder{}, byWorkspace: map[string][]domain.Folder{}}, &fakeWorkspaceRepo{memberships: map[string][]domain.WorkspaceMember{}, invitations: map[string]domain.WorkspaceInvitation{}, owners: map[string]int{}})
	if _, err := svc.ListFolders(context.Background(), "u1", "w1"); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected forbidden membership error, got %v", err)
	}
}

func TestWorkspaceServiceInviteErrorBranches(t *testing.T) {
	notifErr := errors.New("notify failed")
	svc := NewWorkspaceService(workspaceRepoStub{
		getMembershipByUserIDFn: func(context.Context, string, string) (domain.WorkspaceMember, error) {
			return domain.WorkspaceMember{Role: domain.RoleOwner}, nil
		},
		getActiveInvitationByEmailFn: func(context.Context, string, string) (domain.WorkspaceInvitation, error) {
			return domain.WorkspaceInvitation{}, errors.New("active invitation query failed")
		},
	}, authUserRepoStub{})

	if _, err := svc.InviteMember(context.Background(), "u1", InviteMemberInput{WorkspaceID: "w1", Email: "a@example.com", Role: domain.RoleViewer}); err == nil || err.Error() != "active invitation query failed" {
		t.Fatalf("expected active invitation query error, got %v", err)
	}

	svc = NewWorkspaceService(workspaceRepoStub{
		getMembershipByUserIDFn: func(context.Context, string, string) (domain.WorkspaceMember, error) {
			return domain.WorkspaceMember{Role: domain.RoleOwner}, nil
		},
		getActiveInvitationByEmailFn: func(context.Context, string, string) (domain.WorkspaceInvitation, error) {
			return domain.WorkspaceInvitation{}, domain.ErrNotFound
		},
		createInvitationFn: func(_ context.Context, inv domain.WorkspaceInvitation) (domain.WorkspaceInvitation, error) {
			return inv, nil
		},
	}, authUserRepoStub{}, failingNotificationPublisher{err: notifErr})

	if _, err := svc.InviteMember(context.Background(), "u1", InviteMemberInput{WorkspaceID: "w1", Email: "a@example.com", Role: domain.RoleViewer}); err == nil || err.Error() != notifErr.Error() {
		t.Fatalf("expected invitation notification error, got %v", err)
	}
}

func TestAuthServiceAdditionalErrorBranches(t *testing.T) {
	pm := appauth.NewPasswordManager()
	tm := appauth.NewTokenManager("super-secret-token", "note-app", 15*time.Minute)

	hash, _ := pm.Hash("Password1")
	svc := NewAuthService(authUserRepoStub{getByEmailFn: func(context.Context, string) (domain.User, error) {
		return domain.User{ID: "u1", Email: "user@example.com", PasswordHash: hash}, nil
	}}, authRefreshRepoStub{}, pm, tm, time.Hour)
	if _, err := svc.Login(context.Background(), LoginInput{Email: "user@example.com", Password: "Wrong123"}); !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Fatalf("expected invalid credentials for wrong password, got %v", err)
	}

	svc = NewAuthService(authUserRepoStub{}, authRefreshRepoStub{getByHashFn: func(context.Context, string) (domain.RefreshToken, error) {
		return domain.RefreshToken{}, errors.New("refresh lookup failed")
	}}, pm, tm, time.Hour)
	if err := svc.Logout(context.Background(), "token"); err == nil || err.Error() != "refresh lookup failed" {
		t.Fatalf("expected logout refresh lookup error, got %v", err)
	}
}

func TestPageServiceResolveFolderHelper(t *testing.T) {
	svc := NewPageService(&pageRepoExtra{}, &fakeWorkspaceRepo{memberships: map[string][]domain.WorkspaceMember{}, invitations: map[string]domain.WorkspaceInvitation{}, owners: map[string]int{}}, &fakeFolderRepo{byID: map[string]domain.Folder{"f1": {ID: "f1", WorkspaceID: "w1", Name: "F"}}, byWorkspace: map[string][]domain.Folder{}})

	blank := "   "
	folderID, err := svc.resolveFolderID(context.Background(), "w1", &blank)
	if err != nil || folderID != nil {
		t.Fatalf("expected blank folder id to resolve to nil, id=%v err=%v", folderID, err)
	}

	wrongWorkspace := "f1"
	if _, err := svc.resolveFolderID(context.Background(), "w2", &wrongWorkspace); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("expected workspace mismatch validation error, got %v", err)
	}
}
