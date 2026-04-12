package application

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"note-app/internal/domain"

	"github.com/google/uuid"
)

type projectionNotificationSinkStub struct {
	rows        map[string]domain.Notification
	inserted    []string
	failAfter   int
	failed      bool
	failureText string
}

func (s *projectionNotificationSinkStub) CreateCommentNotifications(_ context.Context, notifications []domain.Notification) (int, error) {
	return s.insert(notifications)
}

func (s *projectionNotificationSinkStub) CreateMentionNotifications(_ context.Context, notifications []domain.Notification) (int, error) {
	return s.insert(notifications)
}

func (s *projectionNotificationSinkStub) CreateCommentAndMentionNotifications(_ context.Context, commentNotifications, mentionNotifications []domain.Notification) (int, int, error) {
	commentCount, err := s.insert(commentNotifications)
	if err != nil {
		return commentCount, 0, err
	}
	mentionCount, err := s.insert(mentionNotifications)
	if err != nil {
		return commentCount, mentionCount, err
	}
	return commentCount, mentionCount, nil
}

func (s *projectionNotificationSinkStub) insert(notifications []domain.Notification) (int, error) {
	if s.rows == nil {
		s.rows = map[string]domain.Notification{}
	}
	inserted := 0
	for _, notification := range notifications {
		key := projectionNotificationKey(notification.UserID, notification.Type, notification.EventID)
		if _, exists := s.rows[key]; exists {
			continue
		}
		s.rows[key] = notification
		s.inserted = append(s.inserted, key)
		inserted++
		if s.failAfter > 0 && !s.failed && inserted >= s.failAfter {
			s.failed = true
			if s.failureText == "" {
				s.failureText = "transient notification sink failure"
			}
			return inserted, errors.New(s.failureText)
		}
	}
	return inserted, nil
}

func (s *projectionNotificationSinkStub) has(userID string, typ domain.NotificationType, eventID string) bool {
	_, ok := s.rows[projectionNotificationKey(userID, typ, eventID)]
	return ok
}

func projectionNotificationKey(userID string, typ domain.NotificationType, eventID string) string {
	return userID + ":" + string(typ) + ":" + eventID
}

func invitationLiveKey(userID, eventID string) string {
	return userID + ":" + eventID
}

func TestNotificationProjectionConcurrencyInvitationReplayPreservesReadState(t *testing.T) {
	now := time.Date(2026, 4, 7, 3, 0, 0, 0, time.UTC)
	readAt := now.Add(-15 * time.Minute)
	invitationID := uuid.NewString()
	user := domain.User{ID: uuid.NewString(), Email: "invitee@example.com", FullName: "Invitee"}
	outbox := &invitationProjectorOutboxRepoStub{
		claimed: []domain.OutboxEvent{
			{
				ID:           uuid.NewString(),
				Topic:        domain.OutboxTopicInvitationUpdated,
				Payload:      json.RawMessage(`{"invitation_id":"` + invitationID + `","workspace_id":"workspace-1","actor_id":"owner-1","email":"invitee@example.com","role":"editor","status":"pending","version":2,"occurred_at":"2026-04-07T03:00:00Z"}`),
				AttemptCount: 1,
			},
		},
	}
	sink := &invitationProjectorNotificationRepoStub{
		nextByKey: map[string]domain.Notification{
			invitationLiveKey(user.ID, invitationID): {
				ID:          invitationID,
				UserID:      user.ID,
				WorkspaceID: "workspace-1",
				Type:        domain.NotificationTypeInvitation,
				EventID:     invitationID,
				Message:     "Workspace invitation",
				Title:       "Workspace invitation",
				Content:     "Workspace invitation",
				ReadAt:      &readAt,
				IsRead:      true,
				CreatedAt:   now.Add(-time.Hour),
				UpdatedAt:   readAt,
			},
		},
	}
	users := &fakeUserRepo{
		byEmail: map[string]domain.User{"invitee@example.com": user},
		byID:    map[string]domain.User{user.ID: user},
	}

	projector := NewInvitationNotificationProjector(outbox, sink, users)

	first, err := projector.ProcessBatch(context.Background(), "worker-1", 10, time.Minute, now)
	if err != nil {
		t.Fatalf("first ProcessBatch() error = %v", err)
	}
	second, err := projector.ProcessBatch(context.Background(), "worker-1", 10, time.Minute, now)
	if err != nil {
		t.Fatalf("second ProcessBatch() error = %v", err)
	}
	if first.Processed != 1 || second.Processed != 1 {
		t.Fatalf("expected replayed invitation event to process cleanly, got first=%+v second=%+v", first, second)
	}
	if len(sink.nextByKey) != 1 {
		t.Fatalf("expected one live invitation row, got %+v", sink.nextByKey)
	}
	stored := sink.nextByKey[invitationLiveKey(user.ID, invitationID)]
	if stored.ReadAt == nil || !stored.IsRead || !stored.ReadAt.Equal(readAt) {
		t.Fatalf("expected read state to be preserved, got %+v", stored)
	}
	if stored.Title != "Workspace invitation updated" {
		t.Fatalf("expected updated invitation title, got %+v", stored)
	}
}

