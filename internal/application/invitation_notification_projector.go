package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"note-app/internal/domain"
)

type invitationNotificationOutboxRepository interface {
	ClaimPendingByTopics(ctx context.Context, workerID string, topics []domain.OutboxTopic, limit int, leaseDuration time.Duration, now time.Time) ([]domain.OutboxEvent, error)
	MarkProcessed(ctx context.Context, id, workerID string, processedAt time.Time) (domain.OutboxEvent, error)
	MarkRetry(ctx context.Context, id, workerID, lastError string, nextAvailableAt, failedAt time.Time) (domain.OutboxEvent, error)
	MarkDeadLetter(ctx context.Context, id, workerID, lastError string, deadLetteredAt time.Time) (domain.OutboxEvent, error)
}

type invitationLiveNotificationRepository interface {
	UpsertInvitationLive(ctx context.Context, notification domain.Notification) (domain.Notification, error)
}

type InvitationNotificationProjector struct {
	outbox        invitationNotificationOutboxRepository
	notifications invitationLiveNotificationRepository
	users         UserRepository
}

type InvitationNotificationProjectorResult struct {
	Claimed      int
	Processed    int
	Retried      int
	DeadLettered int
	Skipped      int
}

type invitationNotificationPayload struct {
	InvitationID string                           `json:"invitation_id"`
	WorkspaceID  string                           `json:"workspace_id"`
	ActorID      string                           `json:"actor_id"`
	Email        string                           `json:"email"`
	Role         domain.WorkspaceRole             `json:"role"`
	Status       domain.WorkspaceInvitationStatus `json:"status"`
	Version      int64                            `json:"version"`
	OccurredAt   time.Time                        `json:"occurred_at"`
}

func NewInvitationNotificationProjector(outbox invitationNotificationOutboxRepository, notifications invitationLiveNotificationRepository, users UserRepository) InvitationNotificationProjector {
	return InvitationNotificationProjector{
		outbox:        outbox,
		notifications: notifications,
		users:         users,
	}
}

func (p InvitationNotificationProjector) ProcessBatch(ctx context.Context, workerID string, limit int, leaseDuration time.Duration, now time.Time) (InvitationNotificationProjectorResult, error) {
	claimed, err := p.outbox.ClaimPendingByTopics(ctx, workerID, invitationOutboxTopics(), limit, leaseDuration, now)
	if err != nil {
		return InvitationNotificationProjectorResult{}, err
	}

	result := InvitationNotificationProjectorResult{Claimed: len(claimed)}
	for _, event := range claimed {
		permanent, processErr := p.processEvent(ctx, event)
		if processErr == nil {
			if _, err := p.outbox.MarkProcessed(ctx, event.ID, workerID, now); err != nil {
				return result, err
			}
			if permanent {
				result.Skipped++
			} else {
				result.Processed++
			}
			continue
		}

		if isPermanentInvitationProjectorError(processErr) {
			if _, err := p.outbox.MarkDeadLetter(ctx, event.ID, workerID, processErr.Error(), now); err != nil {
				return result, err
			}
			result.DeadLettered++
			continue
		}

		nextAvailableAt := now.Add(invitationProjectorBackoff(event.AttemptCount))
		if _, err := p.outbox.MarkRetry(ctx, event.ID, workerID, processErr.Error(), nextAvailableAt, now); err != nil {
			return result, err
		}
		result.Retried++
	}

	return result, nil
}

func (p InvitationNotificationProjector) processEvent(ctx context.Context, event domain.OutboxEvent) (bool, error) {
	if !isInvitationOutboxTopic(event.Topic) {
		return false, invitationProjectorPermanentError{err: fmt.Errorf("unsupported topic %q", event.Topic)}
	}

	payload, err := decodeInvitationNotificationPayload(event.Payload)
	if err != nil {
		return false, err
	}

	user, err := p.users.GetByEmail(ctx, payload.Email)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return true, nil
	case err != nil:
		return false, err
	}

	notification := mapInvitationNotification(event.Topic, payload, user.ID)
	if _, err := p.notifications.UpsertInvitationLive(ctx, notification); err != nil {
		return false, err
	}

	return false, nil
}

type invitationProjectorPermanentError struct {
	err error
}

func (e invitationProjectorPermanentError) Error() string {
	return e.err.Error()
}

func (e invitationProjectorPermanentError) Unwrap() error {
	return e.err
}

func isPermanentInvitationProjectorError(err error) bool {
	var permanent invitationProjectorPermanentError
	return errors.As(err, &permanent)
}

