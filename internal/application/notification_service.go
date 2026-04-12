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

type NotificationRepository interface {
	Create(ctx context.Context, notification domain.Notification) (domain.Notification, error)
	CreateMany(ctx context.Context, notifications []domain.Notification) error
	BatchMarkRead(ctx context.Context, userID string, notificationIDs []string, readAt time.Time) (domain.NotificationBatchReadResult, error)
	ListInbox(ctx context.Context, userID string, filter domain.NotificationInboxFilter) (domain.NotificationInboxPage, error)
	MarkRead(ctx context.Context, notificationID, userID string, readAt time.Time) (domain.NotificationInboxItem, error)
	GetUnreadCount(ctx context.Context, userID string) (int64, error)
}

type ListNotificationsInput struct {
	Status string
	Type   string
	Limit  int
	Cursor string
}

type NotificationService struct {
	notifications NotificationRepository
	users         UserRepository
	memberships   WorkspaceMembershipReader
}

func NewNotificationService(notifications NotificationRepository, users UserRepository, memberships WorkspaceMembershipReader) NotificationService {
	return NotificationService{
		notifications: notifications,
		users:         users,
		memberships:   memberships,
	}
}

func (s NotificationService) ListNotifications(ctx context.Context, actorID string, input ListNotificationsInput) (domain.NotificationInboxPage, error) {
	if _, err := s.users.GetByID(ctx, actorID); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.NotificationInboxPage{}, domain.ErrUnauthorized
		}
		return domain.NotificationInboxPage{}, err
	}

	filter, err := normalizeNotificationInboxFilter(input)
	if err != nil {
		return domain.NotificationInboxPage{}, err
	}
	return s.notifications.ListInbox(ctx, actorID, filter)
}

func (s NotificationService) MarkNotificationRead(ctx context.Context, actorID, notificationID string) (domain.NotificationInboxItem, error) {
	if _, err := s.users.GetByID(ctx, actorID); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.NotificationInboxItem{}, domain.ErrUnauthorized
		}
		return domain.NotificationInboxItem{}, err
	}
	if _, err := uuid.Parse(strings.TrimSpace(notificationID)); err != nil {
		return domain.NotificationInboxItem{}, domain.ErrNotFound
	}
	return s.notifications.MarkRead(ctx, notificationID, actorID, time.Now().UTC())
}

func (s NotificationService) MarkNotificationsRead(ctx context.Context, actorID string, input domain.BatchMarkNotificationsReadInput) (domain.NotificationBatchReadResult, error) {
	if _, err := s.users.GetByID(ctx, actorID); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.NotificationBatchReadResult{}, domain.ErrUnauthorized
		}
		return domain.NotificationBatchReadResult{}, err
	}

	notificationIDs, err := normalizeBatchNotificationIDs(input.NotificationIDs)
	if err != nil {
		return domain.NotificationBatchReadResult{}, err
	}

	return s.notifications.BatchMarkRead(ctx, actorID, notificationIDs, time.Now().UTC())
}

func (s NotificationService) GetUnreadCount(ctx context.Context, actorID string) (domain.NotificationUnreadCount, error) {
	if _, err := s.users.GetByID(ctx, actorID); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.NotificationUnreadCount{}, domain.ErrUnauthorized
		}
		return domain.NotificationUnreadCount{}, err
	}
	unreadCount, err := s.notifications.GetUnreadCount(ctx, actorID)
	if err != nil {
		return domain.NotificationUnreadCount{}, err
	}
	return domain.NotificationUnreadCount{UnreadCount: unreadCount}, nil
}

func (s NotificationService) NotifyInvitationCreated(ctx context.Context, invitation domain.WorkspaceInvitation) error {
	user, err := s.users.GetByEmail(ctx, invitation.Email)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return nil
	case err != nil:
		return err
	}

	_, err = s.notifications.Create(ctx, domain.Notification{
		ID:           uuid.NewString(),
		UserID:       user.ID,
		WorkspaceID:  invitation.WorkspaceID,
		Type:         domain.NotificationTypeInvitation,
		EventID:      invitation.ID,
		Message:      "You have a new workspace invitation",
		ActorID:      &invitation.InvitedBy,
		Title:        "Workspace invitation",
		Content:      "You have a new workspace invitation",
		IsRead:       false,
		Actionable:   true,
		ActionKind:   notificationActionKindPtr(domain.NotificationActionKindInvitationResponse),
		ResourceType: notificationResourceTypePtr(domain.NotificationResourceTypeInvitation),
		ResourceID:   &invitation.ID,
		Payload:      json.RawMessage(`{}`),
		CreatedAt:    time.Now().UTC(),
	})
	if errors.Is(err, domain.ErrConflict) {
		return nil
	}
	return err
}

