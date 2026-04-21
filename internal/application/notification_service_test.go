package application

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"note-app/internal/domain"
)

type fakeNotificationRepo struct {
	notifications map[string]domain.Notification
	ordered       []domain.Notification
}

func (r *fakeNotificationRepo) GetUnreadCount(_ context.Context, userID string) (int64, error) {
	unreadCount := int64(0)
	for _, notification := range r.notifications {
		if notification.UserID == userID && !notification.IsRead {
			unreadCount++
		}
	}
	return unreadCount, nil
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
	if notification.Title == "" {
		switch notification.Type {
		case domain.NotificationTypeInvitation:
			notification.Title = "Workspace invitation"
		case domain.NotificationTypeComment, domain.NotificationTypeMention:
			notification.Title = "Comment activity"
		default:
			notification.Title = "Notification"
		}
	}
	if notification.Content == "" {
		notification.Content = notification.Message
	}
	if len(notification.Payload) == 0 {
		notification.Payload = []byte(`{}`)
	}
	notification.IsRead = notification.ReadAt != nil
	if notification.UpdatedAt.IsZero() {
		if notification.ReadAt != nil {
			notification.UpdatedAt = *notification.ReadAt
		} else {
			notification.UpdatedAt = notification.CreatedAt
		}
	}
	r.notifications[notification.ID] = notification
	r.ordered = append(r.ordered, notification)
	return notification, nil
}

func (r *fakeNotificationRepo) CreateMany(_ context.Context, notifications []domain.Notification) error {
	for _, notification := range notifications {
		if _, err := r.Create(context.Background(), notification); err != nil && !errors.Is(err, domain.ErrConflict) {
			return err
		}
	}
	return nil
}

func (r *fakeNotificationRepo) BatchMarkRead(_ context.Context, userID string, notificationIDs []string, readAt time.Time) (domain.NotificationBatchReadResult, error) {
	if len(notificationIDs) == 0 {
		return domain.NotificationBatchReadResult{}, domain.ErrValidation
	}

	updatedCount := int64(0)
	for _, notificationID := range notificationIDs {
		notification, ok := r.notifications[notificationID]
		if !ok || notification.UserID != userID {
			return domain.NotificationBatchReadResult{}, domain.ErrNotFound
		}
		if notification.ReadAt == nil {
			notification.ReadAt = &readAt
			notification.IsRead = true
			notification.UpdatedAt = readAt
			r.notifications[notificationID] = notification
			updatedCount++
			for idx := range r.ordered {
				if r.ordered[idx].ID == notificationID {
					r.ordered[idx] = notification
				}
			}
		}
	}

	unreadCount := int64(0)
	for _, notification := range r.notifications {
		if notification.UserID == userID && !notification.IsRead {
			unreadCount++
		}
	}

	return domain.NotificationBatchReadResult{UpdatedCount: updatedCount, UnreadCount: unreadCount}, nil
}

func (r *fakeNotificationRepo) ListInbox(_ context.Context, userID string, filter domain.NotificationInboxFilter) (domain.NotificationInboxPage, error) {
	result := make([]domain.NotificationInboxItem, 0)
	for idx := len(r.ordered) - 1; idx >= 0; idx-- {
		notification := r.ordered[idx]
		if notification.UserID != userID {
			continue
		}
		result = append(result, domain.NotificationInboxItem{
			ID:           notification.ID,
			WorkspaceID:  notification.WorkspaceID,
			Type:         notification.Type,
			ActorID:      notification.ActorID,
			Title:        notification.Title,
			Content:      notification.Content,
			IsRead:       notification.IsRead,
			ReadAt:       notification.ReadAt,
			Actionable:   notification.Actionable,
			ActionKind:   notification.ActionKind,
			ResourceType: notification.ResourceType,
			ResourceID:   notification.ResourceID,
			Payload:      notification.Payload,
			CreatedAt:    notification.CreatedAt,
			UpdatedAt:    notification.UpdatedAt,
		})
		if filter.Limit > 0 && len(result) == filter.Limit {
			break
		}
	}
	unreadCount := int64(0)
	for _, notification := range r.notifications {
		if notification.UserID == userID && !notification.IsRead {
			unreadCount++
		}
	}
	return domain.NotificationInboxPage{Items: result, UnreadCount: unreadCount, HasMore: false}, nil
}

