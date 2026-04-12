package domain

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

type OutboxTopic string
type OutboxAggregateType string
type OutboxStatus string

const (
	OutboxTopicInvitationCreated   OutboxTopic = "invitation_created"
	OutboxTopicInvitationUpdated   OutboxTopic = "invitation_updated"
	OutboxTopicInvitationAccepted  OutboxTopic = "invitation_accepted"
	OutboxTopicInvitationRejected  OutboxTopic = "invitation_rejected"
	OutboxTopicInvitationCancelled OutboxTopic = "invitation_cancelled"
	OutboxTopicThreadCreated       OutboxTopic = "thread_created"
	OutboxTopicThreadReplyCreated  OutboxTopic = "thread_reply_created"
	OutboxTopicMentionCreated      OutboxTopic = "mention_created"
)

const (
	OutboxAggregateTypeInvitation    OutboxAggregateType = "invitation"
	OutboxAggregateTypeThread        OutboxAggregateType = "thread"
	OutboxAggregateTypeThreadMessage OutboxAggregateType = "thread_message"
)

const (
	OutboxStatusPending    OutboxStatus = "pending"
	OutboxStatusProcessing OutboxStatus = "processing"
	OutboxStatusProcessed  OutboxStatus = "processed"
	OutboxStatusDeadLetter OutboxStatus = "dead_letter"
)

type OutboxEvent struct {
	ID             string              `json:"id"`
	Topic          OutboxTopic         `json:"topic"`
	AggregateType  OutboxAggregateType `json:"aggregate_type"`
	AggregateID    string              `json:"aggregate_id"`
	IdempotencyKey string              `json:"idempotency_key"`
	Payload        json.RawMessage     `json:"payload"`
	Status         OutboxStatus        `json:"status"`
	AttemptCount   int                 `json:"attempt_count"`
	MaxAttempts    int                 `json:"max_attempts"`
	AvailableAt    time.Time           `json:"available_at"`
	ClaimedBy      *string             `json:"claimed_by,omitempty"`
	ClaimedAt      *time.Time          `json:"claimed_at,omitempty"`
	LeaseExpiresAt *time.Time          `json:"lease_expires_at,omitempty"`
	LastError      *string             `json:"last_error,omitempty"`
	ProcessedAt    *time.Time          `json:"processed_at,omitempty"`
	DeadLetteredAt *time.Time          `json:"dead_lettered_at,omitempty"`
	CreatedAt      time.Time           `json:"created_at"`
	UpdatedAt      time.Time           `json:"updated_at"`
}

type ThreadCreatedOutboxPayload struct {
	ThreadID       string    `json:"thread_id"`
	MessageID      string    `json:"message_id"`
	PageID         string    `json:"page_id"`
	WorkspaceID    string    `json:"workspace_id"`
	ActorID        string    `json:"actor_id"`
	OccurredAt     time.Time `json:"occurred_at"`
	MentionUserIDs []string  `json:"mention_user_ids"`
}

type ThreadReplyCreatedOutboxPayload struct {
	ThreadID       string    `json:"thread_id"`
	MessageID      string    `json:"message_id"`
	PageID         string    `json:"page_id"`
	WorkspaceID    string    `json:"workspace_id"`
	ActorID        string    `json:"actor_id"`
	OccurredAt     time.Time `json:"occurred_at"`
	MentionUserIDs []string  `json:"mention_user_ids"`
}

func NewThreadCreatedOutboxEvent(thread PageCommentThread, message PageCommentThreadMessage, workspaceID string, mentionUserIDs []string) (OutboxEvent, error) {
	normalizedMentionUserIDs, err := normalizeThreadCreatedMentionUserIDs(mentionUserIDs)
	if err != nil {
		return OutboxEvent{}, err
	}
	payload := ThreadCreatedOutboxPayload{
		ThreadID:       thread.ID,
		MessageID:      message.ID,
		PageID:         thread.PageID,
		WorkspaceID:    workspaceID,
		ActorID:        thread.CreatedBy,
		OccurredAt:     thread.CreatedAt.UTC(),
		MentionUserIDs: normalizedMentionUserIDs,
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return OutboxEvent{}, fmt.Errorf("marshal thread created outbox payload: %w", err)
	}

	return OutboxEvent{
		ID:             uuid.NewString(),
		Topic:          OutboxTopicThreadCreated,
		AggregateType:  OutboxAggregateTypeThread,
		AggregateID:    thread.ID,
		IdempotencyKey: "thread_created:" + thread.ID,
		Payload:        payloadJSON,
		AvailableAt:    thread.CreatedAt,
		CreatedAt:      thread.CreatedAt,
		UpdatedAt:      thread.CreatedAt,
	}, nil
}

