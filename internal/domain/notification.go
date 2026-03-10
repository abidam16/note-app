package domain

import "time"

type NotificationType string

const (
	NotificationTypeInvitation NotificationType = "invitation"
	NotificationTypeComment    NotificationType = "comment"
)

type Notification struct {
	ID          string           `json:"id"`
	UserID      string           `json:"user_id"`
	WorkspaceID string           `json:"workspace_id"`
	Type        NotificationType `json:"type"`
	EventID     string           `json:"event_id"`
	Message     string           `json:"message"`
	CreatedAt   time.Time        `json:"created_at"`
	ReadAt      *time.Time       `json:"read_at,omitempty"`
}
