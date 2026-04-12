package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"note-app/internal/domain"
)

type notificationRepoStub struct {
	createFn        func(context.Context, domain.Notification) (domain.Notification, error)
	createManyFn    func(context.Context, []domain.Notification) error
	batchMarkReadFn func(context.Context, string, []string, time.Time) (domain.NotificationBatchReadResult, error)
	listInboxFn     func(context.Context, string, domain.NotificationInboxFilter) (domain.NotificationInboxPage, error)
	markReadFn      func(context.Context, string, string, time.Time) (domain.NotificationInboxItem, error)
	getUnreadFn     func(context.Context, string) (int64, error)
}

func (s notificationRepoStub) Create(ctx context.Context, n domain.Notification) (domain.Notification, error) {
	if s.createFn != nil {
		return s.createFn(ctx, n)
	}
	return n, nil
}
func (s notificationRepoStub) CreateMany(ctx context.Context, notifications []domain.Notification) error {
	if s.createManyFn != nil {
		return s.createManyFn(ctx, notifications)
	}
	return nil
}
func (s notificationRepoStub) BatchMarkRead(ctx context.Context, userID string, notificationIDs []string, readAt time.Time) (domain.NotificationBatchReadResult, error) {
	if s.batchMarkReadFn != nil {
		return s.batchMarkReadFn(ctx, userID, notificationIDs, readAt)
	}
	return domain.NotificationBatchReadResult{}, nil
}
func (s notificationRepoStub) ListInbox(ctx context.Context, userID string, filter domain.NotificationInboxFilter) (domain.NotificationInboxPage, error) {
	if s.listInboxFn != nil {
		return s.listInboxFn(ctx, userID, filter)
	}
	return domain.NotificationInboxPage{}, nil
}
func (s notificationRepoStub) MarkRead(ctx context.Context, notificationID, userID string, readAt time.Time) (domain.NotificationInboxItem, error) {
	if s.markReadFn != nil {
		return s.markReadFn(ctx, notificationID, userID, readAt)
	}
	return domain.NotificationInboxItem{}, nil
}
func (s notificationRepoStub) GetUnreadCount(ctx context.Context, userID string) (int64, error) {
	if s.getUnreadFn != nil {
		return s.getUnreadFn(ctx, userID)
	}
	return 0, nil
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

func timePtr(value time.Time) *time.Time {
	return &value
}

func TestNotificationServiceAdditionalBranches(t *testing.T) {
	t.Run("list and mark read propagate repository errors", func(t *testing.T) {
		svc := NewNotificationService(notificationRepoStub{
			listInboxFn: func(context.Context, string, domain.NotificationInboxFilter) (domain.NotificationInboxPage, error) {
				return domain.NotificationInboxPage{}, errors.New("list failed")
			},
			markReadFn: func(context.Context, string, string, time.Time) (domain.NotificationInboxItem, error) {
				return domain.NotificationInboxItem{}, errors.New("mark failed")
			},
		}, authUserRepoStub{getByIDFn: func(context.Context, string) (domain.User, error) {
			return domain.User{ID: "u1", Email: "u1@example.com"}, nil
		}}, membershipListStub{})

		if _, err := svc.ListNotifications(context.Background(), "u1", ListNotificationsInput{}); err == nil || err.Error() != "list failed" {
			t.Fatalf("expected list error propagation, got %v", err)
		}
		if _, err := svc.MarkNotificationRead(context.Background(), "u1", "11111111-1111-1111-1111-111111111111"); err == nil || err.Error() != "mark failed" {
			t.Fatalf("expected mark error propagation, got %v", err)
		}
	})

	t.Run("get unread count validates actor and propagates repository errors", func(t *testing.T) {
		svc := NewNotificationService(notificationRepoStub{
			getUnreadFn: func(context.Context, string) (int64, error) {
				return 0, errors.New("count failed")
			},
		}, authUserRepoStub{getByIDFn: func(context.Context, string) (domain.User, error) {
			return domain.User{ID: "u1", Email: "u1@example.com"}, nil
		}}, membershipListStub{})

		if _, err := svc.GetUnreadCount(context.Background(), "u1"); err == nil || err.Error() != "count failed" {
			t.Fatalf("expected unread-count error propagation, got %v", err)
		}

		svc = NewNotificationService(notificationRepoStub{
			getUnreadFn: func(_ context.Context, userID string) (int64, error) {
				if userID != "u1" {
					t.Fatalf("unexpected unread-count userID: %s", userID)
				}
				return 7, nil
			},
		}, authUserRepoStub{getByIDFn: func(context.Context, string) (domain.User, error) {
			return domain.User{ID: "u1", Email: "u1@example.com"}, nil
		}}, membershipListStub{})
		count, err := svc.GetUnreadCount(context.Background(), "u1")
		if err != nil {
			t.Fatalf("expected unread-count success, got %v", err)
		}
		if count.UnreadCount != 7 {
			t.Fatalf("expected unread_count=7, got %+v", count)
		}

		unauthorizedSvc := NewNotificationService(notificationRepoStub{}, authUserRepoStub{getByIDFn: func(context.Context, string) (domain.User, error) {
			return domain.User{}, domain.ErrNotFound
		}}, membershipListStub{})
		if _, err := unauthorizedSvc.GetUnreadCount(context.Background(), "missing"); !errors.Is(err, domain.ErrUnauthorized) {
			t.Fatalf("expected unauthorized for missing actor, got %v", err)
		}
	})

	t.Run("list validates actor, defaults, and filters", func(t *testing.T) {
		var gotFilter domain.NotificationInboxFilter
		svc := NewNotificationService(notificationRepoStub{
			listInboxFn: func(_ context.Context, userID string, filter domain.NotificationInboxFilter) (domain.NotificationInboxPage, error) {
				if userID != "u1" {
					t.Fatalf("unexpected userID: %s", userID)
				}
				gotFilter = filter
				return domain.NotificationInboxPage{}, nil
			},
		}, authUserRepoStub{getByIDFn: func(context.Context, string) (domain.User, error) {
			return domain.User{ID: "u1", Email: "u1@example.com"}, nil
		}}, membershipListStub{})

		if _, err := svc.ListNotifications(context.Background(), "u1", ListNotificationsInput{}); err != nil {
			t.Fatalf("expected default list success, got %v", err)
		}
		if gotFilter.Status != domain.NotificationInboxStatusAll || gotFilter.Type != domain.NotificationInboxTypeAll || gotFilter.Limit != 50 {
			t.Fatalf("unexpected default filter: %+v", gotFilter)
		}

		if _, err := svc.ListNotifications(context.Background(), "u1", ListNotificationsInput{Status: "bad"}); !errors.Is(err, domain.ErrValidation) {
			t.Fatalf("expected invalid status validation error, got %v", err)
		}
		if _, err := svc.ListNotifications(context.Background(), "u1", ListNotificationsInput{Type: "bad"}); !errors.Is(err, domain.ErrValidation) {
			t.Fatalf("expected invalid type validation error, got %v", err)
		}
		if _, err := svc.ListNotifications(context.Background(), "u1", ListNotificationsInput{Limit: -1}); !errors.Is(err, domain.ErrValidation) {
			t.Fatalf("expected invalid limit validation error, got %v", err)
		}
		if _, err := svc.ListNotifications(context.Background(), "u1", ListNotificationsInput{Limit: 101}); !errors.Is(err, domain.ErrValidation) {
			t.Fatalf("expected oversized limit validation error, got %v", err)
		}
	})

	t.Run("list returns unauthorized for unknown actor", func(t *testing.T) {
		svc := NewNotificationService(notificationRepoStub{}, authUserRepoStub{getByIDFn: func(context.Context, string) (domain.User, error) {
			return domain.User{}, domain.ErrNotFound
		}}, membershipListStub{})
		if _, err := svc.ListNotifications(context.Background(), "missing", ListNotificationsInput{}); !errors.Is(err, domain.ErrUnauthorized) {
			t.Fatalf("expected unauthorized for missing actor, got %v", err)
		}
	})

	t.Run("mark read validates actor and notification id", func(t *testing.T) {
		svc := NewNotificationService(notificationRepoStub{
			markReadFn: func(_ context.Context, notificationID, userID string, _ time.Time) (domain.NotificationInboxItem, error) {
				if notificationID != "11111111-1111-1111-1111-111111111111" || userID != "u1" {
					t.Fatalf("unexpected mark-read args: notificationID=%s userID=%s", notificationID, userID)
				}
				return domain.NotificationInboxItem{ID: notificationID, IsRead: true, ReadAt: timePtr(time.Date(2026, 4, 7, 0, 0, 0, 0, time.UTC))}, nil
			},
		}, authUserRepoStub{getByIDFn: func(context.Context, string) (domain.User, error) {
			return domain.User{ID: "u1", Email: "u1@example.com"}, nil
		}}, membershipListStub{})

		item, err := svc.MarkNotificationRead(context.Background(), "u1", "11111111-1111-1111-1111-111111111111")
		if err != nil {
			t.Fatalf("expected mark-read success, got %v", err)
		}
		if !item.IsRead || item.ReadAt == nil {
			t.Fatalf("expected read inbox item, got %+v", item)
		}

		if _, err := svc.MarkNotificationRead(context.Background(), "u1", "broken"); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("expected malformed notification id not found, got %v", err)
		}

		unauthorizedSvc := NewNotificationService(notificationRepoStub{}, authUserRepoStub{getByIDFn: func(context.Context, string) (domain.User, error) {
			return domain.User{}, domain.ErrNotFound
		}}, membershipListStub{})
		if _, err := unauthorizedSvc.MarkNotificationRead(context.Background(), "u1", "11111111-1111-1111-1111-111111111111"); !errors.Is(err, domain.ErrUnauthorized) {
			t.Fatalf("expected unauthorized actor, got %v", err)
		}
	})

	t.Run("batch mark read validates actor and request semantics", func(t *testing.T) {
		svc := NewNotificationService(notificationRepoStub{
			batchMarkReadFn: func(_ context.Context, userID string, notificationIDs []string, _ time.Time) (domain.NotificationBatchReadResult, error) {
				if userID != "u1" {
					t.Fatalf("unexpected userID: %s", userID)
				}
				if len(notificationIDs) != 2 || notificationIDs[0] != "11111111-1111-1111-1111-111111111111" || notificationIDs[1] != "22222222-2222-2222-2222-222222222222" {
					t.Fatalf("unexpected notification ids: %+v", notificationIDs)
				}
				return domain.NotificationBatchReadResult{UpdatedCount: 2, UnreadCount: 4}, nil
			},
		}, authUserRepoStub{getByIDFn: func(context.Context, string) (domain.User, error) {
			return domain.User{ID: "u1", Email: "u1@example.com"}, nil
		}}, membershipListStub{})

		result, err := svc.MarkNotificationsRead(context.Background(), "u1", domain.BatchMarkNotificationsReadInput{
			NotificationIDs: []string{"11111111-1111-1111-1111-111111111111", "22222222-2222-2222-2222-222222222222"},
		})
		if err != nil {
			t.Fatalf("expected batch mark-read success, got %v", err)
		}
		if result.UpdatedCount != 2 || result.UnreadCount != 4 {
			t.Fatalf("unexpected batch result: %+v", result)
		}

		if _, err := svc.MarkNotificationsRead(context.Background(), "u1", domain.BatchMarkNotificationsReadInput{}); !errors.Is(err, domain.ErrValidation) {
			t.Fatalf("expected missing ids validation error, got %v", err)
		}
		if _, err := svc.MarkNotificationsRead(context.Background(), "u1", domain.BatchMarkNotificationsReadInput{NotificationIDs: []string{"11111111-1111-1111-1111-111111111111", "11111111-1111-1111-1111-111111111111"}}); !errors.Is(err, domain.ErrValidation) {
			t.Fatalf("expected duplicate ids validation error, got %v", err)
		}
		if _, err := svc.MarkNotificationsRead(context.Background(), "u1", domain.BatchMarkNotificationsReadInput{NotificationIDs: []string{"broken"}}); !errors.Is(err, domain.ErrValidation) {
			t.Fatalf("expected invalid uuid validation error, got %v", err)
		}

		unauthorizedSvc := NewNotificationService(notificationRepoStub{}, authUserRepoStub{getByIDFn: func(context.Context, string) (domain.User, error) {
			return domain.User{}, domain.ErrNotFound
		}}, membershipListStub{})
		if _, err := unauthorizedSvc.MarkNotificationsRead(context.Background(), "missing", domain.BatchMarkNotificationsReadInput{NotificationIDs: []string{"11111111-1111-1111-1111-111111111111"}}); !errors.Is(err, domain.ErrUnauthorized) {
			t.Fatalf("expected unauthorized actor, got %v", err)
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

		svc = NewNotificationService(notificationRepoStub{createManyFn: func(_ context.Context, notifications []domain.Notification) error {
			if len(notifications) != 1 || notifications[0].UserID != "u2" {
				t.Fatalf("unexpected notifications batch: %+v", notifications)
			}
			return errors.New("create failed")
		}}, authUserRepoStub{}, membershipListStub{members: []domain.WorkspaceMember{{UserID: "u2", WorkspaceID: "w1", Role: domain.RoleEditor}}})
		if err := svc.NotifyCommentCreated(context.Background(), domain.Page{ID: "p1", WorkspaceID: "w1"}, domain.PageComment{ID: "c1", PageID: "p1", CreatedBy: "u1", CreatedAt: time.Now().UTC()}); err == nil || err.Error() != "create failed" {
			t.Fatalf("expected create failure propagation, got %v", err)
		}
	})

}
