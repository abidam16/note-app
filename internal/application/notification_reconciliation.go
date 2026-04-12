package application

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"note-app/internal/domain"
)

type NotificationReconciliationRunner interface {
	Run(ctx context.Context, input RunNotificationReconciliationInput) (NotificationReconciliationSummary, error)
}

type NotificationReconciliationPublisher interface {
	Publish(ctx context.Context, signal domain.NotificationStreamSignal) error
}

type NotificationReconciliationLock interface {
	Release(ctx context.Context) error
}

type NotificationReconciliationRepository interface {
	AcquireReconciliationLock(ctx context.Context) (NotificationReconciliationLock, error)
	ListInvitationSources(ctx context.Context, workspaceID string, cutoff time.Time, limit int, cursor string) (NotificationReconciliationInvitationPage, error)
	GetInvitationSourceByID(ctx context.Context, invitationID string) (NotificationReconciliationInvitationSource, error)
	ListThreadSources(ctx context.Context, workspaceID string, cutoff time.Time, limit int, cursor string) (NotificationReconciliationThreadPage, error)
	LoadThreadHistory(ctx context.Context, threadID string, cutoff time.Time) (ThreadNotificationHistory, error)
	ListManagedNotifications(ctx context.Context, workspaceID string, cutoff time.Time, types []domain.NotificationType) ([]domain.Notification, error)
	DeleteManagedNotifications(ctx context.Context, ids []string) (int64, error)
	CountUnreadNotifications(ctx context.Context, userID string) (int64, error)
	ListCounterStates(ctx context.Context, userIDs []string) ([]NotificationReconciliationCounterState, error)
	UpsertManagedNotification(ctx context.Context, notification domain.Notification) (NotificationReconciliationMutationResult, error)
	UpsertUnreadCounter(ctx context.Context, userID string, unreadCount int64, updatedAt time.Time) (bool, error)
	DeleteUnreadCounter(ctx context.Context, userID string) (bool, error)
}

type RunNotificationReconciliationInput struct {
	WorkspaceID string
	DryRun      bool
	BatchSize   int
}

type NotificationReconciliationSummary struct {
	Status      string                                      `json:"status"`
	DryRun      bool                                        `json:"dry_run"`
	WorkspaceID *string                                     `json:"workspace_id,omitempty"`
	BatchSize   int                                         `json:"batch_size"`
	StartedAt   time.Time                                   `json:"started_at"`
	CutoffAt    time.Time                                   `json:"cutoff_at"`
	FinishedAt  time.Time                                   `json:"finished_at"`
	Invitations NotificationReconciliationInvitationSummary `json:"invitations"`
	Comments    NotificationReconciliationCommentSummary    `json:"comments"`
	Mentions    NotificationReconciliationMentionSummary    `json:"mentions"`
	Counters    NotificationReconciliationCounterSummary    `json:"counters"`
}

type NotificationReconciliationInvitationSummary struct {
	Scanned             int64 `json:"scanned"`
	UnregisteredSkipped int64 `json:"unregistered_skipped"`
	Inserted            int64 `json:"inserted"`
	Updated             int64 `json:"updated"`
	Deleted             int64 `json:"deleted"`
}

type NotificationReconciliationCommentSummary struct {
	ThreadsScanned  int64 `json:"threads_scanned"`
	MessagesScanned int64 `json:"messages_scanned"`
	Inserted        int64 `json:"inserted"`
	Updated         int64 `json:"updated"`
	Deleted         int64 `json:"deleted"`
}

type NotificationReconciliationMentionSummary struct {
	MessagesScanned    int64 `json:"messages_scanned"`
	MentionRowsScanned int64 `json:"mention_rows_scanned"`
	Inserted           int64 `json:"inserted"`
	Updated            int64 `json:"updated"`
	Deleted            int64 `json:"deleted"`
}

type NotificationReconciliationCounterSummary struct {
	UsersRecomputed int64 `json:"users_recomputed"`
	Upserted        int64 `json:"upserted"`
	Deleted         int64 `json:"deleted"`
}

