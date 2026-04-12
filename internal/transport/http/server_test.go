package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"note-app/internal/application"
	"note-app/internal/domain"
	appauth "note-app/internal/infrastructure/auth"
	"note-app/internal/infrastructure/storage"
	appmiddleware "note-app/internal/transport/http/middleware"
)

type testMembershipRepo struct {
	memberships map[string][]domain.WorkspaceMember
}

func stringPtr(value string) *string {
	return &value
}

func TestWriteMappedErrorSanitizesLoggedErrorText(t *testing.T) {
	var logs bytes.Buffer
	server := Server{
		logger: slog.New(slog.NewJSONHandler(&logs, nil)),
	}
	secret := "postgres://user:super-secret@db.internal:5432/note"
	err := fmt.Errorf("unexpected dependency failure: %s", secret)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces", nil)
	rec := httptest.NewRecorder()

	server.writeMappedError(rec, req, err)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
	if strings.Contains(logs.String(), secret) {
		t.Fatalf("expected logs to redact secret, got %s", logs.String())
	}
	if !strings.Contains(logs.String(), `"msg":"request failed"`) {
		t.Fatalf("expected request failure log, got %s", logs.String())
	}
	if !strings.Contains(logs.String(), `"error_code":"internal_error"`) {
		t.Fatalf("expected internal error code in log, got %s", logs.String())
	}
}

func TestWriteMappedErrorSanitizesLoggedQueryText(t *testing.T) {
	var logs bytes.Buffer
	server := Server{
		logger: slog.New(slog.NewJSONHandler(&logs, nil)),
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/refresh?refresh_token=rt-secret&password=Password1&safe=ok", nil)
	req.Header.Set("Authorization", "Bearer bearer-secret")
	rec := httptest.NewRecorder()

	server.writeMappedError(rec, req, domain.ErrUnauthorized)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
	for _, secret := range []string{"rt-secret", "Password1", "bearer-secret"} {
		if strings.Contains(logs.String(), secret) {
			t.Fatalf("expected logs to redact %q, got %s", secret, logs.String())
		}
	}
	if !strings.Contains(logs.String(), `"query":"password=%5BREDACTED%5D&refresh_token=%5BREDACTED%5D&safe=ok"`) {
		t.Fatalf("expected sanitized query in log, got %s", logs.String())
	}
}

func (r *testMembershipRepo) GetMembershipByUserID(_ context.Context, workspaceID, userID string) (domain.WorkspaceMember, error) {
	for _, member := range r.memberships[workspaceID] {
		if member.UserID == userID {
			return member, nil
		}
	}
	return domain.WorkspaceMember{}, domain.ErrForbidden
}

func (r *testMembershipRepo) ListMembers(_ context.Context, workspaceID string) ([]domain.WorkspaceMember, error) {
	return r.memberships[workspaceID], nil
}

type testFolderRepo struct {
	byID        map[string]domain.Folder
	byWorkspace map[string][]domain.Folder
}

func (r *testFolderRepo) Create(_ context.Context, folder domain.Folder) (domain.Folder, error) {
	for _, existing := range r.byWorkspace[folder.WorkspaceID] {
		if testFolderLocationEqual(existing.ParentID, folder.ParentID) && strings.EqualFold(strings.TrimSpace(existing.Name), strings.TrimSpace(folder.Name)) {
			return domain.Folder{}, domain.ErrValidation
		}
	}
	r.byID[folder.ID] = folder
	r.byWorkspace[folder.WorkspaceID] = append(r.byWorkspace[folder.WorkspaceID], folder)
	return folder, nil
}

func (r *testFolderRepo) GetByID(_ context.Context, folderID string) (domain.Folder, error) {
	folder, ok := r.byID[folderID]
	if !ok {
		return domain.Folder{}, domain.ErrNotFound
	}
	return folder, nil
}

func (r *testFolderRepo) ListByWorkspaceID(_ context.Context, workspaceID string) ([]domain.Folder, error) {
	return r.byWorkspace[workspaceID], nil
}

func (r *testFolderRepo) HasSiblingWithName(_ context.Context, workspaceID string, parentID *string, name string, excludeFolderID *string) (bool, error) {
	for _, folder := range r.byWorkspace[workspaceID] {
		if excludeFolderID != nil && folder.ID == *excludeFolderID {
			continue
		}
		if testFolderLocationEqual(folder.ParentID, parentID) && strings.EqualFold(strings.TrimSpace(folder.Name), strings.TrimSpace(name)) {
			return true, nil
		}
	}
	return false, nil
}

func (r *testFolderRepo) UpdateName(_ context.Context, folderID, name string, updatedAt time.Time) (domain.Folder, error) {
	folder, ok := r.byID[folderID]
	if !ok {
		return domain.Folder{}, domain.ErrNotFound
	}
	for idx, existing := range r.byWorkspace[folder.WorkspaceID] {
		if existing.ID == folderID {
			folder.Name = name
			folder.UpdatedAt = updatedAt
			r.byWorkspace[folder.WorkspaceID][idx] = folder
			r.byID[folderID] = folder
			return folder, nil
		}
	}
	return domain.Folder{}, domain.ErrNotFound
}

func testFolderLocationEqual(left, right *string) bool {
	if left == nil && right == nil {
		return true
	}
	if left == nil || right == nil {
		return false
	}
	return *left == *right
}

type testPageRepo struct {
	mu                 sync.Mutex
	pages              map[string]domain.Page
	drafts             map[string]domain.PageDraft
	trash              map[string]domain.TrashItem
	trashedPages       map[string]domain.Page
	trashedDraft       map[string]domain.PageDraft
	updateDraftEntered chan struct{}
	releaseUpdateDraft <-chan struct{}
}

func (r *testPageRepo) CreateWithDraft(_ context.Context, page domain.Page, draft domain.PageDraft) (domain.Page, domain.PageDraft, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pages[page.ID] = page
	r.drafts[draft.PageID] = draft
	return page, draft, nil
}

func (r *testPageRepo) GetByID(_ context.Context, pageID string) (domain.Page, domain.PageDraft, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	page, ok := r.pages[pageID]
	if !ok {
		return domain.Page{}, domain.PageDraft{}, domain.ErrNotFound
	}
	draft, ok := r.drafts[pageID]
	if !ok {
		return domain.Page{}, domain.PageDraft{}, domain.ErrNotFound
	}
	return page, draft, nil
}

func (r *testPageRepo) GetTrashedByTrashItemID(_ context.Context, trashItemID string) (domain.TrashItem, domain.Page, domain.PageDraft, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	item, ok := r.trash[trashItemID]
	if !ok {
		return domain.TrashItem{}, domain.Page{}, domain.PageDraft{}, domain.ErrNotFound
	}
	page, ok := r.trashedPages[item.PageID]
	if !ok {
		return domain.TrashItem{}, domain.Page{}, domain.PageDraft{}, domain.ErrNotFound
	}
	draft, ok := r.trashedDraft[item.PageID]
	if !ok {
		return domain.TrashItem{}, domain.Page{}, domain.PageDraft{}, domain.ErrNotFound
	}
	return item, page, draft, nil
}

func (r *testPageRepo) ListByWorkspaceIDAndFolderID(_ context.Context, workspaceID string, folderID *string) ([]domain.PageSummary, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	items := make([]domain.PageSummary, 0)
	for _, page := range r.pages {
		if page.WorkspaceID != workspaceID {
			continue
		}
		if folderID == nil && page.FolderID != nil {
			continue
		}
		if folderID != nil && (page.FolderID == nil || *page.FolderID != *folderID) {
			continue
		}
		items = append(items, domain.PageSummary{
			ID:          page.ID,
			WorkspaceID: page.WorkspaceID,
			FolderID:    page.FolderID,
			Title:       page.Title,
			UpdatedAt:   page.UpdatedAt,
		})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].UpdatedAt.Equal(items[j].UpdatedAt) {
			return items[i].ID < items[j].ID
		}
		return items[i].UpdatedAt.After(items[j].UpdatedAt)
	})
	return items, nil
}

func (r *testPageRepo) UpdateMetadata(_ context.Context, pageID string, title string, folderID *string, updatedAt time.Time) (domain.Page, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	page, ok := r.pages[pageID]
	if !ok {
		return domain.Page{}, domain.ErrNotFound
	}
	page.Title = title
	page.FolderID = folderID
	page.UpdatedAt = updatedAt
	r.pages[pageID] = page
	return page, nil
}

func (r *testPageRepo) UpdateDraft(_ context.Context, pageID string, content json.RawMessage, lastEditedBy string, updatedAt time.Time) (domain.PageDraft, error) {
	r.mu.Lock()
	draft, ok := r.drafts[pageID]
	if !ok {
		r.mu.Unlock()
		return domain.PageDraft{}, domain.ErrNotFound
	}
	if r.updateDraftEntered != nil {
		r.updateDraftEntered <- struct{}{}
	}
	r.mu.Unlock()
	if r.releaseUpdateDraft != nil {
		<-r.releaseUpdateDraft
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	draft.Content = content
	draft.LastEditedBy = lastEditedBy
	draft.UpdatedAt = updatedAt
	r.drafts[pageID] = draft
	page := r.pages[pageID]
	page.UpdatedAt = updatedAt
	r.pages[pageID] = page
	return draft, nil
}

func (r *testPageRepo) SoftDelete(_ context.Context, trashItem domain.TrashItem) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	page, ok := r.pages[trashItem.PageID]
	if !ok {
		return domain.ErrNotFound
	}
	draft, ok := r.drafts[trashItem.PageID]
	if !ok {
		return domain.ErrNotFound
	}
	delete(r.pages, trashItem.PageID)
	delete(r.drafts, trashItem.PageID)
	if r.trash == nil {
		r.trash = map[string]domain.TrashItem{}
	}
	if r.trashedPages == nil {
		r.trashedPages = map[string]domain.Page{}
	}
	if r.trashedDraft == nil {
		r.trashedDraft = map[string]domain.PageDraft{}
	}
	r.trash[trashItem.ID] = trashItem
	r.trashedPages[trashItem.PageID] = page
	r.trashedDraft[trashItem.PageID] = draft
	return nil
}

func (r *testPageRepo) ListTrashByWorkspaceID(_ context.Context, workspaceID string) ([]domain.TrashItem, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	items := make([]domain.TrashItem, 0)
	for _, item := range r.trash {
		if item.WorkspaceID == workspaceID {
			items = append(items, item)
		}
	}
	return items, nil
}

func (r *testPageRepo) GetTrashItemByID(_ context.Context, trashItemID string) (domain.TrashItem, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	item, ok := r.trash[trashItemID]
	if !ok {
		return domain.TrashItem{}, domain.ErrNotFound
	}
	return item, nil
}

func (r *testPageRepo) RestoreTrashItem(_ context.Context, trashItemID string, _ string, restoredAt time.Time) (domain.Page, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	item, ok := r.trash[trashItemID]
	if !ok {
		return domain.Page{}, domain.ErrNotFound
	}
	page, ok := r.trashedPages[item.PageID]
	if !ok {
		page = domain.Page{
			ID:          item.PageID,
			WorkspaceID: item.WorkspaceID,
			Title:       item.PageTitle,
			CreatedBy:   item.DeletedBy,
			CreatedAt:   item.DeletedAt,
			UpdatedAt:   restoredAt,
		}
	}
	page.UpdatedAt = restoredAt
	r.pages[page.ID] = page
	if draft, ok := r.trashedDraft[item.PageID]; ok {
		draft.UpdatedAt = restoredAt
		r.drafts[page.ID] = draft
	}
	delete(r.trash, trashItemID)
	delete(r.trashedPages, item.PageID)
	delete(r.trashedDraft, item.PageID)
	return page, nil
}

type testRevisionRepo struct {
	revisions map[string]domain.Revision
	ordered   []domain.Revision
}

func (r *testRevisionRepo) Create(_ context.Context, revision domain.Revision) (domain.Revision, error) {
	r.revisions[revision.ID] = revision
	r.ordered = append(r.ordered, revision)
	return revision, nil
}

func (r *testRevisionRepo) GetByID(_ context.Context, revisionID string) (domain.Revision, error) {
	revision, ok := r.revisions[revisionID]
	if !ok {
		return domain.Revision{}, domain.ErrNotFound
	}
	return revision, nil
}

func (r *testRevisionRepo) ListByPageID(_ context.Context, pageID string) ([]domain.Revision, error) {
	result := make([]domain.Revision, 0)
	for _, revision := range r.ordered {
		if revision.PageID == pageID {
			revision.Content = nil
			result = append(result, revision)
		}
	}
	return result, nil
}

type testCommentRepo struct {
	comments map[string]domain.PageComment
	ordered  []domain.PageComment
}

func (r *testCommentRepo) Create(_ context.Context, comment domain.PageComment) (domain.PageComment, error) {
	r.comments[comment.ID] = comment
	r.ordered = append(r.ordered, comment)
	return comment, nil
}

func (r *testCommentRepo) GetByID(_ context.Context, commentID string) (domain.PageComment, error) {
	comment, ok := r.comments[commentID]
	if !ok {
		return domain.PageComment{}, domain.ErrNotFound
	}
	return comment, nil
}

func (r *testCommentRepo) ListByPageID(_ context.Context, pageID string) ([]domain.PageComment, error) {
	result := make([]domain.PageComment, 0)
	for _, comment := range r.ordered {
		if comment.PageID == pageID {
			result = append(result, r.comments[comment.ID])
		}
	}
	return result, nil
}

func (r *testCommentRepo) Resolve(_ context.Context, commentID string, resolvedBy string, resolvedAt time.Time) (domain.PageComment, error) {
	comment, ok := r.comments[commentID]
	if !ok {
		return domain.PageComment{}, domain.ErrNotFound
	}
	comment.ResolvedBy = &resolvedBy
	comment.ResolvedAt = &resolvedAt
	r.comments[commentID] = comment
	for idx := range r.ordered {
		if r.ordered[idx].ID == commentID {
			r.ordered[idx] = comment
		}
	}
	return comment, nil
}

type testThreadRepo struct {
	details             map[string]domain.PageCommentThreadDetail
	createdOutboxEvents []domain.OutboxEvent
	createdMentions     [][]domain.PageCommentMessageMention
	replyOutboxEvents   []domain.OutboxEvent
	replyMentions       [][]domain.PageCommentMessageMention
	addReplyErr         error
}

type testThreadNotificationPreferenceRepo struct {
	preferences map[string]domain.ThreadNotificationPreference
	writes      []domain.ThreadNotificationPreference
}

