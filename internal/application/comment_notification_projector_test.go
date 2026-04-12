package application

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"note-app/internal/domain"
)

type commentProjectorNotificationRepoStub struct {
	inserted []domain.Notification
	seen     map[string]domain.Notification
	err      error
}

func (s *commentProjectorNotificationRepoStub) CreateCommentNotifications(_ context.Context, notifications []domain.Notification) (int, error) {
	return s.createNotifications(notifications)
}

func (s *commentProjectorNotificationRepoStub) CreateMentionNotifications(_ context.Context, notifications []domain.Notification) (int, error) {
	return s.createNotifications(notifications)
}

func (s *commentProjectorNotificationRepoStub) CreateCommentAndMentionNotifications(_ context.Context, commentNotifications, mentionNotifications []domain.Notification) (int, int, error) {
	commentCount, err := s.createNotifications(commentNotifications)
	if err != nil {
		return 0, 0, err
	}
	mentionCount, err := s.createNotifications(mentionNotifications)
	if err != nil {
		return 0, 0, err
	}
	return commentCount, mentionCount, nil
}

func (s *commentProjectorNotificationRepoStub) createNotifications(notifications []domain.Notification) (int, error) {
	if s.err != nil {
		return 0, s.err
	}
	if s.seen == nil {
		s.seen = map[string]domain.Notification{}
	}
	inserted := 0
	for _, notification := range notifications {
		key := notification.UserID + ":" + string(notification.Type) + ":" + notification.EventID
		if _, ok := s.seen[key]; ok {
			continue
		}
		s.seen[key] = notification
		s.inserted = append(s.inserted, notification)
		inserted++
	}
	return inserted, nil
}

type commentProjectorResolverStub struct {
	inputs     []ResolveThreadNotificationRecipientsInput
	recipients []string
	err        error
}

func (s *commentProjectorResolverStub) ResolveRecipients(_ context.Context, input ResolveThreadNotificationRecipientsInput) ([]string, error) {
	s.inputs = append(s.inputs, input)
	if s.err != nil {
		return nil, s.err
	}
	return append([]string(nil), s.recipients...), nil
}

type commentProjectorThreadRepoStub struct {
	details map[string]domain.PageCommentThreadDetail
	err     error
}

func (s *commentProjectorThreadRepoStub) GetThread(_ context.Context, threadID string) (domain.PageCommentThreadDetail, error) {
	if s.err != nil {
		return domain.PageCommentThreadDetail{}, s.err
	}
	detail, ok := s.details[threadID]
	if !ok {
		return domain.PageCommentThreadDetail{}, domain.ErrNotFound
	}
	return detail, nil
}

