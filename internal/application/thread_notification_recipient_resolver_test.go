package application

import (
	"context"
	"errors"
	"testing"

	"note-app/internal/domain"
)

type threadNotificationMembershipStub struct {
	members []domain.WorkspaceMember
	err     error
}

func (s threadNotificationMembershipStub) GetMembershipByUserID(context.Context, string, string) (domain.WorkspaceMember, error) {
	return domain.WorkspaceMember{}, nil
}

func (s threadNotificationMembershipStub) ListMembers(context.Context, string) ([]domain.WorkspaceMember, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.members, nil
}

func TestThreadNotificationRecipientResolverResolvesRelevantRecipientsInDeterministicOrder(t *testing.T) {
	resolver := NewThreadNotificationRecipientResolver(threadNotificationMembershipStub{
		members: []domain.WorkspaceMember{
			{UserID: "creator"},
			{UserID: "prior-1"},
			{UserID: "prior-2"},
			{UserID: "mention-1"},
		},
	})

	detail := domain.PageCommentThreadDetail{
		Thread: domain.PageCommentThread{
			ID:        "thread-1",
			CreatedBy: "creator",
		},
		Messages: []domain.PageCommentThreadMessage{
			{CreatedBy: "actor"},
			{CreatedBy: "prior-1"},
			{CreatedBy: "prior-2"},
			{CreatedBy: "prior-1"},
		},
	}

	got, err := resolver.ResolveRecipients(context.Background(), ResolveThreadNotificationRecipientsInput{
		WorkspaceID:            "workspace-1",
		ActorID:                "actor",
		Detail:                 detail,
		ExplicitMentionUserIDs: []string{"prior-2", "mention-1", "actor", "", "non-member", "creator"},
	})
	if err != nil {
		t.Fatalf("ResolveRecipients() error = %v", err)
	}

	want := []string{"creator", "prior-1", "prior-2", "mention-1"}
	if len(got) != len(want) {
		t.Fatalf("ResolveRecipients() len = %d, want %d (%v)", len(got), len(want), got)
	}
	for idx := range want {
		if got[idx] != want[idx] {
			t.Fatalf("ResolveRecipients()[%d] = %q, want %q (full=%v)", idx, got[idx], want[idx], got)
		}
	}
}

func TestThreadNotificationRecipientResolverReturnsEmptyForActorOnlyThreadCreation(t *testing.T) {
	resolver := NewThreadNotificationRecipientResolver(threadNotificationMembershipStub{
		members: []domain.WorkspaceMember{{UserID: "actor"}},
	})

	got, err := resolver.ResolveRecipients(context.Background(), ResolveThreadNotificationRecipientsInput{
		WorkspaceID: "workspace-1",
		ActorID:     "actor",
		Detail: domain.PageCommentThreadDetail{
			Thread: domain.PageCommentThread{ID: "thread-1", CreatedBy: "actor"},
			Messages: []domain.PageCommentThreadMessage{
				{CreatedBy: "actor"},
			},
		},
	})
	if err != nil {
		t.Fatalf("ResolveRecipients() error = %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("ResolveRecipients() = %v, want empty", got)
	}
}

func TestThreadNotificationRecipientResolverValidatesRequiredFields(t *testing.T) {
	resolver := NewThreadNotificationRecipientResolver(threadNotificationMembershipStub{})

	tests := []struct {
		name string
		in   ResolveThreadNotificationRecipientsInput
	}{
		{
			name: "workspace",
			in: ResolveThreadNotificationRecipientsInput{
				ActorID: "actor",
				Detail:  domain.PageCommentThreadDetail{Thread: domain.PageCommentThread{ID: "thread-1"}},
			},
		},
		{
			name: "actor",
			in: ResolveThreadNotificationRecipientsInput{
				WorkspaceID: "workspace-1",
				Detail:      domain.PageCommentThreadDetail{Thread: domain.PageCommentThread{ID: "thread-1"}},
			},
		},
		{
			name: "thread",
			in: ResolveThreadNotificationRecipientsInput{
				WorkspaceID: "workspace-1",
				ActorID:     "actor",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := resolver.ResolveRecipients(context.Background(), tt.in)
			if !errors.Is(err, domain.ErrValidation) {
				t.Fatalf("ResolveRecipients() error = %v, want validation", err)
			}
		})
	}
}

func TestThreadNotificationRecipientResolverPropagatesMembershipErrors(t *testing.T) {
	resolver := NewThreadNotificationRecipientResolver(threadNotificationMembershipStub{
		err: errors.New("members failed"),
	})

	_, err := resolver.ResolveRecipients(context.Background(), ResolveThreadNotificationRecipientsInput{
		WorkspaceID: "workspace-1",
		ActorID:     "actor",
		Detail:      domain.PageCommentThreadDetail{Thread: domain.PageCommentThread{ID: "thread-1", CreatedBy: "creator"}},
	})
	if err == nil || err.Error() != "members failed" {
		t.Fatalf("ResolveRecipients() error = %v, want membership error", err)
	}
}
