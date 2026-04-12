package postgres

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"note-app/internal/application"
	"note-app/internal/domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const notificationReconciliationLockKey int64 = 764238460127834111

type NotificationReconciliationRepository struct {
	db *pgxpool.Pool
}

type notificationReconciliationLock struct {
	conn *pgxpool.Conn
	key  int64
}

type notificationReconciliationCursor struct {
	CreatedAt time.Time `json:"created_at"`
	ID        string    `json:"id"`
}

func NewNotificationReconciliationRepository(db *pgxpool.Pool) NotificationReconciliationRepository {
	return NotificationReconciliationRepository{db: db}
}

func (r NotificationReconciliationRepository) AcquireReconciliationLock(ctx context.Context) (application.NotificationReconciliationLock, error) {
	conn, err := r.db.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("acquire reconciliation connection: %w", err)
	}

	var acquired bool
	if err := conn.QueryRow(ctx, `SELECT pg_try_advisory_lock($1)`, notificationReconciliationLockKey).Scan(&acquired); err != nil {
		conn.Release()
		return nil, fmt.Errorf("acquire reconciliation advisory lock: %w", err)
	}
	if !acquired {
		conn.Release()
		return nil, fmt.Errorf("reconciliation job is already running")
	}

	return &notificationReconciliationLock{conn: conn, key: notificationReconciliationLockKey}, nil
}

func (l *notificationReconciliationLock) Release(ctx context.Context) error {
	if l == nil || l.conn == nil {
		return nil
	}
	releaseCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := l.conn.Exec(releaseCtx, `SELECT pg_advisory_unlock($1)`, l.key); err != nil {
		hijacked := l.conn.Hijack()
		l.conn = nil
		closeErr := hijacked.Close(context.Background())
		if closeErr != nil {
			return fmt.Errorf("release reconciliation advisory lock: %w (connection close failed: %v)", err, closeErr)
		}
		return fmt.Errorf("release reconciliation advisory lock: %w", err)
	}
	l.conn.Release()
	l.conn = nil
	return nil
}

func (r NotificationReconciliationRepository) ListInvitationSources(ctx context.Context, workspaceID string, cutoff time.Time, limit int, cursor string) (application.NotificationReconciliationInvitationPage, error) {
	decodedCursor, err := decodeNotificationReconciliationCursor(cursor)
	if err != nil {
		return application.NotificationReconciliationInvitationPage{}, err
	}

	args := []any{cutoff.UTC()}
	where := []string{"i.updated_at <= $1"}
	if trimmed := strings.TrimSpace(workspaceID); trimmed != "" {
		args = append(args, trimmed)
		where = append(where, fmt.Sprintf("i.workspace_id = $%d", len(args)))
	}
	if decodedCursor != nil {
		args = append(args, decodedCursor.CreatedAt.UTC(), decodedCursor.ID)
		where = append(where, fmt.Sprintf("(i.created_at, i.id) > ($%d, $%d::uuid)", len(args)-1, len(args)))
	}
	args = append(args, limit+1)

	query := fmt.Sprintf(`
		SELECT
			i.id, i.workspace_id, i.email, i.role, i.invited_by, i.accepted_at, i.created_at, i.status, i.version, i.updated_at,
			i.responded_by, i.responded_at, i.cancelled_by, i.cancelled_at,
			u.id
		FROM workspace_invitations i
		LEFT JOIN users u ON LOWER(u.email) = LOWER(i.email)
		WHERE %s
		ORDER BY i.created_at ASC, i.id ASC
		LIMIT $%d
	`, strings.Join(where, " AND "), len(args))

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return application.NotificationReconciliationInvitationPage{}, fmt.Errorf("list reconciliation invitations: %w", err)
	}
	defer rows.Close()

	items := make([]application.NotificationReconciliationInvitationSource, 0, limit+1)
	for rows.Next() {
		var source application.NotificationReconciliationInvitationSource
		if err := rows.Scan(
			&source.Invitation.ID,
			&source.Invitation.WorkspaceID,
			&source.Invitation.Email,
			&source.Invitation.Role,
			&source.Invitation.InvitedBy,
			&source.Invitation.AcceptedAt,
			&source.Invitation.CreatedAt,
			&source.Invitation.Status,
			&source.Invitation.Version,
			&source.Invitation.UpdatedAt,
			&source.Invitation.RespondedBy,
			&source.Invitation.RespondedAt,
			&source.Invitation.CancelledBy,
			&source.Invitation.CancelledAt,
			&source.RegisteredUserID,
		); err != nil {
			return application.NotificationReconciliationInvitationPage{}, fmt.Errorf("scan reconciliation invitation: %w", err)
		}
		items = append(items, source)
	}
	if err := rows.Err(); err != nil {
		return application.NotificationReconciliationInvitationPage{}, fmt.Errorf("iterate reconciliation invitations: %w", err)
	}

	page := application.NotificationReconciliationInvitationPage{Items: items}
	if len(items) > limit {
		page.HasMore = true
		page.Items = items[:limit]
		nextCursor, err := encodeNotificationReconciliationCursor(page.Items[len(page.Items)-1].Invitation.CreatedAt, page.Items[len(page.Items)-1].Invitation.ID)
		if err != nil {
			return application.NotificationReconciliationInvitationPage{}, err
		}
		page.NextCursor = &nextCursor
	}

	return page, nil
}