func TestCommentNotificationProjector(t *testing.T) {
	now := time.Date(2026, 4, 7, 3, 0, 0, 0, time.UTC)
	workspaceID := "workspace-1"
	pageID := "page-1"
	threadID := "thread-1"
	createdMessageID := "message-created-1"
	replyMessageID := "message-reply-1"
	actorID := "user-actor"
	ownerID := "user-owner"
	replierID := "user-replier"
	mentionID := "user-mention"

	buildPayload := func(threadID, messageID, pageID, workspaceID, actorID string, occurredAt time.Time, mentionIDs ...string) json.RawMessage {
		payload := map[string]any{
			"thread_id":    threadID,
			"message_id":   messageID,
			"page_id":      pageID,
			"workspace_id": workspaceID,
			"actor_id":     actorID,
			"occurred_at":  occurredAt.Format(time.RFC3339),
		}
		if mentionIDs != nil {
			payload["mention_user_ids"] = mentionIDs
		}
		encoded, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("marshal projector payload: %v", err)
		}
		return encoded
	}

	buildThreadDetail := func(threadCreator string, messageAuthors ...string) domain.PageCommentThreadDetail {
		messages := make([]domain.PageCommentThreadMessage, 0, len(messageAuthors))
		events := make([]domain.PageCommentThreadEvent, 0, len(messageAuthors))
		for idx, authorID := range messageAuthors {
			messageID := createdMessageID
			if idx > 0 {
				messageID = replyMessageID
			}
			if idx > 1 {
				messageID = "message-extra-" + string(rune('0'+idx))
			}
			message := domain.PageCommentThreadMessage{
				ID:        messageID,
				ThreadID:  threadID,
				Body:      "message body",
				CreatedBy: authorID,
				CreatedAt: now.Add(time.Duration(idx) * time.Minute),
			}
			messages = append(messages, message)
			eventType := domain.PageCommentThreadEventTypeCreated
			if idx > 0 {
				eventType = domain.PageCommentThreadEventTypeReplied
			}
			actor := authorID
			events = append(events, domain.PageCommentThreadEvent{
				ID:        "event-" + messageID,
				ThreadID:  threadID,
				Type:      eventType,
				ActorID:   &actor,
				MessageID: &message.ID,
				CreatedAt: message.CreatedAt,
			})
		}

		thread := domain.PageCommentThread{
			ID:             threadID,
			PageID:         pageID,
			ThreadState:    domain.PageCommentThreadStateOpen,
			AnchorState:    domain.PageCommentThreadAnchorStateActive,
			CreatedBy:      threadCreator,
			CreatedAt:      now,
			LastActivityAt: now.Add(time.Duration(len(messages)) * time.Minute),
			ReplyCount:     len(messages) - 1,
		}
		return domain.PageCommentThreadDetail{Thread: thread, Messages: messages, Events: events}
	}

	buildReplyThreadDetail := func() domain.PageCommentThreadDetail {
		createdAt := now
		repliedAt := now.Add(time.Minute)
		replyAt := now.Add(2 * time.Minute)
		createdID := createdMessageID
		priorReplyID := "message-prior-reply-1"
		replyID := replyMessageID
		created := domain.PageCommentThreadMessage{
			ID:        createdID,
			ThreadID:  threadID,
			Body:      "thread starter",
			CreatedBy: ownerID,
			CreatedAt: createdAt,
		}
		priorReply := domain.PageCommentThreadMessage{
			ID:        priorReplyID,
			ThreadID:  threadID,
			Body:      "prior reply",
			CreatedBy: replierID,
			CreatedAt: repliedAt,
		}
		reply := domain.PageCommentThreadMessage{
			ID:        replyID,
			ThreadID:  threadID,
			Body:      "current reply",
			CreatedBy: actorID,
			CreatedAt: replyAt,
		}
		thread := domain.PageCommentThread{
			ID:             threadID,
			PageID:         pageID,
			ThreadState:    domain.PageCommentThreadStateOpen,
			AnchorState:    domain.PageCommentThreadAnchorStateActive,
			CreatedBy:      ownerID,
			CreatedAt:      createdAt,
			LastActivityAt: replyAt,
			ReplyCount:     2,
		}
		events := []domain.PageCommentThreadEvent{
			{
				ID:        "event-" + createdID,
				ThreadID:  threadID,
				Type:      domain.PageCommentThreadEventTypeCreated,
				ActorID:   &ownerID,
				MessageID: &createdID,
				CreatedAt: createdAt,
			},
			{
				ID:        "event-" + priorReplyID,
				ThreadID:  threadID,
				Type:      domain.PageCommentThreadEventTypeReplied,
				ActorID:   &replierID,
				MessageID: &priorReplyID,
				CreatedAt: repliedAt,
			},
			{
				ID:        "event-" + replyID,
				ThreadID:  threadID,
				Type:      domain.PageCommentThreadEventTypeReplied,
				ActorID:   &actorID,
				MessageID: &replyID,
				CreatedAt: replyAt,
			},
		}
		return domain.PageCommentThreadDetail{Thread: thread, Messages: []domain.PageCommentThreadMessage{created, priorReply, reply}, Events: events}
	}

	buildPageRepo := func(workspaceID string) *fakePageRepo {
		return &fakePageRepo{
			pages: map[string]domain.Page{
				pageID: {ID: pageID, WorkspaceID: workspaceID, Title: "Doc"},
			},
			drafts: map[string]domain.PageDraft{
				pageID: {PageID: pageID, Content: json.RawMessage(`[]`)},
			},
		}
	}

	t.Run("thread created with no relevant recipients is skipped", func(t *testing.T) {
		outbox := &invitationProjectorOutboxRepoStub{
			claimed: []domain.OutboxEvent{
				{
					ID:      "event-created",
					Topic:   domain.OutboxTopicThreadCreated,
					Payload: buildPayload(threadID, createdMessageID, pageID, workspaceID, actorID, now),
				},
			},
		}
		threadRepo := &fakeThreadRepo{details: map[string]domain.PageCommentThreadDetail{
			threadID: buildThreadDetail(actorID, actorID),
		}}
		resolver := NewThreadNotificationRecipientResolver(&fakeWorkspaceRepo{
			memberships: map[string][]domain.WorkspaceMember{
				workspaceID: {
					{ID: "member-actor", WorkspaceID: workspaceID, UserID: actorID, Role: domain.RoleOwner, CreatedAt: now},
				},
			},
		})
		notifications := &commentProjectorNotificationRepoStub{}
		projector := NewCommentNotificationProjector(outbox, threadRepo, buildPageRepo(workspaceID), &fakeWorkspaceRepo{memberships: map[string][]domain.WorkspaceMember{workspaceID: []domain.WorkspaceMember{{ID: "member-actor", WorkspaceID: workspaceID, UserID: actorID, Role: domain.RoleOwner, CreatedAt: now}}}}, resolver, notifications)

		result, err := projector.ProcessBatch(context.Background(), "worker-1", 10, time.Minute, now)
		if err != nil {
			t.Fatalf("ProcessBatch() error = %v", err)
		}
		if result.Claimed != 1 || result.Skipped != 1 || result.Processed != 0 || result.DeadLettered != 0 || result.Retried != 0 {
			t.Fatalf("unexpected projector result: %+v", result)
		}
		if len(notifications.inserted) != 0 {
			t.Fatalf("expected no notifications, got %+v", notifications.inserted)
		}
		if len(outbox.processedIDs) != 1 {
			t.Fatalf("expected processed comment outbox event, got %+v", outbox.processedIDs)
		}
		if len(outbox.claimedTopics) != 2 || outbox.claimedTopics[0] != domain.OutboxTopicThreadCreated || outbox.claimedTopics[1] != domain.OutboxTopicThreadReplyCreated {
			t.Fatalf("expected comment-topic claim filter, got %+v", outbox.claimedTopics)
		}
	})

	t.Run("thread reply resolves relevant recipients and forwards mentions", func(t *testing.T) {
		outbox := &invitationProjectorOutboxRepoStub{
			claimed: []domain.OutboxEvent{
				{
					ID:      "event-reply",
					Topic:   domain.OutboxTopicThreadReplyCreated,
					Payload: buildPayload(threadID, replyMessageID, pageID, workspaceID, actorID, now.Add(2*time.Minute), mentionID, "", actorID),
				},
			},
		}
		threadRepo := &fakeThreadRepo{details: map[string]domain.PageCommentThreadDetail{
			threadID: buildReplyThreadDetail(),
		}}
		workspaceRepo := &fakeWorkspaceRepo{
			memberships: map[string][]domain.WorkspaceMember{
				workspaceID: {
					{ID: "member-owner", WorkspaceID: workspaceID, UserID: ownerID, Role: domain.RoleOwner, CreatedAt: now},
					{ID: "member-replier", WorkspaceID: workspaceID, UserID: replierID, Role: domain.RoleEditor, CreatedAt: now},
					{ID: "member-mention", WorkspaceID: workspaceID, UserID: mentionID, Role: domain.RoleViewer, CreatedAt: now},
					{ID: "member-actor", WorkspaceID: workspaceID, UserID: actorID, Role: domain.RoleEditor, CreatedAt: now},
				},
			},
		}
		resolver := NewThreadNotificationRecipientResolver(workspaceRepo)
		notifications := &commentProjectorNotificationRepoStub{}
		projector := NewCommentNotificationProjector(outbox, threadRepo, buildPageRepo(workspaceID), workspaceRepo, resolver, notifications)

		result, err := projector.ProcessBatch(context.Background(), "worker-2", 10, time.Minute, now)
		if err != nil {
			t.Fatalf("ProcessBatch() error = %v", err)
		}
		if result.Claimed != 1 || result.Processed != 1 || result.Skipped != 0 || result.DeadLettered != 0 || result.Retried != 0 {
			t.Fatalf("unexpected projector result: %+v", result)
		}
		if len(notifications.inserted) != 4 {
			t.Fatalf("expected three comment notifications and one mention notification, got %+v", notifications.inserted)
		}
		if notifications.inserted[0].UserID != ownerID || notifications.inserted[1].UserID != replierID || notifications.inserted[2].UserID != mentionID {
			t.Fatalf("unexpected recipient order: %+v", notifications.inserted)
		}
		for i, notification := range notifications.inserted {
			if i < 3 {
				if notification.Type != domain.NotificationTypeComment || notification.EventID != replyMessageID || notification.ResourceType == nil || *notification.ResourceType != domain.NotificationResourceTypeThreadMsg {
					t.Fatalf("unexpected notification mapping: %+v", notification)
				}
				if notification.Title != "New thread reply" || notification.Content != "A relevant comment thread has a new reply" {
					t.Fatalf("unexpected comment notification text: %+v", notification)
				}
			} else {
				if notification.Type != domain.NotificationTypeMention || notification.EventID != replyMessageID || notification.ResourceType == nil || *notification.ResourceType != domain.NotificationResourceTypeThreadMsg {
					t.Fatalf("unexpected mention notification mapping: %+v", notification)
				}
				if notification.Title != "Mentioned in a thread reply" || notification.Content != "You were mentioned in a thread reply" {
					t.Fatalf("unexpected mention notification text: %+v", notification)
				}
			}
			if notification.Payload == nil || !json.Valid(notification.Payload) {
				t.Fatalf("unexpected notification mapping: %+v", notification)
			}
		}
		if len(outbox.processedIDs) != 1 {
			t.Fatalf("expected processed comment outbox event, got %+v", outbox.processedIDs)
		}
	})

	t.Run("mention ids are forwarded to the resolver", func(t *testing.T) {
		outbox := &invitationProjectorOutboxRepoStub{
			claimed: []domain.OutboxEvent{{
				ID:      "event-mentions",
				Topic:   domain.OutboxTopicThreadReplyCreated,
				Payload: buildPayload(threadID, replyMessageID, pageID, workspaceID, actorID, now.Add(2*time.Minute), mentionID, "", actorID),
			}},
		}
		threadRepo := &fakeThreadRepo{details: map[string]domain.PageCommentThreadDetail{
			threadID: buildReplyThreadDetail(),
		}}
		resolver := &commentProjectorResolverStub{recipients: []string{}}
		projector := NewCommentNotificationProjector(outbox, threadRepo, buildPageRepo(workspaceID), &fakeWorkspaceRepo{memberships: map[string][]domain.WorkspaceMember{}}, resolver, &commentProjectorNotificationRepoStub{})

		if _, err := projector.ProcessBatch(context.Background(), "worker-2b", 10, time.Minute, now); err != nil {
			t.Fatalf("ProcessBatch() error = %v", err)
		}
		if len(resolver.inputs) != 1 {
			t.Fatalf("expected one resolver call, got %+v", resolver.inputs)
		}
		if got := resolver.inputs[0].ExplicitMentionUserIDs; len(got) != 3 || got[0] != mentionID || got[1] != "" || got[2] != actorID {
			t.Fatalf("expected mention ids forwarded to resolver, got %+v", got)
		}
	})

	t.Run("only mention recipients still succeeds when comment recipients are empty", func(t *testing.T) {
		outbox := &invitationProjectorOutboxRepoStub{
			claimed: []domain.OutboxEvent{{
				ID:      "event-mention-only",
				Topic:   domain.OutboxTopicThreadReplyCreated,
				Payload: buildPayload(threadID, replyMessageID, pageID, workspaceID, actorID, now.Add(2*time.Minute), mentionID),
			}},
		}
		threadRepo := &fakeThreadRepo{details: map[string]domain.PageCommentThreadDetail{
			threadID: buildReplyThreadDetail(),
		}}
		workspaceRepo := &fakeWorkspaceRepo{
			memberships: map[string][]domain.WorkspaceMember{
				workspaceID: {
					{ID: "member-mention", WorkspaceID: workspaceID, UserID: mentionID, Role: domain.RoleViewer, CreatedAt: now},
				},
			},
		}
		resolver := &commentProjectorResolverStub{recipients: []string{}}
		notifications := &commentProjectorNotificationRepoStub{}
		projector := NewCommentNotificationProjector(outbox, threadRepo, buildPageRepo(workspaceID), workspaceRepo, resolver, notifications)

		result, err := projector.ProcessBatch(context.Background(), "worker-2c", 10, time.Minute, now)
		if err != nil {
			t.Fatalf("ProcessBatch() error = %v", err)
		}
		if result.Processed != 1 || result.Skipped != 0 || len(notifications.inserted) != 1 || notifications.inserted[0].Type != domain.NotificationTypeMention {
			t.Fatalf("expected mention-only event to process successfully, got result=%+v notifications=%+v", result, notifications.inserted)
		}
	})

	t.Run("malformed payload is dead lettered", func(t *testing.T) {
		outbox := &invitationProjectorOutboxRepoStub{
			claimed: []domain.OutboxEvent{{
				ID:      "event-bad",
				Topic:   domain.OutboxTopicThreadCreated,
				Payload: json.RawMessage(`{"thread_id":"","message_id":"bad"}`),
			}},
		}
		projector := NewCommentNotificationProjector(outbox, &fakeThreadRepo{details: map[string]domain.PageCommentThreadDetail{}}, buildPageRepo(workspaceID), &fakeWorkspaceRepo{memberships: map[string][]domain.WorkspaceMember{}}, NewThreadNotificationRecipientResolver(&fakeWorkspaceRepo{memberships: map[string][]domain.WorkspaceMember{}}), &commentProjectorNotificationRepoStub{})
		result, err := projector.ProcessBatch(context.Background(), "worker-3", 10, time.Minute, now)
		if err != nil {
			t.Fatalf("ProcessBatch() error = %v", err)
		}
		if result.DeadLettered != 1 || len(outbox.deadLettered) != 1 {
			t.Fatalf("expected dead letter for malformed payload, got result=%+v dead=%+v", result, outbox.deadLettered)
		}
	})

	t.Run("invalid JSON payload is dead lettered while later event still succeeds", func(t *testing.T) {
		outbox := &invitationProjectorOutboxRepoStub{
			claimed: []domain.OutboxEvent{
				{
					ID:      "event-bad-json",
					Topic:   domain.OutboxTopicThreadCreated,
					Payload: json.RawMessage(`{"thread_id":"thread-1"`),
				},
				{
					ID:      "event-good",
					Topic:   domain.OutboxTopicThreadReplyCreated,
					Payload: buildPayload(threadID, replyMessageID, pageID, workspaceID, actorID, now.Add(2*time.Minute)),
				},
			},
		}
		threadRepo := &fakeThreadRepo{details: map[string]domain.PageCommentThreadDetail{
			threadID: buildReplyThreadDetail(),
		}}
		workspaceRepo := &fakeWorkspaceRepo{
			memberships: map[string][]domain.WorkspaceMember{
				workspaceID: {
					{ID: "member-owner", WorkspaceID: workspaceID, UserID: ownerID, Role: domain.RoleOwner, CreatedAt: now},
					{ID: "member-actor", WorkspaceID: workspaceID, UserID: actorID, Role: domain.RoleEditor, CreatedAt: now},
				},
			},
		}
		notifications := &commentProjectorNotificationRepoStub{}
		projector := NewCommentNotificationProjector(outbox, threadRepo, buildPageRepo(workspaceID), workspaceRepo, NewThreadNotificationRecipientResolver(workspaceRepo), notifications)

		result, err := projector.ProcessBatch(context.Background(), "worker-3a", 10, time.Minute, now)
		if err != nil {
			t.Fatalf("ProcessBatch() error = %v", err)
		}
		if result.Claimed != 2 || result.DeadLettered != 1 || result.Processed != 1 || result.Retried != 0 || result.Skipped != 0 {
			t.Fatalf("expected mixed batch result, got %+v", result)
		}
		if len(outbox.deadLettered) != 1 || len(outbox.processedIDs) != 1 {
			t.Fatalf("expected one dead-letter and one processed event, got dead=%+v processed=%+v", outbox.deadLettered, outbox.processedIDs)
		}
		if len(notifications.inserted) != 1 || notifications.inserted[0].UserID != ownerID {
			t.Fatalf("expected successful event to still create one owner notification, got %+v", notifications.inserted)
		}
	})

	t.Run("invalid mention shape is dead lettered", func(t *testing.T) {
		invalidPayload, err := json.Marshal(map[string]any{
			"thread_id":        threadID,
			"message_id":       replyMessageID,
			"page_id":          pageID,
			"workspace_id":     workspaceID,
			"actor_id":         actorID,
			"occurred_at":      now.Add(2 * time.Minute).Format(time.RFC3339),
			"mention_user_ids": nil,
		})
		if err != nil {
			t.Fatalf("marshal invalid mention payload: %v", err)
		}
		outbox := &invitationProjectorOutboxRepoStub{
			claimed: []domain.OutboxEvent{{
				ID:      "event-bad-mention",
				Topic:   domain.OutboxTopicThreadReplyCreated,
				Payload: invalidPayload,
			}},
		}
		threadRepo := &fakeThreadRepo{details: map[string]domain.PageCommentThreadDetail{
			threadID: buildReplyThreadDetail(),
		}}
		workspaceRepo := &fakeWorkspaceRepo{
			memberships: map[string][]domain.WorkspaceMember{
				workspaceID: {
					{ID: "member-owner", WorkspaceID: workspaceID, UserID: ownerID, Role: domain.RoleOwner, CreatedAt: now},
					{ID: "member-actor", WorkspaceID: workspaceID, UserID: actorID, Role: domain.RoleEditor, CreatedAt: now},
				},
			},
		}
		notifications := &commentProjectorNotificationRepoStub{}
		projector := NewCommentNotificationProjector(outbox, threadRepo, buildPageRepo(workspaceID), workspaceRepo, NewThreadNotificationRecipientResolver(workspaceRepo), notifications)
		result, err := projector.ProcessBatch(context.Background(), "worker-3b", 10, time.Minute, now)
		if err != nil {
			t.Fatalf("ProcessBatch() error = %v", err)
		}
		if result.DeadLettered != 1 || len(outbox.deadLettered) != 1 {
			t.Fatalf("expected dead letter for invalid mention shape, got result=%+v dead=%+v", result, outbox.deadLettered)
		}
		if len(notifications.inserted) != 0 {
			t.Fatalf("expected malformed mention payload to block all projection, got %+v", notifications.inserted)
		}
	})

	t.Run("unsupported topic is dead lettered", func(t *testing.T) {
		outbox := &invitationProjectorOutboxRepoStub{
			claimed: []domain.OutboxEvent{{
				ID:      "event-unsupported",
				Topic:   domain.OutboxTopicMentionCreated,
				Payload: buildPayload(threadID, createdMessageID, pageID, workspaceID, actorID, now),
			}},
		}
		projector := NewCommentNotificationProjector(outbox, &fakeThreadRepo{details: map[string]domain.PageCommentThreadDetail{}}, buildPageRepo(workspaceID), &fakeWorkspaceRepo{memberships: map[string][]domain.WorkspaceMember{}}, NewThreadNotificationRecipientResolver(&fakeWorkspaceRepo{memberships: map[string][]domain.WorkspaceMember{}}), &commentProjectorNotificationRepoStub{})
		result, err := projector.ProcessBatch(context.Background(), "worker-4", 10, time.Minute, now)
		if err != nil {
			t.Fatalf("ProcessBatch() error = %v", err)
		}
		if result.DeadLettered != 1 || len(outbox.deadLettered) != 1 {
			t.Fatalf("expected dead letter for unsupported topic, got result=%+v dead=%+v", result, outbox.deadLettered)
		}
	})

	t.Run("missing thread is dead lettered", func(t *testing.T) {
		outbox := &invitationProjectorOutboxRepoStub{
			claimed: []domain.OutboxEvent{{
				ID:      "event-missing",
				Topic:   domain.OutboxTopicThreadReplyCreated,
				Payload: buildPayload("missing-thread", replyMessageID, pageID, workspaceID, actorID, now.Add(time.Minute)),
			}},
		}
		projector := NewCommentNotificationProjector(outbox, &fakeThreadRepo{details: map[string]domain.PageCommentThreadDetail{}}, buildPageRepo(workspaceID), &fakeWorkspaceRepo{memberships: map[string][]domain.WorkspaceMember{}}, NewThreadNotificationRecipientResolver(&fakeWorkspaceRepo{memberships: map[string][]domain.WorkspaceMember{}}), &commentProjectorNotificationRepoStub{})
		result, err := projector.ProcessBatch(context.Background(), "worker-5", 10, time.Minute, now)
		if err != nil {
			t.Fatalf("ProcessBatch() error = %v", err)
		}
		if result.DeadLettered != 1 || len(outbox.deadLettered) != 1 {
			t.Fatalf("expected dead letter for missing thread, got result=%+v dead=%+v", result, outbox.deadLettered)
		}
	})

	t.Run("transient notification write failure retries", func(t *testing.T) {
		outbox := &invitationProjectorOutboxRepoStub{
			claimed: []domain.OutboxEvent{{
				ID:      "event-retry",
				Topic:   domain.OutboxTopicThreadReplyCreated,
				Payload: buildPayload(threadID, replyMessageID, pageID, workspaceID, actorID, now.Add(2*time.Minute)),
			}},
		}
		threadRepo := &fakeThreadRepo{details: map[string]domain.PageCommentThreadDetail{
			threadID: buildReplyThreadDetail(),
		}}
		workspaceRepo := &fakeWorkspaceRepo{
			memberships: map[string][]domain.WorkspaceMember{
				workspaceID: {
					{ID: "member-owner", WorkspaceID: workspaceID, UserID: ownerID, Role: domain.RoleOwner, CreatedAt: now},
					{ID: "member-actor", WorkspaceID: workspaceID, UserID: actorID, Role: domain.RoleEditor, CreatedAt: now},
				},
			},
		}
		projector := NewCommentNotificationProjector(outbox, threadRepo, buildPageRepo(workspaceID), workspaceRepo, NewThreadNotificationRecipientResolver(workspaceRepo), &commentProjectorNotificationRepoStub{err: errors.New("write failed")})
		result, err := projector.ProcessBatch(context.Background(), "worker-6", 10, time.Minute, now)
		if err != nil {
			t.Fatalf("ProcessBatch() error = %v", err)
		}
		if result.Retried != 1 || len(outbox.retried) != 1 {
			t.Fatalf("expected retry on transient notification write error, got result=%+v retried=%+v", result, outbox.retried)
		}
	})

	t.Run("transient thread read failure retries", func(t *testing.T) {
		outbox := &invitationProjectorOutboxRepoStub{
			claimed: []domain.OutboxEvent{{
				ID:      "event-retry-thread-read",
				Topic:   domain.OutboxTopicThreadReplyCreated,
				Payload: buildPayload(threadID, replyMessageID, pageID, workspaceID, actorID, now.Add(time.Minute)),
			}},
		}
		projector := NewCommentNotificationProjector(outbox, &commentProjectorThreadRepoStub{err: errors.New("db down")}, buildPageRepo(workspaceID), &fakeWorkspaceRepo{memberships: map[string][]domain.WorkspaceMember{}}, &commentProjectorResolverStub{}, &commentProjectorNotificationRepoStub{})
		result, err := projector.ProcessBatch(context.Background(), "worker-6b", 10, time.Minute, now)
		if err != nil {
			t.Fatalf("ProcessBatch() error = %v", err)
		}
		if result.Retried != 1 || len(outbox.retried) != 1 {
			t.Fatalf("expected retry on transient thread read error, got result=%+v retried=%+v", result, outbox.retried)
		}
	})

	t.Run("duplicate replay does not duplicate rows", func(t *testing.T) {
		outbox := &invitationProjectorOutboxRepoStub{
			claimed: []domain.OutboxEvent{{
				ID:      "event-idempotent",
				Topic:   domain.OutboxTopicThreadReplyCreated,
				Payload: buildPayload(threadID, replyMessageID, pageID, workspaceID, actorID, now.Add(2*time.Minute), mentionID),
			}},
		}
		threadRepo := &fakeThreadRepo{details: map[string]domain.PageCommentThreadDetail{
			threadID: buildReplyThreadDetail(),
		}}
		workspaceRepo := &fakeWorkspaceRepo{
			memberships: map[string][]domain.WorkspaceMember{
				workspaceID: {
					{ID: "member-owner", WorkspaceID: workspaceID, UserID: ownerID, Role: domain.RoleOwner, CreatedAt: now},
					{ID: "member-mention", WorkspaceID: workspaceID, UserID: mentionID, Role: domain.RoleViewer, CreatedAt: now},
					{ID: "member-actor", WorkspaceID: workspaceID, UserID: actorID, Role: domain.RoleEditor, CreatedAt: now},
				},
			},
		}
		notifications := &commentProjectorNotificationRepoStub{}
		projector := NewCommentNotificationProjector(outbox, threadRepo, buildPageRepo(workspaceID), workspaceRepo, NewThreadNotificationRecipientResolver(workspaceRepo), notifications)

		first, err := projector.ProcessBatch(context.Background(), "worker-7", 10, time.Minute, now)
		if err != nil {
			t.Fatalf("first ProcessBatch() error = %v", err)
		}
		second, err := projector.ProcessBatch(context.Background(), "worker-7", 10, time.Minute, now)
		if err != nil {
			t.Fatalf("second ProcessBatch() error = %v", err)
		}
		if first.Processed != 1 || second.Processed != 1 {
			t.Fatalf("expected processed event on both runs, got first=%+v second=%+v", first, second)
		}
		if len(notifications.inserted) != 3 {
			t.Fatalf("expected idempotent insert to keep two comment rows and one mention row, got %+v", notifications.inserted)
		}
		if len(outbox.processedIDs) != 2 {
			t.Fatalf("expected processed twice, got %+v", outbox.processedIDs)
		}
	})

	t.Run("workspace or timestamp drift is dead lettered", func(t *testing.T) {
		outbox := &invitationProjectorOutboxRepoStub{
			claimed: []domain.OutboxEvent{
				{
					ID:      "event-bad-workspace",
					Topic:   domain.OutboxTopicThreadReplyCreated,
					Payload: buildPayload(threadID, replyMessageID, pageID, "workspace-wrong", actorID, now.Add(2*time.Minute)),
				},
				{
					ID:      "event-bad-time",
					Topic:   domain.OutboxTopicThreadReplyCreated,
					Payload: buildPayload(threadID, replyMessageID, pageID, workspaceID, actorID, now.Add(3*time.Minute)),
				},
			},
		}
		threadRepo := &fakeThreadRepo{details: map[string]domain.PageCommentThreadDetail{
			threadID: buildReplyThreadDetail(),
		}}
		projector := NewCommentNotificationProjector(outbox, threadRepo, buildPageRepo(workspaceID), &fakeWorkspaceRepo{memberships: map[string][]domain.WorkspaceMember{}}, &commentProjectorResolverStub{}, &commentProjectorNotificationRepoStub{})

		result, err := projector.ProcessBatch(context.Background(), "worker-8", 10, time.Minute, now)
		if err != nil {
			t.Fatalf("ProcessBatch() error = %v", err)
		}
		if result.DeadLettered != 2 || len(outbox.deadLettered) != 2 {
			t.Fatalf("expected workspace/timestamp drift to dead-letter, got result=%+v dead=%+v", result, outbox.deadLettered)
		}
	})
}
