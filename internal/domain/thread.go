package domain

import "time"

type PageCommentThreadState string

const (
	PageCommentThreadStateOpen     PageCommentThreadState = "open"
	PageCommentThreadStateResolved PageCommentThreadState = "resolved"
)

type PageCommentThreadAnchorState string

const (
	PageCommentThreadAnchorStateActive   PageCommentThreadAnchorState = "active"
	PageCommentThreadAnchorStateOutdated PageCommentThreadAnchorState = "outdated"
	PageCommentThreadAnchorStateMissing  PageCommentThreadAnchorState = "missing"
)

type PageCommentThreadAnchorType string

const (
	PageCommentThreadAnchorTypeBlock      PageCommentThreadAnchorType = "block"
	PageCommentThreadAnchorTypePageLegacy PageCommentThreadAnchorType = "page_legacy"
)

type PageCommentThreadAnchor struct {
	Type            PageCommentThreadAnchorType `json:"type"`
	BlockID         *string                     `json:"block_id,omitempty"`
	QuotedText      *string                     `json:"quoted_text,omitempty"`
	QuotedBlockText string                      `json:"quoted_block_text"`
}

type PageCommentThread struct {
	ID             string                       `json:"id"`
	PageID         string                       `json:"page_id"`
	Anchor         PageCommentThreadAnchor      `json:"anchor"`
	ThreadState    PageCommentThreadState       `json:"thread_state"`
	AnchorState    PageCommentThreadAnchorState `json:"anchor_state"`
	CreatedBy      string                       `json:"created_by"`
	CreatedAt      time.Time                    `json:"created_at"`
	ResolvedBy     *string                      `json:"resolved_by,omitempty"`
	ResolvedAt     *time.Time                   `json:"resolved_at,omitempty"`
	ResolveNote    *string                      `json:"resolve_note,omitempty"`
	ReopenedBy     *string                      `json:"reopened_by,omitempty"`
	ReopenedAt     *time.Time                   `json:"reopened_at,omitempty"`
	ReopenReason   *string                      `json:"reopen_reason,omitempty"`
	LastActivityAt time.Time                    `json:"last_activity_at"`
	ReplyCount     int                          `json:"reply_count"`
}

type PageCommentThreadMessage struct {
	ID        string    `json:"id"`
	ThreadID  string    `json:"thread_id"`
	Body      string    `json:"body"`
	CreatedBy string    `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
}

type PageCommentMessageMention struct {
	MessageID       string `json:"message_id"`
	MentionedUserID string `json:"mentioned_user_id"`
}

type ThreadNotificationMode string

const (
	ThreadNotificationModeAll          ThreadNotificationMode = "all"
	ThreadNotificationModeMentionsOnly ThreadNotificationMode = "mentions_only"
	ThreadNotificationModeMute         ThreadNotificationMode = "mute"
)

func IsValidThreadNotificationMode(mode ThreadNotificationMode) bool {
	switch mode {
	case ThreadNotificationModeAll, ThreadNotificationModeMentionsOnly, ThreadNotificationModeMute:
		return true
	default:
		return false
	}
}

type ThreadNotificationPreference struct {
	ThreadID  string                 `json:"thread_id"`
	UserID    string                 `json:"user_id"`
	Mode      ThreadNotificationMode `json:"mode"`
	CreatedAt time.Time              `json:"created_at"`
	UpdatedAt time.Time              `json:"updated_at"`
}

type ThreadNotificationPreferenceView struct {
	ThreadID string                 `json:"thread_id"`
	Mode     ThreadNotificationMode `json:"mode"`
}

type ThreadNotificationPreferenceUpdateResult struct {
	ThreadID  string                 `json:"thread_id"`
	Mode      ThreadNotificationMode `json:"mode"`
	UpdatedAt time.Time              `json:"updated_at"`
}

type PageCommentThreadEventType string

const (
	PageCommentThreadEventTypeCreated            PageCommentThreadEventType = "created"
	PageCommentThreadEventTypeReplied            PageCommentThreadEventType = "replied"
	PageCommentThreadEventTypeResolved           PageCommentThreadEventType = "resolved"
	PageCommentThreadEventTypeReopened           PageCommentThreadEventType = "reopened"
	PageCommentThreadEventTypeAnchorStateChanged PageCommentThreadEventType = "anchor_state_changed"
	PageCommentThreadEventTypeAnchorRecovered    PageCommentThreadEventType = "anchor_recovered"
)

type PageCommentThreadEvent struct {
	ID              string                        `json:"id"`
	ThreadID        string                        `json:"thread_id"`
	Type            PageCommentThreadEventType    `json:"type"`
	ActorID         *string                       `json:"actor_id,omitempty"`
	MessageID       *string                       `json:"message_id,omitempty"`
	RevisionID      *string                       `json:"revision_id,omitempty"`
	FromThreadState *PageCommentThreadState       `json:"from_thread_state,omitempty"`
	ToThreadState   *PageCommentThreadState       `json:"to_thread_state,omitempty"`
	FromAnchorState *PageCommentThreadAnchorState `json:"from_anchor_state,omitempty"`
	ToAnchorState   *PageCommentThreadAnchorState `json:"to_anchor_state,omitempty"`
	FromBlockID     *string                       `json:"from_block_id,omitempty"`
	ToBlockID       *string                       `json:"to_block_id,omitempty"`
	Reason          *PageCommentThreadEventReason `json:"reason,omitempty"`
	Note            *string                       `json:"note,omitempty"`
	CreatedAt       time.Time                     `json:"created_at"`
}

type PageCommentThreadEventReason string

const (
	PageCommentThreadEventReasonDraftUpdated    PageCommentThreadEventReason = "draft_updated"
	PageCommentThreadEventReasonPageDeleted     PageCommentThreadEventReason = "page_deleted"
	PageCommentThreadEventReasonPageRestored    PageCommentThreadEventReason = "page_restored"
	PageCommentThreadEventReasonRevisionRestore PageCommentThreadEventReason = "revision_restored"
)

type ThreadAnchorReevaluationContext struct {
	Reason     PageCommentThreadEventReason `json:"reason"`
	RevisionID *string                      `json:"revision_id,omitempty"`
}

type PageCommentThreadDetail struct {
	Thread   PageCommentThread          `json:"thread"`
	Messages []PageCommentThreadMessage `json:"messages"`
	Events   []PageCommentThreadEvent   `json:"events"`
}

type PageCommentThreadFilterCounts struct {
	Open     int `json:"open"`
	Resolved int `json:"resolved"`
	Active   int `json:"active"`
	Outdated int `json:"outdated"`
	Missing  int `json:"missing"`
}

type PageCommentThreadList struct {
	Threads    []PageCommentThread           `json:"threads"`
	Counts     PageCommentThreadFilterCounts `json:"counts"`
	NextCursor *string                       `json:"next_cursor,omitempty"`
	HasMore    bool                          `json:"has_more"`
}

type WorkspaceCommentThreadListItem struct {
	Thread PageCommentThread `json:"thread"`
	Page   PageSummary       `json:"page"`
}

type WorkspaceCommentThreadList struct {
	Threads    []WorkspaceCommentThreadListItem `json:"threads"`
	Counts     PageCommentThreadFilterCounts    `json:"counts"`
	NextCursor *string                          `json:"next_cursor,omitempty"`
	HasMore    bool                             `json:"has_more"`
}
