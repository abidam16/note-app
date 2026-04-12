package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"note-app/internal/domain"

	"github.com/google/uuid"
)

func TestOutboxRepositoryIntegration(t *testing.T) {
	pool := integrationPool(t)
	repo := NewOutboxRepository(pool)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	newEvent := func(topic domain.OutboxTopic, aggregateType domain.OutboxAggregateType, aggregateID, key string, availableAt time.Time) domain.OutboxEvent {
		return domain.OutboxEvent{
			ID:             uuid.NewString(),
			Topic:          topic,
			AggregateType:  aggregateType,
			AggregateID:    aggregateID,
			IdempotencyKey: key,
			Payload:        json.RawMessage(`{"ok":true}`),
			MaxAttempts:    5,
			AvailableAt:    availableAt,
			CreatedAt:      now,
			UpdatedAt:      now,
		}
	}

	t.Run("create sets pending defaults", func(t *testing.T) {
		event := newEvent(domain.OutboxTopicInvitationCreated, domain.OutboxAggregateTypeInvitation, uuid.NewString(), "outbox-create-1", time.Time{})
		created, err := repo.Create(ctx, event)
		if err != nil {
			t.Fatalf("create outbox event: %v", err)
		}
		if created.Status != domain.OutboxStatusPending || created.AttemptCount != 0 {
			t.Fatalf("unexpected created outbox state: %+v", created)
		}
		if created.AvailableAt.IsZero() || !created.AvailableAt.Equal(created.CreatedAt) {
			t.Fatalf("expected available_at to default to created_at, got %+v", created)
		}
	})

	t.Run("create rejects duplicate idempotency key", func(t *testing.T) {
		key := "outbox-dup-1"
		if _, err := repo.Create(ctx, newEvent(domain.OutboxTopicInvitationCreated, domain.OutboxAggregateTypeInvitation, uuid.NewString(), key, now)); err != nil {
			t.Fatalf("seed outbox event: %v", err)
		}
		if _, err := repo.Create(ctx, newEvent(domain.OutboxTopicInvitationUpdated, domain.OutboxAggregateTypeInvitation, uuid.NewString(), key, now)); !errors.Is(err, domain.ErrConflict) {
			t.Fatalf("expected idempotency conflict, got %v", err)
		}
	})

	t.Run("create rejects non-object payload", func(t *testing.T) {
		event := newEvent(domain.OutboxTopicInvitationCreated, domain.OutboxAggregateTypeInvitation, uuid.NewString(), "outbox-bad-payload", now)
		event.Payload = json.RawMessage(`[]`)
		if _, err := repo.Create(ctx, event); !errors.Is(err, domain.ErrValidation) {
			t.Fatalf("expected payload validation error, got %v", err)
		}
	})

	t.Run("create many inserts multiple rows", func(t *testing.T) {
		err := repo.CreateMany(ctx, []domain.OutboxEvent{
			newEvent(domain.OutboxTopicThreadCreated, domain.OutboxAggregateTypeThread, uuid.NewString(), "outbox-batch-1", now),
			newEvent(domain.OutboxTopicThreadReplyCreated, domain.OutboxAggregateTypeThreadMessage, uuid.NewString(), "outbox-batch-2", now.Add(time.Second)),
		})
		if err != nil {
			t.Fatalf("create many outbox events: %v", err)
		}
	})

	t.Run("claim pending returns oldest ready first and updates lease", func(t *testing.T) {
		first := newEvent(domain.OutboxTopicInvitationCreated, domain.OutboxAggregateTypeInvitation, uuid.NewString(), "outbox-claim-1", now.Add(-2*time.Minute))
		second := newEvent(domain.OutboxTopicInvitationUpdated, domain.OutboxAggregateTypeInvitation, uuid.NewString(), "outbox-claim-2", now.Add(-time.Minute))
		future := newEvent(domain.OutboxTopicInvitationCancelled, domain.OutboxAggregateTypeInvitation, uuid.NewString(), "outbox-claim-future", now.Add(time.Hour))
		if err := repo.CreateMany(ctx, []domain.OutboxEvent{first, second, future}); err != nil {
			t.Fatalf("seed claim events: %v", err)
		}

		claimed, err := repo.ClaimPending(ctx, "worker-1", 2, 5*time.Minute, now)
		if err != nil {
			t.Fatalf("claim pending: %v", err)
		}
		if len(claimed) != 2 || claimed[0].IdempotencyKey != first.IdempotencyKey || claimed[1].IdempotencyKey != second.IdempotencyKey {
			t.Fatalf("unexpected claim ordering: %+v", claimed)
		}
		for _, event := range claimed {
			if event.Status != domain.OutboxStatusProcessing || event.ClaimedBy == nil || *event.ClaimedBy != "worker-1" || event.ClaimedAt == nil || event.LeaseExpiresAt == nil || event.AttemptCount != 1 {
				t.Fatalf("unexpected claimed event metadata: %+v", event)
			}
		}
	})

	t.Run("claim validates inputs", func(t *testing.T) {
		if _, err := repo.ClaimPending(ctx, "", 1, time.Minute, now); !errors.Is(err, domain.ErrValidation) {
			t.Fatalf("expected worker validation error, got %v", err)
		}
		if _, err := repo.ClaimPending(ctx, "worker-1", 0, time.Minute, now); !errors.Is(err, domain.ErrValidation) {
			t.Fatalf("expected limit validation error, got %v", err)
		}
		if _, err := repo.ClaimPending(ctx, "worker-1", 1, 0, now); !errors.Is(err, domain.ErrValidation) {
			t.Fatalf("expected lease validation error, got %v", err)
		}
	})

	t.Run("mark processed transitions processing row", func(t *testing.T) {
		event := newEvent(domain.OutboxTopicInvitationAccepted, domain.OutboxAggregateTypeInvitation, uuid.NewString(), "outbox-processed-1", now.Add(-time.Minute))
		if _, err := repo.Create(ctx, event); err != nil {
			t.Fatalf("seed event: %v", err)
		}
		claimed, err := repo.ClaimPending(ctx, "worker-processed", 1, 5*time.Minute, now)
		if err != nil {
			t.Fatalf("claim processed seed: %v", err)
		}
		processedAt := now.Add(2 * time.Minute)
		processed, err := repo.MarkProcessed(ctx, claimed[0].ID, "worker-processed", processedAt)
		if err != nil {
			t.Fatalf("mark processed: %v", err)
		}
		if processed.Status != domain.OutboxStatusProcessed || processed.ProcessedAt == nil || !processed.ProcessedAt.Equal(processedAt) || processed.LeaseExpiresAt != nil {
			t.Fatalf("unexpected processed event: %+v", processed)
		}
	})

	t.Run("mark retry requeues row", func(t *testing.T) {
		event := newEvent(domain.OutboxTopicInvitationRejected, domain.OutboxAggregateTypeInvitation, uuid.NewString(), "outbox-retry-1", now.Add(-time.Minute))
		if _, err := repo.Create(ctx, event); err != nil {
			t.Fatalf("seed retry event: %v", err)
		}
		claimed, err := repo.ClaimPending(ctx, "worker-retry", 1, 5*time.Minute, now)
		if err != nil {
			t.Fatalf("claim retry seed: %v", err)
		}
		nextAvailableAt := now.Add(10 * time.Minute)
		retried, err := repo.MarkRetry(ctx, claimed[0].ID, "worker-retry", "temporary error", nextAvailableAt, now.Add(time.Minute))
		if err != nil {
			t.Fatalf("mark retry: %v", err)
		}
		if retried.Status != domain.OutboxStatusPending || retried.ClaimedBy != nil || retried.ClaimedAt != nil || retried.LeaseExpiresAt != nil || retried.LastError == nil || *retried.LastError != "temporary error" || !retried.AvailableAt.Equal(nextAvailableAt) {
			t.Fatalf("unexpected retried event: %+v", retried)
		}
	})

	t.Run("mark retry dead letters exhausted row", func(t *testing.T) {
		event := newEvent(domain.OutboxTopicMentionCreated, domain.OutboxAggregateTypeThreadMessage, uuid.NewString(), "outbox-dead-on-retry", now.Add(-time.Minute))
		event.MaxAttempts = 1
		if _, err := repo.Create(ctx, event); err != nil {
			t.Fatalf("seed dead-letter event: %v", err)
		}
		claimed, err := repo.ClaimPending(ctx, "worker-exhausted", 1, 5*time.Minute, now)
		if err != nil {
			t.Fatalf("claim exhausted seed: %v", err)
		}
		dead, err := repo.MarkRetry(ctx, claimed[0].ID, "worker-exhausted", "poison event", now.Add(time.Hour), now.Add(time.Minute))
		if err != nil {
			t.Fatalf("mark retry exhausted: %v", err)
		}
		if dead.Status != domain.OutboxStatusDeadLetter || dead.DeadLetteredAt == nil || dead.LastError == nil || *dead.LastError != "poison event" {
			t.Fatalf("unexpected dead-lettered event: %+v", dead)
		}
	})

	t.Run("mark dead letter transitions processing row", func(t *testing.T) {
		event := newEvent(domain.OutboxTopicThreadCreated, domain.OutboxAggregateTypeThread, uuid.NewString(), "outbox-dead-1", now.Add(-time.Minute))
		if _, err := repo.Create(ctx, event); err != nil {
			t.Fatalf("seed dead letter event: %v", err)
		}
		claimed, err := repo.ClaimPending(ctx, "worker-dead", 1, 5*time.Minute, now)
		if err != nil {
			t.Fatalf("claim dead seed: %v", err)
		}
		dead, err := repo.MarkDeadLetter(ctx, claimed[0].ID, "worker-dead", "fatal error", now.Add(time.Minute))
		if err != nil {
			t.Fatalf("mark dead letter: %v", err)
		}
		if dead.Status != domain.OutboxStatusDeadLetter || dead.DeadLetteredAt == nil || dead.LastError == nil || dead.LeaseExpiresAt != nil {
			t.Fatalf("unexpected dead-letter state: %+v", dead)
		}
	})

	t.Run("claim can reclaim stale processing rows", func(t *testing.T) {
		event := newEvent(domain.OutboxTopicThreadReplyCreated, domain.OutboxAggregateTypeThreadMessage, uuid.NewString(), "outbox-stale-1", now.Add(-time.Minute))
		if _, err := repo.Create(ctx, event); err != nil {
			t.Fatalf("seed stale event: %v", err)
		}
		firstClaim, err := repo.ClaimPending(ctx, "worker-stale-1", 1, time.Minute, now)
		if err != nil {
			t.Fatalf("first stale claim: %v", err)
		}
		reclaimed, err := repo.ClaimPending(ctx, "worker-stale-2", 1, time.Minute, now.Add(2*time.Minute))
		if err != nil {
			t.Fatalf("reclaim stale event: %v", err)
		}
		if len(reclaimed) != 1 || reclaimed[0].ID != firstClaim[0].ID || reclaimed[0].ClaimedBy == nil || *reclaimed[0].ClaimedBy != "worker-stale-2" || reclaimed[0].AttemptCount != 2 {
			t.Fatalf("unexpected reclaimed event: %+v", reclaimed)
		}
	})

	t.Run("wrong worker and missing row checks return conflict or not found", func(t *testing.T) {
		event := newEvent(domain.OutboxTopicInvitationCreated, domain.OutboxAggregateTypeInvitation, uuid.NewString(), "outbox-worker-check-1", now.Add(-time.Minute))
		if _, err := repo.Create(ctx, event); err != nil {
			t.Fatalf("seed worker check event: %v", err)
		}
		claimed, err := repo.ClaimPending(ctx, "worker-check", 1, 5*time.Minute, now)
		if err != nil {
			t.Fatalf("claim worker check event: %v", err)
		}
		if _, err := repo.MarkProcessed(ctx, claimed[0].ID, "other-worker", now.Add(time.Minute)); !errors.Is(err, domain.ErrConflict) {
			t.Fatalf("expected wrong worker conflict, got %v", err)
		}
		if _, err := repo.MarkDeadLetter(ctx, uuid.NewString(), "worker-check", "missing", now.Add(time.Minute)); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("expected missing dead-letter row not found, got %v", err)
		}
	})

	t.Run("claim pending by topics returns only invitation topics in ready order", func(t *testing.T) {
		first := newEvent(domain.OutboxTopicInvitationCreated, domain.OutboxAggregateTypeInvitation, uuid.NewString(), "outbox-topic-claim-1", now.Add(-2*time.Minute))
		second := newEvent(domain.OutboxTopicInvitationUpdated, domain.OutboxAggregateTypeInvitation, uuid.NewString(), "outbox-topic-claim-2", now.Add(-time.Minute))
		thread := newEvent(domain.OutboxTopicThreadCreated, domain.OutboxAggregateTypeThread, uuid.NewString(), "outbox-topic-thread", now.Add(-3*time.Minute))
		if err := repo.CreateMany(ctx, []domain.OutboxEvent{first, second, thread}); err != nil {
			t.Fatalf("seed topic-claim events: %v", err)
		}

		claimed, err := repo.ClaimPendingByTopics(ctx, "worker-topic", []domain.OutboxTopic{
			domain.OutboxTopicInvitationCreated,
			domain.OutboxTopicInvitationUpdated,
		}, 2, 5*time.Minute, now)
		if err != nil {
			t.Fatalf("claim by topics: %v", err)
		}
		if len(claimed) != 2 || claimed[0].IdempotencyKey != first.IdempotencyKey || claimed[1].IdempotencyKey != second.IdempotencyKey {
			t.Fatalf("unexpected topic claim results: %+v", claimed)
		}
	})

	t.Run("claim pending by topics can reclaim stale processing rows", func(t *testing.T) {
		event := newEvent(domain.OutboxTopicInvitationCancelled, domain.OutboxAggregateTypeInvitation, uuid.NewString(), "outbox-topic-stale-1", now.Add(-time.Minute))
		if _, err := repo.Create(ctx, event); err != nil {
			t.Fatalf("seed topic stale event: %v", err)
		}
		firstClaim, err := repo.ClaimPendingByTopics(ctx, "worker-topic-stale-1", []domain.OutboxTopic{domain.OutboxTopicInvitationCancelled}, 1, time.Minute, now)
		if err != nil {
			t.Fatalf("first topic stale claim: %v", err)
		}
		reclaimed, err := repo.ClaimPendingByTopics(ctx, "worker-topic-stale-2", []domain.OutboxTopic{domain.OutboxTopicInvitationCancelled}, 1, time.Minute, now.Add(2*time.Minute))
		if err != nil {
			t.Fatalf("reclaim topic stale event: %v", err)
		}
		if len(reclaimed) != 1 || reclaimed[0].ID != firstClaim[0].ID || reclaimed[0].ClaimedBy == nil || *reclaimed[0].ClaimedBy != "worker-topic-stale-2" {
			t.Fatalf("unexpected topic stale reclaim: %+v", reclaimed)
		}
	})

	t.Run("claim pending by topics validates topic list", func(t *testing.T) {
		if _, err := repo.ClaimPendingByTopics(ctx, "worker-topic-empty", nil, 1, time.Minute, now); !errors.Is(err, domain.ErrValidation) {
			t.Fatalf("expected empty topic validation error, got %v", err)
		}
		if _, err := repo.ClaimPendingByTopics(ctx, "worker-topic-empty", []domain.OutboxTopic{""}, 1, time.Minute, now); !errors.Is(err, domain.ErrValidation) {
			t.Fatalf("expected blank topic validation error, got %v", err)
		}
	})
}
