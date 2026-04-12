package postgres

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"note-app/internal/domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type NotificationRepository struct {
	db              *pgxpool.Pool
	streamPublisher NotificationStreamPublisher
	logger          *slog.Logger
}

type NotificationStreamPublisher interface {
	Publish(ctx context.Context, signal domain.NotificationStreamSignal) error
}

type unreadDelta struct {
	count     int64
	updatedAt time.Time
}

type notificationInboxCursor struct {
	Status    domain.NotificationInboxStatus `json:"status"`
	Type      domain.NotificationInboxType   `json:"type"`
	CreatedAt time.Time                      `json:"created_at"`
	ID        string                         `json:"id"`
}

func NewNotificationRepository(db *pgxpool.Pool) NotificationRepository {
	return NotificationRepository{db: db}
}

func (r NotificationRepository) WithStreamPublisher(streamPublisher NotificationStreamPublisher) NotificationRepository {
	r.streamPublisher = streamPublisher
	return r
}

func (r NotificationRepository) WithLogger(logger *slog.Logger) NotificationRepository {
	r.logger = logger
	return r
}

func (r NotificationRepository) Create(ctx context.Context, notification domain.Notification) (domain.Notification, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return domain.Notification{}, fmt.Errorf("begin notification create tx: %w", err)
	}
	defer rollbackTx(ctx, tx)

	saved, err := insertNotification(ctx, tx, notification, false)
	if err != nil {
		return domain.Notification{}, err
	}
	if !saved.IsRead {
		if err := incrementUnreadCounter(ctx, tx, saved.UserID, 1, saved.UpdatedAt); err != nil {
			return domain.Notification{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Notification{}, fmt.Errorf("commit notification create tx: %w", err)
	}
	if !saved.IsRead {
		r.publishStreamInvalidation(ctx, saved.UserID)
	}
	return saved, nil
}

func (r NotificationRepository) CreateMany(ctx context.Context, notifications []domain.Notification) error {
	_, err := r.createNotificationBatch(ctx, notifications, nil)
	return err
}

func (r NotificationRepository) CreateCommentNotifications(ctx context.Context, notifications []domain.Notification) (int, error) {
	return r.createNotificationBatch(ctx, notifications, validateCommentNotification)
}

func (r NotificationRepository) CreateMentionNotifications(ctx context.Context, notifications []domain.Notification) (int, error) {
	return r.createNotificationBatch(ctx, notifications, validateMentionNotification)
}

func (r NotificationRepository) CreateCommentAndMentionNotifications(ctx context.Context, commentNotifications, mentionNotifications []domain.Notification) (int, int, error) {
	if len(commentNotifications) == 0 && len(mentionNotifications) == 0 {
		return 0, 0, nil
	}
	for i := range commentNotifications {
		if err := validateCommentNotification(commentNotifications[i]); err != nil {
			return 0, 0, err
		}
	}
	for i := range mentionNotifications {
		if err := validateMentionNotification(mentionNotifications[i]); err != nil {
			return 0, 0, err
		}
	}

	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return 0, 0, fmt.Errorf("begin notifications combined tx: %w", err)
	}
	defer rollbackTx(ctx, tx)

	deltas := make(map[string]unreadDelta, len(commentNotifications)+len(mentionNotifications))
	commentInserted, err := insertNotificationBatch(ctx, tx, commentNotifications, deltas)
	if err != nil {
		return 0, 0, err
	}
	mentionInserted, err := insertNotificationBatch(ctx, tx, mentionNotifications, deltas)
	if err != nil {
		return 0, 0, err
	}

	for userID, delta := range deltas {
		if err := incrementUnreadCounter(ctx, tx, userID, delta.count, delta.updatedAt); err != nil {
			return 0, 0, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, 0, fmt.Errorf("commit notifications combined tx: %w", err)
	}
	r.publishStreamInvalidations(ctx, mapKeys(deltas))
	return commentInserted, mentionInserted, nil
}

func (r NotificationRepository) GetUnreadCount(ctx context.Context, userID string) (int64, error) {
	return r.getUnreadCount(ctx, r.db, userID)
}

func (r NotificationRepository) ListByUserID(ctx context.Context, userID string) ([]domain.Notification, error) {
	query := `
		SELECT
			id, user_id, workspace_id, type, event_id, message, created_at, read_at,
			actor_id, title, content, is_read, actionable, action_kind, resource_type, resource_id, payload, updated_at
		FROM notifications
		WHERE user_id = $1
		ORDER BY created_at DESC, id DESC
	`

	rows, err := r.db.Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("list notifications: %w", err)
	}
	defer rows.Close()

	notifications := make([]domain.Notification, 0)
	for rows.Next() {
		var notification domain.Notification
		if err := scanNotification(rows, &notification); err != nil {
			return nil, fmt.Errorf("scan notification: %w", err)
		}
		notifications = append(notifications, notification)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate notifications: %w", err)
	}

	return notifications, nil
}

