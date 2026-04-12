package application

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"note-app/internal/domain"
)

type invitationProjectorOutboxRepoStub struct {
	claimed           []domain.OutboxEvent
	claimErr          error
	processedIDs      []string
	deadLettered      map[string]string
	retried           map[string]string
	claimedWorkerID   string
	claimedTopics     []domain.OutboxTopic
	claimedLimit      int
	claimedLease      time.Duration
	markProcessedErr  error
	markDeadLetterErr error
	markRetryErr      error
}

func (s *invitationProjectorOutboxRepoStub) ClaimPendingByTopics(_ context.Context, workerID string, topics []domain.OutboxTopic, limit int, leaseDuration time.Duration, _ time.Time) ([]domain.OutboxEvent, error) {
	s.claimedWorkerID = workerID
	s.claimedTopics = append([]domain.OutboxTopic(nil), topics...)
	s.claimedLimit = limit
	s.claimedLease = leaseDuration
	if s.claimErr != nil {
		return nil, s.claimErr
	}
	return append([]domain.OutboxEvent(nil), s.claimed...), nil
}

func (s *invitationProjectorOutboxRepoStub) MarkProcessed(_ context.Context, id, _ string, _ time.Time) (domain.OutboxEvent, error) {
	if s.markProcessedErr != nil {
		return domain.OutboxEvent{}, s.markProcessedErr
	}
	s.processedIDs = append(s.processedIDs, id)
	return domain.OutboxEvent{ID: id, Status: domain.OutboxStatusProcessed}, nil
}

func (s *invitationProjectorOutboxRepoStub) MarkRetry(_ context.Context, id, _ string, lastError string, _ time.Time, _ time.Time) (domain.OutboxEvent, error) {
	if s.markRetryErr != nil {
		return domain.OutboxEvent{}, s.markRetryErr
	}
	if s.retried == nil {
		s.retried = map[string]string{}
	}
	s.retried[id] = lastError
	return domain.OutboxEvent{ID: id, Status: domain.OutboxStatusPending}, nil
}

func (s *invitationProjectorOutboxRepoStub) MarkDeadLetter(_ context.Context, id, _ string, lastError string, _ time.Time) (domain.OutboxEvent, error) {
	if s.markDeadLetterErr != nil {
		return domain.OutboxEvent{}, s.markDeadLetterErr
	}
	if s.deadLettered == nil {
		s.deadLettered = map[string]string{}
	}
	s.deadLettered[id] = lastError
	return domain.OutboxEvent{ID: id, Status: domain.OutboxStatusDeadLetter}, nil
}

type invitationProjectorNotificationRepoStub struct {
	upserted  []domain.Notification
	nextByKey map[string]domain.Notification
	err       error
}

func (s *invitationProjectorNotificationRepoStub) UpsertInvitationLive(_ context.Context, notification domain.Notification) (domain.Notification, error) {
	if s.err != nil {
		return domain.Notification{}, s.err
	}
	if s.nextByKey == nil {
		s.nextByKey = map[string]domain.Notification{}
	}
	key := notification.UserID + ":" + notification.EventID
	if existing, ok := s.nextByKey[key]; ok {
		notification.ID = existing.ID
		notification.CreatedAt = existing.CreatedAt
		notification.ReadAt = existing.ReadAt
		notification.IsRead = existing.IsRead
	}
	if notification.ID == "" {
		notification.ID = "notif-" + notification.EventID
	}
	s.nextByKey[key] = notification
	s.upserted = append(s.upserted, notification)
	return notification, nil
}

