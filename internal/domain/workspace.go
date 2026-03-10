package domain

import "time"

type WorkspaceRole string

const (
	RoleOwner  WorkspaceRole = "owner"
	RoleEditor WorkspaceRole = "editor"
	RoleViewer WorkspaceRole = "viewer"
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
	ID          string        `json:"id"`
	WorkspaceID string        `json:"workspace_id"`
	Email       string        `json:"email"`
	Role        WorkspaceRole `json:"role"`
	InvitedBy   string        `json:"invited_by"`
	AcceptedAt  *time.Time    `json:"accepted_at,omitempty"`
	CreatedAt   time.Time     `json:"created_at"`
}
