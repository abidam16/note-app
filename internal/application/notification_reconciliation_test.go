package application

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"
	"time"

	"note-app/internal/domain"
)

type notificationReconciliationLockStub struct {
	released      bool
	releaseCtxErr error
	err           error
}

func (l *notificationReconciliationLockStub) Release(ctx context.Context) error {
	l.released = true
	l.releaseCtxErr = ctx.Err()
	return l.err
}

type notificationReconciliationPublisherStub struct {
	signals []domain.NotificationStreamSignal
	err     error
}

func (p *notificationReconciliationPublisherStub) Publish(_ context.Context, signal domain.NotificationStreamSignal) error {
	p.signals = append(p.signals, signal)
	return p.err
}

type notificationReconciliationRepoStub struct {
	lock                  *notificationReconciliationLockStub
	lockErr               error
	invitationPages       []NotificationReconciliationInvitationPage
	invitationSourcesByID map[string]NotificationReconciliationInvitationSource
	threadPages           []NotificationReconciliationThreadPage
	histories             map[string]ThreadNotificationHistory
	managedNotifications  map[domain.NotificationType][]domain.Notification
	managedUsers          []string
	counterStates         []NotificationReconciliationCounterState
	unreadCounts          map[string]int64

	acquireCalls     int
	upserted         []domain.Notification
	deletedIDs       []string
	counterUpserts   map[string]int64
	counterDeletes   []string
	listManagedCalls []domain.NotificationType
	upsertErr        error
	deleteErr        error
	countErr         error
	listErr          error
}

func (r *notificationReconciliationRepoStub) AcquireReconciliationLock(context.Context) (NotificationReconciliationLock, error) {
	r.acquireCalls++
	if r.lockErr != nil {
		return nil, r.lockErr
	}
	if r.lock == nil {
		r.lock = &notificationReconciliationLockStub{}
	}
	return r.lock, nil
}

func (r *notificationReconciliationRepoStub) ListInvitationSources(_ context.Context, _ string, _ time.Time, _ int, _ string) (NotificationReconciliationInvitationPage, error) {
	if r.listErr != nil {
		return NotificationReconciliationInvitationPage{}, r.listErr
	}
	if len(r.invitationPages) == 0 {
		return NotificationReconciliationInvitationPage{}, nil
	}
	page := r.invitationPages[0]
	r.invitationPages = r.invitationPages[1:]
	return page, nil
}

func (r *notificationReconciliationRepoStub) GetInvitationSourceByID(_ context.Context, invitationID string) (NotificationReconciliationInvitationSource, error) {
	if r.listErr != nil {
		return NotificationReconciliationInvitationSource{}, r.listErr
	}
	if r.invitationSourcesByID == nil {
		return NotificationReconciliationInvitationSource{}, domain.ErrNotFound
	}
	source, ok := r.invitationSourcesByID[invitationID]
	if !ok {
		return NotificationReconciliationInvitationSource{}, domain.ErrNotFound
	}
	return source, nil
}

func (r *notificationReconciliationRepoStub) ListThreadSources(_ context.Context, _ string, _ time.Time, _ int, _ string) (NotificationReconciliationThreadPage, error) {
	if r.listErr != nil {
		return NotificationReconciliationThreadPage{}, r.listErr
	}
	if len(r.threadPages) == 0 {
		return NotificationReconciliationThreadPage{}, nil
	}
	page := r.threadPages[0]
	r.threadPages = r.threadPages[1:]
	return page, nil
}

func (r *notificationReconciliationRepoStub) LoadThreadHistory(_ context.Context, threadID string, _ time.Time) (ThreadNotificationHistory, error) {
	if r.listErr != nil {
		return ThreadNotificationHistory{}, r.listErr
	}
	if r.histories == nil {
		return ThreadNotificationHistory{}, domain.ErrNotFound
	}
	history, ok := r.histories[threadID]
	if !ok {
		return ThreadNotificationHistory{}, domain.ErrNotFound
	}
	return history, nil
}

