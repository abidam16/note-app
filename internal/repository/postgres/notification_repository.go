package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"note-app/internal/domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type NotificationRepository struct {
	db *pgxpool.Pool
}

func NewNotificationRepository(db *pgxpool.Pool) NotificationRepository {
	return NotificationRepository{db: db}
}

func (r NotificationRepository) Create(ctx context.Context, notification domain.Notification) (domain.Notification, error) {
	query := `
		INSERT INTO notifications (id, user_id, workspace_id, type, event_id, message, created_at, read_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, user_id, workspace_id, type, event_id, message, created_at, read_at
	`

	var saved domain.Notification
	if err := r.db.QueryRow(
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
	).Scan(
		&saved.ID,
		&saved.UserID,
		&saved.WorkspaceID,
		&saved.Type,
		&saved.EventID,
		&saved.Message,
		&saved.CreatedAt,
		&saved.ReadAt,
	); err != nil {
		if isUniqueViolation(err) {
			return domain.Notification{}, domain.ErrConflict
		}
		return domain.Notification{}, fmt.Errorf("insert notification: %w", err)
	}

	return saved, nil
}

func (r NotificationRepository) ListByUserID(ctx context.Context, userID string) ([]domain.Notification, error) {
	query := `
		SELECT id, user_id, workspace_id, type, event_id, message, created_at, read_at
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
		if err := rows.Scan(
			&notification.ID,
			&notification.UserID,
			&notification.WorkspaceID,
			&notification.Type,
			&notification.EventID,
			&notification.Message,
			&notification.CreatedAt,
			&notification.ReadAt,
		); err != nil {
			return nil, fmt.Errorf("scan notification: %w", err)
		}
		notifications = append(notifications, notification)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate notifications: %w", err)
	}

	return notifications, nil
}

func (r NotificationRepository) MarkRead(ctx context.Context, notificationID, userID string, readAt time.Time) (domain.Notification, error) {
	query := `
		UPDATE notifications
		SET read_at = COALESCE(read_at, $3)
		WHERE id = $1
		  AND user_id = $2
		RETURNING id, user_id, workspace_id, type, event_id, message, created_at, read_at
	`

	var notification domain.Notification
	if err := r.db.QueryRow(ctx, query, notificationID, userID, readAt).Scan(
		&notification.ID,
		&notification.UserID,
		&notification.WorkspaceID,
		&notification.Type,
		&notification.EventID,
		&notification.Message,
		&notification.CreatedAt,
		&notification.ReadAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Notification{}, domain.ErrNotFound
		}
		return domain.Notification{}, fmt.Errorf("mark notification read: %w", err)
	}

	return notification, nil
}
