package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"note-app/internal/domain"
)

type fakeThreadRepo struct {
	details             map[string]domain.PageCommentThreadDetail
	createdOutboxEvents []domain.OutboxEvent
	createdMentions     [][]domain.PageCommentMessageMention
	replyOutboxEvents   []domain.OutboxEvent
	replyMentions       [][]domain.PageCommentMessageMention
}

type fakeThreadNotificationPreferenceRepo struct {
	preferences map[string]domain.ThreadNotificationPreference
	calls       []string
	writes      []domain.ThreadNotificationPreference
	err         error
}

func (r *fakeThreadNotificationPreferenceRepo) GetThreadNotificationPreference(_ context.Context, threadID, userID string) (*domain.ThreadNotificationPreference, error) {
	r.calls = append(r.calls, threadID+":"+userID)
	if r.err != nil {
		return nil, r.err
	}
	if r.preferences == nil {
		return nil, nil
	}
	preference, ok := r.preferences[threadID+":"+userID]
	if !ok {
		return nil, nil
	}
	copy := preference
	return &copy, nil
}

func (r *fakeThreadNotificationPreferenceRepo) SetThreadNotificationPreference(_ context.Context, preference domain.ThreadNotificationPreference) error {
	r.writes = append(r.writes, preference)
	if r.err != nil {
		return r.err
	}
	if r.preferences == nil {
		r.preferences = map[string]domain.ThreadNotificationPreference{}
	}
	key := preference.ThreadID + ":" + preference.UserID
	if preference.Mode == domain.ThreadNotificationModeAll {
		delete(r.preferences, key)
		return nil
	}
	r.preferences[key] = preference
	return nil
}

func (r *fakeThreadRepo) CreateThread(_ context.Context, thread domain.PageCommentThread, firstMessage domain.PageCommentThreadMessage, mentions []domain.PageCommentMessageMention, outboxEvent domain.OutboxEvent) (domain.PageCommentThreadDetail, error) {
	if r.details == nil {
		r.details = map[string]domain.PageCommentThreadDetail{}
	}
	r.createdOutboxEvents = append(r.createdOutboxEvents, outboxEvent)
	r.createdMentions = append(r.createdMentions, mentions)
	createdEvent := domain.PageCommentThreadEvent{
		ID:        "event-created-" + thread.ID,
		ThreadID:  thread.ID,
		Type:      domain.PageCommentThreadEventTypeCreated,
		ActorID:   &thread.CreatedBy,
		MessageID: &firstMessage.ID,
		CreatedAt: thread.CreatedAt,
	}
	detail := domain.PageCommentThreadDetail{
		Thread:   thread,
		Messages: []domain.PageCommentThreadMessage{firstMessage},
		Events:   []domain.PageCommentThreadEvent{createdEvent},
	}
	r.details[thread.ID] = detail
	return detail, nil
}

func (r *fakeThreadRepo) GetThread(_ context.Context, threadID string) (domain.PageCommentThreadDetail, error) {
	detail, ok := r.details[threadID]
	if !ok {
		return domain.PageCommentThreadDetail{}, domain.ErrNotFound
	}
	return detail, nil
}

func (r *fakeThreadRepo) ListThreads(_ context.Context, pageID string, threadState *domain.PageCommentThreadState, anchorState *domain.PageCommentThreadAnchorState, createdBy *string, hasMissingAnchor *bool, hasOutdatedAnchor *bool, sortMode string, query string, limit int, cursor string) (domain.PageCommentThreadList, error) {
	if cursor == "broken" {
		return domain.PageCommentThreadList{}, fmt.Errorf("%w: invalid cursor", domain.ErrValidation)
	}
	list := domain.PageCommentThreadList{
		Threads: make([]domain.PageCommentThread, 0),
		Counts:  domain.PageCommentThreadFilterCounts{Open: 1, Resolved: 1, Active: 1, Outdated: 1, Missing: 1},
	}
	for _, detail := range r.details {
		if detail.Thread.PageID != pageID {
			continue
		}
		if threadState != nil && detail.Thread.ThreadState != *threadState {
			continue
		}
		if anchorState != nil && detail.Thread.AnchorState != *anchorState {
			continue
		}
		if createdBy != nil && detail.Thread.CreatedBy != *createdBy {
			continue
		}
		if hasMissingAnchor != nil {
			hasMissing := detail.Thread.AnchorState == domain.PageCommentThreadAnchorStateMissing
			if hasMissing != *hasMissingAnchor {
				continue
			}
		}
		if hasOutdatedAnchor != nil {
			hasOutdated := detail.Thread.AnchorState == domain.PageCommentThreadAnchorStateOutdated
			if hasOutdated != *hasOutdatedAnchor {
				continue
			}
		}
		if query != "" {
			matches := strings.Contains(strings.ToLower(detail.Thread.Anchor.QuotedBlockText), strings.ToLower(query))
			if detail.Thread.Anchor.QuotedText != nil && strings.Contains(strings.ToLower(*detail.Thread.Anchor.QuotedText), strings.ToLower(query)) {
				matches = true
			}
			if !matches {
				for _, message := range detail.Messages {
					if strings.Contains(strings.ToLower(message.Body), strings.ToLower(query)) {
						matches = true
						break
					}
				}
			}
			if !matches {
				continue
			}
		}
		detail.Thread.ReplyCount = len(detail.Messages)
		list.Threads = append(list.Threads, detail.Thread)
	}
	sort.Slice(list.Threads, func(i, j int) bool {
		left := list.Threads[i]
		right := list.Threads[j]
		switch sortMode {
		case "newest":
			if !left.CreatedAt.Equal(right.CreatedAt) {
				return left.CreatedAt.After(right.CreatedAt)
			}
		case "oldest":
			if !left.CreatedAt.Equal(right.CreatedAt) {
				return left.CreatedAt.Before(right.CreatedAt)
			}
		default:
			if left.ThreadState != right.ThreadState {
				return left.ThreadState == domain.PageCommentThreadStateOpen
			}
			if !left.LastActivityAt.Equal(right.LastActivityAt) {
				return left.LastActivityAt.After(right.LastActivityAt)
			}
		}
		return left.ID < right.ID
	})
	if cursor != "" {
		start := len(list.Threads)
		for idx, thread := range list.Threads {
			if thread.ID == cursor {
				start = idx + 1
				break
			}
		}
		if start < len(list.Threads) {
			list.Threads = list.Threads[start:]
		} else {
			list.Threads = nil
		}
	}
	if limit > 0 && len(list.Threads) > limit {
		nextCursor := list.Threads[limit-1].ID
		list.NextCursor = &nextCursor
		list.HasMore = true
		list.Threads = list.Threads[:limit]
	}
	return list, nil
}

func (r *fakeThreadRepo) ListWorkspaceThreads(_ context.Context, workspaceID string, threadState *domain.PageCommentThreadState, anchorState *domain.PageCommentThreadAnchorState, createdBy *string, hasMissingAnchor *bool, hasOutdatedAnchor *bool, sortMode string, query string, limit int, cursor string) (domain.WorkspaceCommentThreadList, error) {
	if cursor == "broken" {
		return domain.WorkspaceCommentThreadList{}, fmt.Errorf("%w: invalid cursor", domain.ErrValidation)
	}
	pageList, err := r.ListThreads(context.Background(), "page-1", threadState, anchorState, createdBy, hasMissingAnchor, hasOutdatedAnchor, sortMode, query, limit, cursor)
	if err != nil {
		return domain.WorkspaceCommentThreadList{}, err
	}
	items := make([]domain.WorkspaceCommentThreadListItem, 0, len(pageList.Threads))
	for _, thread := range pageList.Threads {
		items = append(items, domain.WorkspaceCommentThreadListItem{
			Thread: thread,
			Page: domain.PageSummary{
				ID:          thread.PageID,
				WorkspaceID: workspaceID,
				Title:       map[string]string{"page-1": "Doc", "page-2": "Architecture"}[thread.PageID],
			},
		})
	}
	return domain.WorkspaceCommentThreadList{Threads: items, Counts: pageList.Counts, NextCursor: pageList.NextCursor, HasMore: pageList.HasMore}, nil
}

func (r *fakeThreadRepo) AddReply(_ context.Context, threadID string, message domain.PageCommentThreadMessage, mentions []domain.PageCommentMessageMention, updatedThread domain.PageCommentThread, outboxEvent domain.OutboxEvent) (domain.PageCommentThreadDetail, error) {
	detail, ok := r.details[threadID]
	if !ok {
		return domain.PageCommentThreadDetail{}, domain.ErrNotFound
	}
	r.replyOutboxEvents = append(r.replyOutboxEvents, outboxEvent)
	r.replyMentions = append(r.replyMentions, mentions)
	detail.Thread = updatedThread
	detail.Messages = append(detail.Messages, message)
	detail.Thread.ReplyCount = len(detail.Messages)
	r.details[threadID] = detail
	return detail, nil
}

func (r *fakeThreadRepo) UpdateThreadState(_ context.Context, threadID string, updatedThread domain.PageCommentThread, reevaluation *domain.ThreadAnchorReevaluationContext) (domain.PageCommentThreadDetail, error) {
	detail, ok := r.details[threadID]
	if !ok {
		return domain.PageCommentThreadDetail{}, domain.ErrNotFound
	}
	if !stringPtrsEqual(detail.Thread.Anchor.BlockID, updatedThread.Anchor.BlockID) {
		detail.Events = append(detail.Events, domain.PageCommentThreadEvent{
			ID:          "event-anchor-recovered-" + threadID,
			ThreadID:    threadID,
			Type:        domain.PageCommentThreadEventTypeAnchorRecovered,
			FromBlockID: detail.Thread.Anchor.BlockID,
			ToBlockID:   updatedThread.Anchor.BlockID,
			Reason:      reevaluationReasonPtr(reevaluation),
			RevisionID:  reevaluationRevisionIDPtr(reevaluation),
		})
	}
	if detail.Thread.AnchorState != updatedThread.AnchorState {
		detail.Events = append(detail.Events, domain.PageCommentThreadEvent{
			ID:              "event-anchor-" + threadID,
			ThreadID:        threadID,
			Type:            domain.PageCommentThreadEventTypeAnchorStateChanged,
			FromAnchorState: &detail.Thread.AnchorState,
			ToAnchorState:   &updatedThread.AnchorState,
			Reason:          reevaluationReasonPtr(reevaluation),
			RevisionID:      reevaluationRevisionIDPtr(reevaluation),
		})
	}
	detail.Thread = updatedThread
	r.details[threadID] = detail
	return detail, nil
}

func hasEventReason(events []domain.PageCommentThreadEvent, reason domain.PageCommentThreadEventReason) bool {
	for _, event := range events {
		if event.Reason != nil && *event.Reason == reason {
			return true
		}
	}
	return false
}

func hasEventType(events []domain.PageCommentThreadEvent, eventType domain.PageCommentThreadEventType) bool {
	for _, event := range events {
		if event.Type == eventType {
			return true
		}
	}
	return false
}

func stringPtrsEqual(left, right *string) bool {
	if left == nil && right == nil {
		return true
	}
	if left == nil || right == nil {
		return false
	}
	return *left == *right
}

func reevaluationReasonPtr(reevaluation *domain.ThreadAnchorReevaluationContext) *domain.PageCommentThreadEventReason {
	if reevaluation == nil {
		return nil
	}
	return &reevaluation.Reason
}

func reevaluationRevisionIDPtr(reevaluation *domain.ThreadAnchorReevaluationContext) *string {
	if reevaluation == nil {
		return nil
	}
	return reevaluation.RevisionID
}

