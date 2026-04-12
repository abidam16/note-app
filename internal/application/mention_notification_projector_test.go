package application

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"note-app/internal/domain"
)

type mentionProjectorMembershipStub struct {
	members []domain.WorkspaceMember
	err     error
	calls   int
}

func (s *mentionProjectorMembershipStub) GetMembershipByUserID(context.Context, string, string) (domain.WorkspaceMember, error) {
	return domain.WorkspaceMember{}, nil
}

func (s *mentionProjectorMembershipStub) ListMembers(_ context.Context, _ string) ([]domain.WorkspaceMember, error) {
	s.calls++
	if s.err != nil {
		return nil, s.err
	}
	return append([]domain.WorkspaceMember(nil), s.members...), nil
}

func TestMentionNotificationProjector(t *testing.T) {
	now := time.Date(2026, 4, 7, 6, 0, 0, 0, time.UTC)
	workspaceID := "workspace-1"
	pageID := "page-1"
	threadID := "thread-1"
	messageID := "message-1"
	actorID := "user-actor"
	memberA := "user-a"
	memberB := "user-b"
	nonMember := "user-non-member"

	buildPayload := func(topic domain.OutboxTopic, mentionIDs ...string) (json.RawMessage, commentNotificationPayload) {
		payload := map[string]any{
			"thread_id":    threadID,
			"message_id":   messageID,
			"page_id":      pageID,
			"workspace_id": workspaceID,
			"actor_id":     actorID,
			"occurred_at":  now.Format(time.RFC3339),
		}
		if mentionIDs != nil {
			payload["mention_user_ids"] = mentionIDs
		}
		encoded, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("marshal payload: %v", err)
		}
		return encoded, commentNotificationPayload{
			ThreadID:       threadID,
			MessageID:      messageID,
			PageID:         pageID,
			WorkspaceID:    workspaceID,
			ActorID:        actorID,
			OccurredAt:     now,
			MentionUserIDs: append([]string(nil), mentionIDs...),
		}
	}

	t.Run("projects explicit current members and filters actor duplicates blanks and non-members", func(t *testing.T) {
		_, payload := buildPayload(domain.OutboxTopicThreadCreated, " "+memberA+" ", memberB, memberA, "", nonMember, actorID)
		notifications := &commentProjectorNotificationRepoStub{}
		projector := NewMentionNotificationProjector(&mentionProjectorMembershipStub{
			members: []domain.WorkspaceMember{
				{UserID: actorID},
				{UserID: memberA},
				{UserID: memberB},
			},
		}, notifications)

		hasRecipients, err := projector.Project(context.Background(), domain.OutboxTopicThreadCreated, payload)
		if err != nil {
			t.Fatalf("Project() error = %v", err)
		}
		if !hasRecipients {
			t.Fatal("expected eligible mention recipients")
		}
		if len(notifications.inserted) != 2 {
			t.Fatalf("expected two mention notifications, got %+v", notifications.inserted)
		}
		if notifications.inserted[0].UserID != memberA || notifications.inserted[1].UserID != memberB {
			t.Fatalf("unexpected recipient order: %+v", notifications.inserted)
		}
		for _, notification := range notifications.inserted {
			if notification.Type != domain.NotificationTypeMention {
				t.Fatalf("expected mention notification type, got %+v", notification)
			}
			if notification.EventID != messageID || notification.ResourceType == nil || *notification.ResourceType != domain.NotificationResourceTypeThreadMsg {
				t.Fatalf("unexpected mention identity mapping: %+v", notification)
			}
			if notification.Message != "You were mentioned in a new comment thread" || notification.Title != "Mentioned in a new comment thread" || notification.Content != "You were mentioned in a new comment thread" {
				t.Fatalf("unexpected mention text mapping: %+v", notification)
			}
			if notification.Payload == nil || !json.Valid(notification.Payload) {
				t.Fatalf("expected valid mention payload JSON, got %+v", notification)
			}
			var projected map[string]any
			if err := json.Unmarshal(notification.Payload, &projected); err != nil {
				t.Fatalf("unmarshal projected payload: %v", err)
			}
			if projected["mention_source"] != "explicit" || projected["event_topic"] != string(domain.OutboxTopicThreadCreated) {
				t.Fatalf("unexpected projected payload: %+v", projected)
			}
		}
	})

	t.Run("omitted mention ids is a no-op", func(t *testing.T) {
		_, payload := buildPayload(domain.OutboxTopicThreadReplyCreated)
		projector := NewMentionNotificationProjector(&mentionProjectorMembershipStub{
			members: []domain.WorkspaceMember{{UserID: memberA}},
		}, &commentProjectorNotificationRepoStub{})

		hasRecipients, err := projector.Project(context.Background(), domain.OutboxTopicThreadReplyCreated, payload)
		if err != nil {
			t.Fatalf("Project() error = %v", err)
		}
		if hasRecipients {
			t.Fatal("expected no eligible mention recipients")
		}
	})

	t.Run("reply mapping uses reply copy", func(t *testing.T) {
		_, payload := buildPayload(domain.OutboxTopicThreadReplyCreated, memberA)
		notifications := &commentProjectorNotificationRepoStub{}
		projector := NewMentionNotificationProjector(&mentionProjectorMembershipStub{
			members: []domain.WorkspaceMember{{UserID: memberA}},
		}, notifications)

		hasRecipients, err := projector.Project(context.Background(), domain.OutboxTopicThreadReplyCreated, payload)
		if err != nil {
			t.Fatalf("Project() error = %v", err)
		}
		if !hasRecipients || len(notifications.inserted) != 1 {
			t.Fatalf("expected one mention notification, got hasRecipients=%v inserted=%+v", hasRecipients, notifications.inserted)
		}
		if notifications.inserted[0].Title != "Mentioned in a thread reply" || notifications.inserted[0].Content != "You were mentioned in a thread reply" {
			t.Fatalf("unexpected reply mention text: %+v", notifications.inserted[0])
		}
	})

	t.Run("membership lookup failure retries", func(t *testing.T) {
		_, payload := buildPayload(domain.OutboxTopicThreadCreated, memberA)
		projector := NewMentionNotificationProjector(&mentionProjectorMembershipStub{
			err: errors.New("members failed"),
		}, &commentProjectorNotificationRepoStub{})

		if _, err := projector.Project(context.Background(), domain.OutboxTopicThreadCreated, payload); err == nil || isPermanentMentionProjectorError(err) || err.Error() != "members failed" {
			t.Fatalf("Project() error = %v, want transient membership error", err)
		}
	})

	t.Run("repository failure retries", func(t *testing.T) {
		_, payload := buildPayload(domain.OutboxTopicThreadCreated, memberA)
		projector := NewMentionNotificationProjector(&mentionProjectorMembershipStub{
			members: []domain.WorkspaceMember{{UserID: memberA}},
		}, &commentProjectorNotificationRepoStub{err: errors.New("write failed")})

		if _, err := projector.Project(context.Background(), domain.OutboxTopicThreadCreated, payload); err == nil || isPermanentMentionProjectorError(err) || err.Error() != "write failed" {
			t.Fatalf("Project() error = %v, want transient repository error", err)
		}
	})

	t.Run("unsupported topic is permanent", func(t *testing.T) {
		_, payload := buildPayload(domain.OutboxTopicThreadCreated, memberA)
		projector := NewMentionNotificationProjector(&mentionProjectorMembershipStub{
			members: []domain.WorkspaceMember{{UserID: memberA}},
		}, &commentProjectorNotificationRepoStub{})

		if _, err := projector.Build(context.Background(), domain.OutboxTopic("unsupported"), payload); !isPermanentMentionProjectorError(err) {
			t.Fatalf("Build() error = %v, want permanent error", err)
		}
	})

	t.Run("missing required payload fields is permanent", func(t *testing.T) {
		projector := NewMentionNotificationProjector(&mentionProjectorMembershipStub{
			members: []domain.WorkspaceMember{{UserID: memberA}},
		}, &commentProjectorNotificationRepoStub{})

		if _, err := projector.Build(context.Background(), domain.OutboxTopicThreadCreated, commentNotificationPayload{}); !isPermanentMentionProjectorError(err) {
			t.Fatalf("Build() error = %v, want permanent error", err)
		}
	})
}
