package application

import (
	"context"
	"strings"

	"note-app/internal/domain"
)

type ResolveThreadNotificationRecipientsInput struct {
	WorkspaceID            string
	ActorID                string
	Detail                 domain.PageCommentThreadDetail
	ExplicitMentionUserIDs []string
}

type ThreadNotificationRecipientResolver interface {
	ResolveRecipients(ctx context.Context, input ResolveThreadNotificationRecipientsInput) ([]string, error)
}

type threadNotificationRecipientResolver struct {
	memberships WorkspaceMembershipReader
}

func NewThreadNotificationRecipientResolver(memberships WorkspaceMembershipReader) ThreadNotificationRecipientResolver {
	return threadNotificationRecipientResolver{memberships: memberships}
}

func (r threadNotificationRecipientResolver) ResolveRecipients(ctx context.Context, input ResolveThreadNotificationRecipientsInput) ([]string, error) {
	if strings.TrimSpace(input.WorkspaceID) == "" || strings.TrimSpace(input.ActorID) == "" || strings.TrimSpace(input.Detail.Thread.ID) == "" {
		return nil, domain.ErrValidation
	}

	members, err := r.memberships.ListMembers(ctx, input.WorkspaceID)
	if err != nil {
		return nil, err
	}

	activeMembers := make(map[string]struct{}, len(members))
	for _, member := range members {
		if userID := strings.TrimSpace(member.UserID); userID != "" {
			activeMembers[userID] = struct{}{}
		}
	}

	seen := make(map[string]struct{})
	recipients := make([]string, 0)
	appendCandidate := func(candidate string) {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" || candidate == input.ActorID {
			return
		}
		if _, ok := activeMembers[candidate]; !ok {
			return
		}
		if _, ok := seen[candidate]; ok {
			return
		}
		seen[candidate] = struct{}{}
		recipients = append(recipients, candidate)
	}

	appendCandidate(input.Detail.Thread.CreatedBy)
	for _, message := range input.Detail.Messages {
		appendCandidate(message.CreatedBy)
	}
	for _, mentionUserID := range input.ExplicitMentionUserIDs {
		appendCandidate(mentionUserID)
	}

	return recipients, nil
}