func (r NotificationReconciliationRepository) GetInvitationSourceByID(ctx context.Context, invitationID string) (application.NotificationReconciliationInvitationSource, error) {
	var source application.NotificationReconciliationInvitationSource
	if err := r.db.QueryRow(ctx, `
		SELECT
			i.id, i.workspace_id, i.email, i.role, i.invited_by, i.accepted_at, i.created_at, i.status, i.version, i.updated_at,
			i.responded_by, i.responded_at, i.cancelled_by, i.cancelled_at,
			u.id
		FROM workspace_invitations i
		LEFT JOIN users u ON LOWER(u.email) = LOWER(i.email)
		WHERE i.id = $1::uuid
	`, invitationID).Scan(
		&source.Invitation.ID,
		&source.Invitation.WorkspaceID,
		&source.Invitation.Email,
		&source.Invitation.Role,
		&source.Invitation.InvitedBy,
		&source.Invitation.AcceptedAt,
		&source.Invitation.CreatedAt,
		&source.Invitation.Status,
		&source.Invitation.Version,
		&source.Invitation.UpdatedAt,
		&source.Invitation.RespondedBy,
		&source.Invitation.RespondedAt,
		&source.Invitation.CancelledBy,
		&source.Invitation.CancelledAt,
		&source.RegisteredUserID,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return application.NotificationReconciliationInvitationSource{}, domain.ErrNotFound
		}
		return application.NotificationReconciliationInvitationSource{}, fmt.Errorf("get reconciliation invitation source: %w", err)
	}
	return source, nil
}

func (r NotificationReconciliationRepository) ListThreadSources(ctx context.Context, workspaceID string, cutoff time.Time, limit int, cursor string) (application.NotificationReconciliationThreadPage, error) {
	decodedCursor, err := decodeNotificationReconciliationCursor(cursor)
	if err != nil {
		return application.NotificationReconciliationThreadPage{}, err
	}

	args := []any{cutoff.UTC()}
	where := []string{"t.created_at <= $1"}
	if trimmed := strings.TrimSpace(workspaceID); trimmed != "" {
		args = append(args, trimmed)
		where = append(where, fmt.Sprintf("p.workspace_id = $%d", len(args)))
	}
	if decodedCursor != nil {
		args = append(args, decodedCursor.CreatedAt.UTC(), decodedCursor.ID)
		where = append(where, fmt.Sprintf("(t.created_at, t.id) > ($%d, $%d::uuid)", len(args)-1, len(args)))
	}
	args = append(args, limit+1)

	query := fmt.Sprintf(`
		SELECT t.id, t.page_id, t.anchor_type, t.block_id, t.quoted_text, t.quoted_block_text, t.thread_state, t.anchor_state, t.created_by, t.created_at, t.resolved_by, t.resolved_at, t.resolve_note, t.reopened_by, t.reopened_at, t.reopen_reason, t.last_activity_at
		FROM page_comment_threads t
		JOIN pages p ON p.id = t.page_id
		WHERE %s
		ORDER BY t.created_at ASC, t.id ASC
		LIMIT $%d
	`, strings.Join(where, " AND "), len(args))

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return application.NotificationReconciliationThreadPage{}, fmt.Errorf("list reconciliation threads: %w", err)
	}
	defer rows.Close()

	items := make([]domain.PageCommentThread, 0, limit+1)
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
		); err != nil {
			return application.NotificationReconciliationThreadPage{}, fmt.Errorf("scan reconciliation thread: %w", err)
		}
		items = append(items, thread)
	}
	if err := rows.Err(); err != nil {
		return application.NotificationReconciliationThreadPage{}, fmt.Errorf("iterate reconciliation threads: %w", err)
	}

	page := application.NotificationReconciliationThreadPage{Items: items}
	if len(items) > limit {
		page.HasMore = true
		page.Items = items[:limit]
		nextCursor, err := encodeNotificationReconciliationCursor(page.Items[len(page.Items)-1].CreatedAt, page.Items[len(page.Items)-1].ID)
		if err != nil {
			return application.NotificationReconciliationThreadPage{}, err
		}
		page.NextCursor = &nextCursor
	}

	return page, nil
}

