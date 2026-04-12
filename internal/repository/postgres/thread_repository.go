package postgres

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"note-app/internal/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ThreadRepository struct {
	db *pgxpool.Pool
}

type threadListCursor struct {
	Sort           string                         `json:"sort"`
	ThreadState    *domain.PageCommentThreadState `json:"thread_state,omitempty"`
	LastActivityAt *time.Time                     `json:"last_activity_at,omitempty"`
	CreatedAt      *time.Time                     `json:"created_at,omitempty"`
	ID             string                         `json:"id"`
}

type dbtx interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func NewThreadRepository(db *pgxpool.Pool) ThreadRepository {
	return ThreadRepository{db: db}
}

func (r ThreadRepository) insertThreadEvent(ctx context.Context, q dbtx, event domain.PageCommentThreadEvent) error {
	_, err := q.Exec(ctx, `
		INSERT INTO page_comment_thread_events (
			id,
			thread_id,
			event_type,
			actor_id,
			message_id,
			revision_id,
			from_thread_state,
			to_thread_state,
			from_anchor_state,
			to_anchor_state,
			from_block_id,
			to_block_id,
			reason,
			note,
			created_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
	`, event.ID, event.ThreadID, event.Type, event.ActorID, event.MessageID, event.RevisionID, event.FromThreadState, event.ToThreadState, event.FromAnchorState, event.ToAnchorState, event.FromBlockID, event.ToBlockID, event.Reason, event.Note, event.CreatedAt)
	if err != nil {
		return fmt.Errorf("insert page comment thread event: %w", err)
	}
	return nil
}

func threadEventTime(preferred *time.Time) time.Time {
	if preferred != nil && !preferred.IsZero() {
		return preferred.UTC()
	}
	return time.Now().UTC()
}

func normalizeThreadListSortMode(sortMode string) (string, error) {
	switch sortMode {
	case "", "recent_activity":
		return "recent_activity", nil
	case "newest", "oldest":
		return sortMode, nil
	default:
		return "", fmt.Errorf("%w: invalid sort mode %q", domain.ErrValidation, sortMode)
	}
}