func TestThreadServiceCreateThread(t *testing.T) {
	memberships := &fakeWorkspaceRepo{memberships: map[string][]domain.WorkspaceMember{
		"workspace-1": {
			{ID: "member-1", WorkspaceID: "workspace-1", UserID: "viewer-1", Role: domain.RoleViewer},
			{ID: "member-2", WorkspaceID: "workspace-1", UserID: "user-2", Role: domain.RoleEditor},
		},
	}, invitations: map[string]domain.WorkspaceInvitation{}, owners: map[string]int{}}
	pages := &fakePageRepo{
		pages: map[string]domain.Page{
			"page-1": {ID: "page-1", WorkspaceID: "workspace-1", Title: "Doc"},
		},
		drafts: map[string]domain.PageDraft{
			"page-1": {PageID: "page-1", Content: json.RawMessage(`[{"id":"block-1","type":"paragraph","children":[{"type":"text","text":"hello world"}]}]`)},
		},
	}
	threads := &fakeThreadRepo{details: map[string]domain.PageCommentThreadDetail{}}
	service := NewThreadService(threads, pages, memberships)
	quotedText := "hello"
	result, err := service.CreateThread(context.Background(), "viewer-1", CreateThreadInput{
		PageID: "page-1",
		Body:   "  Please revise this line  ",
		Anchor: CreateThreadAnchorInput{
			Type:            domain.PageCommentThreadAnchorTypeBlock,
			BlockID:         "block-1",
			QuotedText:      &quotedText,
			QuotedBlockText: "hello world",
		},
	})
	if err != nil {
		t.Fatalf("CreateThread() error = %v", err)
	}
	if result.Thread.ThreadState != domain.PageCommentThreadStateOpen || result.Thread.AnchorState != domain.PageCommentThreadAnchorStateActive {
		t.Fatalf("unexpected thread state: %+v", result.Thread)
	}
	if result.Thread.Anchor.BlockID == nil || *result.Thread.Anchor.BlockID != "block-1" {
		t.Fatalf("unexpected block anchor: %+v", result.Thread.Anchor)
	}
	if result.Thread.Anchor.QuotedText == nil || *result.Thread.Anchor.QuotedText != "hello" {
		t.Fatalf("unexpected quoted text: %+v", result.Thread.Anchor.QuotedText)
	}
	if len(result.Messages) != 1 || result.Messages[0].Body != "Please revise this line" {
		t.Fatalf("unexpected starter message: %+v", result.Messages)
	}
	if len(result.Events) != 1 || result.Events[0].Type != domain.PageCommentThreadEventTypeCreated {
		t.Fatalf("unexpected thread events: %+v", result.Events)
	}
	if len(threads.createdOutboxEvents) != 1 {
		t.Fatalf("expected one outbox event, got %+v", threads.createdOutboxEvents)
	}
	event := threads.createdOutboxEvents[0]
	if event.Topic != domain.OutboxTopicThreadCreated || event.AggregateType != domain.OutboxAggregateTypeThread || event.AggregateID != result.Thread.ID {
		t.Fatalf("unexpected outbox identity: %+v", event)
	}
	if event.IdempotencyKey != "thread_created:"+result.Thread.ID {
		t.Fatalf("unexpected outbox idempotency key: %+v", event)
	}
	if !event.AvailableAt.Equal(result.Thread.CreatedAt) {
		t.Fatalf("expected available_at to match thread created_at, got %+v", event)
	}
	var payload struct {
		ThreadID       string    `json:"thread_id"`
		MessageID      string    `json:"message_id"`
		PageID         string    `json:"page_id"`
		WorkspaceID    string    `json:"workspace_id"`
		ActorID        string    `json:"actor_id"`
		OccurredAt     time.Time `json:"occurred_at"`
		MentionUserIDs []string  `json:"mention_user_ids"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		t.Fatalf("unmarshal outbox payload: %v", err)
	}
	if payload.ThreadID != result.Thread.ID || payload.MessageID != result.Messages[0].ID || payload.PageID != result.Thread.PageID || payload.WorkspaceID != "workspace-1" || payload.ActorID != "viewer-1" || !payload.OccurredAt.Equal(result.Thread.CreatedAt) {
		t.Fatalf("unexpected outbox payload: %+v", payload)
	}
	if len(payload.MentionUserIDs) != 0 {
		t.Fatalf("expected no mention ids in create payload, got %+v", payload.MentionUserIDs)
	}
}

func TestThreadServiceCreateThreadWithMentions(t *testing.T) {
	memberships := &fakeWorkspaceRepo{memberships: map[string][]domain.WorkspaceMember{
		"workspace-1": {
			{ID: "member-1", WorkspaceID: "workspace-1", UserID: "viewer-1", Role: domain.RoleViewer},
			{ID: "member-2", WorkspaceID: "workspace-1", UserID: "user-2", Role: domain.RoleEditor},
			{ID: "member-3", WorkspaceID: "workspace-1", UserID: "user-3", Role: domain.RoleEditor},
		},
	}, invitations: map[string]domain.WorkspaceInvitation{}, owners: map[string]int{}}
	pages := &fakePageRepo{
		pages: map[string]domain.Page{
			"page-1": {ID: "page-1", WorkspaceID: "workspace-1", Title: "Doc"},
		},
		drafts: map[string]domain.PageDraft{
			"page-1": {PageID: "page-1", Content: json.RawMessage(`[{"id":"block-1","type":"paragraph","children":[{"type":"text","text":"hello world"}]}]`)},
		},
	}
	threads := &fakeThreadRepo{details: map[string]domain.PageCommentThreadDetail{}}
	service := NewThreadService(threads, pages, memberships)

	result, err := service.CreateThread(context.Background(), "viewer-1", CreateThreadInput{
		PageID:   "page-1",
		Body:     "Please revise this line",
		Mentions: []string{" user-2 ", "user-3", "user-2"},
		Anchor: CreateThreadAnchorInput{
			Type:            domain.PageCommentThreadAnchorTypeBlock,
			BlockID:         "block-1",
			QuotedBlockText: "hello world",
		},
	})
	if err != nil {
		t.Fatalf("CreateThread() error = %v", err)
	}
	if len(threads.createdMentions) != 1 {
		t.Fatalf("expected mention rows to be passed to repository, got %+v", threads.createdMentions)
	}
	if got := threads.createdMentions[0]; len(got) != 2 || got[0].MentionedUserID != "user-2" || got[1].MentionedUserID != "user-3" || got[0].MessageID != result.Messages[0].ID || got[1].MessageID != result.Messages[0].ID {
		t.Fatalf("unexpected mention rows: %+v", got)
	}
	if len(threads.createdOutboxEvents) != 1 {
		t.Fatalf("expected one outbox event, got %+v", threads.createdOutboxEvents)
	}
	var payload struct {
		MentionUserIDs []string `json:"mention_user_ids"`
	}
	if err := json.Unmarshal(threads.createdOutboxEvents[0].Payload, &payload); err != nil {
		t.Fatalf("unmarshal outbox payload: %v", err)
	}
	if len(payload.MentionUserIDs) != 2 || payload.MentionUserIDs[0] != "user-2" || payload.MentionUserIDs[1] != "user-3" {
		t.Fatalf("unexpected outbox mention ids: %+v", payload.MentionUserIDs)
	}
}

func TestThreadServiceCreateThreadRejectsInvalidMentions(t *testing.T) {
	memberships := &fakeWorkspaceRepo{memberships: map[string][]domain.WorkspaceMember{
		"workspace-1": {
			{ID: "member-1", WorkspaceID: "workspace-1", UserID: "viewer-1", Role: domain.RoleViewer},
			{ID: "member-2", WorkspaceID: "workspace-1", UserID: "user-2", Role: domain.RoleEditor},
		},
	}, invitations: map[string]domain.WorkspaceInvitation{}, owners: map[string]int{}}
	pages := &fakePageRepo{
		pages: map[string]domain.Page{
			"page-1": {ID: "page-1", WorkspaceID: "workspace-1", Title: "Doc"},
		},
		drafts: map[string]domain.PageDraft{
			"page-1": {PageID: "page-1", Content: json.RawMessage(`[{"id":"block-1","type":"paragraph","children":[{"type":"text","text":"hello world"}]}]`)},
		},
	}
	service := NewThreadService(&fakeThreadRepo{}, pages, memberships)

	_, err := service.CreateThread(context.Background(), "viewer-1", CreateThreadInput{
		PageID:   "page-1",
		Body:     "Please revise this line",
		Mentions: []string{"user-2", "missing-user"},
		Anchor: CreateThreadAnchorInput{
			Type:            domain.PageCommentThreadAnchorTypeBlock,
			BlockID:         "block-1",
			QuotedBlockText: "hello world",
		},
	})
	if !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("expected invalid mention validation error, got %v", err)
	}
}

func TestThreadServiceCreateThreadRejectsInvalidAnchorAndBody(t *testing.T) {
	memberships := &fakeWorkspaceRepo{memberships: map[string][]domain.WorkspaceMember{
		"workspace-1": {
			{ID: "member-1", WorkspaceID: "workspace-1", UserID: "viewer-1", Role: domain.RoleViewer},
			{ID: "member-2", WorkspaceID: "workspace-1", UserID: "user-2", Role: domain.RoleEditor},
		},
	}, invitations: map[string]domain.WorkspaceInvitation{}, owners: map[string]int{}}
	pages := &fakePageRepo{
		pages: map[string]domain.Page{
			"page-1": {ID: "page-1", WorkspaceID: "workspace-1", Title: "Doc"},
		},
		drafts: map[string]domain.PageDraft{
			"page-1": {PageID: "page-1", Content: json.RawMessage(`[{"id":"block-1","type":"paragraph","children":[{"type":"text","text":"hello world"}]}]`)},
		},
	}
	service := NewThreadService(&fakeThreadRepo{}, pages, memberships)

	_, err := service.CreateThread(context.Background(), "viewer-1", CreateThreadInput{
		PageID: "page-1",
		Body:   "   ",
		Anchor: CreateThreadAnchorInput{Type: domain.PageCommentThreadAnchorTypeBlock, BlockID: "block-1"},
	})
	if !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("expected body validation error, got %v", err)
	}

	_, err = service.CreateThread(context.Background(), "viewer-1", CreateThreadInput{
		PageID: "page-1",
		Body:   "hello",
		Anchor: CreateThreadAnchorInput{Type: domain.PageCommentThreadAnchorTypePageLegacy, BlockID: "block-1"},
	})
	if !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("expected anchor type validation error, got %v", err)
	}

	_, err = service.CreateThread(context.Background(), "viewer-1", CreateThreadInput{
		PageID: "page-1",
		Body:   "hello",
		Anchor: CreateThreadAnchorInput{Type: domain.PageCommentThreadAnchorTypeBlock, BlockID: "missing"},
	})
	if !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("expected missing block validation error, got %v", err)
	}

	wrongQuote := "missing"
	_, err = service.CreateThread(context.Background(), "viewer-1", CreateThreadInput{
		PageID: "page-1",
		Body:   "hello",
		Anchor: CreateThreadAnchorInput{Type: domain.PageCommentThreadAnchorTypeBlock, BlockID: "block-1", QuotedText: &wrongQuote},
	})
	if !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("expected quoted text validation error, got %v", err)
	}
}

func TestThreadServiceCreateThreadReturnsNotFoundWhenPageIsTrashed(t *testing.T) {
	memberships := &fakeWorkspaceRepo{memberships: map[string][]domain.WorkspaceMember{
		"workspace-1": {
			{ID: "member-1", WorkspaceID: "workspace-1", UserID: "viewer-1", Role: domain.RoleViewer},
		},
	}, invitations: map[string]domain.WorkspaceInvitation{}, owners: map[string]int{}}
	pages := &fakePageRepo{
		pages:  map[string]domain.Page{},
		drafts: map[string]domain.PageDraft{},
	}
	service := NewThreadService(&fakeThreadRepo{}, pages, memberships)

	_, err := service.CreateThread(context.Background(), "viewer-1", CreateThreadInput{
		PageID: "page-1",
		Body:   "hello",
		Anchor: CreateThreadAnchorInput{Type: domain.PageCommentThreadAnchorTypeBlock, BlockID: "block-1"},
	})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected not found for create on trashed page, got %v", err)
	}
}

func TestThreadServiceCreateThreadIgnoresNotificationFailure(t *testing.T) {
	memberships := &fakeWorkspaceRepo{memberships: map[string][]domain.WorkspaceMember{
		"workspace-1": {
			{ID: "member-1", WorkspaceID: "workspace-1", UserID: "viewer-1", Role: domain.RoleViewer},
		},
	}, invitations: map[string]domain.WorkspaceInvitation{}, owners: map[string]int{}}
	pages := &fakePageRepo{
		pages: map[string]domain.Page{
			"page-1": {ID: "page-1", WorkspaceID: "workspace-1", Title: "Doc"},
		},
		drafts: map[string]domain.PageDraft{
			"page-1": {PageID: "page-1", Content: json.RawMessage(`[{"id":"block-1","type":"paragraph","children":[{"type":"text","text":"hello world"}]}]`)},
		},
	}
	threads := &fakeThreadRepo{details: map[string]domain.PageCommentThreadDetail{}}
	service := NewThreadService(threads, pages, memberships)

	_, err := service.CreateThread(context.Background(), "viewer-1", CreateThreadInput{
		PageID: "page-1",
		Body:   "Please revise this line",
		Anchor: CreateThreadAnchorInput{Type: domain.PageCommentThreadAnchorTypeBlock, BlockID: "block-1"},
	})
	if err != nil {
		t.Fatalf("expected create thread to ignore notification failure, got %v", err)
	}
	if len(threads.details) != 1 {
		t.Fatalf("expected thread persistence, got %+v", threads.details)
	}
	if len(threads.createdOutboxEvents) != 1 || threads.createdOutboxEvents[0].Topic != domain.OutboxTopicThreadCreated {
		t.Fatalf("expected outbox event persistence, got %+v", threads.createdOutboxEvents)
	}
}

func TestThreadServiceGetThread(t *testing.T) {
	memberships := &fakeWorkspaceRepo{memberships: map[string][]domain.WorkspaceMember{
		"workspace-1": {
			{ID: "member-1", WorkspaceID: "workspace-1", UserID: "viewer-1", Role: domain.RoleViewer},
		},
	}, invitations: map[string]domain.WorkspaceInvitation{}, owners: map[string]int{}}
	pages := &fakePageRepo{
		pages: map[string]domain.Page{
			"page-1": {ID: "page-1", WorkspaceID: "workspace-1", Title: "Doc"},
		},
		drafts: map[string]domain.PageDraft{
			"page-1": {PageID: "page-1", Content: json.RawMessage(`[]`)},
		},
	}
	blockID := "block-1"
	threads := &fakeThreadRepo{details: map[string]domain.PageCommentThreadDetail{
		"thread-1": {
			Thread: domain.PageCommentThread{
				ID:          "thread-1",
				PageID:      "page-1",
				Anchor:      domain.PageCommentThreadAnchor{Type: domain.PageCommentThreadAnchorTypeBlock, BlockID: &blockID, QuotedBlockText: "hello world"},
				ThreadState: domain.PageCommentThreadStateOpen,
				AnchorState: domain.PageCommentThreadAnchorStateActive,
				CreatedBy:   "viewer-1",
				ReplyCount:  2,
			},
			Messages: []domain.PageCommentThreadMessage{
				{ID: "message-1", ThreadID: "thread-1", Body: "Please revise this line", CreatedBy: "viewer-1"},
				{ID: "message-2", ThreadID: "thread-1", Body: "Second reply", CreatedBy: "viewer-1"},
			},
			Events: []domain.PageCommentThreadEvent{
				{ID: "event-1", ThreadID: "thread-1", Type: domain.PageCommentThreadEventTypeCreated, ActorID: stringPtr("viewer-1")},
			},
		},
	}}
	service := NewThreadService(threads, pages, memberships)

	result, err := service.GetThread(context.Background(), "viewer-1", GetThreadInput{ThreadID: "thread-1"})
	if err != nil {
		t.Fatalf("GetThread() error = %v", err)
	}
	if result.Thread.ID != "thread-1" || len(result.Messages) != 2 || len(result.Events) != 1 || result.Events[0].Type != domain.PageCommentThreadEventTypeCreated {
		t.Fatalf("unexpected thread detail: %+v", result)
	}

	_, err = service.GetThread(context.Background(), "viewer-x", GetThreadInput{ThreadID: "thread-1"})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected not found for non-member, got %v", err)
	}
}

func TestThreadServiceGetThreadReturnsNotFoundWhenPageIsTrashed(t *testing.T) {
	memberships := &fakeWorkspaceRepo{memberships: map[string][]domain.WorkspaceMember{
		"workspace-1": {
			{ID: "member-1", WorkspaceID: "workspace-1", UserID: "viewer-1", Role: domain.RoleViewer},
		},
	}, invitations: map[string]domain.WorkspaceInvitation{}, owners: map[string]int{}}
	pages := &fakePageRepo{
		pages:  map[string]domain.Page{},
		drafts: map[string]domain.PageDraft{},
	}
	blockID := "block-1"
	threads := &fakeThreadRepo{details: map[string]domain.PageCommentThreadDetail{
		"thread-1": {
			Thread: domain.PageCommentThread{
				ID:          "thread-1",
				PageID:      "page-1",
				Anchor:      domain.PageCommentThreadAnchor{Type: domain.PageCommentThreadAnchorTypeBlock, BlockID: &blockID, QuotedBlockText: "hello world"},
				ThreadState: domain.PageCommentThreadStateOpen,
				AnchorState: domain.PageCommentThreadAnchorStateMissing,
				CreatedBy:   "viewer-1",
			},
			Messages: []domain.PageCommentThreadMessage{
				{ID: "message-1", ThreadID: "thread-1", Body: "Please revise this line", CreatedBy: "viewer-1"},
			},
		},
	}}
	service := NewThreadService(threads, pages, memberships)

	_, err := service.GetThread(context.Background(), "viewer-1", GetThreadInput{ThreadID: "thread-1"})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected not found for thread on trashed page, got %v", err)
	}
}

func TestThreadServiceGetNotificationPreference(t *testing.T) {
	now := time.Date(2026, 4, 9, 3, 0, 0, 0, time.UTC)
	memberships := &fakeWorkspaceRepo{memberships: map[string][]domain.WorkspaceMember{
		"workspace-1": {
			{ID: "member-1", WorkspaceID: "workspace-1", UserID: "viewer-1", Role: domain.RoleViewer},
		},
	}, invitations: map[string]domain.WorkspaceInvitation{}, owners: map[string]int{}}
	pages := &fakePageRepo{
		pages: map[string]domain.Page{
			"page-1": {ID: "page-1", WorkspaceID: "workspace-1", Title: "Doc"},
		},
		drafts: map[string]domain.PageDraft{
			"page-1": {PageID: "page-1", Content: json.RawMessage(`[]`)},
		},
	}
	blockID := "block-1"
	threads := &fakeThreadRepo{details: map[string]domain.PageCommentThreadDetail{
		"thread-1": {
			Thread: domain.PageCommentThread{
				ID:          "thread-1",
				PageID:      "page-1",
				Anchor:      domain.PageCommentThreadAnchor{Type: domain.PageCommentThreadAnchorTypeBlock, BlockID: &blockID, QuotedBlockText: "hello world"},
				ThreadState: domain.PageCommentThreadStateOpen,
				AnchorState: domain.PageCommentThreadAnchorStateActive,
				CreatedBy:   "viewer-1",
			},
			Messages: []domain.PageCommentThreadMessage{
				{ID: "message-1", ThreadID: "thread-1", Body: "Please revise this line", CreatedBy: "viewer-1"},
			},
		},
	}}
	prefs := &fakeThreadNotificationPreferenceRepo{
		preferences: map[string]domain.ThreadNotificationPreference{
			"thread-1:viewer-1": {
				ThreadID:  "thread-1",
				UserID:    "viewer-1",
				Mode:      domain.ThreadNotificationModeMentionsOnly,
				CreatedAt: now,
				UpdatedAt: now,
			},
		},
	}
	service := NewThreadService(threads, pages, memberships, prefs)

	result, err := service.GetNotificationPreference(context.Background(), "viewer-1", GetThreadNotificationPreferenceInput{ThreadID: "thread-1"})
	if err != nil {
		t.Fatalf("GetNotificationPreference() error = %v", err)
	}
	if result.ThreadID != "thread-1" || result.Mode != domain.ThreadNotificationModeMentionsOnly {
		t.Fatalf("unexpected preference result: %+v", result)
	}
	if len(prefs.calls) != 1 || prefs.calls[0] != "thread-1:viewer-1" {
		t.Fatalf("expected preference repo to be queried once, got %+v", prefs.calls)
	}

	missingService := NewThreadService(threads, pages, memberships, &fakeThreadNotificationPreferenceRepo{})
	missing, err := missingService.GetNotificationPreference(context.Background(), "viewer-1", GetThreadNotificationPreferenceInput{ThreadID: "thread-1"})
	if err != nil {
		t.Fatalf("GetNotificationPreference() missing error = %v", err)
	}
	if missing.Mode != domain.ThreadNotificationModeAll {
		t.Fatalf("expected default all mode, got %+v", missing)
	}

	_, err = service.GetNotificationPreference(context.Background(), "viewer-x", GetThreadNotificationPreferenceInput{ThreadID: "thread-1"})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected not found for non-member, got %v", err)
	}

	trashedService := NewThreadService(&fakeThreadRepo{details: map[string]domain.PageCommentThreadDetail{
		"thread-1": {
			Thread: domain.PageCommentThread{
				ID:          "thread-1",
				PageID:      "page-missing",
				Anchor:      domain.PageCommentThreadAnchor{Type: domain.PageCommentThreadAnchorTypeBlock, BlockID: &blockID, QuotedBlockText: "hello world"},
				ThreadState: domain.PageCommentThreadStateOpen,
				AnchorState: domain.PageCommentThreadAnchorStateMissing,
				CreatedBy:   "viewer-1",
			},
			Messages: []domain.PageCommentThreadMessage{
				{ID: "message-1", ThreadID: "thread-1", Body: "Please revise this line", CreatedBy: "viewer-1"},
			},
		},
	}}, &fakePageRepo{
		pages:  map[string]domain.Page{},
		drafts: map[string]domain.PageDraft{},
	}, memberships, prefs)
	_, err = trashedService.GetNotificationPreference(context.Background(), "viewer-1", GetThreadNotificationPreferenceInput{ThreadID: "thread-1"})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected not found for trashed page, got %v", err)
	}
}

func TestThreadServiceUpdateNotificationPreference(t *testing.T) {
	memberships := &fakeWorkspaceRepo{memberships: map[string][]domain.WorkspaceMember{
		"workspace-1": {
			{ID: "member-1", WorkspaceID: "workspace-1", UserID: "viewer-1", Role: domain.RoleViewer},
			{ID: "member-2", WorkspaceID: "workspace-1", UserID: "user-2", Role: domain.RoleEditor},
		},
	}, invitations: map[string]domain.WorkspaceInvitation{}, owners: map[string]int{}}
	pages := &fakePageRepo{
		pages: map[string]domain.Page{
			"page-1": {ID: "page-1", WorkspaceID: "workspace-1", Title: "Doc"},
		},
		drafts: map[string]domain.PageDraft{
			"page-1": {PageID: "page-1", Content: json.RawMessage(`[{"id":"block-1","type":"paragraph","children":[{"type":"text","text":"hello world"}]}]`)} ,
		},
	}
	blockID := "block-1"
	prefs := &fakeThreadNotificationPreferenceRepo{preferences: map[string]domain.ThreadNotificationPreference{}}
	threads := &fakeThreadRepo{details: map[string]domain.PageCommentThreadDetail{
		"thread-1": {
			Thread: domain.PageCommentThread{
				ID:          "thread-1",
				PageID:      "page-1",
				Anchor:      domain.PageCommentThreadAnchor{Type: domain.PageCommentThreadAnchorTypeBlock, BlockID: &blockID, QuotedBlockText: "hello world"},
				ThreadState: domain.PageCommentThreadStateOpen,
				AnchorState: domain.PageCommentThreadAnchorStateActive,
				CreatedBy:   "viewer-1",
			},
			Messages: []domain.PageCommentThreadMessage{
				{ID: "message-1", ThreadID: "thread-1", Body: "Please revise this line", CreatedBy: "viewer-1"},
			},
		},
	}}
	service := NewThreadService(threads, pages, memberships, prefs)

	inserted, err := service.UpdateNotificationPreference(context.Background(), "viewer-1", UpdateThreadNotificationPreferenceInput{
		ThreadID: "thread-1",
		Mode:     string(domain.ThreadNotificationModeMentionsOnly),
	})
	if err != nil {
		t.Fatalf("UpdateNotificationPreference() insert error = %v", err)
	}
	if inserted.ThreadID != "thread-1" || inserted.Mode != domain.ThreadNotificationModeMentionsOnly || inserted.UpdatedAt.IsZero() {
		t.Fatalf("unexpected insert result: %+v", inserted)
	}
	if len(prefs.writes) != 1 {
		t.Fatalf("expected one write, got %+v", prefs.writes)
	}
	if got := prefs.preferences["thread-1:viewer-1"]; got.Mode != domain.ThreadNotificationModeMentionsOnly || got.CreatedAt.IsZero() || got.UpdatedAt.IsZero() {
		t.Fatalf("unexpected stored insert preference: %+v", got)
	}

	updated, err := service.UpdateNotificationPreference(context.Background(), "viewer-1", UpdateThreadNotificationPreferenceInput{
		ThreadID: "thread-1",
		Mode:     string(domain.ThreadNotificationModeMute),
	})
	if err != nil {
		t.Fatalf("UpdateNotificationPreference() update error = %v", err)
	}
	if updated.Mode != domain.ThreadNotificationModeMute || updated.UpdatedAt.IsZero() {
		t.Fatalf("unexpected update result: %+v", updated)
	}
	if got := prefs.preferences["thread-1:viewer-1"]; got.Mode != domain.ThreadNotificationModeMute {
		t.Fatalf("unexpected stored update preference: %+v", got)
	}
	if len(prefs.writes) != 2 {
		t.Fatalf("expected two writes, got %+v", prefs.writes)
	}
	if !prefs.writes[0].CreatedAt.Equal(prefs.writes[0].UpdatedAt) || !prefs.writes[1].CreatedAt.Equal(prefs.writes[1].UpdatedAt) {
		t.Fatalf("expected write timestamps to use a single captured now, got %+v", prefs.writes)
	}

	defaulted, err := service.UpdateNotificationPreference(context.Background(), "viewer-1", UpdateThreadNotificationPreferenceInput{
		ThreadID: "thread-1",
		Mode:     string(domain.ThreadNotificationModeAll),
	})
	if err != nil {
		t.Fatalf("UpdateNotificationPreference() delete-to-default error = %v", err)
	}
	if defaulted.Mode != domain.ThreadNotificationModeAll || defaulted.UpdatedAt.IsZero() {
		t.Fatalf("unexpected default result: %+v", defaulted)
	}
	if _, ok := prefs.preferences["thread-1:viewer-1"]; ok {
		t.Fatalf("expected stored preference to be deleted, got %+v", prefs.preferences)
	}

	missingDefault, err := service.UpdateNotificationPreference(context.Background(), "viewer-1", UpdateThreadNotificationPreferenceInput{
		ThreadID: "thread-1",
		Mode:     string(domain.ThreadNotificationModeAll),
	})
	if err != nil {
		t.Fatalf("UpdateNotificationPreference() default without row error = %v", err)
	}
	if missingDefault.Mode != domain.ThreadNotificationModeAll || missingDefault.ThreadID != "thread-1" {
		t.Fatalf("unexpected default without row result: %+v", missingDefault)
	}

	_, err = service.UpdateNotificationPreference(context.Background(), "viewer-1", UpdateThreadNotificationPreferenceInput{
		ThreadID: "thread-1",
		Mode:     "bogus",
	})
	if !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("expected invalid mode validation error, got %v", err)
	}

	_, err = service.UpdateNotificationPreference(context.Background(), "viewer-1", UpdateThreadNotificationPreferenceInput{
		ThreadID: "thread-1",
		Mode:     " ",
	})
	if !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("expected blank mode validation error, got %v", err)
	}

	_, err = service.UpdateNotificationPreference(context.Background(), "viewer-x", UpdateThreadNotificationPreferenceInput{
		ThreadID: "thread-1",
		Mode:     string(domain.ThreadNotificationModeMute),
	})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected hidden 404 for outsider, got %v", err)
	}

	trashedService := NewThreadService(&fakeThreadRepo{details: map[string]domain.PageCommentThreadDetail{
		"thread-trashed": {
			Thread: domain.PageCommentThread{
				ID:          "thread-trashed",
				PageID:      "page-missing",
				Anchor:      domain.PageCommentThreadAnchor{Type: domain.PageCommentThreadAnchorTypeBlock, BlockID: &blockID, QuotedBlockText: "hello world"},
				ThreadState: domain.PageCommentThreadStateOpen,
				AnchorState: domain.PageCommentThreadAnchorStateMissing,
				CreatedBy:   "viewer-1",
			},
			Messages: []domain.PageCommentThreadMessage{{ID: "message-1", ThreadID: "thread-trashed", Body: "Please revise this line", CreatedBy: "viewer-1"}},
		},
	}}, &fakePageRepo{pages: map[string]domain.Page{}, drafts: map[string]domain.PageDraft{}}, memberships, prefs)
	_, err = trashedService.UpdateNotificationPreference(context.Background(), "viewer-1", UpdateThreadNotificationPreferenceInput{
		ThreadID: "thread-trashed",
		Mode:     string(domain.ThreadNotificationModeMute),
	})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected hidden 404 for trashed page, got %v", err)
	}
}

func TestThreadServiceListThreads(t *testing.T) {
	memberships := &fakeWorkspaceRepo{memberships: map[string][]domain.WorkspaceMember{
		"workspace-1": {
			{ID: "member-1", WorkspaceID: "workspace-1", UserID: "viewer-1", Role: domain.RoleViewer},
		},
	}, invitations: map[string]domain.WorkspaceInvitation{}, owners: map[string]int{}}
	pages := &fakePageRepo{
		pages: map[string]domain.Page{
			"page-1": {ID: "page-1", WorkspaceID: "workspace-1", Title: "Doc"},
		},
		drafts: map[string]domain.PageDraft{
			"page-1": {PageID: "page-1", Content: json.RawMessage(`[]`)},
		},
	}
	blockID := "block-1"
	threads := &fakeThreadRepo{details: map[string]domain.PageCommentThreadDetail{
		"thread-open": {
			Thread: domain.PageCommentThread{
				ID:             "thread-open",
				PageID:         "page-1",
				Anchor:         domain.PageCommentThreadAnchor{Type: domain.PageCommentThreadAnchorTypeBlock, BlockID: &blockID, QuotedBlockText: "hello world"},
				ThreadState:    domain.PageCommentThreadStateOpen,
				AnchorState:    domain.PageCommentThreadAnchorStateActive,
				CreatedBy:      "viewer-1",
				CreatedAt:      time.Date(2026, 3, 19, 8, 0, 0, 0, time.UTC),
				LastActivityAt: time.Date(2026, 3, 19, 8, 2, 0, 0, time.UTC),
			},
			Messages: []domain.PageCommentThreadMessage{{ID: "message-1", ThreadID: "thread-open", Body: "Please revise this line", CreatedBy: "viewer-1"}},
		},
		"thread-resolved": {
			Thread: domain.PageCommentThread{
				ID:             "thread-resolved",
				PageID:         "page-1",
				Anchor:         domain.PageCommentThreadAnchor{Type: domain.PageCommentThreadAnchorTypeBlock, BlockID: &blockID, QuotedBlockText: "architecture notes"},
				ThreadState:    domain.PageCommentThreadStateResolved,
				AnchorState:    domain.PageCommentThreadAnchorStateMissing,
				CreatedBy:      "owner-1",
				CreatedAt:      time.Date(2026, 3, 19, 8, 1, 0, 0, time.UTC),
				LastActivityAt: time.Date(2026, 3, 19, 8, 1, 0, 0, time.UTC),
			},
			Messages: []domain.PageCommentThreadMessage{{ID: "message-2", ThreadID: "thread-resolved", Body: "Archived discussion", CreatedBy: "viewer-1"}},
		},
		"thread-outdated": {
			Thread: domain.PageCommentThread{
				ID:             "thread-outdated",
				PageID:         "page-1",
				Anchor:         domain.PageCommentThreadAnchor{Type: domain.PageCommentThreadAnchorTypeBlock, BlockID: &blockID, QuotedBlockText: "stale text"},
				ThreadState:    domain.PageCommentThreadStateOpen,
				AnchorState:    domain.PageCommentThreadAnchorStateOutdated,
				CreatedBy:      "editor-1",
				CreatedAt:      time.Date(2026, 3, 19, 8, 3, 0, 0, time.UTC),
				LastActivityAt: time.Date(2026, 3, 19, 8, 3, 0, 0, time.UTC),
			},
			Messages: []domain.PageCommentThreadMessage{{ID: "message-3", ThreadID: "thread-outdated", Body: "Outdated thread", CreatedBy: "editor-1"}},
		},
	}}
	service := NewThreadService(threads, pages, memberships)
	threadState := domain.PageCommentThreadStateOpen

	result, err := service.ListThreads(context.Background(), "viewer-1", ListThreadsInput{PageID: "page-1", ThreadState: &threadState, Query: " revise "})
	if err != nil {
		t.Fatalf("ListThreads() error = %v", err)
	}
	if len(result.Threads) != 1 || result.Threads[0].ID != "thread-open" {
		t.Fatalf("unexpected list result: %+v", result)
	}
	if result.Threads[0].ReplyCount != 1 {
		t.Fatalf("expected reply_count to be set, got %+v", result.Threads[0])
	}

	quotedText := "hello"
	threads.details["thread-open"] = domain.PageCommentThreadDetail{
		Thread: domain.PageCommentThread{
			ID:          "thread-open",
			PageID:      "page-1",
			Anchor:      domain.PageCommentThreadAnchor{Type: domain.PageCommentThreadAnchorTypeBlock, BlockID: &blockID, QuotedText: &quotedText, QuotedBlockText: "hello world"},
			ThreadState: domain.PageCommentThreadStateOpen,
			AnchorState: domain.PageCommentThreadAnchorStateActive,
			CreatedBy:   "viewer-1",
		},
		Messages: []domain.PageCommentThreadMessage{{ID: "message-1", ThreadID: "thread-open", Body: "Please revise this line", CreatedBy: "viewer-1"}},
	}
	quotedResult, err := service.ListThreads(context.Background(), "viewer-1", ListThreadsInput{PageID: "page-1", Query: " hello "})
	if err != nil {
		t.Fatalf("ListThreads() quoted text error = %v", err)
	}
	if len(quotedResult.Threads) != 1 || quotedResult.Threads[0].ID != "thread-open" {
		t.Fatalf("expected quoted_text search to match thread-open, got %+v", quotedResult.Threads)
	}

	createdByMeResult, err := service.ListThreads(context.Background(), "viewer-1", ListThreadsInput{PageID: "page-1", CreatedByMe: true})
	if err != nil {
		t.Fatalf("ListThreads() created_by=me error = %v", err)
	}
	if len(createdByMeResult.Threads) != 1 || createdByMeResult.Threads[0].ID != "thread-open" {
		t.Fatalf("expected created_by=me filter to match viewer-owned thread, got %+v", createdByMeResult.Threads)
	}

	hasMissingAnchor := true
	missingAnchorResult, err := service.ListThreads(context.Background(), "viewer-1", ListThreadsInput{PageID: "page-1", HasMissingAnchor: &hasMissingAnchor})
	if err != nil {
		t.Fatalf("ListThreads() has_missing_anchor=true error = %v", err)
	}
	if len(missingAnchorResult.Threads) != 1 || missingAnchorResult.Threads[0].ID != "thread-resolved" {
		t.Fatalf("expected has_missing_anchor=true to match missing thread, got %+v", missingAnchorResult.Threads)
	}

	hasNoMissingAnchor := false
	activeAnchorResult, err := service.ListThreads(context.Background(), "viewer-1", ListThreadsInput{PageID: "page-1", HasMissingAnchor: &hasNoMissingAnchor})
	if err != nil {
		t.Fatalf("ListThreads() has_missing_anchor=false error = %v", err)
	}
	if len(activeAnchorResult.Threads) != 2 {
		t.Fatalf("expected has_missing_anchor=false to exclude missing threads, got %+v", activeAnchorResult.Threads)
	}

	hasOutdatedAnchor := true
	outdatedAnchorResult, err := service.ListThreads(context.Background(), "viewer-1", ListThreadsInput{PageID: "page-1", HasOutdatedAnchor: &hasOutdatedAnchor})
	if err != nil {
		t.Fatalf("ListThreads() has_outdated_anchor=true error = %v", err)
	}
	if len(outdatedAnchorResult.Threads) != 1 || outdatedAnchorResult.Threads[0].ID != "thread-outdated" {
		t.Fatalf("expected has_outdated_anchor=true to match outdated thread, got %+v", outdatedAnchorResult.Threads)
	}

	hasNoOutdatedAnchor := false
	noOutdatedAnchorResult, err := service.ListThreads(context.Background(), "viewer-1", ListThreadsInput{PageID: "page-1", HasOutdatedAnchor: &hasNoOutdatedAnchor})
	if err != nil {
		t.Fatalf("ListThreads() has_outdated_anchor=false error = %v", err)
	}
	if len(noOutdatedAnchorResult.Threads) != 2 {
		t.Fatalf("expected has_outdated_anchor=false to exclude outdated thread, got %+v", noOutdatedAnchorResult.Threads)
	}

	newestResult, err := service.ListThreads(context.Background(), "viewer-1", ListThreadsInput{PageID: "page-1", Sort: "newest"})
	if err != nil {
		t.Fatalf("ListThreads() newest sort error = %v", err)
	}
	if len(newestResult.Threads) != 3 || newestResult.Threads[0].ID != "thread-outdated" || newestResult.Threads[1].ID != "thread-resolved" || newestResult.Threads[2].ID != "thread-open" {
		t.Fatalf("expected newest sort order, got %+v", newestResult.Threads)
	}

	oldestResult, err := service.ListThreads(context.Background(), "viewer-1", ListThreadsInput{PageID: "page-1", Sort: "oldest"})
	if err != nil {
		t.Fatalf("ListThreads() oldest sort error = %v", err)
	}
	if len(oldestResult.Threads) != 3 || oldestResult.Threads[0].ID != "thread-open" || oldestResult.Threads[1].ID != "thread-resolved" || oldestResult.Threads[2].ID != "thread-outdated" {
		t.Fatalf("expected oldest sort order, got %+v", oldestResult.Threads)
	}

	recentActivityResult, err := service.ListThreads(context.Background(), "viewer-1", ListThreadsInput{PageID: "page-1", Sort: "recent_activity"})
	if err != nil {
		t.Fatalf("ListThreads() recent_activity sort error = %v", err)
	}
	if len(recentActivityResult.Threads) != 3 || recentActivityResult.Threads[0].ID != "thread-outdated" || recentActivityResult.Threads[1].ID != "thread-open" || recentActivityResult.Threads[2].ID != "thread-resolved" {
		t.Fatalf("expected recent_activity sort order, got %+v", recentActivityResult.Threads)
	}

	paginatedResult, err := service.ListThreads(context.Background(), "viewer-1", ListThreadsInput{PageID: "page-1", Sort: "recent_activity", Limit: 2})
	if err != nil {
		t.Fatalf("ListThreads() paginated error = %v", err)
	}
	if len(paginatedResult.Threads) != 2 || paginatedResult.Threads[0].ID != "thread-outdated" || paginatedResult.Threads[1].ID != "thread-open" {
		t.Fatalf("expected paginated recent_activity threads, got %+v", paginatedResult.Threads)
	}
	if !paginatedResult.HasMore || paginatedResult.NextCursor == nil || *paginatedResult.NextCursor != "thread-open" {
		t.Fatalf("expected pagination cursor after second thread, got %+v", paginatedResult)
	}

	nextPageResult, err := service.ListThreads(context.Background(), "viewer-1", ListThreadsInput{PageID: "page-1", Sort: "recent_activity", Limit: 2, Cursor: *paginatedResult.NextCursor})
	if err != nil {
		t.Fatalf("ListThreads() next page error = %v", err)
	}
	if len(nextPageResult.Threads) != 1 || nextPageResult.Threads[0].ID != "thread-resolved" {
		t.Fatalf("expected cursor page to continue after thread-open, got %+v", nextPageResult.Threads)
	}
	if nextPageResult.HasMore || nextPageResult.NextCursor != nil {
		t.Fatalf("expected final page without more results, got %+v", nextPageResult)
	}

	invalidThreadState := domain.PageCommentThreadState("broken")
	_, err = service.ListThreads(context.Background(), "viewer-1", ListThreadsInput{PageID: "page-1", ThreadState: &invalidThreadState})
	if !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("expected invalid thread state validation error, got %v", err)
	}

	_, err = service.ListThreads(context.Background(), "viewer-1", ListThreadsInput{PageID: "page-1", Sort: "broken"})
	if !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("expected invalid sort validation error, got %v", err)
	}

	_, err = service.ListThreads(context.Background(), "viewer-1", ListThreadsInput{PageID: "page-1", Limit: -1})
	if !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("expected invalid limit validation error, got %v", err)
	}

	_, err = service.ListThreads(context.Background(), "viewer-1", ListThreadsInput{PageID: "page-1", Cursor: "broken"})
	if !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("expected invalid cursor validation error, got %v", err)
	}

	_, err = service.ListThreads(context.Background(), "viewer-x", ListThreadsInput{PageID: "page-1"})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected not found for non-member list, got %v", err)
	}
}

func TestThreadServiceListThreadsReturnsNotFoundWhenPageIsTrashed(t *testing.T) {
	memberships := &fakeWorkspaceRepo{memberships: map[string][]domain.WorkspaceMember{
		"workspace-1": {
			{ID: "member-1", WorkspaceID: "workspace-1", UserID: "viewer-1", Role: domain.RoleViewer},
		},
	}, invitations: map[string]domain.WorkspaceInvitation{}, owners: map[string]int{}}
	pages := &fakePageRepo{
		pages:  map[string]domain.Page{},
		drafts: map[string]domain.PageDraft{},
	}
	service := NewThreadService(&fakeThreadRepo{details: map[string]domain.PageCommentThreadDetail{}}, pages, memberships)

	_, err := service.ListThreads(context.Background(), "viewer-1", ListThreadsInput{PageID: "page-1"})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected not found for trashed page thread list, got %v", err)
	}
}

func TestThreadServiceListWorkspaceThreads(t *testing.T) {
	memberships := &fakeWorkspaceRepo{memberships: map[string][]domain.WorkspaceMember{
		"workspace-1": {
			{ID: "member-1", WorkspaceID: "workspace-1", UserID: "viewer-1", Role: domain.RoleViewer},
		},
	}, invitations: map[string]domain.WorkspaceInvitation{}, owners: map[string]int{}}
	pages := &fakePageRepo{
		pages: map[string]domain.Page{
			"page-1": {ID: "page-1", WorkspaceID: "workspace-1", Title: "Doc"},
			"page-2": {ID: "page-2", WorkspaceID: "workspace-1", Title: "Architecture"},
		},
		drafts: map[string]domain.PageDraft{
			"page-1": {PageID: "page-1", Content: json.RawMessage(`[]`)},
			"page-2": {PageID: "page-2", Content: json.RawMessage(`[]`)},
		},
	}
	blockID := "block-1"
	threads := &fakeThreadRepo{details: map[string]domain.PageCommentThreadDetail{
		"thread-open": {
			Thread: domain.PageCommentThread{
				ID:             "thread-open",
				PageID:         "page-1",
				Anchor:         domain.PageCommentThreadAnchor{Type: domain.PageCommentThreadAnchorTypeBlock, BlockID: &blockID, QuotedBlockText: "hello world"},
				ThreadState:    domain.PageCommentThreadStateOpen,
				AnchorState:    domain.PageCommentThreadAnchorStateActive,
				CreatedBy:      "viewer-1",
				CreatedAt:      time.Date(2026, 3, 19, 8, 0, 0, 0, time.UTC),
				LastActivityAt: time.Date(2026, 3, 19, 8, 2, 0, 0, time.UTC),
			},
			Messages: []domain.PageCommentThreadMessage{{ID: "message-1", ThreadID: "thread-open", Body: "Please revise this line", CreatedBy: "viewer-1"}},
		},
	}}
	service := NewThreadService(threads, pages, memberships)

	result, err := service.ListWorkspaceThreads(context.Background(), "viewer-1", ListWorkspaceThreadsInput{WorkspaceID: "workspace-1", Sort: "recent_activity"})
	if err != nil {
		t.Fatalf("ListWorkspaceThreads() error = %v", err)
	}
	if len(result.Threads) != 1 || result.Threads[0].Thread.ID != "thread-open" || result.Threads[0].Page.ID != "page-1" {
		t.Fatalf("unexpected workspace threads result: %+v", result)
	}

	paginatedResult, err := service.ListWorkspaceThreads(context.Background(), "viewer-1", ListWorkspaceThreadsInput{WorkspaceID: "workspace-1", Sort: "recent_activity", Limit: 1})
	if err != nil {
		t.Fatalf("ListWorkspaceThreads() paginated error = %v", err)
	}
	if len(paginatedResult.Threads) != 1 || paginatedResult.Threads[0].Thread.ID != "thread-open" {
		t.Fatalf("unexpected paginated workspace threads result: %+v", paginatedResult)
	}
	if paginatedResult.HasMore || paginatedResult.NextCursor != nil {
		t.Fatalf("expected single workspace page without more results, got %+v", paginatedResult)
	}

	_, err = service.ListWorkspaceThreads(context.Background(), "viewer-1", ListWorkspaceThreadsInput{WorkspaceID: "workspace-1", Sort: "broken"})
	if !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("expected invalid sort validation error, got %v", err)
	}

	_, err = service.ListWorkspaceThreads(context.Background(), "viewer-1", ListWorkspaceThreadsInput{WorkspaceID: "workspace-1", Limit: 101})
	if !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("expected invalid limit validation error, got %v", err)
	}

	_, err = service.ListWorkspaceThreads(context.Background(), "viewer-1", ListWorkspaceThreadsInput{WorkspaceID: "workspace-1", Cursor: "broken"})
	if !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("expected invalid cursor validation error, got %v", err)
	}

	_, err = service.ListWorkspaceThreads(context.Background(), "viewer-x", ListWorkspaceThreadsInput{WorkspaceID: "workspace-1"})
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected forbidden for non-member workspace inbox, got %v", err)
	}
}

func TestThreadServiceCreateReply(t *testing.T) {
	memberships := &fakeWorkspaceRepo{memberships: map[string][]domain.WorkspaceMember{
		"workspace-1": {
			{ID: "member-1", WorkspaceID: "workspace-1", UserID: "viewer-1", Role: domain.RoleViewer},
			{ID: "member-2", WorkspaceID: "workspace-1", UserID: "user-2", Role: domain.RoleEditor},
		},
	}, invitations: map[string]domain.WorkspaceInvitation{}, owners: map[string]int{}}
	pages := &fakePageRepo{
		pages: map[string]domain.Page{
			"page-1": {ID: "page-1", WorkspaceID: "workspace-1", Title: "Doc"},
		},
		drafts: map[string]domain.PageDraft{
			"page-1": {PageID: "page-1", Content: json.RawMessage(`[]`)},
		},
	}
	blockID := "block-1"
	resolvedBy := "editor-1"
	resolvedAt := time.Date(2026, 3, 19, 8, 0, 0, 0, time.UTC)
	threads := &fakeThreadRepo{details: map[string]domain.PageCommentThreadDetail{
		"thread-1": {
			Thread: domain.PageCommentThread{
				ID:             "thread-1",
				PageID:         "page-1",
				Anchor:         domain.PageCommentThreadAnchor{Type: domain.PageCommentThreadAnchorTypeBlock, BlockID: &blockID, QuotedBlockText: "hello world"},
				ThreadState:    domain.PageCommentThreadStateResolved,
				AnchorState:    domain.PageCommentThreadAnchorStateActive,
				CreatedBy:      "viewer-1",
				CreatedAt:      resolvedAt.Add(-time.Minute),
				ResolvedBy:     &resolvedBy,
				ResolvedAt:     &resolvedAt,
				ResolveNote:    stringPtr("Already fixed"),
				ReopenReason:   stringPtr("Old reason"),
				LastActivityAt: resolvedAt,
				ReplyCount:     1,
			},
			Messages: []domain.PageCommentThreadMessage{
				{ID: "message-1", ThreadID: "thread-1", Body: "Initial comment", CreatedBy: "viewer-1", CreatedAt: resolvedAt.Add(-time.Minute)},
			},
		},
	}}
	service := NewThreadService(threads, pages, memberships)

	result, err := service.CreateReply(context.Background(), "viewer-1", CreateThreadReplyInput{
		ThreadID: "thread-1",
		Body:     "  Follow-up reply  ",
		Mentions: []string{" viewer-1 ", "user-2"},
	})
	if err != nil {
		t.Fatalf("CreateReply() error = %v", err)
	}
	if result.Thread.ThreadState != domain.PageCommentThreadStateOpen {
		t.Fatalf("expected resolved thread to reopen, got %+v", result.Thread)
	}
	if result.Thread.ReopenedBy == nil || *result.Thread.ReopenedBy != "viewer-1" || result.Thread.ReopenedAt == nil {
		t.Fatalf("expected reopened markers, got %+v", result.Thread)
	}
	if result.Thread.ResolvedBy != nil || result.Thread.ResolvedAt != nil {
		t.Fatalf("expected resolved markers cleared, got %+v", result.Thread)
	}
	if result.Thread.ResolveNote != nil || result.Thread.ReopenReason != nil {
		t.Fatalf("expected auto-reopen reply to clear stale note metadata, got %+v", result.Thread)
	}
	if len(result.Messages) != 2 || result.Messages[1].Body != "Follow-up reply" {
		t.Fatalf("unexpected reply payload: %+v", result.Messages)
	}
	if result.Thread.ReplyCount != 2 {
		t.Fatalf("expected reply_count 2, got %+v", result.Thread)
	}
	if len(threads.replyMentions) != 1 {
		t.Fatalf("expected one reply mention batch, got %+v", threads.replyMentions)
	}
	if got := threads.replyMentions[0]; len(got) != 2 || got[0].MessageID != result.Messages[1].ID || got[1].MessageID != result.Messages[1].ID || got[0].MentionedUserID != "viewer-1" || got[1].MentionedUserID != "user-2" {
		t.Fatalf("unexpected reply mention rows: %+v", got)
	}
	if len(threads.replyOutboxEvents) != 1 {
		t.Fatalf("expected one reply outbox event, got %+v", threads.replyOutboxEvents)
	}
	replyEvent := threads.replyOutboxEvents[0]
	if replyEvent.Topic != domain.OutboxTopicThreadReplyCreated || replyEvent.AggregateType != domain.OutboxAggregateTypeThreadMessage || replyEvent.AggregateID != result.Messages[1].ID {
		t.Fatalf("unexpected reply outbox identity: %+v", replyEvent)
	}
	if replyEvent.IdempotencyKey != "thread_reply_created:"+result.Messages[1].ID {
		t.Fatalf("unexpected reply outbox idempotency key: %+v", replyEvent)
	}
	if !replyEvent.AvailableAt.Equal(result.Messages[1].CreatedAt) {
		t.Fatalf("expected reply outbox available_at to match message created_at, got %+v", replyEvent)
	}
	var replyPayload struct {
		ThreadID       string    `json:"thread_id"`
		MessageID      string    `json:"message_id"`
		PageID         string    `json:"page_id"`
		WorkspaceID    string    `json:"workspace_id"`
		ActorID        string    `json:"actor_id"`
		OccurredAt     time.Time `json:"occurred_at"`
		MentionUserIDs []string  `json:"mention_user_ids"`
	}
	if err := json.Unmarshal(replyEvent.Payload, &replyPayload); err != nil {
		t.Fatalf("unmarshal reply outbox payload: %v", err)
	}
	if replyPayload.ThreadID != result.Thread.ID || replyPayload.MessageID != result.Messages[1].ID || replyPayload.PageID != result.Thread.PageID || replyPayload.WorkspaceID != "workspace-1" || replyPayload.ActorID != "viewer-1" || !replyPayload.OccurredAt.Equal(result.Messages[1].CreatedAt) {
		t.Fatalf("unexpected reply outbox payload: %+v", replyPayload)
	}
	if len(replyPayload.MentionUserIDs) != 2 || replyPayload.MentionUserIDs[0] != "viewer-1" || replyPayload.MentionUserIDs[1] != "user-2" {
		t.Fatalf("unexpected reply outbox mention ids: %+v", replyPayload.MentionUserIDs)
	}
	_, err = service.CreateReply(context.Background(), "viewer-1", CreateThreadReplyInput{ThreadID: "thread-1", Body: "   "})
	if !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("expected empty reply validation error, got %v", err)
	}

	_, err = service.CreateReply(context.Background(), "viewer-x", CreateThreadReplyInput{ThreadID: "thread-1", Body: "reply"})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected not found for non-member reply, got %v", err)
	}
}

func TestThreadServiceCreateReplyAllowsSelfMention(t *testing.T) {
	memberships := &fakeWorkspaceRepo{memberships: map[string][]domain.WorkspaceMember{
		"workspace-1": {
			{ID: "member-1", WorkspaceID: "workspace-1", UserID: "viewer-1", Role: domain.RoleViewer},
		},
	}, invitations: map[string]domain.WorkspaceInvitation{}, owners: map[string]int{}}
	pages := &fakePageRepo{
		pages: map[string]domain.Page{
			"page-1": {ID: "page-1", WorkspaceID: "workspace-1", Title: "Doc"},
		},
		drafts: map[string]domain.PageDraft{
			"page-1": {PageID: "page-1", Content: json.RawMessage(`[]`)},
		},
	}
	blockID := "block-1"
	resolvedAt := time.Date(2026, 3, 19, 8, 0, 0, 0, time.UTC)
	threads := &fakeThreadRepo{details: map[string]domain.PageCommentThreadDetail{
		"thread-1": {
			Thread: domain.PageCommentThread{
				ID:             "thread-1",
				PageID:         "page-1",
				Anchor:         domain.PageCommentThreadAnchor{Type: domain.PageCommentThreadAnchorTypeBlock, BlockID: &blockID, QuotedBlockText: "hello world"},
				ThreadState:    domain.PageCommentThreadStateResolved,
				AnchorState:    domain.PageCommentThreadAnchorStateActive,
				CreatedBy:      "viewer-1",
				CreatedAt:      resolvedAt.Add(-time.Minute),
				ResolvedBy:     stringPtr("editor-1"),
				ResolvedAt:     &resolvedAt,
				LastActivityAt: resolvedAt,
				ReplyCount:     1,
			},
			Messages: []domain.PageCommentThreadMessage{
				{ID: "message-1", ThreadID: "thread-1", Body: "Initial comment", CreatedBy: "viewer-1", CreatedAt: resolvedAt.Add(-time.Minute)},
			},
		},
	}}
	service := NewThreadService(threads, pages, memberships)

	result, err := service.CreateReply(context.Background(), "viewer-1", CreateThreadReplyInput{
		ThreadID: "thread-1",
		Body:     "Self mention reply",
		Mentions: []string{" viewer-1 "},
	})
	if err != nil {
		t.Fatalf("CreateReply() error = %v", err)
	}
	if result.Thread.ThreadState != domain.PageCommentThreadStateOpen || result.Thread.ReopenedBy == nil || *result.Thread.ReopenedBy != "viewer-1" {
		t.Fatalf("expected resolved thread to reopen, got %+v", result.Thread)
	}
	if len(threads.replyMentions) != 1 || len(threads.replyMentions[0]) != 1 || threads.replyMentions[0][0].MentionedUserID != "viewer-1" {
		t.Fatalf("expected self mention to persist once, got %+v", threads.replyMentions)
	}
	var replyPayload struct {
		MentionUserIDs []string `json:"mention_user_ids"`
	}
	if err := json.Unmarshal(threads.replyOutboxEvents[0].Payload, &replyPayload); err != nil {
		t.Fatalf("unmarshal reply outbox payload: %v", err)
	}
	if len(replyPayload.MentionUserIDs) != 1 || replyPayload.MentionUserIDs[0] != "viewer-1" {
		t.Fatalf("expected self mention to normalize into outbox payload, got %+v", replyPayload.MentionUserIDs)
	}
}

func TestThreadServiceCreateReplyReturnsNotFoundWhenPageIsTrashed(t *testing.T) {
	memberships := &fakeWorkspaceRepo{memberships: map[string][]domain.WorkspaceMember{
		"workspace-1": {
			{ID: "member-1", WorkspaceID: "workspace-1", UserID: "viewer-1", Role: domain.RoleViewer},
		},
	}, invitations: map[string]domain.WorkspaceInvitation{}, owners: map[string]int{}}
	pages := &fakePageRepo{
		pages:  map[string]domain.Page{},
		drafts: map[string]domain.PageDraft{},
	}
	blockID := "block-1"
	threads := &fakeThreadRepo{details: map[string]domain.PageCommentThreadDetail{
		"thread-1": {
			Thread: domain.PageCommentThread{
				ID:          "thread-1",
				PageID:      "page-1",
				Anchor:      domain.PageCommentThreadAnchor{Type: domain.PageCommentThreadAnchorTypeBlock, BlockID: &blockID, QuotedBlockText: "hello world"},
				ThreadState: domain.PageCommentThreadStateOpen,
				AnchorState: domain.PageCommentThreadAnchorStateMissing,
				CreatedBy:   "viewer-1",
				ReplyCount:  1,
			},
			Messages: []domain.PageCommentThreadMessage{
				{ID: "message-1", ThreadID: "thread-1", Body: "Initial comment", CreatedBy: "viewer-1"},
			},
		},
	}}
	service := NewThreadService(threads, pages, memberships)

	_, err := service.CreateReply(context.Background(), "viewer-1", CreateThreadReplyInput{ThreadID: "thread-1", Body: "reply"})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected not found for reply on trashed page thread, got %v", err)
	}
}

func TestThreadServiceCreateReplyPersistsOutboxEvent(t *testing.T) {
	memberships := &fakeWorkspaceRepo{memberships: map[string][]domain.WorkspaceMember{
		"workspace-1": {
			{ID: "member-1", WorkspaceID: "workspace-1", UserID: "viewer-1", Role: domain.RoleViewer},
		},
	}, invitations: map[string]domain.WorkspaceInvitation{}, owners: map[string]int{}}
	pages := &fakePageRepo{
		pages: map[string]domain.Page{
			"page-1": {ID: "page-1", WorkspaceID: "workspace-1", Title: "Doc"},
		},
		drafts: map[string]domain.PageDraft{
			"page-1": {PageID: "page-1", Content: json.RawMessage(`[]`)},
		},
	}
	blockID := "block-1"
	threads := &fakeThreadRepo{details: map[string]domain.PageCommentThreadDetail{
		"thread-1": {
			Thread: domain.PageCommentThread{
				ID:          "thread-1",
				PageID:      "page-1",
				Anchor:      domain.PageCommentThreadAnchor{Type: domain.PageCommentThreadAnchorTypeBlock, BlockID: &blockID, QuotedBlockText: "hello world"},
				ThreadState: domain.PageCommentThreadStateOpen,
				AnchorState: domain.PageCommentThreadAnchorStateActive,
				CreatedBy:   "viewer-1",
				ReplyCount:  1,
			},
			Messages: []domain.PageCommentThreadMessage{
				{ID: "message-1", ThreadID: "thread-1", Body: "Initial comment", CreatedBy: "viewer-1"},
			},
		},
	}}
	service := NewThreadService(threads, pages, memberships)

	result, err := service.CreateReply(context.Background(), "viewer-1", CreateThreadReplyInput{ThreadID: "thread-1", Body: "reply"})
	if err != nil {
		t.Fatalf("expected reply success, got %v", err)
	}
	if len(threads.details["thread-1"].Messages) != 2 {
		t.Fatalf("expected reply persistence, got %+v", threads.details["thread-1"])
	}
	if len(threads.replyOutboxEvents) != 1 {
		t.Fatalf("expected reply outbox event, got %+v", threads.replyOutboxEvents)
	}
	event := threads.replyOutboxEvents[0]
	if event.Topic != domain.OutboxTopicThreadReplyCreated || event.AggregateType != domain.OutboxAggregateTypeThreadMessage || event.AggregateID != result.Messages[1].ID {
		t.Fatalf("unexpected reply outbox identity: %+v", event)
	}
	if len(threads.replyMentions) != 1 || len(threads.replyMentions[0]) != 0 {
		t.Fatalf("expected no reply mentions, got %+v", threads.replyMentions)
	}
	var payload struct {
		MentionUserIDs []string `json:"mention_user_ids"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		t.Fatalf("unmarshal reply outbox payload: %v", err)
	}
	if len(payload.MentionUserIDs) != 0 {
		t.Fatalf("expected empty reply outbox mention ids, got %+v", payload.MentionUserIDs)
	}
}

