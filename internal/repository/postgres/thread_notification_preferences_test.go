package postgres

import (
	"context"
	"testing"
	"time"

	"note-app/internal/domain"

	"github.com/google/uuid"
)

func TestThreadNotificationPreferenceRepositoryIntegration(t *testing.T) {
	pool := integrationPool(t)
	ctx := context.Background()

	owner := seedUser(t, pool, "thread-pref-owner@example.com")
	member := seedUser(t, pool, "thread-pref-member@example.com")
	workspace, _ := seedWorkspaceWithOwner(t, pool, owner)
	page, _ := seedPageWithDraft(t, pool, workspace.ID, owner.ID, nil, "Thread Pref Page")
	threadRepo := NewThreadRepository(pool)
	blockID := uuid.NewString()
	now := time.Now().UTC().Truncate(time.Microsecond)
	thread := domain.PageCommentThread{
		ID:     uuid.NewString(),
		PageID: page.ID,
		Anchor: domain.PageCommentThreadAnchor{
			Type:            domain.PageCommentThreadAnchorTypeBlock,
			BlockID:         &blockID,
			QuotedBlockText: "thread pref",
		},
		ThreadState:    domain.PageCommentThreadStateOpen,
		AnchorState:    domain.PageCommentThreadAnchorStateActive,
		CreatedBy:      owner.ID,
		CreatedAt:      now,
		LastActivityAt: now,
	}
	message := domain.PageCommentThreadMessage{
		ID:        uuid.NewString(),
		ThreadID:  thread.ID,
		Body:      "starter",
		CreatedBy: owner.ID,
		CreatedAt: thread.CreatedAt,
	}
	outboxEvent, err := domain.NewThreadCreatedOutboxEvent(thread, message, workspace.ID, nil)
	if err != nil {
		t.Fatalf("build thread outbox event: %v", err)
	}
	if _, err := threadRepo.CreateThread(ctx, thread, message, nil, outboxEvent); err != nil {
		t.Fatalf("create thread: %v", err)
	}

	repo := NewThreadNotificationPreferenceRepository(pool)
	if pref, err := repo.GetThreadNotificationPreference(ctx, thread.ID, member.ID); err != nil {
		t.Fatalf("get missing thread preference: %v", err)
	} else if pref != nil {
		t.Fatalf("expected no preference row, got %+v", pref)
	}

	mustExec(t, pool, `
		INSERT INTO thread_notification_preferences (thread_id, user_id, mode, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5)
	`, thread.ID, member.ID, string(domain.ThreadNotificationModeMentionsOnly), now, now)

	pref, err := repo.GetThreadNotificationPreference(ctx, thread.ID, member.ID)
	if err != nil {
		t.Fatalf("get thread preference: %v", err)
	}
	if pref == nil {
		t.Fatal("expected preference row")
	}
	if pref.ThreadID != thread.ID || pref.UserID != member.ID || pref.Mode != domain.ThreadNotificationModeMentionsOnly {
		t.Fatalf("unexpected preference row: %+v", pref)
	}
	if !pref.CreatedAt.Equal(now) || !pref.UpdatedAt.Equal(now) {
		t.Fatalf("unexpected preference timestamps: %+v", pref)
	}

	updatedAt := now.Add(2 * time.Minute).Truncate(time.Microsecond)
	if err := repo.SetThreadNotificationPreference(ctx, domain.ThreadNotificationPreference{
		ThreadID:  thread.ID,
		UserID:    member.ID,
		Mode:      domain.ThreadNotificationModeMute,
		CreatedAt: now,
		UpdatedAt: updatedAt,
	}); err != nil {
		t.Fatalf("set thread preference update: %v", err)
	}
	pref, err = repo.GetThreadNotificationPreference(ctx, thread.ID, member.ID)
	if err != nil {
		t.Fatalf("get updated thread preference: %v", err)
	}
	if pref == nil {
		t.Fatal("expected updated preference row")
	}
	if pref.Mode != domain.ThreadNotificationModeMute {
		t.Fatalf("expected updated mode mute, got %+v", pref)
	}
	if !pref.CreatedAt.Equal(now) || !pref.UpdatedAt.Equal(updatedAt) {
		t.Fatalf("expected created_at preserved and updated_at refreshed, got %+v", pref)
	}

	defaultAt := updatedAt.Add(2 * time.Minute).Truncate(time.Microsecond)
	if err := repo.SetThreadNotificationPreference(ctx, domain.ThreadNotificationPreference{
		ThreadID:  thread.ID,
		UserID:    member.ID,
		Mode:      domain.ThreadNotificationModeAll,
		UpdatedAt: defaultAt,
	}); err != nil {
		t.Fatalf("set thread preference default: %v", err)
	}
	if pref, err := repo.GetThreadNotificationPreference(ctx, thread.ID, member.ID); err != nil {
		t.Fatalf("get defaulted thread preference: %v", err)
	} else if pref != nil {
		t.Fatalf("expected preference row deleted, got %+v", pref)
	}

	if err := repo.SetThreadNotificationPreference(ctx, domain.ThreadNotificationPreference{
		ThreadID:  thread.ID,
		UserID:    member.ID,
		Mode:      domain.ThreadNotificationModeAll,
		UpdatedAt: defaultAt,
	}); err != nil {
		t.Fatalf("set thread preference default without row: %v", err)
	}
	if pref, err := repo.GetThreadNotificationPreference(ctx, thread.ID, member.ID); err != nil {
		t.Fatalf("get default without row preference: %v", err)
	} else if pref != nil {
		t.Fatalf("expected no preference row after defaulting without row, got %+v", pref)
	}
}

func TestThreadNotificationPreferenceRepositoryValidation(t *testing.T) {
	pool := integrationPool(t)
	ctx := context.Background()
	repo := NewThreadNotificationPreferenceRepository(pool)

	if err := repo.SetThreadNotificationPreference(ctx, domain.ThreadNotificationPreference{
		ThreadID:  "thread-1",
		UserID:    "user-1",
		Mode:      "bogus",
		UpdatedAt: time.Now().UTC(),
	}); err == nil {
		t.Fatal("expected invalid mode validation error")
	}
}
