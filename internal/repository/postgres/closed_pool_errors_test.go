package postgres

import (
	"context"
	"encoding/json"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"note-app/internal/domain"
	"note-app/internal/infrastructure/database"
	"note-app/internal/testutil/testenv"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func closedPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, thisFile, _, _ := runtime.Caller(0)
	projectRoot := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", ".."))
	dsn, err := testenv.ResolvePostgresDSN(projectRoot)
	if err != nil {
		t.Fatalf("resolve postgres test dsn: %v", err)
	}

	pool, err := database.NewPool(ctx, dsn)
	if err != nil {
		t.Fatalf("create pool for closed-pool tests: %v", err)
	}
	pool.Close()
	return pool
}
func TestRepositoriesReturnErrorsWhenPoolClosed(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	pool := closedPool(t)

	userRepo := NewUserRepository(pool)
	if _, err := userRepo.Create(ctx, domain.User{ID: uuid.NewString(), Email: "a@b.com", FullName: "A", PasswordHash: "x", CreatedAt: now, UpdatedAt: now}); err == nil {
		t.Fatal("expected user create error on closed pool")
	}
	if _, err := userRepo.GetByEmail(ctx, "a@b.com"); err == nil {
		t.Fatal("expected user get by email error on closed pool")
	}
	if _, err := userRepo.GetByID(ctx, uuid.NewString()); err == nil {
		t.Fatal("expected user get by id error on closed pool")
	}

	refreshRepo := NewRefreshTokenRepository(pool)
	if _, err := refreshRepo.Create(ctx, domain.RefreshToken{ID: uuid.NewString(), UserID: uuid.NewString(), TokenHash: "h", ExpiresAt: now.Add(time.Hour), CreatedAt: now}); err == nil {
		t.Fatal("expected refresh create error on closed pool")
	}
	if _, err := refreshRepo.GetByHash(ctx, "h"); err == nil {
		t.Fatal("expected refresh get error on closed pool")
	}
	if err := refreshRepo.RevokeByID(ctx, uuid.NewString(), now); err == nil {
		t.Fatal("expected refresh revoke error on closed pool")
	}

	workspaceRepo := NewWorkspaceRepository(pool)
	if _, _, err := workspaceRepo.CreateWithOwner(ctx, domain.Workspace{ID: uuid.NewString(), Name: "W", CreatedAt: now, UpdatedAt: now}, domain.WorkspaceMember{ID: uuid.NewString(), WorkspaceID: uuid.NewString(), UserID: uuid.NewString(), Role: domain.RoleOwner, CreatedAt: now}); err == nil {
		t.Fatal("expected workspace create error on closed pool")
	}
	if _, err := workspaceRepo.GetMembershipByUserID(ctx, uuid.NewString(), uuid.NewString()); err == nil {
		t.Fatal("expected get membership error on closed pool")
	}
	if _, err := workspaceRepo.GetMembershipByID(ctx, uuid.NewString(), uuid.NewString()); err == nil {
		t.Fatal("expected get membership by id error on closed pool")
	}
	if _, err := workspaceRepo.HasWorkspaceWithNameForUserExcludingID(ctx, uuid.NewString(), "w", uuid.NewString()); err == nil {
		t.Fatal("expected workspace duplicate check with exclusion error on closed pool")
	}
	if _, err := workspaceRepo.CreateInvitation(ctx, domain.WorkspaceInvitation{ID: uuid.NewString(), WorkspaceID: uuid.NewString(), Email: "a@b.com", Role: domain.RoleEditor, InvitedBy: uuid.NewString(), CreatedAt: now}); err == nil {
		t.Fatal("expected create invitation error on closed pool")
	}
	if _, err := workspaceRepo.GetActiveInvitationByEmail(ctx, uuid.NewString(), "a@b.com"); err == nil {
		t.Fatal("expected get active invitation error on closed pool")
	}
	if _, err := workspaceRepo.GetInvitationByID(ctx, uuid.NewString()); err == nil {
		t.Fatal("expected get invitation by id error on closed pool")
	}
	if _, err := workspaceRepo.AcceptInvitation(ctx, uuid.NewString(), uuid.NewString(), 1, now); err == nil {
		t.Fatal("expected accept invitation error on closed pool")
	}
	if _, err := workspaceRepo.ListMembers(ctx, uuid.NewString()); err == nil {
		t.Fatal("expected list members error on closed pool")
	}
	if _, err := workspaceRepo.UpdateMemberRole(ctx, uuid.NewString(), uuid.NewString(), domain.RoleViewer); err == nil {
		t.Fatal("expected update member role error on closed pool")
	}
	if _, err := workspaceRepo.CountOwners(ctx, uuid.NewString()); err == nil {
		t.Fatal("expected count owners error on closed pool")
	}

	folderRepo := NewFolderRepository(pool)
	if _, err := folderRepo.Create(ctx, domain.Folder{ID: uuid.NewString(), WorkspaceID: uuid.NewString(), Name: "F", CreatedAt: now, UpdatedAt: now}); err == nil {
		t.Fatal("expected folder create error on closed pool")
	}
	if _, err := folderRepo.GetByID(ctx, uuid.NewString()); err == nil {
		t.Fatal("expected folder get error on closed pool")
	}
	if _, err := folderRepo.ListByWorkspaceID(ctx, uuid.NewString()); err == nil {
		t.Fatal("expected folder list error on closed pool")
	}

	pageRepo := NewPageRepository(pool)
	if _, _, err := pageRepo.CreateWithDraft(ctx, domain.Page{ID: uuid.NewString(), WorkspaceID: uuid.NewString(), Title: "P", CreatedBy: uuid.NewString(), CreatedAt: now, UpdatedAt: now}, domain.PageDraft{PageID: uuid.NewString(), Content: json.RawMessage(`[]`), LastEditedBy: uuid.NewString(), CreatedAt: now, UpdatedAt: now}); err == nil {
		t.Fatal("expected page create error on closed pool")
	}
	if _, _, err := pageRepo.GetByID(ctx, uuid.NewString()); err == nil {
		t.Fatal("expected page get error on closed pool")
	}
	if _, err := pageRepo.ListByWorkspaceIDAndFolderID(ctx, uuid.NewString(), nil); err == nil {
		t.Fatal("expected page list error on closed pool")
	}
	if _, err := pageRepo.UpdateMetadata(ctx, uuid.NewString(), "x", nil, now); err == nil {
		t.Fatal("expected page update metadata error on closed pool")
	}
	if _, err := pageRepo.UpdateDraft(ctx, uuid.NewString(), json.RawMessage(`[]`), uuid.NewString(), now); err == nil {
		t.Fatal("expected page update draft error on closed pool")
	}
	if _, err := pageRepo.SearchPages(ctx, uuid.NewString(), "q"); err == nil {
		t.Fatal("expected page search error on closed pool")
	}
	if err := pageRepo.SoftDelete(ctx, domain.TrashItem{ID: uuid.NewString(), WorkspaceID: uuid.NewString(), PageID: uuid.NewString(), PageTitle: "x", DeletedBy: uuid.NewString(), DeletedAt: now}); err == nil {
		t.Fatal("expected page soft delete error on closed pool")
	}
	if _, err := pageRepo.ListTrashByWorkspaceID(ctx, uuid.NewString()); err == nil {
		t.Fatal("expected list trash error on closed pool")
	}
	if _, err := pageRepo.GetTrashItemByID(ctx, uuid.NewString()); err == nil {
		t.Fatal("expected get trash item error on closed pool")
	}
	if _, _, _, err := pageRepo.GetTrashedByTrashItemID(ctx, uuid.NewString()); err == nil {
		t.Fatal("expected get trashed page error on closed pool")
	}
	if _, err := pageRepo.RestoreTrashItem(ctx, uuid.NewString(), uuid.NewString(), now); err == nil {
		t.Fatal("expected restore trash item error on closed pool")
	}

	threadPreferenceRepo := NewThreadNotificationPreferenceRepository(pool)
	if _, err := threadPreferenceRepo.GetThreadNotificationPreference(ctx, uuid.NewString(), uuid.NewString()); err == nil {
		t.Fatal("expected thread notification preference read error on closed pool")
	}
	if err := threadPreferenceRepo.SetThreadNotificationPreference(ctx, domain.ThreadNotificationPreference{ThreadID: uuid.NewString(), UserID: uuid.NewString(), Mode: domain.ThreadNotificationModeMute, CreatedAt: now, UpdatedAt: now}); err == nil {
		t.Fatal("expected thread notification preference write error on closed pool")
	}

	revisionRepo := NewRevisionRepository(pool)
	if _, err := revisionRepo.Create(ctx, domain.Revision{ID: uuid.NewString(), PageID: uuid.NewString(), Content: json.RawMessage(`[]`), CreatedBy: uuid.NewString(), CreatedAt: now}); err == nil {
		t.Fatal("expected revision create error on closed pool")
	}
	if _, err := revisionRepo.GetByID(ctx, uuid.NewString()); err == nil {
		t.Fatal("expected revision get error on closed pool")
	}
	if _, err := revisionRepo.ListByPageID(ctx, uuid.NewString()); err == nil {
		t.Fatal("expected revision list error on closed pool")
	}

	commentRepo := NewCommentRepository(pool)
	if _, err := commentRepo.Create(ctx, domain.PageComment{ID: uuid.NewString(), PageID: uuid.NewString(), Body: "x", CreatedBy: uuid.NewString(), CreatedAt: now}); err == nil {
		t.Fatal("expected comment create error on closed pool")
	}
	if _, err := commentRepo.GetByID(ctx, uuid.NewString()); err == nil {
		t.Fatal("expected comment get error on closed pool")
	}
	if _, err := commentRepo.ListByPageID(ctx, uuid.NewString()); err == nil {
		t.Fatal("expected comment list error on closed pool")
	}
	if _, err := commentRepo.Resolve(ctx, uuid.NewString(), uuid.NewString(), now); err == nil {
		t.Fatal("expected comment resolve error on closed pool")
	}

	threadRepo := NewThreadRepository(pool)
	blockID := uuid.NewString()
	thread := domain.PageCommentThread{
		ID:     uuid.NewString(),
		PageID: uuid.NewString(),
		Anchor: domain.PageCommentThreadAnchor{
			Type:            domain.PageCommentThreadAnchorTypeBlock,
			BlockID:         &blockID,
			QuotedBlockText: "x",
		},
		ThreadState:    domain.PageCommentThreadStateOpen,
		AnchorState:    domain.PageCommentThreadAnchorStateActive,
		CreatedBy:      uuid.NewString(),
		CreatedAt:      now,
		LastActivityAt: now,
	}
	message := domain.PageCommentThreadMessage{ID: uuid.NewString(), ThreadID: thread.ID, Body: "x", CreatedBy: thread.CreatedBy, CreatedAt: now}
	if _, err := threadRepo.CreateThread(ctx, thread, message, nil, domain.OutboxEvent{}); err == nil {
		t.Fatal("expected thread create error on closed pool")
	}
	if _, err := threadRepo.GetThread(ctx, uuid.NewString()); err == nil {
		t.Fatal("expected thread get error on closed pool")
	}
	if _, err := threadRepo.ListThreads(ctx, uuid.NewString(), nil, nil, nil, nil, nil, "", "", 0, ""); err == nil {
		t.Fatal("expected thread list error on closed pool")
	}
	if _, err := threadRepo.ListWorkspaceThreads(ctx, uuid.NewString(), nil, nil, nil, nil, nil, "", "", 0, ""); err == nil {
		t.Fatal("expected workspace thread list error on closed pool")
	}
	if _, err := threadRepo.AddReply(ctx, thread.ID, domain.PageCommentThreadMessage{ID: uuid.NewString(), ThreadID: thread.ID, Body: "reply", CreatedBy: thread.CreatedBy, CreatedAt: now}, nil, thread, domain.OutboxEvent{}); err == nil {
		t.Fatal("expected thread reply error on closed pool")
	}
	if _, err := threadRepo.UpdateThreadState(ctx, thread.ID, thread, nil); err == nil {
		t.Fatal("expected thread state update error on closed pool")
	}

	notificationRepo := NewNotificationRepository(pool)
	if _, err := notificationRepo.Create(ctx, domain.Notification{ID: uuid.NewString(), UserID: uuid.NewString(), WorkspaceID: uuid.NewString(), Type: domain.NotificationTypeComment, EventID: uuid.NewString(), Message: "x", CreatedAt: now}); err == nil {
		t.Fatal("expected notification create error on closed pool")
	}
	if err := notificationRepo.CreateMany(ctx, []domain.Notification{{ID: uuid.NewString(), UserID: uuid.NewString(), WorkspaceID: uuid.NewString(), Type: domain.NotificationTypeComment, EventID: uuid.NewString(), Message: "x", CreatedAt: now}}); err == nil {
		t.Fatal("expected notification batch create error on closed pool")
	}
	threadMsgResourceType := domain.NotificationResourceTypeThreadMsg
	threadResourceID := uuid.NewString()
	if _, err := notificationRepo.CreateCommentNotifications(ctx, []domain.Notification{{ID: uuid.NewString(), UserID: uuid.NewString(), WorkspaceID: uuid.NewString(), Type: domain.NotificationTypeComment, EventID: threadResourceID, Message: "x", ResourceType: &threadMsgResourceType, ResourceID: &threadResourceID, Payload: json.RawMessage(`{"thread_id":"thread-1","message_id":"message-1","page_id":"page-1","workspace_id":"workspace-1","event_topic":"thread_reply_created"}`), CreatedAt: now}}); err == nil {
		t.Fatal("expected comment notification batch create error on closed pool")
	}
	if _, err := notificationRepo.CreateMentionNotifications(ctx, []domain.Notification{{ID: uuid.NewString(), UserID: uuid.NewString(), WorkspaceID: uuid.NewString(), Type: domain.NotificationTypeMention, EventID: threadResourceID, Message: "x", ResourceType: &threadMsgResourceType, ResourceID: &threadResourceID, Payload: json.RawMessage(`{"thread_id":"thread-1","message_id":"message-1","page_id":"page-1","workspace_id":"workspace-1","event_topic":"thread_reply_created","mention_source":"explicit"}`), CreatedAt: now}}); err == nil {
		t.Fatal("expected mention notification batch create error on closed pool")
	}
	if _, _, err := notificationRepo.CreateCommentAndMentionNotifications(ctx, []domain.Notification{{ID: uuid.NewString(), UserID: uuid.NewString(), WorkspaceID: uuid.NewString(), Type: domain.NotificationTypeComment, EventID: threadResourceID, Message: "x", ResourceType: &threadMsgResourceType, ResourceID: &threadResourceID, Payload: json.RawMessage(`{"thread_id":"thread-1","message_id":"message-1","page_id":"page-1","workspace_id":"workspace-1","event_topic":"thread_reply_created"}`), CreatedAt: now}}, []domain.Notification{{ID: uuid.NewString(), UserID: uuid.NewString(), WorkspaceID: uuid.NewString(), Type: domain.NotificationTypeMention, EventID: threadResourceID, Message: "x", ResourceType: &threadMsgResourceType, ResourceID: &threadResourceID, Payload: json.RawMessage(`{"thread_id":"thread-1","message_id":"message-1","page_id":"page-1","workspace_id":"workspace-1","event_topic":"thread_reply_created","mention_source":"explicit"}`), CreatedAt: now}}); err == nil {
		t.Fatal("expected combined comment and mention create error on closed pool")
	}
	if _, err := notificationRepo.ListByUserID(ctx, uuid.NewString()); err == nil {
		t.Fatal("expected notification list error on closed pool")
	}
	if _, err := notificationRepo.MarkRead(ctx, uuid.NewString(), uuid.NewString(), now); err == nil {
		t.Fatal("expected notification mark read error on closed pool")
	}
	if _, err := notificationRepo.BatchMarkRead(ctx, uuid.NewString(), []string{uuid.NewString()}, now); err == nil {
		t.Fatal("expected notification batch mark read error on closed pool")
	}
	if _, err := notificationRepo.GetUnreadCount(ctx, uuid.NewString()); err == nil {
		t.Fatal("expected notification unread count error on closed pool")
	}
	invitationResourceType := domain.NotificationResourceTypeInvitation
	invitationID := uuid.NewString()
	if _, err := notificationRepo.UpsertInvitationLive(ctx, domain.Notification{
		ID:           uuid.NewString(),
		UserID:       uuid.NewString(),
		WorkspaceID:  uuid.NewString(),
		Type:         domain.NotificationTypeInvitation,
		EventID:      invitationID,
		Message:      "x",
		ResourceType: &invitationResourceType,
		ResourceID:   &invitationID,
		Payload:      json.RawMessage(`{"status":"pending","version":1}`),
		CreatedAt:    now,
		UpdatedAt:    now,
	}); err == nil {
		t.Fatal("expected notification invitation live upsert error on closed pool")
	}

	reconcileRepo := NewNotificationReconciliationRepository(pool)
	if _, err := reconcileRepo.AcquireReconciliationLock(ctx); err == nil {
		t.Fatal("expected notification reconciliation lock error on closed pool")
	}
	if _, err := reconcileRepo.ListInvitationSources(ctx, uuid.NewString(), now, 1, ""); err == nil {
		t.Fatal("expected notification reconciliation invitation scan error on closed pool")
	}
	if _, err := reconcileRepo.ListThreadSources(ctx, uuid.NewString(), now, 1, ""); err == nil {
		t.Fatal("expected notification reconciliation thread scan error on closed pool")
	}
	if _, err := reconcileRepo.LoadThreadHistory(ctx, uuid.NewString(), now); err == nil {
		t.Fatal("expected notification reconciliation history error on closed pool")
	}
	if _, err := reconcileRepo.ListManagedNotifications(ctx, uuid.NewString(), now, []domain.NotificationType{domain.NotificationTypeComment}); err == nil {
		t.Fatal("expected notification reconciliation managed listing error on closed pool")
	}
	if _, err := reconcileRepo.UpsertManagedNotification(ctx, domain.Notification{ID: uuid.NewString()}); err == nil {
		t.Fatal("expected notification reconciliation upsert error on closed pool")
	}
	if _, err := reconcileRepo.DeleteManagedNotifications(ctx, []string{uuid.NewString()}); err == nil {
		t.Fatal("expected notification reconciliation delete error on closed pool")
	}
	if _, err := reconcileRepo.CountUnreadNotifications(ctx, uuid.NewString()); err == nil {
		t.Fatal("expected notification reconciliation unread count error on closed pool")
	}
	if _, err := reconcileRepo.UpsertUnreadCounter(ctx, uuid.NewString(), 1, now); err == nil {
		t.Fatal("expected notification reconciliation counter upsert error on closed pool")
	}
	if _, err := reconcileRepo.DeleteUnreadCounter(ctx, uuid.NewString()); err == nil {
		t.Fatal("expected notification reconciliation counter delete error on closed pool")
	}

	outboxRepo := NewOutboxRepository(pool)
	if _, err := outboxRepo.Create(ctx, domain.OutboxEvent{ID: uuid.NewString(), Topic: domain.OutboxTopicInvitationCreated, AggregateType: domain.OutboxAggregateTypeInvitation, AggregateID: uuid.NewString(), IdempotencyKey: "closed-pool-outbox-create", Payload: []byte(`{"ok":true}`), CreatedAt: now, UpdatedAt: now}); err == nil {
		t.Fatal("expected outbox create error on closed pool")
	}
	if err := outboxRepo.CreateMany(ctx, []domain.OutboxEvent{{ID: uuid.NewString(), Topic: domain.OutboxTopicInvitationCreated, AggregateType: domain.OutboxAggregateTypeInvitation, AggregateID: uuid.NewString(), IdempotencyKey: "closed-pool-outbox-batch", Payload: []byte(`{"ok":true}`), CreatedAt: now, UpdatedAt: now}}); err == nil {
		t.Fatal("expected outbox batch create error on closed pool")
	}
	if _, err := outboxRepo.ClaimPending(ctx, "worker-closed", 1, time.Minute, now); err == nil {
		t.Fatal("expected outbox claim error on closed pool")
	}
	if _, err := outboxRepo.ClaimPendingByTopics(ctx, "worker-closed", []domain.OutboxTopic{domain.OutboxTopicInvitationCreated}, 1, time.Minute, now); err == nil {
		t.Fatal("expected outbox topic-scoped claim error on closed pool")
	}
	if _, err := outboxRepo.MarkProcessed(ctx, uuid.NewString(), "worker-closed", now); err == nil {
		t.Fatal("expected outbox mark processed error on closed pool")
	}
	if _, err := outboxRepo.MarkRetry(ctx, uuid.NewString(), "worker-closed", "x", now.Add(time.Minute), now); err == nil {
		t.Fatal("expected outbox mark retry error on closed pool")
	}
	if _, err := outboxRepo.MarkDeadLetter(ctx, uuid.NewString(), "worker-closed", "x", now); err == nil {
		t.Fatal("expected outbox mark dead-letter error on closed pool")
	}
}
