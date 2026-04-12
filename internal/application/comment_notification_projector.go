package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"note-app/internal/domain"

	"github.com/google/uuid"
)

type commentNotificationOutboxRepository interface {
	ClaimPendingByTopics(ctx context.Context, workerID string, topics []domain.OutboxTopic, limit int, leaseDuration time.Duration, now time.Time) ([]domain.OutboxEvent, error)
	MarkProcessed(ctx context.Context, id, workerID string, processedAt time.Time) (domain.OutboxEvent, error)
	MarkRetry(ctx context.Context, id, workerID, lastError string, nextAvailableAt, failedAt time.Time) (domain.OutboxEvent, error)
	MarkDeadLetter(ctx context.Context, id, workerID, lastError string, deadLetteredAt time.Time) (domain.OutboxEvent, error)
}

type commentNotificationThreadRepository interface {
	GetThread(ctx context.Context, threadID string) (domain.PageCommentThreadDetail, error)
}

type commentNotificationPageRepository interface {
	GetByID(ctx context.Context, pageID string) (domain.Page, domain.PageDraft, error)
}

type commentNotificationRepository interface {
	CreateCommentNotifications(ctx context.Context, notifications []domain.Notification) (int, error)
	CreateMentionNotifications(ctx context.Context, notifications []domain.Notification) (int, error)
	CreateCommentAndMentionNotifications(ctx context.Context, commentNotifications, mentionNotifications []domain.Notification) (int, int, error)
}

type CommentNotificationProjector struct {
	outbox        commentNotificationOutboxRepository
	threads       commentNotificationThreadRepository
	pages         commentNotificationPageRepository
	resolver      ThreadNotificationRecipientResolver
	notifications commentNotificationRepository
	mentions      MentionNotificationProjector
}

type CommentNotificationProjectorResult struct {
	Claimed      int
	Processed    int
	Retried      int
	DeadLettered int
	Skipped      int
}

type commentNotificationPayload struct {
	ThreadID       string    `json:"thread_id"`
	MessageID      string    `json:"message_id"`
	PageID         string    `json:"page_id"`
	WorkspaceID    string    `json:"workspace_id"`
	ActorID        string    `json:"actor_id"`
	OccurredAt     time.Time `json:"occurred_at"`
	MentionUserIDs []string  `json:"mention_user_ids,omitempty"`
}

func NewCommentNotificationProjector(outbox commentNotificationOutboxRepository, threads commentNotificationThreadRepository, pages commentNotificationPageRepository, memberships WorkspaceMembershipReader, resolver ThreadNotificationRecipientResolver, notifications commentNotificationRepository) CommentNotificationProjector {
	return CommentNotificationProjector{
		outbox:        outbox,
		threads:       threads,
		pages:         pages,
		resolver:      resolver,
		notifications: notifications,
		mentions:      NewMentionNotificationProjector(memberships, notifications),
	}
}

func (p CommentNotificationProjector) ProcessBatch(ctx context.Context, workerID string, limit int, leaseDuration time.Duration, now time.Time) (CommentNotificationProjectorResult, error) {
	claimed, err := p.outbox.ClaimPendingByTopics(ctx, workerID, commentOutboxTopics(), limit, leaseDuration, now)
	if err != nil {
		return CommentNotificationProjectorResult{}, err
	}

	result := CommentNotificationProjectorResult{Claimed: len(claimed)}
	var batchErr error
	recordErr := func(err error) {
		if err != nil && batchErr == nil {
			batchErr = err
		}
	}

	for _, event := range claimed {
		permanentSkip, processErr := p.processEvent(ctx, event)
		switch {
		case processErr == nil:
			if _, err := p.outbox.MarkProcessed(ctx, event.ID, workerID, now); err != nil {
				recordErr(err)
				continue
			}
			if permanentSkip {
				result.Skipped++
			} else {
				result.Processed++
			}
		case isPermanentCommentProjectorError(processErr):
			if _, err := p.outbox.MarkDeadLetter(ctx, event.ID, workerID, processErr.Error(), now); err != nil {
				recordErr(err)
				continue
			}
			result.DeadLettered++
		default:
			nextAvailableAt := now.Add(commentProjectorBackoff(event.AttemptCount))
			if _, err := p.outbox.MarkRetry(ctx, event.ID, workerID, processErr.Error(), nextAvailableAt, now); err != nil {
				recordErr(err)
				continue
			}
			result.Retried++
		}
	}

	return result, batchErr
}