func (r NotificationReconciliationRepository) LoadThreadHistory(ctx context.Context, threadID string, cutoff time.Time) (application.ThreadNotificationHistory, error) {
	var history application.ThreadNotificationHistory
	if err := r.db.QueryRow(ctx, `
		SELECT p.workspace_id::text, t.id, t.page_id, t.anchor_type, t.block_id, t.quoted_text, t.quoted_block_text, t.thread_state, t.anchor_state, t.created_by, t.created_at, t.resolved_by, t.resolved_at, t.resolve_note, t.reopened_by, t.reopened_at, t.reopen_reason, t.last_activity_at
		FROM page_comment_threads t
		JOIN pages p ON p.id = t.page_id
		WHERE t.id = $1
	`, threadID).Scan(
		&history.WorkspaceID,
		&history.Thread.ID,
		&history.Thread.PageID,
		&history.Thread.Anchor.Type,
		&history.Thread.Anchor.BlockID,
		&history.Thread.Anchor.QuotedText,
		&history.Thread.Anchor.QuotedBlockText,
		&history.Thread.ThreadState,
		&history.Thread.AnchorState,
		&history.Thread.CreatedBy,
		&history.Thread.CreatedAt,
		&history.Thread.ResolvedBy,
		&history.Thread.ResolvedAt,
		&history.Thread.ResolveNote,
		&history.Thread.ReopenedBy,
		&history.Thread.ReopenedAt,
		&history.Thread.ReopenReason,
		&history.Thread.LastActivityAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return application.ThreadNotificationHistory{}, domain.ErrNotFound
		}
		return application.ThreadNotificationHistory{}, fmt.Errorf("load reconciliation thread: %w", err)
	}

	messageRows, err := r.db.Query(ctx, `
		SELECT id, thread_id, body, created_by, created_at
		FROM page_comment_messages
		WHERE thread_id = $1
		  AND created_at <= $2
		ORDER BY created_at ASC, id ASC
	`, threadID, cutoff.UTC())
	if err != nil {
		return application.ThreadNotificationHistory{}, fmt.Errorf("list reconciliation thread messages: %w", err)
	}
	defer messageRows.Close()

	messages := make([]domain.PageCommentThreadMessage, 0)
	for messageRows.Next() {
		var message domain.PageCommentThreadMessage
		if err := messageRows.Scan(&message.ID, &message.ThreadID, &message.Body, &message.CreatedBy, &message.CreatedAt); err != nil {
			return application.ThreadNotificationHistory{}, fmt.Errorf("scan reconciliation thread message: %w", err)
		}
		messages = append(messages, message)
	}
	if err := messageRows.Err(); err != nil {
		return application.ThreadNotificationHistory{}, fmt.Errorf("iterate reconciliation thread messages: %w", err)
	}

	mentionRows, err := r.db.Query(ctx, `
		SELECT m.message_id::text, m.mentioned_user_id::text, msg.created_at, msg.id::text
		FROM page_comment_message_mentions m
		JOIN page_comment_messages msg ON msg.id = m.message_id
		WHERE msg.thread_id = $1
		  AND msg.created_at <= $2
		ORDER BY msg.created_at ASC, msg.id ASC, m.mentioned_user_id ASC
	`, threadID, cutoff.UTC())
	if err != nil {
		return application.ThreadNotificationHistory{}, fmt.Errorf("list reconciliation thread mentions: %w", err)
	}
	defer mentionRows.Close()

	mentions := map[string][]string{}
	for mentionRows.Next() {
		var messageID string
		var mentionedUserID string
		var _messageCreatedAt time.Time
		var _messageInternalID string
		if err := mentionRows.Scan(&messageID, &mentionedUserID, &_messageCreatedAt, &_messageInternalID); err != nil {
			return application.ThreadNotificationHistory{}, fmt.Errorf("scan reconciliation thread mention: %w", err)
		}
		mentions[messageID] = append(mentions[messageID], mentionedUserID)
	}
	if err := mentionRows.Err(); err != nil {
		return application.ThreadNotificationHistory{}, fmt.Errorf("iterate reconciliation thread mentions: %w", err)
	}

	memberRows, err := r.db.Query(ctx, `
		SELECT wm.user_id::text
		FROM workspace_members wm
		JOIN pages p ON p.workspace_id = wm.workspace_id
		JOIN page_comment_threads t ON t.page_id = p.id
		WHERE t.id = $1
		ORDER BY wm.created_at ASC
	`, threadID)
	if err != nil {
		return application.ThreadNotificationHistory{}, fmt.Errorf("list reconciliation thread members: %w", err)
	}
	defer memberRows.Close()

	members := make([]string, 0)
	for memberRows.Next() {
		var userID string
		if err := memberRows.Scan(&userID); err != nil {
			return application.ThreadNotificationHistory{}, fmt.Errorf("scan reconciliation thread member: %w", err)
		}
		members = append(members, userID)
	}
	if err := memberRows.Err(); err != nil {
		return application.ThreadNotificationHistory{}, fmt.Errorf("iterate reconciliation thread members: %w", err)
	}

	history.Messages = messages
	history.ExplicitMentionsByMessageID = mentions
	history.WorkspaceMemberIDs = members
	return history, nil
}

