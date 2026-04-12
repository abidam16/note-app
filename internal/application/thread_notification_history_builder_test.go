package application

import (
	"testing"
	"time"

	"note-app/internal/domain"
)

func TestThreadNotificationHistoryBuilder(t *testing.T) {
	now := time.Date(2026, 4, 7, 8, 0, 0, 0, time.UTC)
	thread := domain.PageCommentThread{
		ID:        "thread-1",
		PageID:    "page-1",
		CreatedBy: "creator",
		CreatedAt: now,
	}

	t.Run("starter message without mentions has no recipients", func(t *testing.T) {
		got, err := BuildThreadNotificationHistory(ThreadNotificationHistoryInput{
			Thread: thread,
			Messages: []domain.PageCommentThreadMessage{
				{ID: "m1", ThreadID: thread.ID, CreatedBy: "creator", CreatedAt: now},
			},
			ExplicitMentionsByMessageID: map[string][]string{},
			WorkspaceMemberIDs:          []string{"creator"},
		})
		if err != nil {
			t.Fatalf("BuildThreadNotificationHistory() error = %v", err)
		}
		if len(got) != 1 {
			t.Fatalf("expected one history entry, got %+v", got)
		}
		if len(got[0].CommentRecipients) != 0 || len(got[0].MentionRecipients) != 0 {
			t.Fatalf("expected no recipients for starter without mentions, got %+v", got[0])
		}
	})

	t.Run("starter message with mention notifies the mention target", func(t *testing.T) {
		got, err := BuildThreadNotificationHistory(ThreadNotificationHistoryInput{
			Thread: thread,
			Messages: []domain.PageCommentThreadMessage{
				{ID: "m1", ThreadID: thread.ID, CreatedBy: "creator", CreatedAt: now},
			},
			ExplicitMentionsByMessageID: map[string][]string{"m1": []string{"member-a"}},
			WorkspaceMemberIDs:          []string{"creator", "member-a"},
		})
		if err != nil {
			t.Fatalf("BuildThreadNotificationHistory() error = %v", err)
		}
		if len(got[0].CommentRecipients) != 1 || got[0].CommentRecipients[0] != "member-a" {
			t.Fatalf("expected comment recipient from starter mention, got %+v", got[0])
		}
		if len(got[0].MentionRecipients) != 1 || got[0].MentionRecipients[0] != "member-a" {
			t.Fatalf("expected mention recipient from starter mention, got %+v", got[0])
		}
	})

	t.Run("prior repliers and mentions are accumulated in first-seen order", func(t *testing.T) {
		got, err := BuildThreadNotificationHistory(ThreadNotificationHistoryInput{
			Thread: thread,
			Messages: []domain.PageCommentThreadMessage{
				{ID: "m1", ThreadID: thread.ID, CreatedBy: "creator", CreatedAt: now},
				{ID: "m2", ThreadID: thread.ID, CreatedBy: "replier-1", CreatedAt: now.Add(time.Minute)},
				{ID: "m3", ThreadID: thread.ID, CreatedBy: "actor", CreatedAt: now.Add(2 * time.Minute)},
			},
			ExplicitMentionsByMessageID: map[string][]string{
				"m2": []string{"replier-1", "member-a", "", "creator"},
				"m3": []string{"member-a", "member-b", "replier-1"},
			},
			WorkspaceMemberIDs: []string{"creator", "replier-1", "member-a", "member-b", "actor"},
		})
		if err != nil {
			t.Fatalf("BuildThreadNotificationHistory() error = %v", err)
		}

		if got[1].CommentRecipients[0] != "creator" {
			t.Fatalf("expected second message to notify creator first, got %+v", got[1])
		}
		wantThird := []string{"creator", "replier-1", "member-a", "member-b"}
		if len(got[2].CommentRecipients) != len(wantThird) {
			t.Fatalf("unexpected comment recipients: %+v", got[2])
		}
		for i, want := range wantThird {
			if got[2].CommentRecipients[i] != want {
				t.Fatalf("comment recipient[%d] = %q, want %q (full=%+v)", i, got[2].CommentRecipients[i], want, got[2].CommentRecipients)
			}
		}
		if len(got[2].MentionRecipients) != 3 || got[2].MentionRecipients[0] != "member-a" || got[2].MentionRecipients[1] != "member-b" || got[2].MentionRecipients[2] != "replier-1" {
			t.Fatalf("unexpected mention recipients: %+v", got[2])
		}
	})

	t.Run("self mentions blanks and non-members are removed", func(t *testing.T) {
		got, err := BuildThreadNotificationHistory(ThreadNotificationHistoryInput{
			Thread: thread,
			Messages: []domain.PageCommentThreadMessage{
				{ID: "m1", ThreadID: thread.ID, CreatedBy: "actor", CreatedAt: now},
			},
			ExplicitMentionsByMessageID: map[string][]string{"m1": []string{"actor", "", "missing", "member-a", "member-a"}},
			WorkspaceMemberIDs:          []string{"actor", "member-a"},
		})
		if err != nil {
			t.Fatalf("BuildThreadNotificationHistory() error = %v", err)
		}
		if len(got[0].CommentRecipients) != 1 || got[0].CommentRecipients[0] != "member-a" {
			t.Fatalf("expected filtered comment recipient, got %+v", got[0])
		}
		if len(got[0].MentionRecipients) != 1 || got[0].MentionRecipients[0] != "member-a" {
			t.Fatalf("expected filtered mention recipient, got %+v", got[0])
		}
	})
}
