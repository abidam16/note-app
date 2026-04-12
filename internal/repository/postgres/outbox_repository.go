package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"note-app/internal/domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type OutboxRepository struct {
	db *pgxpool.Pool
}

const outboxInsertQuery = `
	INSERT INTO outbox_events (
		id, topic, aggregate_type, aggregate_id, idempotency_key, payload,
		status, attempt_count, max_attempts, available_at,
		claimed_by, claimed_at, lease_expires_at, last_error,
		processed_at, dead_lettered_at, created_at, updated_at
	)
	VALUES (
		$1, $2, $3, $4, $5, $6,
		$7, $8, $9, $10,
		$11, $12, $13, $14,
		$15, $16, $17, $18
	)
	RETURNING
		id, topic, aggregate_type, aggregate_id, idempotency_key, payload,
		status, attempt_count, max_attempts, available_at,
		claimed_by, claimed_at, lease_expires_at, last_error,
		processed_at, dead_lettered_at, created_at, updated_at
`

func NewOutboxRepository(db *pgxpool.Pool) OutboxRepository {
	return OutboxRepository{db: db}
}

func (r OutboxRepository) Create(ctx context.Context, event domain.OutboxEvent) (domain.OutboxEvent, error) {
	created, err := insertOutboxEvent(ctx, r.db, event)
	if err != nil {
		if isUniqueViolation(err) {
			return domain.OutboxEvent{}, domain.ErrConflict
		}
		return domain.OutboxEvent{}, fmt.Errorf("insert outbox event: %w", err)
	}
	return created, nil
}

func (r OutboxRepository) CreateMany(ctx context.Context, events []domain.OutboxEvent) error {
	if len(events) == 0 {
		return nil
	}

	normalized := make([]domain.OutboxEvent, len(events))
	for i := range events {
		event, err := normalizeOutboxForCreate(events[i])
		if err != nil {
			return err
		}
		normalized[i] = event
	}

	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin outbox batch create: %w", err)
	}
	defer tx.Rollback(ctx)

	for i := range normalized {
		if _, err := insertOutboxEvent(ctx, tx, normalized[i]); err != nil {
			if isUniqueViolation(err) {
				return domain.ErrConflict
			}
			return fmt.Errorf("insert outbox events batch: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit outbox batch create: %w", err)
	}

	return nil
}

func (r OutboxRepository) ClaimPending(ctx context.Context, workerID string, limit int, leaseDuration time.Duration, now time.Time) ([]domain.OutboxEvent, error) {
	return r.claimPending(ctx, workerID, nil, limit, leaseDuration, now)
}

func (r OutboxRepository) ClaimPendingByTopics(ctx context.Context, workerID string, topics []domain.OutboxTopic, limit int, leaseDuration time.Duration, now time.Time) ([]domain.OutboxEvent, error) {
	if len(topics) == 0 {
		return nil, fmt.Errorf("%w: topics are required", domain.ErrValidation)
	}
	topicValues := make([]string, 0, len(topics))
	for _, topic := range topics {
		if strings.TrimSpace(string(topic)) == "" {
			return nil, fmt.Errorf("%w: topics are required", domain.ErrValidation)
		}
		topicValues = append(topicValues, string(topic))
	}
	return r.claimPending(ctx, workerID, topicValues, limit, leaseDuration, now)
}