func (r *notificationReconciliationRepoStub) ListManagedNotifications(_ context.Context, _ string, _ time.Time, types []domain.NotificationType) ([]domain.Notification, error) {
	if r.listErr != nil {
		return nil, r.listErr
	}
	managed := make([]domain.Notification, 0)
	for _, typ := range types {
		r.listManagedCalls = append(r.listManagedCalls, typ)
		managed = append(managed, r.managedNotifications[typ]...)
	}
	return managed, nil
}

func (r *notificationReconciliationRepoStub) ListManagedNotificationUsers(_ context.Context, _ string, _ time.Time, _ []domain.NotificationType) ([]string, error) {
	if r.listErr != nil {
		return nil, r.listErr
	}
	return append([]string(nil), r.managedUsers...), nil
}

func (r *notificationReconciliationRepoStub) ListCounterStates(_ context.Context, _ []string) ([]NotificationReconciliationCounterState, error) {
	if r.listErr != nil {
		return nil, r.listErr
	}
	return append([]NotificationReconciliationCounterState(nil), r.counterStates...), nil
}

func (r *notificationReconciliationRepoStub) UpsertManagedNotification(_ context.Context, notification domain.Notification) (NotificationReconciliationMutationResult, error) {
	if r.upsertErr != nil {
		return NotificationReconciliationMutationResult{}, r.upsertErr
	}
	r.upserted = append(r.upserted, notification)
	return NotificationReconciliationMutationResult{Changed: true, Inserted: true}, nil
}

func (r *notificationReconciliationRepoStub) DeleteManagedNotifications(_ context.Context, ids []string) (int64, error) {
	if r.deleteErr != nil {
		return 0, r.deleteErr
	}
	r.deletedIDs = append(r.deletedIDs, ids...)
	return int64(len(ids)), nil
}

func (r *notificationReconciliationRepoStub) CountUnreadNotifications(_ context.Context, userID string) (int64, error) {
	if r.countErr != nil {
		return 0, r.countErr
	}
	return r.unreadCounts[userID], nil
}

func (r *notificationReconciliationRepoStub) UpsertUnreadCounter(_ context.Context, userID string, unreadCount int64, _ time.Time) (bool, error) {
	if r.upsertErr != nil {
		return false, r.upsertErr
	}
	if r.counterUpserts == nil {
		r.counterUpserts = map[string]int64{}
	}
	r.counterUpserts[userID] = unreadCount
	return true, nil
}

func (r *notificationReconciliationRepoStub) DeleteUnreadCounter(_ context.Context, userID string) (bool, error) {
	if r.deleteErr != nil {
		return false, r.deleteErr
	}
	r.counterDeletes = append(r.counterDeletes, userID)
	return true, nil
}