func TestThreadServiceCreateReplyRejectsInvalidMentions(t *testing.T) {
	memberships := &fakeWorkspaceRepo{memberships: map[string][]domain.WorkspaceMember{
		"workspace-1": {
			{ID: "member-1", WorkspaceID: "workspace-1", UserID: "viewer-1", Role: domain.RoleViewer},
			{ID: "member-2", WorkspaceID: "workspace-1", UserID: "user-2", Role: domain.RoleEditor},
		},
	}, invitations: map[string]domain.WorkspaceInvitation{}, owners: map[string]int{}}
	pages := &fakePageRepo{
		pages: map[string]domain.Page{
			"page-1": {ID: "page-1", WorkspaceID: "workspace-1", Title: "Doc"},
		},
		drafts: map[string]domain.PageDraft{
			"page-1": {PageID: "page-1", Content: json.RawMessage(`[]`)},
		},
	}
	blockID := "block-1"
	service := NewThreadService(&fakeThreadRepo{details: map[string]domain.PageCommentThreadDetail{
		"thread-1": {
			Thread: domain.PageCommentThread{
				ID:             "thread-1",
				PageID:         "page-1",
				Anchor:         domain.PageCommentThreadAnchor{Type: domain.PageCommentThreadAnchorTypeBlock, BlockID: &blockID, QuotedBlockText: "hello world"},
				ThreadState:    domain.PageCommentThreadStateOpen,
				AnchorState:    domain.PageCommentThreadAnchorStateActive,
				CreatedBy:      "viewer-1",
				LastActivityAt: time.Date(2026, 3, 19, 8, 0, 0, 0, time.UTC),
				ReplyCount:     1,
			},
			Messages: []domain.PageCommentThreadMessage{{ID: "message-1", ThreadID: "thread-1", Body: "Initial comment", CreatedBy: "viewer-1"}},
		},
	}}, pages, memberships)

	for _, tt := range []struct {
		name     string
		mentions []string
	}{
		{name: "blank", mentions: []string{" "}},
		{name: "non_member", mentions: []string{"user-x"}},
		{name: "too_many", mentions: func() []string {
			values := make([]string, 0, maxThreadMentionUserIDs+1)
			for i := 0; i < maxThreadMentionUserIDs+1; i++ {
				values = append(values, fmt.Sprintf("user-%d", i))
			}
			return values
		}()},
	} {
		t.Run(tt.name, func(t *testing.T) {
			repo := service.threads.(*fakeThreadRepo)
			repo.replyOutboxEvents = nil
			repo.replyMentions = nil
			_, err := service.CreateReply(context.Background(), "viewer-1", CreateThreadReplyInput{ThreadID: "thread-1", Body: "reply", Mentions: tt.mentions})
			if !errors.Is(err, domain.ErrValidation) {
				t.Fatalf("expected validation error, got %v", err)
			}
			if len(repo.replyOutboxEvents) != 0 || len(repo.replyMentions) != 0 {
				t.Fatalf("expected repository not to be called on invalid mentions, got outboxes=%d mentions=%d", len(repo.replyOutboxEvents), len(repo.replyMentions))
			}
		})
	}
}