func (r NotificationReconciliationRepository) ListManagedNotifications(ctx context.Context, workspaceID string, cutoff time.Time, types []domain.NotificationType) ([]domain.Notification, error) {
	if len(types) == 0 {
		return nil, nil
	}

	args := []any{cutoff.UTC()}
	cutoffArg := len(args)
	where := make([]string, 0, 4)
	if trimmed := strings.TrimSpace(workspaceID); trimmed != "" {
		args = append(args, trimmed)
		where = append(where, fmt.Sprintf("n.workspace_id = $%d", len(args)))
	}

	typeClauses := make([]string, 0, len(types))
	for _, typ := range types {
		switch typ {
		case domain.NotificationTypeInvitation:
			typeClauses = append(typeClauses, fmt.Sprintf(`(
				n.type = 'invitation'
				AND n.resource_type = 'invitation'
				AND n.resource_id IS NOT NULL
				AND n.payload ? 'invitation_id'
				AND n.payload ? 'workspace_id'
				AND n.payload ? 'status'
				AND n.payload ? 'version'
				AND (
					NOT EXISTS (
						SELECT 1
						FROM workspace_invitations wi_missing
						WHERE wi_missing.id = n.resource_id
					)
					OR EXISTS (
						SELECT 1
						FROM workspace_invitations wi_visible
						WHERE wi_visible.id = n.resource_id
						  AND wi_visible.updated_at <= $%d
					)
				)
			)`, cutoffArg))
		case domain.NotificationTypeComment, domain.NotificationTypeMention:
			typeClauses = append(typeClauses, fmt.Sprintf("(n.type = '%s' AND n.resource_type = 'thread_message' AND n.payload ? 'thread_id' AND n.payload ? 'message_id' AND n.payload ? 'page_id' AND n.payload ? 'workspace_id')", typ))
		}
	}
	if len(typeClauses) > 0 {
		where = append(where, "("+strings.Join(typeClauses, " OR ")+")")
	}
	where = append(where, fmt.Sprintf("n.created_at <= $%d", cutoffArg))

	query := fmt.Sprintf(`
		SELECT
			n.id, n.user_id, n.workspace_id, n.type, n.event_id, n.message, n.created_at, n.read_at,
			n.actor_id, n.title, n.content, n.is_read, n.actionable, n.action_kind, n.resource_type, n.resource_id, n.payload, n.updated_at
		FROM notifications n
		WHERE %s
		ORDER BY n.created_at ASC, n.id ASC
	`, strings.Join(where, " AND "))

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list reconciliation managed notifications: %w", err)
	}
	defer rows.Close()

	notifications := make([]domain.Notification, 0)
	for rows.Next() {
		var notification domain.Notification
		if err := scanNotification(rows, &notification); err != nil {
			return nil, fmt.Errorf("scan reconciliation managed notification: %w", err)
		}
		notifications = append(notifications, notification)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate reconciliation managed notifications: %w", err)
	}

	return notifications, nil
}

func (r NotificationReconciliationRepository) DeleteManagedNotifications(ctx context.Context, ids []string) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}

	tag, err := r.db.Exec(ctx, `
		DELETE FROM notifications
		WHERE id = ANY($1::uuid[])
		  AND (
			(
				type = 'invitation'
				AND resource_type = 'invitation'
				AND payload ? 'invitation_id'
				AND payload ? 'workspace_id'
				AND payload ? 'status'
				AND payload ? 'version'
			)
			OR
			(
				type IN ('comment', 'mention')
				AND resource_type = 'thread_message'
				AND payload ? 'thread_id'
				AND payload ? 'message_id'
				AND payload ? 'page_id'
				AND payload ? 'workspace_id'
			)
		  )
	`, ids)
	if err != nil {
		return 0, fmt.Errorf("delete reconciliation notifications: %w", err)
	}
	return tag.RowsAffected(), nil
}