func decodeInvitationNotificationPayload(raw json.RawMessage) (invitationNotificationPayload, error) {
	if len(raw) == 0 {
		return invitationNotificationPayload{}, invitationProjectorPermanentError{err: errors.New("payload must be a JSON object")}
	}
	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return invitationNotificationPayload{}, invitationProjectorPermanentError{err: fmt.Errorf("invalid JSON payload: %w", err)}
	}

	var payload invitationNotificationPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return invitationNotificationPayload{}, invitationProjectorPermanentError{err: fmt.Errorf("invalid invitation payload: %w", err)}
	}
	if strings.TrimSpace(payload.InvitationID) == "" {
		return invitationNotificationPayload{}, invitationProjectorPermanentError{err: errors.New("payload.invitation_id is required")}
	}
	if strings.TrimSpace(payload.WorkspaceID) == "" {
		return invitationNotificationPayload{}, invitationProjectorPermanentError{err: errors.New("payload.workspace_id is required")}
	}
	if strings.TrimSpace(payload.ActorID) == "" {
		return invitationNotificationPayload{}, invitationProjectorPermanentError{err: errors.New("payload.actor_id is required")}
	}
	if strings.TrimSpace(payload.Email) == "" {
		return invitationNotificationPayload{}, invitationProjectorPermanentError{err: errors.New("payload.email is required")}
	}
	if !domain.IsValidWorkspaceRole(payload.Role) {
		return invitationNotificationPayload{}, invitationProjectorPermanentError{err: fmt.Errorf("payload.role %q is invalid", payload.Role)}
	}
	switch payload.Status {
	case domain.WorkspaceInvitationStatusPending, domain.WorkspaceInvitationStatusAccepted, domain.WorkspaceInvitationStatusRejected, domain.WorkspaceInvitationStatusCancelled:
	default:
		return invitationNotificationPayload{}, invitationProjectorPermanentError{err: fmt.Errorf("payload.status %q is invalid", payload.Status)}
	}
	if payload.Version <= 0 {
		return invitationNotificationPayload{}, invitationProjectorPermanentError{err: errors.New("payload.version must be greater than zero")}
	}
	if payload.OccurredAt.IsZero() {
		return invitationNotificationPayload{}, invitationProjectorPermanentError{err: errors.New("payload.occurred_at is required")}
	}
	return payload, nil
}

func mapInvitationNotification(topic domain.OutboxTopic, payload invitationNotificationPayload, userID string) domain.Notification {
	title := "Workspace invitation"
	message := "You have a new workspace invitation"
	actionable := true
	var actionKind *domain.NotificationActionKind
	invitationAction := domain.NotificationActionKindInvitationResponse
	actionKind = &invitationAction
	canAccept := true
	canReject := true

	switch topic {
	case domain.OutboxTopicInvitationUpdated:
		title = "Workspace invitation updated"
		message = "Your workspace invitation was updated"
	case domain.OutboxTopicInvitationAccepted:
		title = "Invitation accepted"
		message = "You accepted the workspace invitation"
		actionable = false
		actionKind = nil
		canAccept = false
		canReject = false
	case domain.OutboxTopicInvitationRejected:
		title = "Invitation rejected"
		message = "You rejected the workspace invitation"
		actionable = false
		actionKind = nil
		canAccept = false
		canReject = false
	case domain.OutboxTopicInvitationCancelled:
		title = "Invitation cancelled"
		message = "The workspace invitation was cancelled"
		actionable = false
		actionKind = nil
		canAccept = false
		canReject = false
	}

	resourceType := domain.NotificationResourceTypeInvitation
	resourceID := payload.InvitationID
	projectedPayload, _ := json.Marshal(map[string]any{
		"invitation_id": payload.InvitationID,
		"workspace_id":  payload.WorkspaceID,
		"email":         payload.Email,
		"role":          payload.Role,
		"status":        payload.Status,
		"version":       payload.Version,
		"can_accept":    canAccept,
		"can_reject":    canReject,
	})

	return domain.Notification{
		ID:           payload.InvitationID,
		UserID:       userID,
		WorkspaceID:  payload.WorkspaceID,
		Type:         domain.NotificationTypeInvitation,
		EventID:      payload.InvitationID,
		Message:      message,
		ActorID:      &payload.ActorID,
		Title:        title,
		Content:      message,
		Actionable:   actionable,
		ActionKind:   actionKind,
		ResourceType: &resourceType,
		ResourceID:   &resourceID,
		Payload:      projectedPayload,
		CreatedAt:    payload.OccurredAt.UTC(),
		UpdatedAt:    payload.OccurredAt.UTC(),
	}
}

func invitationOutboxTopics() []domain.OutboxTopic {
	return []domain.OutboxTopic{
		domain.OutboxTopicInvitationCreated,
		domain.OutboxTopicInvitationUpdated,
		domain.OutboxTopicInvitationAccepted,
		domain.OutboxTopicInvitationRejected,
		domain.OutboxTopicInvitationCancelled,
	}
}

func isInvitationOutboxTopic(topic domain.OutboxTopic) bool {
	for _, candidate := range invitationOutboxTopics() {
		if topic == candidate {
			return true
		}
	}
	return false
}

func invitationProjectorBackoff(attemptCount int) time.Duration {
	if attemptCount <= 1 {
		return 30 * time.Second
	}
	delay := 30 * time.Second
	for i := 1; i < attemptCount; i++ {
		delay *= 2
		if delay >= 15*time.Minute {
			return 15 * time.Minute
		}
	}
	return delay
}
