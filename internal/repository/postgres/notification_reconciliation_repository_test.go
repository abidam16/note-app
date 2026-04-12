package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"note-app/internal/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func TestNotificationReconciliationRepositoryIntegration(t *testing.T) {
	pool := integrationPool(t)
	repo := NewNotificationReconciliationRepository(pool)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	t.Run("advisory lock is exclusive", func(t *testing.T) {
		lock, err := repo.AcquireReconciliationLock(ctx)
		if err != nil {
			t.Fatalf("AcquireReconciliationLock() error = %v", err)
		}
		defer func() {
			if lock != nil {
				_ = lock.Release(ctx)
			}
		}()

		secondRepo := NewNotificationReconciliationRepository(pool)
		if _, err := secondRepo.AcquireReconciliationLock(ctx); err == nil {
			t.Fatal("expected second lock acquisition to fail")
		}
	})

	t.Run("invitation scans honor workspace scope cutoff and ordering", func(t *testing.T) {
		owner := seedUser(t, pool, "reconcile-inv-owner@example.com")
		invited := seedUser(t, pool, "reconcile-inv-user@example.com")
		workspace, _ := seedWorkspaceWithOwner(t, pool, owner)
		otherWorkspaceOwner := seedUser(t, pool, "reconcile-inv-other-owner@example.com")
		otherWorkspace, _ := seedWorkspaceWithOwner(t, pool, otherWorkspaceOwner)

		oldInvitation := domain.WorkspaceInvitation{
			ID:          uuid.NewString(),
			WorkspaceID: workspace.ID,
			Email:       invited.Email,
			Role:        domain.RoleViewer,
			InvitedBy:   owner.ID,
			CreatedAt:   now,
			Status:      domain.WorkspaceInvitationStatusPending,
			Version:     1,
			UpdatedAt:   now,
		}
		newerInvitation := domain.WorkspaceInvitation{
			ID:          uuid.NewString(),
			WorkspaceID: workspace.ID,
			Email:       "reconcile-inv-new@example.com",
			Role:        domain.RoleEditor,
			InvitedBy:   owner.ID,
			CreatedAt:   now.Add(time.Minute),
			Status:      domain.WorkspaceInvitationStatusPending,
			Version:     1,
			UpdatedAt:   now.Add(time.Minute),
		}
		otherWorkspaceInvitation := domain.WorkspaceInvitation{
			ID:          uuid.NewString(),
			WorkspaceID: otherWorkspace.ID,
			Email:       invited.Email,
			Role:        domain.RoleEditor,
			InvitedBy:   otherWorkspaceOwner.ID,
			CreatedAt:   now.Add(2 * time.Minute),
			Status:      domain.WorkspaceInvitationStatusPending,
			Version:     1,
			UpdatedAt:   now.Add(2 * time.Minute),
		}
		updatedAfterCutoff := domain.WorkspaceInvitation{
			ID:          uuid.NewString(),
			WorkspaceID: workspace.ID,
			Email:       "reconcile-inv-updated-after@example.com",
			Role:        domain.RoleViewer,
			InvitedBy:   owner.ID,
			CreatedAt:   now.Add(30 * time.Second),
			Status:      domain.WorkspaceInvitationStatusPending,
			Version:     2,
			UpdatedAt:   now.Add(3 * time.Minute),
		}
		for _, invitation := range []domain.WorkspaceInvitation{oldInvitation, newerInvitation, otherWorkspaceInvitation, updatedAfterCutoff} {
			if _, err := NewWorkspaceRepository(pool).CreateInvitation(ctx, invitation); err != nil {
				t.Fatalf("create invitation: %v", err)
			}
		}

		page, err := repo.ListInvitationSources(ctx, workspace.ID, now.Add(90*time.Second), 10, "")
		if err != nil {
			t.Fatalf("ListInvitationSources() error = %v", err)
		}
		if len(page.Items) != 2 {
			t.Fatalf("expected two workspace invitations before cutoff, got %+v", page.Items)
		}
		if page.Items[0].Invitation.ID != oldInvitation.ID || page.Items[1].Invitation.ID != newerInvitation.ID {
			t.Fatalf("unexpected invitation ordering: %+v", page.Items)
		}
		if page.Items[0].RegisteredUserID == nil || *page.Items[0].RegisteredUserID != invited.ID {
			t.Fatalf("expected first invitation to resolve registered user, got %+v", page.Items[0])
		}
		if page.Items[1].RegisteredUserID != nil {
			t.Fatalf("expected unregistered invitee to remain nil, got %+v", page.Items[1])
		}
	})

	t.Run("invitation upsert converges a wrong-recipient row in one run", func(t *testing.T) {
		owner := seedUser(t, pool, "reconcile-inv-repair-owner@example.com")
		invited := seedUser(t, pool, "reconcile-inv-repair-user@example.com")
		wrong := seedUser(t, pool, "reconcile-inv-repair-wrong@example.com")
		workspace, _ := seedWorkspaceWithOwner(t, pool, owner)
		invitation := domain.WorkspaceInvitation{
			ID:          uuid.NewString(),
			WorkspaceID: workspace.ID,
			Email:       invited.Email,
			Role:        domain.RoleEditor,
			InvitedBy:   owner.ID,
			CreatedAt:   now,
			Status:      domain.WorkspaceInvitationStatusPending,
			Version:     1,
			UpdatedAt:   now,
		}
		if _, err := NewWorkspaceRepository(pool).CreateInvitation(ctx, invitation); err != nil {
			t.Fatalf("create invitation: %v", err)
		}

		wrongResourceType := domain.NotificationResourceTypeInvitation
		wrongResourceID := invitation.ID
		blocking := domain.Notification{
			ID:           invitation.ID,
			UserID:       wrong.ID,
			WorkspaceID:  workspace.ID,
			Type:         domain.NotificationTypeInvitation,
			EventID:      invitation.ID,
			Message:      "stale invitation",
			Title:        "Workspace invitation",
			Content:      "stale invitation",
			ResourceType: &wrongResourceType,
			ResourceID:   &wrongResourceID,
			Payload:      json.RawMessage(`{"invitation_id":"` + invitation.ID + `","status":"pending","version":1}`),
			CreatedAt:    now,
			UpdatedAt:    now,
		}
		mustExec(t, pool, `
			INSERT INTO notifications (id, user_id, workspace_id, type, event_id, message, created_at, read_at, actor_id, title, content, is_read, actionable, action_kind, resource_type, resource_id, payload, updated_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)
		`, blocking.ID, blocking.UserID, blocking.WorkspaceID, blocking.Type, blocking.EventID, blocking.Message, blocking.CreatedAt, blocking.ReadAt, blocking.ActorID, blocking.Title, blocking.Content, blocking.IsRead, blocking.Actionable, blocking.ActionKind, blocking.ResourceType, blocking.ResourceID, blocking.Payload, blocking.UpdatedAt)

		expected := domain.Notification{
			ID:          invitation.ID,
			UserID:      invited.ID,
			WorkspaceID: workspace.ID,
			Type:        domain.NotificationTypeInvitation,
			EventID:     invitation.ID,
			Message:     "You have a new workspace invitation",
			Title:       "Workspace invitation",
			Content:     "You have a new workspace invitation",
			Actionable:  true,
			CreatedAt:   invitation.CreatedAt,
			UpdatedAt:   invitation.UpdatedAt,
		}
		expectedResourceType := domain.NotificationResourceTypeInvitation
		expectedResourceID := invitation.ID
		expected.ActionKind = func() *domain.NotificationActionKind {
			value := domain.NotificationActionKindInvitationResponse
			return &value
		}()
		expected.ResourceType = &expectedResourceType
		expected.ResourceID = &expectedResourceID
		expected.Payload = json.RawMessage(`{"invitation_id":"` + invitation.ID + `","workspace_id":"` + workspace.ID + `","email":"` + invited.Email + `","role":"` + string(invitation.Role) + `","status":"pending","version":1,"can_accept":true,"can_reject":true}`)

		mutated, err := repo.UpsertManagedNotification(ctx, expected)
		if err != nil {
			t.Fatalf("UpsertManagedNotification() error = %v", err)
		}
		if mutated.Notification.UserID != invited.ID {
			t.Fatalf("expected wrong-recipient row to converge to invited user, got %+v", mutated.Notification)
		}
		if mutated.Notification.ReadAt != nil || mutated.Notification.IsRead {
			t.Fatalf("expected read state preserved on unread row, got %+v", mutated.Notification)
		}
	})

	t.Run("thread history loads ordered messages and mention rows", func(t *testing.T) {
		owner := seedUser(t, pool, "reconcile-thread-owner@example.com")
		replier := seedUser(t, pool, "reconcile-thread-replier@example.com")
		mentioned := seedUser(t, pool, "reconcile-thread-mentioned@example.com")
		workspace, _ := seedWorkspaceWithOwner(t, pool, owner)
		if _, err := pool.Exec(ctx, `INSERT INTO workspace_members (id, workspace_id, user_id, role, created_at) VALUES ($1,$2,$3,$4,$5)`, uuid.NewString(), workspace.ID, replier.ID, domain.RoleEditor, now); err != nil {
			t.Fatalf("seed replier membership: %v", err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO workspace_members (id, workspace_id, user_id, role, created_at) VALUES ($1,$2,$3,$4,$5)`, uuid.NewString(), workspace.ID, mentioned.ID, domain.RoleViewer, now); err != nil {
			t.Fatalf("seed mention membership: %v", err)
		}
		page, draft := seedPageWithDraft(t, pool, workspace.ID, owner.ID, nil, "Thread page")
		threadRepo := NewThreadRepository(pool)
		thread := domain.PageCommentThread{
			ID:             uuid.NewString(),
			PageID:         page.ID,
			Anchor:         domain.PageCommentThreadAnchor{Type: domain.PageCommentThreadAnchorTypePageLegacy, QuotedBlockText: "hello"},
			ThreadState:    domain.PageCommentThreadStateOpen,
			AnchorState:    domain.PageCommentThreadAnchorStateActive,
			CreatedBy:      owner.ID,
			CreatedAt:      now,
			LastActivityAt: now.Add(2 * time.Minute),
			ReplyCount:     2,
		}
		first := domain.PageCommentThreadMessage{ID: uuid.NewString(), ThreadID: thread.ID, Body: "first", CreatedBy: owner.ID, CreatedAt: now}
		second := domain.PageCommentThreadMessage{ID: uuid.NewString(), ThreadID: thread.ID, Body: "second", CreatedBy: replier.ID, CreatedAt: now.Add(time.Minute)}
		third := domain.PageCommentThreadMessage{ID: uuid.NewString(), ThreadID: thread.ID, Body: "third", CreatedBy: mentioned.ID, CreatedAt: now.Add(2 * time.Minute)}
		outboxEvent, err := domain.NewThreadCreatedOutboxEvent(thread, first, workspace.ID, []string{mentioned.ID})
		if err != nil {
			t.Fatalf("build outbox event: %v", err)
		}
		if _, err := threadRepo.CreateThread(ctx, thread, first, []domain.PageCommentMessageMention{{MessageID: first.ID, MentionedUserID: mentioned.ID}}, outboxEvent); err != nil {
			t.Fatalf("create thread: %v", err)
		}
		mustExec(t, pool, `INSERT INTO page_comment_messages (id, thread_id, body, created_by, created_at) VALUES ($1,$2,$3,$4,$5)`, second.ID, thread.ID, second.Body, second.CreatedBy, second.CreatedAt)
		mustExec(t, pool, `INSERT INTO page_comment_messages (id, thread_id, body, created_by, created_at) VALUES ($1,$2,$3,$4,$5)`, third.ID, thread.ID, third.Body, third.CreatedBy, third.CreatedAt)
		mustExec(t, pool, `INSERT INTO page_comment_message_mentions (message_id, mentioned_user_id) VALUES ($1,$2)`, second.ID, replier.ID)
		mustExec(t, pool, `INSERT INTO page_comment_message_mentions (message_id, mentioned_user_id) VALUES ($1,$2)`, second.ID, mentioned.ID)

		history, err := repo.LoadThreadHistory(ctx, thread.ID, now.Add(3*time.Minute))
		if err != nil {
			t.Fatalf("LoadThreadHistory() error = %v", err)
		}
		if history.Thread.ID != thread.ID {
			t.Fatalf("unexpected thread history thread: %+v", history.Thread)
		}
		if len(history.Messages) != 3 || history.Messages[0].ID != first.ID || history.Messages[1].ID != second.ID || history.Messages[2].ID != third.ID {
			t.Fatalf("unexpected message ordering: %+v", history.Messages)
		}
		if got := history.ExplicitMentionsByMessageID[first.ID]; len(got) != 1 || got[0] != mentioned.ID {
			t.Fatalf("unexpected first-message mention history: %+v", history.ExplicitMentionsByMessageID)
		}
		if got := history.ExplicitMentionsByMessageID[second.ID]; len(got) != 2 || !containsAll(got, replier.ID, mentioned.ID) {
			t.Fatalf("unexpected second-message mention history: %+v", history.ExplicitMentionsByMessageID)
		}
		if len(history.WorkspaceMemberIDs) != 3 {
			t.Fatalf("expected workspace member ids, got %+v", history.WorkspaceMemberIDs)
		}
		_ = draft
	})

	t.Run("managed listings exclude legacy rows", func(t *testing.T) {
		owner := seedUser(t, pool, "reconcile-managed-owner@example.com")
		workspace, _ := seedWorkspaceWithOwner(t, pool, owner)
		managedUser := seedUser(t, pool, "reconcile-managed-user@example.com")
		legacyUser := seedUser(t, pool, "reconcile-managed-legacy@example.com")

		resourceType := domain.NotificationResourceTypeThreadMsg
		resourceID := uuid.NewString()
		managed := domain.Notification{
			ID:           uuid.NewString(),
			UserID:       managedUser.ID,
			WorkspaceID:  workspace.ID,
			Type:         domain.NotificationTypeComment,
			EventID:      resourceID,
			Message:      "comment",
			Title:        "New thread reply",
			Content:      "comment",
			ResourceType: &resourceType,
			ResourceID:   &resourceID,
			Payload:      json.RawMessage(`{"thread_id":"thread-1","message_id":"message-1","page_id":"page-1","workspace_id":"workspace-1","event_topic":"thread_reply_created"}`),
			CreatedAt:    now,
			UpdatedAt:    now,
		}
		if _, err := NewNotificationRepository(pool).CreateCommentNotifications(ctx, []domain.Notification{managed}); err != nil {
			t.Fatalf("create managed notification: %v", err)
		}
		mustExec(t, pool, `
			INSERT INTO notifications (id, user_id, workspace_id, type, event_id, message, created_at, read_at, actor_id, title, content, is_read, actionable, action_kind, resource_type, resource_id, payload, updated_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)
		`, legacyUser.ID, legacyUser.ID, workspace.ID, domain.NotificationTypeComment, uuid.NewString(), "legacy", now, nil, nil, "Legacy", "legacy", false, false, nil, nil, nil, json.RawMessage(`{}`), now)
		malformedID := uuid.NewString()
		mustExec(t, pool, `
			INSERT INTO notifications (id, user_id, workspace_id, type, event_id, message, created_at, read_at, actor_id, title, content, is_read, actionable, action_kind, resource_type, resource_id, payload, updated_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)
		`, malformedID, managedUser.ID, workspace.ID, domain.NotificationTypeComment, uuid.NewString(), "malformed", now, nil, nil, "Malformed", "malformed", false, false, nil, resourceType, &resourceID, json.RawMessage(`{"thread_id":"thread-1"}`), now)

		rows, err := repo.ListManagedNotifications(ctx, workspace.ID, now.Add(time.Minute), []domain.NotificationType{domain.NotificationTypeComment})
		if err != nil {
			t.Fatalf("ListManagedNotifications() error = %v", err)
		}
		if len(rows) != 1 || rows[0].UserID != managedUser.ID {
			t.Fatalf("expected only managed comment row, got %+v", rows)
		}
	})

	t.Run("upsert preserves read state and exact counters are stored", func(t *testing.T) {
		owner := seedUser(t, pool, "reconcile-counter-owner@example.com")
		workspace, _ := seedWorkspaceWithOwner(t, pool, owner)
		readAt := now.Add(-time.Minute)
		invitationID := uuid.NewString()
		invitationResourceType := domain.NotificationResourceTypeInvitation
		existing := domain.Notification{
			ID:           uuid.NewString(),
			UserID:       owner.ID,
			WorkspaceID:  workspace.ID,
			Type:         domain.NotificationTypeInvitation,
			EventID:      invitationID,
			Message:      "old",
			Title:        "Workspace invitation",
			Content:      "old",
			ReadAt:       &readAt,
			IsRead:       true,
			ResourceType: &invitationResourceType,
			ResourceID:   &invitationID,
			Payload:      json.RawMessage(`{"invitation_id":"` + invitationID + `","status":"pending","version":1}`),
			CreatedAt:    now,
			UpdatedAt:    now,
		}
		mustExec(t, pool, `
			INSERT INTO notifications (id, user_id, workspace_id, type, event_id, message, created_at, read_at, actor_id, title, content, is_read, actionable, action_kind, resource_type, resource_id, payload, updated_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)
		`, existing.ID, existing.UserID, existing.WorkspaceID, existing.Type, existing.EventID, existing.Message, existing.CreatedAt, existing.ReadAt, nil, existing.Title, existing.Content, existing.IsRead, false, nil, existing.ResourceType, existing.ResourceID, existing.Payload, existing.UpdatedAt)

		repaired := existing
		repaired.Message = "new"
		repaired.Content = "new"
		repaired.UpdatedAt = now.Add(time.Minute)
		mutated, err := repo.UpsertManagedNotification(ctx, repaired)
		if err != nil {
			t.Fatalf("UpsertManagedNotification() error = %v", err)
		}
		if mutated.Notification.ReadAt == nil || !mutated.Notification.ReadAt.Equal(readAt) || !mutated.Notification.IsRead {
			t.Fatalf("expected read state preserved, got %+v", mutated)
		}

		changed, err := repo.UpsertUnreadCounter(ctx, owner.ID, 3, now.Add(2*time.Minute))
		if err != nil {
			t.Fatalf("UpsertUnreadCounter() error = %v", err)
		}
		if !changed {
			t.Fatal("expected unread counter upsert to report change")
		}
		deleted, err := repo.DeleteUnreadCounter(ctx, owner.ID)
		if err != nil {
			t.Fatalf("DeleteUnreadCounter() error = %v", err)
		}
		if !deleted {
			t.Fatal("expected unread counter delete to report change")
		}
	})

	t.Run("blocking malformed rows return a clear conflict error", func(t *testing.T) {
		owner := seedUser(t, pool, "reconcile-block-owner@example.com")
		workspace, _ := seedWorkspaceWithOwner(t, pool, owner)
		recipient := seedUser(t, pool, "reconcile-block-recipient@example.com")
		eventID := uuid.NewString()
		mustExec(t, pool, `
			INSERT INTO notifications (id, user_id, workspace_id, type, event_id, message, created_at, read_at, actor_id, title, content, is_read, actionable, action_kind, resource_type, resource_id, payload, updated_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)
		`, uuid.NewString(), recipient.ID, workspace.ID, domain.NotificationTypeComment, eventID, "legacy comment", now, nil, nil, "Legacy", "legacy comment", false, false, nil, nil, nil, json.RawMessage(`{"thread_id":"thread-1","message_id":"`+eventID+`","page_id":"page-1","workspace_id":"`+workspace.ID+`","event_topic":"thread_reply_created"}`), now)

		resourceType := domain.NotificationResourceTypeThreadMsg
		resourceID := eventID
		_, err := repo.UpsertManagedNotification(ctx, domain.Notification{
			ID:           uuid.NewString(),
			UserID:       recipient.ID,
			WorkspaceID:  workspace.ID,
			Type:         domain.NotificationTypeComment,
			EventID:      eventID,
			Message:      "A relevant comment thread has a new reply",
			Title:        "New thread reply",
			Content:      "A relevant comment thread has a new reply",
			Actionable:   false,
			ResourceType: &resourceType,
			ResourceID:   &resourceID,
			Payload:      json.RawMessage(`{"thread_id":"thread-1","message_id":"` + eventID + `","page_id":"page-1","workspace_id":"` + workspace.ID + `","event_topic":"thread_reply_created"}`),
			CreatedAt:    now,
			UpdatedAt:    now,
		})
		if err == nil {
			t.Fatal("expected malformed blocking row error")
		}
		if !errors.Is(err, domain.ErrConflict) {
			t.Fatalf("expected conflict error, got %v", err)
		}
	})
}

func TestNotificationReconciliationRepositoryClosedPoolErrors(t *testing.T) {
	pool := closedPool(t)
	repo := NewNotificationReconciliationRepository(pool)
	ctx := context.Background()

	if _, err := repo.AcquireReconciliationLock(ctx); err == nil {
		t.Fatal("expected lock acquisition error on closed pool")
	}
	if _, err := repo.ListInvitationSources(ctx, uuid.NewString(), time.Now().UTC(), 1, ""); err == nil {
		t.Fatal("expected invitation source error on closed pool")
	}
	if _, err := repo.ListThreadSources(ctx, uuid.NewString(), time.Now().UTC(), 1, ""); err == nil {
		t.Fatal("expected thread source error on closed pool")
	}
	if _, err := repo.LoadThreadHistory(ctx, uuid.NewString(), time.Now().UTC()); err == nil {
		t.Fatal("expected thread history error on closed pool")
	}
	if _, err := repo.ListManagedNotifications(ctx, uuid.NewString(), time.Now().UTC(), []domain.NotificationType{domain.NotificationTypeComment}); err == nil {
		t.Fatal("expected managed notification listing error on closed pool")
	}
	if _, err := repo.UpsertManagedNotification(ctx, domain.Notification{ID: uuid.NewString()}); err == nil {
		t.Fatal("expected upsert error on closed pool")
	}
	if _, err := repo.DeleteManagedNotifications(ctx, []string{uuid.NewString()}); err == nil {
		t.Fatal("expected delete error on closed pool")
	}
	if _, err := repo.CountUnreadNotifications(ctx, uuid.NewString()); err == nil {
		t.Fatal("expected unread count error on closed pool")
	}
	if _, err := repo.UpsertUnreadCounter(ctx, uuid.NewString(), 1, time.Now().UTC()); err == nil {
		t.Fatal("expected counter upsert error on closed pool")
	}
	if _, err := repo.DeleteUnreadCounter(ctx, uuid.NewString()); err == nil {
		t.Fatal("expected counter delete error on closed pool")
	}
}

func TestNotificationReconciliationRepositoryDetectsNotFoundSources(t *testing.T) {
	pool := integrationPool(t)
	repo := NewNotificationReconciliationRepository(pool)
	ctx := context.Background()

	if _, err := repo.LoadThreadHistory(ctx, uuid.NewString(), time.Now().UTC()); !errors.Is(err, domain.ErrNotFound) && !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("expected not found for missing thread history, got %v", err)
	}
}

func containsAll(values []string, want ...string) bool {
	if len(values) != len(want) {
		return false
	}
	seen := make(map[string]int, len(values))
	for _, value := range values {
		seen[value]++
	}
	for _, value := range want {
		if seen[value] == 0 {
			return false
		}
		seen[value]--
	}
	return true
}