func (r NotificationReconciliationRepository) CountUnreadNotifications(ctx context.Context, userID string) (int64, error) {
	var unreadCount int64
	if err := r.db.QueryRow(ctx, `
		SELECT COUNT(*)::bigint
		FROM notifications
		WHERE user_id = $1
		  AND read_at IS NULL
	`, userID).Scan(&unreadCount); err != nil {
		return 0, fmt.Errorf("count reconciliation unread notifications: %w", err)
	}
	return unreadCount, nil
}

func (r NotificationReconciliationRepository) ListCounterStates(ctx context.Context, userIDs []string) ([]application.NotificationReconciliationCounterState, error) {
	if len(userIDs) == 0 {
		return nil, nil
	}

	args := []any{userIDs}
	query := `
		SELECT c.user_id::text, c.unread_count
		FROM notification_unread_counters c
		WHERE c.user_id = ANY($1::uuid[])
	`
	query += ` ORDER BY c.user_id ASC`

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list reconciliation counter states: %w", err)
	}
	defer rows.Close()

	states := make([]application.NotificationReconciliationCounterState, 0)
	for rows.Next() {
		var state application.NotificationReconciliationCounterState
		if err := rows.Scan(&state.UserID, &state.UnreadCount); err != nil {
			return nil, fmt.Errorf("scan reconciliation counter state: %w", err)
		}
		states = append(states, state)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate reconciliation counter states: %w", err)
	}

	return states, nil
}

func (r NotificationReconciliationRepository) UpsertManagedNotification(ctx context.Context, notification domain.Notification) (application.NotificationReconciliationMutationResult, error) {
	notification = normalizeNotificationV2(notification)
	if strings.TrimSpace(notification.UserID) == "" || strings.TrimSpace(notification.WorkspaceID) == "" || strings.TrimSpace(string(notification.Type)) == "" || strings.TrimSpace(notification.EventID) == "" {
		return application.NotificationReconciliationMutationResult{}, fmt.Errorf("%w: reconciliation notification fields are required", domain.ErrValidation)
	}

	switch notification.Type {
	case domain.NotificationTypeInvitation:
		if notification.ResourceType == nil {
			resourceType := domain.NotificationResourceTypeInvitation
			notification.ResourceType = &resourceType
		}
		if notification.ResourceID == nil && notification.EventID != "" {
			resourceID := notification.EventID
			notification.ResourceID = &resourceID
		}
		return upsertInvitationReconciliationNotification(ctx, r.db, notification)
	case domain.NotificationTypeComment, domain.NotificationTypeMention:
		if notification.ResourceType == nil {
			resourceType := domain.NotificationResourceTypeThreadMsg
			notification.ResourceType = &resourceType
		}
		if notification.ResourceID == nil && notification.EventID != "" {
			resourceID := notification.EventID
			notification.ResourceID = &resourceID
		}
		return upsertThreadReconciliationNotification(ctx, r.db, notification)
	default:
		return application.NotificationReconciliationMutationResult{}, fmt.Errorf("%w: unsupported reconciliation notification type %q", domain.ErrValidation, notification.Type)
	}
}

func upsertInvitationReconciliationNotification(ctx context.Context, db interface {
	QueryRow(context.Context, string, ...any) pgx.Row
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}, notification domain.Notification) (application.NotificationReconciliationMutationResult, error) {
	inserted, _, err := insertReconciliationNotification(ctx, db, notification)
	if err != nil {
		return application.NotificationReconciliationMutationResult{}, err
	}
	if inserted {
		return application.NotificationReconciliationMutationResult{Notification: notification, Changed: true, Inserted: true}, nil
	}

	saved, changed, err := updateInvitationReconciliationNotification(ctx, db, notification)
	if err != nil {
		return application.NotificationReconciliationMutationResult{}, err
	}
	if changed {
		return application.NotificationReconciliationMutationResult{Notification: saved, Changed: true, Updated: true}, nil
	}
	existing, found, err := loadInvitationReconciliationNotificationByIdentity(ctx, db, notification)
	if err != nil {
		return application.NotificationReconciliationMutationResult{}, err
	}
	if found && invitationNotificationNeedsRepair(existing, notification) {
		return application.NotificationReconciliationMutationResult{}, fmt.Errorf("%w: invitation reconciliation blocked by existing notification %s", domain.ErrConflict, existing.ID)
	}
	return application.NotificationReconciliationMutationResult{}, nil
}