func TestNotificationProjectionConcurrencyCommentReplayAndRetry(t *testing.T) {
	now := time.Date(2026, 4, 7, 4, 0, 0, 0, time.UTC)
	workspaceID := "workspace-1"
	pageID := "page-1"
	threadID := "thread-1"
	creatorID := "user-creator"
	replierID := "user-replier"
	actorID := "user-actor"
	mentionA := "user-mention-a"
	mentionB := "user-mention-b"
	messageID := uuid.NewString()

	threadDetail := domain.PageCommentThreadDetail{
		Thread: domain.PageCommentThread{
			ID:             threadID,
			PageID:         pageID,
			CreatedBy:      creatorID,
			CreatedAt:      now,
			LastActivityAt: now.Add(2 * time.Minute),
		},
		Messages: []domain.PageCommentThreadMessage{
			{ID: uuid.NewString(), ThreadID: threadID, Body: "starter", CreatedBy: creatorID, CreatedAt: now},
			{ID: uuid.NewString(), ThreadID: threadID, Body: "prior reply", CreatedBy: replierID, CreatedAt: now.Add(time.Minute)},
			{ID: messageID, ThreadID: threadID, Body: "current reply", CreatedBy: actorID, CreatedAt: now.Add(2 * time.Minute)},
		},
	}

	payload := mustMarshalJSON(t, map[string]any{
		"thread_id":        threadID,
		"message_id":       messageID,
		"page_id":          pageID,
		"workspace_id":     workspaceID,
		"actor_id":         actorID,
		"occurred_at":      now.Add(2 * time.Minute).Format(time.RFC3339),
		"mention_user_ids": []string{creatorID, replierID, mentionA, mentionB, actorID},
	})
	outbox := &invitationProjectorOutboxRepoStub{
		claimed: []domain.OutboxEvent{
			{
				ID:      uuid.NewString(),
				Topic:   domain.OutboxTopicThreadReplyCreated,
				Payload: payload,
			},
		},
	}
	sink := &projectionNotificationSinkStub{failAfter: 3}
	workspaceRepo := &fakeWorkspaceRepo{
		memberships: map[string][]domain.WorkspaceMember{
			workspaceID: {
				{ID: uuid.NewString(), WorkspaceID: workspaceID, UserID: creatorID, Role: domain.RoleOwner, CreatedAt: now},
				{ID: uuid.NewString(), WorkspaceID: workspaceID, UserID: replierID, Role: domain.RoleEditor, CreatedAt: now},
				{ID: uuid.NewString(), WorkspaceID: workspaceID, UserID: mentionA, Role: domain.RoleViewer, CreatedAt: now},
				{ID: uuid.NewString(), WorkspaceID: workspaceID, UserID: mentionB, Role: domain.RoleViewer, CreatedAt: now},
				{ID: uuid.NewString(), WorkspaceID: workspaceID, UserID: actorID, Role: domain.RoleEditor, CreatedAt: now},
			},
		},
	}
	commentProjector := NewCommentNotificationProjector(
		outbox,
		&fakeThreadRepo{details: map[string]domain.PageCommentThreadDetail{threadID: threadDetail}},
		&fakePageRepo{
			pages: map[string]domain.Page{
				pageID: {ID: pageID, WorkspaceID: workspaceID, Title: "Doc"},
			},
			drafts: map[string]domain.PageDraft{
				pageID: {PageID: pageID, Content: json.RawMessage(`[]`)},
			},
		},
		workspaceRepo,
		NewThreadNotificationRecipientResolver(workspaceRepo),
		sink,
	)

	first, err := commentProjector.ProcessBatch(context.Background(), "worker-2", 10, time.Minute, now)
	if err != nil {
		t.Fatalf("first ProcessBatch() error = %v", err)
	}
	if first.Retried != 1 {
		t.Fatalf("expected first run to retry, got %+v", first)
	}

	second, err := commentProjector.ProcessBatch(context.Background(), "worker-2", 10, time.Minute, now)
	if err != nil {
		t.Fatalf("second ProcessBatch() error = %v", err)
	}
	third, err := commentProjector.ProcessBatch(context.Background(), "worker-2", 10, time.Minute, now)
	if err != nil {
		t.Fatalf("third ProcessBatch() error = %v", err)
	}
	if second.Processed != 1 || third.Processed != 1 {
		t.Fatalf("expected successful replays after retry, got second=%+v third=%+v", second, third)
	}
	if len(sink.rows) != 8 {
		t.Fatalf("expected four comment rows and four mention rows, got %d rows: %+v", len(sink.rows), sink.rows)
	}
	if !sink.has(creatorID, domain.NotificationTypeComment, messageID) || !sink.has(creatorID, domain.NotificationTypeMention, messageID) {
		t.Fatalf("expected comment and mention rows to coexist for creator, got %+v", sink.rows)
	}
	if !sink.has(mentionB, domain.NotificationTypeComment, messageID) || !sink.has(mentionB, domain.NotificationTypeMention, messageID) {
		t.Fatalf("expected comment and mention rows to coexist for mention target, got %+v", sink.rows)
	}
	if sink.has(actorID, domain.NotificationTypeMention, messageID) {
		t.Fatalf("expected actor to be excluded from mention notifications, got %+v", sink.rows)
	}
	if len(sink.inserted) != 8 {
		t.Fatalf("expected eight unique notification inserts, got %d (%+v)", len(sink.inserted), sink.inserted)
	}
}