func encodeThreadListCursor(sortMode string, thread domain.PageCommentThread) (string, error) {
	cursor := threadListCursor{
		Sort: sortMode,
		ID:   thread.ID,
	}
	switch sortMode {
	case "recent_activity":
		cursor.ThreadState = &thread.ThreadState
		lastActivityAt := thread.LastActivityAt.UTC()
		cursor.LastActivityAt = &lastActivityAt
	case "newest", "oldest":
		createdAt := thread.CreatedAt.UTC()
		cursor.CreatedAt = &createdAt
	default:
		return "", fmt.Errorf("%w: invalid sort mode %q", domain.ErrValidation, sortMode)
	}

	encoded, err := json.Marshal(cursor)
	if err != nil {
		return "", fmt.Errorf("marshal thread list cursor: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(encoded), nil
}

func decodeThreadListCursor(rawCursor, sortMode string) (*threadListCursor, error) {
	if strings.TrimSpace(rawCursor) == "" {
		return nil, nil
	}

	decoded, err := base64.RawURLEncoding.DecodeString(rawCursor)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid cursor", domain.ErrValidation)
	}

	var cursor threadListCursor
	if err := json.Unmarshal(decoded, &cursor); err != nil {
		return nil, fmt.Errorf("%w: invalid cursor", domain.ErrValidation)
	}
	if cursor.Sort != sortMode || strings.TrimSpace(cursor.ID) == "" {
		return nil, fmt.Errorf("%w: invalid cursor", domain.ErrValidation)
	}

	switch sortMode {
	case "recent_activity":
		if cursor.ThreadState == nil || cursor.LastActivityAt == nil {
			return nil, fmt.Errorf("%w: invalid cursor", domain.ErrValidation)
		}
	case "newest", "oldest":
		if cursor.CreatedAt == nil {
			return nil, fmt.Errorf("%w: invalid cursor", domain.ErrValidation)
		}
	default:
		return nil, fmt.Errorf("%w: invalid cursor", domain.ErrValidation)
	}

	return &cursor, nil
}

func threadStateRank(state domain.PageCommentThreadState) int {
	if state == domain.PageCommentThreadStateOpen {
		return 0
	}
	return 1
}

func (r ThreadRepository) CreateThread(ctx context.Context, thread domain.PageCommentThread, firstMessage domain.PageCommentThreadMessage, mentions []domain.PageCommentMessageMention, outboxEvent domain.OutboxEvent) (domain.PageCommentThreadDetail, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return domain.PageCommentThreadDetail{}, fmt.Errorf("begin thread create transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	if firstMessage.ThreadID != thread.ID {
		return domain.PageCommentThreadDetail{}, fmt.Errorf("%w: starter message thread_id must match thread id", domain.ErrValidation)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO page_comment_threads (
			id,
			page_id,
			anchor_type,
			block_id,
			quoted_text,
			quoted_block_text,
			thread_state,
			anchor_state,
			created_by,
			created_at,
			resolved_by,
			resolved_at,
			resolve_note,
			reopened_by,
			reopened_at,
			reopen_reason,
			last_activity_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
	`, thread.ID, thread.PageID, thread.Anchor.Type, thread.Anchor.BlockID, thread.Anchor.QuotedText, thread.Anchor.QuotedBlockText, thread.ThreadState, thread.AnchorState, thread.CreatedBy, thread.CreatedAt, thread.ResolvedBy, thread.ResolvedAt, thread.ResolveNote, thread.ReopenedBy, thread.ReopenedAt, thread.ReopenReason, thread.LastActivityAt); err != nil {
		return domain.PageCommentThreadDetail{}, fmt.Errorf("insert page comment thread: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO page_comment_messages (id, thread_id, body, created_by, created_at)
		VALUES ($1, $2, $3, $4, $5)
	`, firstMessage.ID, firstMessage.ThreadID, firstMessage.Body, firstMessage.CreatedBy, firstMessage.CreatedAt); err != nil {
		return domain.PageCommentThreadDetail{}, fmt.Errorf("insert page comment message: %w", err)
	}

	if err := validateThreadCreateMentions(ctx, tx, thread.PageID, firstMessage.ID, mentions); err != nil {
		return domain.PageCommentThreadDetail{}, err
	}

	for _, mention := range mentions {
		if _, err := tx.Exec(ctx, `
			INSERT INTO page_comment_message_mentions (message_id, mentioned_user_id)
			VALUES ($1, $2)
		`, mention.MessageID, mention.MentionedUserID); err != nil {
			return domain.PageCommentThreadDetail{}, fmt.Errorf("insert page comment message mention: %w", err)
		}
	}

	createdEvent := domain.PageCommentThreadEvent{
		ID:        uuid.NewString(),
		ThreadID:  thread.ID,
		Type:      domain.PageCommentThreadEventTypeCreated,
		ActorID:   &thread.CreatedBy,
		MessageID: &firstMessage.ID,
		CreatedAt: thread.CreatedAt,
	}
	if err := r.insertThreadEvent(ctx, tx, createdEvent); err != nil {
		return domain.PageCommentThreadDetail{}, err
	}

	mentionUserIDs := make([]string, 0, len(mentions))
	for _, mention := range mentions {
		mentionUserIDs = append(mentionUserIDs, mention.MentionedUserID)
	}
	workspaceID, err := loadPageWorkspaceID(ctx, tx, thread.PageID)
	if err != nil {
		return domain.PageCommentThreadDetail{}, err
	}
	if err := domain.ValidateThreadCreatedOutboxEvent(outboxEvent, thread, firstMessage, workspaceID, mentionUserIDs); err != nil {
		return domain.PageCommentThreadDetail{}, fmt.Errorf("validate page comment thread outbox event: %w", err)
	}

	if _, err := insertOutboxEvent(ctx, tx, outboxEvent); err != nil {
		return domain.PageCommentThreadDetail{}, fmt.Errorf("insert page comment thread outbox event: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return domain.PageCommentThreadDetail{}, fmt.Errorf("commit thread create transaction: %w", err)
	}

	return domain.PageCommentThreadDetail{
		Thread:   thread,
		Messages: []domain.PageCommentThreadMessage{firstMessage},
		Events:   []domain.PageCommentThreadEvent{createdEvent},
	}, nil
}

func (r ThreadRepository) GetThread(ctx context.Context, threadID string) (domain.PageCommentThreadDetail, error) {
	query := `
		SELECT
			id,
			page_id,
			anchor_type,
			block_id,
			quoted_text,
			quoted_block_text,
			thread_state,
			anchor_state,
			created_by,
			created_at,
			resolved_by,
			resolved_at,
			resolve_note,
			reopened_by,
			reopened_at,
			reopen_reason,
			last_activity_at
		FROM page_comment_threads
		WHERE id = $1
	`

	var thread domain.PageCommentThread
	if err := r.db.QueryRow(ctx, query, threadID).Scan(
		&thread.ID,
		&thread.PageID,
		&thread.Anchor.Type,
		&thread.Anchor.BlockID,
		&thread.Anchor.QuotedText,
		&thread.Anchor.QuotedBlockText,
		&thread.ThreadState,
		&thread.AnchorState,
		&thread.CreatedBy,
		&thread.CreatedAt,
		&thread.ResolvedBy,
		&thread.ResolvedAt,
		&thread.ResolveNote,
		&thread.ReopenedBy,
		&thread.ReopenedAt,
		&thread.ReopenReason,
		&thread.LastActivityAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.PageCommentThreadDetail{}, domain.ErrNotFound
		}
		return domain.PageCommentThreadDetail{}, fmt.Errorf("select page comment thread by id: %w", err)
	}

	messageRows, err := r.db.Query(ctx, `
		SELECT id, thread_id, body, created_by, created_at
		FROM page_comment_messages
		WHERE thread_id = $1
		ORDER BY created_at ASC, id ASC
	`, threadID)
	if err != nil {
		return domain.PageCommentThreadDetail{}, fmt.Errorf("list page comment thread messages: %w", err)
	}
	defer messageRows.Close()

	messages := make([]domain.PageCommentThreadMessage, 0, 4)
	for messageRows.Next() {
		var message domain.PageCommentThreadMessage
		if err := messageRows.Scan(
			&message.ID,
			&message.ThreadID,
			&message.Body,
			&message.CreatedBy,
			&message.CreatedAt,
		); err != nil {
			return domain.PageCommentThreadDetail{}, fmt.Errorf("scan page comment thread message: %w", err)
		}
		messages = append(messages, message)
	}
	if err := messageRows.Err(); err != nil {
		return domain.PageCommentThreadDetail{}, fmt.Errorf("iterate page comment thread messages: %w", err)
	}

	eventRows, err := r.db.Query(ctx, `
		SELECT
			id,
			thread_id,
			event_type,
			actor_id,
			message_id,
			revision_id,
			from_thread_state,
			to_thread_state,
			from_anchor_state,
			to_anchor_state,
			from_block_id,
			to_block_id,
			reason,
			note,
			created_at
		FROM page_comment_thread_events
		WHERE thread_id = $1
		ORDER BY created_at ASC, id ASC
	`, threadID)
	if err != nil {
		return domain.PageCommentThreadDetail{}, fmt.Errorf("list page comment thread events: %w", err)
	}
	defer eventRows.Close()

	events := make([]domain.PageCommentThreadEvent, 0, 4)
	for eventRows.Next() {
		var event domain.PageCommentThreadEvent
		if err := eventRows.Scan(
			&event.ID,
			&event.ThreadID,
			&event.Type,
			&event.ActorID,
			&event.MessageID,
			&event.RevisionID,
			&event.FromThreadState,
			&event.ToThreadState,
			&event.FromAnchorState,
			&event.ToAnchorState,
			&event.FromBlockID,
			&event.ToBlockID,
			&event.Reason,
			&event.Note,
			&event.CreatedAt,
		); err != nil {
			return domain.PageCommentThreadDetail{}, fmt.Errorf("scan page comment thread event: %w", err)
		}
		events = append(events, event)
	}
	if err := eventRows.Err(); err != nil {
		return domain.PageCommentThreadDetail{}, fmt.Errorf("iterate page comment thread events: %w", err)
	}

	thread.ReplyCount = len(messages)

	return domain.PageCommentThreadDetail{
		Thread:   thread,
		Messages: messages,
		Events:   events,
	}, nil
}

func (r ThreadRepository) ListThreads(ctx context.Context, pageID string, threadState *domain.PageCommentThreadState, anchorState *domain.PageCommentThreadAnchorState, createdBy *string, hasMissingAnchor *bool, hasOutdatedAnchor *bool, sortMode string, query string, limit int, rawCursor string) (domain.PageCommentThreadList, error) {
	var threadStateArg any
	if threadState != nil {
		threadStateArg = string(*threadState)
	}

	var anchorStateArg any
	if anchorState != nil {
		anchorStateArg = string(*anchorState)
	}

	var createdByArg any
	if createdBy != nil {
		createdByArg = *createdBy
	}

	var hasMissingAnchorArg any
	if hasMissingAnchor != nil {
		hasMissingAnchorArg = *hasMissingAnchor
	}

	var hasOutdatedAnchorArg any
	if hasOutdatedAnchor != nil {
		hasOutdatedAnchorArg = *hasOutdatedAnchor
	}

	normalizedSortMode, err := normalizeThreadListSortMode(sortMode)
	if err != nil {
		return domain.PageCommentThreadList{}, fmt.Errorf("list page comment threads: %w", err)
	}
	cursor, err := decodeThreadListCursor(rawCursor, normalizedSortMode)
	if err != nil {
		return domain.PageCommentThreadList{}, fmt.Errorf("list page comment threads: %w", err)
	}

	orderBy := `
		CASE WHEN t.thread_state = 'open' THEN 0 ELSE 1 END,
		t.last_activity_at DESC,
		t.id ASC
	`
	cursorClause := ""
	args := []any{pageID, threadStateArg, anchorStateArg, createdByArg, hasMissingAnchorArg, hasOutdatedAnchorArg, query}
	switch normalizedSortMode {
	case "newest":
		orderBy = `
			t.created_at DESC,
			t.id ASC
		`
		if cursor != nil {
			createdAtPos := len(args) + 1
			idPos := len(args) + 2
			cursorClause = fmt.Sprintf(`
		  AND (
			t.created_at < $%d
			OR (t.created_at = $%d AND t.id > $%d::uuid)
		  )`, createdAtPos, createdAtPos, idPos)
			args = append(args, *cursor.CreatedAt, cursor.ID)
		}
	case "oldest":
		orderBy = `
			t.created_at ASC,
			t.id ASC
		`
		if cursor != nil {
			createdAtPos := len(args) + 1
			idPos := len(args) + 2
			cursorClause = fmt.Sprintf(`
		  AND (
			t.created_at > $%d
			OR (t.created_at = $%d AND t.id > $%d::uuid)
		  )`, createdAtPos, createdAtPos, idPos)
			args = append(args, *cursor.CreatedAt, cursor.ID)
		}
	case "recent_activity":
		if cursor != nil {
			rankPos := len(args) + 1
			lastActivityPos := len(args) + 2
			idPos := len(args) + 3
			cursorClause = fmt.Sprintf(`
		  AND (
			CASE WHEN t.thread_state = 'open' THEN 0 ELSE 1 END > $%d
			OR (
				CASE WHEN t.thread_state = 'open' THEN 0 ELSE 1 END = $%d
				AND (
					t.last_activity_at < $%d
					OR (t.last_activity_at = $%d AND t.id > $%d::uuid)
				)
			)
		  )`, rankPos, rankPos, lastActivityPos, lastActivityPos, idPos)
			args = append(args, threadStateRank(*cursor.ThreadState), *cursor.LastActivityAt, cursor.ID)
		}
	}

	limitClause := ""
	if limit > 0 {
		limitPos := len(args) + 1
		limitClause = fmt.Sprintf(" LIMIT $%d", limitPos)
		args = append(args, limit+1)
	}

	listQuery := fmt.Sprintf(`
		SELECT
			t.id,
			t.page_id,
			t.anchor_type,
			t.block_id,
			t.quoted_text,
			t.quoted_block_text,
			t.thread_state,
			t.anchor_state,
			t.created_by,
			t.created_at,
			t.resolved_by,
			t.resolved_at,
			t.resolve_note,
			t.reopened_by,
			t.reopened_at,
			t.reopen_reason,
			t.last_activity_at,
			COALESCE(message_stats.reply_count, 0) AS reply_count
		FROM page_comment_threads t
		LEFT JOIN LATERAL (
			SELECT COUNT(*)::int AS reply_count
			FROM page_comment_messages m
			WHERE m.thread_id = t.id
		) AS message_stats ON TRUE
		WHERE t.page_id = $1
		  AND ($2::text IS NULL OR t.thread_state = $2)
		  AND ($3::text IS NULL OR t.anchor_state = $3)
		  AND ($4::uuid IS NULL OR t.created_by = $4::uuid)
		  AND (
			$5::bool IS NULL
			OR ($5::bool = TRUE AND t.anchor_state = 'missing')
			OR ($5::bool = FALSE AND t.anchor_state <> 'missing')
		  )
		  AND (
			$6::bool IS NULL
			OR ($6::bool = TRUE AND t.anchor_state = 'outdated')
			OR ($6::bool = FALSE AND t.anchor_state <> 'outdated')
		  )
		  AND (
			$7 = ''
			OR t.quoted_text ILIKE '%%' || $7 || '%%'
			OR t.quoted_block_text ILIKE '%%' || $7 || '%%'
			OR EXISTS (
				SELECT 1
				FROM page_comment_messages m
				WHERE m.thread_id = t.id
				  AND m.body ILIKE '%%' || $7 || '%%'
			)
		  )
		  %s
		ORDER BY %s
		%s
	`, cursorClause, orderBy, limitClause)

	rows, err := r.db.Query(ctx, listQuery, args...)
	if err != nil {
		return domain.PageCommentThreadList{}, fmt.Errorf("list page comment threads: %w", err)
	}
	defer rows.Close()

	threads := make([]domain.PageCommentThread, 0)
	for rows.Next() {
		var thread domain.PageCommentThread
		if err := rows.Scan(
			&thread.ID,
			&thread.PageID,
			&thread.Anchor.Type,
			&thread.Anchor.BlockID,
			&thread.Anchor.QuotedText,
			&thread.Anchor.QuotedBlockText,
			&thread.ThreadState,
			&thread.AnchorState,
			&thread.CreatedBy,
			&thread.CreatedAt,
			&thread.ResolvedBy,
			&thread.ResolvedAt,
			&thread.ResolveNote,
			&thread.ReopenedBy,
			&thread.ReopenedAt,
			&thread.ReopenReason,
			&thread.LastActivityAt,
			&thread.ReplyCount,
		); err != nil {
			return domain.PageCommentThreadList{}, fmt.Errorf("scan page comment thread: %w", err)
		}
		threads = append(threads, thread)
	}
	if err := rows.Err(); err != nil {
		return domain.PageCommentThreadList{}, fmt.Errorf("iterate page comment threads: %w", err)
	}

	hasMore := false
	var nextCursor *string
	if limit > 0 && len(threads) > limit {
		hasMore = true
		cursorToken, err := encodeThreadListCursor(normalizedSortMode, threads[limit-1])
		if err != nil {
			return domain.PageCommentThreadList{}, err
		}
		nextCursor = &cursorToken
		threads = threads[:limit]
	}

	countsQuery := `
		SELECT
			COUNT(*) FILTER (WHERE thread_state = 'open')::int AS open_count,
			COUNT(*) FILTER (WHERE thread_state = 'resolved')::int AS resolved_count,
			COUNT(*) FILTER (WHERE anchor_state = 'active')::int AS active_count,
			COUNT(*) FILTER (WHERE anchor_state = 'outdated')::int AS outdated_count,
			COUNT(*) FILTER (WHERE anchor_state = 'missing')::int AS missing_count
		FROM page_comment_threads
		WHERE page_id = $1
	`

	var counts domain.PageCommentThreadFilterCounts
	if err := r.db.QueryRow(ctx, countsQuery, pageID).Scan(
		&counts.Open,
		&counts.Resolved,
		&counts.Active,
		&counts.Outdated,
		&counts.Missing,
	); err != nil {
		return domain.PageCommentThreadList{}, fmt.Errorf("count page comment threads: %w", err)
	}

	return domain.PageCommentThreadList{
		Threads:    threads,
		Counts:     counts,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}, nil
}

func (r ThreadRepository) ListWorkspaceThreads(ctx context.Context, workspaceID string, threadState *domain.PageCommentThreadState, anchorState *domain.PageCommentThreadAnchorState, createdBy *string, hasMissingAnchor *bool, hasOutdatedAnchor *bool, sortMode string, query string, limit int, rawCursor string) (domain.WorkspaceCommentThreadList, error) {
	var threadStateArg any
	if threadState != nil {
		threadStateArg = string(*threadState)
	}

	var anchorStateArg any
	if anchorState != nil {
		anchorStateArg = string(*anchorState)
	}

	var createdByArg any
	if createdBy != nil {
		createdByArg = *createdBy
	}

	var hasMissingAnchorArg any
	if hasMissingAnchor != nil {
		hasMissingAnchorArg = *hasMissingAnchor
	}

	var hasOutdatedAnchorArg any
	if hasOutdatedAnchor != nil {
		hasOutdatedAnchorArg = *hasOutdatedAnchor
	}

	normalizedSortMode, err := normalizeThreadListSortMode(sortMode)
	if err != nil {
		return domain.WorkspaceCommentThreadList{}, fmt.Errorf("list workspace page comment threads: %w", err)
	}
	cursor, err := decodeThreadListCursor(rawCursor, normalizedSortMode)
	if err != nil {
		return domain.WorkspaceCommentThreadList{}, fmt.Errorf("list workspace page comment threads: %w", err)
	}

	orderBy := `
		CASE WHEN t.thread_state = 'open' THEN 0 ELSE 1 END,
		t.last_activity_at DESC,
		t.id ASC
	`
	cursorClause := ""
	args := []any{workspaceID, threadStateArg, anchorStateArg, createdByArg, hasMissingAnchorArg, hasOutdatedAnchorArg, query}
	switch normalizedSortMode {
	case "newest":
		orderBy = `
			t.created_at DESC,
			t.id ASC
		`
		if cursor != nil {
			createdAtPos := len(args) + 1
			idPos := len(args) + 2
			cursorClause = fmt.Sprintf(`
		  AND (
			t.created_at < $%d
			OR (t.created_at = $%d AND t.id > $%d::uuid)
		  )`, createdAtPos, createdAtPos, idPos)
			args = append(args, *cursor.CreatedAt, cursor.ID)
		}
	case "oldest":
		orderBy = `
			t.created_at ASC,
			t.id ASC
		`
		if cursor != nil {
			createdAtPos := len(args) + 1
			idPos := len(args) + 2
			cursorClause = fmt.Sprintf(`
		  AND (
			t.created_at > $%d
			OR (t.created_at = $%d AND t.id > $%d::uuid)
		  )`, createdAtPos, createdAtPos, idPos)
			args = append(args, *cursor.CreatedAt, cursor.ID)
		}
	case "recent_activity":
		if cursor != nil {
			rankPos := len(args) + 1
			lastActivityPos := len(args) + 2
			idPos := len(args) + 3
			cursorClause = fmt.Sprintf(`
		  AND (
			CASE WHEN t.thread_state = 'open' THEN 0 ELSE 1 END > $%d
			OR (
				CASE WHEN t.thread_state = 'open' THEN 0 ELSE 1 END = $%d
				AND (
					t.last_activity_at < $%d
					OR (t.last_activity_at = $%d AND t.id > $%d::uuid)
				)
			)
		  )`, rankPos, rankPos, lastActivityPos, lastActivityPos, idPos)
			args = append(args, threadStateRank(*cursor.ThreadState), *cursor.LastActivityAt, cursor.ID)
		}
	}

	limitClause := ""
	if limit > 0 {
		limitPos := len(args) + 1
		limitClause = fmt.Sprintf(" LIMIT $%d", limitPos)
		args = append(args, limit+1)
	}

	listQuery := fmt.Sprintf(`
		SELECT
			t.id,
			t.page_id,
			t.anchor_type,
			t.block_id,
			t.quoted_text,
			t.quoted_block_text,
			t.thread_state,
			t.anchor_state,
			t.created_by,
			t.created_at,
			t.resolved_by,
			t.resolved_at,
			t.resolve_note,
			t.reopened_by,
			t.reopened_at,
			t.reopen_reason,
			t.last_activity_at,
			COALESCE(message_stats.reply_count, 0) AS reply_count,
			p.id,
			p.workspace_id,
			p.folder_id,
			p.title,
			p.updated_at
		FROM page_comment_threads t
		JOIN pages p ON p.id = t.page_id
		LEFT JOIN LATERAL (
			SELECT COUNT(*)::int AS reply_count
			FROM page_comment_messages m
			WHERE m.thread_id = t.id
		) AS message_stats ON TRUE
		WHERE p.workspace_id = $1
		  AND p.deleted_at IS NULL
		  AND ($2::text IS NULL OR t.thread_state = $2)
		  AND ($3::text IS NULL OR t.anchor_state = $3)
		  AND ($4::uuid IS NULL OR t.created_by = $4::uuid)
		  AND (
			$5::bool IS NULL
			OR ($5::bool = TRUE AND t.anchor_state = 'missing')
			OR ($5::bool = FALSE AND t.anchor_state <> 'missing')
		  )
		  AND (
			$6::bool IS NULL
			OR ($6::bool = TRUE AND t.anchor_state = 'outdated')
			OR ($6::bool = FALSE AND t.anchor_state <> 'outdated')
		  )
		  AND (
			$7 = ''
			OR p.title ILIKE '%%' || $7 || '%%'
			OR t.quoted_text ILIKE '%%' || $7 || '%%'
			OR t.quoted_block_text ILIKE '%%' || $7 || '%%'
			OR EXISTS (
				SELECT 1
				FROM page_comment_messages m
				WHERE m.thread_id = t.id
				  AND m.body ILIKE '%%' || $7 || '%%'
			)
		  )
		  %s
		ORDER BY %s
		%s
	`, cursorClause, orderBy, limitClause)

	rows, err := r.db.Query(ctx, listQuery, args...)
	if err != nil {
		return domain.WorkspaceCommentThreadList{}, fmt.Errorf("list workspace page comment threads: %w", err)
	}
	defer rows.Close()

	items := make([]domain.WorkspaceCommentThreadListItem, 0)
	for rows.Next() {
		var item domain.WorkspaceCommentThreadListItem
		if err := rows.Scan(
			&item.Thread.ID,
			&item.Thread.PageID,
			&item.Thread.Anchor.Type,
			&item.Thread.Anchor.BlockID,
			&item.Thread.Anchor.QuotedText,
			&item.Thread.Anchor.QuotedBlockText,
			&item.Thread.ThreadState,
			&item.Thread.AnchorState,
			&item.Thread.CreatedBy,
			&item.Thread.CreatedAt,
			&item.Thread.ResolvedBy,
			&item.Thread.ResolvedAt,
			&item.Thread.ResolveNote,
			&item.Thread.ReopenedBy,
			&item.Thread.ReopenedAt,
			&item.Thread.ReopenReason,
			&item.Thread.LastActivityAt,
			&item.Thread.ReplyCount,
			&item.Page.ID,
			&item.Page.WorkspaceID,
			&item.Page.FolderID,
			&item.Page.Title,
			&item.Page.UpdatedAt,
		); err != nil {
			return domain.WorkspaceCommentThreadList{}, fmt.Errorf("scan workspace page comment thread: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return domain.WorkspaceCommentThreadList{}, fmt.Errorf("iterate workspace page comment threads: %w", err)
	}

	hasMore := false
	var nextCursor *string
	if limit > 0 && len(items) > limit {
		hasMore = true
		cursorToken, err := encodeThreadListCursor(normalizedSortMode, items[limit-1].Thread)
		if err != nil {
			return domain.WorkspaceCommentThreadList{}, err
		}
		nextCursor = &cursorToken
		items = items[:limit]
	}

	countsQuery := `
		SELECT
			COUNT(*) FILTER (WHERE t.thread_state = 'open')::int AS open_count,
			COUNT(*) FILTER (WHERE t.thread_state = 'resolved')::int AS resolved_count,
			COUNT(*) FILTER (WHERE t.anchor_state = 'active')::int AS active_count,
			COUNT(*) FILTER (WHERE t.anchor_state = 'outdated')::int AS outdated_count,
			COUNT(*) FILTER (WHERE t.anchor_state = 'missing')::int AS missing_count
		FROM page_comment_threads t
		JOIN pages p ON p.id = t.page_id
		WHERE p.workspace_id = $1
		  AND p.deleted_at IS NULL
	`

	var counts domain.PageCommentThreadFilterCounts
	if err := r.db.QueryRow(ctx, countsQuery, workspaceID).Scan(
		&counts.Open,
		&counts.Resolved,
		&counts.Active,
		&counts.Outdated,
		&counts.Missing,
	); err != nil {
		return domain.WorkspaceCommentThreadList{}, fmt.Errorf("count workspace page comment threads: %w", err)
	}

	return domain.WorkspaceCommentThreadList{
		Threads:    items,
		Counts:     counts,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}, nil
}

func (r ThreadRepository) AddReply(ctx context.Context, threadID string, message domain.PageCommentThreadMessage, mentions []domain.PageCommentMessageMention, updatedThread domain.PageCommentThread, outboxEvent domain.OutboxEvent) (domain.PageCommentThreadDetail, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return domain.PageCommentThreadDetail{}, fmt.Errorf("begin thread reply transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	var currentThreadState domain.PageCommentThreadState
	var pageID string
	var workspaceID string
	var currentResolvedBy *string
	var currentResolvedAt *time.Time
	var currentResolveNote *string
	var currentReopenedBy *string
	var currentReopenedAt *time.Time
	var currentReopenReason *string
	if err := tx.QueryRow(ctx, `
		SELECT
			t.thread_state,
			t.page_id::text,
			p.workspace_id::text,
			t.resolved_by::text,
			t.resolved_at,
			t.resolve_note,
			t.reopened_by::text,
			t.reopened_at,
			t.reopen_reason
		FROM page_comment_threads t
		JOIN pages p ON p.id = t.page_id
		WHERE t.id = $1
		FOR UPDATE OF t
	`, threadID).Scan(&currentThreadState, &pageID, &workspaceID, &currentResolvedBy, &currentResolvedAt, &currentResolveNote, &currentReopenedBy, &currentReopenedAt, &currentReopenReason); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.PageCommentThreadDetail{}, domain.ErrNotFound
		}
		return domain.PageCommentThreadDetail{}, fmt.Errorf("select page comment thread for reply: %w", err)
	}
	if err := domain.ValidateThreadReplyCreatedOutboxEvent(outboxEvent, domain.PageCommentThread{
		ID:     threadID,
		PageID: pageID,
	}, message, workspaceID, mentionUserIDsFromReplyMentions(mentions)); err != nil {
		return domain.PageCommentThreadDetail{}, fmt.Errorf("validate page comment reply outbox event: %w", err)
	}
	if message.ThreadID != threadID {
		return domain.PageCommentThreadDetail{}, fmt.Errorf("%w: reply message thread_id must match target thread id", domain.ErrValidation)
	}

	persistedThread := updatedThread
	if currentThreadState != domain.PageCommentThreadStateResolved {
		persistedThread.ThreadState = currentThreadState
		persistedThread.ResolvedBy = currentResolvedBy
		persistedThread.ResolvedAt = currentResolvedAt
		persistedThread.ResolveNote = currentResolveNote
		persistedThread.ReopenedBy = currentReopenedBy
		persistedThread.ReopenedAt = currentReopenedAt
		persistedThread.ReopenReason = currentReopenReason
	}

	result, err := tx.Exec(ctx, `
		UPDATE page_comment_threads
		SET thread_state = $2,
			resolved_by = $3,
			resolved_at = $4,
			resolve_note = $5,
			reopened_by = $6,
			reopened_at = $7,
			reopen_reason = $8,
			last_activity_at = $9
		WHERE id = $1
	`, threadID, persistedThread.ThreadState, persistedThread.ResolvedBy, persistedThread.ResolvedAt, persistedThread.ResolveNote, persistedThread.ReopenedBy, persistedThread.ReopenedAt, persistedThread.ReopenReason, persistedThread.LastActivityAt)
	if err != nil {
		return domain.PageCommentThreadDetail{}, fmt.Errorf("update page comment thread for reply: %w", err)
	}
	if result.RowsAffected() == 0 {
		return domain.PageCommentThreadDetail{}, domain.ErrNotFound
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO page_comment_messages (id, thread_id, body, created_by, created_at)
		VALUES ($1, $2, $3, $4, $5)
	`, message.ID, message.ThreadID, message.Body, message.CreatedBy, message.CreatedAt); err != nil {
		return domain.PageCommentThreadDetail{}, fmt.Errorf("insert page comment reply: %w", err)
	}

	if err := validateThreadCreateMentions(ctx, tx, pageID, message.ID, mentions); err != nil {
		return domain.PageCommentThreadDetail{}, err
	}

	for _, mention := range mentions {
		if _, err := tx.Exec(ctx, `
			INSERT INTO page_comment_message_mentions (message_id, mentioned_user_id)
			VALUES ($1, $2)
		`, mention.MessageID, mention.MentionedUserID); err != nil {
			return domain.PageCommentThreadDetail{}, fmt.Errorf("insert page comment reply mention: %w", err)
		}
	}

	if currentThreadState == domain.PageCommentThreadStateResolved && persistedThread.ThreadState == domain.PageCommentThreadStateOpen {
		if err := r.insertThreadEvent(ctx, tx, domain.PageCommentThreadEvent{
			ID:              uuid.NewString(),
			ThreadID:        threadID,
			Type:            domain.PageCommentThreadEventTypeReopened,
			ActorID:         persistedThread.ReopenedBy,
			FromThreadState: &currentThreadState,
			ToThreadState:   &persistedThread.ThreadState,
			CreatedAt:       message.CreatedAt,
		}); err != nil {
			return domain.PageCommentThreadDetail{}, err
		}
	}

	if err := r.insertThreadEvent(ctx, tx, domain.PageCommentThreadEvent{
		ID:        uuid.NewString(),
		ThreadID:  threadID,
		Type:      domain.PageCommentThreadEventTypeReplied,
		ActorID:   &message.CreatedBy,
		MessageID: &message.ID,
		CreatedAt: message.CreatedAt,
	}); err != nil {
		return domain.PageCommentThreadDetail{}, err
	}

	if _, err := insertOutboxEvent(ctx, tx, outboxEvent); err != nil {
		return domain.PageCommentThreadDetail{}, fmt.Errorf("insert page comment reply outbox event: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return domain.PageCommentThreadDetail{}, fmt.Errorf("commit thread reply transaction: %w", err)
	}

	return r.GetThread(ctx, threadID)
}

func mentionUserIDsFromReplyMentions(mentions []domain.PageCommentMessageMention) []string {
	if len(mentions) == 0 {
		return []string{}
	}

	mentionUserIDs := make([]string, 0, len(mentions))
	for _, mention := range mentions {
		mentionUserIDs = append(mentionUserIDs, mention.MentionedUserID)
	}
	return mentionUserIDs
}

func (r ThreadRepository) UpdateThreadState(ctx context.Context, threadID string, updatedThread domain.PageCommentThread, reevaluation *domain.ThreadAnchorReevaluationContext) (domain.PageCommentThreadDetail, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return domain.PageCommentThreadDetail{}, fmt.Errorf("begin thread state transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	var currentThreadState domain.PageCommentThreadState
	var currentAnchorState domain.PageCommentThreadAnchorState
	var currentBlockID *string
	if err := tx.QueryRow(ctx, `
		SELECT thread_state, anchor_state, block_id::text
		FROM page_comment_threads
		WHERE id = $1
		FOR UPDATE
	`, threadID).Scan(&currentThreadState, &currentAnchorState, &currentBlockID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.PageCommentThreadDetail{}, domain.ErrNotFound
		}
		return domain.PageCommentThreadDetail{}, fmt.Errorf("select page comment thread for state update: %w", err)
	}

	result, err := tx.Exec(ctx, `
		UPDATE page_comment_threads
		SET anchor_type = $2,
			block_id = $3,
			quoted_text = $4,
			quoted_block_text = $5,
			thread_state = $6,
			anchor_state = $7,
			last_activity_at = $8,
			resolved_by = $9,
			resolved_at = $10,
			resolve_note = $11,
			reopened_by = $12,
			reopened_at = $13,
			reopen_reason = $14
		WHERE id = $1
	`, threadID, updatedThread.Anchor.Type, updatedThread.Anchor.BlockID, updatedThread.Anchor.QuotedText, updatedThread.Anchor.QuotedBlockText, updatedThread.ThreadState, updatedThread.AnchorState, updatedThread.LastActivityAt, updatedThread.ResolvedBy, updatedThread.ResolvedAt, updatedThread.ResolveNote, updatedThread.ReopenedBy, updatedThread.ReopenedAt, updatedThread.ReopenReason)
	if err != nil {
		return domain.PageCommentThreadDetail{}, fmt.Errorf("update page comment thread state: %w", err)
	}
	if result.RowsAffected() == 0 {
		return domain.PageCommentThreadDetail{}, domain.ErrNotFound
	}

	if currentThreadState != updatedThread.ThreadState {
		event := domain.PageCommentThreadEvent{
			ID:              uuid.NewString(),
			ThreadID:        threadID,
			FromThreadState: &currentThreadState,
			ToThreadState:   &updatedThread.ThreadState,
		}
		switch updatedThread.ThreadState {
		case domain.PageCommentThreadStateResolved:
			event.Type = domain.PageCommentThreadEventTypeResolved
			event.ActorID = updatedThread.ResolvedBy
			event.Note = updatedThread.ResolveNote
			event.CreatedAt = threadEventTime(updatedThread.ResolvedAt)
		case domain.PageCommentThreadStateOpen:
			event.Type = domain.PageCommentThreadEventTypeReopened
			event.ActorID = updatedThread.ReopenedBy
			event.Note = updatedThread.ReopenReason
			event.CreatedAt = threadEventTime(updatedThread.ReopenedAt)
		}
		if err := r.insertThreadEvent(ctx, tx, event); err != nil {
			return domain.PageCommentThreadDetail{}, err
		}
	}

	if currentAnchorState != updatedThread.AnchorState {
		now := time.Now().UTC()
		if err := r.insertThreadEvent(ctx, tx, domain.PageCommentThreadEvent{
			ID:              uuid.NewString(),
			ThreadID:        threadID,
			Type:            domain.PageCommentThreadEventTypeAnchorStateChanged,
			FromAnchorState: &currentAnchorState,
			ToAnchorState:   &updatedThread.AnchorState,
			RevisionID:      reevaluationRevisionID(reevaluation),
			Reason:          reevaluationReason(reevaluation),
			CreatedAt:       now,
		}); err != nil {
			return domain.PageCommentThreadDetail{}, err
		}
	}

	blockIDChanged := false
	switch {
	case currentBlockID == nil && updatedThread.Anchor.BlockID == nil:
	case currentBlockID != nil && updatedThread.Anchor.BlockID != nil && *currentBlockID == *updatedThread.Anchor.BlockID:
	default:
		blockIDChanged = true
	}
	if blockIDChanged {
		now := time.Now().UTC()
		if err := r.insertThreadEvent(ctx, tx, domain.PageCommentThreadEvent{
			ID:          uuid.NewString(),
			ThreadID:    threadID,
			Type:        domain.PageCommentThreadEventTypeAnchorRecovered,
			FromBlockID: currentBlockID,
			ToBlockID:   updatedThread.Anchor.BlockID,
			RevisionID:  reevaluationRevisionID(reevaluation),
			Reason:      reevaluationReason(reevaluation),
			CreatedAt:   now,
		}); err != nil {
			return domain.PageCommentThreadDetail{}, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return domain.PageCommentThreadDetail{}, fmt.Errorf("commit thread state transaction: %w", err)
	}

	return r.GetThread(ctx, threadID)
}

func reevaluationReason(reevaluation *domain.ThreadAnchorReevaluationContext) *domain.PageCommentThreadEventReason {
	if reevaluation == nil {
		return nil
	}
	return &reevaluation.Reason
}

func reevaluationRevisionID(reevaluation *domain.ThreadAnchorReevaluationContext) *string {
	if reevaluation == nil {
		return nil
	}
	return reevaluation.RevisionID
}

func validateThreadCreateMentions(ctx context.Context, q dbtx, pageID, messageID string, mentions []domain.PageCommentMessageMention) error {
	if len(mentions) == 0 {
		return nil
	}

	mentionUserIDs := make([]string, 0, len(mentions))
	for _, mention := range mentions {
		if mention.MessageID != messageID {
			return fmt.Errorf("%w: mention message_id must match starter message id", domain.ErrValidation)
		}
		mentionUserID := strings.TrimSpace(mention.MentionedUserID)
		if mentionUserID == "" {
			return fmt.Errorf("%w: mention user id is required", domain.ErrValidation)
		}
		mentionUserIDs = append(mentionUserIDs, mentionUserID)
	}

	allowedRows, err := q.Query(ctx, `
		SELECT m.user_id::text
		FROM workspace_members m
		JOIN pages p ON p.workspace_id = m.workspace_id
		WHERE p.id = $1
		  AND m.user_id::text = ANY($2::text[])
		FOR KEY SHARE OF m
	`, pageID, mentionUserIDs)
	if err != nil {
		return fmt.Errorf("select valid thread mention members: %w", err)
	}
	defer allowedRows.Close()

	allowedUserIDs := make(map[string]struct{}, len(mentionUserIDs))
	for allowedRows.Next() {
		var userID string
		if err := allowedRows.Scan(&userID); err != nil {
			return fmt.Errorf("scan valid thread mention member: %w", err)
		}
		allowedUserIDs[userID] = struct{}{}
	}
	if err := allowedRows.Err(); err != nil {
		return fmt.Errorf("iterate valid thread mention members: %w", err)
	}

	for _, mentionUserID := range mentionUserIDs {
		if _, ok := allowedUserIDs[mentionUserID]; !ok {
			return fmt.Errorf("%w: mention user id must belong to the workspace", domain.ErrValidation)
		}
	}

	return nil
}

func loadPageWorkspaceID(ctx context.Context, q dbtx, pageID string) (string, error) {
	var workspaceID string
	if err := q.QueryRow(ctx, `SELECT workspace_id::text FROM pages WHERE id = $1`, pageID).Scan(&workspaceID); err != nil {
		return "", fmt.Errorf("select page workspace for thread create: %w", err)
	}
	return workspaceID, nil
}