func ValidateThreadCreatedOutboxEvent(event OutboxEvent, thread PageCommentThread, message PageCommentThreadMessage, workspaceID string, mentionUserIDs []string) error {
	expectedOccurredAt := thread.CreatedAt.UTC()
	normalizedMentionUserIDs, err := normalizeThreadCreatedMentionUserIDs(mentionUserIDs)
	if err != nil {
		return err
	}
	if event.Topic != OutboxTopicThreadCreated {
		return fmt.Errorf("%w: outbox topic must be %q", ErrValidation, OutboxTopicThreadCreated)
	}
	if event.AggregateType != OutboxAggregateTypeThread {
		return fmt.Errorf("%w: outbox aggregate_type must be %q", ErrValidation, OutboxAggregateTypeThread)
	}
	if event.AggregateID != thread.ID {
		return fmt.Errorf("%w: outbox aggregate_id must match thread id", ErrValidation)
	}
	expectedKey := "thread_created:" + thread.ID
	if event.IdempotencyKey != expectedKey {
		return fmt.Errorf("%w: outbox idempotency_key must be %q", ErrValidation, expectedKey)
	}

	var payload ThreadCreatedOutboxPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("%w: decode thread created outbox payload: %v", ErrValidation, err)
	}
	if payload.ThreadID != thread.ID {
		return fmt.Errorf("%w: payload thread_id must match thread id", ErrValidation)
	}
	if message.ThreadID != thread.ID {
		return fmt.Errorf("%w: starter message thread_id must match thread id", ErrValidation)
	}
	if payload.MessageID != message.ID {
		return fmt.Errorf("%w: payload message_id must match starter message id", ErrValidation)
	}
	if payload.PageID != thread.PageID {
		return fmt.Errorf("%w: payload page_id must match thread page id", ErrValidation)
	}
	if payload.WorkspaceID != workspaceID {
		return fmt.Errorf("%w: payload workspace_id must match thread workspace id", ErrValidation)
	}
	if payload.ActorID != thread.CreatedBy {
		return fmt.Errorf("%w: payload actor_id must match thread creator id", ErrValidation)
	}
	if !payload.OccurredAt.Equal(expectedOccurredAt) {
		return fmt.Errorf("%w: payload occurred_at must match thread created_at", ErrValidation)
	}
	if len(payload.MentionUserIDs) != len(normalizedMentionUserIDs) {
		return fmt.Errorf("%w: payload mention_user_ids must match normalized mention ids", ErrValidation)
	}
	for idx := range normalizedMentionUserIDs {
		if payload.MentionUserIDs[idx] != normalizedMentionUserIDs[idx] {
			return fmt.Errorf("%w: payload mention_user_ids must match normalized mention ids", ErrValidation)
		}
	}
	if !event.AvailableAt.UTC().Equal(expectedOccurredAt) {
		return fmt.Errorf("%w: outbox available_at must match thread created_at", ErrValidation)
	}
	if !event.CreatedAt.UTC().Equal(expectedOccurredAt) {
		return fmt.Errorf("%w: outbox created_at must match thread created_at", ErrValidation)
	}
	if !event.UpdatedAt.UTC().Equal(expectedOccurredAt) {
		return fmt.Errorf("%w: outbox updated_at must match thread created_at", ErrValidation)
	}

	return nil
}

func normalizeThreadCreatedMentionUserIDs(mentionUserIDs []string) ([]string, error) {
	if len(mentionUserIDs) == 0 {
		return []string{}, nil
	}

	normalized := make([]string, 0, len(mentionUserIDs))
	seen := make(map[string]struct{}, len(mentionUserIDs))
	for _, mentionUserID := range mentionUserIDs {
		trimmed := strings.TrimSpace(mentionUserID)
		if trimmed == "" {
			return nil, fmt.Errorf("%w: outbox mention_user_ids must not contain blank values", ErrValidation)
		}
		if _, exists := seen[trimmed]; exists {
			continue
		}
		seen[trimmed] = struct{}{}
		normalized = append(normalized, trimmed)
	}

	return normalized, nil
}