func TestNotificationProjectionConcurrencyMentionReplayAndCommentCoexistence(t *testing.T) {
	now := time.Date(2026, 4, 7, 5, 0, 0, 0, time.UTC)
	workspaceID := "workspace-2"
	pageID := "page-2"
	threadID := "thread-2"
	actorID := "user-actor"
	commentRecipient := "user-comment"
	mentionRecipient := "user-mention"

	sink := &projectionNotificationSinkStub{}
	threadMsgResourceType := domain.NotificationResourceTypeThreadMsg
	messageID := uuid.NewString()
	commentNotification := domain.Notification{
		ID:           uuid.NewString(),
		UserID:       commentRecipient,
		WorkspaceID:  workspaceID,
		Type:         domain.NotificationTypeComment,
		EventID:      messageID,
		Message:      "A relevant comment thread has a new reply",
		Title:        "New thread reply",
		Content:      "A relevant comment thread has a new reply",
		ResourceType: &threadMsgResourceType,
		ResourceID:   &messageID,
		Payload: mustMarshalJSON(t, map[string]any{
			"thread_id":    threadID,
			"message_id":   messageID,
			"page_id":      pageID,
			"workspace_id": workspaceID,
			"actor_id":     actorID,
			"event_topic":  string(domain.OutboxTopicThreadReplyCreated),
		}),
		CreatedAt: now,
		UpdatedAt: now,
	}
	if _, err := sink.CreateCommentNotifications(context.Background(), []domain.Notification{commentNotification}); err != nil {
		t.Fatalf("seed comment notification: %v", err)
	}

	projector := NewMentionNotificationProjector(&mentionProjectorMembershipStub{
		members: []domain.WorkspaceMember{
			{UserID: commentRecipient},
			{UserID: mentionRecipient},
			{UserID: actorID},
		},
	}, sink)

	payload := commentNotificationPayload{
		ThreadID:       threadID,
		MessageID:      messageID,
		PageID:         pageID,
		WorkspaceID:    workspaceID,
		ActorID:        actorID,
		OccurredAt:     now,
		MentionUserIDs: []string{commentRecipient, mentionRecipient, actorID, commentRecipient},
	}

	first, err := projector.Project(context.Background(), domain.OutboxTopicThreadReplyCreated, payload)
	if err != nil {
		t.Fatalf("first mention projection error = %v", err)
	}
	second, err := projector.Project(context.Background(), domain.OutboxTopicThreadReplyCreated, payload)
	if err != nil {
		t.Fatalf("second mention projection error = %v", err)
	}
	if !first || !second {
		t.Fatalf("expected mention projector to emit recipients on both runs, got first=%t second=%t", first, second)
	}
	if len(sink.rows) != 3 {
		t.Fatalf("expected one comment row and two mention rows, got %+v", sink.rows)
	}
	if !sink.has(commentRecipient, domain.NotificationTypeComment, messageID) || !sink.has(commentRecipient, domain.NotificationTypeMention, messageID) {
		t.Fatalf("expected comment and mention coexistence for shared recipient, got %+v", sink.rows)
	}
	if !sink.has(mentionRecipient, domain.NotificationTypeMention, messageID) {
		t.Fatalf("expected mention recipient row, got %+v", sink.rows)
	}
	if sink.has(actorID, domain.NotificationTypeMention, messageID) {
		t.Fatalf("expected actor to be excluded from mention notifications, got %+v", sink.rows)
	}
}

