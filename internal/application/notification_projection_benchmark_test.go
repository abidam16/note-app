package application

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"note-app/internal/domain"

	"github.com/google/uuid"
)

var (
	benchmarkInvitationReplayRows int
	benchmarkThreadHistorySize    int
	benchmarkBatchProjectionRows   int
)

func BenchmarkInvitationProjectorReplay(b *testing.B) {
	b.ReportAllocs()

	now := time.Date(2026, 4, 7, 3, 0, 0, 0, time.UTC)
	invitationID := uuid.NewString()
	user := domain.User{ID: uuid.NewString(), Email: "bench-invitee@example.com", FullName: "Invitee"}
	outbox := &invitationProjectorOutboxRepoStub{
		claimed: []domain.OutboxEvent{{
			ID:      uuid.NewString(),
			Topic:   domain.OutboxTopicInvitationUpdated,
			Payload: mustBenchmarkJSON(map[string]any{"invitation_id": invitationID, "workspace_id": "workspace-1", "actor_id": "owner-1", "email": "bench-invitee@example.com", "role": "editor", "status": "pending", "version": 2, "occurred_at": now.Format(time.RFC3339)}),
		}},
	}
	notifs := &invitationProjectorNotificationRepoStub{
		nextByKey: map[string]domain.Notification{
			invitationLiveKey(user.ID, invitationID): {
				ID:        invitationID,
				UserID:    user.ID,
				EventID:   invitationID,
				CreatedAt: now,
				IsRead:    false,
			},
		},
	}
	projector := NewInvitationNotificationProjector(outbox, notifs, &fakeUserRepo{
		byEmail: map[string]domain.User{"bench-invitee@example.com": user},
		byID:    map[string]domain.User{user.ID: user},
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result, err := projector.ProcessBatch(context.Background(), "bench-worker", 1, time.Minute, now)
		if err != nil {
			b.Fatalf("ProcessBatch() error = %v", err)
		}
		benchmarkInvitationReplayRows += result.Processed + result.Skipped + result.DeadLettered + result.Retried
	}
	if benchmarkInvitationReplayRows == 0 || len(notifs.upserted) == 0 {
		b.Fatal("expected invitation projector benchmark to process at least one row")
	}
}

func BenchmarkThreadNotificationHistoryBuilder(b *testing.B) {
	b.ReportAllocs()

	input := benchmarkThreadNotificationHistoryInput()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		got, err := BuildThreadNotificationHistory(input)
		if err != nil {
			b.Fatalf("BuildThreadNotificationHistory() error = %v", err)
		}
		benchmarkThreadHistorySize += len(got)
	}
	if benchmarkThreadHistorySize == 0 {
		b.Fatal("expected non-empty thread history benchmark result")
	}
}

func BenchmarkNotificationBatchProjection(b *testing.B) {
	b.ReportAllocs()

	input := benchmarkThreadNotificationHistoryInput()
	history, err := BuildThreadNotificationHistory(input)
	if err != nil {
		b.Fatalf("BuildThreadNotificationHistory() error = %v", err)
	}
	commentNotifications, mentionNotifications := benchmarkNotificationBatch(history, input)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sink := &projectionNotificationSinkStub{}
		commentCount, err := sink.insert(commentNotifications)
		if err != nil {
			b.Fatalf("insert comment notifications: %v", err)
		}
		mentionCount, err := sink.insert(mentionNotifications)
		if err != nil {
			b.Fatalf("insert mention notifications: %v", err)
		}
		benchmarkBatchProjectionRows += commentCount + mentionCount
		if len(sink.rows) == 0 {
			b.Fatal("expected non-empty notification batch projection result")
		}
	}
	if benchmarkBatchProjectionRows == 0 {
		b.Fatal("expected batch projection benchmark to insert rows")
	}
}

func benchmarkThreadNotificationHistoryInput() ThreadNotificationHistoryInput {
	now := time.Date(2026, 4, 7, 7, 0, 0, 0, time.UTC)
	thread := domain.PageCommentThread{
		ID:        "thread-bench",
		PageID:    "page-bench",
		CreatedBy: "creator",
		CreatedAt: now,
	}
	messages := make([]domain.PageCommentThreadMessage, 0, 24)
	explicitMentions := make(map[string][]string, 24)
	members := []string{"creator", "actor-1", "actor-2", "member-1", "member-2", "member-3", "member-4"}
	for i := 0; i < 24; i++ {
		actor := members[i%3+1]
		messageID := uuid.NewString()
		messages = append(messages, domain.PageCommentThreadMessage{
			ID:        messageID,
			ThreadID:  thread.ID,
			Body:      "message body",
			CreatedBy: actor,
			CreatedAt: now.Add(time.Duration(i) * time.Minute),
		})
		explicitMentions[messageID] = []string{"member-1", "member-2", actor, "member-3", "member-4", "member-1"}
	}
	return ThreadNotificationHistoryInput{
		Thread:                      thread,
		Messages:                    messages,
		ExplicitMentionsByMessageID: explicitMentions,
		WorkspaceMemberIDs:          members,
	}
}

func benchmarkNotificationBatch(history []ThreadNotificationHistoryEntry, input ThreadNotificationHistoryInput) ([]domain.Notification, []domain.Notification) {
	commentNotifications := make([]domain.Notification, 0, len(history)*4)
	mentionNotifications := make([]domain.Notification, 0, len(history)*4)
	workspaceID := "workspace-bench"
	for idx, entry := range history {
		message := input.Messages[idx]
		payload := commentNotificationPayload{
			ThreadID:       input.Thread.ID,
			MessageID:      message.ID,
			PageID:         input.Thread.PageID,
			WorkspaceID:    workspaceID,
			ActorID:        message.CreatedBy,
			OccurredAt:     message.CreatedAt,
			MentionUserIDs: input.ExplicitMentionsByMessageID[message.ID],
		}
		for _, recipientID := range entry.CommentRecipients {
			commentNotifications = append(commentNotifications, mapCommentNotification(domain.OutboxTopicThreadReplyCreated, payload, recipientID))
		}
		for _, recipientID := range entry.MentionRecipients {
			mentionNotifications = append(mentionNotifications, mapMentionNotification(domain.OutboxTopicThreadReplyCreated, payload, recipientID))
		}
	}
	return commentNotifications, mentionNotifications
}

func mustBenchmarkJSON(value any) json.RawMessage {
	encoded, _ := json.Marshal(value)
	return encoded
}