func TestThreadServiceResolveAndReopenThread(t *testing.T) {
	memberships := &fakeWorkspaceRepo{memberships: map[string][]domain.WorkspaceMember{
		"workspace-1": {
			{ID: "member-owner", WorkspaceID: "workspace-1", UserID: "owner-1", Role: domain.RoleOwner},
			{ID: "member-viewer", WorkspaceID: "workspace-1", UserID: "viewer-1", Role: domain.RoleViewer},
		},
	}, invitations: map[string]domain.WorkspaceInvitation{}, owners: map[string]int{}}
	pages := &fakePageRepo{
		pages: map[string]domain.Page{
			"page-1": {ID: "page-1", WorkspaceID: "workspace-1", Title: "Doc"},
		},
		drafts: map[string]domain.PageDraft{
			"page-1": {PageID: "page-1", Content: json.RawMessage(`[]`)},
		},
	}
	blockID := "block-1"
	threads := &fakeThreadRepo{details: map[string]domain.PageCommentThreadDetail{
		"thread-1": {
			Thread: domain.PageCommentThread{
				ID:          "thread-1",
				PageID:      "page-1",
				Anchor:      domain.PageCommentThreadAnchor{Type: domain.PageCommentThreadAnchorTypeBlock, BlockID: &blockID, QuotedBlockText: "hello world"},
				ThreadState: domain.PageCommentThreadStateOpen,
				AnchorState: domain.PageCommentThreadAnchorStateActive,
				CreatedBy:   "viewer-1",
			},
			Messages: []domain.PageCommentThreadMessage{
				{ID: "message-1", ThreadID: "thread-1", Body: "Initial comment", CreatedBy: "viewer-1"},
			},
		},
	}}
	service := NewThreadService(threads, pages, memberships)

	resolved, err := service.ResolveThread(context.Background(), "owner-1", ResolveThreadInput{
		ThreadID:    "thread-1",
		ResolveNote: "  Fixed in latest revision  ",
	})
	if err != nil {
		t.Fatalf("ResolveThread() error = %v", err)
	}
	if resolved.Thread.ThreadState != domain.PageCommentThreadStateResolved || resolved.Thread.ResolvedBy == nil || *resolved.Thread.ResolvedBy != "owner-1" || resolved.Thread.ResolvedAt == nil {
		t.Fatalf("expected resolved markers, got %+v", resolved.Thread)
	}
	if resolved.Thread.ResolveNote == nil || *resolved.Thread.ResolveNote != "Fixed in latest revision" {
		t.Fatalf("expected trimmed resolve note, got %+v", resolved.Thread)
	}
	if !resolved.Thread.LastActivityAt.Equal(*resolved.Thread.ResolvedAt) {
		t.Fatalf("expected resolve to update last_activity_at, got %+v", resolved.Thread)
	}

	idempotentResolved, err := service.ResolveThread(context.Background(), "owner-1", ResolveThreadInput{ThreadID: "thread-1"})
	if err != nil {
		t.Fatalf("ResolveThread() idempotent error = %v", err)
	}
	if idempotentResolved.Thread.ResolvedBy == nil || *idempotentResolved.Thread.ResolvedBy != "owner-1" {
		t.Fatalf("expected idempotent resolved result, got %+v", idempotentResolved.Thread)
	}

	_, err = service.ResolveThread(context.Background(), "viewer-1", ResolveThreadInput{ThreadID: "thread-1"})
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected viewer resolve forbidden, got %v", err)
	}

	reopened, err := service.ReopenThread(context.Background(), "viewer-1", ReopenThreadInput{ThreadID: "thread-1"})
	if err != nil {
		t.Fatalf("ReopenThread() error = %v", err)
	}
	if reopened.Thread.ThreadState != domain.PageCommentThreadStateOpen || reopened.Thread.ReopenedBy == nil || *reopened.Thread.ReopenedBy != "viewer-1" || reopened.Thread.ReopenedAt == nil {
		t.Fatalf("expected reopened markers, got %+v", reopened.Thread)
	}
	if reopened.Thread.ResolvedBy != nil || reopened.Thread.ResolvedAt != nil {
		t.Fatalf("expected resolved markers cleared, got %+v", reopened.Thread)
	}
	if !reopened.Thread.LastActivityAt.Equal(*reopened.Thread.ReopenedAt) {
		t.Fatalf("expected reopen to update last_activity_at, got %+v", reopened.Thread)
	}
	if reopened.Thread.ResolveNote != nil {
		t.Fatalf("expected reopen to clear resolve note, got %+v", reopened.Thread)
	}
	if reopened.Thread.ReopenReason != nil {
		t.Fatalf("expected empty reopen reason by default, got %+v", reopened.Thread.ReopenReason)
	}

	resolvedAgain, err := service.ResolveThread(context.Background(), "owner-1", ResolveThreadInput{
		ThreadID:    "thread-1",
		ResolveNote: "Needs no more follow-up",
	})
	if err != nil {
		t.Fatalf("ResolveThread() second resolve error = %v", err)
	}
	if resolvedAgain.Thread.ResolveNote == nil || *resolvedAgain.Thread.ResolveNote != "Needs no more follow-up" {
		t.Fatalf("expected stored resolve note, got %+v", resolvedAgain.Thread)
	}

	reopenedWithReason, err := service.ReopenThread(context.Background(), "viewer-1", ReopenThreadInput{
		ThreadID:     "thread-1",
		ReopenReason: "  Follow-up requested  ",
	})
	if err != nil {
		t.Fatalf("ReopenThread() with reason error = %v", err)
	}
	if reopenedWithReason.Thread.ReopenReason == nil || *reopenedWithReason.Thread.ReopenReason != "Follow-up requested" {
		t.Fatalf("expected trimmed reopen reason, got %+v", reopenedWithReason.Thread)
	}
	if reopenedWithReason.Thread.ResolveNote != nil {
		t.Fatalf("expected reopen to clear resolve note, got %+v", reopenedWithReason.Thread)
	}

	idempotentReopen, err := service.ReopenThread(context.Background(), "viewer-1", ReopenThreadInput{ThreadID: "thread-1"})
	if err != nil {
		t.Fatalf("ReopenThread() idempotent error = %v", err)
	}
	if idempotentReopen.Thread.ThreadState != domain.PageCommentThreadStateOpen {
		t.Fatalf("expected idempotent open result, got %+v", idempotentReopen.Thread)
	}

	_, err = service.ResolveThread(context.Background(), "viewer-x", ResolveThreadInput{ThreadID: "thread-1"})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected non-member resolve not found, got %v", err)
	}

	_, err = service.ReopenThread(context.Background(), "viewer-x", ReopenThreadInput{ThreadID: "thread-1"})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected non-member reopen not found, got %v", err)
	}
}

