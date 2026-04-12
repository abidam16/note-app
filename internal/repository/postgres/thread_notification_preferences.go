package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"note-app/internal/domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ThreadNotificationPreferenceRepository struct {
	db *pgxpool.Pool
}

func NewThreadNotificationPreferenceRepository(db *pgxpool.Pool) ThreadNotificationPreferenceRepository {
	return ThreadNotificationPreferenceRepository{db: db}
}

func (r ThreadNotificationPreferenceRepository) GetThreadNotificationPreference(ctx context.Context, threadID, userID string) (*domain.ThreadNotificationPreference, error) {
	if strings.TrimSpace(threadID) == "" {
		return nil, fmt.Errorf("%w: thread_id is required", domain.ErrValidation)
	}
	if strings.TrimSpace(userID) == "" {
		return nil, fmt.Errorf("%w: user_id is required", domain.ErrValidation)
	}

	row := r.db.QueryRow(ctx, `
		SELECT thread_id, user_id, mode, created_at, updated_at
		FROM thread_notification_preferences
		WHERE thread_id = $1 AND user_id = $2
	`, threadID, userID)

	preference, err := scanThreadNotificationPreference(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get thread notification preference: %w", err)
	}
	return &preference, nil
}

func (r ThreadNotificationPreferenceRepository) SetThreadNotificationPreference(ctx context.Context, preference domain.ThreadNotificationPreference) error {
	if strings.TrimSpace(preference.ThreadID) == "" {
		return fmt.Errorf("%w: thread_id is required", domain.ErrValidation)
	}
	if strings.TrimSpace(preference.UserID) == "" {
		return fmt.Errorf("%w: user_id is required", domain.ErrValidation)
	}
	if !domain.IsValidThreadNotificationMode(preference.Mode) {
		return fmt.Errorf("%w: invalid mode", domain.ErrValidation)
	}
	if preference.Mode != domain.ThreadNotificationModeAll && preference.CreatedAt.IsZero() {
		return fmt.Errorf("%w: created_at is required", domain.ErrValidation)
	}
	if preference.UpdatedAt.IsZero() {
		return fmt.Errorf("%w: updated_at is required", domain.ErrValidation)
	}

	if preference.Mode == domain.ThreadNotificationModeAll {
		if _, err := r.db.Exec(ctx, `
			DELETE FROM thread_notification_preferences
			WHERE thread_id = $1 AND user_id = $2
		`, preference.ThreadID, preference.UserID); err != nil {
			return fmt.Errorf("delete thread notification preference: %w", err)
		}
		return nil
	}

	_, err := r.db.Exec(ctx, `
		INSERT INTO thread_notification_preferences (thread_id, user_id, mode, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (thread_id, user_id) DO UPDATE
		SET mode = EXCLUDED.mode,
			updated_at = EXCLUDED.updated_at
	`, preference.ThreadID, preference.UserID, string(preference.Mode), preference.CreatedAt, preference.UpdatedAt)
	if err != nil {
		return fmt.Errorf("set thread notification preference: %w", err)
	}
	return nil
}

func scanThreadNotificationPreference(row notificationScanner) (domain.ThreadNotificationPreference, error) {
	var preference domain.ThreadNotificationPreference
	var mode string
	if err := row.Scan(&preference.ThreadID, &preference.UserID, &mode, &preference.CreatedAt, &preference.UpdatedAt); err != nil {
		return domain.ThreadNotificationPreference{}, err
	}

	if !domain.IsValidThreadNotificationMode(domain.ThreadNotificationMode(mode)) {
		return domain.ThreadNotificationPreference{}, fmt.Errorf("%w: invalid thread notification mode", domain.ErrValidation)
	}
	preference.Mode = domain.ThreadNotificationMode(mode)

	return preference, nil
}