func (r *testThreadNotificationPreferenceRepo) GetThreadNotificationPreference(_ context.Context, threadID, userID string) (*domain.ThreadNotificationPreference, error) {
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

func (r *testThreadNotificationPreferenceRepo) SetThreadNotificationPreference(_ context.Context, preference domain.ThreadNotificationPreference) error {
	r.writes = append(r.writes, preference)
	if r.preferences == nil {
		r.preferences = map[string]domain.ThreadNotificationPreference{}
	}
	key := threadIDUserKey(preference.ThreadID, preference.UserID)
	if preference.Mode == domain.ThreadNotificationModeAll {
		delete(r.preferences, key)
		return nil
	}
	r.preferences[key] = preference
	return nil
}

func threadIDUserKey(threadID, userID string) string {
	return threadID + ":" + userID
}

func (r *testThreadRepo) CreateThread(_ context.Context, thread domain.PageCommentThread, firstMessage domain.PageCommentThreadMessage, mentions []domain.PageCommentMessageMention, outboxEvent domain.OutboxEvent) (domain.PageCommentThreadDetail, error) {
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

func (r *testThreadRepo) GetThread(_ context.Context, threadID string) (domain.PageCommentThreadDetail, error) {
	detail, ok := r.details[threadID]
	if !ok {
		return domain.PageCommentThreadDetail{}, domain.ErrNotFound
	}
	return detail, nil
}

func (r *testThreadRepo) ListThreads(_ context.Context, pageID string, threadState *domain.PageCommentThreadState, anchorState *domain.PageCommentThreadAnchorState, createdBy *string, hasMissingAnchor *bool, hasOutdatedAnchor *bool, sortMode string, query string, limit int, cursor string) (domain.PageCommentThreadList, error) {
	if cursor == "broken" {
		return domain.PageCommentThreadList{}, fmt.Errorf("%w: invalid cursor", domain.ErrValidation)
	}
	list := domain.PageCommentThreadList{
		Threads: make([]domain.PageCommentThread, 0),
		Counts:  domain.PageCommentThreadFilterCounts{},
	}
	normalizedQuery := strings.ToLower(strings.TrimSpace(query))
	for _, detail := range r.details {
		if detail.Thread.PageID != pageID {
			continue
		}
		switch detail.Thread.ThreadState {
		case domain.PageCommentThreadStateOpen:
			list.Counts.Open++
		case domain.PageCommentThreadStateResolved:
			list.Counts.Resolved++
		}
		switch detail.Thread.AnchorState {
		case domain.PageCommentThreadAnchorStateActive:
			list.Counts.Active++
		case domain.PageCommentThreadAnchorStateOutdated:
			list.Counts.Outdated++
		case domain.PageCommentThreadAnchorStateMissing:
			list.Counts.Missing++
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
		if normalizedQuery != "" {
			matches := strings.Contains(strings.ToLower(detail.Thread.Anchor.QuotedBlockText), normalizedQuery)
			if detail.Thread.Anchor.QuotedText != nil && strings.Contains(strings.ToLower(*detail.Thread.Anchor.QuotedText), normalizedQuery) {
				matches = true
			}
			if !matches {
				for _, message := range detail.Messages {
					if strings.Contains(strings.ToLower(message.Body), normalizedQuery) {
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

func (r *testThreadRepo) ListWorkspaceThreads(_ context.Context, workspaceID string, threadState *domain.PageCommentThreadState, anchorState *domain.PageCommentThreadAnchorState, createdBy *string, hasMissingAnchor *bool, hasOutdatedAnchor *bool, sortMode string, query string, limit int, cursor string) (domain.WorkspaceCommentThreadList, error) {
	if cursor == "broken" {
		return domain.WorkspaceCommentThreadList{}, fmt.Errorf("%w: invalid cursor", domain.ErrValidation)
	}
	list := domain.WorkspaceCommentThreadList{
		Threads: make([]domain.WorkspaceCommentThreadListItem, 0),
		Counts:  domain.PageCommentThreadFilterCounts{},
	}
	normalizedQuery := strings.ToLower(strings.TrimSpace(query))
	for _, detail := range r.details {
		switch detail.Thread.ThreadState {
		case domain.PageCommentThreadStateOpen:
			list.Counts.Open++
		case domain.PageCommentThreadStateResolved:
			list.Counts.Resolved++
		}
		switch detail.Thread.AnchorState {
		case domain.PageCommentThreadAnchorStateActive:
			list.Counts.Active++
		case domain.PageCommentThreadAnchorStateOutdated:
			list.Counts.Outdated++
		case domain.PageCommentThreadAnchorStateMissing:
			list.Counts.Missing++
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
		pageTitle := map[string]string{"page-1": "Doc", "page-2": "Architecture"}[detail.Thread.PageID]
		if normalizedQuery != "" {
			matches := strings.Contains(strings.ToLower(pageTitle), normalizedQuery) ||
				strings.Contains(strings.ToLower(detail.Thread.Anchor.QuotedBlockText), normalizedQuery)
			if detail.Thread.Anchor.QuotedText != nil && strings.Contains(strings.ToLower(*detail.Thread.Anchor.QuotedText), normalizedQuery) {
				matches = true
			}
			if !matches {
				for _, message := range detail.Messages {
					if strings.Contains(strings.ToLower(message.Body), normalizedQuery) {
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
		list.Threads = append(list.Threads, domain.WorkspaceCommentThreadListItem{
			Thread: detail.Thread,
			Page: domain.PageSummary{
				ID:          detail.Thread.PageID,
				WorkspaceID: workspaceID,
				Title:       pageTitle,
				UpdatedAt:   detail.Thread.LastActivityAt,
			},
		})
	}
	sort.Slice(list.Threads, func(i, j int) bool {
		left := list.Threads[i].Thread
		right := list.Threads[j].Thread
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
			if thread.Thread.ID == cursor {
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
		nextCursor := list.Threads[limit-1].Thread.ID
		list.NextCursor = &nextCursor
		list.HasMore = true
		list.Threads = list.Threads[:limit]
	}
	return list, nil
}

func (r *testThreadRepo) AddReply(_ context.Context, threadID string, message domain.PageCommentThreadMessage, mentions []domain.PageCommentMessageMention, updatedThread domain.PageCommentThread, outboxEvent domain.OutboxEvent) (domain.PageCommentThreadDetail, error) {
	if r.addReplyErr != nil {
		return domain.PageCommentThreadDetail{}, r.addReplyErr
	}
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

func (r *testThreadRepo) UpdateThreadState(_ context.Context, threadID string, updatedThread domain.PageCommentThread, _ *domain.ThreadAnchorReevaluationContext) (domain.PageCommentThreadDetail, error) {
	detail, ok := r.details[threadID]
	if !ok {
		return domain.PageCommentThreadDetail{}, domain.ErrNotFound
	}
	detail.Thread = updatedThread
	r.details[threadID] = detail
	return detail, nil
}

type testSearchRepo struct {
	resultsByQuery map[string][]domain.PageSearchResult
}

func (r *testSearchRepo) SearchPages(_ context.Context, workspaceID string, query string) ([]domain.PageSearchResult, error) {
	results := r.resultsByQuery[workspaceID+":"+query]
	return results, nil
}

func TestHealthEndpoint(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	server := NewServer(logger, application.AuthService{}, application.WorkspaceService{}, application.FolderService{}, application.PageService{}, application.RevisionService{}, appauth.NewTokenManager("super-secret-token", "note-app", 15*time.Minute), storage.NewLocal(t.TempDir()))

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var payload struct {
		Data struct {
			Status string `json:"status"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if payload.Data.Status != "ok" {
		t.Fatalf("expected status ok, got %s", payload.Data.Status)
	}
}

func TestFolderEndpoints(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	tokenManager := appauth.NewTokenManager("super-secret-token", "note-app", 15*time.Minute)
	memberships := &testMembershipRepo{
		memberships: map[string][]domain.WorkspaceMember{
			"workspace-1": {
				{ID: "member-1", WorkspaceID: "workspace-1", UserID: "user-1", Role: domain.RoleEditor},
				{ID: "member-2", WorkspaceID: "workspace-1", UserID: "user-2", Role: domain.RoleViewer},
			},
		},
	}
	folders := &testFolderRepo{byID: map[string]domain.Folder{}, byWorkspace: map[string][]domain.Folder{}}
	folderService := application.NewFolderService(folders, memberships)
	server := NewServer(logger, application.AuthService{}, application.WorkspaceService{}, folderService, application.PageService{}, application.RevisionService{}, tokenManager, storage.NewLocal(t.TempDir()))

	accessToken, _, err := tokenManager.GenerateAccessToken("user-1", "user@example.com", time.Now().UTC())
	if err != nil {
		t.Fatalf("GenerateAccessToken() error = %v", err)
	}
	viewerToken, _, err := tokenManager.GenerateAccessToken("user-2", "viewer@example.com", time.Now().UTC())
	if err != nil {
		t.Fatalf("GenerateAccessToken() error = %v", err)
	}

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces/workspace-1/folders", bytes.NewBufferString(`{"name":"Engineering"}`))
	createReq.Header.Set("Authorization", "Bearer "+accessToken)
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(createRec, createReq)

	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d body=%s", createRec.Code, createRec.Body.String())
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces/workspace-1/folders", nil)
	listReq.Header.Set("Authorization", "Bearer "+accessToken)
	listRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(listRec, listReq)

	if listRec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", listRec.Code, listRec.Body.String())
	}

	var payload struct {
		Data []domain.Folder `json:"data"`
	}
	if err := json.Unmarshal(listRec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal folders response: %v", err)
	}
	if len(payload.Data) != 1 || payload.Data[0].Name != "Engineering" {
		t.Fatalf("unexpected folders payload: %+v", payload.Data)
	}

	renameReq := httptest.NewRequest(http.MethodPatch, "/api/v1/folders/"+payload.Data[0].ID, bytes.NewBufferString(`{"name":"Platform"}`))
	renameReq.Header.Set("Authorization", "Bearer "+accessToken)
	renameReq.Header.Set("Content-Type", "application/json")
	renameRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(renameRec, renameReq)
	if renameRec.Code != http.StatusOK {
		t.Fatalf("expected rename folder status 200, got %d body=%s", renameRec.Code, renameRec.Body.String())
	}

	duplicateReq := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces/workspace-1/folders", bytes.NewBufferString(`{"name":" platform "}`))
	duplicateReq.Header.Set("Authorization", "Bearer "+accessToken)
	duplicateReq.Header.Set("Content-Type", "application/json")
	duplicateRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(duplicateRec, duplicateReq)
	if duplicateRec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected duplicate folder status 422, got %d body=%s", duplicateRec.Code, duplicateRec.Body.String())
	}

	viewerRenameReq := httptest.NewRequest(http.MethodPatch, "/api/v1/folders/"+payload.Data[0].ID, bytes.NewBufferString(`{"name":"Viewer Attempt"}`))
	viewerRenameReq.Header.Set("Authorization", "Bearer "+viewerToken)
	viewerRenameReq.Header.Set("Content-Type", "application/json")
	viewerRenameRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(viewerRenameRec, viewerRenameReq)
	if viewerRenameRec.Code != http.StatusForbidden {
		t.Fatalf("expected viewer rename folder status 403, got %d body=%s", viewerRenameRec.Code, viewerRenameRec.Body.String())
	}
}

func TestPageEndpoints(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	tokenManager := appauth.NewTokenManager("super-secret-token", "note-app", 15*time.Minute)
	memberships := &testMembershipRepo{
		memberships: map[string][]domain.WorkspaceMember{
			"workspace-1": {
				{ID: "member-1", WorkspaceID: "workspace-1", UserID: "user-1", Role: domain.RoleEditor},
			},
		},
	}
	folders := &testFolderRepo{byID: map[string]domain.Folder{}, byWorkspace: map[string][]domain.Folder{}}
	pages := &testPageRepo{pages: map[string]domain.Page{}, drafts: map[string]domain.PageDraft{}}
	pageService := application.NewPageService(pages, memberships, folders)
	server := NewServer(logger, application.AuthService{}, application.WorkspaceService{}, application.FolderService{}, pageService, application.RevisionService{}, tokenManager, storage.NewLocal(t.TempDir()))

	accessToken, _, err := tokenManager.GenerateAccessToken("user-1", "user@example.com", time.Now().UTC())
	if err != nil {
		t.Fatalf("GenerateAccessToken() error = %v", err)
	}

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces/workspace-1/pages", bytes.NewBufferString(`{"title":"Architecture"}`))
	createReq.Header.Set("Authorization", "Bearer "+accessToken)
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(createRec, createReq)

	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d body=%s", createRec.Code, createRec.Body.String())
	}

	var created struct {
		Data struct {
			Page domain.Page `json:"page"`
		} `json:"data"`
	}
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal create page response: %v", err)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/pages/"+created.Data.Page.ID, nil)
	getReq.Header.Set("Authorization", "Bearer "+accessToken)
	getRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(getRec, getReq)

	if getRec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", getRec.Code, getRec.Body.String())
	}

	var payload struct {
		Data struct {
			Page  domain.Page      `json:"page"`
			Draft domain.PageDraft `json:"draft"`
		} `json:"data"`
	}
	if err := json.Unmarshal(getRec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal get page response: %v", err)
	}
	if payload.Data.Page.Title != "Architecture" || string(payload.Data.Draft.Content) != "[]" {
		t.Fatalf("unexpected page payload: %+v", payload.Data)
	}
}

func TestRateLimitAppliesOnlyToAPIRoutes(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	tokenManager := appauth.NewTokenManager("super-secret-token", "note-app", 15*time.Minute)
	server := NewServer(logger, application.AuthService{}, application.WorkspaceService{}, application.FolderService{}, application.PageService{}, application.RevisionService{}, tokenManager, storage.NewLocal(t.TempDir()))
	handler := server.Handler()

	for i := 0; i < 120; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/unknown", nil)
		req.RemoteAddr = "203.0.113.99:1234"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("request %d: expected 404 before rate limit, got %d", i+1, rec.Code)
		}
	}

	limitedReq := httptest.NewRequest(http.MethodGet, "/api/v1/unknown", nil)
	limitedReq.RemoteAddr = "203.0.113.99:1234"
	limitedRec := httptest.NewRecorder()
	handler.ServeHTTP(limitedRec, limitedReq)
	if limitedRec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected api route to be rate limited, got %d", limitedRec.Code)
	}

	for i := 0; i < 5; i++ {
		healthReq := httptest.NewRequest(http.MethodGet, "/healthz", nil)
		healthReq.RemoteAddr = "203.0.113.99:1234"
		healthRec := httptest.NewRecorder()
		handler.ServeHTTP(healthRec, healthReq)
		if healthRec.Code != http.StatusOK {
			t.Fatalf("healthz request %d: expected 200, got %d", i+1, healthRec.Code)
		}
	}
}

func TestRateLimitIgnoresSpoofedForwardedHeadersWithoutTrustedProxy(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	tokenManager := appauth.NewTokenManager("super-secret-token", "note-app", 15*time.Minute)
	server := NewServer(logger, application.AuthService{}, application.WorkspaceService{}, application.FolderService{}, application.PageService{}, application.RevisionService{}, tokenManager, storage.NewLocal(t.TempDir()))
	handler := server.Handler()

	firstReq := httptest.NewRequest(http.MethodGet, "/api/v1/unknown", nil)
	firstReq.RemoteAddr = "203.0.113.55:1234"
	firstReq.Header.Set("X-Forwarded-For", "198.51.100.10")

	for i := 0; i < 120; i++ {
		req := firstReq.Clone(firstReq.Context())
		req.Header.Set("X-Forwarded-For", "198.51.100."+strconv.Itoa(10+i))
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("request %d: expected 404 before rate limit, got %d", i+1, rec.Code)
		}
	}

	blockedReq := httptest.NewRequest(http.MethodGet, "/api/v1/unknown", nil)
	blockedReq.RemoteAddr = "203.0.113.55:9999"
	blockedReq.Header.Set("X-Forwarded-For", "198.51.100.250")
	blockedRec := httptest.NewRecorder()
	handler.ServeHTTP(blockedRec, blockedReq)
	if blockedRec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected spoofed forwarded headers not to bypass limiter, got %d", blockedRec.Code)
	}
}

func TestRateLimitUsesForwardedClientIPFromTrustedProxy(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	tokenManager := appauth.NewTokenManager("super-secret-token", "note-app", 15*time.Minute)
	server := NewServer(logger, application.AuthService{}, application.WorkspaceService{}, application.FolderService{}, application.PageService{}, application.RevisionService{}, tokenManager, storage.NewLocal(t.TempDir())).
		WithClientIPConfig(appmiddleware.ClientIPConfig{
			TrustProxyHeaders: true,
			TrustedProxyCIDRs: []string{"10.0.0.0/8"},
		})
	handler := server.Handler()

	for i := 0; i < 120; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/unknown", nil)
		req.RemoteAddr = "10.1.1.1:1234"
		req.Header.Set("X-Forwarded-For", "198.51.100.1")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("request %d: expected 404 before rate limit, got %d", i+1, rec.Code)
		}
	}

	otherClientReq := httptest.NewRequest(http.MethodGet, "/api/v1/unknown", nil)
	otherClientReq.RemoteAddr = "10.1.1.1:5678"
	otherClientReq.Header.Set("X-Forwarded-For", "198.51.100.2")
	otherClientRec := httptest.NewRecorder()
	handler.ServeHTTP(otherClientRec, otherClientReq)
	if otherClientRec.Code != http.StatusNotFound {
		t.Fatalf("expected distinct forwarded client to avoid first client's limit bucket, got %d", otherClientRec.Code)
	}
}

func TestHeavyRoutesUseOverloadSheddingWithoutBlockingLightRoutes(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	tokenManager := appauth.NewTokenManager("super-secret-token", "note-app", 15*time.Minute)
	memberships := &testMembershipRepo{
		memberships: map[string][]domain.WorkspaceMember{
			"workspace-1": {
				{ID: "member-1", WorkspaceID: "workspace-1", UserID: "user-1", Role: domain.RoleEditor},
			},
		},
	}
	release := make(chan struct{})
	pages := &testPageRepo{
		pages: map[string]domain.Page{
			"page-1": {ID: "page-1", WorkspaceID: "workspace-1", Title: "Doc"},
		},
		drafts: map[string]domain.PageDraft{
			"page-1": {PageID: "page-1", Content: json.RawMessage(`[]`), LastEditedBy: "user-1"},
		},
		updateDraftEntered: make(chan struct{}, 4),
		releaseUpdateDraft: release,
	}
	pageService := application.NewPageService(pages, memberships, &testFolderRepo{byID: map[string]domain.Folder{}, byWorkspace: map[string][]domain.Folder{}})
	server := NewServer(logger, application.AuthService{}, application.WorkspaceService{}, application.FolderService{}, pageService, application.RevisionService{}, tokenManager, storage.NewLocal(t.TempDir()))
	handler := server.Handler()

	editorToken, _, err := tokenManager.GenerateAccessToken("user-1", "editor@example.com", time.Now().UTC())
	if err != nil {
		t.Fatalf("GenerateAccessToken() error = %v", err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodPut, "/api/v1/pages/page-1/draft", bytes.NewBufferString(`{"content":[]}`))
			req.Header.Set("Authorization", "Bearer "+editorToken)
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
		}()
	}

	for i := 0; i < 4; i++ {
		<-pages.updateDraftEntered
	}

	overloadedReq := httptest.NewRequest(http.MethodPut, "/api/v1/pages/page-1/draft", bytes.NewBufferString(`{"content":[]}`))
	overloadedReq.Header.Set("Authorization", "Bearer "+editorToken)
	overloadedReq.Header.Set("Content-Type", "application/json")
	overloadedRec := httptest.NewRecorder()
	handler.ServeHTTP(overloadedRec, overloadedReq)
	if overloadedRec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected overloaded heavy route to return 503, got %d body=%s", overloadedRec.Code, overloadedRec.Body.String())
	}
	if got := overloadedRec.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("expected overloaded route content-type application/json, got %q", got)
	}
	if got := overloadedRec.Header().Get("Retry-After"); got != "1" {
		t.Fatalf("expected overloaded route retry-after 1, got %q", got)
	}
	var overloadedPayload map[string]map[string]string
	if err := json.Unmarshal(overloadedRec.Body.Bytes(), &overloadedPayload); err != nil {
		t.Fatalf("parse overloaded route body: %v", err)
	}
	if overloadedPayload["error"]["code"] != "overloaded" || overloadedPayload["error"]["message"] != "server is handling too many expensive requests" {
		t.Fatalf("unexpected overloaded route payload: %+v", overloadedPayload)
	}

	lightReq := httptest.NewRequest(http.MethodGet, "/api/v1/pages/page-1", nil)
	lightReq.Header.Set("Authorization", "Bearer "+editorToken)
	lightRec := httptest.NewRecorder()
	handler.ServeHTTP(lightRec, lightReq)
	if lightRec.Code != http.StatusOK {
		t.Fatalf("expected light route to stay available, got %d body=%s", lightRec.Code, lightRec.Body.String())
	}

	close(release)
	wg.Wait()
}

func TestHeavyRoutesShareOneOverloadPoolAcrossEndpoints(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	tokenManager := appauth.NewTokenManager("super-secret-token", "note-app", 15*time.Minute)
	memberships := &testMembershipRepo{
		memberships: map[string][]domain.WorkspaceMember{
			"workspace-1": {
				{ID: "member-1", WorkspaceID: "workspace-1", UserID: "user-1", Role: domain.RoleEditor},
			},
		},
	}
	release := make(chan struct{})
	pages := &testPageRepo{
		pages: map[string]domain.Page{
			"page-1": {ID: "page-1", WorkspaceID: "workspace-1", Title: "Doc"},
		},
		drafts: map[string]domain.PageDraft{
			"page-1": {PageID: "page-1", Content: json.RawMessage(`[]`), LastEditedBy: "user-1"},
		},
		updateDraftEntered: make(chan struct{}, 4),
		releaseUpdateDraft: release,
	}
	pageService := application.NewPageService(pages, memberships, &testFolderRepo{byID: map[string]domain.Folder{}, byWorkspace: map[string][]domain.Folder{}})
	revisionRepo := &testRevisionRepo{revisions: map[string]domain.Revision{}, ordered: []domain.Revision{}}
	revisionService := application.NewRevisionService(revisionRepo, pages, memberships)
	server := NewServer(logger, application.AuthService{}, application.WorkspaceService{}, application.FolderService{}, pageService, revisionService, tokenManager, storage.NewLocal(t.TempDir()))
	handler := server.Handler()

	editorToken, _, err := tokenManager.GenerateAccessToken("user-1", "editor@example.com", time.Now().UTC())
	if err != nil {
		t.Fatalf("GenerateAccessToken() error = %v", err)
	}

	var wg sync.WaitGroup
	for i := 0; i < heavyRouteMaxConcurrent; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodPut, "/api/v1/pages/page-1/draft", bytes.NewBufferString(`{"content":[]}`))
			req.Header.Set("Authorization", "Bearer "+editorToken)
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
		}()
	}

	for i := 0; i < heavyRouteMaxConcurrent; i++ {
		<-pages.updateDraftEntered
	}

	createRevisionReq := httptest.NewRequest(http.MethodPost, "/api/v1/pages/page-1/revisions", bytes.NewBufferString(`{}`))
	createRevisionReq.Header.Set("Authorization", "Bearer "+editorToken)
	createRevisionReq.Header.Set("Content-Type", "application/json")
	createRevisionRec := httptest.NewRecorder()
	handler.ServeHTTP(createRevisionRec, createRevisionReq)
	if createRevisionRec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected create revision to be blocked by shared heavy-route limiter, got %d body=%s", createRevisionRec.Code, createRevisionRec.Body.String())
	}

	close(release)
	wg.Wait()
}

func TestPageListEndpoint(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	tokenManager := appauth.NewTokenManager("super-secret-token", "note-app", 15*time.Minute)
	memberships := &testMembershipRepo{
		memberships: map[string][]domain.WorkspaceMember{
			"workspace-1": {
				{ID: "member-1", WorkspaceID: "workspace-1", UserID: "viewer-1", Role: domain.RoleViewer},
			},
		},
	}
	folderID := "folder-1"
	otherFolderID := "folder-2"
	now := time.Now().UTC()
	folders := &testFolderRepo{
		byID: map[string]domain.Folder{
			"folder-1": {ID: "folder-1", WorkspaceID: "workspace-1", Name: "Docs"},
			"folder-2": {ID: "folder-2", WorkspaceID: "workspace-1", Name: "Notes"},
			"folder-x": {ID: "folder-x", WorkspaceID: "workspace-2", Name: "Other"},
		},
		byWorkspace: map[string][]domain.Folder{
			"workspace-1": {
				{ID: "folder-1", WorkspaceID: "workspace-1", Name: "Docs"},
				{ID: "folder-2", WorkspaceID: "workspace-1", Name: "Notes"},
			},
		},
	}
	pages := &testPageRepo{
		pages: map[string]domain.Page{
			"root-new":     {ID: "root-new", WorkspaceID: "workspace-1", Title: "Root New", UpdatedAt: now},
			"root-old":     {ID: "root-old", WorkspaceID: "workspace-1", Title: "Root Old", UpdatedAt: now.Add(-time.Minute)},
			"folder-page":  {ID: "folder-page", WorkspaceID: "workspace-1", FolderID: &folderID, Title: "Folder Page", UpdatedAt: now.Add(-2 * time.Minute)},
			"other-folder": {ID: "other-folder", WorkspaceID: "workspace-1", FolderID: &otherFolderID, Title: "Other Folder Page", UpdatedAt: now.Add(-3 * time.Minute)},
		},
		drafts: map[string]domain.PageDraft{
			"root-new":     {PageID: "root-new", Content: json.RawMessage("[]")},
			"root-old":     {PageID: "root-old", Content: json.RawMessage("[]")},
			"folder-page":  {PageID: "folder-page", Content: json.RawMessage("[]")},
			"other-folder": {PageID: "other-folder", Content: json.RawMessage("[]")},
		},
	}
	pageService := application.NewPageService(pages, memberships, folders)
	server := NewServer(logger, application.AuthService{}, application.WorkspaceService{}, application.FolderService{}, pageService, application.RevisionService{}, tokenManager, storage.NewLocal(t.TempDir()))

	viewerToken, _, err := tokenManager.GenerateAccessToken("viewer-1", "viewer@example.com", time.Now().UTC())
	if err != nil {
		t.Fatalf("GenerateAccessToken() error = %v", err)
	}

	rootReq := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces/workspace-1/pages", nil)
	rootReq.Header.Set("Authorization", "Bearer "+viewerToken)
	rootRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rootRec, rootReq)
	if rootRec.Code != http.StatusOK {
		t.Fatalf("expected root page list status 200, got %d body=%s", rootRec.Code, rootRec.Body.String())
	}

	var rootPayload struct {
		Data []domain.PageSummary `json:"data"`
	}
	if err := json.Unmarshal(rootRec.Body.Bytes(), &rootPayload); err != nil {
		t.Fatalf("unmarshal root page list response: %v", err)
	}
	if len(rootPayload.Data) != 2 || rootPayload.Data[0].ID != "root-new" || rootPayload.Data[1].ID != "root-old" {
		t.Fatalf("unexpected root page list: %+v", rootPayload.Data)
	}

	blankReq := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces/workspace-1/pages?folder_id=%20%20%20", nil)
	blankReq.Header.Set("Authorization", "Bearer "+viewerToken)
	blankRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(blankRec, blankReq)
	if blankRec.Code != http.StatusOK {
		t.Fatalf("expected blank folder id list status 200, got %d body=%s", blankRec.Code, blankRec.Body.String())
	}

	var blankPayload struct {
		Data []domain.PageSummary `json:"data"`
	}
	if err := json.Unmarshal(blankRec.Body.Bytes(), &blankPayload); err != nil {
		t.Fatalf("unmarshal blank folder id list response: %v", err)
	}
	if len(blankPayload.Data) != 2 || blankPayload.Data[0].ID != "root-new" || blankPayload.Data[1].ID != "root-old" {
		t.Fatalf("unexpected blank folder id list: %+v", blankPayload.Data)
	}

	folderReq := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces/workspace-1/pages?folder_id=folder-1", nil)
	folderReq.Header.Set("Authorization", "Bearer "+viewerToken)
	folderRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(folderRec, folderReq)
	if folderRec.Code != http.StatusOK {
		t.Fatalf("expected folder page list status 200, got %d body=%s", folderRec.Code, folderRec.Body.String())
	}

	var folderPayload struct {
		Data []domain.PageSummary `json:"data"`
	}
	if err := json.Unmarshal(folderRec.Body.Bytes(), &folderPayload); err != nil {
		t.Fatalf("unmarshal folder page list response: %v", err)
	}
	if len(folderPayload.Data) != 1 || folderPayload.Data[0].ID != "folder-page" {
		t.Fatalf("unexpected folder page list: %+v", folderPayload.Data)
	}

	missingFolderReq := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces/workspace-1/pages?folder_id=missing", nil)
	missingFolderReq.Header.Set("Authorization", "Bearer "+viewerToken)
	missingFolderRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(missingFolderRec, missingFolderReq)
	if missingFolderRec.Code != http.StatusNotFound {
		t.Fatalf("expected missing folder status 404, got %d body=%s", missingFolderRec.Code, missingFolderRec.Body.String())
	}

	crossWorkspaceFolderReq := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces/workspace-1/pages?folder_id=folder-x", nil)
	crossWorkspaceFolderReq.Header.Set("Authorization", "Bearer "+viewerToken)
	crossWorkspaceFolderRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(crossWorkspaceFolderRec, crossWorkspaceFolderReq)
	if crossWorkspaceFolderRec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected cross-workspace folder status 422, got %d body=%s", crossWorkspaceFolderRec.Code, crossWorkspaceFolderRec.Body.String())
	}

	nonMemberToken, _, err := tokenManager.GenerateAccessToken("outsider", "outsider@example.com", time.Now().UTC())
	if err != nil {
		t.Fatalf("GenerateAccessToken() outsider error = %v", err)
	}
	nonMemberReq := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces/workspace-1/pages", nil)
	nonMemberReq.Header.Set("Authorization", "Bearer "+nonMemberToken)
	nonMemberRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(nonMemberRec, nonMemberReq)
	if nonMemberRec.Code != http.StatusForbidden {
		t.Fatalf("expected non-member page list status 403, got %d body=%s", nonMemberRec.Code, nonMemberRec.Body.String())
	}
}

func TestPageUpdateEndpoint(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	tokenManager := appauth.NewTokenManager("super-secret-token", "note-app", 15*time.Minute)
	memberships := &testMembershipRepo{
		memberships: map[string][]domain.WorkspaceMember{
			"workspace-1": {
				{ID: "member-1", WorkspaceID: "workspace-1", UserID: "user-1", Role: domain.RoleEditor},
				{ID: "member-2", WorkspaceID: "workspace-1", UserID: "user-2", Role: domain.RoleViewer},
			},
		},
	}
	folders := &testFolderRepo{
		byID: map[string]domain.Folder{
			"folder-1": {ID: "folder-1", WorkspaceID: "workspace-1", Name: "Engineering"},
		},
		byWorkspace: map[string][]domain.Folder{"workspace-1": {{ID: "folder-1", WorkspaceID: "workspace-1", Name: "Engineering"}}},
	}
	pages := &testPageRepo{
		pages: map[string]domain.Page{
			"page-1": {ID: "page-1", WorkspaceID: "workspace-1", Title: "Old Title"},
		},
		drafts: map[string]domain.PageDraft{
			"page-1": {PageID: "page-1", Content: json.RawMessage("[]")},
		},
	}
	pageService := application.NewPageService(pages, memberships, folders)
	server := NewServer(logger, application.AuthService{}, application.WorkspaceService{}, application.FolderService{}, pageService, application.RevisionService{}, tokenManager, storage.NewLocal(t.TempDir()))

	editorToken, _, err := tokenManager.GenerateAccessToken("user-1", "editor@example.com", time.Now().UTC())
	if err != nil {
		t.Fatalf("GenerateAccessToken() error = %v", err)
	}
	viewerToken, _, err := tokenManager.GenerateAccessToken("user-2", "viewer@example.com", time.Now().UTC())
	if err != nil {
		t.Fatalf("GenerateAccessToken() error = %v", err)
	}

	renameReq := httptest.NewRequest(http.MethodPatch, "/api/v1/pages/page-1", bytes.NewBufferString(`{"title":"New Title"}`))
	renameReq.Header.Set("Authorization", "Bearer "+editorToken)
	renameReq.Header.Set("Content-Type", "application/json")
	renameRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(renameRec, renameReq)
	if renameRec.Code != http.StatusOK {
		t.Fatalf("expected rename status 200, got %d body=%s", renameRec.Code, renameRec.Body.String())
	}

	moveReq := httptest.NewRequest(http.MethodPatch, "/api/v1/pages/page-1", bytes.NewBufferString(`{"folder_id":"folder-1"}`))
	moveReq.Header.Set("Authorization", "Bearer "+editorToken)
	moveReq.Header.Set("Content-Type", "application/json")
	moveRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(moveRec, moveReq)
	if moveRec.Code != http.StatusOK {
		t.Fatalf("expected move status 200, got %d body=%s", moveRec.Code, moveRec.Body.String())
	}

	moveRootReq := httptest.NewRequest(http.MethodPatch, "/api/v1/pages/page-1", bytes.NewBufferString(`{"folder_id":null}`))
	moveRootReq.Header.Set("Authorization", "Bearer "+editorToken)
	moveRootReq.Header.Set("Content-Type", "application/json")
	moveRootRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(moveRootRec, moveRootReq)
	if moveRootRec.Code != http.StatusOK {
		t.Fatalf("expected move root status 200, got %d body=%s", moveRootRec.Code, moveRootRec.Body.String())
	}

	invalidReq := httptest.NewRequest(http.MethodPatch, "/api/v1/pages/page-1", bytes.NewBufferString(`{"folder_id":"folder-x"}`))
	invalidReq.Header.Set("Authorization", "Bearer "+editorToken)
	invalidReq.Header.Set("Content-Type", "application/json")
	invalidRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(invalidRec, invalidReq)
	if invalidRec.Code != http.StatusNotFound {
		t.Fatalf("expected invalid folder status 404, got %d body=%s", invalidRec.Code, invalidRec.Body.String())
	}

	viewerReq := httptest.NewRequest(http.MethodPatch, "/api/v1/pages/page-1", bytes.NewBufferString(`{"title":"Viewer Attempt"}`))
	viewerReq.Header.Set("Authorization", "Bearer "+viewerToken)
	viewerReq.Header.Set("Content-Type", "application/json")
	viewerRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(viewerRec, viewerReq)
	if viewerRec.Code != http.StatusForbidden {
		t.Fatalf("expected viewer status 403, got %d body=%s", viewerRec.Code, viewerRec.Body.String())
	}
}

func TestPageDraftEndpoint(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	tokenManager := appauth.NewTokenManager("super-secret-token", "note-app", 15*time.Minute)
	memberships := &testMembershipRepo{
		memberships: map[string][]domain.WorkspaceMember{
			"workspace-1": {
				{ID: "member-1", WorkspaceID: "workspace-1", UserID: "user-1", Role: domain.RoleEditor},
				{ID: "member-2", WorkspaceID: "workspace-1", UserID: "user-2", Role: domain.RoleViewer},
			},
		},
	}
	folders := &testFolderRepo{byID: map[string]domain.Folder{}, byWorkspace: map[string][]domain.Folder{}}
	pages := &testPageRepo{
		pages: map[string]domain.Page{
			"page-1": {ID: "page-1", WorkspaceID: "workspace-1", Title: "Doc"},
		},
		drafts: map[string]domain.PageDraft{
			"page-1": {PageID: "page-1", Content: json.RawMessage("[]"), LastEditedBy: "user-1"},
		},
	}
	pageService := application.NewPageService(pages, memberships, folders)
	server := NewServer(logger, application.AuthService{}, application.WorkspaceService{}, application.FolderService{}, pageService, application.RevisionService{}, tokenManager, storage.NewLocal(t.TempDir()))

	editorToken, _, err := tokenManager.GenerateAccessToken("user-1", "editor@example.com", time.Now().UTC())
	if err != nil {
		t.Fatalf("GenerateAccessToken() error = %v", err)
	}
	viewerToken, _, err := tokenManager.GenerateAccessToken("user-2", "viewer@example.com", time.Now().UTC())
	if err != nil {
		t.Fatalf("GenerateAccessToken() error = %v", err)
	}

	validContent := `{"content":[{"id":"block-1","type":"paragraph","children":[{"type":"text","text":"hello","marks":[{"type":"bold"},{"type":"link","href":"https://example.com/docs"}]}]}]}`
	updateReq := httptest.NewRequest(http.MethodPut, "/api/v1/pages/page-1/draft", bytes.NewBufferString(validContent))
	updateReq.Header.Set("Authorization", "Bearer "+editorToken)
	updateReq.Header.Set("Content-Type", "application/json")
	updateRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(updateRec, updateReq)
	if updateRec.Code != http.StatusOK {
		t.Fatalf("expected draft update status 200, got %d body=%s", updateRec.Code, updateRec.Body.String())
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/pages/page-1", nil)
	getReq.Header.Set("Authorization", "Bearer "+editorToken)
	getRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("expected get page status 200, got %d body=%s", getRec.Code, getRec.Body.String())
	}

	var payload struct {
		Data struct {
			Draft domain.PageDraft `json:"draft"`
		} `json:"data"`
	}
	if err := json.Unmarshal(getRec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal updated page response: %v", err)
	}
	if string(payload.Data.Draft.Content) != `[{"id":"block-1","type":"paragraph","children":[{"type":"text","text":"hello","marks":[{"type":"bold"},{"type":"link","href":"https://example.com/docs"}]}]}]` {
		t.Fatalf("unexpected draft content: %s", string(payload.Data.Draft.Content))
	}

	largeText := strings.Repeat("a", int(defaultJSONBodyLimitBytes)+1024)
	largeContent := `{"content":[{"id":"block-2","type":"paragraph","children":[{"type":"text","text":"` + largeText + `"}]}]}`
	largeReq := httptest.NewRequest(http.MethodPut, "/api/v1/pages/page-1/draft", bytes.NewBufferString(largeContent))
	largeReq.Header.Set("Authorization", "Bearer "+editorToken)
	largeReq.Header.Set("Content-Type", "application/json")
	largeRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(largeRec, largeReq)
	if largeRec.Code != http.StatusOK {
		t.Fatalf("expected large draft update status 200, got %d body=%s", largeRec.Code, largeRec.Body.String())
	}

	invalidBlockReq := httptest.NewRequest(http.MethodPut, "/api/v1/pages/page-1/draft", bytes.NewBufferString(`{"content":[{"type":"unsupported"}]}`))
	invalidBlockReq.Header.Set("Authorization", "Bearer "+editorToken)
	invalidBlockReq.Header.Set("Content-Type", "application/json")
	invalidBlockRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(invalidBlockRec, invalidBlockReq)
	if invalidBlockRec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected invalid block status 422, got %d body=%s", invalidBlockRec.Code, invalidBlockRec.Body.String())
	}

	invalidLinkReq := httptest.NewRequest(http.MethodPut, "/api/v1/pages/page-1/draft", bytes.NewBufferString(`{"content":[{"id":"block-3","type":"paragraph","children":[{"type":"text","text":"bad","marks":[{"type":"link","href":"notaurl"}]}]}]}`))
	invalidLinkReq.Header.Set("Authorization", "Bearer "+editorToken)
	invalidLinkReq.Header.Set("Content-Type", "application/json")
	invalidLinkRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(invalidLinkRec, invalidLinkReq)
	if invalidLinkRec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected invalid link status 422, got %d body=%s", invalidLinkRec.Code, invalidLinkRec.Body.String())
	}

	invalidImageReq := httptest.NewRequest(http.MethodPut, "/api/v1/pages/page-1/draft", bytes.NewBufferString(`{"content":[{"type":"image","alt":"missing src"}]}`))
	invalidImageReq.Header.Set("Authorization", "Bearer "+editorToken)
	invalidImageReq.Header.Set("Content-Type", "application/json")
	invalidImageRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(invalidImageRec, invalidImageReq)
	if invalidImageRec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected invalid image status 422, got %d body=%s", invalidImageRec.Code, invalidImageRec.Body.String())
	}

	viewerReq := httptest.NewRequest(http.MethodPut, "/api/v1/pages/page-1/draft", bytes.NewBufferString(`{"content":[]}`))
	viewerReq.Header.Set("Authorization", "Bearer "+viewerToken)
	viewerReq.Header.Set("Content-Type", "application/json")
	viewerRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(viewerRec, viewerReq)
	if viewerRec.Code != http.StatusForbidden {
		t.Fatalf("expected viewer draft status 403, got %d body=%s", viewerRec.Code, viewerRec.Body.String())
	}

	missingReq := httptest.NewRequest(http.MethodPut, "/api/v1/pages/missing-page/draft", bytes.NewBufferString(`{"content":[]}`))
	missingReq.Header.Set("Authorization", "Bearer "+editorToken)
	missingReq.Header.Set("Content-Type", "application/json")
	missingRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(missingRec, missingReq)
	if missingRec.Code != http.StatusNotFound {
		t.Fatalf("expected missing page status 404, got %d body=%s", missingRec.Code, missingRec.Body.String())
	}
}

func TestPageRevisionEndpoint(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	tokenManager := appauth.NewTokenManager("super-secret-token", "note-app", 15*time.Minute)
	memberships := &testMembershipRepo{
		memberships: map[string][]domain.WorkspaceMember{
			"workspace-1": {
				{ID: "member-1", WorkspaceID: "workspace-1", UserID: "user-1", Role: domain.RoleEditor},
				{ID: "member-2", WorkspaceID: "workspace-1", UserID: "user-2", Role: domain.RoleViewer},
			},
		},
	}
	pages := &testPageRepo{
		pages: map[string]domain.Page{
			"page-1": {ID: "page-1", WorkspaceID: "workspace-1", Title: "Doc"},
		},
		drafts: map[string]domain.PageDraft{
			"page-1": {PageID: "page-1", Content: json.RawMessage(`[{"type":"paragraph","children":[{"type":"text","text":"checkpoint"}]}]`), LastEditedBy: "user-1"},
		},
	}
	revisions := &testRevisionRepo{revisions: map[string]domain.Revision{}}
	revisionService := application.NewRevisionService(revisions, pages, memberships)
	server := NewServer(logger, application.AuthService{}, application.WorkspaceService{}, application.FolderService{}, application.PageService{}, revisionService, tokenManager, storage.NewLocal(t.TempDir()))

	editorToken, _, err := tokenManager.GenerateAccessToken("user-1", "editor@example.com", time.Now().UTC())
	if err != nil {
		t.Fatalf("GenerateAccessToken() error = %v", err)
	}
	viewerToken, _, err := tokenManager.GenerateAccessToken("user-2", "viewer@example.com", time.Now().UTC())
	if err != nil {
		t.Fatalf("GenerateAccessToken() error = %v", err)
	}

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/pages/page-1/revisions", bytes.NewBufferString(`{"label":"Milestone 1","note":"Before rewrite"}`))
	createReq.Header.Set("Authorization", "Bearer "+editorToken)
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected revision create status 201, got %d body=%s", createRec.Code, createRec.Body.String())
	}
	if len(revisions.revisions) != 1 {
		t.Fatalf("expected one revision, got %d", len(revisions.revisions))
	}
	for _, revision := range revisions.revisions {
		if revision.Label == nil || *revision.Label != "Milestone 1" {
			t.Fatalf("unexpected label: %+v", revision.Label)
		}
		if revision.Note == nil || *revision.Note != "Before rewrite" {
			t.Fatalf("unexpected note: %+v", revision.Note)
		}
		if string(revision.Content) != string(pages.drafts["page-1"].Content) {
			t.Fatalf("expected revision content to match draft")
		}
	}

	viewerReq := httptest.NewRequest(http.MethodPost, "/api/v1/pages/page-1/revisions", bytes.NewBufferString(`{}`))
	viewerReq.Header.Set("Authorization", "Bearer "+viewerToken)
	viewerReq.Header.Set("Content-Type", "application/json")
	viewerRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(viewerRec, viewerReq)
	if viewerRec.Code != http.StatusForbidden {
		t.Fatalf("expected viewer revision status 403, got %d body=%s", viewerRec.Code, viewerRec.Body.String())
	}

	invalidReq := httptest.NewRequest(http.MethodPost, "/api/v1/pages/page-1/revisions", bytes.NewBufferString(`{"label":true}`))
	invalidReq.Header.Set("Authorization", "Bearer "+editorToken)
	invalidReq.Header.Set("Content-Type", "application/json")
	invalidRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(invalidRec, invalidReq)
	if invalidRec.Code != http.StatusBadRequest {
		t.Fatalf("expected invalid json status 400, got %d body=%s", invalidRec.Code, invalidRec.Body.String())
	}
}

func TestPageRevisionHistoryEndpoint(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	tokenManager := appauth.NewTokenManager("super-secret-token", "note-app", 15*time.Minute)
	memberships := &testMembershipRepo{
		memberships: map[string][]domain.WorkspaceMember{
			"workspace-1": {
				{ID: "member-1", WorkspaceID: "workspace-1", UserID: "user-1", Role: domain.RoleEditor},
				{ID: "member-2", WorkspaceID: "workspace-1", UserID: "user-2", Role: domain.RoleViewer},
			},
		},
	}
	pages := &testPageRepo{
		pages: map[string]domain.Page{
			"page-1": {ID: "page-1", WorkspaceID: "workspace-1", Title: "Doc"},
		},
		drafts: map[string]domain.PageDraft{
			"page-1": {PageID: "page-1", Content: json.RawMessage(`[]`)},
		},
	}
	revisions := &testRevisionRepo{revisions: map[string]domain.Revision{}, ordered: []domain.Revision{
		{ID: "rev-1", PageID: "page-1", Label: stringPtrHTTP("First"), CreatedBy: "user-1", CreatedAt: time.Date(2026, 3, 7, 10, 0, 0, 0, time.UTC), Content: json.RawMessage(`[{"type":"paragraph"}]`)},
		{ID: "rev-2", PageID: "page-1", Label: stringPtrHTTP("Second"), CreatedBy: "user-1", CreatedAt: time.Date(2026, 3, 7, 11, 0, 0, 0, time.UTC), Content: json.RawMessage(`[{"type":"paragraph"}]`)},
	}}
	revisionService := application.NewRevisionService(revisions, pages, memberships)
	server := NewServer(logger, application.AuthService{}, application.WorkspaceService{}, application.FolderService{}, application.PageService{}, revisionService, tokenManager, storage.NewLocal(t.TempDir()))

	viewerToken, _, err := tokenManager.GenerateAccessToken("user-2", "viewer@example.com", time.Now().UTC())
	if err != nil {
		t.Fatalf("GenerateAccessToken() error = %v", err)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/pages/page-1/revisions", nil)
	listReq.Header.Set("Authorization", "Bearer "+viewerToken)
	listRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected list status 200, got %d body=%s", listRec.Code, listRec.Body.String())
	}

	var payload struct {
		Data []domain.Revision `json:"data"`
	}
	if err := json.Unmarshal(listRec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal revision list response: %v", err)
	}
	if len(payload.Data) != 2 {
		t.Fatalf("expected two revisions, got %d", len(payload.Data))
	}
	if payload.Data[0].ID != "rev-1" || payload.Data[1].ID != "rev-2" {
		t.Fatalf("unexpected order: %+v", payload.Data)
	}
	if payload.Data[0].Content != nil || payload.Data[1].Content != nil {
		t.Fatalf("expected history payload to omit content")
	}

	missingReq := httptest.NewRequest(http.MethodGet, "/api/v1/pages/missing-page/revisions", nil)
	missingReq.Header.Set("Authorization", "Bearer "+viewerToken)
	missingRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(missingRec, missingReq)
	if missingRec.Code != http.StatusNotFound {
		t.Fatalf("expected missing page status 404, got %d body=%s", missingRec.Code, missingRec.Body.String())
	}
}

func stringPtrHTTP(value string) *string {
	return &value
}

func TestPageRevisionCompareEndpoint(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	tokenManager := appauth.NewTokenManager("super-secret-token", "note-app", 15*time.Minute)
	memberships := &testMembershipRepo{
		memberships: map[string][]domain.WorkspaceMember{
			"workspace-1": {
				{ID: "member-1", WorkspaceID: "workspace-1", UserID: "user-1", Role: domain.RoleViewer},
			},
		},
	}
	pages := &testPageRepo{
		pages: map[string]domain.Page{
			"page-1": {ID: "page-1", WorkspaceID: "workspace-1", Title: "Doc"},
		},
		drafts: map[string]domain.PageDraft{
			"page-1": {PageID: "page-1", Content: json.RawMessage(`[]`)},
		},
	}
	revisions := &testRevisionRepo{revisions: map[string]domain.Revision{
		"rev-1": {ID: "rev-1", PageID: "page-1", CreatedBy: "user-1", CreatedAt: time.Date(2026, 3, 7, 10, 0, 0, 0, time.UTC), Content: json.RawMessage(`[{"type":"paragraph","children":[{"type":"text","text":"hello world"}]}]`)},
		"rev-2": {ID: "rev-2", PageID: "page-1", CreatedBy: "user-1", CreatedAt: time.Date(2026, 3, 7, 11, 0, 0, 0, time.UTC), Content: json.RawMessage(`[{"type":"paragraph","children":[{"type":"text","text":"hello brave world"}]},{"type":"image","src":"/uploads/a.png"}]`)},
		"rev-x": {ID: "rev-x", PageID: "page-x", CreatedBy: "user-1", CreatedAt: time.Date(2026, 3, 7, 12, 0, 0, 0, time.UTC), Content: json.RawMessage(`[{"type":"paragraph","children":[{"type":"text","text":"other"}]}]`)},
	}, ordered: []domain.Revision{}}
	revisionService := application.NewRevisionService(revisions, pages, memberships)
	server := NewServer(logger, application.AuthService{}, application.WorkspaceService{}, application.FolderService{}, application.PageService{}, revisionService, tokenManager, storage.NewLocal(t.TempDir()))

	viewerToken, _, err := tokenManager.GenerateAccessToken("user-1", "viewer@example.com", time.Now().UTC())
	if err != nil {
		t.Fatalf("GenerateAccessToken() error = %v", err)
	}

	compareReq := httptest.NewRequest(http.MethodGet, "/api/v1/pages/page-1/revisions/compare?from=rev-1&to=rev-2", nil)
	compareReq.Header.Set("Authorization", "Bearer "+viewerToken)
	compareRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(compareRec, compareReq)
	if compareRec.Code != http.StatusOK {
		t.Fatalf("expected compare status 200, got %d body=%s", compareRec.Code, compareRec.Body.String())
	}

	var payload struct {
		Data domain.RevisionDiff `json:"data"`
	}
	if err := json.Unmarshal(compareRec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal compare response: %v", err)
	}
	if payload.Data.FromRevisionID != "rev-1" || payload.Data.ToRevisionID != "rev-2" {
		t.Fatalf("unexpected compare payload: %+v", payload.Data)
	}
	if len(payload.Data.Blocks) != 2 {
		t.Fatalf("expected two diff blocks, got %d", len(payload.Data.Blocks))
	}
	if payload.Data.Blocks[0].Status != "modified" || payload.Data.Blocks[1].Status != "added" {
		t.Fatalf("unexpected diff blocks: %+v", payload.Data.Blocks)
	}
	if len(payload.Data.Blocks[0].Lines) != 2 || payload.Data.Blocks[0].Lines[0].Operation != "removed" || payload.Data.Blocks[0].Lines[1].Operation != "added" {
		t.Fatalf("expected compare payload to include line-level diff for modified block, got %+v", payload.Data.Blocks[0].Lines)
	}
	if len(payload.Data.Blocks[1].Lines) != 1 || payload.Data.Blocks[1].Lines[0].Operation != "added" {
		t.Fatalf("expected compare payload to include line-level diff for added block, got %+v", payload.Data.Blocks[1].Lines)
	}

	invalidReq := httptest.NewRequest(http.MethodGet, "/api/v1/pages/page-1/revisions/compare?from=rev-1&to=rev-x", nil)
	invalidReq.Header.Set("Authorization", "Bearer "+viewerToken)
	invalidRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(invalidRec, invalidReq)
	if invalidRec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected invalid comparison status 422, got %d body=%s", invalidRec.Code, invalidRec.Body.String())
	}

	missingReq := httptest.NewRequest(http.MethodGet, "/api/v1/pages/page-1/revisions/compare?from=rev-1&to=missing", nil)
	missingReq.Header.Set("Authorization", "Bearer "+viewerToken)
	missingRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(missingRec, missingReq)
	if missingRec.Code != http.StatusNotFound {
		t.Fatalf("expected missing revision status 404, got %d body=%s", missingRec.Code, missingRec.Body.String())
	}
}

func TestPageRevisionRestoreEndpoint(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	tokenManager := appauth.NewTokenManager("super-secret-token", "note-app", 15*time.Minute)
	memberships := &testMembershipRepo{
		memberships: map[string][]domain.WorkspaceMember{
			"workspace-1": {
				{ID: "member-1", WorkspaceID: "workspace-1", UserID: "user-1", Role: domain.RoleEditor},
				{ID: "member-2", WorkspaceID: "workspace-1", UserID: "user-2", Role: domain.RoleViewer},
			},
		},
	}
	pages := &testPageRepo{
		pages: map[string]domain.Page{
			"page-1": {ID: "page-1", WorkspaceID: "workspace-1", Title: "Doc"},
		},
		drafts: map[string]domain.PageDraft{
			"page-1": {PageID: "page-1", Content: json.RawMessage(`[{"type":"paragraph","children":[{"type":"text","text":"current"}]}]`), LastEditedBy: "user-1"},
		},
	}
	revisions := &testRevisionRepo{revisions: map[string]domain.Revision{
		"rev-1": {ID: "rev-1", PageID: "page-1", CreatedBy: "user-1", CreatedAt: time.Date(2026, 3, 7, 10, 0, 0, 0, time.UTC), Content: json.RawMessage(`[{"type":"paragraph","children":[{"type":"text","text":"old value"}]}]`)},
		"rev-2": {ID: "rev-2", PageID: "page-1", CreatedBy: "user-1", CreatedAt: time.Date(2026, 3, 7, 11, 0, 0, 0, time.UTC), Content: json.RawMessage(`[{"type":"paragraph","children":[{"type":"text","text":"current"}]}]`)},
		"rev-x": {ID: "rev-x", PageID: "page-x", CreatedBy: "user-1", CreatedAt: time.Date(2026, 3, 7, 12, 0, 0, 0, time.UTC), Content: json.RawMessage(`[{"type":"paragraph","children":[{"type":"text","text":"other"}]}]`)},
	}, ordered: []domain.Revision{
		{ID: "rev-1", PageID: "page-1", CreatedBy: "user-1", CreatedAt: time.Date(2026, 3, 7, 10, 0, 0, 0, time.UTC)},
		{ID: "rev-2", PageID: "page-1", CreatedBy: "user-1", CreatedAt: time.Date(2026, 3, 7, 11, 0, 0, 0, time.UTC)},
	}}
	revisionService := application.NewRevisionService(revisions, pages, memberships)
	server := NewServer(logger, application.AuthService{}, application.WorkspaceService{}, application.FolderService{}, application.PageService{}, revisionService, tokenManager, storage.NewLocal(t.TempDir()))

	editorToken, _, err := tokenManager.GenerateAccessToken("user-1", "editor@example.com", time.Now().UTC())
	if err != nil {
		t.Fatalf("GenerateAccessToken() error = %v", err)
	}
	viewerToken, _, err := tokenManager.GenerateAccessToken("user-2", "viewer@example.com", time.Now().UTC())
	if err != nil {
		t.Fatalf("GenerateAccessToken() error = %v", err)
	}

	restoreReq := httptest.NewRequest(http.MethodPost, "/api/v1/pages/page-1/revisions/rev-1/restore", nil)
	restoreReq.Header.Set("Authorization", "Bearer "+editorToken)
	restoreRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(restoreRec, restoreReq)
	if restoreRec.Code != http.StatusOK {
		t.Fatalf("expected restore status 200, got %d body=%s", restoreRec.Code, restoreRec.Body.String())
	}
	if string(pages.drafts["page-1"].Content) != string(revisions.revisions["rev-1"].Content) {
		t.Fatalf("expected draft content to be restored")
	}
	if len(revisions.ordered) != 3 {
		t.Fatalf("expected new revision event, got %d history entries", len(revisions.ordered))
	}

	viewerReq := httptest.NewRequest(http.MethodPost, "/api/v1/pages/page-1/revisions/rev-1/restore", nil)
	viewerReq.Header.Set("Authorization", "Bearer "+viewerToken)
	viewerRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(viewerRec, viewerReq)
	if viewerRec.Code != http.StatusForbidden {
		t.Fatalf("expected viewer restore status 403, got %d body=%s", viewerRec.Code, viewerRec.Body.String())
	}

	invalidReq := httptest.NewRequest(http.MethodPost, "/api/v1/pages/page-1/revisions/rev-x/restore", nil)
	invalidReq.Header.Set("Authorization", "Bearer "+editorToken)
	invalidRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(invalidRec, invalidReq)
	if invalidRec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected mismatched revision status 422, got %d body=%s", invalidRec.Code, invalidRec.Body.String())
	}
}
func TestCommentEndpoints(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	tokenManager := appauth.NewTokenManager("super-secret-token", "note-app", 15*time.Minute)
	memberships := &testMembershipRepo{
		memberships: map[string][]domain.WorkspaceMember{
			"workspace-1": {
				{ID: "member-1", WorkspaceID: "workspace-1", UserID: "user-1", Role: domain.RoleViewer},
				{ID: "member-2", WorkspaceID: "workspace-1", UserID: "user-2", Role: domain.RoleEditor},
			},
		},
	}
	pages := &testPageRepo{
		pages: map[string]domain.Page{
			"page-1": {ID: "page-1", WorkspaceID: "workspace-1", Title: "Doc"},
		},
		drafts: map[string]domain.PageDraft{
			"page-1": {PageID: "page-1", Content: json.RawMessage(`[]`)},
		},
	}
	comments := &testCommentRepo{comments: map[string]domain.PageComment{}, ordered: []domain.PageComment{}}
	commentService := application.NewCommentService(comments, pages, memberships)
	server := NewServer(logger, application.AuthService{}, application.WorkspaceService{}, application.FolderService{}, application.PageService{}, application.RevisionService{}, tokenManager, storage.NewLocal(t.TempDir())).WithCommentService(commentService)

	viewerToken, _, err := tokenManager.GenerateAccessToken("user-1", "viewer@example.com", time.Now().UTC())
	if err != nil {
		t.Fatalf("GenerateAccessToken() error = %v", err)
	}
	editorToken, _, err := tokenManager.GenerateAccessToken("user-2", "editor@example.com", time.Now().UTC())
	if err != nil {
		t.Fatalf("GenerateAccessToken() error = %v", err)
	}

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/pages/page-1/comments", bytes.NewBufferString(`{"body":"  Please verify this section  "}`))
	createReq.Header.Set("Authorization", "Bearer "+viewerToken)
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected create comment status 201, got %d body=%s", createRec.Code, createRec.Body.String())
	}

	var created struct {
		Data domain.PageComment `json:"data"`
	}
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal create comment response: %v", err)
	}
	if created.Data.Body != "Please verify this section" {
		t.Fatalf("unexpected comment body: %q", created.Data.Body)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/pages/page-1/comments", nil)
	listReq.Header.Set("Authorization", "Bearer "+viewerToken)
	listRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected list comments status 200, got %d body=%s", listRec.Code, listRec.Body.String())
	}

	var listed struct {
		Data []domain.PageComment `json:"data"`
	}
	if err := json.Unmarshal(listRec.Body.Bytes(), &listed); err != nil {
		t.Fatalf("unmarshal list comments response: %v", err)
	}
	if len(listed.Data) != 1 {
		t.Fatalf("expected one comment, got %d", len(listed.Data))
	}
	if listed.Data[0].ResolvedAt != nil {
		t.Fatalf("expected unresolved comment before resolve")
	}

	resolveReq := httptest.NewRequest(http.MethodPost, "/api/v1/comments/"+created.Data.ID+"/resolve", nil)
	resolveReq.Header.Set("Authorization", "Bearer "+editorToken)
	resolveRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(resolveRec, resolveReq)
	if resolveRec.Code != http.StatusOK {
		t.Fatalf("expected resolve status 200, got %d body=%s", resolveRec.Code, resolveRec.Body.String())
	}

	viewerResolveReq := httptest.NewRequest(http.MethodPost, "/api/v1/comments/"+created.Data.ID+"/resolve", nil)
	viewerResolveReq.Header.Set("Authorization", "Bearer "+viewerToken)
	viewerResolveRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(viewerResolveRec, viewerResolveReq)
	if viewerResolveRec.Code != http.StatusForbidden {
		t.Fatalf("expected viewer resolve status 403, got %d body=%s", viewerResolveRec.Code, viewerResolveRec.Body.String())
	}

	listAfterResolveReq := httptest.NewRequest(http.MethodGet, "/api/v1/pages/page-1/comments", nil)
	listAfterResolveReq.Header.Set("Authorization", "Bearer "+viewerToken)
	listAfterResolveRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(listAfterResolveRec, listAfterResolveReq)
	if listAfterResolveRec.Code != http.StatusOK {
		t.Fatalf("expected post-resolve list status 200, got %d body=%s", listAfterResolveRec.Code, listAfterResolveRec.Body.String())
	}

	var listedAfterResolve struct {
		Data []domain.PageComment `json:"data"`
	}
	if err := json.Unmarshal(listAfterResolveRec.Body.Bytes(), &listedAfterResolve); err != nil {
		t.Fatalf("unmarshal post-resolve list response: %v", err)
	}
	if len(listedAfterResolve.Data) != 1 || listedAfterResolve.Data[0].ResolvedAt == nil {
		t.Fatalf("expected resolved comment to remain visible: %+v", listedAfterResolve.Data)
	}

	emptyBodyReq := httptest.NewRequest(http.MethodPost, "/api/v1/pages/page-1/comments", bytes.NewBufferString(`{"body":"   "}`))
	emptyBodyReq.Header.Set("Authorization", "Bearer "+viewerToken)
	emptyBodyReq.Header.Set("Content-Type", "application/json")
	emptyBodyRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(emptyBodyRec, emptyBodyReq)
	if emptyBodyRec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected empty body status 422, got %d body=%s", emptyBodyRec.Code, emptyBodyRec.Body.String())
	}

	missingPageReq := httptest.NewRequest(http.MethodPost, "/api/v1/pages/missing-page/comments", bytes.NewBufferString(`{"body":"hello"}`))
	missingPageReq.Header.Set("Authorization", "Bearer "+viewerToken)
	missingPageReq.Header.Set("Content-Type", "application/json")
	missingPageRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(missingPageRec, missingPageReq)
	if missingPageRec.Code != http.StatusNotFound {
		t.Fatalf("expected missing page status 404, got %d body=%s", missingPageRec.Code, missingPageRec.Body.String())
	}
}

func TestThreadCreateEndpoint(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	tokenManager := appauth.NewTokenManager("super-secret-token", "note-app", 15*time.Minute)
	memberships := &testMembershipRepo{
		memberships: map[string][]domain.WorkspaceMember{
			"workspace-1": {
				{ID: "member-1", WorkspaceID: "workspace-1", UserID: "user-1", Role: domain.RoleViewer},
			},
		},
	}
	pages := &testPageRepo{
		pages: map[string]domain.Page{
			"page-1": {ID: "page-1", WorkspaceID: "workspace-1", Title: "Doc"},
		},
		drafts: map[string]domain.PageDraft{
			"page-1": {PageID: "page-1", Content: json.RawMessage(`[{"id":"block-1","type":"paragraph","children":[{"type":"text","text":"hello world"}]}]`)},
		},
	}
	threads := &testThreadRepo{details: map[string]domain.PageCommentThreadDetail{}}
	threadService := application.NewThreadService(threads, pages, memberships)
	server := NewServer(logger, application.AuthService{}, application.WorkspaceService{}, application.FolderService{}, application.PageService{}, application.RevisionService{}, tokenManager, storage.NewLocal(t.TempDir())).WithThreadService(threadService)

	viewerToken, _, err := tokenManager.GenerateAccessToken("user-1", "viewer@example.com", time.Now().UTC())
	if err != nil {
		t.Fatalf("GenerateAccessToken() error = %v", err)
	}

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/pages/page-1/threads", bytes.NewBufferString(`{"body":"  Please revise this line  ","anchor":{"type":"block","block_id":"block-1","quoted_text":"hello","quoted_block_text":"hello world"}}`))
	createReq.Header.Set("Authorization", "Bearer "+viewerToken)
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected create thread status 201, got %d body=%s", createRec.Code, createRec.Body.String())
	}

	var created struct {
		Data domain.PageCommentThreadDetail `json:"data"`
	}
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal create thread response: %v", err)
	}
	if created.Data.Thread.PageID != "page-1" || created.Data.Thread.Anchor.BlockID == nil || *created.Data.Thread.Anchor.BlockID != "block-1" {
		t.Fatalf("unexpected thread payload: %+v", created.Data.Thread)
	}
	if len(created.Data.Messages) != 1 || created.Data.Messages[0].Body != "Please revise this line" {
		t.Fatalf("unexpected starter message payload: %+v", created.Data.Messages)
	}
	if len(created.Data.Events) != 1 || created.Data.Events[0].Type != domain.PageCommentThreadEventTypeCreated {
		t.Fatalf("unexpected create thread events payload: %+v", created.Data.Events)
	}
	if len(threads.createdMentions) != 1 || len(threads.createdMentions[0]) != 0 {
		t.Fatalf("expected no starter mentions by default, got %+v", threads.createdMentions)
	}

	mentionsReq := httptest.NewRequest(http.MethodPost, "/api/v1/pages/page-1/threads", bytes.NewBufferString(`{"body":"hello","mentions":[" user-2 ","user-3","user-2"],"anchor":{"type":"block","block_id":"block-1"}}`))
	mentionsReq.Header.Set("Authorization", "Bearer "+viewerToken)
	mentionsReq.Header.Set("Content-Type", "application/json")
	mentionsRec := httptest.NewRecorder()
	pages.drafts["page-1"] = domain.PageDraft{PageID: "page-1", Content: json.RawMessage(`[{"id":"block-1","type":"paragraph","children":[{"type":"text","text":"hello world"}]}]`)}
	memberships.memberships["workspace-1"] = []domain.WorkspaceMember{
		{ID: "member-1", WorkspaceID: "workspace-1", UserID: "user-1", Role: domain.RoleViewer},
		{ID: "member-2", WorkspaceID: "workspace-1", UserID: "user-2", Role: domain.RoleEditor},
		{ID: "member-3", WorkspaceID: "workspace-1", UserID: "user-3", Role: domain.RoleEditor},
	}
	server.Handler().ServeHTTP(mentionsRec, mentionsReq)
	if mentionsRec.Code != http.StatusCreated {
		t.Fatalf("expected create thread with mentions status 201, got %d body=%s", mentionsRec.Code, mentionsRec.Body.String())
	}
	if len(threads.createdMentions) < 2 || len(threads.createdMentions[1]) != 2 || threads.createdMentions[1][0].MentionedUserID != "user-2" || threads.createdMentions[1][1].MentionedUserID != "user-3" {
		t.Fatalf("unexpected request mention rows: %+v", threads.createdMentions)
	}

	invalidReq := httptest.NewRequest(http.MethodPost, "/api/v1/pages/page-1/threads", bytes.NewBufferString(`{"body":"hello","anchor":{"type":"block","block_id":"missing","quoted_block_text":"hello world"}}`))
	invalidReq.Header.Set("Authorization", "Bearer "+viewerToken)
	invalidReq.Header.Set("Content-Type", "application/json")
	invalidRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(invalidRec, invalidReq)
	if invalidRec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected invalid anchor status 422, got %d body=%s", invalidRec.Code, invalidRec.Body.String())
	}

	invalidMentionsReq := httptest.NewRequest(http.MethodPost, "/api/v1/pages/page-1/threads", bytes.NewBufferString(`{"body":"hello","mentions":"user-2","anchor":{"type":"block","block_id":"block-1"}}`))
	invalidMentionsReq.Header.Set("Authorization", "Bearer "+viewerToken)
	invalidMentionsReq.Header.Set("Content-Type", "application/json")
	invalidMentionsRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(invalidMentionsRec, invalidMentionsReq)
	if invalidMentionsRec.Code != http.StatusBadRequest {
		t.Fatalf("expected invalid mentions type status 400, got %d body=%s", invalidMentionsRec.Code, invalidMentionsRec.Body.String())
	}

	blankMentionReq := httptest.NewRequest(http.MethodPost, "/api/v1/pages/page-1/threads", bytes.NewBufferString(`{"body":"hello","mentions":["   "],"anchor":{"type":"block","block_id":"block-1"}}`))
	blankMentionReq.Header.Set("Authorization", "Bearer "+viewerToken)
	blankMentionReq.Header.Set("Content-Type", "application/json")
	blankMentionRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(blankMentionRec, blankMentionReq)
	if blankMentionRec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected blank mention status 422, got %d body=%s", blankMentionRec.Code, blankMentionRec.Body.String())
	}

	nonMemberMentionReq := httptest.NewRequest(http.MethodPost, "/api/v1/pages/page-1/threads", bytes.NewBufferString(`{"body":"hello","mentions":["user-x"],"anchor":{"type":"block","block_id":"block-1"}}`))
	nonMemberMentionReq.Header.Set("Authorization", "Bearer "+viewerToken)
	nonMemberMentionReq.Header.Set("Content-Type", "application/json")
	nonMemberMentionRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(nonMemberMentionRec, nonMemberMentionReq)
	if nonMemberMentionRec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected non-member mention status 422, got %d body=%s", nonMemberMentionRec.Code, nonMemberMentionRec.Body.String())
	}

	tooManyMentionsReq := httptest.NewRequest(http.MethodPost, "/api/v1/pages/page-1/threads", bytes.NewBufferString(`{"body":"hello","mentions":["user-2","user-3","user-4","user-5","user-6","user-7","user-8","user-9","user-10","user-11","user-12","user-13","user-14","user-15","user-16","user-17","user-18","user-19","user-20","user-21","user-22"],"anchor":{"type":"block","block_id":"block-1"}}`))
	tooManyMentionsReq.Header.Set("Authorization", "Bearer "+viewerToken)
	tooManyMentionsReq.Header.Set("Content-Type", "application/json")
	tooManyMentionsRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(tooManyMentionsRec, tooManyMentionsReq)
	if tooManyMentionsRec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected too many mentions status 422, got %d body=%s", tooManyMentionsRec.Code, tooManyMentionsRec.Body.String())
	}

	malformedReq := httptest.NewRequest(http.MethodPost, "/api/v1/pages/page-1/threads", bytes.NewBufferString(`{"body":"hello"`))
	malformedReq.Header.Set("Authorization", "Bearer "+viewerToken)
	malformedReq.Header.Set("Content-Type", "application/json")
	malformedRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(malformedRec, malformedReq)
	if malformedRec.Code != http.StatusBadRequest {
		t.Fatalf("expected malformed create thread status 400, got %d body=%s", malformedRec.Code, malformedRec.Body.String())
	}

	nonMemberToken, _, err := tokenManager.GenerateAccessToken("user-x", "outsider@example.com", time.Now().UTC())
	if err != nil {
		t.Fatalf("GenerateAccessToken() non-member error = %v", err)
	}
	nonMemberReq := httptest.NewRequest(http.MethodPost, "/api/v1/pages/page-1/threads", bytes.NewBufferString(`{"body":"hello","anchor":{"type":"block","block_id":"block-1"}}`))
	nonMemberReq.Header.Set("Authorization", "Bearer "+nonMemberToken)
	nonMemberReq.Header.Set("Content-Type", "application/json")
	nonMemberRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(nonMemberRec, nonMemberReq)
	if nonMemberRec.Code != http.StatusNotFound {
		t.Fatalf("expected non-member create thread status 404, got %d body=%s", nonMemberRec.Code, nonMemberRec.Body.String())
	}
}

func TestThreadNotificationPreferenceEndpoint(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	tokenManager := appauth.NewTokenManager("super-secret-token", "note-app", 15*time.Minute)
	memberships := &testMembershipRepo{
		memberships: map[string][]domain.WorkspaceMember{
			"workspace-1": {
				{ID: "member-1", WorkspaceID: "workspace-1", UserID: "user-1", Role: domain.RoleViewer},
				{ID: "member-2", WorkspaceID: "workspace-1", UserID: "user-2", Role: domain.RoleEditor},
			},
		},
	}
	pages := &testPageRepo{
		pages: map[string]domain.Page{
			"page-1": {ID: "page-1", WorkspaceID: "workspace-1", Title: "Doc"},
		},
		drafts: map[string]domain.PageDraft{
			"page-1": {PageID: "page-1", Content: json.RawMessage(`[{"id":"block-1","type":"paragraph","children":[{"type":"text","text":"hello world"}]}]`)},
		},
	}
	blockID := "block-1"
	threads := &testThreadRepo{details: map[string]domain.PageCommentThreadDetail{
		"thread-1": {
			Thread: domain.PageCommentThread{
				ID:          "thread-1",
				PageID:      "page-1",
				Anchor:      domain.PageCommentThreadAnchor{Type: domain.PageCommentThreadAnchorTypeBlock, BlockID: &blockID, QuotedBlockText: "hello world"},
				ThreadState: domain.PageCommentThreadStateOpen,
				AnchorState: domain.PageCommentThreadAnchorStateActive,
				CreatedBy:   "user-1",
			},
			Messages: []domain.PageCommentThreadMessage{
				{ID: "message-1", ThreadID: "thread-1", Body: "Please revise this line", CreatedBy: "user-1"},
			},
		},
	}}
	prefs := &testThreadNotificationPreferenceRepo{
		preferences: map[string]domain.ThreadNotificationPreference{
			"thread-1:user-2": {
				ThreadID:  "thread-1",
				UserID:    "user-2",
				Mode:      domain.ThreadNotificationModeMute,
				CreatedAt: time.Date(2026, 4, 9, 4, 0, 0, 0, time.UTC),
				UpdatedAt: time.Date(2026, 4, 9, 4, 0, 0, 0, time.UTC),
			},
		},
	}
	threadService := application.NewThreadService(threads, pages, memberships, prefs)
	server := NewServer(logger, application.AuthService{}, application.WorkspaceService{}, application.FolderService{}, application.PageService{}, application.RevisionService{}, tokenManager, storage.NewLocal(t.TempDir())).WithThreadService(threadService)

	viewerToken, _, err := tokenManager.GenerateAccessToken("user-1", "viewer@example.com", time.Now().UTC())
	if err != nil {
		t.Fatalf("GenerateAccessToken() viewer error = %v", err)
	}
	editorToken, _, err := tokenManager.GenerateAccessToken("user-2", "editor@example.com", time.Now().UTC())
	if err != nil {
		t.Fatalf("GenerateAccessToken() editor error = %v", err)
	}
	outsiderToken, _, err := tokenManager.GenerateAccessToken("user-x", "outsider@example.com", time.Now().UTC())
	if err != nil {
		t.Fatalf("GenerateAccessToken() outsider error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/threads/thread-1/notification-preference", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected missing auth status 401, got %d body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/threads/thread-1/notification-preference", nil)
	req.Header.Set("Authorization", "Bearer "+viewerToken)
	rec = httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected default preference status 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var defaultPayload struct {
		Data domain.ThreadNotificationPreferenceView `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &defaultPayload); err != nil {
		t.Fatalf("unmarshal default preference response: %v", err)
	}
	if defaultPayload.Data.ThreadID != "thread-1" || defaultPayload.Data.Mode != domain.ThreadNotificationModeAll {
		t.Fatalf("unexpected default preference response: %+v", defaultPayload.Data)
	}

	putReq := httptest.NewRequest(http.MethodPut, "/api/v1/threads/thread-1/notification-preference", bytes.NewBufferString(`{"mode":"mute"}`))
	putReq.Header.Set("Content-Type", "application/json")
	putRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(putRec, putReq)
	if putRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected missing auth put status 401, got %d body=%s", putRec.Code, putRec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/threads/thread-1/notification-preference", nil)
	req.Header.Set("Authorization", "Bearer "+editorToken)
	rec = httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected stored preference status 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var storedPayload struct {
		Data domain.ThreadNotificationPreferenceView `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &storedPayload); err != nil {
		t.Fatalf("unmarshal stored preference response: %v", err)
	}
	if storedPayload.Data.ThreadID != "thread-1" || storedPayload.Data.Mode != domain.ThreadNotificationModeMute {
		t.Fatalf("unexpected stored preference response: %+v", storedPayload.Data)
	}

	putReq = httptest.NewRequest(http.MethodPut, "/api/v1/threads/thread-1/notification-preference", bytes.NewBufferString(`{"mode":"mentions_only"}`))
	putReq.Header.Set("Authorization", "Bearer "+editorToken)
	putReq.Header.Set("Content-Type", "application/json")
	putRec = httptest.NewRecorder()
	server.Handler().ServeHTTP(putRec, putReq)
	if putRec.Code != http.StatusOK {
		t.Fatalf("expected update preference status 200, got %d body=%s", putRec.Code, putRec.Body.String())
	}
	var updatePayload struct {
		Data domain.ThreadNotificationPreferenceUpdateResult `json:"data"`
	}
	if err := json.Unmarshal(putRec.Body.Bytes(), &updatePayload); err != nil {
		t.Fatalf("unmarshal update preference response: %v", err)
	}
	if updatePayload.Data.ThreadID != "thread-1" || updatePayload.Data.Mode != domain.ThreadNotificationModeMentionsOnly || updatePayload.Data.UpdatedAt.IsZero() {
		t.Fatalf("unexpected update preference response: %+v", updatePayload.Data)
	}
	if len(prefs.writes) == 0 {
		t.Fatal("expected preference writes to be recorded")
	}
	lastWrite := prefs.writes[len(prefs.writes)-1]
	if lastWrite.ThreadID != "thread-1" || lastWrite.UserID != "user-2" || lastWrite.Mode != domain.ThreadNotificationModeMentionsOnly {
		t.Fatalf("unexpected stored write: %+v", lastWrite)
	}
	if got := prefs.preferences["thread-1:user-2"]; got.Mode != domain.ThreadNotificationModeMentionsOnly {
		t.Fatalf("unexpected stored updated preference: %+v", got)
	}

	putReq = httptest.NewRequest(http.MethodPut, "/api/v1/threads/thread-1/notification-preference", bytes.NewBufferString(`{"mode":"all"}`))
	putReq.Header.Set("Authorization", "Bearer "+editorToken)
	putReq.Header.Set("Content-Type", "application/json")
	putRec = httptest.NewRecorder()
	server.Handler().ServeHTTP(putRec, putReq)
	if putRec.Code != http.StatusOK {
		t.Fatalf("expected delete-to-default status 200, got %d body=%s", putRec.Code, putRec.Body.String())
	}
	var defaultWritePayload struct {
		Data domain.ThreadNotificationPreferenceUpdateResult `json:"data"`
	}
	if err := json.Unmarshal(putRec.Body.Bytes(), &defaultWritePayload); err != nil {
		t.Fatalf("unmarshal delete-to-default response: %v", err)
	}
	if defaultWritePayload.Data.ThreadID != "thread-1" || defaultWritePayload.Data.Mode != domain.ThreadNotificationModeAll || defaultWritePayload.Data.UpdatedAt.IsZero() {
		t.Fatalf("unexpected delete-to-default response: %+v", defaultWritePayload.Data)
	}
	if _, ok := prefs.preferences["thread-1:user-2"]; ok {
		t.Fatalf("expected stored preference deleted after mode=all, got %+v", prefs.preferences)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/threads/thread-1/notification-preference", nil)
	req.Header.Set("Authorization", "Bearer "+outsiderToken)
	rec = httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected outsider status 404, got %d body=%s", rec.Code, rec.Body.String())
	}

	putReq = httptest.NewRequest(http.MethodPut, "/api/v1/threads/thread-1/notification-preference", bytes.NewBufferString(`{"mode":"mute"}`))
	putReq.Header.Set("Authorization", "Bearer "+outsiderToken)
	putReq.Header.Set("Content-Type", "application/json")
	putRec = httptest.NewRecorder()
	server.Handler().ServeHTTP(putRec, putReq)
	if putRec.Code != http.StatusNotFound {
		t.Fatalf("expected outsider put status 404, got %d body=%s", putRec.Code, putRec.Body.String())
	}

	missingPutReq := httptest.NewRequest(http.MethodPut, "/api/v1/threads/thread-missing/notification-preference", bytes.NewBufferString(`{"mode":"mute"}`))
	missingPutReq.Header.Set("Authorization", "Bearer "+viewerToken)
	missingPutReq.Header.Set("Content-Type", "application/json")
	missingPutRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(missingPutRec, missingPutReq)
	if missingPutRec.Code != http.StatusNotFound {
		t.Fatalf("expected missing thread put status 404, got %d body=%s", missingPutRec.Code, missingPutRec.Body.String())
	}

	trashedThreads := &testThreadRepo{details: map[string]domain.PageCommentThreadDetail{
		"thread-trashed": {
			Thread: domain.PageCommentThread{
				ID:          "thread-trashed",
				PageID:      "page-missing",
				Anchor:      domain.PageCommentThreadAnchor{Type: domain.PageCommentThreadAnchorTypeBlock, BlockID: &blockID, QuotedBlockText: "hello world"},
				ThreadState: domain.PageCommentThreadStateOpen,
				AnchorState: domain.PageCommentThreadAnchorStateMissing,
				CreatedBy:   "user-1",
			},
			Messages: []domain.PageCommentThreadMessage{{ID: "message-1", ThreadID: "thread-trashed", Body: "Please revise this line", CreatedBy: "user-1"}},
		},
	}}
	trashedServer := NewServer(logger, application.AuthService{}, application.WorkspaceService{}, application.FolderService{}, application.PageService{}, application.RevisionService{}, tokenManager, storage.NewLocal(t.TempDir())).WithThreadService(application.NewThreadService(trashedThreads, &testPageRepo{pages: map[string]domain.Page{}, drafts: map[string]domain.PageDraft{}}, memberships, prefs))
	trashedReq := httptest.NewRequest(http.MethodGet, "/api/v1/threads/thread-trashed/notification-preference", nil)
	trashedReq.Header.Set("Authorization", "Bearer "+viewerToken)
	trashedRec := httptest.NewRecorder()
	trashedServer.Handler().ServeHTTP(trashedRec, trashedReq)
	if trashedRec.Code != http.StatusNotFound {
		t.Fatalf("expected trashed thread status 404, got %d body=%s", trashedRec.Code, trashedRec.Body.String())
	}

	trashedPutReq := httptest.NewRequest(http.MethodPut, "/api/v1/threads/thread-trashed/notification-preference", bytes.NewBufferString(`{"mode":"mute"}`))
	trashedPutReq.Header.Set("Authorization", "Bearer "+viewerToken)
	trashedPutReq.Header.Set("Content-Type", "application/json")
	trashedPutRec := httptest.NewRecorder()
	trashedServer.Handler().ServeHTTP(trashedPutRec, trashedPutReq)
	if trashedPutRec.Code != http.StatusNotFound {
		t.Fatalf("expected trashed put status 404, got %d body=%s", trashedPutRec.Code, trashedPutRec.Body.String())
	}
}

func TestThreadNotificationPreferenceEndpointRejectsInvalidMode(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	tokenManager := appauth.NewTokenManager("super-secret-token", "note-app", 15*time.Minute)
	memberships := &testMembershipRepo{
		memberships: map[string][]domain.WorkspaceMember{
			"workspace-1": {
				{ID: "member-1", WorkspaceID: "workspace-1", UserID: "user-1", Role: domain.RoleViewer},
			},
		},
	}
	pages := &testPageRepo{
		pages: map[string]domain.Page{
			"page-1": {ID: "page-1", WorkspaceID: "workspace-1", Title: "Doc"},
		},
		drafts: map[string]domain.PageDraft{
			"page-1": {PageID: "page-1", Content: json.RawMessage(`[{"id":"block-1","type":"paragraph","children":[{"type":"text","text":"hello world"}]}]`)} ,
		},
	}
	blockID := "block-1"
	threads := &testThreadRepo{details: map[string]domain.PageCommentThreadDetail{
		"thread-1": {
			Thread: domain.PageCommentThread{
				ID:          "thread-1",
				PageID:      "page-1",
				Anchor:      domain.PageCommentThreadAnchor{Type: domain.PageCommentThreadAnchorTypeBlock, BlockID: &blockID, QuotedBlockText: "hello world"},
				ThreadState: domain.PageCommentThreadStateOpen,
				AnchorState: domain.PageCommentThreadAnchorStateActive,
				CreatedBy:   "user-1",
			},
		},
	}}
	threadService := application.NewThreadService(threads, pages, memberships, &testThreadNotificationPreferenceRepo{})
	server := NewServer(logger, application.AuthService{}, application.WorkspaceService{}, application.FolderService{}, application.PageService{}, application.RevisionService{}, tokenManager, storage.NewLocal(t.TempDir())).WithThreadService(threadService)

	viewerToken, _, err := tokenManager.GenerateAccessToken("user-1", "viewer@example.com", time.Now().UTC())
	if err != nil {
		t.Fatalf("GenerateAccessToken() error = %v", err)
	}

	for _, tc := range []struct {
		name string
		body string
	}{
		{name: "missing mode", body: `{}`},
		{name: "blank mode", body: `{"mode":" "}`},
		{name: "invalid mode", body: `{"mode":"bogus"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPut, "/api/v1/threads/thread-1/notification-preference", bytes.NewBufferString(tc.body))
			req.Header.Set("Authorization", "Bearer "+viewerToken)
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			server.Handler().ServeHTTP(rec, req)
			if rec.Code != http.StatusUnprocessableEntity {
				t.Fatalf("expected validation status 422, got %d body=%s", rec.Code, rec.Body.String())
			}
			var payload struct {
				Error APIError `json:"error"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
				t.Fatalf("unmarshal validation response: %v", err)
			}
			if payload.Error.Code != "validation_failed" {
				t.Fatalf("expected validation_failed error code, got %+v", payload.Error)
			}
		})
	}
}

func TestThreadCreateResolveAndReopenReturnNotFoundWhenPageIsTrashed(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	tokenManager := appauth.NewTokenManager("super-secret-token", "note-app", 15*time.Minute)
	memberships := &testMembershipRepo{
		memberships: map[string][]domain.WorkspaceMember{
			"workspace-1": {
				{ID: "member-owner", WorkspaceID: "workspace-1", UserID: "owner-1", Role: domain.RoleOwner},
				{ID: "member-viewer", WorkspaceID: "workspace-1", UserID: "viewer-1", Role: domain.RoleViewer},
			},
		},
	}
	pages := &testPageRepo{
		pages:  map[string]domain.Page{},
		drafts: map[string]domain.PageDraft{},
	}
	blockID := "block-1"
	threads := &testThreadRepo{details: map[string]domain.PageCommentThreadDetail{
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
	threadService := application.NewThreadService(threads, pages, memberships)
	server := NewServer(logger, application.AuthService{}, application.WorkspaceService{}, application.FolderService{}, application.PageService{}, application.RevisionService{}, tokenManager, storage.NewLocal(t.TempDir())).WithThreadService(threadService)

	ownerToken, _, err := tokenManager.GenerateAccessToken("owner-1", "owner@example.com", time.Now().UTC())
	if err != nil {
		t.Fatalf("GenerateAccessToken() error = %v", err)
	}
	viewerToken, _, err := tokenManager.GenerateAccessToken("viewer-1", "viewer@example.com", time.Now().UTC())
	if err != nil {
		t.Fatalf("GenerateAccessToken() error = %v", err)
	}

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/pages/page-1/threads", bytes.NewBufferString(`{"body":"hello","anchor":{"type":"block","block_id":"block-1"}}`))
	createReq.Header.Set("Authorization", "Bearer "+viewerToken)
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusNotFound {
		t.Fatalf("expected trashed create thread status 404, got %d body=%s", createRec.Code, createRec.Body.String())
	}

	resolveReq := httptest.NewRequest(http.MethodPost, "/api/v1/threads/thread-1/resolve", bytes.NewBufferString(`{"resolve_note":"  Fixed in latest revision  "}`))
	resolveReq.Header.Set("Authorization", "Bearer "+ownerToken)
	resolveRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(resolveRec, resolveReq)
	if resolveRec.Code != http.StatusNotFound {
		t.Fatalf("expected trashed resolve thread status 404, got %d body=%s", resolveRec.Code, resolveRec.Body.String())
	}

	reopenReq := httptest.NewRequest(http.MethodPost, "/api/v1/threads/thread-1/reopen", nil)
	reopenReq.Header.Set("Authorization", "Bearer "+viewerToken)
	reopenRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(reopenRec, reopenReq)
	if reopenRec.Code != http.StatusNotFound {
		t.Fatalf("expected trashed reopen thread status 404, got %d body=%s", reopenRec.Code, reopenRec.Body.String())
	}
}

func TestThreadDetailEndpoint(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	tokenManager := appauth.NewTokenManager("super-secret-token", "note-app", 15*time.Minute)
	memberships := &testMembershipRepo{
		memberships: map[string][]domain.WorkspaceMember{
			"workspace-1": {
				{ID: "member-1", WorkspaceID: "workspace-1", UserID: "user-1", Role: domain.RoleViewer},
			},
		},
	}
	pages := &testPageRepo{
		pages: map[string]domain.Page{
			"page-1": {ID: "page-1", WorkspaceID: "workspace-1", Title: "Doc"},
		},
		drafts: map[string]domain.PageDraft{
			"page-1": {PageID: "page-1", Content: json.RawMessage(`[]`)},
		},
	}
	blockID := "block-1"
	threads := &testThreadRepo{details: map[string]domain.PageCommentThreadDetail{
		"thread-1": {
			Thread: domain.PageCommentThread{
				ID:             "thread-1",
				PageID:         "page-1",
				Anchor:         domain.PageCommentThreadAnchor{Type: domain.PageCommentThreadAnchorTypeBlock, BlockID: &blockID, QuotedBlockText: "hello world"},
				ThreadState:    domain.PageCommentThreadStateOpen,
				AnchorState:    domain.PageCommentThreadAnchorStateActive,
				CreatedBy:      "user-1",
				CreatedAt:      time.Date(2026, 3, 19, 8, 0, 0, 0, time.UTC),
				LastActivityAt: time.Date(2026, 3, 19, 8, 1, 0, 0, time.UTC),
				ReplyCount:     2,
			},
			Messages: []domain.PageCommentThreadMessage{
				{ID: "message-1", ThreadID: "thread-1", Body: "Please revise this line", CreatedBy: "user-1", CreatedAt: time.Date(2026, 3, 19, 8, 0, 0, 0, time.UTC)},
				{ID: "message-2", ThreadID: "thread-1", Body: "Second reply", CreatedBy: "user-1", CreatedAt: time.Date(2026, 3, 19, 8, 1, 0, 0, time.UTC)},
			},
			Events: []domain.PageCommentThreadEvent{
				{ID: "event-1", ThreadID: "thread-1", Type: domain.PageCommentThreadEventTypeCreated, ActorID: stringPtr("user-1"), CreatedAt: time.Date(2026, 3, 19, 8, 0, 0, 0, time.UTC)},
			},
		},
	}}
	threadService := application.NewThreadService(threads, pages, memberships)
	server := NewServer(logger, application.AuthService{}, application.WorkspaceService{}, application.FolderService{}, application.PageService{}, application.RevisionService{}, tokenManager, storage.NewLocal(t.TempDir())).WithThreadService(threadService)

	viewerToken, _, err := tokenManager.GenerateAccessToken("user-1", "viewer@example.com", time.Now().UTC())
	if err != nil {
		t.Fatalf("GenerateAccessToken() error = %v", err)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/threads/thread-1", nil)
	getReq.Header.Set("Authorization", "Bearer "+viewerToken)
	getRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("expected get thread status 200, got %d body=%s", getRec.Code, getRec.Body.String())
	}

	var payload struct {
		Data domain.PageCommentThreadDetail `json:"data"`
	}
	if err := json.Unmarshal(getRec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal get thread response: %v", err)
	}
	if payload.Data.Thread.ID != "thread-1" || len(payload.Data.Messages) != 2 || len(payload.Data.Events) != 1 || payload.Data.Events[0].Type != domain.PageCommentThreadEventTypeCreated {
		t.Fatalf("unexpected thread detail payload: %+v", payload.Data)
	}

	missingReq := httptest.NewRequest(http.MethodGet, "/api/v1/threads/missing-thread", nil)
	missingReq.Header.Set("Authorization", "Bearer "+viewerToken)
	missingRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(missingRec, missingReq)
	if missingRec.Code != http.StatusNotFound {
		t.Fatalf("expected missing thread status 404, got %d body=%s", missingRec.Code, missingRec.Body.String())
	}

	nonMemberToken, _, err := tokenManager.GenerateAccessToken("user-x", "outsider@example.com", time.Now().UTC())
	if err != nil {
		t.Fatalf("GenerateAccessToken() non-member error = %v", err)
	}
	nonMemberReq := httptest.NewRequest(http.MethodGet, "/api/v1/threads/thread-1", nil)
	nonMemberReq.Header.Set("Authorization", "Bearer "+nonMemberToken)
	nonMemberRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(nonMemberRec, nonMemberReq)
	if nonMemberRec.Code != http.StatusNotFound {
		t.Fatalf("expected non-member thread detail status 404, got %d body=%s", nonMemberRec.Code, nonMemberRec.Body.String())
	}
}

func TestThreadEndpointsReturnNotFoundWhenPageIsTrashed(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	tokenManager := appauth.NewTokenManager("super-secret-token", "note-app", 15*time.Minute)
	memberships := &testMembershipRepo{
		memberships: map[string][]domain.WorkspaceMember{
			"workspace-1": {
				{ID: "member-1", WorkspaceID: "workspace-1", UserID: "user-1", Role: domain.RoleViewer},
			},
		},
	}
	pages := &testPageRepo{
		pages:  map[string]domain.Page{},
		drafts: map[string]domain.PageDraft{},
	}
	blockID := "block-1"
	threads := &testThreadRepo{details: map[string]domain.PageCommentThreadDetail{
		"thread-1": {
			Thread: domain.PageCommentThread{
				ID:             "thread-1",
				PageID:         "page-1",
				Anchor:         domain.PageCommentThreadAnchor{Type: domain.PageCommentThreadAnchorTypeBlock, BlockID: &blockID, QuotedBlockText: "hello world"},
				ThreadState:    domain.PageCommentThreadStateOpen,
				AnchorState:    domain.PageCommentThreadAnchorStateMissing,
				CreatedBy:      "user-1",
				CreatedAt:      time.Date(2026, 3, 19, 8, 0, 0, 0, time.UTC),
				LastActivityAt: time.Date(2026, 3, 19, 8, 0, 0, 0, time.UTC),
				ReplyCount:     1,
			},
			Messages: []domain.PageCommentThreadMessage{
				{ID: "message-1", ThreadID: "thread-1", Body: "Please revise this line", CreatedBy: "user-1", CreatedAt: time.Date(2026, 3, 19, 8, 0, 0, 0, time.UTC)},
			},
		},
	}}
	threadService := application.NewThreadService(threads, pages, memberships)
	server := NewServer(logger, application.AuthService{}, application.WorkspaceService{}, application.FolderService{}, application.PageService{}, application.RevisionService{}, tokenManager, storage.NewLocal(t.TempDir())).WithThreadService(threadService)

	viewerToken, _, err := tokenManager.GenerateAccessToken("user-1", "viewer@example.com", time.Now().UTC())
	if err != nil {
		t.Fatalf("GenerateAccessToken() error = %v", err)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/threads/thread-1", nil)
	getReq.Header.Set("Authorization", "Bearer "+viewerToken)
	getRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusNotFound {
		t.Fatalf("expected trashed thread detail status 404, got %d body=%s", getRec.Code, getRec.Body.String())
	}

	replyReq := httptest.NewRequest(http.MethodPost, "/api/v1/threads/thread-1/replies", bytes.NewBufferString(`{"body":"reply"}`))
	replyReq.Header.Set("Authorization", "Bearer "+viewerToken)
	replyReq.Header.Set("Content-Type", "application/json")
	replyRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(replyRec, replyReq)
	if replyRec.Code != http.StatusNotFound {
		t.Fatalf("expected trashed thread reply status 404, got %d body=%s", replyRec.Code, replyRec.Body.String())
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/pages/page-1/threads", nil)
	listReq.Header.Set("Authorization", "Bearer "+viewerToken)
	listRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusNotFound {
		t.Fatalf("expected trashed page thread list status 404, got %d body=%s", listRec.Code, listRec.Body.String())
	}
}

func TestThreadListEndpoint(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	tokenManager := appauth.NewTokenManager("super-secret-token", "note-app", 15*time.Minute)
	memberships := &testMembershipRepo{
		memberships: map[string][]domain.WorkspaceMember{
			"workspace-1": {
				{ID: "member-1", WorkspaceID: "workspace-1", UserID: "user-1", Role: domain.RoleViewer},
			},
		},
	}
	pages := &testPageRepo{
		pages: map[string]domain.Page{
			"page-1": {ID: "page-1", WorkspaceID: "workspace-1", Title: "Doc"},
		},
		drafts: map[string]domain.PageDraft{
			"page-1": {PageID: "page-1", Content: json.RawMessage(`[]`)},
		},
	}
	blockIDOne := "block-1"
	blockIDTwo := "block-2"
	quotedText := "hello"
	threads := &testThreadRepo{details: map[string]domain.PageCommentThreadDetail{
		"thread-open": {
			Thread: domain.PageCommentThread{
				ID:             "thread-open",
				PageID:         "page-1",
				Anchor:         domain.PageCommentThreadAnchor{Type: domain.PageCommentThreadAnchorTypeBlock, BlockID: &blockIDOne, QuotedText: &quotedText, QuotedBlockText: "hello world"},
				ThreadState:    domain.PageCommentThreadStateOpen,
				AnchorState:    domain.PageCommentThreadAnchorStateActive,
				CreatedBy:      "user-1",
				CreatedAt:      time.Date(2026, 3, 19, 8, 0, 0, 0, time.UTC),
				LastActivityAt: time.Date(2026, 3, 19, 8, 2, 0, 0, time.UTC),
			},
			Messages: []domain.PageCommentThreadMessage{{ID: "message-1", ThreadID: "thread-open", Body: "Please revise this line", CreatedBy: "user-1", CreatedAt: time.Date(2026, 3, 19, 8, 0, 0, 0, time.UTC)}},
		},
		"thread-resolved": {
			Thread: domain.PageCommentThread{
				ID:             "thread-resolved",
				PageID:         "page-1",
				Anchor:         domain.PageCommentThreadAnchor{Type: domain.PageCommentThreadAnchorTypeBlock, BlockID: &blockIDTwo, QuotedBlockText: "architecture notes"},
				ThreadState:    domain.PageCommentThreadStateResolved,
				AnchorState:    domain.PageCommentThreadAnchorStateMissing,
				CreatedBy:      "owner-1",
				CreatedAt:      time.Date(2026, 3, 19, 8, 1, 0, 0, time.UTC),
				LastActivityAt: time.Date(2026, 3, 19, 8, 1, 0, 0, time.UTC),
			},
			Messages: []domain.PageCommentThreadMessage{{ID: "message-2", ThreadID: "thread-resolved", Body: "Archived discussion", CreatedBy: "user-1", CreatedAt: time.Date(2026, 3, 19, 8, 1, 0, 0, time.UTC)}},
		},
		"thread-outdated": {
			Thread: domain.PageCommentThread{
				ID:             "thread-outdated",
				PageID:         "page-1",
				Anchor:         domain.PageCommentThreadAnchor{Type: domain.PageCommentThreadAnchorTypeBlock, BlockID: &blockIDTwo, QuotedBlockText: "stale text"},
				ThreadState:    domain.PageCommentThreadStateOpen,
				AnchorState:    domain.PageCommentThreadAnchorStateOutdated,
				CreatedBy:      "editor-1",
				CreatedAt:      time.Date(2026, 3, 19, 8, 3, 0, 0, time.UTC),
				LastActivityAt: time.Date(2026, 3, 19, 8, 3, 0, 0, time.UTC),
			},
			Messages: []domain.PageCommentThreadMessage{{ID: "message-3", ThreadID: "thread-outdated", Body: "Outdated discussion", CreatedBy: "user-1", CreatedAt: time.Date(2026, 3, 19, 8, 3, 0, 0, time.UTC)}},
		},
	}}
	threadService := application.NewThreadService(threads, pages, memberships)
	server := NewServer(logger, application.AuthService{}, application.WorkspaceService{}, application.FolderService{}, application.PageService{}, application.RevisionService{}, tokenManager, storage.NewLocal(t.TempDir())).WithThreadService(threadService)

	viewerToken, _, err := tokenManager.GenerateAccessToken("user-1", "viewer@example.com", time.Now().UTC())
	if err != nil {
		t.Fatalf("GenerateAccessToken() error = %v", err)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/pages/page-1/threads?thread_state=open&q=revise", nil)
	listReq.Header.Set("Authorization", "Bearer "+viewerToken)
	listRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected list threads status 200, got %d body=%s", listRec.Code, listRec.Body.String())
	}

	var payload struct {
		Data domain.PageCommentThreadList `json:"data"`
	}
	if err := json.Unmarshal(listRec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal list threads response: %v", err)
	}
	if len(payload.Data.Threads) != 1 || payload.Data.Threads[0].ID != "thread-open" {
		t.Fatalf("unexpected thread list payload: %+v", payload.Data)
	}
	if payload.Data.Counts.Open != 2 || payload.Data.Counts.Resolved != 1 || payload.Data.Counts.Active != 1 || payload.Data.Counts.Outdated != 1 || payload.Data.Counts.Missing != 1 {
		t.Fatalf("unexpected thread counts payload: %+v", payload.Data.Counts)
	}

	quotedReq := httptest.NewRequest(http.MethodGet, "/api/v1/pages/page-1/threads?q=hello", nil)
	quotedReq.Header.Set("Authorization", "Bearer "+viewerToken)
	quotedRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(quotedRec, quotedReq)
	if quotedRec.Code != http.StatusOK {
		t.Fatalf("expected quoted text search status 200, got %d body=%s", quotedRec.Code, quotedRec.Body.String())
	}

	var quotedPayload struct {
		Data domain.PageCommentThreadList `json:"data"`
	}
	if err := json.Unmarshal(quotedRec.Body.Bytes(), &quotedPayload); err != nil {
		t.Fatalf("unmarshal quoted search response: %v", err)
	}
	if len(quotedPayload.Data.Threads) != 1 || quotedPayload.Data.Threads[0].ID != "thread-open" {
		t.Fatalf("expected quoted text search to match thread-open, got %+v", quotedPayload.Data.Threads)
	}

	createdByReq := httptest.NewRequest(http.MethodGet, "/api/v1/pages/page-1/threads?created_by=me", nil)
	createdByReq.Header.Set("Authorization", "Bearer "+viewerToken)
	createdByRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(createdByRec, createdByReq)
	if createdByRec.Code != http.StatusOK {
		t.Fatalf("expected created_by=me status 200, got %d body=%s", createdByRec.Code, createdByRec.Body.String())
	}

	var createdByPayload struct {
		Data domain.PageCommentThreadList `json:"data"`
	}
	if err := json.Unmarshal(createdByRec.Body.Bytes(), &createdByPayload); err != nil {
		t.Fatalf("unmarshal created_by=me response: %v", err)
	}
	if len(createdByPayload.Data.Threads) != 1 || createdByPayload.Data.Threads[0].ID != "thread-open" {
		t.Fatalf("expected created_by=me to match viewer-owned thread, got %+v", createdByPayload.Data.Threads)
	}

	missingAnchorReq := httptest.NewRequest(http.MethodGet, "/api/v1/pages/page-1/threads?has_missing_anchor=true", nil)
	missingAnchorReq.Header.Set("Authorization", "Bearer "+viewerToken)
	missingAnchorRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(missingAnchorRec, missingAnchorReq)
	if missingAnchorRec.Code != http.StatusOK {
		t.Fatalf("expected has_missing_anchor=true status 200, got %d body=%s", missingAnchorRec.Code, missingAnchorRec.Body.String())
	}

	var missingAnchorPayload struct {
		Data domain.PageCommentThreadList `json:"data"`
	}
	if err := json.Unmarshal(missingAnchorRec.Body.Bytes(), &missingAnchorPayload); err != nil {
		t.Fatalf("unmarshal has_missing_anchor=true response: %v", err)
	}
	if len(missingAnchorPayload.Data.Threads) != 1 || missingAnchorPayload.Data.Threads[0].ID != "thread-resolved" {
		t.Fatalf("expected has_missing_anchor=true to match missing thread, got %+v", missingAnchorPayload.Data.Threads)
	}

	noMissingAnchorReq := httptest.NewRequest(http.MethodGet, "/api/v1/pages/page-1/threads?has_missing_anchor=false", nil)
	noMissingAnchorReq.Header.Set("Authorization", "Bearer "+viewerToken)
	noMissingAnchorRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(noMissingAnchorRec, noMissingAnchorReq)
	if noMissingAnchorRec.Code != http.StatusOK {
		t.Fatalf("expected has_missing_anchor=false status 200, got %d body=%s", noMissingAnchorRec.Code, noMissingAnchorRec.Body.String())
	}

	var noMissingAnchorPayload struct {
		Data domain.PageCommentThreadList `json:"data"`
	}
	if err := json.Unmarshal(noMissingAnchorRec.Body.Bytes(), &noMissingAnchorPayload); err != nil {
		t.Fatalf("unmarshal has_missing_anchor=false response: %v", err)
	}
	if len(noMissingAnchorPayload.Data.Threads) != 2 {
		t.Fatalf("expected has_missing_anchor=false to exclude missing thread, got %+v", noMissingAnchorPayload.Data.Threads)
	}

	outdatedAnchorReq := httptest.NewRequest(http.MethodGet, "/api/v1/pages/page-1/threads?has_outdated_anchor=true", nil)
	outdatedAnchorReq.Header.Set("Authorization", "Bearer "+viewerToken)
	outdatedAnchorRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(outdatedAnchorRec, outdatedAnchorReq)
	if outdatedAnchorRec.Code != http.StatusOK {
		t.Fatalf("expected has_outdated_anchor=true status 200, got %d body=%s", outdatedAnchorRec.Code, outdatedAnchorRec.Body.String())
	}

	var outdatedAnchorPayload struct {
		Data domain.PageCommentThreadList `json:"data"`
	}
	if err := json.Unmarshal(outdatedAnchorRec.Body.Bytes(), &outdatedAnchorPayload); err != nil {
		t.Fatalf("unmarshal has_outdated_anchor=true response: %v", err)
	}
	if len(outdatedAnchorPayload.Data.Threads) != 1 || outdatedAnchorPayload.Data.Threads[0].ID != "thread-outdated" {
		t.Fatalf("expected has_outdated_anchor=true to match outdated thread, got %+v", outdatedAnchorPayload.Data.Threads)
	}

	noOutdatedAnchorReq := httptest.NewRequest(http.MethodGet, "/api/v1/pages/page-1/threads?has_outdated_anchor=false", nil)
	noOutdatedAnchorReq.Header.Set("Authorization", "Bearer "+viewerToken)
	noOutdatedAnchorRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(noOutdatedAnchorRec, noOutdatedAnchorReq)
	if noOutdatedAnchorRec.Code != http.StatusOK {
		t.Fatalf("expected has_outdated_anchor=false status 200, got %d body=%s", noOutdatedAnchorRec.Code, noOutdatedAnchorRec.Body.String())
	}

	var noOutdatedAnchorPayload struct {
		Data domain.PageCommentThreadList `json:"data"`
	}
	if err := json.Unmarshal(noOutdatedAnchorRec.Body.Bytes(), &noOutdatedAnchorPayload); err != nil {
		t.Fatalf("unmarshal has_outdated_anchor=false response: %v", err)
	}
	if len(noOutdatedAnchorPayload.Data.Threads) != 2 {
		t.Fatalf("expected has_outdated_anchor=false to exclude outdated thread, got %+v", noOutdatedAnchorPayload.Data.Threads)
	}

	newestReq := httptest.NewRequest(http.MethodGet, "/api/v1/pages/page-1/threads?sort=newest", nil)
	newestReq.Header.Set("Authorization", "Bearer "+viewerToken)
	newestRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(newestRec, newestReq)
	if newestRec.Code != http.StatusOK {
		t.Fatalf("expected sort=newest status 200, got %d body=%s", newestRec.Code, newestRec.Body.String())
	}

	var newestPayload struct {
		Data domain.PageCommentThreadList `json:"data"`
	}
	if err := json.Unmarshal(newestRec.Body.Bytes(), &newestPayload); err != nil {
		t.Fatalf("unmarshal sort=newest response: %v", err)
	}
	if len(newestPayload.Data.Threads) != 3 || newestPayload.Data.Threads[0].ID != "thread-outdated" || newestPayload.Data.Threads[1].ID != "thread-resolved" || newestPayload.Data.Threads[2].ID != "thread-open" {
		t.Fatalf("expected sort=newest order, got %+v", newestPayload.Data.Threads)
	}

	oldestReq := httptest.NewRequest(http.MethodGet, "/api/v1/pages/page-1/threads?sort=oldest", nil)
	oldestReq.Header.Set("Authorization", "Bearer "+viewerToken)
	oldestRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(oldestRec, oldestReq)
	if oldestRec.Code != http.StatusOK {
		t.Fatalf("expected sort=oldest status 200, got %d body=%s", oldestRec.Code, oldestRec.Body.String())
	}

	var oldestPayload struct {
		Data domain.PageCommentThreadList `json:"data"`
	}
	if err := json.Unmarshal(oldestRec.Body.Bytes(), &oldestPayload); err != nil {
		t.Fatalf("unmarshal sort=oldest response: %v", err)
	}
	if len(oldestPayload.Data.Threads) != 3 || oldestPayload.Data.Threads[0].ID != "thread-open" || oldestPayload.Data.Threads[1].ID != "thread-resolved" || oldestPayload.Data.Threads[2].ID != "thread-outdated" {
		t.Fatalf("expected sort=oldest order, got %+v", oldestPayload.Data.Threads)
	}

	recentActivityReq := httptest.NewRequest(http.MethodGet, "/api/v1/pages/page-1/threads?sort=recent_activity", nil)
	recentActivityReq.Header.Set("Authorization", "Bearer "+viewerToken)
	recentActivityRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(recentActivityRec, recentActivityReq)
	if recentActivityRec.Code != http.StatusOK {
		t.Fatalf("expected sort=recent_activity status 200, got %d body=%s", recentActivityRec.Code, recentActivityRec.Body.String())
	}

	var recentActivityPayload struct {
		Data domain.PageCommentThreadList `json:"data"`
	}
	if err := json.Unmarshal(recentActivityRec.Body.Bytes(), &recentActivityPayload); err != nil {
		t.Fatalf("unmarshal sort=recent_activity response: %v", err)
	}
	if len(recentActivityPayload.Data.Threads) != 3 || recentActivityPayload.Data.Threads[0].ID != "thread-outdated" || recentActivityPayload.Data.Threads[1].ID != "thread-open" || recentActivityPayload.Data.Threads[2].ID != "thread-resolved" {
		t.Fatalf("expected sort=recent_activity order, got %+v", recentActivityPayload.Data.Threads)
	}

	paginatedReq := httptest.NewRequest(http.MethodGet, "/api/v1/pages/page-1/threads?sort=recent_activity&limit=2", nil)
	paginatedReq.Header.Set("Authorization", "Bearer "+viewerToken)
	paginatedRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(paginatedRec, paginatedReq)
	if paginatedRec.Code != http.StatusOK {
		t.Fatalf("expected paginated thread list status 200, got %d body=%s", paginatedRec.Code, paginatedRec.Body.String())
	}

	var paginatedPayload struct {
		Data domain.PageCommentThreadList `json:"data"`
	}
	if err := json.Unmarshal(paginatedRec.Body.Bytes(), &paginatedPayload); err != nil {
		t.Fatalf("unmarshal paginated thread list response: %v", err)
	}
	if len(paginatedPayload.Data.Threads) != 2 || paginatedPayload.Data.Threads[0].ID != "thread-outdated" || paginatedPayload.Data.Threads[1].ID != "thread-open" {
		t.Fatalf("unexpected paginated thread payload: %+v", paginatedPayload.Data.Threads)
	}
	if !paginatedPayload.Data.HasMore || paginatedPayload.Data.NextCursor == nil || *paginatedPayload.Data.NextCursor != "thread-open" {
		t.Fatalf("expected next cursor after paginated page, got %+v", paginatedPayload.Data)
	}

	cursorReq := httptest.NewRequest(http.MethodGet, "/api/v1/pages/page-1/threads?sort=recent_activity&limit=2&cursor="+*paginatedPayload.Data.NextCursor, nil)
	cursorReq.Header.Set("Authorization", "Bearer "+viewerToken)
	cursorRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(cursorRec, cursorReq)
	if cursorRec.Code != http.StatusOK {
		t.Fatalf("expected cursor thread list status 200, got %d body=%s", cursorRec.Code, cursorRec.Body.String())
	}

	var cursorPayload struct {
		Data domain.PageCommentThreadList `json:"data"`
	}
	if err := json.Unmarshal(cursorRec.Body.Bytes(), &cursorPayload); err != nil {
		t.Fatalf("unmarshal cursor thread list response: %v", err)
	}
	if len(cursorPayload.Data.Threads) != 1 || cursorPayload.Data.Threads[0].ID != "thread-resolved" {
		t.Fatalf("unexpected cursor thread payload: %+v", cursorPayload.Data.Threads)
	}
	if cursorPayload.Data.HasMore || cursorPayload.Data.NextCursor != nil {
		t.Fatalf("expected final cursor page without next cursor, got %+v", cursorPayload.Data)
	}

	invalidReq := httptest.NewRequest(http.MethodGet, "/api/v1/pages/page-1/threads?thread_state=broken", nil)
	invalidReq.Header.Set("Authorization", "Bearer "+viewerToken)
	invalidRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(invalidRec, invalidReq)
	if invalidRec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected invalid thread_state status 422, got %d body=%s", invalidRec.Code, invalidRec.Body.String())
	}

	invalidAnchorReq := httptest.NewRequest(http.MethodGet, "/api/v1/pages/page-1/threads?anchor_state=broken", nil)
	invalidAnchorReq.Header.Set("Authorization", "Bearer "+viewerToken)
	invalidAnchorRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(invalidAnchorRec, invalidAnchorReq)
	if invalidAnchorRec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected invalid anchor_state status 422, got %d body=%s", invalidAnchorRec.Code, invalidAnchorRec.Body.String())
	}

	invalidCreatedByReq := httptest.NewRequest(http.MethodGet, "/api/v1/pages/page-1/threads?created_by=user-1", nil)
	invalidCreatedByReq.Header.Set("Authorization", "Bearer "+viewerToken)
	invalidCreatedByRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(invalidCreatedByRec, invalidCreatedByReq)
	if invalidCreatedByRec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected invalid created_by status 422, got %d body=%s", invalidCreatedByRec.Code, invalidCreatedByRec.Body.String())
	}

	invalidMissingAnchorReq := httptest.NewRequest(http.MethodGet, "/api/v1/pages/page-1/threads?has_missing_anchor=maybe", nil)
	invalidMissingAnchorReq.Header.Set("Authorization", "Bearer "+viewerToken)
	invalidMissingAnchorRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(invalidMissingAnchorRec, invalidMissingAnchorReq)
	if invalidMissingAnchorRec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected invalid has_missing_anchor status 422, got %d body=%s", invalidMissingAnchorRec.Code, invalidMissingAnchorRec.Body.String())
	}

	invalidOutdatedAnchorReq := httptest.NewRequest(http.MethodGet, "/api/v1/pages/page-1/threads?has_outdated_anchor=maybe", nil)
	invalidOutdatedAnchorReq.Header.Set("Authorization", "Bearer "+viewerToken)
	invalidOutdatedAnchorRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(invalidOutdatedAnchorRec, invalidOutdatedAnchorReq)
	if invalidOutdatedAnchorRec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected invalid has_outdated_anchor status 422, got %d body=%s", invalidOutdatedAnchorRec.Code, invalidOutdatedAnchorRec.Body.String())
	}

	invalidSortReq := httptest.NewRequest(http.MethodGet, "/api/v1/pages/page-1/threads?sort=broken", nil)
	invalidSortReq.Header.Set("Authorization", "Bearer "+viewerToken)
	invalidSortRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(invalidSortRec, invalidSortReq)
	if invalidSortRec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected invalid sort status 422, got %d body=%s", invalidSortRec.Code, invalidSortRec.Body.String())
	}

	invalidLimitReq := httptest.NewRequest(http.MethodGet, "/api/v1/pages/page-1/threads?limit=0", nil)
	invalidLimitReq.Header.Set("Authorization", "Bearer "+viewerToken)
	invalidLimitRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(invalidLimitRec, invalidLimitReq)
	if invalidLimitRec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected invalid limit status 422, got %d body=%s", invalidLimitRec.Code, invalidLimitRec.Body.String())
	}

	invalidCursorReq := httptest.NewRequest(http.MethodGet, "/api/v1/pages/page-1/threads?cursor=broken", nil)
	invalidCursorReq.Header.Set("Authorization", "Bearer "+viewerToken)
	invalidCursorRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(invalidCursorRec, invalidCursorReq)
	if invalidCursorRec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected invalid cursor status 422, got %d body=%s", invalidCursorRec.Code, invalidCursorRec.Body.String())
	}

	nonMemberToken, _, err := tokenManager.GenerateAccessToken("user-x", "outsider@example.com", time.Now().UTC())
	if err != nil {
		t.Fatalf("GenerateAccessToken() non-member error = %v", err)
	}
	nonMemberReq := httptest.NewRequest(http.MethodGet, "/api/v1/pages/page-1/threads", nil)
	nonMemberReq.Header.Set("Authorization", "Bearer "+nonMemberToken)
	nonMemberRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(nonMemberRec, nonMemberReq)
	if nonMemberRec.Code != http.StatusNotFound {
		t.Fatalf("expected non-member list threads status 404, got %d body=%s", nonMemberRec.Code, nonMemberRec.Body.String())
	}
}

func TestWorkspaceThreadListEndpoint(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	tokenManager := appauth.NewTokenManager("super-secret-token", "note-app", 15*time.Minute)
	memberships := &testMembershipRepo{
		memberships: map[string][]domain.WorkspaceMember{
			"workspace-1": {
				{ID: "member-1", WorkspaceID: "workspace-1", UserID: "user-1", Role: domain.RoleViewer},
			},
		},
	}
	pages := &testPageRepo{
		pages: map[string]domain.Page{
			"page-1": {ID: "page-1", WorkspaceID: "workspace-1", Title: "Doc"},
			"page-2": {ID: "page-2", WorkspaceID: "workspace-1", Title: "Architecture"},
		},
		drafts: map[string]domain.PageDraft{
			"page-1": {PageID: "page-1", Content: json.RawMessage(`[]`)},
			"page-2": {PageID: "page-2", Content: json.RawMessage(`[]`)},
		},
	}
	blockIDOne := "block-1"
	blockIDTwo := "block-2"
	threads := &testThreadRepo{details: map[string]domain.PageCommentThreadDetail{
		"thread-open": {
			Thread: domain.PageCommentThread{
				ID:             "thread-open",
				PageID:         "page-1",
				Anchor:         domain.PageCommentThreadAnchor{Type: domain.PageCommentThreadAnchorTypeBlock, BlockID: &blockIDOne, QuotedBlockText: "hello world"},
				ThreadState:    domain.PageCommentThreadStateOpen,
				AnchorState:    domain.PageCommentThreadAnchorStateActive,
				CreatedBy:      "user-1",
				CreatedAt:      time.Date(2026, 3, 19, 8, 0, 0, 0, time.UTC),
				LastActivityAt: time.Date(2026, 3, 19, 8, 2, 0, 0, time.UTC),
			},
			Messages: []domain.PageCommentThreadMessage{{ID: "message-1", ThreadID: "thread-open", Body: "Please revise this line", CreatedBy: "user-1", CreatedAt: time.Date(2026, 3, 19, 8, 0, 0, 0, time.UTC)}},
		},
		"thread-outdated": {
			Thread: domain.PageCommentThread{
				ID:             "thread-outdated",
				PageID:         "page-2",
				Anchor:         domain.PageCommentThreadAnchor{Type: domain.PageCommentThreadAnchorTypeBlock, BlockID: &blockIDTwo, QuotedBlockText: "stale text"},
				ThreadState:    domain.PageCommentThreadStateOpen,
				AnchorState:    domain.PageCommentThreadAnchorStateOutdated,
				CreatedBy:      "editor-1",
				CreatedAt:      time.Date(2026, 3, 19, 8, 3, 0, 0, time.UTC),
				LastActivityAt: time.Date(2026, 3, 19, 8, 3, 0, 0, time.UTC),
			},
			Messages: []domain.PageCommentThreadMessage{{ID: "message-2", ThreadID: "thread-outdated", Body: "Outdated discussion", CreatedBy: "editor-1", CreatedAt: time.Date(2026, 3, 19, 8, 3, 0, 0, time.UTC)}},
		},
	}}
	threadService := application.NewThreadService(threads, pages, memberships)
	server := NewServer(logger, application.AuthService{}, application.WorkspaceService{}, application.FolderService{}, application.PageService{}, application.RevisionService{}, tokenManager, storage.NewLocal(t.TempDir())).WithThreadService(threadService)

	viewerToken, _, err := tokenManager.GenerateAccessToken("user-1", "viewer@example.com", time.Now().UTC())
	if err != nil {
		t.Fatalf("GenerateAccessToken() error = %v", err)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces/workspace-1/threads?has_outdated_anchor=true&sort=recent_activity", nil)
	listReq.Header.Set("Authorization", "Bearer "+viewerToken)
	listRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected workspace thread list status 200, got %d body=%s", listRec.Code, listRec.Body.String())
	}

	var payload struct {
		Data domain.WorkspaceCommentThreadList `json:"data"`
	}
	if err := json.Unmarshal(listRec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal workspace thread list response: %v", err)
	}
	if len(payload.Data.Threads) != 1 || payload.Data.Threads[0].Thread.ID != "thread-outdated" || payload.Data.Threads[0].Page.ID != "page-2" {
		t.Fatalf("unexpected workspace thread list payload: %+v", payload.Data)
	}
	if payload.Data.Counts.Open != 2 || payload.Data.Counts.Outdated != 1 {
		t.Fatalf("unexpected workspace thread counts payload: %+v", payload.Data.Counts)
	}

	searchReq := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces/workspace-1/threads?q=architecture", nil)
	searchReq.Header.Set("Authorization", "Bearer "+viewerToken)
	searchRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(searchRec, searchReq)
	if searchRec.Code != http.StatusOK {
		t.Fatalf("expected workspace thread search status 200, got %d body=%s", searchRec.Code, searchRec.Body.String())
	}

	var searchPayload struct {
		Data domain.WorkspaceCommentThreadList `json:"data"`
	}
	if err := json.Unmarshal(searchRec.Body.Bytes(), &searchPayload); err != nil {
		t.Fatalf("unmarshal workspace thread search response: %v", err)
	}
	if len(searchPayload.Data.Threads) != 1 || searchPayload.Data.Threads[0].Thread.ID != "thread-outdated" || searchPayload.Data.Threads[0].Page.Title != "Architecture" {
		t.Fatalf("unexpected workspace thread search payload: %+v", searchPayload.Data)
	}

	createdByReq := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces/workspace-1/threads?created_by=me", nil)
	createdByReq.Header.Set("Authorization", "Bearer "+viewerToken)
	createdByRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(createdByRec, createdByReq)
	if createdByRec.Code != http.StatusOK {
		t.Fatalf("expected workspace created_by=me status 200, got %d body=%s", createdByRec.Code, createdByRec.Body.String())
	}

	var createdByPayload struct {
		Data domain.WorkspaceCommentThreadList `json:"data"`
	}
	if err := json.Unmarshal(createdByRec.Body.Bytes(), &createdByPayload); err != nil {
		t.Fatalf("unmarshal workspace created_by response: %v", err)
	}
	if len(createdByPayload.Data.Threads) != 1 || createdByPayload.Data.Threads[0].Thread.ID != "thread-open" {
		t.Fatalf("unexpected workspace created_by payload: %+v", createdByPayload.Data)
	}

	paginatedReq := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces/workspace-1/threads?sort=recent_activity&limit=1", nil)
	paginatedReq.Header.Set("Authorization", "Bearer "+viewerToken)
	paginatedRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(paginatedRec, paginatedReq)
	if paginatedRec.Code != http.StatusOK {
		t.Fatalf("expected paginated workspace thread list status 200, got %d body=%s", paginatedRec.Code, paginatedRec.Body.String())
	}

	var paginatedPayload struct {
		Data domain.WorkspaceCommentThreadList `json:"data"`
	}
	if err := json.Unmarshal(paginatedRec.Body.Bytes(), &paginatedPayload); err != nil {
		t.Fatalf("unmarshal paginated workspace thread response: %v", err)
	}
	if len(paginatedPayload.Data.Threads) != 1 || paginatedPayload.Data.Threads[0].Thread.ID != "thread-outdated" {
		t.Fatalf("unexpected paginated workspace thread payload: %+v", paginatedPayload.Data.Threads)
	}
	if !paginatedPayload.Data.HasMore || paginatedPayload.Data.NextCursor == nil || *paginatedPayload.Data.NextCursor != "thread-outdated" {
		t.Fatalf("expected workspace next cursor after first result, got %+v", paginatedPayload.Data)
	}

	invalidReq := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces/workspace-1/threads?sort=broken", nil)
	invalidReq.Header.Set("Authorization", "Bearer "+viewerToken)
	invalidRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(invalidRec, invalidReq)
	if invalidRec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected invalid workspace thread sort status 422, got %d body=%s", invalidRec.Code, invalidRec.Body.String())
	}

	invalidCreatedByReq := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces/workspace-1/threads?created_by=user-1", nil)
	invalidCreatedByReq.Header.Set("Authorization", "Bearer "+viewerToken)
	invalidCreatedByRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(invalidCreatedByRec, invalidCreatedByReq)
	if invalidCreatedByRec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected invalid workspace thread created_by status 422, got %d body=%s", invalidCreatedByRec.Code, invalidCreatedByRec.Body.String())
	}

	invalidLimitReq := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces/workspace-1/threads?limit=500", nil)
	invalidLimitReq.Header.Set("Authorization", "Bearer "+viewerToken)
	invalidLimitRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(invalidLimitRec, invalidLimitReq)
	if invalidLimitRec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected invalid workspace thread limit status 422, got %d body=%s", invalidLimitRec.Code, invalidLimitRec.Body.String())
	}

	invalidCursorReq := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces/workspace-1/threads?cursor=broken", nil)
	invalidCursorReq.Header.Set("Authorization", "Bearer "+viewerToken)
	invalidCursorRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(invalidCursorRec, invalidCursorReq)
	if invalidCursorRec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected invalid workspace thread cursor status 422, got %d body=%s", invalidCursorRec.Code, invalidCursorRec.Body.String())
	}

	nonMemberToken, _, err := tokenManager.GenerateAccessToken("user-x", "outsider@example.com", time.Now().UTC())
	if err != nil {
		t.Fatalf("GenerateAccessToken() non-member error = %v", err)
	}
	nonMemberReq := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces/workspace-1/threads", nil)
	nonMemberReq.Header.Set("Authorization", "Bearer "+nonMemberToken)
	nonMemberRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(nonMemberRec, nonMemberReq)
	if nonMemberRec.Code != http.StatusForbidden {
		t.Fatalf("expected non-member workspace thread list status 403, got %d body=%s", nonMemberRec.Code, nonMemberRec.Body.String())
	}
}

func TestThreadReplyEndpoint(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	tokenManager := appauth.NewTokenManager("super-secret-token", "note-app", 15*time.Minute)
	memberships := &testMembershipRepo{
		memberships: map[string][]domain.WorkspaceMember{
			"workspace-1": {
				{ID: "member-1", WorkspaceID: "workspace-1", UserID: "user-1", Role: domain.RoleViewer},
				{ID: "member-2", WorkspaceID: "workspace-1", UserID: "user-2", Role: domain.RoleEditor},
				{ID: "member-3", WorkspaceID: "workspace-1", UserID: "user-3", Role: domain.RoleEditor},
			},
		},
	}
	pages := &testPageRepo{
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
	threads := &testThreadRepo{details: map[string]domain.PageCommentThreadDetail{
		"thread-1": {
			Thread: domain.PageCommentThread{
				ID:             "thread-1",
				PageID:         "page-1",
				Anchor:         domain.PageCommentThreadAnchor{Type: domain.PageCommentThreadAnchorTypeBlock, BlockID: &blockID, QuotedBlockText: "hello world"},
				ThreadState:    domain.PageCommentThreadStateResolved,
				AnchorState:    domain.PageCommentThreadAnchorStateActive,
				CreatedBy:      "user-1",
				CreatedAt:      resolvedAt.Add(-time.Minute),
				ResolvedBy:     &resolvedBy,
				ResolvedAt:     &resolvedAt,
				LastActivityAt: resolvedAt,
				ReplyCount:     1,
			},
			Messages: []domain.PageCommentThreadMessage{
				{ID: "message-1", ThreadID: "thread-1", Body: "Initial comment", CreatedBy: "user-1", CreatedAt: resolvedAt.Add(-time.Minute)},
			},
		},
	}}
	threadService := application.NewThreadService(threads, pages, memberships)
	server := NewServer(logger, application.AuthService{}, application.WorkspaceService{}, application.FolderService{}, application.PageService{}, application.RevisionService{}, tokenManager, storage.NewLocal(t.TempDir())).WithThreadService(threadService)

	viewerToken, _, err := tokenManager.GenerateAccessToken("user-1", "viewer@example.com", time.Now().UTC())
	if err != nil {
		t.Fatalf("GenerateAccessToken() error = %v", err)
	}

	replyReq := httptest.NewRequest(http.MethodPost, "/api/v1/threads/thread-1/replies", bytes.NewBufferString(`{"body":"  Follow-up reply  ","mentions":[" user-2 ","user-3","user-2"]}`))
	replyReq.Header.Set("Authorization", "Bearer "+viewerToken)
	replyReq.Header.Set("Content-Type", "application/json")
	replyRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(replyRec, replyReq)
	if replyRec.Code != http.StatusCreated {
		t.Fatalf("expected create reply status 201, got %d body=%s", replyRec.Code, replyRec.Body.String())
	}

	var payload struct {
		Data domain.PageCommentThreadDetail `json:"data"`
	}
	if err := json.Unmarshal(replyRec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal reply response: %v", err)
	}
	if payload.Data.Thread.ThreadState != domain.PageCommentThreadStateOpen || len(payload.Data.Messages) != 2 || payload.Data.Messages[1].Body != "Follow-up reply" {
		t.Fatalf("unexpected reply payload: %+v", payload.Data)
	}
	if len(threads.replyMentions) != 1 || len(threads.replyMentions[0]) != 2 || threads.replyMentions[0][0].MentionedUserID != "user-2" || threads.replyMentions[0][1].MentionedUserID != "user-3" {
		t.Fatalf("unexpected reply mentions: %+v", threads.replyMentions)
	}
	if len(threads.replyOutboxEvents) != 1 {
		t.Fatalf("expected one reply outbox event, got %+v", threads.replyOutboxEvents)
	}
	replyEvent := threads.replyOutboxEvents[0]
	if replyEvent.Topic != domain.OutboxTopicThreadReplyCreated || replyEvent.AggregateType != domain.OutboxAggregateTypeThreadMessage || replyEvent.AggregateID != payload.Data.Messages[1].ID {
		t.Fatalf("unexpected reply outbox identity: %+v", replyEvent)
	}
	if replyEvent.IdempotencyKey != "thread_reply_created:"+payload.Data.Messages[1].ID {
		t.Fatalf("unexpected reply outbox idempotency key: %+v", replyEvent)
	}
	var replyPayload struct {
		MentionUserIDs []string `json:"mention_user_ids"`
	}
	if err := json.Unmarshal(replyEvent.Payload, &replyPayload); err != nil {
		t.Fatalf("unmarshal reply outbox payload: %v", err)
	}
	if len(replyPayload.MentionUserIDs) != 2 || replyPayload.MentionUserIDs[0] != "user-2" || replyPayload.MentionUserIDs[1] != "user-3" {
		t.Fatalf("unexpected reply outbox mention ids: %+v", replyPayload.MentionUserIDs)
	}

	omittedReq := httptest.NewRequest(http.MethodPost, "/api/v1/threads/thread-1/replies", bytes.NewBufferString(`{"body":"Second reply"}`))
	omittedReq.Header.Set("Authorization", "Bearer "+viewerToken)
	omittedReq.Header.Set("Content-Type", "application/json")
	omittedRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(omittedRec, omittedReq)
	if omittedRec.Code != http.StatusCreated {
		t.Fatalf("expected omitted mentions status 201, got %d body=%s", omittedRec.Code, omittedRec.Body.String())
	}
	if len(threads.replyMentions) != 2 || len(threads.replyMentions[1]) != 0 {
		t.Fatalf("expected omitted mentions to record an empty mention batch, got %+v", threads.replyMentions)
	}

	nullMentionsReq := httptest.NewRequest(http.MethodPost, "/api/v1/threads/thread-1/replies", bytes.NewBufferString(`{"body":"Null reply","mentions":null}`))
	nullMentionsReq.Header.Set("Authorization", "Bearer "+viewerToken)
	nullMentionsReq.Header.Set("Content-Type", "application/json")
	nullMentionsRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(nullMentionsRec, nullMentionsReq)
	if nullMentionsRec.Code != http.StatusCreated {
		t.Fatalf("expected null mentions status 201, got %d body=%s", nullMentionsRec.Code, nullMentionsRec.Body.String())
	}
	if len(threads.replyMentions) != 3 || len(threads.replyMentions[2]) != 0 {
		t.Fatalf("expected null mentions to record an empty mention batch, got %+v", threads.replyMentions)
	}

	duplicateReq := httptest.NewRequest(http.MethodPost, "/api/v1/threads/thread-1/replies", bytes.NewBufferString(`{"body":"Third reply","mentions":["user-2"," user-2 ","user-3"]}`))
	duplicateReq.Header.Set("Authorization", "Bearer "+viewerToken)
	duplicateReq.Header.Set("Content-Type", "application/json")
	duplicateRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(duplicateRec, duplicateReq)
	if duplicateRec.Code != http.StatusCreated {
		t.Fatalf("expected duplicate mentions status 201, got %d body=%s", duplicateRec.Code, duplicateRec.Body.String())
	}
	if len(threads.replyMentions) != 4 || len(threads.replyMentions[3]) != 2 || threads.replyMentions[3][0].MentionedUserID != "user-2" || threads.replyMentions[3][1].MentionedUserID != "user-3" {
		t.Fatalf("expected duplicate mentions to normalize internally, got %+v", threads.replyMentions)
	}

	emptyReq := httptest.NewRequest(http.MethodPost, "/api/v1/threads/thread-1/replies", bytes.NewBufferString(`{"body":"   "}`))
	emptyReq.Header.Set("Authorization", "Bearer "+viewerToken)
	emptyReq.Header.Set("Content-Type", "application/json")
	emptyRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(emptyRec, emptyReq)
	if emptyRec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected empty reply status 422, got %d body=%s", emptyRec.Code, emptyRec.Body.String())
	}

	nonArrayReq := httptest.NewRequest(http.MethodPost, "/api/v1/threads/thread-1/replies", bytes.NewBufferString(`{"body":"reply","mentions":{"user-2":true}}`))
	nonArrayReq.Header.Set("Authorization", "Bearer "+viewerToken)
	nonArrayReq.Header.Set("Content-Type", "application/json")
	nonArrayRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(nonArrayRec, nonArrayReq)
	if nonArrayRec.Code != http.StatusBadRequest {
		t.Fatalf("expected non-array mentions status 400, got %d body=%s", nonArrayRec.Code, nonArrayRec.Body.String())
	}

	blankMentionReq := httptest.NewRequest(http.MethodPost, "/api/v1/threads/thread-1/replies", bytes.NewBufferString(`{"body":"reply","mentions":[" "]}`))
	blankMentionReq.Header.Set("Authorization", "Bearer "+viewerToken)
	blankMentionReq.Header.Set("Content-Type", "application/json")
	blankMentionRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(blankMentionRec, blankMentionReq)
	if blankMentionRec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected blank mention status 422, got %d body=%s", blankMentionRec.Code, blankMentionRec.Body.String())
	}

	nonMemberMentionReq := httptest.NewRequest(http.MethodPost, "/api/v1/threads/thread-1/replies", bytes.NewBufferString(`{"body":"reply","mentions":["user-x"]}`))
	nonMemberMentionReq.Header.Set("Authorization", "Bearer "+viewerToken)
	nonMemberMentionReq.Header.Set("Content-Type", "application/json")
	nonMemberMentionRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(nonMemberMentionRec, nonMemberMentionReq)
	if nonMemberMentionRec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected non-member mention status 422, got %d body=%s", nonMemberMentionRec.Code, nonMemberMentionRec.Body.String())
	}

	tooManyMentions := make([]string, 0, 21)
	for i := 0; i < 21; i++ {
		tooManyMentions = append(tooManyMentions, fmt.Sprintf("user-%d", i+10))
	}
	tooManyBody, err := json.Marshal(map[string]any{"body": "reply", "mentions": tooManyMentions})
	if err != nil {
		t.Fatalf("marshal too many mentions body: %v", err)
	}
	tooManyReq := httptest.NewRequest(http.MethodPost, "/api/v1/threads/thread-1/replies", bytes.NewReader(tooManyBody))
	tooManyReq.Header.Set("Authorization", "Bearer "+viewerToken)
	tooManyReq.Header.Set("Content-Type", "application/json")
	tooManyRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(tooManyRec, tooManyReq)
	if tooManyRec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected too many mentions status 422, got %d body=%s", tooManyRec.Code, tooManyRec.Body.String())
	}

	unknownFieldReq := httptest.NewRequest(http.MethodPost, "/api/v1/threads/thread-1/replies", bytes.NewBufferString(`{"body":"reply","mentions":["user-2"],"extra":1}`))
	unknownFieldReq.Header.Set("Authorization", "Bearer "+viewerToken)
	unknownFieldReq.Header.Set("Content-Type", "application/json")
	unknownFieldRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(unknownFieldRec, unknownFieldReq)
	if unknownFieldRec.Code != http.StatusBadRequest {
		t.Fatalf("expected unknown field status 400, got %d body=%s", unknownFieldRec.Code, unknownFieldRec.Body.String())
	}

	malformedReq := httptest.NewRequest(http.MethodPost, "/api/v1/threads/thread-1/replies", bytes.NewBufferString(`{"body":"reply"`))
	malformedReq.Header.Set("Authorization", "Bearer "+viewerToken)
	malformedReq.Header.Set("Content-Type", "application/json")
	malformedRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(malformedRec, malformedReq)
	if malformedRec.Code != http.StatusBadRequest {
		t.Fatalf("expected malformed reply status 400, got %d body=%s", malformedRec.Code, malformedRec.Body.String())
	}

	nonMemberToken, _, err := tokenManager.GenerateAccessToken("user-x", "outsider@example.com", time.Now().UTC())
	if err != nil {
		t.Fatalf("GenerateAccessToken() non-member error = %v", err)
	}
	nonMemberReq := httptest.NewRequest(http.MethodPost, "/api/v1/threads/thread-1/replies", bytes.NewBufferString(`{"body":"reply"}`))
	nonMemberReq.Header.Set("Authorization", "Bearer "+nonMemberToken)
	nonMemberReq.Header.Set("Content-Type", "application/json")
	nonMemberRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(nonMemberRec, nonMemberReq)
	if nonMemberRec.Code != http.StatusNotFound {
		t.Fatalf("expected non-member reply status 404, got %d body=%s", nonMemberRec.Code, nonMemberRec.Body.String())
	}
}

func TestThreadEndpointsPropagateTransactionalFailures(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	tokenManager := appauth.NewTokenManager("super-secret-token", "note-app", 15*time.Minute)
	memberships := &testMembershipRepo{
		memberships: map[string][]domain.WorkspaceMember{
			"workspace-1": {
				{ID: "member-1", WorkspaceID: "workspace-1", UserID: "user-1", Role: domain.RoleViewer},
			},
		},
	}
	pages := &testPageRepo{
		pages: map[string]domain.Page{
			"page-1": {ID: "page-1", WorkspaceID: "workspace-1", Title: "Doc"},
		},
		drafts: map[string]domain.PageDraft{
			"page-1": {PageID: "page-1", Content: json.RawMessage(`[{"id":"block-1","type":"paragraph","children":[{"type":"text","text":"hello world"}]}]`)},
		},
	}
	blockID := "block-1"
	threads := &testThreadRepo{details: map[string]domain.PageCommentThreadDetail{
		"thread-1": {
			Thread: domain.PageCommentThread{
				ID:          "thread-1",
				PageID:      "page-1",
				Anchor:      domain.PageCommentThreadAnchor{Type: domain.PageCommentThreadAnchorTypeBlock, BlockID: &blockID, QuotedBlockText: "hello world"},
				ThreadState: domain.PageCommentThreadStateOpen,
				AnchorState: domain.PageCommentThreadAnchorStateActive,
				CreatedBy:   "user-1",
				ReplyCount:  1,
			},
			Messages: []domain.PageCommentThreadMessage{
				{ID: "message-1", ThreadID: "thread-1", Body: "Initial comment", CreatedBy: "user-1"},
			},
		},
	}}
	threadService := application.NewThreadService(threads, pages, memberships)
	server := NewServer(logger, application.AuthService{}, application.WorkspaceService{}, application.FolderService{}, application.PageService{}, application.RevisionService{}, tokenManager, storage.NewLocal(t.TempDir())).WithThreadService(threadService)

	viewerToken, _, err := tokenManager.GenerateAccessToken("user-1", "viewer@example.com", time.Now().UTC())
	if err != nil {
		t.Fatalf("GenerateAccessToken() error = %v", err)
	}

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/pages/page-1/threads", bytes.NewBufferString(`{"body":"hello","anchor":{"type":"block","block_id":"block-1"}}`))
	createReq.Header.Set("Authorization", "Bearer "+viewerToken)
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected create thread to return 201, got %d body=%s", createRec.Code, createRec.Body.String())
	}

	threads.addReplyErr = errors.New("reply tx failed")
	replyReq := httptest.NewRequest(http.MethodPost, "/api/v1/threads/thread-1/replies", bytes.NewBufferString(`{"body":"reply"}`))
	replyReq.Header.Set("Authorization", "Bearer "+viewerToken)
	replyReq.Header.Set("Content-Type", "application/json")
	replyRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(replyRec, replyReq)
	if replyRec.Code != http.StatusInternalServerError {
		t.Fatalf("expected transactional reply failure status 500, got %d body=%s", replyRec.Code, replyRec.Body.String())
	}
}

func TestThreadResolveAndReopenEndpoints(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	tokenManager := appauth.NewTokenManager("super-secret-token", "note-app", 15*time.Minute)
	memberships := &testMembershipRepo{
		memberships: map[string][]domain.WorkspaceMember{
			"workspace-1": {
				{ID: "member-owner", WorkspaceID: "workspace-1", UserID: "owner-1", Role: domain.RoleOwner},
				{ID: "member-viewer", WorkspaceID: "workspace-1", UserID: "viewer-1", Role: domain.RoleViewer},
			},
		},
	}
	pages := &testPageRepo{
		pages: map[string]domain.Page{
			"page-1": {ID: "page-1", WorkspaceID: "workspace-1", Title: "Doc"},
		},
		drafts: map[string]domain.PageDraft{
			"page-1": {PageID: "page-1", Content: json.RawMessage(`[]`)},
		},
	}
	blockID := "block-1"
	threads := &testThreadRepo{details: map[string]domain.PageCommentThreadDetail{
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
	threadService := application.NewThreadService(threads, pages, memberships)
	server := NewServer(logger, application.AuthService{}, application.WorkspaceService{}, application.FolderService{}, application.PageService{}, application.RevisionService{}, tokenManager, storage.NewLocal(t.TempDir())).WithThreadService(threadService)

	ownerToken, _, err := tokenManager.GenerateAccessToken("owner-1", "owner@example.com", time.Now().UTC())
	if err != nil {
		t.Fatalf("GenerateAccessToken() error = %v", err)
	}
	viewerToken, _, err := tokenManager.GenerateAccessToken("viewer-1", "viewer@example.com", time.Now().UTC())
	if err != nil {
		t.Fatalf("GenerateAccessToken() error = %v", err)
	}

	resolveReq := httptest.NewRequest(http.MethodPost, "/api/v1/threads/thread-1/resolve", bytes.NewBufferString(`{"resolve_note":"  Fixed in latest revision  "}`))
	resolveReq.Header.Set("Authorization", "Bearer "+ownerToken)
	resolveRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(resolveRec, resolveReq)
	if resolveRec.Code != http.StatusOK {
		t.Fatalf("expected resolve thread status 200, got %d body=%s", resolveRec.Code, resolveRec.Body.String())
	}

	var resolvedPayload struct {
		Data domain.PageCommentThreadDetail `json:"data"`
	}
	if err := json.Unmarshal(resolveRec.Body.Bytes(), &resolvedPayload); err != nil {
		t.Fatalf("unmarshal resolve thread response: %v", err)
	}
	if resolvedPayload.Data.Thread.ThreadState != domain.PageCommentThreadStateResolved || resolvedPayload.Data.Thread.ResolvedBy == nil || *resolvedPayload.Data.Thread.ResolvedBy != "owner-1" {
		t.Fatalf("unexpected resolved thread payload: %+v", resolvedPayload.Data.Thread)
	}
	if resolvedPayload.Data.Thread.ResolveNote == nil || *resolvedPayload.Data.Thread.ResolveNote != "Fixed in latest revision" {
		t.Fatalf("expected resolve note in payload, got %+v", resolvedPayload.Data.Thread)
	}

	viewerResolveReq := httptest.NewRequest(http.MethodPost, "/api/v1/threads/thread-1/resolve", nil)
	viewerResolveReq.Header.Set("Authorization", "Bearer "+viewerToken)
	viewerResolveRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(viewerResolveRec, viewerResolveReq)
	if viewerResolveRec.Code != http.StatusForbidden {
		t.Fatalf("expected viewer resolve status 403, got %d body=%s", viewerResolveRec.Code, viewerResolveRec.Body.String())
	}

	reopenReq := httptest.NewRequest(http.MethodPost, "/api/v1/threads/thread-1/reopen", bytes.NewBufferString(`{"reopen_reason":"  Follow-up requested  "}`))
	reopenReq.Header.Set("Authorization", "Bearer "+viewerToken)
	reopenRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(reopenRec, reopenReq)
	if reopenRec.Code != http.StatusOK {
		t.Fatalf("expected reopen thread status 200, got %d body=%s", reopenRec.Code, reopenRec.Body.String())
	}

	var reopenedPayload struct {
		Data domain.PageCommentThreadDetail `json:"data"`
	}
	if err := json.Unmarshal(reopenRec.Body.Bytes(), &reopenedPayload); err != nil {
		t.Fatalf("unmarshal reopen thread response: %v", err)
	}
	if reopenedPayload.Data.Thread.ThreadState != domain.PageCommentThreadStateOpen || reopenedPayload.Data.Thread.ReopenedBy == nil || *reopenedPayload.Data.Thread.ReopenedBy != "viewer-1" {
		t.Fatalf("unexpected reopened thread payload: %+v", reopenedPayload.Data.Thread)
	}
	if reopenedPayload.Data.Thread.ResolvedBy != nil || reopenedPayload.Data.Thread.ResolvedAt != nil {
		t.Fatalf("expected resolved markers cleared, got %+v", reopenedPayload.Data.Thread)
	}
	if reopenedPayload.Data.Thread.ResolveNote != nil {
		t.Fatalf("expected resolve note cleared after reopen, got %+v", reopenedPayload.Data.Thread)
	}
	if reopenedPayload.Data.Thread.ReopenReason == nil || *reopenedPayload.Data.Thread.ReopenReason != "Follow-up requested" {
		t.Fatalf("expected reopen reason in payload, got %+v", reopenedPayload.Data.Thread)
	}

	resolveAgainReq := httptest.NewRequest(http.MethodPost, "/api/v1/threads/thread-1/resolve", nil)
	resolveAgainReq.Header.Set("Authorization", "Bearer "+ownerToken)
	resolveAgainRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(resolveAgainRec, resolveAgainReq)
	if resolveAgainRec.Code != http.StatusOK {
		t.Fatalf("expected idempotent resolve status 200, got %d body=%s", resolveAgainRec.Code, resolveAgainRec.Body.String())
	}

	reopenAgainReq := httptest.NewRequest(http.MethodPost, "/api/v1/threads/thread-1/reopen", nil)
	reopenAgainReq.Header.Set("Authorization", "Bearer "+viewerToken)
	reopenAgainRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(reopenAgainRec, reopenAgainReq)
	if reopenAgainRec.Code != http.StatusOK {
		t.Fatalf("expected idempotent reopen status 200, got %d body=%s", reopenAgainRec.Code, reopenAgainRec.Body.String())
	}

	nonMemberToken, _, err := tokenManager.GenerateAccessToken("viewer-x", "outsider@example.com", time.Now().UTC())
	if err != nil {
		t.Fatalf("GenerateAccessToken() non-member error = %v", err)
	}
	nonMemberResolveReq := httptest.NewRequest(http.MethodPost, "/api/v1/threads/thread-1/resolve", nil)
	nonMemberResolveReq.Header.Set("Authorization", "Bearer "+nonMemberToken)
	nonMemberResolveRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(nonMemberResolveRec, nonMemberResolveReq)
	if nonMemberResolveRec.Code != http.StatusNotFound {
		t.Fatalf("expected non-member resolve status 404, got %d body=%s", nonMemberResolveRec.Code, nonMemberResolveRec.Body.String())
	}

	nonMemberReopenReq := httptest.NewRequest(http.MethodPost, "/api/v1/threads/thread-1/reopen", nil)
	nonMemberReopenReq.Header.Set("Authorization", "Bearer "+nonMemberToken)
	nonMemberReopenRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(nonMemberReopenRec, nonMemberReopenReq)
	if nonMemberReopenRec.Code != http.StatusNotFound {
		t.Fatalf("expected non-member reopen status 404, got %d body=%s", nonMemberReopenRec.Code, nonMemberReopenRec.Body.String())
	}

	invalidResolveReq := httptest.NewRequest(http.MethodPost, "/api/v1/threads/thread-1/resolve", bytes.NewBufferString(`{"resolve_note":`))
	invalidResolveReq.Header.Set("Authorization", "Bearer "+ownerToken)
	invalidResolveRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(invalidResolveRec, invalidResolveReq)
	if invalidResolveRec.Code != http.StatusBadRequest {
		t.Fatalf("expected invalid resolve thread status 400, got %d body=%s", invalidResolveRec.Code, invalidResolveRec.Body.String())
	}

	invalidReopenReq := httptest.NewRequest(http.MethodPost, "/api/v1/threads/thread-1/reopen", bytes.NewBufferString(`{"reopen_reason":`))
	invalidReopenReq.Header.Set("Authorization", "Bearer "+viewerToken)
	invalidReopenRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(invalidReopenRec, invalidReopenReq)
	if invalidReopenRec.Code != http.StatusBadRequest {
		t.Fatalf("expected invalid reopen thread status 400, got %d body=%s", invalidReopenRec.Code, invalidReopenRec.Body.String())
	}
}

func TestSearchEndpoint(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	tokenManager := appauth.NewTokenManager("super-secret-token", "note-app", 15*time.Minute)
	memberships := &testMembershipRepo{
		memberships: map[string][]domain.WorkspaceMember{
			"workspace-1": {
				{ID: "member-1", WorkspaceID: "workspace-1", UserID: "user-1", Role: domain.RoleViewer},
			},
		},
	}
	searches := &testSearchRepo{resultsByQuery: map[string][]domain.PageSearchResult{
		"workspace-1:architecture": {
			{ID: "page-title", WorkspaceID: "workspace-1", Title: "Architecture Spec", UpdatedAt: time.Date(2026, 3, 7, 13, 0, 0, 0, time.UTC)},
		},
		"workspace-1:postgres": {
			{ID: "page-body", WorkspaceID: "workspace-1", Title: "Storage Notes", UpdatedAt: time.Date(2026, 3, 7, 14, 0, 0, 0, time.UTC)},
		},
	}}
	searchService := application.NewSearchService(searches, memberships)
	server := NewServer(logger, application.AuthService{}, application.WorkspaceService{}, application.FolderService{}, application.PageService{}, application.RevisionService{}, tokenManager, storage.NewLocal(t.TempDir())).WithSearchService(searchService)

	viewerToken, _, err := tokenManager.GenerateAccessToken("user-1", "viewer@example.com", time.Now().UTC())
	if err != nil {
		t.Fatalf("GenerateAccessToken() error = %v", err)
	}

	titleReq := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces/workspace-1/search?q=architecture", nil)
	titleReq.Header.Set("Authorization", "Bearer "+viewerToken)
	titleRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(titleRec, titleReq)
	if titleRec.Code != http.StatusOK {
		t.Fatalf("expected title search status 200, got %d body=%s", titleRec.Code, titleRec.Body.String())
	}

	var titlePayload struct {
		Data []domain.PageSearchResult `json:"data"`
	}
	if err := json.Unmarshal(titleRec.Body.Bytes(), &titlePayload); err != nil {
		t.Fatalf("unmarshal title search response: %v", err)
	}
	if len(titlePayload.Data) != 1 || titlePayload.Data[0].ID != "page-title" {
		t.Fatalf("unexpected title search results: %+v", titlePayload.Data)
	}

	bodyReq := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces/workspace-1/search?q=postgres", nil)
	bodyReq.Header.Set("Authorization", "Bearer "+viewerToken)
	bodyRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(bodyRec, bodyReq)
	if bodyRec.Code != http.StatusOK {
		t.Fatalf("expected body search status 200, got %d body=%s", bodyRec.Code, bodyRec.Body.String())
	}

	var bodyPayload struct {
		Data []domain.PageSearchResult `json:"data"`
	}
	if err := json.Unmarshal(bodyRec.Body.Bytes(), &bodyPayload); err != nil {
		t.Fatalf("unmarshal body search response: %v", err)
	}
	if len(bodyPayload.Data) != 1 || bodyPayload.Data[0].ID != "page-body" {
		t.Fatalf("unexpected body search results: %+v", bodyPayload.Data)
	}

	missingQueryReq := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces/workspace-1/search?q=%20%20%20", nil)
	missingQueryReq.Header.Set("Authorization", "Bearer "+viewerToken)
	missingQueryRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(missingQueryRec, missingQueryReq)
	if missingQueryRec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected missing query status 422, got %d body=%s", missingQueryRec.Code, missingQueryRec.Body.String())
	}
}
func TestTrashEndpoints(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	tokenManager := appauth.NewTokenManager("super-secret-token", "note-app", 15*time.Minute)
	memberships := &testMembershipRepo{memberships: map[string][]domain.WorkspaceMember{
		"workspace-1": {
			{ID: "member-1", WorkspaceID: "workspace-1", UserID: "editor-1", Role: domain.RoleEditor},
			{ID: "member-2", WorkspaceID: "workspace-1", UserID: "viewer-1", Role: domain.RoleViewer},
		},
	}}
	folders := &testFolderRepo{byID: map[string]domain.Folder{}, byWorkspace: map[string][]domain.Folder{}}
	pages := &testPageRepo{
		pages: map[string]domain.Page{
			"page-1": {ID: "page-1", WorkspaceID: "workspace-1", Title: "Doc to Trash", CreatedBy: "editor-1", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()},
		},
		drafts: map[string]domain.PageDraft{
			"page-1": {PageID: "page-1", Content: json.RawMessage(`[]`)},
		},
		trash: map[string]domain.TrashItem{},
	}
	pageService := application.NewPageService(pages, memberships, folders)
	server := NewServer(logger, application.AuthService{}, application.WorkspaceService{}, application.FolderService{}, pageService, application.RevisionService{}, tokenManager, storage.NewLocal(t.TempDir()))

	editorToken, _, err := tokenManager.GenerateAccessToken("editor-1", "editor@example.com", time.Now().UTC())
	if err != nil {
		t.Fatalf("GenerateAccessToken() error = %v", err)
	}
	viewerToken, _, err := tokenManager.GenerateAccessToken("viewer-1", "viewer@example.com", time.Now().UTC())
	if err != nil {
		t.Fatalf("GenerateAccessToken() error = %v", err)
	}

	viewerDeleteReq := httptest.NewRequest(http.MethodDelete, "/api/v1/pages/page-1", nil)
	viewerDeleteReq.Header.Set("Authorization", "Bearer "+viewerToken)
	viewerDeleteRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(viewerDeleteRec, viewerDeleteReq)
	if viewerDeleteRec.Code != http.StatusForbidden {
		t.Fatalf("expected viewer delete status 403, got %d body=%s", viewerDeleteRec.Code, viewerDeleteRec.Body.String())
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/api/v1/pages/page-1", nil)
	deleteReq.Header.Set("Authorization", "Bearer "+editorToken)
	deleteRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(deleteRec, deleteReq)
	if deleteRec.Code != http.StatusNoContent {
		t.Fatalf("expected delete status 204, got %d body=%s", deleteRec.Code, deleteRec.Body.String())
	}

	trashListReq := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces/workspace-1/trash", nil)
	trashListReq.Header.Set("Authorization", "Bearer "+viewerToken)
	trashListRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(trashListRec, trashListReq)
	if trashListRec.Code != http.StatusOK {
		t.Fatalf("expected trash list status 200, got %d body=%s", trashListRec.Code, trashListRec.Body.String())
	}

	var trashPayload struct {
		Data []domain.TrashItem `json:"data"`
	}
	if err := json.Unmarshal(trashListRec.Body.Bytes(), &trashPayload); err != nil {
		t.Fatalf("unmarshal trash list response: %v", err)
	}
	if len(trashPayload.Data) != 1 {
		t.Fatalf("expected one trash item, got %d", len(trashPayload.Data))
	}

	previewReq := httptest.NewRequest(http.MethodGet, "/api/v1/trash/"+trashPayload.Data[0].ID, nil)
	previewReq.Header.Set("Authorization", "Bearer "+viewerToken)
	previewRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(previewRec, previewReq)
	if previewRec.Code != http.StatusOK {
		t.Fatalf("expected trash preview status 200, got %d body=%s", previewRec.Code, previewRec.Body.String())
	}

	var previewPayload struct {
		Data struct {
			TrashItem domain.TrashItem `json:"trash_item"`
			Page      domain.Page      `json:"page"`
			Draft     domain.PageDraft `json:"draft"`
		} `json:"data"`
	}
	if err := json.Unmarshal(previewRec.Body.Bytes(), &previewPayload); err != nil {
		t.Fatalf("unmarshal trash preview response: %v", err)
	}
	if previewPayload.Data.TrashItem.ID != trashPayload.Data[0].ID || previewPayload.Data.Page.ID != "page-1" || string(previewPayload.Data.Draft.Content) != "[]" {
		t.Fatalf("unexpected trash preview payload: %+v", previewPayload.Data)
	}

	restoreReq := httptest.NewRequest(http.MethodPost, "/api/v1/trash/"+trashPayload.Data[0].ID+"/restore", nil)
	restoreReq.Header.Set("Authorization", "Bearer "+editorToken)
	restoreRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(restoreRec, restoreReq)
	if restoreRec.Code != http.StatusOK {
		t.Fatalf("expected restore status 200, got %d body=%s", restoreRec.Code, restoreRec.Body.String())
	}
}

type testUserRepo struct {
	byEmail map[string]domain.User
	byID    map[string]domain.User
}

func (r *testUserRepo) Create(_ context.Context, user domain.User) (domain.User, error) {
	if r.byEmail == nil {
		r.byEmail = map[string]domain.User{}
	}
	if r.byID == nil {
		r.byID = map[string]domain.User{}
	}
	r.byEmail[user.Email] = user
	r.byID[user.ID] = user
	return user, nil
}

func (r *testUserRepo) GetByEmail(_ context.Context, email string) (domain.User, error) {
	if user, ok := r.byEmail[email]; ok {
		return user, nil
	}
	return domain.User{}, domain.ErrNotFound
}

func (r *testUserRepo) GetByID(_ context.Context, userID string) (domain.User, error) {
	if user, ok := r.byID[userID]; ok {
		return user, nil
	}
	return domain.User{}, domain.ErrNotFound
}

type testNotificationRepo struct {
	notifications map[string]domain.Notification
	ordered       []domain.Notification
}

func (r *testNotificationRepo) GetUnreadCount(_ context.Context, userID string) (int64, error) {
	unreadCount := int64(0)
	for _, notification := range r.notifications {
		if notification.UserID == userID && !notification.IsRead {
			unreadCount++
		}
	}
	return unreadCount, nil
}

func (r *testNotificationRepo) Create(_ context.Context, notification domain.Notification) (domain.Notification, error) {
	if r.notifications == nil {
		r.notifications = map[string]domain.Notification{}
	}
	for _, existing := range r.ordered {
		if existing.UserID == notification.UserID && existing.Type == notification.Type && existing.EventID == notification.EventID {
			return domain.Notification{}, domain.ErrConflict
		}
	}
	r.notifications[notification.ID] = notification
	r.ordered = append(r.ordered, notification)
	return notification, nil
}

func (r *testNotificationRepo) CreateMany(_ context.Context, notifications []domain.Notification) error {
	for _, notification := range notifications {
		if _, err := r.Create(context.Background(), notification); err != nil && !errors.Is(err, domain.ErrConflict) {
			return err
		}
	}
	return nil
}

func (r *testNotificationRepo) BatchMarkRead(_ context.Context, userID string, notificationIDs []string, readAt time.Time) (domain.NotificationBatchReadResult, error) {
	if len(notificationIDs) == 0 {
		return domain.NotificationBatchReadResult{}, domain.ErrValidation
	}

	updatedCount := int64(0)
	for _, notificationID := range notificationIDs {
		notification, ok := r.notifications[notificationID]
		if !ok || notification.UserID != userID {
			return domain.NotificationBatchReadResult{}, domain.ErrNotFound
		}
		if notification.ReadAt == nil {
			notification.ReadAt = &readAt
			notification.IsRead = true
			notification.UpdatedAt = readAt
			r.notifications[notificationID] = notification
			updatedCount++
			for idx := range r.ordered {
				if r.ordered[idx].ID == notificationID {
					r.ordered[idx] = notification
					break
				}
			}
		}
	}

	unreadCount := int64(0)
	for _, notification := range r.notifications {
		if notification.UserID == userID && !notification.IsRead {
			unreadCount++
		}
	}

	return domain.NotificationBatchReadResult{UpdatedCount: updatedCount, UnreadCount: unreadCount}, nil
}

func (r *testNotificationRepo) ListInbox(_ context.Context, userID string, filter domain.NotificationInboxFilter) (domain.NotificationInboxPage, error) {
	if filter.Cursor == "broken" {
		return domain.NotificationInboxPage{}, domain.ErrValidation
	}
	result := make([]domain.NotificationInboxItem, 0)
	for idx := len(r.ordered) - 1; idx >= 0; idx-- {
		notification := r.ordered[idx]
		if notification.UserID != userID {
			continue
		}
		if filter.Status == domain.NotificationInboxStatusRead && !notification.IsRead {
			continue
		}
		if filter.Status == domain.NotificationInboxStatusUnread && notification.IsRead {
			continue
		}
		if filter.Type != domain.NotificationInboxTypeAll && string(notification.Type) != string(filter.Type) {
			continue
		}
		result = append(result, domain.NotificationInboxItem{
			ID:           notification.ID,
			WorkspaceID:  notification.WorkspaceID,
			Type:         notification.Type,
			ActorID:      notification.ActorID,
			Title:        notification.Title,
			Content:      notification.Content,
			IsRead:       notification.IsRead,
			ReadAt:       notification.ReadAt,
			Actionable:   notification.Actionable,
			ActionKind:   notification.ActionKind,
			ResourceType: notification.ResourceType,
			ResourceID:   notification.ResourceID,
			Payload:      notification.Payload,
			CreatedAt:    notification.CreatedAt,
			UpdatedAt:    notification.UpdatedAt,
		})
		if filter.Limit > 0 && len(result) >= filter.Limit {
			break
		}
	}
	unreadCount := int64(0)
	for _, notification := range r.notifications {
		if notification.UserID == userID && !notification.IsRead {
			unreadCount++
		}
	}
	return domain.NotificationInboxPage{Items: result, UnreadCount: unreadCount, HasMore: false}, nil
}

func (r *testNotificationRepo) MarkRead(_ context.Context, notificationID, userID string, readAt time.Time) (domain.NotificationInboxItem, error) {
	notification, ok := r.notifications[notificationID]
	if !ok || notification.UserID != userID {
		return domain.NotificationInboxItem{}, domain.ErrNotFound
	}
	if notification.ReadAt == nil {
		notification.ReadAt = &readAt
		notification.IsRead = true
		notification.UpdatedAt = readAt
		r.notifications[notificationID] = notification
		for idx := range r.ordered {
			if r.ordered[idx].ID == notificationID {
				r.ordered[idx] = notification
				break
			}
		}
	}
	return domain.NotificationInboxItem{
		ID:           notification.ID,
		WorkspaceID:  notification.WorkspaceID,
		Type:         notification.Type,
		ActorID:      notification.ActorID,
		Title:        notification.Title,
		Content:      notification.Content,
		IsRead:       notification.IsRead,
		ReadAt:       notification.ReadAt,
		Actionable:   notification.Actionable,
		ActionKind:   notification.ActionKind,
		ResourceType: notification.ResourceType,
		ResourceID:   notification.ResourceID,
		Payload:      notification.Payload,
		CreatedAt:    notification.CreatedAt,
		UpdatedAt:    notification.UpdatedAt,
	}, nil
}

func TestNotificationEndpoints(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	tokenManager := appauth.NewTokenManager("super-secret-token", "note-app", 15*time.Minute)
	memberships := &testMembershipRepo{memberships: map[string][]domain.WorkspaceMember{
		"workspace-1": {
			{ID: "member-1", WorkspaceID: "workspace-1", UserID: "user-1", Role: domain.RoleEditor},
			{ID: "member-2", WorkspaceID: "workspace-1", UserID: "user-2", Role: domain.RoleViewer},
		},
	}}
	users := &testUserRepo{byEmail: map[string]domain.User{
		"user1@example.com": {ID: "user-1", Email: "user1@example.com", FullName: "User One"},
		"user2@example.com": {ID: "user-2", Email: "user2@example.com", FullName: "User Two"},
		"owner@example.com": {ID: "owner-1", Email: "owner@example.com", FullName: "Owner"},
	}, byID: map[string]domain.User{
		"user-1":  {ID: "user-1", Email: "user1@example.com", FullName: "User One"},
		"user-2":  {ID: "user-2", Email: "user2@example.com", FullName: "User Two"},
		"owner-1": {ID: "owner-1", Email: "owner@example.com", FullName: "Owner"},
	}}
	notifications := &testNotificationRepo{notifications: map[string]domain.Notification{}, ordered: []domain.Notification{
		{ID: "11111111-1111-1111-1111-111111111111", UserID: "user-1", WorkspaceID: "workspace-1", Type: domain.NotificationTypeComment, EventID: "comment-1", Message: "New comment", CreatedAt: time.Date(2026, 3, 8, 2, 0, 0, 0, time.UTC), UpdatedAt: time.Date(2026, 3, 8, 2, 0, 0, 0, time.UTC), Title: "Comment activity", Content: "New comment", ActorID: stringPtr("owner-1")},
		{ID: "22222222-2222-2222-2222-222222222222", UserID: "user-2", WorkspaceID: "workspace-1", Type: domain.NotificationTypeInvitation, EventID: "inv-1", Message: "Invitation", CreatedAt: time.Date(2026, 3, 8, 3, 0, 0, 0, time.UTC), UpdatedAt: time.Date(2026, 3, 8, 3, 0, 0, 0, time.UTC), Title: "Workspace invitation", Content: "Invitation", ActorID: stringPtr("owner-1")},
	}}
	notifications.notifications["11111111-1111-1111-1111-111111111111"] = notifications.ordered[0]
	notifications.notifications["22222222-2222-2222-2222-222222222222"] = notifications.ordered[1]

	notificationService := application.NewNotificationService(notifications, users, memberships)
	server := NewServer(logger, application.AuthService{}, application.WorkspaceService{}, application.FolderService{}, application.PageService{}, application.RevisionService{}, tokenManager, storage.NewLocal(t.TempDir())).WithNotificationService(notificationService)

	userToken, _, err := tokenManager.GenerateAccessToken("user-1", "user1@example.com", time.Now().UTC())
	if err != nil {
		t.Fatalf("GenerateAccessToken() error = %v", err)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/notifications", nil)
	listReq.Header.Set("Authorization", "Bearer "+userToken)
	listRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected list status 200, got %d body=%s", listRec.Code, listRec.Body.String())
	}

	var listed struct {
		Data domain.NotificationInboxPage `json:"data"`
	}
	if err := json.Unmarshal(listRec.Body.Bytes(), &listed); err != nil {
		t.Fatalf("unmarshal list notifications response: %v", err)
	}
	if len(listed.Data.Items) != 1 || listed.Data.Items[0].ID != "11111111-1111-1111-1111-111111111111" || listed.Data.UnreadCount != 1 || listed.Data.HasMore {
		t.Fatalf("unexpected listed notifications: %+v", listed.Data)
	}

	filteredReq := httptest.NewRequest(http.MethodGet, "/api/v1/notifications?status=unread&type=comment&limit=1", nil)
	filteredReq.Header.Set("Authorization", "Bearer "+userToken)
	filteredRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(filteredRec, filteredReq)
	if filteredRec.Code != http.StatusOK {
		t.Fatalf("expected filtered list status 200, got %d body=%s", filteredRec.Code, filteredRec.Body.String())
	}

	invalidReq := httptest.NewRequest(http.MethodGet, "/api/v1/notifications?status=nope", nil)
	invalidReq.Header.Set("Authorization", "Bearer "+userToken)
	invalidRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(invalidRec, invalidReq)
	if invalidRec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected invalid status 422, got %d body=%s", invalidRec.Code, invalidRec.Body.String())
	}

	invalidTypeReq := httptest.NewRequest(http.MethodGet, "/api/v1/notifications?type=nope", nil)
	invalidTypeReq.Header.Set("Authorization", "Bearer "+userToken)
	invalidTypeRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(invalidTypeRec, invalidTypeReq)
	if invalidTypeRec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected invalid type 422, got %d body=%s", invalidTypeRec.Code, invalidTypeRec.Body.String())
	}

	invalidLimitReq := httptest.NewRequest(http.MethodGet, "/api/v1/notifications?limit=0", nil)
	invalidLimitReq.Header.Set("Authorization", "Bearer "+userToken)
	invalidLimitRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(invalidLimitRec, invalidLimitReq)
	if invalidLimitRec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected invalid limit 422, got %d body=%s", invalidLimitRec.Code, invalidLimitRec.Body.String())
	}

	invalidCursorReq := httptest.NewRequest(http.MethodGet, "/api/v1/notifications?cursor=broken", nil)
	invalidCursorReq.Header.Set("Authorization", "Bearer "+userToken)
	invalidCursorRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(invalidCursorRec, invalidCursorReq)
	if invalidCursorRec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected invalid cursor 422, got %d body=%s", invalidCursorRec.Code, invalidCursorRec.Body.String())
	}

	missingUserToken, _, err := tokenManager.GenerateAccessToken("missing-user", "missing@example.com", time.Now().UTC())
	if err != nil {
		t.Fatalf("GenerateAccessToken() missing user error = %v", err)
	}
	unknownActorReq := httptest.NewRequest(http.MethodGet, "/api/v1/notifications", nil)
	unknownActorReq.Header.Set("Authorization", "Bearer "+missingUserToken)
	unknownActorRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(unknownActorRec, unknownActorReq)
	if unknownActorRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected unknown actor 401, got %d body=%s", unknownActorRec.Code, unknownActorRec.Body.String())
	}

	unreadReq := httptest.NewRequest(http.MethodGet, "/api/v1/notifications/unread-count", nil)
	unreadReq.Header.Set("Authorization", "Bearer "+userToken)
	unreadRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(unreadRec, unreadReq)
	if unreadRec.Code != http.StatusOK {
		t.Fatalf("expected unread-count status 200, got %d body=%s", unreadRec.Code, unreadRec.Body.String())
	}
	var unreadPayload struct {
		Data domain.NotificationUnreadCount `json:"data"`
	}
	if err := json.Unmarshal(unreadRec.Body.Bytes(), &unreadPayload); err != nil {
		t.Fatalf("unmarshal unread-count response: %v", err)
	}
	if unreadPayload.Data.UnreadCount != 1 {
		t.Fatalf("expected unread_count=1, got %+v", unreadPayload.Data)
	}

	emptyToken, _, err := tokenManager.GenerateAccessToken("user-3", "user3@example.com", time.Now().UTC())
	if err != nil {
		t.Fatalf("GenerateAccessToken() empty user error = %v", err)
	}
	users.byID["user-3"] = domain.User{ID: "user-3", Email: "user3@example.com", FullName: "User Three"}
	zeroUnreadReq := httptest.NewRequest(http.MethodGet, "/api/v1/notifications/unread-count", nil)
	zeroUnreadReq.Header.Set("Authorization", "Bearer "+emptyToken)
	zeroUnreadRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(zeroUnreadRec, zeroUnreadReq)
	if zeroUnreadRec.Code != http.StatusOK {
		t.Fatalf("expected unread-count zero status 200, got %d body=%s", zeroUnreadRec.Code, zeroUnreadRec.Body.String())
	}
	if err := json.Unmarshal(zeroUnreadRec.Body.Bytes(), &unreadPayload); err != nil {
		t.Fatalf("unmarshal zero unread-count response: %v", err)
	}
	if unreadPayload.Data.UnreadCount != 0 {
		t.Fatalf("expected unread_count=0 for empty inbox, got %+v", unreadPayload.Data)
	}

	unknownUnreadReq := httptest.NewRequest(http.MethodGet, "/api/v1/notifications/unread-count", nil)
	unknownUnreadReq.Header.Set("Authorization", "Bearer "+missingUserToken)
	unknownUnreadRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(unknownUnreadRec, unknownUnreadReq)
	if unknownUnreadRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected unknown actor unread-count 401, got %d body=%s", unknownUnreadRec.Code, unknownUnreadRec.Body.String())
	}

	readReq := httptest.NewRequest(http.MethodPost, "/api/v1/notifications/11111111-1111-1111-1111-111111111111/read", nil)
	readReq.Header.Set("Authorization", "Bearer "+userToken)
	readRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(readRec, readReq)
	if readRec.Code != http.StatusOK {
		t.Fatalf("expected read status 200, got %d body=%s", readRec.Code, readRec.Body.String())
	}

	var readPayload struct {
		Data domain.NotificationInboxItem `json:"data"`
	}
	if err := json.Unmarshal(readRec.Body.Bytes(), &readPayload); err != nil {
		t.Fatalf("unmarshal mark read response: %v", err)
	}
	if readPayload.Data.ReadAt == nil || !readPayload.Data.IsRead {
		t.Fatalf("expected read_at to be set")
	}

	batchNotification, err := notifications.Create(context.Background(), domain.Notification{
		ID:          "33333333-3333-3333-3333-333333333333",
		UserID:      "user-1",
		WorkspaceID: "workspace-1",
		Type:        domain.NotificationTypeComment,
		EventID:     "comment-2",
		Message:     "Second comment",
		CreatedAt:   time.Date(2026, 3, 8, 4, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("create batch notification: %v", err)
	}

	batchReqBody := strings.NewReader(fmt.Sprintf(`{"notification_ids":["%s","%s"]}`, readPayload.Data.ID, batchNotification.ID))
	batchReq := httptest.NewRequest(http.MethodPost, "/api/v1/notifications/read", batchReqBody)
	batchReq.Header.Set("Authorization", "Bearer "+userToken)
	batchReq.Header.Set("Content-Type", "application/json")
	batchRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(batchRec, batchReq)
	if batchRec.Code != http.StatusOK {
		t.Fatalf("expected batch read status 200, got %d body=%s", batchRec.Code, batchRec.Body.String())
	}
	var batchPayload struct {
		Data domain.NotificationBatchReadResult `json:"data"`
	}
	if err := json.Unmarshal(batchRec.Body.Bytes(), &batchPayload); err != nil {
		t.Fatalf("unmarshal batch read response: %v", err)
	}
	if batchPayload.Data.UpdatedCount != 1 || batchPayload.Data.UnreadCount != 0 {
		t.Fatalf("unexpected batch read payload: %+v", batchPayload.Data)
	}

	repeatBatchReq := httptest.NewRequest(http.MethodPost, "/api/v1/notifications/read", strings.NewReader(fmt.Sprintf(`{"notification_ids":["%s","%s"]}`, readPayload.Data.ID, batchNotification.ID)))
	repeatBatchReq.Header.Set("Authorization", "Bearer "+userToken)
	repeatBatchReq.Header.Set("Content-Type", "application/json")
	repeatBatchRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(repeatBatchRec, repeatBatchReq)
	if repeatBatchRec.Code != http.StatusOK {
		t.Fatalf("expected repeat batch read status 200, got %d body=%s", repeatBatchRec.Code, repeatBatchRec.Body.String())
	}
	if err := json.Unmarshal(repeatBatchRec.Body.Bytes(), &batchPayload); err != nil {
		t.Fatalf("unmarshal repeat batch read response: %v", err)
	}
	if batchPayload.Data.UpdatedCount != 0 || batchPayload.Data.UnreadCount != 0 {
		t.Fatalf("unexpected repeat batch read payload: %+v", batchPayload.Data)
	}

	missingReq := httptest.NewRequest(http.MethodPost, "/api/v1/notifications/22222222-2222-2222-2222-222222222222/read", nil)
	missingReq.Header.Set("Authorization", "Bearer "+userToken)
	missingRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(missingRec, missingReq)
	if missingRec.Code != http.StatusNotFound {
		t.Fatalf("expected other-user notification read status 404, got %d body=%s", missingRec.Code, missingRec.Body.String())
	}

	badIDReq := httptest.NewRequest(http.MethodPost, "/api/v1/notifications/bad-id/read", nil)
	badIDReq.Header.Set("Authorization", "Bearer "+userToken)
	badIDRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(badIDRec, badIDReq)
	if badIDRec.Code != http.StatusNotFound {
		t.Fatalf("expected malformed notification id 404, got %d body=%s", badIDRec.Code, badIDRec.Body.String())
	}

	batchMissingReq := httptest.NewRequest(http.MethodPost, "/api/v1/notifications/read", strings.NewReader(`{"notification_ids":["22222222-2222-2222-2222-222222222222","11111111-1111-1111-1111-111111111111"]}`))
	batchMissingReq.Header.Set("Authorization", "Bearer "+userToken)
	batchMissingReq.Header.Set("Content-Type", "application/json")
	batchMissingRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(batchMissingRec, batchMissingReq)
	if batchMissingRec.Code != http.StatusNotFound {
		t.Fatalf("expected batch other-user notification status 404, got %d body=%s", batchMissingRec.Code, batchMissingRec.Body.String())
	}

	batchUnauthorizedReq := httptest.NewRequest(http.MethodPost, "/api/v1/notifications/read", strings.NewReader(`{"notification_ids":["11111111-1111-1111-1111-111111111111"]}`))
	batchUnauthorizedReq.Header.Set("Content-Type", "application/json")
	batchUnauthorizedRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(batchUnauthorizedRec, batchUnauthorizedReq)
	if batchUnauthorizedRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected missing-auth batch read status 401, got %d body=%s", batchUnauthorizedRec.Code, batchUnauthorizedRec.Body.String())
	}

	batchUnknownActorReq := httptest.NewRequest(http.MethodPost, "/api/v1/notifications/read", strings.NewReader(`{"notification_ids":["11111111-1111-1111-1111-111111111111"]}`))
	batchUnknownActorReq.Header.Set("Authorization", "Bearer "+missingUserToken)
	batchUnknownActorReq.Header.Set("Content-Type", "application/json")
	batchUnknownActorRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(batchUnknownActorRec, batchUnknownActorReq)
	if batchUnknownActorRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected unknown-actor batch read status 401, got %d body=%s", batchUnknownActorRec.Code, batchUnknownActorRec.Body.String())
	}

	for _, tc := range []struct {
		name       string
		body       string
		expectCode int
	}{
		{name: "missing ids", body: `{}`, expectCode: http.StatusUnprocessableEntity},
		{name: "empty ids", body: `{"notification_ids":[]}`, expectCode: http.StatusUnprocessableEntity},
		{name: "invalid uuid", body: `{"notification_ids":["broken"]}`, expectCode: http.StatusUnprocessableEntity},
		{name: "duplicate ids", body: fmt.Sprintf(`{"notification_ids":["%s","%s"]}`, readPayload.Data.ID, readPayload.Data.ID), expectCode: http.StatusUnprocessableEntity},
		{name: "oversized ids", body: func() string {
			ids := make([]string, 101)
			for i := range ids {
				ids[i] = fmt.Sprintf("aaaaaaaa-aaaa-aaaa-aaaa-%012d", i)
			}
			return `{"notification_ids":["` + strings.Join(ids, `","`) + `"]}`
		}(), expectCode: http.StatusUnprocessableEntity},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/notifications/read", strings.NewReader(tc.body))
			req.Header.Set("Authorization", "Bearer "+userToken)
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			server.Handler().ServeHTTP(rec, req)
			if rec.Code != tc.expectCode {
				t.Fatalf("expected batch validation status %d, got %d body=%s", tc.expectCode, rec.Code, rec.Body.String())
			}
		})
	}
}

func TestWorkspaceInvitationStaleVersionMapsToConflict(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	tokenManager := appauth.NewTokenManager("super-secret-token", "note-app", 15*time.Minute)
	userRepo := &httpUserRepo{byID: map[string]domain.User{}, byEmail: map[string]domain.User{}}
	refreshRepo := &httpRefreshTokenRepo{byHash: map[string]domain.RefreshToken{}}
	workspaceRepo := &httpWorkspaceRepo{memberships: map[string][]domain.WorkspaceMember{}, invitations: map[string]domain.WorkspaceInvitation{}, workspaces: map[string]domain.Workspace{}}

	authService := application.NewAuthService(userRepo, refreshRepo, appauth.NewPasswordManager(), tokenManager, 24*time.Hour)
	workspaceService := application.NewWorkspaceService(workspaceRepo, userRepo)
	server := NewServer(logger, authService, workspaceService, application.FolderService{}, application.PageService{}, application.RevisionService{}, tokenManager, storage.NewLocal(t.TempDir()))

	registerAndLogin := func(email string) application.AuthResult {
		registerReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewBufferString(`{"email":"`+email+`","password":"Password1","full_name":"User"}`))
		registerReq.Header.Set("Content-Type", "application/json")
		registerRec := httptest.NewRecorder()
		server.Handler().ServeHTTP(registerRec, registerReq)
		if registerRec.Code != http.StatusCreated {
			t.Fatalf("register failed for %s: %d %s", email, registerRec.Code, registerRec.Body.String())
		}

		loginReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(`{"email":"`+email+`","password":"Password1"}`))
		loginReq.Header.Set("Content-Type", "application/json")
		loginRec := httptest.NewRecorder()
		server.Handler().ServeHTTP(loginRec, loginReq)
		if loginRec.Code != http.StatusOK {
			t.Fatalf("login failed for %s: %d %s", email, loginRec.Code, loginRec.Body.String())
		}

		var payload struct {
			Data application.AuthResult `json:"data"`
		}
		if err := json.Unmarshal(loginRec.Body.Bytes(), &payload); err != nil {
			t.Fatalf("unmarshal login payload for %s: %v", email, err)
		}
		return payload.Data
	}

	ownerAuth := registerAndLogin("conflict-owner@example.com")
	memberAuth := registerAndLogin("conflict-member@example.com")

	createWorkspaceReq := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces", bytes.NewBufferString(`{"name":"Engineering"}`))
	createWorkspaceReq.Header.Set("Authorization", "Bearer "+ownerAuth.Tokens.AccessToken)
	createWorkspaceReq.Header.Set("Content-Type", "application/json")
	createWorkspaceRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(createWorkspaceRec, createWorkspaceReq)
	if createWorkspaceRec.Code != http.StatusCreated {
		t.Fatalf("workspace create failed: %d %s", createWorkspaceRec.Code, createWorkspaceRec.Body.String())
	}

	var workspacePayload struct {
		Data struct {
			Workspace domain.Workspace `json:"workspace"`
		} `json:"data"`
	}
	if err := json.Unmarshal(createWorkspaceRec.Body.Bytes(), &workspacePayload); err != nil {
		t.Fatalf("unmarshal workspace payload: %v", err)
	}

	inviteReq := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces/"+workspacePayload.Data.Workspace.ID+"/invitations", bytes.NewBufferString(`{"email":"conflict-member@example.com","role":"viewer"}`))
	inviteReq.Header.Set("Authorization", "Bearer "+ownerAuth.Tokens.AccessToken)
	inviteReq.Header.Set("Content-Type", "application/json")
	inviteRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(inviteRec, inviteReq)
	if inviteRec.Code != http.StatusCreated {
		t.Fatalf("invite create failed: %d %s", inviteRec.Code, inviteRec.Body.String())
	}

	var invitationPayload struct {
		Data domain.WorkspaceInvitation `json:"data"`
	}
	if err := json.Unmarshal(inviteRec.Body.Bytes(), &invitationPayload); err != nil {
		t.Fatalf("unmarshal invitation payload: %v", err)
	}

	staleReq := httptest.NewRequest(http.MethodPost, "/api/v1/workspace-invitations/"+invitationPayload.Data.ID+"/accept", bytes.NewBufferString(`{"version":2}`))
	staleReq.Header.Set("Authorization", "Bearer "+memberAuth.Tokens.AccessToken)
	staleReq.Header.Set("Content-Type", "application/json")
	staleRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(staleRec, staleReq)
	if staleRec.Code != http.StatusConflict {
		t.Fatalf("expected stale invitation accept 409, got %d body=%s", staleRec.Code, staleRec.Body.String())
	}
}