func TestThreadServiceCreateThreadReturnsNotFoundForNonMember(t *testing.T) {
	memberships := &fakeWorkspaceRepo{memberships: map[string][]domain.WorkspaceMember{
		"workspace-1": {
			{ID: "member-1", WorkspaceID: "workspace-1", UserID: "viewer-1", Role: domain.RoleViewer},
		},
	}, invitations: map[string]domain.WorkspaceInvitation{}, owners: map[string]int{}}
	pages := &fakePageRepo{
		pages: map[string]domain.Page{
			"page-1": {ID: "page-1", WorkspaceID: "workspace-1", Title: "Doc"},
		},
		drafts: map[string]domain.PageDraft{
			"page-1": {PageID: "page-1", Content: json.RawMessage(`[{"id":"block-1","type":"paragraph","children":[{"type":"text","text":"hello world"}]}]`)},
		},
	}
	service := NewThreadService(&fakeThreadRepo{details: map[string]domain.PageCommentThreadDetail{}}, pages, memberships)

	_, err := service.CreateThread(context.Background(), "viewer-x", CreateThreadInput{
		PageID: "page-1",
		Body:   "Please revise this line",
		Anchor: CreateThreadAnchorInput{Type: domain.PageCommentThreadAnchorTypeBlock, BlockID: "block-1"},
	})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected not found for non-member create, got %v", err)
	}
}