func upsertThreadReconciliationNotification(ctx context.Context, db interface {
	QueryRow(context.Context, string, ...any) pgx.Row
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}, notification domain.Notification) (application.NotificationReconciliationMutationResult, error) {
	inserted, _, err := insertReconciliationNotification(ctx, db, notification)
	if err != nil {
		return application.NotificationReconciliationMutationResult{}, err
	}
	if inserted {
		return application.NotificationReconciliationMutationResult{Notification: notification, Changed: true, Inserted: true}, nil
	}

	saved, changed, err := updateThreadReconciliationNotification(ctx, db, notification)
	if err != nil {
		return application.NotificationReconciliationMutationResult{}, err
	}
	if changed {
		return application.NotificationReconciliationMutationResult{Notification: saved, Changed: true, Updated: true}, nil
	}
	existing, found, err := loadThreadReconciliationNotificationByIdentity(ctx, db, notification)
	if err != nil {
		return application.NotificationReconciliationMutationResult{}, err
	}
	if found && threadNotificationNeedsRepair(existing, notification) {
		return application.NotificationReconciliationMutationResult{}, fmt.Errorf("%w: thread reconciliation blocked by existing notification %s", domain.ErrConflict, existing.ID)
	}
	return application.NotificationReconciliationMutationResult{}, nil
}

func loadInvitationReconciliationNotificationByIdentity(ctx context.Context, db interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, notification domain.Notification) (domain.Notification, bool, error) {
	if notification.ResourceID == nil && strings.TrimSpace(notification.ID) == "" {
		return domain.Notification{}, false, nil
	}

	resourceID := notification.ID
	if notification.ResourceID != nil && strings.TrimSpace(*notification.ResourceID) != "" {
		resourceID = strings.TrimSpace(*notification.ResourceID)
	}

	var existing domain.Notification
	if err := scanNotification(db.QueryRow(ctx, `
		SELECT
			id, user_id, workspace_id, type, event_id, message, created_at, read_at,
			actor_id, title, content, is_read, actionable, action_kind, resource_type, resource_id, payload, updated_at
		FROM notifications
		WHERE id = $1::uuid
		   OR (type = 'invitation' AND resource_type = 'invitation' AND resource_id = $1::uuid)
		ORDER BY created_at ASC, id ASC
		LIMIT 1
	`, resourceID), &existing); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Notification{}, false, nil
		}
		return domain.Notification{}, false, fmt.Errorf("load invitation reconciliation notification: %w", err)
	}
	return existing, true, nil
}

func loadThreadReconciliationNotificationByIdentity(ctx context.Context, db interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, notification domain.Notification) (domain.Notification, bool, error) {
	var existing domain.Notification
	if err := scanNotification(db.QueryRow(ctx, `
		SELECT
			id, user_id, workspace_id, type, event_id, message, created_at, read_at,
			actor_id, title, content, is_read, actionable, action_kind, resource_type, resource_id, payload, updated_at
		FROM notifications
		WHERE user_id = $1
		  AND type = $2
		  AND event_id = $3
		ORDER BY created_at ASC, id ASC
		LIMIT 1
	`, notification.UserID, notification.Type, notification.EventID), &existing); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Notification{}, false, nil
		}
		return domain.Notification{}, false, fmt.Errorf("load thread reconciliation notification: %w", err)
	}
	return existing, true, nil
}

func invitationNotificationNeedsRepair(existing, expected domain.Notification) bool {
	if existing.UserID != expected.UserID || existing.WorkspaceID != expected.WorkspaceID || existing.Type != expected.Type || existing.EventID != expected.EventID || existing.Message != expected.Message {
		return true
	}
	if existing.Title != expected.Title || existing.Content != expected.Content || existing.Actionable != expected.Actionable {
		return true
	}
	if !notificationStringPtrEqual(existing.ActorID, expected.ActorID) {
		return true
	}
	if !notificationActionKindEqual(existing.ActionKind, expected.ActionKind) {
		return true
	}
	if !notificationResourceTypeEqual(existing.ResourceType, expected.ResourceType) {
		return true
	}
	if !notificationStringPtrEqual(existing.ResourceID, expected.ResourceID) {
		return true
	}
	if !jsonRawMessageEqual(existing.Payload, expected.Payload) {
		return true
	}
	return !existing.UpdatedAt.Equal(expected.UpdatedAt)
}

