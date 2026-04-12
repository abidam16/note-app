package application

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"note-app/internal/domain"
)

type threadMentionMembershipStub struct {
	members []domain.WorkspaceMember
	err     error
	calls   int
}

func (s *threadMentionMembershipStub) GetMembershipByUserID(context.Context, string, string) (domain.WorkspaceMember, error) {
	return domain.WorkspaceMember{}, nil
}

func (s *threadMentionMembershipStub) ListMembers(context.Context, string) ([]domain.WorkspaceMember, error) {
	s.calls++
	if s.err != nil {
		return nil, s.err
	}
	return s.members, nil
}

func TestNormalizeThreadMentionUserIDs(t *testing.T) {
	tests := []struct {
		name    string
		input   []string
		want    []string
		wantErr bool
	}{
		{
			name:  "omitted",
			input: nil,
			want:  []string{},
		},
		{
			name:  "trim dedupe preserve order",
			input: []string{" user-2 ", "user-3", "user-2", "user-4", "user-3"},
			want:  []string{"user-2", "user-3", "user-4"},
		},
		{
			name:    "blank rejected",
			input:   []string{"user-1", "   "},
			wantErr: true,
		},
		{
			name:    "too many unique rejected",
			input:   func() []string { ids := make([]string, 21); for i := range ids { ids[i] = "user-" + string(rune('a'+i)) }; return ids }(),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeThreadMentionUserIDs(tt.input)
			if tt.wantErr {
				if !errors.Is(err, domain.ErrValidation) {
					t.Fatalf("normalizeThreadMentionUserIDs() error = %v, want validation", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("normalizeThreadMentionUserIDs() error = %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("normalizeThreadMentionUserIDs() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestValidateThreadMentionUserIDs(t *testing.T) {
	t.Run("accepts members and self mention", func(t *testing.T) {
		memberships := &threadMentionMembershipStub{
			members: []domain.WorkspaceMember{
				{UserID: "user-1"},
				{UserID: "user-2"},
			},
		}

		if err := validateThreadMentionUserIDs(context.Background(), memberships, "workspace-1", []string{"user-2", "user-1"}); err != nil {
			t.Fatalf("validateThreadMentionUserIDs() error = %v", err)
		}
		if memberships.calls != 1 {
			t.Fatalf("expected one membership lookup, got %d", memberships.calls)
		}
	})

	t.Run("rejects non-member", func(t *testing.T) {
		memberships := &threadMentionMembershipStub{
			members: []domain.WorkspaceMember{{UserID: "user-1"}},
		}

		err := validateThreadMentionUserIDs(context.Background(), memberships, "workspace-1", []string{"user-1", "user-2"})
		if !errors.Is(err, domain.ErrValidation) {
			t.Fatalf("validateThreadMentionUserIDs() error = %v, want validation", err)
		}
	})

	t.Run("propagates membership lookup errors", func(t *testing.T) {
		memberships := &threadMentionMembershipStub{err: errors.New("members failed")}

		err := validateThreadMentionUserIDs(context.Background(), memberships, "workspace-1", []string{"user-1"})
		if err == nil || err.Error() != "members failed" {
			t.Fatalf("validateThreadMentionUserIDs() error = %v, want membership error", err)
		}
	})
}

