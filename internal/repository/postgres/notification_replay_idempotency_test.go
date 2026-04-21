package postgres

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"note-app/internal/domain"

	"github.com/google/uuid"
)

func TestNotificationReplayIdempotencyCommentAndMentionRows(t *testing.T) {
	pool := integrationPool(t)
	repo := NewNotificationRepository(pool)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	owner := seedUser(t, pool, "replay-owner@example.com")
	member := seedUser(t, pool, "replay-member@example.com")
	mentioned := seedUser(t, pool, "replay-mentioned@example.com")
	workspace, _ := seedWorkspaceWithOwner(t, pool, owner)
	if _, err := pool.Exec(ctx, `INSERT INTO workspace_members (id, workspace_id, user_id, role, created_at) VALUES ($1,$2,$3,$4,$5)`, uuid.NewString(), workspace.ID, member.ID, domain.RoleEditor, now); err != nil {
		t.Fatalf("seed member membership: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO workspace_members (id, workspace_id, user_id, role, created_at) VALUES ($1,$2,$3,$4,$5)`, uuid.NewString(), workspace.ID, mentioned.ID, domain.RoleViewer, now); err != nil {
		t.Fatalf("seed mentioned membership: %v", err)
	}

	threadMsgResourceType := domain.NotificationResourceTypeThreadMsg
	messageID := uuid.NewString()
	commentNotification := domain.Notification{
		ID:           uuid.NewString(),
		UserID:       member.ID,
		WorkspaceID:  workspace.ID,
		Type:         domain.NotificationTypeComment,
		EventID:      messageID,
		Message:      "A relevant comment thread has a new reply",
		Title:        "New thread reply",
		Content:      "A relevant comment thread has a new reply",
		ResourceType: &threadMsgResourceType,
		ResourceID:   &messageID,
		Payload: mustNotificationPayload(t, map[string]any{
			"thread_id":    "thread-1",
			"message_id":   messageID,
			"page_id":      "page-1",
			"workspace_id": workspace.ID,
			"event_topic":  string(domain.OutboxTopicThreadReplyCreated),
		}),
		CreatedAt: now,
		UpdatedAt: now,
	}
	mentionNotification := domain.Notification{
		ID:           uuid.NewString(),
		UserID:       member.ID,
		WorkspaceID:  workspace.ID,
		Type:         domain.NotificationTypeMention,
		EventID:      messageID,
		Message:      "You were mentioned in a thread reply",
		Title:        "Mentioned in a thread reply",
		Content:      "You were mentioned in a thread reply",
		ActorID:      &owner.ID,
		ResourceType: &threadMsgResourceType,
		ResourceID:   &messageID,
		Payload: mustNotificationPayload(t, map[string]any{
			"thread_id":      "thread-1",
			"message_id":     messageID,
			"page_id":        "page-1",
			"workspace_id":   workspace.ID,
			"actor_id":       owner.ID,
			"event_topic":    string(domain.OutboxTopicThreadReplyCreated),
			"mention_source": "explicit",
		}),
		CreatedAt: now.Add(time.Second),
		UpdatedAt: now.Add(time.Second),
	}

	insertedCommentCount, err := repo.CreateCommentNotifications(ctx, []domain.Notification{commentNotification, commentNotification})
	if err != nil {
		t.Fatalf("CreateCommentNotifications() error = %v", err)
	}
	if insertedCommentCount != 1 {
		t.Fatalf("expected one inserted comment row, got %d", insertedCommentCount)
	}
	if unreadCount, err := repo.GetUnreadCount(ctx, member.ID); err != nil {
		t.Fatalf("GetUnreadCount() after comment replay error = %v", err)
	} else if unreadCount != 1 {
		t.Fatalf("expected unread count 1 after comment replay, got %d", unreadCount)
	}

	insertedMentionCount, err := repo.CreateMentionNotifications(ctx, []domain.Notification{mentionNotification, mentionNotification})
	if err != nil {
		t.Fatalf("CreateMentionNotifications() error = %v", err)
	}
	if insertedMentionCount != 1 {
		t.Fatalf("expected one inserted mention row, got %d", insertedMentionCount)
	}
	if unreadCount, err := repo.GetUnreadCount(ctx, member.ID); err != nil {
		t.Fatalf("GetUnreadCount() after mention replay error = %v", err)
	} else if unreadCount != 2 {
		t.Fatalf("expected unread count 2 after comment+mention replay, got %d", unreadCount)
	}

	memberNotifications, err := repo.ListByUserID(ctx, member.ID)
	if err != nil {
		t.Fatalf("ListByUserID() error = %v", err)
	}
	commentRows := 0
	mentionRows := 0
	for _, notification := range memberNotifications {
		switch notification.Type {
		case domain.NotificationTypeComment:
			if notification.EventID == messageID {
				commentRows++
			}
		case domain.NotificationTypeMention:
			if notification.EventID == messageID {
				mentionRows++
			}
		}
	}
	if commentRows != 1 || mentionRows != 1 {
		t.Fatalf("expected one comment row and one mention row after replay, got comments=%d mentions=%d rows=%+v", commentRows, mentionRows, memberNotifications)
	}

	if _, err := repo.CreateCommentNotifications(ctx, []domain.Notification{commentNotification}); err != nil {
		t.Fatalf("repeat CreateCommentNotifications() error = %v", err)
	}
	if _, err := repo.CreateMentionNotifications(ctx, []domain.Notification{mentionNotification}); err != nil {
		t.Fatalf("repeat CreateMentionNotifications() error = %v", err)
	}
	if unreadCount, err := repo.GetUnreadCount(ctx, member.ID); err != nil {
		t.Fatalf("GetUnreadCount() after duplicate replay error = %v", err)
	} else if unreadCount != 2 {
		t.Fatalf("expected unread count stable after duplicate replay, got %d", unreadCount)
	}
}

func TestNotificationReplayIdempotencyInvitationLiveReadState(t *testing.T) {
	pool := integrationPool(t)
	repo := NewNotificationRepository(pool)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	owner := seedUser(t, pool, "replay-invite-owner@example.com")
	member := seedUser(t, pool, "replay-invite-member@example.com")
	workspace, _ := seedWorkspaceWithOwner(t, pool, owner)

	invitationResourceType := domain.NotificationResourceTypeInvitation
	invitationActionKind := domain.NotificationActionKindInvitationResponse
	invitationID := uuid.NewString()
	createdAt := now
	created, err := repo.UpsertInvitationLive(ctx, domain.Notification{
		ID:           uuid.NewString(),
		UserID:       member.ID,
		WorkspaceID:  workspace.ID,
		Type:         domain.NotificationTypeInvitation,
		EventID:      invitationID,
		Message:      "You have a new workspace invitation",
		ActorID:      &owner.ID,
		Title:        "Workspace invitation",
		Content:      "You have a new workspace invitation",
		Actionable:   true,
		ActionKind:   &invitationActionKind,
		ResourceType: &invitationResourceType,
		ResourceID:   &invitationID,
		Payload: mustNotificationPayload(t, map[string]any{
			"status":        "pending",
			"version":       1,
			"can_accept":    true,
			"can_reject":    true,
			"workspace_id":  workspace.ID,
			"invitation_id": invitationID,
			"email":         member.Email,
			"role":          string(domain.RoleViewer),
		}),
		CreatedAt: createdAt,
		UpdatedAt: createdAt,
	})
	if err != nil {
		t.Fatalf("UpsertInvitationLive() create error = %v", err)
	}
	if unreadCount, err := repo.GetUnreadCount(ctx, member.ID); err != nil {
		t.Fatalf("GetUnreadCount() after invitation create error = %v", err)
	} else if unreadCount != 1 {
		t.Fatalf("expected unread count 1 after invitation create, got %d", unreadCount)
	}

	readAt := now.Add(time.Minute)
	if _, err := repo.MarkRead(ctx, created.ID, member.ID, readAt); err != nil {
		t.Fatalf("MarkRead() error = %v", err)
	}
	if unreadCount, err := repo.GetUnreadCount(ctx, member.ID); err != nil {
		t.Fatalf("GetUnreadCount() after mark read error = %v", err)
	} else if unreadCount != 0 {
		t.Fatalf("expected unread count 0 after mark read, got %d", unreadCount)
	}

	updated, err := repo.UpsertInvitationLive(ctx, domain.Notification{
		ID:           uuid.NewString(),
		UserID:       member.ID,
		WorkspaceID:  workspace.ID,
		Type:         domain.NotificationTypeInvitation,
		EventID:      invitationID,
		Message:      "Invitation accepted",
		ActorID:      &owner.ID,
		Title:        "Invitation accepted",
		Content:      "Invitation accepted",
		Actionable:   false,
		ActionKind:   nil,
		ResourceType: &invitationResourceType,
		ResourceID:   &invitationID,
		Payload: mustNotificationPayload(t, map[string]any{
			"status":        "accepted",
			"version":       2,
			"can_accept":    false,
			"can_reject":    false,
			"workspace_id":  workspace.ID,
			"invitation_id": invitationID,
			"email":         member.Email,
			"role":          string(domain.RoleViewer),
		}),
		CreatedAt: now.Add(2 * time.Minute),
		UpdatedAt: now.Add(2 * time.Minute),
	})
	if err != nil {
		t.Fatalf("UpsertInvitationLive() update error = %v", err)
	}
	if updated.ID != created.ID {
		t.Fatalf("expected live update to keep the same row id, got create=%s update=%s", created.ID, updated.ID)
	}
	if !updated.IsRead || updated.ReadAt == nil || !updated.ReadAt.Equal(readAt) {
		t.Fatalf("expected live update to preserve read state, got %+v", updated)
	}
	var payload map[string]any
	if err := json.Unmarshal(updated.Payload, &payload); err != nil {
		t.Fatalf("unmarshal replay invitation payload: %v", err)
	}
	if payload["invitation_id"] != invitationID || payload["workspace_id"] != workspace.ID || payload["email"] != member.Email || payload["role"] != string(domain.RoleViewer) || payload["status"] != string(domain.WorkspaceInvitationStatusAccepted) {
		t.Fatalf("unexpected replay invitation payload: %+v", payload)
	}
	if unreadCount, err := repo.GetUnreadCount(ctx, member.ID); err != nil {
		t.Fatalf("GetUnreadCount() after invitation update error = %v", err)
	} else if unreadCount != 0 {
		t.Fatalf("expected unread count 0 after invitation update, got %d", unreadCount)
	}

	if _, err := repo.UpsertInvitationLive(ctx, domain.Notification{
		ID:           uuid.NewString(),
		UserID:       member.ID,
		WorkspaceID:  workspace.ID,
		Type:         domain.NotificationTypeInvitation,
		EventID:      invitationID,
		Message:      "Invitation accepted",
		ActorID:      &owner.ID,
		Title:        "Invitation accepted",
		Content:      "Invitation accepted",
		Actionable:   false,
		ResourceType: &invitationResourceType,
		ResourceID:   &invitationID,
		Payload: mustNotificationPayload(t, map[string]any{
			"status":        "accepted",
			"version":       2,
			"can_accept":    false,
			"can_reject":    false,
			"workspace_id":  workspace.ID,
			"invitation_id": invitationID,
		}),
		CreatedAt: now.Add(3 * time.Minute),
		UpdatedAt: now.Add(3 * time.Minute),
	}); err != nil {
		t.Fatalf("repeat UpsertInvitationLive() error = %v", err)
	}
	if unreadCount, err := repo.GetUnreadCount(ctx, member.ID); err != nil {
		t.Fatalf("GetUnreadCount() after repeated invitation update error = %v", err)
	} else if unreadCount != 0 {
		t.Fatalf("expected unread count to remain 0 after repeated invitation update, got %d", unreadCount)
	}

	if _, err := repo.MarkRead(ctx, created.ID, member.ID, now.Add(4*time.Minute)); err != nil {
		t.Fatalf("repeat MarkRead() error = %v", err)
	}
	if unreadCount, err := repo.GetUnreadCount(ctx, member.ID); err != nil {
		t.Fatalf("GetUnreadCount() after repeated mark read error = %v", err)
	} else if unreadCount != 0 {
		t.Fatalf("expected unread count to stay 0 after repeated mark read, got %d", unreadCount)
	}

	rows, err := repo.ListByUserID(ctx, member.ID)
	if err != nil {
		t.Fatalf("ListByUserID() error = %v", err)
	}
	liveRows := 0
	for _, row := range rows {
		if row.Type == domain.NotificationTypeInvitation && row.EventID == invitationID {
			liveRows++
		}
	}
	if liveRows != 1 {
		t.Fatalf("expected one live invitation row after replay, got %d rows=%+v", liveRows, rows)
	}
}

func mustNotificationPayload(t *testing.T, value any) json.RawMessage {
	t.Helper()

	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal notification payload: %v", err)
	}
	return encoded
}

