package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"note-app/internal/domain"
)

type notificationRepoStub struct {
	createFn   func(context.Context, domain.Notification) (domain.Notification, error)
	listFn     func(context.Context, string) ([]domain.Notification, error)
	markReadFn func(context.Context, string, string, time.Time) (domain.Notification, error)
}

func (s notificationRepoStub) Create(ctx context.Context, n domain.Notification) (domain.Notification, error) {
	if s.createFn != nil {
		return s.createFn(ctx, n)
	}
	return n, nil
}
func (s notificationRepoStub) ListByUserID(ctx context.Context, userID string) ([]domain.Notification, error) {
	if s.listFn != nil {
		return s.listFn(ctx, userID)
	}
	return nil, nil
}
func (s notificationRepoStub) MarkRead(ctx context.Context, notificationID, userID string, readAt time.Time) (domain.Notification, error) {
	if s.markReadFn != nil {
		return s.markReadFn(ctx, notificationID, userID, readAt)
	}
	return domain.Notification{}, nil
}

type membershipListStub struct {
	members []domain.WorkspaceMember
	err     error
}

func (s membershipListStub) GetMembershipByUserID(context.Context, string, string) (domain.WorkspaceMember, error) {
	return domain.WorkspaceMember{}, nil
}
func (s membershipListStub) ListMembers(context.Context, string) ([]domain.WorkspaceMember, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.members, nil
}

func TestNotificationServiceAdditionalBranches(t *testing.T) {
	t.Run("list and mark read propagate repository errors", func(t *testing.T) {
		svc := NewNotificationService(notificationRepoStub{
			listFn: func(context.Context, string) ([]domain.Notification, error) { return nil, errors.New("list failed") },
			markReadFn: func(context.Context, string, string, time.Time) (domain.Notification, error) {
				return domain.Notification{}, errors.New("mark failed")
			},
		}, authUserRepoStub{}, membershipListStub{})

		if _, err := svc.ListNotifications(context.Background(), "u1"); err == nil || err.Error() != "list failed" {
			t.Fatalf("expected list error propagation, got %v", err)
		}
		if _, err := svc.MarkNotificationRead(context.Background(), "u1", "n1"); err == nil || err.Error() != "mark failed" {
			t.Fatalf("expected mark error propagation, got %v", err)
		}
	})

	t.Run("invitation notification user lookup and create paths", func(t *testing.T) {
		svc := NewNotificationService(notificationRepoStub{}, authUserRepoStub{getByEmailFn: func(context.Context, string) (domain.User, error) {
			return domain.User{}, errors.New("lookup failed")
		}}, membershipListStub{})
		if err := svc.NotifyInvitationCreated(context.Background(), domain.WorkspaceInvitation{ID: "inv", WorkspaceID: "w", Email: "x@example.com"}); err == nil || err.Error() != "lookup failed" {
			t.Fatalf("expected lookup error propagation, got %v", err)
		}

		svc = NewNotificationService(notificationRepoStub{createFn: func(context.Context, domain.Notification) (domain.Notification, error) {
			return domain.Notification{}, domain.ErrConflict
		}}, authUserRepoStub{getByEmailFn: func(context.Context, string) (domain.User, error) {
			return domain.User{ID: "u1", Email: "x@example.com"}, nil
		}}, membershipListStub{})
		if err := svc.NotifyInvitationCreated(context.Background(), domain.WorkspaceInvitation{ID: "inv", WorkspaceID: "w", Email: "x@example.com"}); err != nil {
			t.Fatalf("expected conflict to be ignored, got %v", err)
		}
	})

	t.Run("comment notification membership and create failures", func(t *testing.T) {
		svc := NewNotificationService(notificationRepoStub{}, authUserRepoStub{}, membershipListStub{err: errors.New("members failed")})
		if err := svc.NotifyCommentCreated(context.Background(), domain.Page{ID: "p1", WorkspaceID: "w1"}, domain.PageComment{ID: "c1", PageID: "p1", CreatedBy: "u1", CreatedAt: time.Now().UTC()}); err == nil || err.Error() != "members failed" {
			t.Fatalf("expected membership list failure, got %v", err)
		}

		svc = NewNotificationService(notificationRepoStub{createFn: func(context.Context, domain.Notification) (domain.Notification, error) {
			return domain.Notification{}, errors.New("create failed")
		}}, authUserRepoStub{}, membershipListStub{members: []domain.WorkspaceMember{{UserID: "u2", WorkspaceID: "w1", Role: domain.RoleEditor}}})
		if err := svc.NotifyCommentCreated(context.Background(), domain.Page{ID: "p1", WorkspaceID: "w1"}, domain.PageComment{ID: "c1", PageID: "p1", CreatedBy: "u1", CreatedAt: time.Now().UTC()}); err == nil || err.Error() != "create failed" {
			t.Fatalf("expected create failure propagation, got %v", err)
		}
	})
}