func TestThreadServiceResolveAndReopenReturnNotFoundWhenPageIsTrashed(t *testing.T) {
	memberships := &fakeWorkspaceRepo{memberships: map[string][]domain.WorkspaceMember{
		"workspace-1": {
			{ID: "member-owner", WorkspaceID: "workspace-1", UserID: "owner-1", Role: domain.RoleOwner},
			{ID: "member-viewer", WorkspaceID: "workspace-1", UserID: "viewer-1", Role: domain.RoleViewer},
		},
	}, invitations: map[string]domain.WorkspaceInvitation{}, owners: map[string]int{}}
	pages := &fakePageRepo{
		pages:  map[string]domain.Page{},
		drafts: map[string]domain.PageDraft{},
	}
	blockID := "block-1"
	threads := &fakeThreadRepo{details: map[string]domain.PageCommentThreadDetail{
		"thread-1": {
			Thread: domain.PageCommentThread{
				ID:          "thread-1",
				PageID:      "page-1",
				Anchor:      domain.PageCommentThreadAnchor{Type: domain.PageCommentThreadAnchorTypeBlock, BlockID: &blockID, QuotedBlockText: "hello world"},
				ThreadState: domain.PageCommentThreadStateOpen,
				AnchorState: domain.PageCommentThreadAnchorStateMissing,
				CreatedBy:   "viewer-1",
			},
			Messages: []domain.PageCommentThreadMessage{
				{ID: "message-1", ThreadID: "thread-1", Body: "Initial comment", CreatedBy: "viewer-1"},
			},
		},
	}}
	service := NewThreadService(threads, pages, memberships)

	_, err := service.ResolveThread(context.Background(), "owner-1", ResolveThreadInput{ThreadID: "thread-1"})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected not found for resolve on trashed page thread, got %v", err)
	}

	_, err = service.ReopenThread(context.Background(), "viewer-1", ReopenThreadInput{ThreadID: "thread-1"})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected not found for reopen on trashed page thread, got %v", err)
	}
}

