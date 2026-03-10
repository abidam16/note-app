package domain

import "time"

type PageComment struct {
	ID         string     `json:"id"`
	PageID     string     `json:"page_id"`
	Body       string     `json:"body"`
	CreatedBy  string     `json:"created_by"`
	CreatedAt  time.Time  `json:"created_at"`
	ResolvedBy *string    `json:"resolved_by,omitempty"`
	ResolvedAt *time.Time `json:"resolved_at,omitempty"`
}
