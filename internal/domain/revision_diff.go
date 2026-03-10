package domain

type RevisionDiff struct {
	PageID         string              `json:"page_id"`
	FromRevisionID string              `json:"from_revision_id"`
	ToRevisionID   string              `json:"to_revision_id"`
	Blocks         []RevisionDiffBlock `json:"blocks"`
}

type RevisionDiffBlock struct {
	Index      int                     `json:"index"`
	Status     string                  `json:"status"`
	From       *RevisionDiffSnapshot   `json:"from,omitempty"`
	To         *RevisionDiffSnapshot   `json:"to,omitempty"`
	InlineDiff []RevisionDiffTextChunk `json:"inline_diff,omitempty"`
}

type RevisionDiffSnapshot struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

type RevisionDiffTextChunk struct {
	Operation string `json:"operation"`
	Text      string `json:"text"`
}
