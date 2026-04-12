package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"note-app/internal/domain"

	"github.com/google/uuid"
)

type mentionNotificationRepository interface {
	CreateMentionNotifications(ctx context.Context, notifications []domain.Notification) (int, error)
}

type MentionNotificationProjector struct {
	memberships   WorkspaceMembershipReader
	notifications mentionNotificationRepository
}

func NewMentionNotificationProjector(memberships WorkspaceMembershipReader, notifications mentionNotificationRepository) MentionNotificationProjector {
	return MentionNotificationProjector{
		memberships:   memberships,
		notifications: notifications,
	}
}

func (p MentionNotificationProjector) Build(ctx context.Context, topic domain.OutboxTopic, payload commentNotificationPayload) ([]domain.Notification, error) {
	if !isCommentOutboxTopic(topic) {
		return nil, mentionProjectorPermanentError{err: fmt.Errorf("unsupported topic %q", topic)}
	}
	if strings.TrimSpace(payload.ThreadID) == "" || strings.TrimSpace(payload.MessageID) == "" || strings.TrimSpace(payload.PageID) == "" || strings.TrimSpace(payload.WorkspaceID) == "" || strings.TrimSpace(payload.ActorID) == "" {
		return nil, mentionProjectorPermanentError{err: errors.New("payload is missing required thread fields")}
	}

	if len(payload.MentionUserIDs) == 0 {
		return nil, nil
	}

	members, err := p.memberships.ListMembers(ctx, payload.WorkspaceID)
	if err != nil {
		return nil, err
	}

	activeMembers := make(map[string]struct{}, len(members))
	for _, member := range members {
		if userID := strings.TrimSpace(member.UserID); userID != "" {
			activeMembers[userID] = struct{}{}
		}
	}

	recipients := normalizeMentionRecipients(payload.MentionUserIDs, payload.ActorID, activeMembers)
	if len(recipients) == 0 {
		return nil, nil
	}

	notifications := make([]domain.Notification, 0, len(recipients))
	for _, recipientID := range recipients {
		notifications = append(notifications, mapMentionNotification(topic, payload, recipientID))
	}

	return notifications, nil
}

func (p MentionNotificationProjector) Project(ctx context.Context, topic domain.OutboxTopic, payload commentNotificationPayload) (bool, error) {
	notifications, err := p.Build(ctx, topic, payload)
	if err != nil {
		return false, err
	}
	if len(notifications) == 0 {
		return false, nil
	}

	if _, err := p.notifications.CreateMentionNotifications(ctx, notifications); err != nil {
		if errors.Is(err, domain.ErrValidation) {
			return false, mentionProjectorPermanentError{err: fmt.Errorf("create mention notifications: %w", err)}
		}
		return false, err
	}

	return true, nil
}

type mentionProjectorPermanentError struct {
	err error
}

func (e mentionProjectorPermanentError) Error() string {
	return e.err.Error()
}

func (e mentionProjectorPermanentError) Unwrap() error {
	return e.err
}

func isPermanentMentionProjectorError(err error) bool {
	var permanent mentionProjectorPermanentError
	return errors.As(err, &permanent)
}

func decodeMentionUserIDs(raw json.RawMessage) ([]string, error) {
	if len(raw) == 0 {
		return nil, mentionProjectorPermanentError{err: errors.New("payload must be a JSON object")}
	}

	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil, mentionProjectorPermanentError{err: fmt.Errorf("invalid JSON payload: %w", err)}
	}
	if decoded == nil {
		return nil, mentionProjectorPermanentError{err: errors.New("payload must be a JSON object")}
	}

	rawMentions, ok := decoded["mention_user_ids"]
	if !ok {
		return nil, nil
	}
	if strings.EqualFold(strings.TrimSpace(string(rawMentions)), "null") {
		return nil, mentionProjectorPermanentError{err: errors.New("payload.mention_user_ids must be an array of strings")}
	}

	var mentionUserIDs []string
	if err := json.Unmarshal(rawMentions, &mentionUserIDs); err != nil {
		return nil, mentionProjectorPermanentError{err: fmt.Errorf("payload.mention_user_ids must be an array of strings: %w", err)}
	}
	return mentionUserIDs, nil
}

func normalizeMentionRecipients(mentionUserIDs []string, actorID string, activeMembers map[string]struct{}) []string {
	seen := make(map[string]struct{}, len(mentionUserIDs))
	recipients := make([]string, 0, len(mentionUserIDs))
	actorID = strings.TrimSpace(actorID)
	for _, mentionUserID := range mentionUserIDs {
		mentionUserID = strings.TrimSpace(mentionUserID)
		if mentionUserID == "" || mentionUserID == actorID {
			continue
		}
		if _, ok := activeMembers[mentionUserID]; !ok {
			continue
		}
		if _, ok := seen[mentionUserID]; ok {
			continue
		}
		seen[mentionUserID] = struct{}{}
		recipients = append(recipients, mentionUserID)
	}
	return recipients
}

func mapMentionNotification(topic domain.OutboxTopic, payload commentNotificationPayload, recipientID string) domain.Notification {
	title, content := mentionNotificationText(topic)
	resourceType := domain.NotificationResourceTypeThreadMsg
	resourceID := payload.MessageID
	projectedPayload, _ := json.Marshal(map[string]any{
		"thread_id":      payload.ThreadID,
		"message_id":     payload.MessageID,
		"page_id":        payload.PageID,
		"workspace_id":   payload.WorkspaceID,
		"actor_id":       payload.ActorID,
		"event_topic":    string(topic),
		"mention_source": "explicit",
	})

	occurredAt := payload.OccurredAt.UTC()
	return domain.Notification{
		ID:           uuid.NewString(),
		UserID:       recipientID,
		WorkspaceID:  payload.WorkspaceID,
		Type:         domain.NotificationTypeMention,
		EventID:      payload.MessageID,
		Message:      content,
		ActorID:      &payload.ActorID,
		Title:        title,
		Content:      content,
		ResourceType: &resourceType,
		ResourceID:   &resourceID,
		Payload:      projectedPayload,
		CreatedAt:    occurredAt,
		UpdatedAt:    occurredAt,
	}
}

func mentionNotificationText(topic domain.OutboxTopic) (string, string) {
	switch topic {
	case domain.OutboxTopicThreadCreated:
		return "Mentioned in a new comment thread", "You were mentioned in a new comment thread"
	case domain.OutboxTopicThreadReplyCreated:
		return "Mentioned in a thread reply", "You were mentioned in a thread reply"
	default:
		return "Mentioned", "You were mentioned"
	}
}