type NotificationReconciliationInvitationSource struct {
	Invitation       domain.WorkspaceInvitation
	RegisteredUserID *string
}

type NotificationReconciliationInvitationPage struct {
	Items      []NotificationReconciliationInvitationSource
	NextCursor *string
	HasMore    bool
}

type NotificationReconciliationThreadPage struct {
	Items      []domain.PageCommentThread
	NextCursor *string
	HasMore    bool
}

type NotificationReconciliationCounterState struct {
	UserID      string
	UnreadCount int64
}

type NotificationReconciliationMutationResult struct {
	Notification domain.Notification
	Changed      bool
	Inserted     bool
	Updated      bool
}

type ThreadNotificationHistory struct {
	WorkspaceID                 string
	Thread                      domain.PageCommentThread
	Messages                    []domain.PageCommentThreadMessage
	ExplicitMentionsByMessageID map[string][]string
	WorkspaceMemberIDs          []string
}

type ThreadNotificationHistoryInput struct {
	Thread                      domain.PageCommentThread
	Messages                    []domain.PageCommentThreadMessage
	ExplicitMentionsByMessageID map[string][]string
	WorkspaceMemberIDs          []string
}

type ThreadNotificationHistoryEntry struct {
	MessageID         string
	CommentRecipients []string
	MentionRecipients []string
}

type notificationReconciliationService struct {
	repo      NotificationReconciliationRepository
	publisher NotificationReconciliationPublisher
	now       func() time.Time
	logger    *slog.Logger
}

type NotificationReconciliationService = notificationReconciliationService

func NewNotificationReconciliationService(repo NotificationReconciliationRepository, publisher NotificationReconciliationPublisher, now func() time.Time, logger ...*slog.Logger) NotificationReconciliationService {
	if now == nil {
		now = time.Now
	}
	serviceLogger := slog.Default()
	if len(logger) > 0 && logger[0] != nil {
		serviceLogger = logger[0]
	}
	return notificationReconciliationService{
		repo:      repo,
		publisher: publisher,
		now:       now,
		logger:    serviceLogger,
	}
}

func (s notificationReconciliationService) Run(ctx context.Context, input RunNotificationReconciliationInput) (NotificationReconciliationSummary, error) {
	if err := validateNotificationReconciliationInput(input); err != nil {
		return NotificationReconciliationSummary{}, err
	}

	lock, err := s.repo.AcquireReconciliationLock(ctx)
	if err != nil {
		return NotificationReconciliationSummary{}, err
	}
	defer func() {
		if lock != nil {
			if err := lock.Release(context.Background()); err != nil {
				s.logReleaseFailure(err)
			}
		}
	}()

	startedAt := s.now().UTC()
	summary := NotificationReconciliationSummary{
		Status:     "ok",
		DryRun:     input.DryRun,
		BatchSize:  input.BatchSize,
		StartedAt:  startedAt,
		CutoffAt:   startedAt,
		FinishedAt: startedAt,
	}
	if workspaceID := strings.TrimSpace(input.WorkspaceID); workspaceID != "" {
		summary.WorkspaceID = &workspaceID
	}

	recomputeUsers := map[string]struct{}{}
	changedUsers := map[string]struct{}{}

	invitations, invitationRecomputeUsers, invitationChangedUsers, err := s.reconcileInvitations(ctx, input, summary.CutoffAt)
	if err != nil {
		return NotificationReconciliationSummary{}, err
	}
	summary.Invitations = invitations
	mergeUsers(recomputeUsers, invitationRecomputeUsers)
	mergeUsers(changedUsers, invitationChangedUsers)

	comments, commentRecomputeUsers, commentChangedUsers, mentions, mentionRecomputeUsers, mentionChangedUsers, err := s.reconcileThreads(ctx, input, summary.CutoffAt)
	if err != nil {
		return NotificationReconciliationSummary{}, err
	}
	summary.Comments = comments
	summary.Mentions = mentions
	mergeUsers(recomputeUsers, commentRecomputeUsers)
	mergeUsers(recomputeUsers, mentionRecomputeUsers)
	mergeUsers(changedUsers, commentChangedUsers)
	mergeUsers(changedUsers, mentionChangedUsers)

	counters, counterChangedUsers, err := s.reconcileCounters(ctx, input, summary.CutoffAt, recomputeUsers)
	if err != nil {
		return NotificationReconciliationSummary{}, err
	}
	summary.Counters = counters
	mergeUsers(changedUsers, counterChangedUsers)

	summary.FinishedAt = s.now().UTC()

	if !input.DryRun && s.publisher != nil {
		for userID := range changedUsers {
			if err := s.publisher.Publish(ctx, domain.NotificationStreamSignal{
				UserID: userID,
				Reason: domain.NotificationStreamReasonNotificationsChanged,
				SentAt: s.now().UTC(),
			}); err != nil {
				slog.Default().Warn("notification reconciliation invalidation publish failed", slog.String("user_id", userID), slog.Any("error", err))
			}
		}
	}

	return summary, nil
}

