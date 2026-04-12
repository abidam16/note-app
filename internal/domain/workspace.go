package domain

import "time"

type WorkspaceRole string

const (
	RoleOwner  WorkspaceRole = "owner"
	RoleEditor WorkspaceRole = "editor"
	RoleViewer WorkspaceRole = "viewer"
)

type WorkspaceInvitationStatus string

const (
	WorkspaceInvitationStatusPending   WorkspaceInvitationStatus = "pending"
	WorkspaceInvitationStatusAccepted  WorkspaceInvitationStatus = "accepted"
	WorkspaceInvitationStatusRejected  WorkspaceInvitationStatus = "rejected"
	WorkspaceInvitationStatusCancelled WorkspaceInvitationStatus = "cancelled"
)

type WorkspaceInvitationStatusFilter string

const (
	WorkspaceInvitationStatusFilterAll       WorkspaceInvitationStatusFilter = "all"
	WorkspaceInvitationStatusFilterPending   WorkspaceInvitationStatusFilter = "pending"
	WorkspaceInvitationStatusFilterAccepted  WorkspaceInvitationStatusFilter = "accepted"
	WorkspaceInvitationStatusFilterRejected  WorkspaceInvitationStatusFilter = "rejected"
	WorkspaceInvitationStatusFilterCancelled WorkspaceInvitationStatusFilter = "cancelled"
)

func IsValidWorkspaceRole(role WorkspaceRole) bool {
	switch role {
	case RoleOwner, RoleEditor, RoleViewer:
		return true
	default:
		return false
	}
}

type Workspace struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type WorkspaceMember struct {
	ID          string        `json:"id"`
	WorkspaceID string        `json:"workspace_id"`
	UserID      string        `json:"user_id"`
	Role        WorkspaceRole `json:"role"`
	CreatedAt   time.Time     `json:"created_at"`
	User        *User         `json:"user,omitempty"`
}

type WorkspaceInvitation struct {
	ID          string                    `json:"id"`
	WorkspaceID string                    `json:"workspace_id"`
	Email       string                    `json:"email"`
	Role        WorkspaceRole             `json:"role"`
	InvitedBy   string                    `json:"invited_by"`
	AcceptedAt  *time.Time                `json:"accepted_at,omitempty"`
	CreatedAt   time.Time                 `json:"created_at"`
	Status      WorkspaceInvitationStatus `json:"status"`
	Version     int64                     `json:"version"`
	UpdatedAt   time.Time                 `json:"updated_at"`
	RespondedBy *string                   `json:"responded_by,omitempty"`
	RespondedAt *time.Time                `json:"responded_at,omitempty"`
	CancelledBy *string                   `json:"cancelled_by,omitempty"`
	CancelledAt *time.Time                `json:"cancelled_at,omitempty"`
}

type WorkspaceInvitationList struct {
	Items      []WorkspaceInvitation `json:"items"`
	NextCursor *string               `json:"next_cursor,omitempty"`
	HasMore    bool                  `json:"has_more"`
}

type AcceptInvitationResult struct {
	Invitation WorkspaceInvitation `json:"invitation"`
	Membership WorkspaceMember     `json:"membership"`
}