func encodeNotificationInboxCursor(filter domain.NotificationInboxFilter, item domain.NotificationInboxItem) (string, error) {
	encoded, err := json.Marshal(notificationInboxCursor{
		Status:    filter.Status,
		Type:      filter.Type,
		CreatedAt: item.CreatedAt.UTC(),
		ID:        item.ID,
	})
	if err != nil {
		return "", fmt.Errorf("marshal notification inbox cursor: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(encoded), nil
}

func decodeNotificationInboxCursor(raw string, filter domain.NotificationInboxFilter) (*notificationInboxCursor, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid cursor", domain.ErrValidation)
	}
	var cursor notificationInboxCursor
	if err := json.Unmarshal(decoded, &cursor); err != nil {
		return nil, fmt.Errorf("%w: invalid cursor", domain.ErrValidation)
	}
	if cursor.CreatedAt.IsZero() || strings.TrimSpace(cursor.ID) == "" || cursor.Status != filter.Status || cursor.Type != filter.Type {
		return nil, fmt.Errorf("%w: invalid cursor", domain.ErrValidation)
	}
	return &cursor, nil
}

func (r NotificationRepository) ListInbox(ctx context.Context, userID string, filter domain.NotificationInboxFilter) (domain.NotificationInboxPage, error) {
	cursor, err := decodeNotificationInboxCursor(filter.Cursor, filter)
	if err != nil {
		return domain.NotificationInboxPage{}, err
	}

	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{
		IsoLevel:   pgx.RepeatableRead,
		AccessMode: pgx.ReadOnly,
	})
	if err != nil {
		return domain.NotificationInboxPage{}, fmt.Errorf("begin notification inbox tx: %w", err)
	}
	defer rollbackTx(ctx, tx)

	args := []any{userID}
	where := []string{"n.user_id = $1"}
	if filter.Status == domain.NotificationInboxStatusRead {
		args = append(args, true)
		where = append(where, fmt.Sprintf("n.is_read = $%d", len(args)))
	} else if filter.Status == domain.NotificationInboxStatusUnread {
		args = append(args, false)
		where = append(where, fmt.Sprintf("n.is_read = $%d", len(args)))
	}
	if filter.Type != domain.NotificationInboxTypeAll {
		args = append(args, string(filter.Type))
		where = append(where, fmt.Sprintf("n.type = $%d", len(args)))
	}
	if cursor != nil {
		args = append(args, cursor.CreatedAt.UTC(), cursor.ID)
		where = append(where, fmt.Sprintf("(n.created_at, n.id) < ($%d, $%d)", len(args)-1, len(args)))
	}
	args = append(args, filter.Limit+1)

	query := fmt.Sprintf(`
		SELECT
			n.id, n.workspace_id, n.type, n.actor_id,
			actor.id, actor.email, actor.full_name,
			n.title, n.content, n.is_read, n.read_at, n.actionable, n.action_kind,
			n.resource_type, n.resource_id, n.payload, n.created_at, n.updated_at
		FROM notifications n
		LEFT JOIN users actor ON actor.id = n.actor_id
		WHERE %s
		ORDER BY n.created_at DESC, n.id DESC
		LIMIT $%d
	`, strings.Join(where, " AND "), len(args))

	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return domain.NotificationInboxPage{}, fmt.Errorf("list notification inbox: %w", err)
	}
	defer rows.Close()

	items := make([]domain.NotificationInboxItem, 0, filter.Limit+1)
	for rows.Next() {
		item, err := scanNotificationInboxItem(rows)
		if err != nil {
			return domain.NotificationInboxPage{}, fmt.Errorf("scan notification inbox item: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return domain.NotificationInboxPage{}, fmt.Errorf("iterate notification inbox: %w", err)
	}

	unreadCount, err := r.getUnreadCount(ctx, tx, userID)
	if err != nil {
		return domain.NotificationInboxPage{}, err
	}

	page := domain.NotificationInboxPage{
		Items:       items,
		UnreadCount: unreadCount,
		HasMore:     false,
	}
	if len(items) > filter.Limit {
		page.HasMore = true
		page.Items = items[:filter.Limit]
		nextCursor, err := encodeNotificationInboxCursor(filter, page.Items[len(page.Items)-1])
		if err != nil {
			return domain.NotificationInboxPage{}, err
		}
		page.NextCursor = &nextCursor
	}

	if err := tx.Commit(ctx); err != nil {
		return domain.NotificationInboxPage{}, fmt.Errorf("commit notification inbox tx: %w", err)
	}

	return page, nil
}

func (r NotificationRepository) MarkRead(ctx context.Context, notificationID, userID string, readAt time.Time) (domain.NotificationInboxItem, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return domain.NotificationInboxItem{}, fmt.Errorf("begin notification mark-read tx: %w", err)
	}
	defer rollbackTx(ctx, tx)

	item, decremented, err := markNotificationRead(ctx, tx, notificationID, userID, readAt)
	if err != nil {
		return domain.NotificationInboxItem{}, err
	}
	if decremented {
		if err := decrementUnreadCounter(ctx, tx, userID, 1, item.UpdatedAt); err != nil {
			return domain.NotificationInboxItem{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.NotificationInboxItem{}, fmt.Errorf("commit notification mark-read tx: %w", err)
	}
	if decremented {
		r.publishStreamInvalidation(ctx, userID)
	}
	return item, nil
}

func (r NotificationRepository) BatchMarkRead(ctx context.Context, userID string, notificationIDs []string, readAt time.Time) (domain.NotificationBatchReadResult, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return domain.NotificationBatchReadResult{}, fmt.Errorf("begin notification batch mark-read tx: %w", err)
	}
	defer rollbackTx(ctx, tx)

	ownedCount, err := countOwnedNotifications(ctx, tx, userID, notificationIDs)
	if err != nil {
		return domain.NotificationBatchReadResult{}, err
	}
	if ownedCount != int64(len(notificationIDs)) {
		return domain.NotificationBatchReadResult{}, domain.ErrNotFound
	}

	updateTag, err := tx.Exec(ctx, `
		UPDATE notifications
		SET read_at = $3,
		    is_read = TRUE,
		    updated_at = $3
		WHERE user_id = $1
		  AND id = ANY($2::uuid[])
		  AND is_read = FALSE
	`, userID, notificationIDs, readAt)
	if err != nil {
		return domain.NotificationBatchReadResult{}, fmt.Errorf("batch mark notifications read: %w", err)
	}

	updatedCount := updateTag.RowsAffected()
	if updatedCount > 0 {
		if err := decrementUnreadCounter(ctx, tx, userID, updatedCount, readAt); err != nil {
			return domain.NotificationBatchReadResult{}, err
		}
	}

	unreadCount, err := r.getUnreadCount(ctx, tx, userID)
	if err != nil {
		return domain.NotificationBatchReadResult{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return domain.NotificationBatchReadResult{}, fmt.Errorf("commit notification batch mark-read tx: %w", err)
	}

	if updatedCount > 0 {
		r.publishStreamInvalidation(ctx, userID)
	}

	return domain.NotificationBatchReadResult{UpdatedCount: updatedCount, UnreadCount: unreadCount}, nil
}

func (r NotificationRepository) UpsertInvitationLive(ctx context.Context, notification domain.Notification) (domain.Notification, error) {
	notification = normalizeNotificationV2(notification)
	invitationResourceType := domain.NotificationResourceTypeInvitation
	notification.Type = domain.NotificationTypeInvitation
	notification.ResourceType = &invitationResourceType
	if notification.ResourceID == nil || *notification.ResourceID == "" {
		if notification.EventID == "" {
			return domain.Notification{}, fmt.Errorf("%w: invitation resource_id is required", domain.ErrValidation)
		}
		resourceID := notification.EventID
		notification.ResourceID = &resourceID
	}
	if notification.EventID == "" {
		notification.EventID = *notification.ResourceID
	}

	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return domain.Notification{}, fmt.Errorf("begin invitation live notification tx: %w", err)
	}
	defer rollbackTx(ctx, tx)

	saved, inserted, err := upsertInvitationLive(ctx, tx, notification)
	if err != nil {
		return domain.Notification{}, err
	}
	if inserted && !saved.IsRead {
		if err := incrementUnreadCounter(ctx, tx, saved.UserID, 1, saved.UpdatedAt); err != nil {
			return domain.Notification{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Notification{}, fmt.Errorf("commit invitation live notification tx: %w", err)
	}
	if inserted || invitationLiveChanged(saved, notification) {
		r.publishStreamInvalidation(ctx, saved.UserID)
	}
	return saved, nil
}

func rollbackTx(ctx context.Context, tx pgx.Tx) {
	_ = tx.Rollback(ctx)
}

func insertNotification(ctx context.Context, db interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, notification domain.Notification, ignoreConflict bool) (domain.Notification, error) {
	query := `
		INSERT INTO notifications (
			id, user_id, workspace_id, type, event_id, message, created_at, read_at,
			actor_id, title, content, is_read, actionable, action_kind, resource_type, resource_id, payload, updated_at
		)
		VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8,
			$9, $10, $11, $12, $13, $14, $15, $16, $17, $18
		)
	`
	if ignoreConflict {
		query += `
		ON CONFLICT (user_id, type, event_id) DO NOTHING
		RETURNING
	`
	} else {
		query += `
		RETURNING
	`
	}
	query += `
			id, user_id, workspace_id, type, event_id, message, created_at, read_at,
			actor_id, title, content, is_read, actionable, action_kind, resource_type, resource_id, payload, updated_at
	`

	notification = normalizeNotificationV2(notification)

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
		if ignoreConflict && errors.Is(err, pgx.ErrNoRows) {
			return domain.Notification{}, domain.ErrNotFound
		}
		if isUniqueViolation(err) {
			return domain.Notification{}, domain.ErrConflict
		}
		return domain.Notification{}, fmt.Errorf("insert notification: %w", err)
	}

	return saved, nil
}

func insertNotificationIfAbsent(ctx context.Context, tx pgx.Tx, notification domain.Notification) (*domain.Notification, error) {
	saved, err := insertNotification(ctx, tx, notification, true)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &saved, nil
}

func (r NotificationRepository) createNotificationBatch(ctx context.Context, notifications []domain.Notification, validator func(domain.Notification) error) (int, error) {
	if len(notifications) == 0 {
		return 0, nil
	}
	if validator != nil {
		for i := range notifications {
			if err := validator(notifications[i]); err != nil {
				return 0, err
			}
		}
	}

	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return 0, fmt.Errorf("begin notifications batch tx: %w", err)
	}
	defer rollbackTx(ctx, tx)

	deltas := make(map[string]unreadDelta, len(notifications))
	insertedCount, err := insertNotificationBatch(ctx, tx, notifications, deltas)
	if err != nil {
		return 0, err
	}

	for userID, delta := range deltas {
		if err := incrementUnreadCounter(ctx, tx, userID, delta.count, delta.updatedAt); err != nil {
			return 0, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit notifications batch tx: %w", err)
	}
	r.publishStreamInvalidations(ctx, mapKeys(deltas))
	return insertedCount, nil
}

func insertNotificationBatch(ctx context.Context, tx pgx.Tx, notifications []domain.Notification, deltas map[string]unreadDelta) (int, error) {
	insertedCount := 0
	for i := range notifications {
		inserted, err := insertNotificationIfAbsent(ctx, tx, notifications[i])
		if err != nil {
			return 0, err
		}
		if inserted == nil || inserted.IsRead {
			continue
		}

		insertedCount++
		delta := deltas[inserted.UserID]
		delta.count++
		if inserted.UpdatedAt.After(delta.updatedAt) {
			delta.updatedAt = inserted.UpdatedAt
		}
		deltas[inserted.UserID] = delta
	}
	return insertedCount, nil
}

func validateCommentNotification(notification domain.Notification) error {
	if strings.TrimSpace(notification.UserID) == "" {
		return fmt.Errorf("%w: user_id is required", domain.ErrValidation)
	}
	if strings.TrimSpace(notification.WorkspaceID) == "" {
		return fmt.Errorf("%w: workspace_id is required", domain.ErrValidation)
	}
	if notification.Type != domain.NotificationTypeComment {
		return fmt.Errorf("%w: comment notifications must have type comment", domain.ErrValidation)
	}
	if strings.TrimSpace(notification.EventID) == "" {
		return fmt.Errorf("%w: event_id is required", domain.ErrValidation)
	}
	if notification.ResourceType == nil || *notification.ResourceType != domain.NotificationResourceTypeThreadMsg {
		return fmt.Errorf("%w: comment notifications must reference thread_message resource type", domain.ErrValidation)
	}
	if notification.ResourceID == nil || strings.TrimSpace(*notification.ResourceID) == "" {
		return fmt.Errorf("%w: comment notifications must reference a resource id", domain.ErrValidation)
	}
	if strings.TrimSpace(*notification.ResourceID) != strings.TrimSpace(notification.EventID) {
		return fmt.Errorf("%w: comment notification resource id must match event_id", domain.ErrValidation)
	}
	if !isJSONObject(notification.Payload) {
		return fmt.Errorf("%w: comment notification payload must be a JSON object", domain.ErrValidation)
	}
	return nil
}

func validateMentionNotification(notification domain.Notification) error {
	if strings.TrimSpace(notification.UserID) == "" {
		return fmt.Errorf("%w: user_id is required", domain.ErrValidation)
	}
	if strings.TrimSpace(notification.WorkspaceID) == "" {
		return fmt.Errorf("%w: workspace_id is required", domain.ErrValidation)
	}
	if notification.Type != domain.NotificationTypeMention {
		return fmt.Errorf("%w: mention notifications must have type mention", domain.ErrValidation)
	}
	if strings.TrimSpace(notification.EventID) == "" {
		return fmt.Errorf("%w: event_id is required", domain.ErrValidation)
	}
	if notification.ResourceType == nil || *notification.ResourceType != domain.NotificationResourceTypeThreadMsg {
		return fmt.Errorf("%w: mention notifications must reference thread_message resource type", domain.ErrValidation)
	}
	if notification.ResourceID == nil || strings.TrimSpace(*notification.ResourceID) == "" {
		return fmt.Errorf("%w: mention notifications must reference a resource id", domain.ErrValidation)
	}
	if strings.TrimSpace(*notification.ResourceID) != strings.TrimSpace(notification.EventID) {
		return fmt.Errorf("%w: mention notification resource id must match event_id", domain.ErrValidation)
	}
	if !isJSONObject(notification.Payload) {
		return fmt.Errorf("%w: mention notification payload must be a JSON object", domain.ErrValidation)
	}
	return nil
}

func countOwnedNotifications(ctx context.Context, tx pgx.Tx, userID string, notificationIDs []string) (int64, error) {
	var ownedCount int64
	if err := tx.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM notifications
		WHERE user_id = $1
		  AND id = ANY($2::uuid[])
	`, userID, notificationIDs).Scan(&ownedCount); err != nil {
		return 0, fmt.Errorf("count batch notification ownership: %w", err)
	}
	return ownedCount, nil
}
func (r NotificationRepository) getUnreadCount(ctx context.Context, db interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, userID string) (int64, error) {
	var unreadCount int64
	if err := db.QueryRow(ctx, `SELECT unread_count FROM notification_unread_counters WHERE user_id = $1`, userID).Scan(&unreadCount); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, nil
		}
		return 0, fmt.Errorf("get unread notification count: %w", err)
	}
	return unreadCount, nil
}

func markNotificationRead(ctx context.Context, tx pgx.Tx, notificationID, userID string, readAt time.Time) (domain.NotificationInboxItem, bool, error) {
	query := `
		WITH existing AS (
			SELECT
				id, workspace_id, type, actor_id, title, content, is_read, read_at,
				actionable, action_kind, resource_type, resource_id, payload, created_at, updated_at
			FROM notifications
			WHERE id = $1
			  AND user_id = $2
			FOR UPDATE
		), updated AS (
			UPDATE notifications
			SET read_at = COALESCE(existing.read_at, $3),
			    is_read = TRUE,
			    updated_at = CASE WHEN existing.is_read THEN existing.updated_at ELSE $3 END
			FROM existing
			WHERE notifications.id = existing.id
			RETURNING
				notifications.id, notifications.workspace_id, notifications.type, notifications.actor_id,
				NOT existing.is_read AS decremented,
				notifications.title, notifications.content, notifications.is_read, notifications.read_at,
				notifications.actionable, notifications.action_kind, notifications.resource_type, notifications.resource_id,
				notifications.payload, notifications.created_at, notifications.updated_at
		)
		SELECT
			u.id, u.workspace_id, u.type, u.actor_id, u.decremented,
			actor.id, actor.email, actor.full_name,
			u.title, u.content, u.is_read, u.read_at, u.actionable, u.action_kind,
			u.resource_type, u.resource_id, u.payload, u.created_at, u.updated_at
		FROM updated u
		LEFT JOIN users actor ON actor.id = u.actor_id
	`

	item, decremented, err := scanNotificationInboxItemWithCounterFlag(tx.QueryRow(ctx, query, notificationID, userID, readAt))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.NotificationInboxItem{}, false, domain.ErrNotFound
		}
		return domain.NotificationInboxItem{}, false, fmt.Errorf("mark notification read: %w", err)
	}
	return item, decremented, nil
}

func scanNotificationInboxItem(row notificationScanner) (domain.NotificationInboxItem, error) {
	var item domain.NotificationInboxItem
	var actorID *string
	var actorUserID *string
	var actorEmail *string
	var actorFullName *string
	var actionKind *string
	var resourceType *string
	var payload []byte
	if err := row.Scan(
		&item.ID,
		&item.WorkspaceID,
		&item.Type,
		&actorID,
		&actorUserID,
		&actorEmail,
		&actorFullName,
		&item.Title,
		&item.Content,
		&item.IsRead,
		&item.ReadAt,
		&item.Actionable,
		&actionKind,
		&resourceType,
		&item.ResourceID,
		&payload,
		&item.CreatedAt,
		&item.UpdatedAt,
	); err != nil {
		return domain.NotificationInboxItem{}, err
	}
	item.ActorID = actorID
	if actorUserID != nil && actorEmail != nil && actorFullName != nil {
		item.Actor = &domain.NotificationActor{
			ID:       *actorUserID,
			Email:    *actorEmail,
			FullName: *actorFullName,
		}
	}
	item.Payload = json.RawMessage(payload)
	if actionKind != nil {
		value := domain.NotificationActionKind(*actionKind)
		item.ActionKind = &value
	}
	if resourceType != nil {
		value := domain.NotificationResourceType(*resourceType)
		item.ResourceType = &value
	}
	return item, nil
}

func scanNotificationInboxItemWithCounterFlag(row notificationScanner) (domain.NotificationInboxItem, bool, error) {
	var item domain.NotificationInboxItem
	var actorID *string
	var actorUserID *string
	var actorEmail *string
	var actorFullName *string
	var actionKind *string
	var resourceType *string
	var payload []byte
	var decremented bool
	if err := row.Scan(
		&item.ID,
		&item.WorkspaceID,
		&item.Type,
		&actorID,
		&decremented,
		&actorUserID,
		&actorEmail,
		&actorFullName,
		&item.Title,
		&item.Content,
		&item.IsRead,
		&item.ReadAt,
		&item.Actionable,
		&actionKind,
		&resourceType,
		&item.ResourceID,
		&payload,
		&item.CreatedAt,
		&item.UpdatedAt,
	); err != nil {
		return domain.NotificationInboxItem{}, false, err
	}
	item.ActorID = actorID
	if actorUserID != nil && actorEmail != nil && actorFullName != nil {
		item.Actor = &domain.NotificationActor{
			ID:       *actorUserID,
			Email:    *actorEmail,
			FullName: *actorFullName,
		}
	}
	item.Payload = json.RawMessage(payload)
	if actionKind != nil {
		value := domain.NotificationActionKind(*actionKind)
		item.ActionKind = &value
	}
	if resourceType != nil {
		value := domain.NotificationResourceType(*resourceType)
		item.ResourceType = &value
	}
	return item, decremented, nil
}

func upsertInvitationLive(ctx context.Context, tx pgx.Tx, notification domain.Notification) (domain.Notification, bool, error) {
	query := `
		INSERT INTO notifications (
			id, user_id, workspace_id, type, event_id, message, created_at, read_at,
			actor_id, title, content, is_read, actionable, action_kind, resource_type, resource_id, payload, updated_at
		)
		VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8,
			$9, $10, $11, $12, $13, $14, $15, $16, $17, $18
		)
		ON CONFLICT (user_id, resource_id)
		WHERE type = 'invitation' AND resource_type = 'invitation'
		DO UPDATE SET
			workspace_id = EXCLUDED.workspace_id,
			actor_id = EXCLUDED.actor_id,
			title = EXCLUDED.title,
			content = EXCLUDED.content,
			message = EXCLUDED.message,
			actionable = EXCLUDED.actionable,
			action_kind = EXCLUDED.action_kind,
			payload = EXCLUDED.payload,
			updated_at = EXCLUDED.updated_at,
			event_id = EXCLUDED.event_id
		RETURNING
			id, user_id, workspace_id, type, event_id, message, created_at, read_at,
			actor_id, title, content, is_read, actionable, action_kind, resource_type, resource_id, payload, updated_at,
			(xmax = 0) AS inserted
	`

	var saved domain.Notification
	var inserted bool
	row := tx.QueryRow(
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
	)
	if err := scanNotificationWithInsertedFlag(row, &saved, &inserted); err != nil {
		return domain.Notification{}, false, fmt.Errorf("upsert invitation live notification: %w", err)
	}

	return saved, inserted, nil
}

func scanNotificationWithInsertedFlag(row notificationScanner, notification *domain.Notification, inserted *bool) error {
	var actionKind *string
	var resourceType *string
	var payload []byte
	if err := row.Scan(
		&notification.ID,
		&notification.UserID,
		&notification.WorkspaceID,
		&notification.Type,
		&notification.EventID,
		&notification.Message,
		&notification.CreatedAt,
		&notification.ReadAt,
		&notification.ActorID,
		&notification.Title,
		&notification.Content,
		&notification.IsRead,
		&notification.Actionable,
		&actionKind,
		&resourceType,
		&notification.ResourceID,
		&payload,
		&notification.UpdatedAt,
		inserted,
	); err != nil {
		return err
	}
	notification.Payload = json.RawMessage(payload)
	if actionKind != nil {
		value := domain.NotificationActionKind(*actionKind)
		notification.ActionKind = &value
	}
	if resourceType != nil {
		value := domain.NotificationResourceType(*resourceType)
		notification.ResourceType = &value
	}
	return nil
}

func incrementUnreadCounter(ctx context.Context, tx pgx.Tx, userID string, delta int64, updatedAt time.Time) error {
	if delta <= 0 {
		return nil
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO notification_unread_counters (user_id, unread_count, created_at, updated_at)
		VALUES ($1, $2, $3, $3)
		ON CONFLICT (user_id) DO UPDATE
		SET unread_count = notification_unread_counters.unread_count + EXCLUDED.unread_count,
		    updated_at = EXCLUDED.updated_at
	`, userID, delta, updatedAt); err != nil {
		return fmt.Errorf("increment unread notification counter: %w", err)
	}
	return nil
}