func (r OutboxRepository) claimPending(ctx context.Context, workerID string, topics []string, limit int, leaseDuration time.Duration, now time.Time) ([]domain.OutboxEvent, error) {
	if strings.TrimSpace(workerID) == "" {
		return nil, fmt.Errorf("%w: worker_id is required", domain.ErrValidation)
	}
	if limit <= 0 {
		return nil, fmt.Errorf("%w: limit must be greater than zero", domain.ErrValidation)
	}
	if leaseDuration <= 0 {
		return nil, fmt.Errorf("%w: lease duration must be greater than zero", domain.ErrValidation)
	}

	leaseExpiresAt := now.UTC().Add(leaseDuration)
	query := `
		WITH claimable AS (
			SELECT id
			FROM outbox_events
			WHERE (
				status = 'pending'
				AND available_at <= $1
				%s
			) OR (
				status = 'processing'
				AND lease_expires_at <= $1
				%s
			)
			ORDER BY available_at ASC, created_at ASC, id ASC
			FOR UPDATE SKIP LOCKED
			LIMIT $2
		)
		UPDATE outbox_events oe
		SET status = 'processing',
		    attempt_count = oe.attempt_count + 1,
		    claimed_by = $3,
		    claimed_at = $1,
		    lease_expires_at = $4,
		    updated_at = $1
		FROM claimable
		WHERE oe.id = claimable.id
		RETURNING
			oe.id, oe.topic, oe.aggregate_type, oe.aggregate_id, oe.idempotency_key, oe.payload,
			oe.status, oe.attempt_count, oe.max_attempts, oe.available_at,
			oe.claimed_by, oe.claimed_at, oe.lease_expires_at, oe.last_error,
			oe.processed_at, oe.dead_lettered_at, oe.created_at, oe.updated_at
	`
	args := []any{now.UTC(), limit, workerID, leaseExpiresAt}
	pendingTopicFilter := ""
	processingTopicFilter := ""
	if len(topics) > 0 {
		pendingTopicFilter = " AND topic = ANY($5)"
		processingTopicFilter = " AND topic = ANY($5)"
		args = append(args, topics)
	}

	rows, err := r.db.Query(ctx, fmt.Sprintf(query, pendingTopicFilter, processingTopicFilter), args...)
	if err != nil {
		return nil, fmt.Errorf("claim outbox events: %w", err)
	}
	defer rows.Close()

	events := make([]domain.OutboxEvent, 0, limit)
	for rows.Next() {
		var event domain.OutboxEvent
		if err := scanOutboxEvent(rows, &event); err != nil {
			return nil, fmt.Errorf("scan claimed outbox event: %w", err)
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate claimed outbox events: %w", err)
	}

	return events, nil
}

func (r OutboxRepository) MarkProcessed(ctx context.Context, id, workerID string, processedAt time.Time) (domain.OutboxEvent, error) {
	query := `
		UPDATE outbox_events
		SET status = 'processed',
		    lease_expires_at = NULL,
		    processed_at = $3,
		    updated_at = $3
		WHERE id = $1
		  AND status = 'processing'
		  AND claimed_by = $2
		RETURNING
			id, topic, aggregate_type, aggregate_id, idempotency_key, payload,
			status, attempt_count, max_attempts, available_at,
			claimed_by, claimed_at, lease_expires_at, last_error,
			processed_at, dead_lettered_at, created_at, updated_at
	`

	var event domain.OutboxEvent
	if err := scanOutboxEvent(r.db.QueryRow(ctx, query, id, workerID, processedAt.UTC()), &event); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.OutboxEvent{}, r.classifyOutboxMutationMiss(ctx, id, workerID)
		}
		return domain.OutboxEvent{}, fmt.Errorf("mark outbox event processed: %w", err)
	}
	return event, nil
}

func (r OutboxRepository) MarkRetry(ctx context.Context, id, workerID, lastError string, nextAvailableAt, failedAt time.Time) (domain.OutboxEvent, error) {
	now := failedAt.UTC()
	query := `
		UPDATE outbox_events
		SET status = CASE WHEN attempt_count >= max_attempts THEN 'dead_letter' ELSE 'pending' END,
		    available_at = CASE WHEN attempt_count >= max_attempts THEN available_at ELSE $3::timestamptz END,
		    claimed_by = CASE WHEN attempt_count >= max_attempts THEN claimed_by ELSE NULL END,
		    claimed_at = CASE WHEN attempt_count >= max_attempts THEN claimed_at ELSE NULL END,
		    lease_expires_at = NULL,
		    last_error = $4,
		    dead_lettered_at = CASE WHEN attempt_count >= max_attempts THEN $5::timestamptz ELSE NULL END,
		    processed_at = NULL,
		    updated_at = $5::timestamptz
		WHERE id = $1
		  AND status = 'processing'
		  AND claimed_by = $2
		RETURNING
			id, topic, aggregate_type, aggregate_id, idempotency_key, payload,
			status, attempt_count, max_attempts, available_at,
			claimed_by, claimed_at, lease_expires_at, last_error,
			processed_at, dead_lettered_at, created_at, updated_at
	`

	var event domain.OutboxEvent
	if err := scanOutboxEvent(r.db.QueryRow(ctx, query, id, workerID, nextAvailableAt.UTC(), lastError, now), &event); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.OutboxEvent{}, r.classifyOutboxMutationMiss(ctx, id, workerID)
		}
		return domain.OutboxEvent{}, fmt.Errorf("mark outbox event retry: %w", err)
	}
	return event, nil
}

func (r OutboxRepository) MarkDeadLetter(ctx context.Context, id, workerID, lastError string, deadLetteredAt time.Time) (domain.OutboxEvent, error) {
	query := `
		UPDATE outbox_events
		SET status = 'dead_letter',
		    lease_expires_at = NULL,
		    last_error = $3,
		    dead_lettered_at = $4,
		    updated_at = $4
		WHERE id = $1
		  AND status = 'processing'
		  AND claimed_by = $2
		RETURNING
			id, topic, aggregate_type, aggregate_id, idempotency_key, payload,
			status, attempt_count, max_attempts, available_at,
			claimed_by, claimed_at, lease_expires_at, last_error,
			processed_at, dead_lettered_at, created_at, updated_at
	`

	var event domain.OutboxEvent
	if err := scanOutboxEvent(r.db.QueryRow(ctx, query, id, workerID, lastError, deadLetteredAt.UTC()), &event); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.OutboxEvent{}, r.classifyOutboxMutationMiss(ctx, id, workerID)
		}
		return domain.OutboxEvent{}, fmt.Errorf("mark outbox event dead letter: %w", err)
	}
	return event, nil
}