func TestThreadServiceReevaluatePageAnchors(t *testing.T) {
	memberships := &fakeWorkspaceRepo{memberships: map[string][]domain.WorkspaceMember{}, invitations: map[string]domain.WorkspaceInvitation{}, owners: map[string]int{}}
	pages := &fakePageRepo{pages: map[string]domain.Page{}, drafts: map[string]domain.PageDraft{}}
	blockOne := "block-1"
	blockTwo := "block-2"
	blockThree := "block-3"
	blockFour := "block-4"
	threads := &fakeThreadRepo{details: map[string]domain.PageCommentThreadDetail{
		"thread-active": {
			Thread: domain.PageCommentThread{
				ID:          "thread-active",
				PageID:      "page-1",
				Anchor:      domain.PageCommentThreadAnchor{Type: domain.PageCommentThreadAnchorTypeBlock, BlockID: &blockOne, QuotedBlockText: "hello world"},
				ThreadState: domain.PageCommentThreadStateOpen,
				AnchorState: domain.PageCommentThreadAnchorStateActive,
			},
		},
		"thread-outdated": {
			Thread: domain.PageCommentThread{
				ID:          "thread-outdated",
				PageID:      "page-1",
				Anchor:      domain.PageCommentThreadAnchor{Type: domain.PageCommentThreadAnchorTypeBlock, BlockID: &blockTwo, QuotedBlockText: "hello world"},
				ThreadState: domain.PageCommentThreadStateOpen,
				AnchorState: domain.PageCommentThreadAnchorStateActive,
			},
		},
		"thread-missing": {
			Thread: domain.PageCommentThread{
				ID:          "thread-missing",
				PageID:      "page-1",
				Anchor:      domain.PageCommentThreadAnchor{Type: domain.PageCommentThreadAnchorTypeBlock, BlockID: &blockThree, QuotedBlockText: "gone"},
				ThreadState: domain.PageCommentThreadStateOpen,
				AnchorState: domain.PageCommentThreadAnchorStateActive,
			},
		},
		"thread-reanchor": {
			Thread: domain.PageCommentThread{
				ID:          "thread-reanchor",
				PageID:      "page-1",
				Anchor:      domain.PageCommentThreadAnchor{Type: domain.PageCommentThreadAnchorTypeBlock, BlockID: &blockFour, QuotedBlockText: "moved text"},
				ThreadState: domain.PageCommentThreadStateOpen,
				AnchorState: domain.PageCommentThreadAnchorStateActive,
			},
		},
		"thread-quoted-reanchor": {
			Thread: domain.PageCommentThread{
				ID:          "thread-quoted-reanchor",
				PageID:      "page-1",
				Anchor:      domain.PageCommentThreadAnchor{Type: domain.PageCommentThreadAnchorTypeBlock, BlockID: stringPtr("block-5"), QuotedText: stringPtr("target sentence"), QuotedBlockText: "full original block"},
				ThreadState: domain.PageCommentThreadStateOpen,
				AnchorState: domain.PageCommentThreadAnchorStateMissing,
			},
		},
	}}
	service := NewThreadService(threads, pages, memberships)

	content := json.RawMessage(`[
		{"id":"block-1","type":"paragraph","children":[{"type":"text","text":"hello world"}]},
		{"id":"block-2","type":"paragraph","children":[{"type":"text","text":"hello brave world"}]},
		{"id":"block-9","type":"paragraph","children":[{"type":"text","text":"moved text"}]},
		{"id":"block-10","type":"paragraph","children":[{"type":"text","text":"prefix target sentence suffix"}]}
	]`)
	if err := service.ReevaluatePageAnchors(context.Background(), "page-1", content, domain.ThreadAnchorReevaluationContext{Reason: domain.PageCommentThreadEventReasonDraftUpdated}); err != nil {
		t.Fatalf("ReevaluatePageAnchors() error = %v", err)
	}

	if threads.details["thread-active"].Thread.AnchorState != domain.PageCommentThreadAnchorStateActive {
		t.Fatalf("expected active thread to remain active, got %+v", threads.details["thread-active"].Thread)
	}
	if threads.details["thread-outdated"].Thread.AnchorState != domain.PageCommentThreadAnchorStateOutdated {
		t.Fatalf("expected outdated thread state, got %+v", threads.details["thread-outdated"].Thread)
	}
	if threads.details["thread-missing"].Thread.AnchorState != domain.PageCommentThreadAnchorStateMissing {
		t.Fatalf("expected missing thread state, got %+v", threads.details["thread-missing"].Thread)
	}
	if threads.details["thread-reanchor"].Thread.AnchorState != domain.PageCommentThreadAnchorStateActive {
		t.Fatalf("expected reanchor thread to remain active, got %+v", threads.details["thread-reanchor"].Thread)
	}
	if threads.details["thread-reanchor"].Thread.Anchor.BlockID == nil || *threads.details["thread-reanchor"].Thread.Anchor.BlockID != "block-9" {
		t.Fatalf("expected reanchor thread block id to recover to block-9, got %+v", threads.details["thread-reanchor"].Thread.Anchor.BlockID)
	}
	if threads.details["thread-quoted-reanchor"].Thread.AnchorState != domain.PageCommentThreadAnchorStateOutdated {
		t.Fatalf("expected quoted-text recovery thread to become outdated, got %+v", threads.details["thread-quoted-reanchor"].Thread)
	}
	if threads.details["thread-quoted-reanchor"].Thread.Anchor.BlockID == nil || *threads.details["thread-quoted-reanchor"].Thread.Anchor.BlockID != "block-10" {
		t.Fatalf("expected quoted-text recovery thread block id to recover to block-10, got %+v", threads.details["thread-quoted-reanchor"].Thread.Anchor.BlockID)
	}
	if !hasEventType(threads.details["thread-reanchor"].Events, domain.PageCommentThreadEventTypeAnchorRecovered) {
		t.Fatalf("expected anchor_recovered event on full block recovery, got %+v", threads.details["thread-reanchor"].Events)
	}
	if !hasEventReason(threads.details["thread-reanchor"].Events, domain.PageCommentThreadEventReasonDraftUpdated) {
		t.Fatalf("expected draft_updated reason on recovered thread events, got %+v", threads.details["thread-reanchor"].Events)
	}
	if !hasEventType(threads.details["thread-quoted-reanchor"].Events, domain.PageCommentThreadEventTypeAnchorRecovered) {
		t.Fatalf("expected anchor_recovered event on quoted-text recovery, got %+v", threads.details["thread-quoted-reanchor"].Events)
	}
	if !hasEventReason(threads.details["thread-outdated"].Events, domain.PageCommentThreadEventReasonDraftUpdated) {
		t.Fatalf("expected draft_updated reason on outdated thread events, got %+v", threads.details["thread-outdated"].Events)
	}
	if !hasEventReason(threads.details["thread-missing"].Events, domain.PageCommentThreadEventReasonDraftUpdated) {
		t.Fatalf("expected draft_updated reason on missing thread events, got %+v", threads.details["thread-missing"].Events)
	}
}
