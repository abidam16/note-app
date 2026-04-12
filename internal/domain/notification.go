package domain

import (
	"encoding/json"
	"time"
)

type NotificationType string
type NotificationActionKind string
type NotificationResourceType string
type NotificationInboxStatus string
type NotificationInboxType string

const (
	NotificationTypeInvitation NotificationType = "invitation"
	NotificationTypeComment    NotificationType = "comment"
	NotificationTypeMention    NotificationType = "mention"
)

const (
	NotificationInboxStatusAll    NotificationInboxStatus = "all"
	NotificationInboxStatusRead   NotificationInboxStatus = "read"
	NotificationInboxStatusUnread NotificationInboxStatus = "unread"
)

const (
	NotificationInboxTypeAll        NotificationInboxType = "all"
	NotificationInboxTypeInvitation NotificationInboxType = "invitation"
	NotificationInboxTypeComment    NotificationInboxType = "comment"
	NotificationInboxTypeMention    NotificationInboxType = "mention"
)

const (
	NotificationActionKindInvitationResponse NotificationActionKind = "invitation_response"
)

const (
	NotificationResourceTypeInvitation  NotificationResourceType = "invitation"
	NotificationResourceTypePageComment NotificationResourceType = "page_comment"
	NotificationResourceTypeThread      NotificationResourceType = "thread"
	NotificationResourceTypeThreadMsg   NotificationResourceType = "thread_message"
)

type Notification struct {
	ID           string                    `json:"id"`
	UserID       string                    `json:"user_id"`
	WorkspaceID  string                    `json:"workspace_id"`
	Type         NotificationType          `json:"type"`
	EventID      string                    `json:"event_id"`
	Message      string                    `json:"message"`
	CreatedAt    time.Time                 `json:"created_at"`
	ReadAt       *time.Time                `json:"read_at,omitempty"`
	ActorID      *string                   `json:"-"`
	Title        string                    `json:"-"`
	Content      string                    `json:"-"`
	IsRead       bool                      `json:"-"`
	Actionable   bool                      `json:"-"`
	ActionKind   *NotificationActionKind   `json:"-"`
	ResourceType *NotificationResourceType `json:"-"`
	ResourceID   *string                   `json:"-"`
	Payload      json.RawMessage           `json:"-"`
	UpdatedAt    time.Time                 `json:"-"`
}

type NotificationActor struct {
	ID       string `json:"id"`
	Email    string `json:"email"`
	FullName string `json:"full_name"`
}

type NotificationInboxItem struct {
	ID           string                    `json:"id"`
	WorkspaceID  string                    `json:"workspace_id"`
	Type         NotificationType          `json:"type"`
	ActorID      *string                   `json:"actor_id"`
	Actor        *NotificationActor        `json:"actor"`
	Title        string                    `json:"title"`
	Content      string                    `json:"content"`
	IsRead       bool                      `json:"is_read"`
	ReadAt       *time.Time                `json:"read_at"`
	Actionable   bool                      `json:"actionable"`
	ActionKind   *NotificationActionKind   `json:"action_kind"`
	ResourceType *NotificationResourceType `json:"resource_type"`
	ResourceID   *string                   `json:"resource_id"`
	Payload      json.RawMessage           `json:"payload"`
	CreatedAt    time.Time                 `json:"created_at"`
	UpdatedAt    time.Time                 `json:"updated_at"`
}

type NotificationInboxPage struct {
	Items       []NotificationInboxItem `json:"items"`
	UnreadCount int64                   `json:"unread_count"`
	NextCursor  *string                 `json:"next_cursor,omitempty"`
	HasMore     bool                    `json:"has_more"`
}

type NotificationUnreadCount struct {
	UnreadCount int64 `json:"unread_count"`
}

type BatchMarkNotificationsReadInput struct {
	NotificationIDs []string `json:"notification_ids"`
}

type NotificationBatchReadResult struct {
	UpdatedCount int64 `json:"updated_count"`
	UnreadCount  int64 `json:"unread_count"`
}

type NotificationStreamReason string

const (
	NotificationStreamReasonNotificationsChanged NotificationStreamReason = "notifications_changed"
)

type NotificationStreamSignal struct {
	UserID string                   `json:"user_id"`
	Reason NotificationStreamReason `json:"reason"`
	SentAt time.Time                `json:"sent_at"`
}

type NotificationStreamSubscription interface {
	Events() <-chan NotificationStreamSignal
	Close() error
}

type NotificationInboxFilter struct {
	Status NotificationInboxStatus
	Type   NotificationInboxType
	Limit  int
	Cursor string
}