func (r OutboxRepository) classifyOutboxMutationMiss(ctx context.Context, id, workerID string) error {
	var existingWorker *string
	err := r.db.QueryRow(ctx, `SELECT claimed_by FROM outbox_events WHERE id = $1`, id).Scan(&existingWorker)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("classify outbox mutation miss: %w", err)
	}
	if existingWorker == nil || *existingWorker != workerID {
		return domain.ErrConflict
	}
	return domain.ErrConflict
}

type outboxScanner interface {
	Scan(dest ...any) error
}

func scanOutboxEvent(row outboxScanner, event *domain.OutboxEvent) error {
	var payload []byte
	if err := row.Scan(
		&event.ID,
		&event.Topic,
		&event.AggregateType,
		&event.AggregateID,
		&event.IdempotencyKey,
		&payload,
		&event.Status,
		&event.AttemptCount,
		&event.MaxAttempts,
		&event.AvailableAt,
		&event.ClaimedBy,
		&event.ClaimedAt,
		&event.LeaseExpiresAt,
		&event.LastError,
		&event.ProcessedAt,
		&event.DeadLetteredAt,
		&event.CreatedAt,
		&event.UpdatedAt,
	); err != nil {
		return err
	}
	event.Payload = json.RawMessage(payload)
	return nil
}

func insertOutboxEvent(ctx context.Context, q dbtx, event domain.OutboxEvent) (domain.OutboxEvent, error) {
	event, err := normalizeOutboxForCreate(event)
	if err != nil {
		return domain.OutboxEvent{}, err
	}

	var created domain.OutboxEvent
	if err := scanOutboxEvent(q.QueryRow(
		ctx,
		outboxInsertQuery,
		event.ID,
		event.Topic,
		event.AggregateType,
		event.AggregateID,
		event.IdempotencyKey,
		event.Payload,
		event.Status,
		event.AttemptCount,
		event.MaxAttempts,
		event.AvailableAt,
		event.ClaimedBy,
		event.ClaimedAt,
		event.LeaseExpiresAt,
		event.LastError,
		event.ProcessedAt,
		event.DeadLetteredAt,
		event.CreatedAt,
		event.UpdatedAt,
	), &created); err != nil {
		return domain.OutboxEvent{}, err
	}

	return created, nil
}

func normalizeOutboxForCreate(event domain.OutboxEvent) (domain.OutboxEvent, error) {
	if strings.TrimSpace(event.ID) == "" {
		return domain.OutboxEvent{}, fmt.Errorf("%w: id is required", domain.ErrValidation)
	}
	if strings.TrimSpace(string(event.Topic)) == "" {
		return domain.OutboxEvent{}, fmt.Errorf("%w: topic is required", domain.ErrValidation)
	}
	if strings.TrimSpace(string(event.AggregateType)) == "" {
		return domain.OutboxEvent{}, fmt.Errorf("%w: aggregate_type is required", domain.ErrValidation)
	}
	if strings.TrimSpace(event.AggregateID) == "" {
		return domain.OutboxEvent{}, fmt.Errorf("%w: aggregate_id is required", domain.ErrValidation)
	}
	if strings.TrimSpace(event.IdempotencyKey) == "" {
		return domain.OutboxEvent{}, fmt.Errorf("%w: idempotency_key is required", domain.ErrValidation)
	}
	if !isJSONObject(event.Payload) {
		return domain.OutboxEvent{}, fmt.Errorf("%w: payload must be a JSON object", domain.ErrValidation)
	}
	if event.MaxAttempts <= 0 {
		event.MaxAttempts = 25
	}
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now().UTC()
	} else {
		event.CreatedAt = event.CreatedAt.UTC()
	}
	if event.UpdatedAt.IsZero() {
		event.UpdatedAt = event.CreatedAt
	} else {
		event.UpdatedAt = event.UpdatedAt.UTC()
	}
	if event.AvailableAt.IsZero() {
		event.AvailableAt = event.CreatedAt
	} else {
		event.AvailableAt = event.AvailableAt.UTC()
	}
	event.Status = domain.OutboxStatusPending
	event.AttemptCount = 0
	event.ClaimedBy = nil
	event.ClaimedAt = nil
	event.LeaseExpiresAt = nil
	event.LastError = nil
	event.ProcessedAt = nil
	event.DeadLetteredAt = nil
	return event, nil
}

func isJSONObject(payload json.RawMessage) bool {
	if len(payload) == 0 {
		return false
	}
	var decoded any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return false
	}
	_, ok := decoded.(map[string]any)
	return ok
}