func (s NotificationService) NotifyCommentCreated(ctx context.Context, page domain.Page, comment domain.PageComment) error {
	return s.notifyPageMembers(ctx, page.WorkspaceID, comment.CreatedBy, comment.ID, "New comment on a page in your workspace", "Comment activity", domain.NotificationResourceTypePageComment)
}

func (s NotificationService) notifyPageMembers(ctx context.Context, workspaceID, actorID, eventID, message, title string, resourceType domain.NotificationResourceType) error {
	members, err := s.memberships.ListMembers(ctx, workspaceID)
	if err != nil {
		return err
	}

	notifications := make([]domain.Notification, 0, len(members))
	createdAt := time.Now().UTC()
	for _, member := range members {
		if member.UserID == actorID {
			continue
		}

		notifications = append(notifications, domain.Notification{
			ID:           uuid.NewString(),
			UserID:       member.UserID,
			WorkspaceID:  workspaceID,
			Type:         domain.NotificationTypeComment,
			EventID:      eventID,
			Message:      message,
			ActorID:      &actorID,
			Title:        title,
			Content:      message,
			IsRead:       false,
			Actionable:   false,
			ResourceType: notificationResourceTypePtr(resourceType),
			ResourceID:   &eventID,
			Payload:      json.RawMessage(`{}`),
			CreatedAt:    createdAt,
		})
	}

	if len(notifications) == 0 {
		return nil
	}

	return s.notifications.CreateMany(ctx, notifications)
}

func notificationActionKindPtr(kind domain.NotificationActionKind) *domain.NotificationActionKind {
	return &kind
}

func notificationResourceTypePtr(resourceType domain.NotificationResourceType) *domain.NotificationResourceType {
	return &resourceType
}

func normalizeNotificationInboxFilter(input ListNotificationsInput) (domain.NotificationInboxFilter, error) {
	filter := domain.NotificationInboxFilter{
		Status: domain.NotificationInboxStatus(strings.TrimSpace(input.Status)),
		Type:   domain.NotificationInboxType(strings.TrimSpace(input.Type)),
		Limit:  input.Limit,
		Cursor: strings.TrimSpace(input.Cursor),
	}
	if filter.Status == "" {
		filter.Status = domain.NotificationInboxStatusAll
	}
	switch filter.Status {
	case domain.NotificationInboxStatusAll, domain.NotificationInboxStatusRead, domain.NotificationInboxStatusUnread:
	default:
		return domain.NotificationInboxFilter{}, fmt.Errorf("%w: invalid status", domain.ErrValidation)
	}

	if filter.Type == "" {
		filter.Type = domain.NotificationInboxTypeAll
	}
	switch filter.Type {
	case domain.NotificationInboxTypeAll, domain.NotificationInboxTypeInvitation, domain.NotificationInboxTypeComment, domain.NotificationInboxTypeMention:
	default:
		return domain.NotificationInboxFilter{}, fmt.Errorf("%w: invalid type", domain.ErrValidation)
	}

	if filter.Limit == 0 {
		filter.Limit = 50
	}
	if filter.Limit <= 0 || filter.Limit > 100 {
		return domain.NotificationInboxFilter{}, fmt.Errorf("%w: invalid limit", domain.ErrValidation)
	}
	return filter, nil
}

func normalizeBatchNotificationIDs(notificationIDs []string) ([]string, error) {
	if len(notificationIDs) == 0 || len(notificationIDs) > 100 {
		return nil, fmt.Errorf("%w: notification_ids must contain between 1 and 100 items", domain.ErrValidation)
	}

	normalized := make([]string, 0, len(notificationIDs))
	seen := make(map[string]struct{}, len(notificationIDs))
	for _, rawID := range notificationIDs {
		parsedID, err := uuid.Parse(strings.TrimSpace(rawID))
		if err != nil {
			return nil, fmt.Errorf("%w: notification_ids must contain valid UUIDs", domain.ErrValidation)
		}

		normalizedID := parsedID.String()
		if _, ok := seen[normalizedID]; ok {
			return nil, fmt.Errorf("%w: duplicate notification_ids are not allowed", domain.ErrValidation)
		}
		seen[normalizedID] = struct{}{}
		normalized = append(normalized, normalizedID)
	}

	return normalized, nil
}
