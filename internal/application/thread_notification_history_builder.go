package application

import (
	"fmt"
	"slices"
	"strings"

	"note-app/internal/domain"
)

func BuildThreadNotificationHistory(input ThreadNotificationHistoryInput) ([]ThreadNotificationHistoryEntry, error) {
	if strings.TrimSpace(input.Thread.ID) == "" {
		return nil, fmt.Errorf("%w: thread id is required", domain.ErrValidation)
	}

	messages := append([]domain.PageCommentThreadMessage(nil), input.Messages...)
	slices.SortFunc(messages, func(left, right domain.PageCommentThreadMessage) int {
		if left.CreatedAt.Before(right.CreatedAt) {
			return -1
		}
		if left.CreatedAt.After(right.CreatedAt) {
			return 1
		}
		switch {
		case left.ID < right.ID:
			return -1
		case left.ID > right.ID:
			return 1
		default:
			return 0
		}
	})

	memberSet := make(map[string]struct{}, len(input.WorkspaceMemberIDs))
	for _, memberID := range input.WorkspaceMemberIDs {
		if trimmed := strings.TrimSpace(memberID); trimmed != "" {
			memberSet[trimmed] = struct{}{}
		}
	}

	entries := make([]ThreadNotificationHistoryEntry, 0, len(messages))
	priorRepliers := make([]string, 0, len(messages))
	seenRepliers := make(map[string]struct{}, len(messages))

	for _, message := range messages {
		actorID := strings.TrimSpace(message.CreatedBy)
		explicitMentions := normalizeThreadNotificationRecipients(input.ExplicitMentionsByMessageID[message.ID], actorID, memberSet)

		commentRecipients := make([]string, 0, 1+len(priorRepliers)+len(explicitMentions))
		commentSeen := make(map[string]struct{}, 1+len(priorRepliers)+len(explicitMentions))
		appendCommentRecipient := func(candidate string) {
			candidate = strings.TrimSpace(candidate)
			if candidate == "" || candidate == actorID {
				return
			}
			if _, ok := memberSet[candidate]; !ok {
				return
			}
			if _, ok := commentSeen[candidate]; ok {
				return
			}
			commentSeen[candidate] = struct{}{}
			commentRecipients = append(commentRecipients, candidate)
		}

		appendCommentRecipient(input.Thread.CreatedBy)
		for _, priorReplier := range priorRepliers {
			appendCommentRecipient(priorReplier)
		}
		for _, mentionID := range explicitMentions {
			appendCommentRecipient(mentionID)
		}

		entries = append(entries, ThreadNotificationHistoryEntry{
			MessageID:         message.ID,
			CommentRecipients: commentRecipients,
			MentionRecipients: explicitMentions,
		})

		if actorID == "" {
			continue
		}
		if _, ok := seenRepliers[actorID]; ok {
			continue
		}
		seenRepliers[actorID] = struct{}{}
		priorRepliers = append(priorRepliers, actorID)
	}

	return entries, nil
}

func normalizeThreadNotificationRecipients(recipients []string, actorID string, memberSet map[string]struct{}) []string {
	seen := make(map[string]struct{}, len(recipients))
	normalized := make([]string, 0, len(recipients))
	actorID = strings.TrimSpace(actorID)
	for _, recipient := range recipients {
		recipient = strings.TrimSpace(recipient)
		if recipient == "" || recipient == actorID {
			continue
		}
		if _, ok := memberSet[recipient]; !ok {
			continue
		}
		if _, ok := seen[recipient]; ok {
			continue
		}
		seen[recipient] = struct{}{}
		normalized = append(normalized, recipient)
	}
	return normalized
}