func NewThreadReplyCreatedOutboxEvent(thread PageCommentThread, message PageCommentThreadMessage, workspaceID string, mentionUserIDs []string) (OutboxEvent, error) {
	occurredAt := message.CreatedAt.UTC()
	normalizedMentionUserIDs, err := normalizeThreadCreatedMentionUserIDs(mentionUserIDs)
	if err != nil {
		return OutboxEvent{}, err
	}
	payload := ThreadReplyCreatedOutboxPayload{
		ThreadID:       thread.ID,
		MessageID:      message.ID,
		PageID:         thread.PageID,
		WorkspaceID:    workspaceID,
		ActorID:        message.CreatedBy,
		OccurredAt:     occurredAt,
		MentionUserIDs: normalizedMentionUserIDs,
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return OutboxEvent{}, fmt.Errorf("marshal thread reply created outbox payload: %w", err)
	}

	return OutboxEvent{
		ID:             uuid.NewString(),
		Topic:          OutboxTopicThreadReplyCreated,
		AggregateType:  OutboxAggregateTypeThreadMessage,
		AggregateID:    message.ID,
		IdempotencyKey: "thread_reply_created:" + message.ID,
		Payload:        payloadJSON,
		AvailableAt:    occurredAt,
		CreatedAt:      occurredAt,
		UpdatedAt:      occurredAt,
	}, nil
}

func ValidateThreadReplyCreatedOutboxEvent(event OutboxEvent, thread PageCommentThread, message PageCommentThreadMessage, workspaceID string, mentionUserIDs []string) error {
	expectedOccurredAt := message.CreatedAt.UTC()
	normalizedMentionUserIDs, err := normalizeThreadCreatedMentionUserIDs(mentionUserIDs)
	if err != nil {
		return err
	}
	if event.Topic != OutboxTopicThreadReplyCreated {
		return fmt.Errorf("%w: outbox topic must be %q", ErrValidation, OutboxTopicThreadReplyCreated)
	}
	if event.AggregateType != OutboxAggregateTypeThreadMessage {
		return fmt.Errorf("%w: outbox aggregate_type must be %q", ErrValidation, OutboxAggregateTypeThreadMessage)
	}
	if event.AggregateID != message.ID {
		return fmt.Errorf("%w: outbox aggregate_id must match reply message id", ErrValidation)
	}
	expectedKey := "thread_reply_created:" + message.ID
	if event.IdempotencyKey != expectedKey {
		return fmt.Errorf("%w: outbox idempotency_key must be %q", ErrValidation, expectedKey)
	}

	var payload ThreadReplyCreatedOutboxPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("%w: decode thread reply created outbox payload: %v", ErrValidation, err)
	}
	if payload.ThreadID != thread.ID {
		return fmt.Errorf("%w: payload thread_id must match reply thread id", ErrValidation)
	}
	if message.ThreadID != thread.ID {
		return fmt.Errorf("%w: reply message thread_id must match reply thread id", ErrValidation)
	}
	if payload.MessageID != message.ID {
		return fmt.Errorf("%w: payload message_id must match reply message id", ErrValidation)
	}
	if payload.PageID != thread.PageID {
		return fmt.Errorf("%w: payload page_id must match reply page id", ErrValidation)
	}
	if payload.WorkspaceID != workspaceID {
		return fmt.Errorf("%w: payload workspace_id must match reply workspace id", ErrValidation)
	}
	if payload.ActorID != message.CreatedBy {
		return fmt.Errorf("%w: payload actor_id must match reply author id", ErrValidation)
	}
	if !payload.OccurredAt.Equal(expectedOccurredAt) {
		return fmt.Errorf("%w: payload occurred_at must match reply created_at", ErrValidation)
	}
	if len(payload.MentionUserIDs) != len(normalizedMentionUserIDs) {
		return fmt.Errorf("%w: payload mention_user_ids must match normalized mention ids", ErrValidation)
	}
	for idx := range normalizedMentionUserIDs {
		if payload.MentionUserIDs[idx] != normalizedMentionUserIDs[idx] {
			return fmt.Errorf("%w: payload mention_user_ids must match normalized mention ids", ErrValidation)
		}
	}
	if !event.AvailableAt.UTC().Equal(expectedOccurredAt) {
		return fmt.Errorf("%w: outbox available_at must match reply created_at", ErrValidation)
	}
	if !event.CreatedAt.UTC().Equal(expectedOccurredAt) {
		return fmt.Errorf("%w: outbox created_at must match reply created_at", ErrValidation)
	}
	if !event.UpdatedAt.UTC().Equal(expectedOccurredAt) {
		return fmt.Errorf("%w: outbox updated_at must match reply created_at", ErrValidation)
	}

	return nil
}
