package application

import (
	"context"
	"fmt"
	"strings"
	"time"

	"note-app/internal/domain"
)

type GetThreadNotificationPreferenceInput struct {
	ThreadID string
}

func (s ThreadService) GetNotificationPreference(ctx context.Context, actorID string, input GetThreadNotificationPreferenceInput) (domain.ThreadNotificationPreferenceView, error) {
	detail, _, err := s.threadDetailWithMembership(ctx, actorID, input.ThreadID)
	if err != nil {
		return domain.ThreadNotificationPreferenceView{}, err
	}

	view := domain.ThreadNotificationPreferenceView{
		ThreadID: detail.Thread.ID,
		Mode:     domain.ThreadNotificationModeAll,
	}
	if s.preferences == nil {
		return view, nil
	}

	preference, err := s.preferences.GetThreadNotificationPreference(ctx, detail.Thread.ID, actorID)
	if err != nil {
		return domain.ThreadNotificationPreferenceView{}, fmt.Errorf("get thread notification preference: %w", err)
	}
	if preference != nil && preference.Mode != "" {
		view.Mode = preference.Mode
	}

	return view, nil
}

type UpdateThreadNotificationPreferenceInput struct {
	ThreadID string
	Mode     string
}

func (s ThreadService) UpdateNotificationPreference(ctx context.Context, actorID string, input UpdateThreadNotificationPreferenceInput) (domain.ThreadNotificationPreferenceUpdateResult, error) {
	detail, _, err := s.threadDetailWithMembership(ctx, actorID, input.ThreadID)
	if err != nil {
		return domain.ThreadNotificationPreferenceUpdateResult{}, err
	}

	mode, err := normalizeThreadNotificationMode(input.Mode)
	if err != nil {
		return domain.ThreadNotificationPreferenceUpdateResult{}, err
	}

	if s.preferences == nil {
		return domain.ThreadNotificationPreferenceUpdateResult{}, fmt.Errorf("thread notification preferences not configured")
	}

	now := time.Now().UTC()
	preference := domain.ThreadNotificationPreference{
		ThreadID:  detail.Thread.ID,
		UserID:    actorID,
		Mode:      mode,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := s.preferences.SetThreadNotificationPreference(ctx, preference); err != nil {
		return domain.ThreadNotificationPreferenceUpdateResult{}, fmt.Errorf("set thread notification preference: %w", err)
	}

	return domain.ThreadNotificationPreferenceUpdateResult{
		ThreadID:  detail.Thread.ID,
		Mode:      mode,
		UpdatedAt: now,
	}, nil
}

func normalizeThreadNotificationMode(rawMode string) (domain.ThreadNotificationMode, error) {
	mode := domain.ThreadNotificationMode(strings.TrimSpace(rawMode))
	if mode == "" {
		return "", fmt.Errorf("%w: mode is required", domain.ErrValidation)
	}
	if !domain.IsValidThreadNotificationMode(mode) {
		return "", fmt.Errorf("%w: invalid mode", domain.ErrValidation)
	}
	return mode, nil
}