func (r *fakeNotificationRepo) MarkRead(_ context.Context, notificationID, userID string, readAt time.Time) (domain.NotificationInboxItem, error) {
	notification, ok := r.notifications[notificationID]
	if !ok || notification.UserID != userID {
		return domain.NotificationInboxItem{}, domain.ErrNotFound
	}
	if notification.ReadAt == nil {
		notification.ReadAt = &readAt
		notification.IsRead = true
		notification.UpdatedAt = readAt
		r.notifications[notificationID] = notification
		for idx := range r.ordered {
			if r.ordered[idx].ID == notificationID {
				r.ordered[idx] = notification
			}
		}
	}
	return domain.NotificationInboxItem{
		ID:           notification.ID,
		WorkspaceID:  notification.WorkspaceID,
		Type:         notification.Type,
		ActorID:      notification.ActorID,
		Title:        notification.Title,
		Content:      notification.Content,
		IsRead:       notification.IsRead,
		ReadAt:       notification.ReadAt,
		Actionable:   notification.Actionable,
		ActionKind:   notification.ActionKind,
		ResourceType: notification.ResourceType,
		ResourceID:   notification.ResourceID,
		Payload:      notification.Payload,
		CreatedAt:    notification.CreatedAt,
		UpdatedAt:    notification.UpdatedAt,
	}, nil
}

func TestNotificationServiceListAndMarkRead(t *testing.T) {
	repo := &fakeNotificationRepo{notifications: map[string]domain.Notification{}, ordered: []domain.Notification{}}
	service := NewNotificationService(repo, &fakeUserRepo{byEmail: map[string]domain.User{}, byID: map[string]domain.User{}}, &fakeWorkspaceRepo{memberships: map[string][]domain.WorkspaceMember{}, invitations: map[string]domain.WorkspaceInvitation{}, owners: map[string]int{}})

	createdAt := time.Date(2026, 3, 8, 4, 0, 0, 0, time.UTC)
	repo.ordered = append(repo.ordered,
		domain.Notification{ID: "11111111-1111-1111-1111-111111111111", UserID: "user-1", WorkspaceID: "workspace-1", Type: domain.NotificationTypeComment, EventID: "comment-1", Message: "comment", CreatedAt: createdAt},
		domain.Notification{ID: "22222222-2222-2222-2222-222222222222", UserID: "user-2", WorkspaceID: "workspace-1", Type: domain.NotificationTypeInvitation, EventID: "inv-1", Message: "invite", CreatedAt: createdAt.Add(time.Minute)},
	)
	repo.notifications["11111111-1111-1111-1111-111111111111"] = repo.ordered[0]
	repo.notifications["22222222-2222-2222-2222-222222222222"] = repo.ordered[1]

	users := &fakeUserRepo{byEmail: map[string]domain.User{}, byID: map[string]domain.User{
		"user-1": {ID: "user-1", Email: "user1@example.com", FullName: "User One"},
	}}
	service = NewNotificationService(repo, users, &fakeWorkspaceRepo{memberships: map[string][]domain.WorkspaceMember{}, invitations: map[string]domain.WorkspaceInvitation{}, owners: map[string]int{}})

	list, err := service.ListNotifications(context.Background(), "user-1", ListNotificationsInput{})
	if err != nil {
		t.Fatalf("ListNotifications() error = %v", err)
	}
	if len(list.Items) != 1 || list.Items[0].ID != "11111111-1111-1111-1111-111111111111" {
		t.Fatalf("unexpected notifications list: %+v", list)
	}
	if list.UnreadCount != 1 {
		t.Fatalf("expected unread_count=1, got %+v", list)
	}

	read, err := service.MarkNotificationRead(context.Background(), "user-1", "11111111-1111-1111-1111-111111111111")
	if err != nil {
		t.Fatalf("MarkNotificationRead() error = %v", err)
	}
	if read.ReadAt == nil || !read.IsRead {
		t.Fatalf("expected read notification")
	}

	if _, err := service.MarkNotificationRead(context.Background(), "user-1", "22222222-2222-2222-2222-222222222222"); err != domain.ErrNotFound {
		t.Fatalf("expected not found for other-user notification, got %v", err)
	}
}