func threadNotificationNeedsRepair(existing, expected domain.Notification) bool {
	if existing.UserID != expected.UserID || existing.WorkspaceID != expected.WorkspaceID || existing.Type != expected.Type || existing.EventID != expected.EventID || existing.Message != expected.Message {
		return true
	}
	if existing.Title != expected.Title || existing.Content != expected.Content || existing.Actionable != expected.Actionable {
		return true
	}
	if !notificationStringPtrEqual(existing.ActorID, expected.ActorID) {
		return true
	}
	if !notificationActionKindEqual(existing.ActionKind, expected.ActionKind) {
		return true
	}
	if !notificationResourceTypeEqual(existing.ResourceType, expected.ResourceType) {
		return true
	}
	if !notificationStringPtrEqual(existing.ResourceID, expected.ResourceID) {
		return true
	}
	return !jsonRawMessageEqual(existing.Payload, expected.Payload)
}

func jsonRawMessageEqual(left, right []byte) bool {
	return strings.TrimSpace(string(left)) == strings.TrimSpace(string(right))
}

func insertReconciliationNotification(ctx context.Context, db interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, notification domain.Notification) (bool, domain.Notification, error) {
	query := `
		INSERT INTO notifications (
			id, user_id, workspace_id, type, event_id, message, created_at, read_at,
			actor_id, title, content, is_read, actionable, action_kind, resource_type, resource_id, payload, updated_at
		)
		VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8,
			$9, $10, $11, $12, $13, $14, $15, $16, $17, $18
		)
		ON CONFLICT DO NOTHING
		RETURNING
			id, user_id, workspace_id, type, event_id, message, created_at, read_at,
			actor_id, title, content, is_read, actionable, action_kind, resource_type, resource_id, payload, updated_at
	`

	var saved domain.Notification
	if err := scanNotification(db.QueryRow(
		ctx,
		query,
		notification.ID,
		notification.UserID,
		notification.WorkspaceID,
		notification.Type,
		notification.EventID,
		notification.Message,
		notification.CreatedAt,
		notification.ReadAt,
		notification.ActorID,
		notification.Title,
		notification.Content,
		notification.IsRead,
		notification.Actionable,
		notification.ActionKind,
		notification.ResourceType,
		notification.ResourceID,
		notification.Payload,
		notification.UpdatedAt,
	), &saved); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, domain.Notification{}, nil
		}
		return false, domain.Notification{}, fmt.Errorf("insert reconciliation notification: %w", err)
	}
	return true, saved, nil
}

func updateInvitationReconciliationNotification(ctx context.Context, db interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, notification domain.Notification) (domain.Notification, bool, error) {
	query := `
		UPDATE notifications
		SET user_id = $1,
		    created_at = $2,
		    workspace_id = $3,
		    actor_id = $4,
		    title = $5,
		    content = $6,
		    message = $7,
		    actionable = $8,
		    action_kind = $9,
		    resource_type = $10,
		    resource_id = $11,
		    payload = $12,
		    updated_at = $13,
		    event_id = $14
		WHERE type = 'invitation'
		  AND resource_type = 'invitation'
		  AND (id = $15::uuid OR resource_id = $11)
		  AND (
			user_id IS DISTINCT FROM $1
			OR
			created_at IS DISTINCT FROM $2
			OR workspace_id IS DISTINCT FROM $3
			OR actor_id IS DISTINCT FROM $4
			OR title IS DISTINCT FROM $5
			OR content IS DISTINCT FROM $6
			OR message IS DISTINCT FROM $7
			OR actionable IS DISTINCT FROM $8
			OR action_kind IS DISTINCT FROM $9
			OR resource_type IS DISTINCT FROM $10
			OR resource_id IS DISTINCT FROM $11
			OR payload IS DISTINCT FROM $12
			OR updated_at IS DISTINCT FROM $13
			OR event_id IS DISTINCT FROM $14
		  )
		RETURNING
			id, user_id, workspace_id, type, event_id, message, created_at, read_at,
			actor_id, title, content, is_read, actionable, action_kind, resource_type, resource_id, payload, updated_at
	`

	var saved domain.Notification
	if err := scanNotification(db.QueryRow(
		ctx,
		query,
		notification.UserID,
		notification.CreatedAt,
		notification.WorkspaceID,
		notification.ActorID,
		notification.Title,
		notification.Content,
		notification.Message,
		notification.Actionable,
		notification.ActionKind,
		notification.ResourceType,
		notification.ResourceID,
		notification.Payload,
		notification.UpdatedAt,
		notification.EventID,
		notification.ID,
	), &saved); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Notification{}, false, nil
		}
		return domain.Notification{}, false, fmt.Errorf("update invitation reconciliation notification: %w", err)
	}
	return saved, true, nil
}