func TestNotificationReconciliationServiceRun(t *testing.T) {
	now := time.Date(2026, 4, 7, 9, 0, 0, 0, time.UTC)
	workspaceID := "workspace-1"
	threadID := "thread-1"
	pageID := "page-1"
	invitedUserID := "user-invited"
	ownerID := "user-owner"
	mentionID := "user-mention"

	threadHistory := ThreadNotificationHistory{
		WorkspaceID: workspaceID,
		Thread: domain.PageCommentThread{
			ID:        threadID,
			PageID:    pageID,
			CreatedBy: ownerID,
			CreatedAt: now,
		},
		Messages: []domain.PageCommentThreadMessage{
			{ID: "m1", ThreadID: threadID, CreatedBy: ownerID, CreatedAt: now},
			{ID: "m2", ThreadID: threadID, CreatedBy: ownerID, CreatedAt: now.Add(time.Minute)},
		},
		ExplicitMentionsByMessageID: map[string][]string{
			"m2": []string{mentionID},
		},
		WorkspaceMemberIDs: []string{ownerID, mentionID},
	}

	repo := &notificationReconciliationRepoStub{
		invitationPages: []NotificationReconciliationInvitationPage{
			{
				Items: []NotificationReconciliationInvitationSource{
					{
						Invitation: domain.WorkspaceInvitation{
							ID:          "inv-1",
							WorkspaceID: workspaceID,
							Email:       "invitee@example.com",
							Role:        domain.RoleViewer,
							InvitedBy:   ownerID,
							CreatedAt:   now,
							Status:      domain.WorkspaceInvitationStatusPending,
							Version:     1,
							UpdatedAt:   now,
						},
						RegisteredUserID: &invitedUserID,
					},
				},
			},
		},
		threadPages: []NotificationReconciliationThreadPage{
			{Items: []domain.PageCommentThread{{ID: threadID, PageID: pageID, CreatedBy: ownerID, CreatedAt: now}}},
		},
		histories: map[string]ThreadNotificationHistory{
			threadID: threadHistory,
		},
		managedNotifications: map[domain.NotificationType][]domain.Notification{
			domain.NotificationTypeInvitation: []domain.Notification{},
			domain.NotificationTypeComment: []domain.Notification{
				{
					ID:          "comment-orphan",
					UserID:      ownerID,
					WorkspaceID: workspaceID,
					Type:        domain.NotificationTypeComment,
					EventID:     "m1",
					CreatedAt:   now,
				},
			},
			domain.NotificationTypeMention: []domain.Notification{},
		},
		managedUsers:  []string{ownerID, invitedUserID, mentionID},
		counterStates: []NotificationReconciliationCounterState{{UserID: ownerID, UnreadCount: 0}},
		unreadCounts: map[string]int64{
			invitedUserID: 1,
			ownerID:       0,
			mentionID:     1,
		},
	}
	publisher := &notificationReconciliationPublisherStub{}
	service := NewNotificationReconciliationService(repo, publisher, func() time.Time { return now })
	baseInvitationPages := append([]NotificationReconciliationInvitationPage(nil), repo.invitationPages...)
	baseThreadPages := append([]NotificationReconciliationThreadPage(nil), repo.threadPages...)
	baseHistories := map[string]ThreadNotificationHistory{}
	for key, value := range repo.histories {
		baseHistories[key] = value
	}
	baseManagedNotifications := map[domain.NotificationType][]domain.Notification{}
	for key, value := range repo.managedNotifications {
		baseManagedNotifications[key] = append([]domain.Notification(nil), value...)
	}
	baseCounterStates := append([]NotificationReconciliationCounterState(nil), repo.counterStates...)
	baseUnreadCounts := map[string]int64{}
	for key, value := range repo.unreadCounts {
		baseUnreadCounts[key] = value
	}
	resetRepo := func() {
		repo.invitationPages = append([]NotificationReconciliationInvitationPage(nil), baseInvitationPages...)
		repo.invitationSourcesByID = nil
		repo.threadPages = append([]NotificationReconciliationThreadPage(nil), baseThreadPages...)
		repo.histories = map[string]ThreadNotificationHistory{}
		for key, value := range baseHistories {
			repo.histories[key] = value
		}
		repo.managedNotifications = map[domain.NotificationType][]domain.Notification{}
		for key, value := range baseManagedNotifications {
			repo.managedNotifications[key] = append([]domain.Notification(nil), value...)
		}
		repo.counterStates = append([]NotificationReconciliationCounterState(nil), baseCounterStates...)
		repo.unreadCounts = map[string]int64{}
		for key, value := range baseUnreadCounts {
			repo.unreadCounts[key] = value
		}
		repo.upserted = nil
		repo.deletedIDs = nil
		repo.counterUpserts = nil
		repo.counterDeletes = nil
		publisher.signals = nil
	}

	t.Run("dry-run computes counts without writes", func(t *testing.T) {
		resetRepo()

		summary, err := service.Run(context.Background(), RunNotificationReconciliationInput{
			WorkspaceID: workspaceID,
			DryRun:      true,
			BatchSize:   100,
		})
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		if !summary.DryRun || summary.WorkspaceID == nil || *summary.WorkspaceID != workspaceID || summary.BatchSize != 100 {
			t.Fatalf("unexpected summary metadata: %+v", summary)
		}
		if summary.Invitations.Inserted != 1 {
			t.Fatalf("expected one predicted invitation insert, got %+v", summary.Invitations)
		}
		if len(repo.upserted) != 0 || len(repo.deletedIDs) != 0 || len(repo.counterUpserts) != 0 || len(repo.counterDeletes) != 0 {
			t.Fatalf("expected no writes in dry-run, got upserts=%+v deletes=%+v counterUpserts=%+v counterDeletes=%+v", repo.upserted, repo.deletedIDs, repo.counterUpserts, repo.counterDeletes)
		}
		if len(publisher.signals) != 0 {
			t.Fatalf("expected no invalidation publish in dry-run, got %+v", publisher.signals)
		}
	})

	t.Run("non-dry-run repairs managed rows and rebuilds counters", func(t *testing.T) {
		resetRepo()

		summary, err := service.Run(context.Background(), RunNotificationReconciliationInput{
			WorkspaceID: workspaceID,
			DryRun:      false,
			BatchSize:   100,
		})
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		if summary.Invitations.Inserted != 1 || summary.Comments.Deleted != 1 || summary.Mentions.Inserted != 1 {
			t.Fatalf("unexpected repair summary: %+v", summary)
		}
		if len(repo.upserted) < 3 {
			t.Fatalf("expected invitation, comment, and mention upserts, got %+v", repo.upserted)
		}
		if len(repo.deletedIDs) != 1 || repo.deletedIDs[0] != "comment-orphan" {
			t.Fatalf("expected one orphan comment delete, got %+v", repo.deletedIDs)
		}
		if got := repo.counterUpserts[invitedUserID]; got != 1 {
			t.Fatalf("expected invited user counter rebuild, got %+v", repo.counterUpserts)
		}
		if len(publisher.signals) == 0 {
			t.Fatal("expected best-effort invalidation publishes for effective changes")
		}
	})

	t.Run("non-dry-run invitation convergence replaces wrong-recipient row in one run", func(t *testing.T) {
		resetRepo()

		invitation := repo.invitationPages[0].Items[0].Invitation
		wrongUserID := "user-wrong"
		blockingRow := buildInvitationNotification(invitation, wrongUserID)
		repo.managedNotifications = map[domain.NotificationType][]domain.Notification{
			domain.NotificationTypeInvitation: []domain.Notification{blockingRow},
			domain.NotificationTypeComment:    []domain.Notification{},
			domain.NotificationTypeMention:    []domain.Notification{},
		}
		repo.threadPages = []NotificationReconciliationThreadPage{}
		repo.histories = map[string]ThreadNotificationHistory{}
		repo.counterStates = nil
		repo.unreadCounts = map[string]int64{invitedUserID: 1}

		summary, err := service.Run(context.Background(), RunNotificationReconciliationInput{
			WorkspaceID: workspaceID,
			BatchSize:   100,
		})
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		if summary.Invitations.Inserted != 1 || summary.Invitations.Deleted != 1 {
			t.Fatalf("expected wrong-recipient delete plus correct insert, got %+v", summary.Invitations)
		}
		if len(repo.deletedIDs) != 1 || repo.deletedIDs[0] != blockingRow.ID {
			t.Fatalf("expected wrong-recipient row to be deleted first, got %+v", repo.deletedIDs)
		}
		if len(repo.upserted) != 1 || repo.upserted[0].UserID != invitedUserID {
			t.Fatalf("expected repaired invitation row for correct user, got %+v", repo.upserted)
		}
	})

	t.Run("stale invitation cleanup skips source rows updated after cutoff", func(t *testing.T) {
		resetRepo()

		invitation := repo.invitationPages[0].Items[0].Invitation
		repo.managedNotifications = map[domain.NotificationType][]domain.Notification{
			domain.NotificationTypeInvitation: []domain.Notification{
				buildInvitationNotification(invitation, invitedUserID),
			},
			domain.NotificationTypeComment: []domain.Notification{},
			domain.NotificationTypeMention: []domain.Notification{},
		}
		repo.invitationPages = []NotificationReconciliationInvitationPage{}
		repo.threadPages = []NotificationReconciliationThreadPage{}
		repo.histories = map[string]ThreadNotificationHistory{}
		repo.counterStates = nil
		repo.unreadCounts = nil
		repo.invitationSourcesByID = map[string]NotificationReconciliationInvitationSource{
			invitation.ID: {
				Invitation: func() domain.WorkspaceInvitation {
					source := invitation
					source.UpdatedAt = now.Add(time.Minute)
					return source
				}(),
				RegisteredUserID: &invitedUserID,
			},
		}

		summary, err := service.Run(context.Background(), RunNotificationReconciliationInput{
			WorkspaceID: workspaceID,
			BatchSize:   100,
		})
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		if summary.Invitations.Deleted != 0 {
			t.Fatalf("expected invitation delete to be skipped, got %+v", summary.Invitations)
		}
		if len(repo.deletedIDs) != 0 {
			t.Fatalf("expected no invitation delete writes, got %+v", repo.deletedIDs)
		}
	})

	t.Run("thread reconciliation uses message ids instead of history order", func(t *testing.T) {
		resetRepo()

		threadHistory := ThreadNotificationHistory{
			WorkspaceID: workspaceID,
			Thread: domain.PageCommentThread{
				ID:        threadID,
				PageID:    pageID,
				CreatedBy: ownerID,
				CreatedAt: now,
			},
			Messages: []domain.PageCommentThreadMessage{
				{ID: "m2", ThreadID: threadID, CreatedBy: "replier-2", CreatedAt: now.Add(time.Minute)},
				{ID: "m1", ThreadID: threadID, CreatedBy: "replier-1", CreatedAt: now},
			},
			ExplicitMentionsByMessageID: map[string][]string{},
			WorkspaceMemberIDs:          []string{ownerID, "replier-1", "replier-2"},
		}
		repo.threadPages = []NotificationReconciliationThreadPage{{Items: []domain.PageCommentThread{{ID: threadID, PageID: pageID, CreatedBy: ownerID, CreatedAt: now}}}}
		repo.histories = map[string]ThreadNotificationHistory{threadID: threadHistory}
		repo.managedNotifications = map[domain.NotificationType][]domain.Notification{
			domain.NotificationTypeInvitation: []domain.Notification{},
			domain.NotificationTypeComment:    []domain.Notification{},
			domain.NotificationTypeMention:    []domain.Notification{},
		}

		summary, err := service.Run(context.Background(), RunNotificationReconciliationInput{
			WorkspaceID: workspaceID,
			BatchSize:   100,
		})
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}

		got := make(map[string]struct{}, len(repo.upserted))
		for _, notification := range repo.upserted {
			if notification.Type != domain.NotificationTypeComment {
				continue
			}
			got[notification.UserID+"|"+notification.EventID] = struct{}{}
		}
		for _, want := range []string{
			ownerID + "|m1",
			ownerID + "|m2",
			"replier-1|m2",
		} {
			if _, ok := got[want]; !ok {
				t.Fatalf("missing expected comment notification %s; summary=%+v upserted=%+v", want, summary.Comments, repo.upserted)
			}
		}
		if _, ok := got[ownerID+"|m1"]; !ok || len(got) != 3 {
			t.Fatalf("unexpected comment notification set: %+v", got)
		}
	})

	t.Run("lock release logs failures and avoids caller cancellation context", func(t *testing.T) {
		resetRepo()

		buf := &bytes.Buffer{}
		logger := slog.New(slog.NewTextHandler(buf, nil))
		cancelCtx, cancel := context.WithCancel(context.Background())
		cancel()

		repo.lock = &notificationReconciliationLockStub{err: errors.New("unlock failed")}
		serviceWithLogger := NewNotificationReconciliationService(repo, publisher, func() time.Time { return now }, logger)

		if _, err := serviceWithLogger.Run(cancelCtx, RunNotificationReconciliationInput{WorkspaceID: workspaceID, BatchSize: 100}); err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		if repo.lock == nil || !repo.lock.released {
			t.Fatal("expected lock release to be attempted")
		}
		if repo.lock.releaseCtxErr != nil {
			t.Fatalf("expected unlock context to remain uncanceled, got %v", repo.lock.releaseCtxErr)
		}
		if !bytes.Contains(buf.Bytes(), []byte("notification reconciliation lock release failed")) {
			t.Fatalf("expected lock release failure to be logged, got %s", buf.String())
		}
	})

	t.Run("invalid batch size is rejected", func(t *testing.T) {
		if _, err := service.Run(context.Background(), RunNotificationReconciliationInput{BatchSize: 0}); err == nil {
			t.Fatal("expected batch size validation error")
		}
	})

	t.Run("lock acquisition failure stops the run", func(t *testing.T) {
		resetRepo()
		repo.lockErr = errors.New("lock failed")
		defer func() { repo.lockErr = nil }()
		if _, err := service.Run(context.Background(), RunNotificationReconciliationInput{WorkspaceID: workspaceID, BatchSize: 100}); err == nil || err.Error() != "lock failed" {
			t.Fatalf("expected lock failure, got %v", err)
		}
	})

	t.Run("publish failure is best effort", func(t *testing.T) {
		resetRepo()
		publisher.err = errors.New("publish failed")
		defer func() { publisher.err = nil }()
		if _, err := service.Run(context.Background(), RunNotificationReconciliationInput{WorkspaceID: workspaceID, BatchSize: 100}); err != nil {
			t.Fatalf("expected publish failure to be ignored, got %v", err)
		}
	})

	t.Run("dry-run reports zero changes when projection and counters already match", func(t *testing.T) {
		resetRepo()

		invitationNotification := buildInvitationNotification(repo.invitationPages[0].Items[0].Invitation, invitedUserID)
		var invitationPayload map[string]any
		if err := json.Unmarshal(invitationNotification.Payload, &invitationPayload); err != nil {
			t.Fatalf("unmarshal reconciliation invitation payload: %v", err)
		}
		if invitationPayload["invitation_id"] != repo.invitationPages[0].Items[0].Invitation.ID || invitationPayload["workspace_id"] != workspaceID || invitationPayload["email"] != "invitee@example.com" || invitationPayload["role"] != string(domain.RoleViewer) || invitationPayload["status"] != string(domain.WorkspaceInvitationStatusPending) {
			t.Fatalf("unexpected reconciliation invitation payload: %+v", invitationPayload)
		}
		commentNotification := mapCommentNotification(domain.OutboxTopicThreadReplyCreated, commentNotificationPayload{
			ThreadID:       threadID,
			MessageID:      "m2",
			PageID:         pageID,
			WorkspaceID:    workspaceID,
			ActorID:        ownerID,
			OccurredAt:     now.Add(time.Minute),
			MentionUserIDs: []string{mentionID},
		}, mentionID)
		mentionNotification := mapMentionNotification(domain.OutboxTopicThreadReplyCreated, commentNotificationPayload{
			ThreadID:       threadID,
			MessageID:      "m2",
			PageID:         pageID,
			WorkspaceID:    workspaceID,
			ActorID:        ownerID,
			OccurredAt:     now.Add(time.Minute),
			MentionUserIDs: []string{mentionID},
		}, mentionID)

		repo.managedNotifications = map[domain.NotificationType][]domain.Notification{
			domain.NotificationTypeInvitation: []domain.Notification{invitationNotification},
			domain.NotificationTypeComment:    []domain.Notification{commentNotification},
			domain.NotificationTypeMention:    []domain.Notification{mentionNotification},
		}
		repo.managedUsers = []string{invitedUserID, mentionID}
		repo.counterStates = []NotificationReconciliationCounterState{
			{UserID: invitedUserID, UnreadCount: 1},
			{UserID: mentionID, UnreadCount: 2},
		}
		repo.unreadCounts = map[string]int64{
			invitedUserID: 1,
			mentionID:     2,
		}

		summary, err := service.Run(context.Background(), RunNotificationReconciliationInput{
			WorkspaceID: workspaceID,
			DryRun:      true,
			BatchSize:   100,
		})
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		if summary.Invitations.Inserted != 0 || summary.Invitations.Updated != 0 || summary.Invitations.Deleted != 0 {
			t.Fatalf("expected no invitation drift, got %+v", summary.Invitations)
		}
		if summary.Comments.Inserted != 0 || summary.Comments.Updated != 0 || summary.Comments.Deleted != 0 {
			t.Fatalf("expected no comment drift, got %+v", summary.Comments)
		}
		if summary.Mentions.Inserted != 0 || summary.Mentions.Updated != 0 || summary.Mentions.Deleted != 0 {
			t.Fatalf("expected no mention drift, got %+v", summary.Mentions)
		}
		if summary.Counters.Upserted != 0 || summary.Counters.Deleted != 0 {
			t.Fatalf("expected no counter drift, got %+v", summary.Counters)
		}
		if len(publisher.signals) != 0 {
			t.Fatalf("expected no invalidation publishes, got %+v", publisher.signals)
		}
	})
}