func TestNotificationProjectionConcurrencyMentionRetryCompletesUniqueRows(t *testing.T) {
	now := time.Date(2026, 4, 7, 5, 30, 0, 0, time.UTC)
	workspaceID := "workspace-mention-retry"
	pageID := "page-mention-retry"
	threadID := "thread-mention-retry"
	actorID := "user-actor"
	mentionA := "user-mention-a"
	mentionB := "user-mention-b"
	messageID := uuid.NewString()

	sink := &projectionNotificationSinkStub{failAfter: 1}
	projector := NewMentionNotificationProjector(&mentionProjectorMembershipStub{
		members: []domain.WorkspaceMember{
			{UserID: actorID},
			{UserID: mentionA},
			{UserID: mentionB},
		},
	}, sink)

	payload := commentNotificationPayload{
		ThreadID:       threadID,
		MessageID:      messageID,
		PageID:         pageID,
		WorkspaceID:    workspaceID,
		ActorID:        actorID,
		OccurredAt:     now,
		MentionUserIDs: []string{mentionA, mentionB, actorID, mentionA},
	}

	if _, err := projector.Project(context.Background(), domain.OutboxTopicThreadReplyCreated, payload); err == nil {
		t.Fatal("expected first mention projection to fail transiently")
	}
	if _, err := projector.Project(context.Background(), domain.OutboxTopicThreadReplyCreated, payload); err != nil {
		t.Fatalf("second mention projection error = %v", err)
	}
	if _, err := projector.Project(context.Background(), domain.OutboxTopicThreadReplyCreated, payload); err != nil {
		t.Fatalf("third mention projection error = %v", err)
	}
	if len(sink.rows) != 2 {
		t.Fatalf("expected two unique mention rows after retry, got %+v", sink.rows)
	}
	if !sink.has(mentionA, domain.NotificationTypeMention, messageID) || !sink.has(mentionB, domain.NotificationTypeMention, messageID) {
		t.Fatalf("expected both mention recipients after retry, got %+v", sink.rows)
	}
	if sink.has(actorID, domain.NotificationTypeMention, messageID) {
		t.Fatalf("expected actor to be excluded from self-mention notifications, got %+v", sink.rows)
	}
	if len(sink.inserted) != 2 {
		t.Fatalf("expected retry to create only two unique mention rows, got %+v", sink.inserted)
	}
}

func TestNotificationProjectionConcurrencyRecipientDedupeAndActorExclusion(t *testing.T) {
	now := time.Date(2026, 4, 7, 6, 0, 0, 0, time.UTC)
	thread := domain.PageCommentThread{
		ID:        "thread-3",
		PageID:    "page-3",
		CreatedBy: "user-creator",
		CreatedAt: now,
	}
	got, err := BuildThreadNotificationHistory(ThreadNotificationHistoryInput{
		Thread: thread,
		Messages: []domain.PageCommentThreadMessage{
			{ID: "m1", ThreadID: thread.ID, CreatedBy: "user-creator", CreatedAt: now},
			{ID: "m2", ThreadID: thread.ID, CreatedBy: "user-replier", CreatedAt: now.Add(time.Minute)},
			{ID: "m3", ThreadID: thread.ID, CreatedBy: "user-actor", CreatedAt: now.Add(2 * time.Minute)},
		},
		ExplicitMentionsByMessageID: map[string][]string{
			"m3": {"user-creator", "user-replier", "user-actor", "user-replier", "user-creator"},
		},
		WorkspaceMemberIDs: []string{"user-creator", "user-replier", "user-actor"},
	})
	if err != nil {
		t.Fatalf("BuildThreadNotificationHistory() error = %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected three history entries, got %+v", got)
	}
	wantCommentRecipients := []string{"user-creator", "user-replier"}
	wantMentionRecipients := []string{"user-creator", "user-replier"}
	if len(got[2].CommentRecipients) != len(wantCommentRecipients) {
		t.Fatalf("unexpected comment recipients: %+v", got[2].CommentRecipients)
	}
	for idx, want := range wantCommentRecipients {
		if got[2].CommentRecipients[idx] != want {
			t.Fatalf("comment recipient[%d] = %q, want %q", idx, got[2].CommentRecipients[idx], want)
		}
	}
	if len(got[2].MentionRecipients) != len(wantMentionRecipients) {
		t.Fatalf("unexpected mention recipients: %+v", got[2].MentionRecipients)
	}
	for idx, want := range wantMentionRecipients {
		if got[2].MentionRecipients[idx] != want {
			t.Fatalf("mention recipient[%d] = %q, want %q", idx, got[2].MentionRecipients[idx], want)
		}
	}
}

func mustMarshalJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()

	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal JSON: %v", err)
	}
	return encoded
}
