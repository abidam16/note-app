package application

import (
	"context"
	"errors"
	"time"

	"note-app/internal/domain"

	"github.com/google/uuid"
)

type NotificationRepository interface {
	Create(ctx context.Context, notification domain.Notification) (domain.Notification, error)
	ListByUserID(ctx context.Context, userID string) ([]domain.Notification, error)
	MarkRead(ctx context.Context, notificationID, userID string, readAt time.Time) (domain.Notification, error)
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

func (s NotificationService) ListNotifications(ctx context.Context, actorID string) ([]domain.Notification, error) {
	return s.notifications.ListByUserID(ctx, actorID)
}

func (s NotificationService) MarkNotificationRead(ctx context.Context, actorID, notificationID string) (domain.Notification, error) {
	return s.notifications.MarkRead(ctx, notificationID, actorID, time.Now().UTC())
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
		ID:          uuid.NewString(),
		UserID:      user.ID,
		WorkspaceID: invitation.WorkspaceID,
		Type:        domain.NotificationTypeInvitation,
		EventID:     invitation.ID,
		Message:     "You have a new workspace invitation",
		CreatedAt:   time.Now().UTC(),
	})
	if errors.Is(err, domain.ErrConflict) {
		return nil
	}
	return err
}

func (s NotificationService) NotifyCommentCreated(ctx context.Context, page domain.Page, comment domain.PageComment) error {
	members, err := s.memberships.ListMembers(ctx, page.WorkspaceID)
	if err != nil {
		return err
	}

	for _, member := range members {
		if member.UserID == comment.CreatedBy {
			continue
		}

		_, err := s.notifications.Create(ctx, domain.Notification{
			ID:          uuid.NewString(),
			UserID:      member.UserID,
			WorkspaceID: page.WorkspaceID,
			Type:        domain.NotificationTypeComment,
			EventID:     comment.ID,
			Message:     "New comment on a page in your workspace",
			CreatedAt:   time.Now().UTC(),
		})
		if err != nil && !errors.Is(err, domain.ErrConflict) {
			return err
		}
	}

	return nil
}