func (p CommentNotificationProjector) processEvent(ctx context.Context, event domain.OutboxEvent) (bool, error) {
	if !isCommentOutboxTopic(event.Topic) {
		return false, commentProjectorPermanentError{err: fmt.Errorf("unsupported topic %q", event.Topic)}
	}

	payload, err := decodeCommentNotificationPayload(event.Payload)
	if err != nil {
		return false, err
	}
	mentionUserIDs, err := decodeMentionUserIDs(event.Payload)
	if err != nil {
		return false, commentProjectorPermanentError{err: fmt.Errorf("project mention notifications: %w", err)}
	}
	payload.MentionUserIDs = mentionUserIDs

	detail, err := p.threads.GetThread(ctx, payload.ThreadID)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return false, commentProjectorPermanentError{err: fmt.Errorf("thread %s not found", payload.ThreadID)}
	case err != nil:
		return false, err
	}
	if detail.Thread.ID != payload.ThreadID {
		return false, commentProjectorPermanentError{err: fmt.Errorf("thread payload %s does not match loaded thread %s", payload.ThreadID, detail.Thread.ID)}
	}
	if detail.Thread.PageID != payload.PageID {
		return false, commentProjectorPermanentError{err: fmt.Errorf("payload page_id %s does not match loaded thread page %s", payload.PageID, detail.Thread.PageID)}
	}
	page, _, err := p.pages.GetByID(ctx, detail.Thread.PageID)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return false, commentProjectorPermanentError{err: fmt.Errorf("page %s not found", detail.Thread.PageID)}
	case err != nil:
		return false, err
	}
	if strings.TrimSpace(page.WorkspaceID) != strings.TrimSpace(payload.WorkspaceID) {
		return false, commentProjectorPermanentError{err: fmt.Errorf("payload workspace_id %s does not match page workspace %s", payload.WorkspaceID, page.WorkspaceID)}
	}

	message, ok := findThreadMessage(detail.Messages, payload.MessageID)
	if !ok {
		return false, commentProjectorPermanentError{err: fmt.Errorf("message %s not found in thread %s", payload.MessageID, payload.ThreadID)}
	}
	if message.ThreadID != payload.ThreadID {
		return false, commentProjectorPermanentError{err: fmt.Errorf("message %s does not belong to thread %s", message.ID, payload.ThreadID)}
	}
	if strings.TrimSpace(message.CreatedBy) != strings.TrimSpace(payload.ActorID) {
		return false, commentProjectorPermanentError{err: fmt.Errorf("payload actor_id %s does not match message author %s", payload.ActorID, message.CreatedBy)}
	}
	if !message.CreatedAt.UTC().Equal(payload.OccurredAt.UTC()) {
		return false, commentProjectorPermanentError{err: fmt.Errorf("payload occurred_at %s does not match message created_at %s", payload.OccurredAt.UTC().Format(time.RFC3339), message.CreatedAt.UTC().Format(time.RFC3339))}
	}
	payload.WorkspaceID = page.WorkspaceID
	payload.OccurredAt = message.CreatedAt.UTC()

	recipients, err := p.resolver.ResolveRecipients(ctx, ResolveThreadNotificationRecipientsInput{
		WorkspaceID:            page.WorkspaceID,
		ActorID:                payload.ActorID,
		Detail:                 detail,
		ExplicitMentionUserIDs: payload.MentionUserIDs,
	})
	switch {
	case errors.Is(err, domain.ErrValidation):
		return false, commentProjectorPermanentError{err: fmt.Errorf("resolve recipients: %w", err)}
	case err != nil:
		return false, err
	}

	mentionNotifications, err := p.mentions.Build(ctx, event.Topic, payload)
	switch {
	case err == nil:
	case isPermanentMentionProjectorError(err):
		return false, commentProjectorPermanentError{err: fmt.Errorf("project mention notifications: %w", err)}
	default:
		return false, err
	}

	commentNotifications := make([]domain.Notification, 0, len(recipients))
	for _, recipientID := range recipients {
		commentNotifications = append(commentNotifications, mapCommentNotification(event.Topic, payload, recipientID))
	}

	if len(commentNotifications) == 0 && len(mentionNotifications) == 0 {
		return true, nil
	}

	_, _, err = p.notifications.CreateCommentAndMentionNotifications(ctx, commentNotifications, mentionNotifications)
	if err != nil {
		if errors.Is(err, domain.ErrValidation) {
			return false, commentProjectorPermanentError{err: fmt.Errorf("create comment and mention notifications: %w", err)}
		}
		return false, err
	}

	return false, nil
}

type commentProjectorPermanentError struct {
	err error
}

func (e commentProjectorPermanentError) Error() string {
	return e.err.Error()
}

func (e commentProjectorPermanentError) Unwrap() error {
	return e.err
}

func isPermanentCommentProjectorError(err error) bool {
	var permanent commentProjectorPermanentError
	return errors.As(err, &permanent)
}

