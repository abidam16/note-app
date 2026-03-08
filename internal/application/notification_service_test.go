package application

import (
	"context"
	"testing"
	"time"

	"note-app/internal/domain"
)

type fakeNotificationRepo struct {
	notifications map[string]domain.Notification
	ordered       []domain.Notification
}

func (r *fakeNotificationRepo) Create(_ context.Context, notification domain.Notification) (domain.Notification, error) {
	if r.notifications == nil {
		r.notifications = map[string]domain.Notification{}
	}
	for _, existing := range r.ordered {
		if existing.UserID == notification.UserID && existing.Type == notification.Type && existing.EventID == notification.EventID {
			return domain.Notification{}, domain.ErrConflict
		}
	}
	r.notifications[notification.ID] = notification
	r.ordered = append(r.ordered, notification)
	return notification, nil
}

func (r *fakeNotificationRepo) ListByUserID(_ context.Context, userID string) ([]domain.Notification, error) {
	result := make([]domain.Notification, 0)
	for idx := len(r.ordered) - 1; idx >= 0; idx-- {
		notification := r.ordered[idx]
		if notification.UserID == userID {
			result = append(result, r.notifications[notification.ID])
		}
	}
	return result, nil
}

func (r *fakeNotificationRepo) MarkRead(_ context.Context, notificationID, userID string, readAt time.Time) (domain.Notification, error) {
	notification, ok := r.notifications[notificationID]
	if !ok || notification.UserID != userID {
		return domain.Notification{}, domain.ErrNotFound
	}
	if notification.ReadAt == nil {
		notification.ReadAt = &readAt
		r.notifications[notificationID] = notification
		for idx := range r.ordered {
			if r.ordered[idx].ID == notificationID {
				r.ordered[idx] = notification
			}
		}
	}
	return notification, nil
}

func TestNotificationServiceListAndMarkRead(t *testing.T) {
	repo := &fakeNotificationRepo{notifications: map[string]domain.Notification{}, ordered: []domain.Notification{}}
	service := NewNotificationService(repo, &fakeUserRepo{byEmail: map[string]domain.User{}, byID: map[string]domain.User{}}, &fakeWorkspaceRepo{memberships: map[string][]domain.WorkspaceMember{}, invitations: map[string]domain.WorkspaceInvitation{}, owners: map[string]int{}})

	createdAt := time.Date(2026, 3, 8, 4, 0, 0, 0, time.UTC)
	repo.ordered = append(repo.ordered,
		domain.Notification{ID: "notif-1", UserID: "user-1", WorkspaceID: "workspace-1", Type: domain.NotificationTypeComment, EventID: "comment-1", Message: "comment", CreatedAt: createdAt},
		domain.Notification{ID: "notif-2", UserID: "user-2", WorkspaceID: "workspace-1", Type: domain.NotificationTypeInvitation, EventID: "inv-1", Message: "invite", CreatedAt: createdAt.Add(time.Minute)},
	)
	repo.notifications["notif-1"] = repo.ordered[0]
	repo.notifications["notif-2"] = repo.ordered[1]

	list, err := service.ListNotifications(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("ListNotifications() error = %v", err)
	}
	if len(list) != 1 || list[0].ID != "notif-1" {
		t.Fatalf("unexpected notifications list: %+v", list)
	}

	read, err := service.MarkNotificationRead(context.Background(), "user-1", "notif-1")
	if err != nil {
		t.Fatalf("MarkNotificationRead() error = %v", err)
	}
	if read.ReadAt == nil {
		t.Fatalf("expected read notification")
	}

	if _, err := service.MarkNotificationRead(context.Background(), "user-1", "notif-2"); err != domain.ErrNotFound {
		t.Fatalf("expected not found for other-user notification, got %v", err)
	}
}

func TestNotificationServicePublishesInvitationAndCommentEvents(t *testing.T) {
	repo := &fakeNotificationRepo{notifications: map[string]domain.Notification{}, ordered: []domain.Notification{}}
	users := &fakeUserRepo{byEmail: map[string]domain.User{
		"invited@example.com": {ID: "invited-user", Email: "invited@example.com", FullName: "Invited"},
	}, byID: map[string]domain.User{}}
	memberships := &fakeWorkspaceRepo{memberships: map[string][]domain.WorkspaceMember{
		"workspace-1": {
			{ID: "member-1", WorkspaceID: "workspace-1", UserID: "author-1", Role: domain.RoleViewer},
			{ID: "member-2", WorkspaceID: "workspace-1", UserID: "editor-1", Role: domain.RoleEditor},
			{ID: "member-3", WorkspaceID: "workspace-1", UserID: "owner-1", Role: domain.RoleOwner},
		},
	}, invitations: map[string]domain.WorkspaceInvitation{}, owners: map[string]int{}}
	service := NewNotificationService(repo, users, memberships)

	err := service.NotifyInvitationCreated(context.Background(), domain.WorkspaceInvitation{
		ID:          "inv-1",
		WorkspaceID: "workspace-1",
		Email:       "invited@example.com",
	})
	if err != nil {
		t.Fatalf("NotifyInvitationCreated() error = %v", err)
	}

	err = service.NotifyInvitationCreated(context.Background(), domain.WorkspaceInvitation{
		ID:          "inv-1",
		WorkspaceID: "workspace-1",
		Email:       "invited@example.com",
	})
	if err != nil {
		t.Fatalf("NotifyInvitationCreated() duplicate error = %v", err)
	}

	err = service.NotifyCommentCreated(context.Background(), domain.Page{ID: "page-1", WorkspaceID: "workspace-1"}, domain.PageComment{
		ID:        "comment-1",
		PageID:    "page-1",
		Body:      "hello",
		CreatedBy: "author-1",
		CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("NotifyCommentCreated() error = %v", err)
	}

	invCount := 0
	commentUsers := map[string]bool{}
	for _, notification := range repo.ordered {
		if notification.Type == domain.NotificationTypeInvitation {
			invCount++
		}
		if notification.Type == domain.NotificationTypeComment {
			commentUsers[notification.UserID] = true
		}
	}
	if invCount != 1 {
		t.Fatalf("expected one invitation notification, got %d", invCount)
	}
	if !commentUsers["editor-1"] || !commentUsers["owner-1"] || commentUsers["author-1"] {
		t.Fatalf("unexpected comment notification users: %+v", commentUsers)
	}
}