func validateNotificationReconciliationInput(input RunNotificationReconciliationInput) error {
	if input.BatchSize < 1 || input.BatchSize > 2000 {
		return fmt.Errorf("%w: batch_size must be between 1 and 2000", domain.ErrValidation)
	}
	return nil
}

func (s notificationReconciliationService) reconcileInvitations(ctx context.Context, input RunNotificationReconciliationInput, cutoffAt time.Time) (NotificationReconciliationInvitationSummary, []string, []string, error) {
	summary := NotificationReconciliationInvitationSummary{}
	recomputeUsers := map[string]struct{}{}
	changedUsers := map[string]struct{}{}

	existing, err := s.repo.ListManagedNotifications(ctx, input.WorkspaceID, cutoffAt, []domain.NotificationType{domain.NotificationTypeInvitation})
	if err != nil {
		return NotificationReconciliationInvitationSummary{}, nil, nil, err
	}

	existingByKey := make(map[string]domain.Notification, len(existing))
	existingByInvitationID := make(map[string]domain.Notification, len(existing))
	for _, notification := range existing {
		existingByKey[notificationKey(notification)] = notification
		if invitationID := invitationNotificationIdentity(notification); invitationID != "" {
			existingByInvitationID[invitationID] = notification
		}
		recomputeUsers[notification.UserID] = struct{}{}
	}

	cursor := ""
	for {
		page, err := s.repo.ListInvitationSources(ctx, input.WorkspaceID, cutoffAt, input.BatchSize, cursor)
		if err != nil {
			return NotificationReconciliationInvitationSummary{}, nil, nil, err
		}
		if len(page.Items) == 0 {
			break
		}

		for _, source := range page.Items {
			summary.Scanned++
			expectedUserID := derefTrimmedString(source.RegisteredUserID)
			invitationID := strings.TrimSpace(source.Invitation.ID)
			if expectedUserID == "" {
				summary.UnregisteredSkipped++
				if existingNotification, ok := existingByInvitationID[invitationID]; ok {
					removeInvitationNotification(existingByKey, existingByInvitationID, existingNotification)
					recomputeUsers[existingNotification.UserID] = struct{}{}
					changedUsers[existingNotification.UserID] = struct{}{}
					if input.DryRun {
						summary.Deleted++
					} else {
						deleted, err := s.repo.DeleteManagedNotifications(ctx, []string{existingNotification.ID})
						if err != nil {
							return NotificationReconciliationInvitationSummary{}, nil, nil, err
						}
						summary.Deleted += deleted
					}
				}
				continue
			}

			expected := buildInvitationNotification(source.Invitation, expectedUserID)
			existingNotification, exists := existingByInvitationID[invitationID]
			recomputeUsers[expectedUserID] = struct{}{}
			if exists && strings.TrimSpace(existingNotification.UserID) != expectedUserID {
				removeInvitationNotification(existingByKey, existingByInvitationID, existingNotification)
				recomputeUsers[existingNotification.UserID] = struct{}{}
				changedUsers[existingNotification.UserID] = struct{}{}
				if input.DryRun {
					summary.Deleted++
				} else {
					deleted, err := s.repo.DeleteManagedNotifications(ctx, []string{existingNotification.ID})
					if err != nil {
						return NotificationReconciliationInvitationSummary{}, nil, nil, err
					}
					summary.Deleted += deleted
				}
				existingNotification = domain.Notification{}
				exists = false
			}

			if input.DryRun {
				switch {
				case exists && invitationNotificationNeedsRepair(existingNotification, expected):
					summary.Updated++
					changedUsers[expectedUserID] = struct{}{}
				case !exists:
					summary.Inserted++
					changedUsers[expectedUserID] = struct{}{}
				}
				if exists {
					removeInvitationNotification(existingByKey, existingByInvitationID, existingNotification)
				}
				continue
			}

			if exists && !invitationNotificationNeedsRepair(existingNotification, expected) {
				removeInvitationNotification(existingByKey, existingByInvitationID, existingNotification)
				continue
			}

			result, err := s.repo.UpsertManagedNotification(ctx, expected)
			if err != nil {
				return NotificationReconciliationInvitationSummary{}, nil, nil, err
			}
			if result.Changed {
				if result.Inserted {
					summary.Inserted++
				} else {
					summary.Updated++
				}
				changedUsers[expectedUserID] = struct{}{}
			}
			if exists {
				removeInvitationNotification(existingByKey, existingByInvitationID, existingNotification)
			}
		}

		if !page.HasMore || page.NextCursor == nil {
			break
		}
		cursor = *page.NextCursor
	}

	if len(existingByKey) > 0 {
		ids := make([]string, 0, len(existingByKey))
		for _, notification := range existingByKey {
			skipDelete, err := s.shouldSkipInvitationDelete(ctx, notification, cutoffAt)
			if err != nil {
				return NotificationReconciliationInvitationSummary{}, nil, nil, err
			}
			if skipDelete {
				continue
			}
			ids = append(ids, notification.ID)
			recomputeUsers[notification.UserID] = struct{}{}
			changedUsers[notification.UserID] = struct{}{}
		}
		if input.DryRun {
			summary.Deleted += int64(len(ids))
		} else {
			deleted, err := s.repo.DeleteManagedNotifications(ctx, ids)
			if err != nil {
				return NotificationReconciliationInvitationSummary{}, nil, nil, err
			}
			summary.Deleted += deleted
		}
	}

	return summary, mapKeys(recomputeUsers), mapKeys(changedUsers), nil
}