func decodeCommentNotificationPayload(raw json.RawMessage) (commentNotificationPayload, error) {
	if len(raw) == 0 {
		return commentNotificationPayload{}, commentProjectorPermanentError{err: errors.New("payload must be a JSON object")}
	}
	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return commentNotificationPayload{}, commentProjectorPermanentError{err: fmt.Errorf("invalid JSON payload: %w", err)}
	}
	if decoded == nil {
		return commentNotificationPayload{}, commentProjectorPermanentError{err: errors.New("payload must be a JSON object")}
	}
	var payload commentNotificationPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return commentNotificationPayload{}, commentProjectorPermanentError{err: fmt.Errorf("invalid comment payload: %w", err)}
	}
	if strings.TrimSpace(payload.ThreadID) == "" {
		return commentNotificationPayload{}, commentProjectorPermanentError{err: errors.New("payload.thread_id is required")}
	}
	if strings.TrimSpace(payload.MessageID) == "" {
		return commentNotificationPayload{}, commentProjectorPermanentError{err: errors.New("payload.message_id is required")}
	}
	if strings.TrimSpace(payload.PageID) == "" {
		return commentNotificationPayload{}, commentProjectorPermanentError{err: errors.New("payload.page_id is required")}
	}
	if strings.TrimSpace(payload.WorkspaceID) == "" {
		return commentNotificationPayload{}, commentProjectorPermanentError{err: errors.New("payload.workspace_id is required")}
	}
	if strings.TrimSpace(payload.ActorID) == "" {
		return commentNotificationPayload{}, commentProjectorPermanentError{err: errors.New("payload.actor_id is required")}
	}
	if payload.OccurredAt.IsZero() {
		return commentNotificationPayload{}, commentProjectorPermanentError{err: errors.New("payload.occurred_at is required")}
	}
	mentionUserIDs, err := decodeMentionUserIDs(raw)
	if err != nil {
		if isPermanentMentionProjectorError(err) {
			return commentNotificationPayload{}, commentProjectorPermanentError{err: fmt.Errorf("validate mention payload: %w", err)}
		}
		return commentNotificationPayload{}, err
	}
	payload.MentionUserIDs = mentionUserIDs
	return payload, nil
}

func mapCommentNotification(topic domain.OutboxTopic, payload commentNotificationPayload, recipientID string) domain.Notification {
	title, content := commentNotificationText(topic)
	resourceType := domain.NotificationResourceTypeThreadMsg
	resourceID := payload.MessageID
	projectedPayload, _ := json.Marshal(map[string]any{
		"thread_id":    payload.ThreadID,
		"message_id":   payload.MessageID,
		"page_id":      payload.PageID,
		"workspace_id": payload.WorkspaceID,
		"actor_id":     payload.ActorID,
		"event_topic":  string(topic),
	})

	occurredAt := payload.OccurredAt.UTC()
	return domain.Notification{
		ID:           uuid.NewString(),
		UserID:       recipientID,
		WorkspaceID:  payload.WorkspaceID,
		Type:         domain.NotificationTypeComment,
		EventID:      payload.MessageID,
		Message:      content,
		ActorID:      &payload.ActorID,
		Title:        title,
		Content:      content,
		IsRead:       false,
		Actionable:   false,
		ResourceType: &resourceType,
		ResourceID:   &resourceID,
		Payload:      projectedPayload,
		CreatedAt:    occurredAt,
		UpdatedAt:    occurredAt,
	}
}

func commentNotificationText(topic domain.OutboxTopic) (string, string) {
	switch topic {
	case domain.OutboxTopicThreadCreated:
		return "New comment thread", "A new relevant comment thread was created"
	case domain.OutboxTopicThreadReplyCreated:
		return "New thread reply", "A relevant comment thread has a new reply"
	default:
		return "Comment activity", "Comment activity"
	}
}

func commentOutboxTopics() []domain.OutboxTopic {
	return []domain.OutboxTopic{
		domain.OutboxTopicThreadCreated,
		domain.OutboxTopicThreadReplyCreated,
	}
}

func isCommentOutboxTopic(topic domain.OutboxTopic) bool {
	for _, candidate := range commentOutboxTopics() {
		if topic == candidate {
			return true
		}
	}
	return false
}

func commentProjectorBackoff(attemptCount int) time.Duration {
	return invitationProjectorBackoff(attemptCount)
}

func findThreadMessage(messages []domain.PageCommentThreadMessage, messageID string) (domain.PageCommentThreadMessage, bool) {
	for _, message := range messages {
		if message.ID == messageID {
			return message, true
		}
	}
	return domain.PageCommentThreadMessage{}, false
}
