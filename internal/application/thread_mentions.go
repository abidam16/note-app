package application

import (
	"context"
	"fmt"
	"strings"

	"note-app/internal/domain"
)

const maxThreadMentionUserIDs = 20

func normalizeThreadMentionUserIDs(mentionUserIDs []string) ([]string, error) {
	if len(mentionUserIDs) == 0 {
		return []string{}, nil
	}

	normalized := make([]string, 0, len(mentionUserIDs))
	seen := make(map[string]struct{}, len(mentionUserIDs))
	for _, mentionUserID := range mentionUserIDs {
		trimmed := strings.TrimSpace(mentionUserID)
		if trimmed == "" {
			return nil, fmt.Errorf("%w: mention user id is required", domain.ErrValidation)
		}
		if _, exists := seen[trimmed]; exists {
			continue
		}
		if len(seen) >= maxThreadMentionUserIDs {
			return nil, fmt.Errorf("%w: mentions must contain at most %d unique user ids", domain.ErrValidation, maxThreadMentionUserIDs)
		}
		seen[trimmed] = struct{}{}
		normalized = append(normalized, trimmed)
	}

	return normalized, nil
}

func validateThreadMentionUserIDs(ctx context.Context, memberships WorkspaceMembershipReader, workspaceID string, mentionUserIDs []string) error {
	if len(mentionUserIDs) == 0 {
		return nil
	}
	if memberships == nil {
		return fmt.Errorf("%w: workspace memberships unavailable", domain.ErrValidation)
	}

	members, err := memberships.ListMembers(ctx, workspaceID)
	if err != nil {
		return err
	}

	memberSet := make(map[string]struct{}, len(members))
	for _, member := range members {
		memberSet[member.UserID] = struct{}{}
	}

	for _, mentionUserID := range mentionUserIDs {
		if _, ok := memberSet[mentionUserID]; !ok {
			return fmt.Errorf("%w: mention user id must belong to the workspace", domain.ErrValidation)
		}
	}

	return nil
}

func buildThreadMessageMentions(messageID string, mentionUserIDs []string) []domain.PageCommentMessageMention {
	if len(mentionUserIDs) == 0 {
		return []domain.PageCommentMessageMention{}
	}

	mentions := make([]domain.PageCommentMessageMention, 0, len(mentionUserIDs))
	for _, mentionUserID := range mentionUserIDs {
		mentions = append(mentions, domain.PageCommentMessageMention{
			MessageID:       messageID,
			MentionedUserID: mentionUserID,
		})
	}
	return mentions
}