func (s notificationReconciliationService) shouldSkipInvitationDelete(ctx context.Context, notification domain.Notification, cutoffAt time.Time) (bool, error) {
	invitationID := invitationNotificationIdentity(notification)
	if invitationID == "" {
		return false, nil
	}
	source, err := s.repo.GetInvitationSourceByID(ctx, invitationID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	return source.Invitation.UpdatedAt.After(cutoffAt), nil
}

func (s notificationReconciliationService) reconcileThreads(ctx context.Context, input RunNotificationReconciliationInput, cutoffAt time.Time) (NotificationReconciliationCommentSummary, []string, []string, NotificationReconciliationMentionSummary, []string, []string, error) {
	commentSummary := NotificationReconciliationCommentSummary{}
	mentionSummary := NotificationReconciliationMentionSummary{}
	recomputeCommentUsers := map[string]struct{}{}
	recomputeMentionUsers := map[string]struct{}{}
	changedCommentUsers := map[string]struct{}{}
	changedMentionUsers := map[string]struct{}{}

	existingComments, err := s.repo.ListManagedNotifications(ctx, input.WorkspaceID, cutoffAt, []domain.NotificationType{domain.NotificationTypeComment})
	if err != nil {
		return NotificationReconciliationCommentSummary{}, nil, nil, NotificationReconciliationMentionSummary{}, nil, nil, err
	}
	existingMentions, err := s.repo.ListManagedNotifications(ctx, input.WorkspaceID, cutoffAt, []domain.NotificationType{domain.NotificationTypeMention})
	if err != nil {
		return NotificationReconciliationCommentSummary{}, nil, nil, NotificationReconciliationMentionSummary{}, nil, nil, err
	}

	existingCommentByKey := make(map[string]domain.Notification, len(existingComments))
	for _, notification := range existingComments {
		existingCommentByKey[notificationKey(notification)] = notification
		recomputeCommentUsers[notification.UserID] = struct{}{}
	}

	existingMentionByKey := make(map[string]domain.Notification, len(existingMentions))
	for _, notification := range existingMentions {
		existingMentionByKey[notificationKey(notification)] = notification
		recomputeMentionUsers[notification.UserID] = struct{}{}
	}

	cursor := ""
	for {
		page, err := s.repo.ListThreadSources(ctx, input.WorkspaceID, cutoffAt, input.BatchSize, cursor)
		if err != nil {
			return NotificationReconciliationCommentSummary{}, nil, nil, NotificationReconciliationMentionSummary{}, nil, nil, err
		}
		if len(page.Items) == 0 {
			break
		}

		commentSummary.ThreadsScanned += int64(len(page.Items))
		for _, thread := range page.Items {
			history, err := s.repo.LoadThreadHistory(ctx, thread.ID, cutoffAt)
			if err != nil {
				return NotificationReconciliationCommentSummary{}, nil, nil, NotificationReconciliationMentionSummary{}, nil, nil, err
			}
			entries, err := BuildThreadNotificationHistory(ThreadNotificationHistoryInput{
				Thread:                      history.Thread,
				Messages:                    history.Messages,
				ExplicitMentionsByMessageID: history.ExplicitMentionsByMessageID,
				WorkspaceMemberIDs:          history.WorkspaceMemberIDs,
			})
			if err != nil {
				return NotificationReconciliationCommentSummary{}, nil, nil, NotificationReconciliationMentionSummary{}, nil, nil, err
			}

			messageByID := make(map[string]domain.PageCommentThreadMessage, len(history.Messages))
			for _, message := range history.Messages {
				messageByID[message.ID] = message
			}

			for index, entry := range entries {
				message, ok := messageByID[entry.MessageID]
				if !ok {
					return NotificationReconciliationCommentSummary{}, nil, nil, NotificationReconciliationMentionSummary{}, nil, nil, fmt.Errorf("%w: thread history is missing message %s", domain.ErrNotFound, entry.MessageID)
				}
				explicitMentions := history.ExplicitMentionsByMessageID[message.ID]
				payload := commentNotificationPayload{
					ThreadID:       history.Thread.ID,
					MessageID:      message.ID,
					PageID:         history.Thread.PageID,
					WorkspaceID:    history.WorkspaceID,
					ActorID:        message.CreatedBy,
					OccurredAt:     message.CreatedAt.UTC(),
					MentionUserIDs: append([]string(nil), explicitMentions...),
				}
				topic := commentNotificationTopicForIndex(index)

				commentSummary.MessagesScanned++
				mentionSummary.MessagesScanned++
				mentionSummary.MentionRowsScanned += int64(len(explicitMentions))

				for _, recipientID := range entry.CommentRecipients {
					expected := mapCommentNotification(topic, payload, recipientID)
					key := notificationKey(expected)
					existingNotification, exists := existingCommentByKey[key]
					recomputeCommentUsers[recipientID] = struct{}{}

					if input.DryRun {
						switch {
						case exists && threadNotificationNeedsRepair(existingNotification, expected):
							commentSummary.Updated++
							changedCommentUsers[recipientID] = struct{}{}
						case !exists:
							commentSummary.Inserted++
							changedCommentUsers[recipientID] = struct{}{}
						}
						delete(existingCommentByKey, key)
						continue
					}

					if exists && !threadNotificationNeedsRepair(existingNotification, expected) {
						delete(existingCommentByKey, key)
						continue
					}
					if exists {
						expected.UpdatedAt = s.now().UTC()
					}

					result, err := s.repo.UpsertManagedNotification(ctx, expected)
					if err != nil {
						return NotificationReconciliationCommentSummary{}, nil, nil, NotificationReconciliationMentionSummary{}, nil, nil, err
					}
					if result.Changed {
						if result.Inserted {
							commentSummary.Inserted++
						} else {
							commentSummary.Updated++
						}
						changedCommentUsers[recipientID] = struct{}{}
					}
					delete(existingCommentByKey, key)
				}

				for _, recipientID := range entry.MentionRecipients {
					expected := mapMentionNotification(topic, payload, recipientID)
					key := notificationKey(expected)
					existingNotification, exists := existingMentionByKey[key]
					recomputeMentionUsers[recipientID] = struct{}{}

					if input.DryRun {
						switch {
						case exists && threadNotificationNeedsRepair(existingNotification, expected):
							mentionSummary.Updated++
							changedMentionUsers[recipientID] = struct{}{}
						case !exists:
							mentionSummary.Inserted++
							changedMentionUsers[recipientID] = struct{}{}
						}
						delete(existingMentionByKey, key)
						continue
					}

					if exists && !threadNotificationNeedsRepair(existingNotification, expected) {
						delete(existingMentionByKey, key)
						continue
					}
					if exists {
						expected.UpdatedAt = s.now().UTC()
					}

					result, err := s.repo.UpsertManagedNotification(ctx, expected)
					if err != nil {
						return NotificationReconciliationCommentSummary{}, nil, nil, NotificationReconciliationMentionSummary{}, nil, nil, err
					}
					if result.Changed {
						if result.Inserted {
							mentionSummary.Inserted++
						} else {
							mentionSummary.Updated++
						}
						changedMentionUsers[recipientID] = struct{}{}
					}
					delete(existingMentionByKey, key)
				}
			}
		}

		if !page.HasMore || page.NextCursor == nil {
			break
		}
		cursor = *page.NextCursor
	}

	if len(existingCommentByKey) > 0 {
		ids := make([]string, 0, len(existingCommentByKey))
		for _, notification := range existingCommentByKey {
			ids = append(ids, notification.ID)
			recomputeCommentUsers[notification.UserID] = struct{}{}
			changedCommentUsers[notification.UserID] = struct{}{}
		}
		if input.DryRun {
			commentSummary.Deleted += int64(len(ids))
		} else {
			deleted, err := s.repo.DeleteManagedNotifications(ctx, ids)
			if err != nil {
				return NotificationReconciliationCommentSummary{}, nil, nil, NotificationReconciliationMentionSummary{}, nil, nil, err
			}
			commentSummary.Deleted += deleted
		}
	}

	if len(existingMentionByKey) > 0 {
		ids := make([]string, 0, len(existingMentionByKey))
		for _, notification := range existingMentionByKey {
			ids = append(ids, notification.ID)
			recomputeMentionUsers[notification.UserID] = struct{}{}
			changedMentionUsers[notification.UserID] = struct{}{}
		}
		if input.DryRun {
			mentionSummary.Deleted += int64(len(ids))
		} else {
			deleted, err := s.repo.DeleteManagedNotifications(ctx, ids)
			if err != nil {
				return NotificationReconciliationCommentSummary{}, nil, nil, NotificationReconciliationMentionSummary{}, nil, nil, err
			}
			mentionSummary.Deleted += deleted
		}
	}

	return commentSummary, mapKeys(recomputeCommentUsers), mapKeys(changedCommentUsers), mentionSummary, mapKeys(recomputeMentionUsers), mapKeys(changedMentionUsers), nil
}

func (s notificationReconciliationService) reconcileCounters(ctx context.Context, input RunNotificationReconciliationInput, cutoffAt time.Time, recomputeUsers map[string]struct{}) (NotificationReconciliationCounterSummary, []string, error) {
	summary := NotificationReconciliationCounterSummary{}
	if len(recomputeUsers) == 0 {
		return summary, nil, nil
	}

	counterStates, err := s.repo.ListCounterStates(ctx, mapKeys(recomputeUsers))
	if err != nil {
		return NotificationReconciliationCounterSummary{}, nil, err
	}

	currentCounters := make(map[string]int64, len(counterStates))
	for _, state := range counterStates {
		currentCounters[state.UserID] = state.UnreadCount
		recomputeUsers[state.UserID] = struct{}{}
	}

	changedUsers := map[string]struct{}{}
	for userID := range recomputeUsers {
		summary.UsersRecomputed++
		desiredCount, err := s.repo.CountUnreadNotifications(ctx, userID)
		if err != nil {
			return NotificationReconciliationCounterSummary{}, nil, err
		}

		currentCount, hasCurrent := currentCounters[userID]
		if input.DryRun {
			switch {
			case desiredCount > 0 && (!hasCurrent || currentCount != desiredCount):
				summary.Upserted++
				changedUsers[userID] = struct{}{}
			case desiredCount == 0 && hasCurrent:
				summary.Deleted++
				changedUsers[userID] = struct{}{}
			}
			continue
		}

		if desiredCount > 0 {
			changed, err := s.repo.UpsertUnreadCounter(ctx, userID, desiredCount, cutoffAt)
			if err != nil {
				return NotificationReconciliationCounterSummary{}, nil, err
			}
			if changed {
				summary.Upserted++
				changedUsers[userID] = struct{}{}
			}
			continue
		}

		changed, err := s.repo.DeleteUnreadCounter(ctx, userID)
		if err != nil {
			return NotificationReconciliationCounterSummary{}, nil, err
		}
		if changed {
			summary.Deleted++
			changedUsers[userID] = struct{}{}
		}
	}

	return summary, mapKeys(changedUsers), nil
}

func (s notificationReconciliationService) logReleaseFailure(err error) {
	if err == nil {
		return
	}
	if s.logger == nil {
		s.logger = slog.Default()
	}
	s.logger.Warn("notification reconciliation lock release failed", slog.Any("error", err))
}

func buildInvitationNotification(invitation domain.WorkspaceInvitation, userID string) domain.Notification {
	notification := mapInvitationNotification(invitationTopicForStatus(invitation), invitationPayloadFromInvitation(invitation), userID)
	notification.CreatedAt = invitation.CreatedAt.UTC()
	notification.UpdatedAt = invitation.UpdatedAt.UTC()
	if notification.ResourceID != nil {
		notification.ID = *notification.ResourceID
	}
	return notification
}

func invitationTopicForStatus(invitation domain.WorkspaceInvitation) domain.OutboxTopic {
	switch invitation.Status {
	case domain.WorkspaceInvitationStatusAccepted:
		return domain.OutboxTopicInvitationAccepted
	case domain.WorkspaceInvitationStatusRejected:
		return domain.OutboxTopicInvitationRejected
	case domain.WorkspaceInvitationStatusCancelled:
		return domain.OutboxTopicInvitationCancelled
	default:
		if invitation.Version > 1 {
			return domain.OutboxTopicInvitationUpdated
		}
		return domain.OutboxTopicInvitationCreated
	}
}

func invitationPayloadFromInvitation(invitation domain.WorkspaceInvitation) invitationNotificationPayload {
	return invitationNotificationPayload{
		InvitationID: invitation.ID,
		WorkspaceID:  invitation.WorkspaceID,
		ActorID:      invitation.InvitedBy,
		Email:        invitation.Email,
		Role:         invitation.Role,
		Status:       invitation.Status,
		Version:      invitation.Version,
		OccurredAt:   invitation.CreatedAt.UTC(),
	}
}

func invitationNotificationIdentity(notification domain.Notification) string {
	if notification.ResourceID != nil && strings.TrimSpace(*notification.ResourceID) != "" {
		return strings.TrimSpace(*notification.ResourceID)
	}
	if strings.TrimSpace(notification.EventID) != "" {
		return strings.TrimSpace(notification.EventID)
	}
	return strings.TrimSpace(notification.ID)
}

func commentNotificationTopicForIndex(index int) domain.OutboxTopic {
	if index == 0 {
		return domain.OutboxTopicThreadCreated
	}
	return domain.OutboxTopicThreadReplyCreated
}

func notificationKey(notification domain.Notification) string {
	resourceID := ""
	if notification.ResourceID != nil {
		resourceID = strings.TrimSpace(*notification.ResourceID)
	}
	return strings.Join([]string{
		strings.TrimSpace(notification.UserID),
		string(notification.Type),
		strings.TrimSpace(notification.EventID),
		resourceID,
	}, "|")
}

func removeInvitationNotification(existingByKey map[string]domain.Notification, existingByInvitationID map[string]domain.Notification, notification domain.Notification) {
	delete(existingByKey, notificationKey(notification))
	if invitationID := invitationNotificationIdentity(notification); invitationID != "" {
		delete(existingByInvitationID, invitationID)
	}
}

func invitationNotificationNeedsRepair(existing, expected domain.Notification) bool {
	if existing.UserID != expected.UserID || existing.WorkspaceID != expected.WorkspaceID || existing.Type != expected.Type || existing.EventID != expected.EventID || existing.Message != expected.Message {
		return true
	}
	if existing.Title != expected.Title || existing.Content != expected.Content || existing.Actionable != expected.Actionable {
		return true
	}
	if !notificationStringPtrEqual(existing.ActorID, expected.ActorID) {
		return true
	}
	if !notificationActionKindEqual(existing.ActionKind, expected.ActionKind) {
		return true
	}
	if !notificationResourceTypeEqual(existing.ResourceType, expected.ResourceType) {
		return true
	}
	if !notificationStringPtrEqual(existing.ResourceID, expected.ResourceID) {
		return true
	}
	if !jsonRawMessageEqual(existing.Payload, expected.Payload) {
		return true
	}
	return !existing.UpdatedAt.Equal(expected.UpdatedAt)
}

func threadNotificationNeedsRepair(existing, expected domain.Notification) bool {
	if existing.UserID != expected.UserID || existing.WorkspaceID != expected.WorkspaceID || existing.Type != expected.Type || existing.EventID != expected.EventID || existing.Message != expected.Message {
		return true
	}
	if existing.Title != expected.Title || existing.Content != expected.Content || existing.Actionable != expected.Actionable {
		return true
	}
	if !notificationStringPtrEqual(existing.ActorID, expected.ActorID) {
		return true
	}
	if !notificationActionKindEqual(existing.ActionKind, expected.ActionKind) {
		return true
	}
	if !notificationResourceTypeEqual(existing.ResourceType, expected.ResourceType) {
		return true
	}
	if !notificationStringPtrEqual(existing.ResourceID, expected.ResourceID) {
		return true
	}
	return !jsonRawMessageEqual(existing.Payload, expected.Payload)
}

func jsonRawMessageEqual(left, right []byte) bool {
	return strings.TrimSpace(string(left)) == strings.TrimSpace(string(right))
}

func notificationActionKindEqual(left, right *domain.NotificationActionKind) bool {
	if left == nil && right == nil {
		return true
	}
	if left == nil || right == nil {
		return false
	}
	return *left == *right
}

func notificationResourceTypeEqual(left, right *domain.NotificationResourceType) bool {
	if left == nil && right == nil {
		return true
	}
	if left == nil || right == nil {
		return false
	}
	return *left == *right
}

func notificationStringPtrEqual(left, right *string) bool {
	if left == nil && right == nil {
		return true
	}
	if left == nil || right == nil {
		return false
	}
	return strings.TrimSpace(*left) == strings.TrimSpace(*right)
}

func derefTrimmedString(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func mergeUsers(target map[string]struct{}, values []string) {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			target[trimmed] = struct{}{}
		}
	}
}

func mapKeys(values map[string]struct{}) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	return keys
}