func TestNotificationReplayIdempotencyRejectsDuplicateLogicalRows(t *testing.T) {
	pool := integrationPool(t)
	repo := NewNotificationRepository(pool)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	owner := seedUser(t, pool, "replay-guard-owner@example.com")
	member := seedUser(t, pool, "replay-guard-member@example.com")
	workspace, _ := seedWorkspaceWithOwner(t, pool, owner)

	threadMsgResourceType := domain.NotificationResourceTypeThreadMsg
	messageID := uuid.NewString()
	notification := domain.Notification{
		ID:           uuid.NewString(),
		UserID:       member.ID,
		WorkspaceID:  workspace.ID,
		Type:         domain.NotificationTypeComment,
		EventID:      messageID,
		Message:      "A relevant comment thread has a new reply",
		Title:        "New thread reply",
		Content:      "A relevant comment thread has a new reply",
		ResourceType: &threadMsgResourceType,
		ResourceID:   &messageID,
		Payload: mustNotificationPayload(t, map[string]any{
			"thread_id":    "thread-1",
			"message_id":   messageID,
			"page_id":      "page-1",
			"workspace_id": workspace.ID,
			"event_topic":  string(domain.OutboxTopicThreadReplyCreated),
		}),
		CreatedAt: now,
		UpdatedAt: now,
	}

	if _, err := repo.CreateCommentNotifications(ctx, []domain.Notification{notification}); err != nil {
		t.Fatalf("CreateCommentNotifications() error = %v", err)
	}
	if _, err := repo.MarkRead(ctx, notification.ID, member.ID, now.Add(time.Minute)); err != nil {
		t.Fatalf("MarkRead() error = %v", err)
	}
	if unreadCount, err := repo.GetUnreadCount(ctx, member.ID); err != nil {
		t.Fatalf("GetUnreadCount() after first mark read error = %v", err)
	} else if unreadCount != 0 {
		t.Fatalf("expected unread count 0 after mark read, got %d", unreadCount)
	}
	if _, err := repo.MarkRead(ctx, notification.ID, member.ID, now.Add(2*time.Minute)); err != nil {
		t.Fatalf("repeat MarkRead() error = %v", err)
	}
	if unreadCount, err := repo.GetUnreadCount(ctx, member.ID); err != nil {
		t.Fatalf("GetUnreadCount() after repeated mark read error = %v", err)
	} else if unreadCount != 0 {
		t.Fatalf("expected unread count to remain 0 after repeated mark read, got %d", unreadCount)
	}

	rows, err := repo.ListByUserID(ctx, member.ID)
	if err != nil {
		t.Fatalf("ListByUserID() error = %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected one notification row after replay guard, got %+v", rows)
	}

	dup := notification
	dup.ID = uuid.NewString()
	if _, err := repo.CreateCommentNotifications(ctx, []domain.Notification{dup}); err != nil {
		t.Fatalf("duplicate CreateCommentNotifications() error = %v", err)
	}
	if unreadCount, err := repo.GetUnreadCount(ctx, member.ID); err != nil {
		t.Fatalf("GetUnreadCount() after duplicate replay guard error = %v", err)
	} else if unreadCount != 0 {
		t.Fatalf("expected unread count to remain 0 after duplicate replay guard, got %d", unreadCount)
	}

	_ = owner
}
