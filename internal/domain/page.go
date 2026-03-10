package domain

import (
	"encoding/json"
	"time"
)

type Page struct {
	ID          string    `json:"id"`
	WorkspaceID string    `json:"workspace_id"`
	FolderID    *string   `json:"folder_id,omitempty"`
	Title       string    `json:"title"`
	CreatedBy   string    `json:"created_by"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type PageDraft struct {
	PageID       string          `json:"page_id"`
	Content      json.RawMessage `json:"content"`
	SearchBody   string          `json:"-"`
	LastEditedBy string          `json:"last_edited_by"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
}

type PageSearchResult struct {
	ID          string    `json:"id"`
	WorkspaceID string    `json:"workspace_id"`
	FolderID    *string   `json:"folder_id,omitempty"`
	Title       string    `json:"title"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type TrashItem struct {
	ID          string    `json:"id"`
	WorkspaceID string    `json:"workspace_id"`
	PageID      string    `json:"page_id"`
	PageTitle   string    `json:"page_title"`
	DeletedBy   string    `json:"deleted_by"`
	DeletedAt   time.Time `json:"deleted_at"`
}