func TestInvitationNotificationProjector(t *testing.T) {
	now := time.Date(2026, 4, 7, 3, 0, 0, 0, time.UTC)
	newEvent := func(id string, topic domain.OutboxTopic, payload string, attempts int) domain.OutboxEvent {
		return domain.OutboxEvent{
			ID:           id,
			Topic:        topic,
			Payload:      json.RawMessage(payload),
			AttemptCount: attempts,
		}
	}

	t.Run("registered invitation events project one live row and preserve read state", func(t *testing.T) {
		outbox := &invitationProjectorOutboxRepoStub{
			claimed: []domain.OutboxEvent{
				newEvent("event-created", domain.OutboxTopicInvitationCreated, `{"invitation_id":"inv-1","workspace_id":"workspace-1","actor_id":"owner-1","email":"invitee@example.com","role":"viewer","status":"pending","version":1,"occurred_at":"2026-04-07T03:00:00Z"}`, 1),
				newEvent("event-updated", domain.OutboxTopicInvitationUpdated, `{"invitation_id":"inv-1","workspace_id":"workspace-1","actor_id":"owner-1","email":"invitee@example.com","role":"editor","status":"pending","version":2,"occurred_at":"2026-04-07T03:05:00Z"}`, 1),
				newEvent("event-accepted", domain.OutboxTopicInvitationAccepted, `{"invitation_id":"inv-1","workspace_id":"workspace-1","actor_id":"owner-1","email":"invitee@example.com","role":"editor","status":"accepted","version":3,"occurred_at":"2026-04-07T03:10:00Z"}`, 1),
			},
		}
		readAt := now.Add(4 * time.Minute)
		notifs := &invitationProjectorNotificationRepoStub{
			nextByKey: map[string]domain.Notification{
				"user-1:inv-1": {
					ID:        "notif-inv-1",
					UserID:    "user-1",
					EventID:   "inv-1",
					CreatedAt: now,
					ReadAt:    &readAt,
					IsRead:    true,
				},
			},
		}
		users := &fakeUserRepo{byEmail: map[string]domain.User{
			"invitee@example.com": {ID: "user-1", Email: "invitee@example.com", FullName: "Invitee"},
		}, byID: map[string]domain.User{}}

		projector := NewInvitationNotificationProjector(outbox, notifs, users)
		result, err := projector.ProcessBatch(context.Background(), "worker-1", 10, 5*time.Minute, now)
		if err != nil {
			t.Fatalf("ProcessBatch() error = %v", err)
		}
		if result.Claimed != 3 || result.Processed != 3 || result.Retried != 0 || result.DeadLettered != 0 || result.Skipped != 0 {
			t.Fatalf("unexpected batch result: %+v", result)
		}
		if len(notifs.upserted) != 3 {
			t.Fatalf("expected three upsert attempts, got %d", len(notifs.upserted))
		}
		last := notifs.upserted[len(notifs.upserted)-1]
		if last.ID != "notif-inv-1" || !last.CreatedAt.Equal(now) || !last.IsRead || last.ReadAt == nil || !last.ReadAt.Equal(readAt) {
			t.Fatalf("expected live notification identity/read state preserved, got %+v", last)
		}
		if last.Actionable || last.ActionKind != nil || last.Title != "Invitation accepted" {
			t.Fatalf("expected terminal accepted mapping, got %+v", last)
		}
		if len(outbox.processedIDs) != 3 {
			t.Fatalf("expected all events marked processed, got %+v", outbox.processedIDs)
		}
		if len(outbox.claimedTopics) != 5 {
			t.Fatalf("expected invitation topics filter, got %+v", outbox.claimedTopics)
		}
	})

	t.Run("unregistered invitee is skipped without notification write", func(t *testing.T) {
		outbox := &invitationProjectorOutboxRepoStub{
			claimed: []domain.OutboxEvent{
				newEvent("event-created", domain.OutboxTopicInvitationCreated, `{"invitation_id":"inv-2","workspace_id":"workspace-1","actor_id":"owner-1","email":"new@example.com","role":"viewer","status":"pending","version":1,"occurred_at":"2026-04-07T03:00:00Z"}`, 1),
			},
		}
		projector := NewInvitationNotificationProjector(outbox, &invitationProjectorNotificationRepoStub{}, &fakeUserRepo{byEmail: map[string]domain.User{}, byID: map[string]domain.User{}})
		result, err := projector.ProcessBatch(context.Background(), "worker-2", 10, time.Minute, now)
		if err != nil {
			t.Fatalf("ProcessBatch() error = %v", err)
		}
		if result.Skipped != 1 || result.Processed != 0 || len(outbox.processedIDs) != 1 {
			t.Fatalf("expected skipped registered-user miss path, got result=%+v processed=%+v", result, outbox.processedIDs)
		}
	})

	t.Run("malformed payload is dead lettered", func(t *testing.T) {
		outbox := &invitationProjectorOutboxRepoStub{
			claimed: []domain.OutboxEvent{
				newEvent("event-bad", domain.OutboxTopicInvitationCreated, `{"invitation_id":"","workspace_id":"workspace-1"}`, 1),
			},
		}
		projector := NewInvitationNotificationProjector(outbox, &invitationProjectorNotificationRepoStub{}, &fakeUserRepo{byEmail: map[string]domain.User{}, byID: map[string]domain.User{}})
		result, err := projector.ProcessBatch(context.Background(), "worker-3", 10, time.Minute, now)
		if err != nil {
			t.Fatalf("ProcessBatch() error = %v", err)
		}
		if result.DeadLettered != 1 || len(outbox.deadLettered) != 1 {
			t.Fatalf("expected dead letter for malformed payload, got result=%+v dead=%+v", result, outbox.deadLettered)
		}
	})

	t.Run("unsupported topic is dead lettered", func(t *testing.T) {
		outbox := &invitationProjectorOutboxRepoStub{
			claimed: []domain.OutboxEvent{
				newEvent("event-thread", domain.OutboxTopicThreadCreated, `{"thread_id":"thread-1"}`, 1),
			},
		}
		projector := NewInvitationNotificationProjector(outbox, &invitationProjectorNotificationRepoStub{}, &fakeUserRepo{byEmail: map[string]domain.User{}, byID: map[string]domain.User{}})
		result, err := projector.ProcessBatch(context.Background(), "worker-4", 10, time.Minute, now)
		if err != nil {
			t.Fatalf("ProcessBatch() error = %v", err)
		}
		if result.DeadLettered != 1 || len(outbox.deadLettered) != 1 {
			t.Fatalf("expected dead letter for unsupported topic, got result=%+v dead=%+v", result, outbox.deadLettered)
		}
	})

	t.Run("transient user lookup failure triggers retry", func(t *testing.T) {
		outbox := &invitationProjectorOutboxRepoStub{
			claimed: []domain.OutboxEvent{
				newEvent("event-retry-user", domain.OutboxTopicInvitationCreated, `{"invitation_id":"inv-3","workspace_id":"workspace-1","actor_id":"owner-1","email":"invitee@example.com","role":"viewer","status":"pending","version":1,"occurred_at":"2026-04-07T03:00:00Z"}`, 2),
			},
		}
		users := &fakeUserRepo{getByEmailErr: errors.New("db down"), byEmail: map[string]domain.User{}, byID: map[string]domain.User{}}
		projector := NewInvitationNotificationProjector(outbox, &invitationProjectorNotificationRepoStub{}, users)
		result, err := projector.ProcessBatch(context.Background(), "worker-5", 10, time.Minute, now)
		if err != nil {
			t.Fatalf("ProcessBatch() error = %v", err)
		}
		if result.Retried != 1 || len(outbox.retried) != 1 {
			t.Fatalf("expected retry on transient user error, got result=%+v retried=%+v", result, outbox.retried)
		}
	})

	t.Run("transient notification write failure triggers retry", func(t *testing.T) {
		outbox := &invitationProjectorOutboxRepoStub{
			claimed: []domain.OutboxEvent{
				newEvent("event-retry-notif", domain.OutboxTopicInvitationCreated, `{"invitation_id":"inv-4","workspace_id":"workspace-1","actor_id":"owner-1","email":"invitee@example.com","role":"viewer","status":"pending","version":1,"occurred_at":"2026-04-07T03:00:00Z"}`, 1),
			},
		}
		users := &fakeUserRepo{byEmail: map[string]domain.User{"invitee@example.com": {ID: "user-1", Email: "invitee@example.com"}}, byID: map[string]domain.User{}}
		projector := NewInvitationNotificationProjector(outbox, &invitationProjectorNotificationRepoStub{err: errors.New("write failed")}, users)
		result, err := projector.ProcessBatch(context.Background(), "worker-6", 10, time.Minute, now)
		if err != nil {
			t.Fatalf("ProcessBatch() error = %v", err)
		}
		if result.Retried != 1 || len(outbox.retried) != 1 {
			t.Fatalf("expected retry on transient notification error, got result=%+v retried=%+v", result, outbox.retried)
		}
	})

	t.Run("no invitation events returns empty batch", func(t *testing.T) {
		outbox := &invitationProjectorOutboxRepoStub{}
		projector := NewInvitationNotificationProjector(outbox, &invitationProjectorNotificationRepoStub{}, &fakeUserRepo{byEmail: map[string]domain.User{}, byID: map[string]domain.User{}})
		result, err := projector.ProcessBatch(context.Background(), "worker-7", 10, time.Minute, now)
		if err != nil {
			t.Fatalf("ProcessBatch() error = %v", err)
		}
		if result != (InvitationNotificationProjectorResult{}) {
			t.Fatalf("expected zero result on empty claim, got %+v", result)
		}
	})
}