func decrementUnreadCounter(ctx context.Context, tx pgx.Tx, userID string, delta int64, updatedAt time.Time) error {
	if delta <= 0 {
		return nil
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO notification_unread_counters (user_id, unread_count, created_at, updated_at)
		VALUES ($1, 0, $2, $2)
		ON CONFLICT (user_id) DO UPDATE
		SET unread_count = GREATEST(0, notification_unread_counters.unread_count - $3),
		    updated_at = $2
	`, userID, updatedAt, delta); err != nil {
		return fmt.Errorf("decrement unread notification counter: %w", err)
	}
	return nil
}

type notificationScanner interface {
	Scan(dest ...any) error
}

func scanNotification(row notificationScanner, notification *domain.Notification) error {
	var actionKind *string
	var resourceType *string
	var payload []byte
	if err := row.Scan(
		&notification.ID,
		&notification.UserID,
		&notification.WorkspaceID,
		&notification.Type,
		&notification.EventID,
		&notification.Message,
		&notification.CreatedAt,
		&notification.ReadAt,
		&notification.ActorID,
		&notification.Title,
		&notification.Content,
		&notification.IsRead,
		&notification.Actionable,
		&actionKind,
		&resourceType,
		&notification.ResourceID,
		&payload,
		&notification.UpdatedAt,
	); err != nil {
		return err
	}
	notification.Payload = json.RawMessage(payload)
	if actionKind != nil {
		value := domain.NotificationActionKind(*actionKind)
		notification.ActionKind = &value
	} else {
		notification.ActionKind = nil
	}
	if resourceType != nil {
		value := domain.NotificationResourceType(*resourceType)
		notification.ResourceType = &value
	} else {
		notification.ResourceType = nil
	}
	return nil
}

func normalizeNotificationV2(notification domain.Notification) domain.Notification {
	notification.Title = fallbackNotificationTitle(notification)
	if notification.Content == "" {
		notification.Content = notification.Message
	}
	if len(notification.Payload) == 0 {
		notification.Payload = json.RawMessage(`{}`)
	}
	if notification.UpdatedAt.IsZero() {
		if notification.ReadAt != nil {
			notification.UpdatedAt = *notification.ReadAt
		} else {
			notification.UpdatedAt = notification.CreatedAt
		}
	}
	notification.IsRead = notification.ReadAt != nil
	return notification
}

func mapKeys[T any](values map[string]T) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	return keys
}

func (r NotificationRepository) publishStreamInvalidation(ctx context.Context, userID string) {
	if r.streamPublisher == nil || strings.TrimSpace(userID) == "" {
		return
	}
	signal := domain.NotificationStreamSignal{
		UserID: strings.TrimSpace(userID),
		Reason: domain.NotificationStreamReasonNotificationsChanged,
		SentAt: time.Now().UTC(),
	}
	if err := r.streamPublisher.Publish(ctx, signal); err != nil {
		r.logStreamPublishFailure(userID, err)
	}
}

func (r NotificationRepository) publishStreamInvalidations(ctx context.Context, userIDs []string) {
	if r.streamPublisher == nil {
		return
	}
	seen := make(map[string]struct{}, len(userIDs))
	for _, userID := range userIDs {
		trimmed := strings.TrimSpace(userID)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		r.publishStreamInvalidation(ctx, trimmed)
	}
}

func (r NotificationRepository) logStreamPublishFailure(userID string, err error) {
	if err == nil {
		return
	}
	logger := r.logger
	if logger == nil {
		logger = slog.Default()
	}
	logger.Warn("notification stream publish failed", slog.String("user_id", userID), slog.Any("error", err))
}

func invitationLiveChanged(left, right domain.Notification) bool {
	if left.Type != right.Type || left.EventID != right.EventID || left.UserID != right.UserID || left.WorkspaceID != right.WorkspaceID {
		return true
	}
	if left.Message != right.Message || left.Title != right.Title || left.Content != right.Content || left.Actionable != right.Actionable || left.IsRead != right.IsRead {
		return true
	}
	if !notificationActionKindEqual(left.ActionKind, right.ActionKind) {
		return true
	}
	if !notificationResourceTypeEqual(left.ResourceType, right.ResourceType) {
		return true
	}
	if !notificationStringPtrEqual(left.ResourceID, right.ResourceID) {
		return true
	}
	if !notificationTimePtrEqual(left.ReadAt, right.ReadAt) {
		return true
	}
	if !bytes.Equal(left.Payload, right.Payload) {
		return true
	}
	return false
}

func notificationActionKindEqual(left, right *domain.NotificationActionKind) bool {
	if left == nil && right == nil {
		return true
	}
	if left == nil || right == nil {
		return false
	}
	return *left == *right
}

func notificationResourceTypeEqual(left, right *domain.NotificationResourceType) bool {
	if left == nil && right == nil {
		return true
	}
	if left == nil || right == nil {
		return false
	}
	return *left == *right
}

func notificationStringPtrEqual(left, right *string) bool {
	if left == nil && right == nil {
		return true
	}
	if left == nil || right == nil {
		return false
	}
	return *left == *right
}

func notificationTimePtrEqual(left, right *time.Time) bool {
	if left == nil && right == nil {
		return true
	}
	if left == nil || right == nil {
		return false
	}
	return left.Equal(*right)
}

func fallbackNotificationTitle(notification domain.Notification) string {
	if notification.Title != "" {
		return notification.Title
	}
	switch notification.Type {
	case domain.NotificationTypeInvitation:
		return "Workspace invitation"
	case domain.NotificationTypeComment, domain.NotificationTypeMention:
		return "Comment activity"
	default:
		return "Notification"
	}
}
