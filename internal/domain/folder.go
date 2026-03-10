package domain

import "time"

type Folder struct {
	ID          string    `json:"id"`
	WorkspaceID string    `json:"workspace_id"`
	ParentID    *string   `json:"parent_id,omitempty"`
	Name        string    `json:"name"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}