func TestNotificationServiceGetUnreadCount(t *testing.T) {
	repo := &fakeNotificationRepo{notifications: map[string]domain.Notification{}, ordered: []domain.Notification{}}
	service := NewNotificationService(repo, &fakeUserRepo{byEmail: map[string]domain.User{}, byID: map[string]domain.User{
		"user-1": {ID: "user-1", Email: "user1@example.com", FullName: "User One"},
	}}, &fakeWorkspaceRepo{memberships: map[string][]domain.WorkspaceMember{}, invitations: map[string]domain.WorkspaceInvitation{}, owners: map[string]int{}})

	createdAt := time.Date(2026, 3, 8, 4, 0, 0, 0, time.UTC)
	repo.ordered = append(repo.ordered,
		domain.Notification{ID: "11111111-1111-1111-1111-111111111111", UserID: "user-1", WorkspaceID: "workspace-1", Type: domain.NotificationTypeComment, EventID: "comment-1", Message: "comment", CreatedAt: createdAt},
		domain.Notification{ID: "22222222-2222-2222-2222-222222222222", UserID: "user-1", WorkspaceID: "workspace-1", Type: domain.NotificationTypeInvitation, EventID: "inv-1", Message: "invite", CreatedAt: createdAt.Add(time.Minute), ReadAt: timePtr(createdAt.Add(2 * time.Minute)), IsRead: true, UpdatedAt: createdAt.Add(2 * time.Minute)},
	)
	repo.notifications["11111111-1111-1111-1111-111111111111"] = repo.ordered[0]
	repo.notifications["22222222-2222-2222-2222-222222222222"] = repo.ordered[1]

	got, err := service.GetUnreadCount(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("GetUnreadCount() error = %v", err)
	}
	if got.UnreadCount != 1 {
		t.Fatalf("expected unread_count=1, got %+v", got)
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
		Role:        domain.RoleViewer,
		Status:      domain.WorkspaceInvitationStatusPending,
		Version:     1,
		InvitedBy:   "owner-1",
	})
	if err != nil {
		t.Fatalf("NotifyInvitationCreated() error = %v", err)
	}

	err = service.NotifyInvitationCreated(context.Background(), domain.WorkspaceInvitation{
		ID:          "inv-1",
		WorkspaceID: "workspace-1",
		Email:       "invited@example.com",
		Role:        domain.RoleViewer,
		Status:      domain.WorkspaceInvitationStatusPending,
		Version:     1,
		InvitedBy:   "owner-1",
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

	invitationNotification := repo.notifications[repo.ordered[0].ID]
	if invitationNotification.ActorID == nil || *invitationNotification.ActorID != "owner-1" {
		t.Fatalf("expected invitation actor_id owner-1, got %+v", invitationNotification.ActorID)
	}
	if invitationNotification.Title != "Workspace invitation" || invitationNotification.Content != "You have a new workspace invitation" {
		t.Fatalf("unexpected invitation v2 content fields: %+v", invitationNotification)
	}
	if !invitationNotification.Actionable || invitationNotification.ActionKind == nil || *invitationNotification.ActionKind != domain.NotificationActionKindInvitationResponse {
		t.Fatalf("expected invitation actionability metadata, got %+v", invitationNotification)
	}
	if invitationNotification.ResourceType == nil || *invitationNotification.ResourceType != domain.NotificationResourceTypeInvitation || invitationNotification.ResourceID == nil || *invitationNotification.ResourceID != "inv-1" {
		t.Fatalf("unexpected invitation resource metadata: %+v", invitationNotification)
	}
	if invitationNotification.IsRead || invitationNotification.UpdatedAt.IsZero() {
		t.Fatalf("unexpected invitation read/payload metadata: %+v", invitationNotification)
	}
	var payload map[string]any
	if err := json.Unmarshal(invitationNotification.Payload, &payload); err != nil {
		t.Fatalf("unmarshal invitation payload: %v", err)
	}
	if payload["invitation_id"] != "inv-1" ||
		payload["workspace_id"] != "workspace-1" ||
		payload["email"] != "invited@example.com" ||
		payload["role"] != string(domain.RoleViewer) ||
		payload["status"] != string(domain.WorkspaceInvitationStatusPending) {
		t.Fatalf("unexpected invitation payload fields: %+v", payload)
	}
	if version, ok := payload["version"].(float64); !ok || version != 1 {
		t.Fatalf("expected version=1 in payload, got %+v", payload["version"])
	}
	if canAccept, ok := payload["can_accept"].(bool); !ok || !canAccept {
		t.Fatalf("expected can_accept=true in payload, got %+v", payload["can_accept"])
	}
	if canReject, ok := payload["can_reject"].(bool); !ok || !canReject {
		t.Fatalf("expected can_reject=true in payload, got %+v", payload["can_reject"])
	}

	var sawCommentV2 bool
	for _, notification := range repo.ordered {
		if notification.Type != domain.NotificationTypeComment {
			continue
		}
		sawCommentV2 = true
		if notification.ActorID == nil || notification.Title != "Comment activity" || notification.Content == "" {
			t.Fatalf("unexpected comment v2 content fields: %+v", notification)
		}
		if notification.Actionable || notification.ActionKind != nil {
			t.Fatalf("expected comment notification to stay non-actionable: %+v", notification)
		}
		if notification.ResourceType == nil || notification.ResourceID == nil {
			t.Fatalf("expected comment resource metadata, got %+v", notification)
		}
	}
	if !sawCommentV2 {
		t.Fatal("expected comment notifications to be created")
	}
}