func updateThreadReconciliationNotification(ctx context.Context, db interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, notification domain.Notification) (domain.Notification, bool, error) {
	query := `
		UPDATE notifications
		SET workspace_id = $3,
		    actor_id = $4,
		    title = $5,
		    content = $6,
		    message = $7,
		    actionable = $8,
		    action_kind = $9,
		    resource_type = $10,
		    resource_id = $11,
		    payload = $12,
		    updated_at = $13
		WHERE user_id = $1
		  AND type = $2
		  AND resource_type = 'thread_message'
		  AND event_id = $14
		  AND payload ? 'thread_id'
		  AND payload ? 'message_id'
		  AND payload ? 'page_id'
		  AND payload ? 'workspace_id'
		  AND (
			workspace_id IS DISTINCT FROM $3
			OR actor_id IS DISTINCT FROM $4
			OR title IS DISTINCT FROM $5
			OR content IS DISTINCT FROM $6
			OR message IS DISTINCT FROM $7
			OR actionable IS DISTINCT FROM $8
			OR action_kind IS DISTINCT FROM $9
			OR resource_type IS DISTINCT FROM $10
			OR resource_id IS DISTINCT FROM $11
			OR payload IS DISTINCT FROM $12
			OR updated_at IS DISTINCT FROM $13
		)
		RETURNING
			id, user_id, workspace_id, type, event_id, message, created_at, read_at,
			actor_id, title, content, is_read, actionable, action_kind, resource_type, resource_id, payload, updated_at
	`

	var saved domain.Notification
	if err := scanNotification(db.QueryRow(
		ctx,
		query,
		notification.UserID,
		notification.Type,
		notification.WorkspaceID,
		notification.ActorID,
		notification.Title,
		notification.Content,
		notification.Message,
		notification.Actionable,
		notification.ActionKind,
		notification.ResourceType,
		notification.ResourceID,
		notification.Payload,
		notification.UpdatedAt,
		notification.EventID,
	), &saved); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Notification{}, false, nil
		}
		return domain.Notification{}, false, fmt.Errorf("update thread reconciliation notification: %w", err)
	}
	return saved, true, nil
}

func (r NotificationReconciliationRepository) UpsertUnreadCounter(ctx context.Context, userID string, unreadCount int64, updatedAt time.Time) (bool, error) {
	if unreadCount <= 0 {
		return false, nil
	}
	tag, err := r.db.Exec(ctx, `
		INSERT INTO notification_unread_counters (user_id, unread_count, created_at, updated_at)
		VALUES ($1, $2, $3, $3)
		ON CONFLICT (user_id) DO UPDATE
		SET unread_count = EXCLUDED.unread_count,
		    updated_at = EXCLUDED.updated_at
		WHERE notification_unread_counters.unread_count IS DISTINCT FROM EXCLUDED.unread_count
	`, userID, unreadCount, updatedAt.UTC())
	if err != nil {
		return false, fmt.Errorf("upsert reconciliation unread counter: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

func (r NotificationReconciliationRepository) DeleteUnreadCounter(ctx context.Context, userID string) (bool, error) {
	tag, err := r.db.Exec(ctx, `
		DELETE FROM notification_unread_counters
		WHERE user_id = $1
	`, userID)
	if err != nil {
		return false, fmt.Errorf("delete reconciliation unread counter: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

func decodeNotificationReconciliationCursor(raw string) (*notificationReconciliationCursor, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid cursor", domain.ErrValidation)
	}
	var cursor notificationReconciliationCursor
	if err := json.Unmarshal(decoded, &cursor); err != nil {
		return nil, fmt.Errorf("%w: invalid cursor", domain.ErrValidation)
	}
	if cursor.CreatedAt.IsZero() || strings.TrimSpace(cursor.ID) == "" {
		return nil, fmt.Errorf("%w: invalid cursor", domain.ErrValidation)
	}
	return &cursor, nil
}

func encodeNotificationReconciliationCursor(createdAt time.Time, id string) (string, error) {
	encoded, err := json.Marshal(notificationReconciliationCursor{
		CreatedAt: createdAt.UTC(),
		ID:        id,
	})
	if err != nil {
		return "", fmt.Errorf("marshal reconciliation cursor: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(encoded), nil
}
