package application

import (
	"context"

	"note-app/internal/domain"
)

type NotificationEventPublisher interface {
	NotifyInvitationCreated(ctx context.Context, invitation domain.WorkspaceInvitation) error
	NotifyCommentCreated(ctx context.Context, page domain.Page, comment domain.PageComment) error
}
