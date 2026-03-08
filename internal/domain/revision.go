package domain

import (
	"encoding/json"
	"time"
)

type Revision struct {
	ID        string          `json:"id"`
	PageID    string          `json:"page_id"`
	Label     *string         `json:"label,omitempty"`
	Note      *string         `json:"note,omitempty"`
	Content   json.RawMessage `json:"content,omitempty"`
	CreatedBy string          `json:"created_by"`
	CreatedAt time.Time       `json:"created_at"`
}
