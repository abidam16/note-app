package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"note-app/internal/domain"

	"github.com/google/uuid"
)

func TestFolderRepositoryIntegration(t *testing.T) {
	pool := integrationPool(t)
	repo := NewFolderRepository(pool)
	ctx := context.Background()

	owner := seedUser(t, pool, "folder-owner@example.com")
	workspace, _ := seedWorkspaceWithOwner(t, pool, owner)
	now := time.Now().UTC().Truncate(time.Microsecond)

	root := domain.Folder{ID: uuid.NewString(), WorkspaceID: workspace.ID, Name: "Root", CreatedAt: now, UpdatedAt: now}
	createdRoot, err := repo.Create(ctx, root)
	if err != nil {
		t.Fatalf("create root folder: %v", err)
	}

	child := domain.Folder{ID: uuid.NewString(), WorkspaceID: workspace.ID, ParentID: &createdRoot.ID, Name: "Child", CreatedAt: now.Add(time.Second), UpdatedAt: now.Add(time.Second)}
	if _, err := repo.Create(ctx, child); err != nil {
		t.Fatalf("create child folder: %v", err)
	}

	fetched, err := repo.GetByID(ctx, createdRoot.ID)
	if err != nil || fetched.Name != "Root" {
		t.Fatalf("get folder mismatch: err=%v name=%s", err, fetched.Name)
	}

	exists, err := repo.HasSiblingWithName(ctx, workspace.ID, nil, " root ", nil)
	if err != nil || !exists {
		t.Fatalf("expected root sibling lookup to match, err=%v exists=%t", err, exists)
	}
	exists, err = repo.HasSiblingWithName(ctx, workspace.ID, createdRoot.ParentID, " child ", &createdRoot.ID)
	if err != nil {
		t.Fatalf("expected child sibling lookup to succeed, got %v", err)
	}
	if exists {
		t.Fatalf("expected excluding root folder to skip child mismatch")
	}

	renamed, err := repo.UpdateName(ctx, createdRoot.ID, "Platform", now.Add(2*time.Second))
	if err != nil || renamed.Name != "Platform" {
		t.Fatalf("update folder name mismatch: err=%v name=%s", err, renamed.Name)
	}

	list, err := repo.ListByWorkspaceID(ctx, workspace.ID)
	if err != nil || len(list) != 2 {
		t.Fatalf("list folders mismatch: err=%v len=%d", err, len(list))
	}

	if _, err := repo.Create(ctx, domain.Folder{ID: uuid.NewString(), WorkspaceID: workspace.ID, Name: " platform ", CreatedAt: now.Add(3 * time.Second), UpdatedAt: now.Add(3 * time.Second)}); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("expected duplicate root folder validation, got %v", err)
	}
	if _, err := repo.Create(ctx, domain.Folder{ID: uuid.NewString(), WorkspaceID: workspace.ID, ParentID: &createdRoot.ID, Name: " child ", CreatedAt: now.Add(4 * time.Second), UpdatedAt: now.Add(4 * time.Second)}); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("expected duplicate child folder validation, got %v", err)
	}
	otherRoot, err := repo.Create(ctx, domain.Folder{ID: uuid.NewString(), WorkspaceID: workspace.ID, Name: "Other Root", CreatedAt: now.Add(5 * time.Second), UpdatedAt: now.Add(5 * time.Second)})
	if err != nil {
		t.Fatalf("create second root folder: %v", err)
	}
	if _, err := repo.Create(ctx, domain.Folder{ID: uuid.NewString(), WorkspaceID: workspace.ID, ParentID: &otherRoot.ID, Name: "Child", CreatedAt: now.Add(6 * time.Second), UpdatedAt: now.Add(6 * time.Second)}); err != nil {
		t.Fatalf("expected same child name under different parent to succeed, got %v", err)
	}
	if _, err := repo.UpdateName(ctx, otherRoot.ID, "platform", now.Add(7*time.Second)); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("expected duplicate rename validation, got %v", err)
	}

	if _, err := repo.GetByID(ctx, uuid.NewString()); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected folder not found, got %v", err)
	}
	if _, err := repo.UpdateName(ctx, uuid.NewString(), "Missing", now); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected update missing folder not found, got %v", err)
	}
}

func hasThreadEventType(events []domain.PageCommentThreadEvent, eventType domain.PageCommentThreadEventType) bool {
	for _, event := range events {
		if event.Type == eventType {
			return true
		}
	}
	return false
}

func hasThreadEventReason(events []domain.PageCommentThreadEvent, reason domain.PageCommentThreadEventReason) bool {
	for _, event := range events {
		if event.Reason != nil && *event.Reason == reason {
			return true
		}
	}
	return false
}

func hasAnchorRecoveredEvent(events []domain.PageCommentThreadEvent, fromBlockID, toBlockID string, reason domain.PageCommentThreadEventReason) bool {
	for _, event := range events {
		if event.Type != domain.PageCommentThreadEventTypeAnchorRecovered {
			continue
		}
		if event.FromBlockID == nil || *event.FromBlockID != fromBlockID {
			continue
		}
		if event.ToBlockID == nil || *event.ToBlockID != toBlockID {
			continue
		}
		if event.Reason == nil || *event.Reason != reason {
			continue
		}
		return true
	}
	return false
}

func hasEventRevisionID(events []domain.PageCommentThreadEvent, revisionID string) bool {
	for _, event := range events {
		if event.RevisionID != nil && *event.RevisionID == revisionID {
			return true
		}
	}
	return false
}

func TestPageRepositoryIntegration(t *testing.T) {
	pool := integrationPool(t)
	repo := NewPageRepository(pool)
	ctx := context.Background()

	owner := seedUser(t, pool, "page-owner@example.com")
	workspace, _ := seedWorkspaceWithOwner(t, pool, owner)
	now := time.Now().UTC().Truncate(time.Microsecond)

	folder := domain.Folder{ID: uuid.NewString(), WorkspaceID: workspace.ID, Name: "Docs", CreatedAt: now, UpdatedAt: now}
	mustExec(t, pool, `INSERT INTO folders (id, workspace_id, parent_id, name, created_at, updated_at) VALUES ($1,$2,$3,$4,$5,$6)`, folder.ID, folder.WorkspaceID, nil, folder.Name, folder.CreatedAt, folder.UpdatedAt)

	page := domain.Page{ID: uuid.NewString(), WorkspaceID: workspace.ID, FolderID: &folder.ID, Title: "Hello", CreatedBy: owner.ID, CreatedAt: now, UpdatedAt: now}
	draft := domain.PageDraft{PageID: page.ID, Content: json.RawMessage(`[{"type":"paragraph","text":"hello search"}]`), LastEditedBy: owner.ID, CreatedAt: now, UpdatedAt: now}
	if _, _, err := repo.CreateWithDraft(ctx, page, draft); err != nil {
		t.Fatalf("create page with draft: %v", err)
	}
	rootPage := domain.Page{ID: uuid.NewString(), WorkspaceID: workspace.ID, Title: "Workspace Root", CreatedBy: owner.ID, CreatedAt: now.Add(10 * time.Second), UpdatedAt: now.Add(10 * time.Second)}
	rootDraft := domain.PageDraft{PageID: rootPage.ID, Content: json.RawMessage(`[{"type":"paragraph","text":"root"}]`), LastEditedBy: owner.ID, CreatedAt: now.Add(10 * time.Second), UpdatedAt: now.Add(10 * time.Second)}
	if _, _, err := repo.CreateWithDraft(ctx, rootPage, rootDraft); err != nil {
		t.Fatalf("create root page with draft: %v", err)
	}
	secondFolderPage := domain.Page{ID: uuid.NewString(), WorkspaceID: workspace.ID, FolderID: &folder.ID, Title: "Folder Newer", CreatedBy: owner.ID, CreatedAt: now.Add(20 * time.Second), UpdatedAt: now.Add(20 * time.Second)}
	secondFolderDraft := domain.PageDraft{PageID: secondFolderPage.ID, Content: json.RawMessage(`[{"type":"paragraph","text":"nested"}]`), LastEditedBy: owner.ID, CreatedAt: now.Add(20 * time.Second), UpdatedAt: now.Add(20 * time.Second)}
	if _, _, err := repo.CreateWithDraft(ctx, secondFolderPage, secondFolderDraft); err != nil {
		t.Fatalf("create second folder page with draft: %v", err)
	}

	fetchedPage, fetchedDraft, err := repo.GetByID(ctx, page.ID)
	if err != nil || fetchedPage.Title != page.Title || len(fetchedDraft.Content) == 0 {
		t.Fatalf("get page mismatch: err=%v title=%s", err, fetchedPage.Title)
	}
	visiblePage, visibleDraft, err := repo.GetVisibleByUserID(ctx, page.ID, owner.ID)
	if err != nil || visiblePage.ID != page.ID || visibleDraft.PageID != page.ID {
		t.Fatalf("get visible page mismatch: err=%v page=%+v draft=%+v", err, visiblePage, visibleDraft)
	}
	outsider := seedUser(t, pool, "page-outsider@example.com")
	outsiderWorkspace, _ := seedWorkspaceWithOwner(t, pool, outsider)
	_ = outsiderWorkspace
	if _, _, err := repo.GetVisibleByUserID(ctx, page.ID, outsider.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected page not found for outsider visibility lookup, got %v", err)
	}

	rootList, err := repo.ListByWorkspaceIDAndFolderID(ctx, workspace.ID, nil)
	if err != nil || len(rootList) != 1 || rootList[0].ID != rootPage.ID {
		t.Fatalf("list root pages mismatch: err=%v items=%+v", err, rootList)
	}
	folderList, err := repo.ListByWorkspaceIDAndFolderID(ctx, workspace.ID, &folder.ID)
	if err != nil || len(folderList) != 2 || folderList[0].ID != secondFolderPage.ID || folderList[1].ID != page.ID {
		t.Fatalf("list folder pages mismatch: err=%v items=%+v", err, folderList)
	}

	updatedPage, err := repo.UpdateMetadata(ctx, page.ID, "Hello Updated", nil, now.Add(time.Minute))
	if err != nil || updatedPage.Title != "Hello Updated" || updatedPage.FolderID != nil {
		t.Fatalf("update metadata mismatch: err=%v title=%s", err, updatedPage.Title)
	}

	rootList, err = repo.ListByWorkspaceIDAndFolderID(ctx, workspace.ID, nil)
	if err != nil || len(rootList) != 2 || rootList[0].ID != page.ID || rootList[1].ID != rootPage.ID {
		t.Fatalf("list root pages after move mismatch: err=%v items=%+v", err, rootList)
	}

	updatedDraft, err := repo.UpdateDraft(ctx, page.ID, json.RawMessage(`[{"type":"paragraph","text":"searchable token"}]`), owner.ID, now.Add(2*time.Minute))
	if err != nil {
		t.Fatalf("update draft: %v", err)
	}
	if updatedDraft.SearchBody == "" {
		t.Fatal("expected search body to be derived")
	}

	titleResults, err := repo.SearchPages(ctx, workspace.ID, "Updated")
	if err != nil || len(titleResults) != 1 || titleResults[0].ID != page.ID {
		t.Fatalf("title search pages mismatch: err=%v len=%d results=%+v", err, len(titleResults), titleResults)
	}

	results, err := repo.SearchPages(ctx, workspace.ID, "searchable")
	if err != nil || len(results) != 1 || results[0].ID != page.ID {
		t.Fatalf("search pages mismatch: err=%v len=%d", err, len(results))
	}

	trash := domain.TrashItem{ID: uuid.NewString(), WorkspaceID: workspace.ID, PageID: page.ID, PageTitle: updatedPage.Title, DeletedBy: owner.ID, DeletedAt: now.Add(3 * time.Minute)}
	if err := repo.SoftDelete(ctx, trash); err != nil {
		t.Fatalf("soft delete page: %v", err)
	}

	if _, _, err := repo.GetByID(ctx, page.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected deleted page not found, got %v", err)
	}
	rootList, err = repo.ListByWorkspaceIDAndFolderID(ctx, workspace.ID, nil)
	if err != nil || len(rootList) != 1 || rootList[0].ID != rootPage.ID {
		t.Fatalf("expected deleted page excluded from root list, err=%v items=%+v", err, rootList)
	}

	trashList, err := repo.ListTrashByWorkspaceID(ctx, workspace.ID)
	if err != nil || len(trashList) != 1 || trashList[0].ID != trash.ID {
		t.Fatalf("list trash mismatch: err=%v len=%d", err, len(trashList))
	}

	trashItem, err := repo.GetTrashItemByID(ctx, trash.ID)
	if err != nil || trashItem.PageID != page.ID {
		t.Fatalf("get trash item mismatch: err=%v page=%s", err, trashItem.PageID)
	}

	previewTrashItem, previewPage, previewDraft, err := repo.GetTrashedByTrashItemID(ctx, trash.ID)
	if err != nil {
		t.Fatalf("get trashed page mismatch: %v", err)
	}
	if previewTrashItem.ID != trash.ID || previewPage.ID != page.ID || string(previewDraft.Content) == "" {
		t.Fatalf("unexpected trashed page payload: trash=%+v page=%+v draft=%+v", previewTrashItem, previewPage, previewDraft)
	}

	restoredPage, err := repo.RestoreTrashItem(ctx, trash.ID, owner.ID, now.Add(4*time.Minute))
	if err != nil || restoredPage.ID != page.ID {
		t.Fatalf("restore page mismatch: err=%v id=%s", err, restoredPage.ID)
	}

	secondTrash := domain.TrashItem{ID: uuid.NewString(), WorkspaceID: workspace.ID, PageID: page.ID, PageTitle: restoredPage.Title, DeletedBy: owner.ID, DeletedAt: now.Add(5 * time.Minute)}
	if err := repo.SoftDelete(ctx, secondTrash); err != nil {
		t.Fatalf("second soft delete after restore should succeed: %v", err)
	}

	var trashHistoryCount int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM trash_items WHERE page_id = $1`, page.ID).Scan(&trashHistoryCount); err != nil {
		t.Fatalf("count trash history: %v", err)
	}
	if trashHistoryCount != 2 {
		t.Fatalf("expected two trash history rows after delete-restore-delete, got %d", trashHistoryCount)
	}

	activeTrashList, err := repo.ListTrashByWorkspaceID(ctx, workspace.ID)
	if err != nil || len(activeTrashList) != 1 || activeTrashList[0].ID != secondTrash.ID {
		t.Fatalf("active trash list after second delete mismatch: err=%v items=%+v", err, activeTrashList)
	}

	if _, err := repo.GetTrashItemByID(ctx, trash.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected restored trash item hidden, got %v", err)
	}

	if _, err := repo.RestoreTrashItem(ctx, trash.ID, owner.ID, now.Add(6*time.Minute)); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected restore not found on already restored item, got %v", err)
	}
	if err := repo.SoftDelete(ctx, domain.TrashItem{ID: uuid.NewString(), WorkspaceID: workspace.ID, PageID: uuid.NewString(), PageTitle: "x", DeletedBy: owner.ID, DeletedAt: now}); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected soft delete not found, got %v", err)
	}
	if _, err := repo.UpdateMetadata(ctx, uuid.NewString(), "x", nil, now); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected update metadata not found, got %v", err)
	}
	if _, err := repo.UpdateDraft(ctx, uuid.NewString(), json.RawMessage(`[]`), owner.ID, now); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected update draft not found, got %v", err)
	}
	if _, err := repo.GetTrashItemByID(ctx, uuid.NewString()); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected trash item not found, got %v", err)
	}
	if _, _, _, err := repo.GetTrashedByTrashItemID(ctx, uuid.NewString()); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected trashed page not found, got %v", err)
	}
}

func TestRevisionCommentNotificationRepositoriesIntegration(t *testing.T) {
	pool := integrationPool(t)
	ctx := context.Background()

	owner := seedUser(t, pool, "combo-owner@example.com")
	member := seedUser(t, pool, "combo-member@example.com")
	mentioned := seedUser(t, pool, "combo-mentioned@example.com")
	workspace, _ := seedWorkspaceWithOwner(t, pool, owner)
	now := time.Now().UTC().Truncate(time.Microsecond)
	mustExec(t, pool, `
		INSERT INTO workspace_members (id, workspace_id, user_id, role, created_at)
		VALUES ($1, $2, $3, $4, $5)
	`, uuid.NewString(), workspace.ID, member.ID, domain.RoleEditor, now)
	mustExec(t, pool, `
		INSERT INTO workspace_members (id, workspace_id, user_id, role, created_at)
		VALUES ($1, $2, $3, $4, $5)
	`, uuid.NewString(), workspace.ID, mentioned.ID, domain.RoleViewer, now)
	page, _ := seedPageWithDraft(t, pool, workspace.ID, owner.ID, nil, "Doc")

	revRepo := NewRevisionRepository(pool)
	label := "v1"
	note := "note"
	revInput := domain.Revision{ID: uuid.NewString(), PageID: page.ID, Label: &label, Note: &note, Content: json.RawMessage(`[{"type":"paragraph","text":"v1"}]`), CreatedBy: owner.ID, CreatedAt: now}
	rev, err := revRepo.Create(ctx, revInput)
	if err != nil {
		t.Fatalf("create revision: %v", err)
	}
	gotRev, err := revRepo.GetByID(ctx, rev.ID)
	if err != nil || gotRev.ID != rev.ID {
		t.Fatalf("get revision mismatch: err=%v id=%s", err, gotRev.ID)
	}
	revisions, err := revRepo.ListByPageID(ctx, page.ID)
	if err != nil || len(revisions) != 1 {
		t.Fatalf("list revisions mismatch: err=%v len=%d", err, len(revisions))
	}
	if _, err := revRepo.GetByID(ctx, uuid.NewString()); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected revision not found, got %v", err)
	}

	commentRepo := NewCommentRepository(pool)
	comment := domain.PageComment{ID: uuid.NewString(), PageID: page.ID, Body: "Looks good", CreatedBy: member.ID, CreatedAt: now}
	createdComment, err := commentRepo.Create(ctx, comment)
	if err != nil {
		t.Fatalf("create comment: %v", err)
	}
	gotComment, err := commentRepo.GetByID(ctx, createdComment.ID)
	if err != nil || gotComment.Body != comment.Body {
		t.Fatalf("get comment mismatch: err=%v body=%s", err, gotComment.Body)
	}
	comments, err := commentRepo.ListByPageID(ctx, page.ID)
	if err != nil || len(comments) != 1 {
		t.Fatalf("list comments mismatch: err=%v len=%d", err, len(comments))
	}
	resolved, err := commentRepo.Resolve(ctx, createdComment.ID, owner.ID, now.Add(time.Minute))
	if err != nil || resolved.ResolvedBy == nil || *resolved.ResolvedBy != owner.ID {
		t.Fatalf("resolve comment mismatch: err=%v", err)
	}
	if _, err := commentRepo.GetByID(ctx, uuid.NewString()); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected comment not found, got %v", err)
	}
	if _, err := commentRepo.Resolve(ctx, uuid.NewString(), owner.ID, now); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected resolve not found, got %v", err)
	}

	threadRepo := NewThreadRepository(pool)
	blockID := "block-1"
	quotedText := "hello"
	thread := domain.PageCommentThread{
		ID:     uuid.NewString(),
		PageID: page.ID,
		Anchor: domain.PageCommentThreadAnchor{
			Type:            domain.PageCommentThreadAnchorTypeBlock,
			BlockID:         &blockID,
			QuotedText:      &quotedText,
			QuotedBlockText: "hello world",
		},
		ThreadState:    domain.PageCommentThreadStateOpen,
		AnchorState:    domain.PageCommentThreadAnchorStateActive,
		CreatedBy:      member.ID,
		CreatedAt:      now.Add(2 * time.Minute),
		LastActivityAt: now.Add(2 * time.Minute),
		ReplyCount:     1,
	}
	message := domain.PageCommentThreadMessage{
		ID:        uuid.NewString(),
		ThreadID:  thread.ID,
		Body:      "Please revise this section",
		CreatedBy: member.ID,
		CreatedAt: now.Add(2 * time.Minute),
	}
	mentions := []string{" " + member.ID + " ", mentioned.ID, member.ID}
	mentionRows := []domain.PageCommentMessageMention{
		{MessageID: message.ID, MentionedUserID: member.ID},
		{MessageID: message.ID, MentionedUserID: mentioned.ID},
	}
	threadOutboxEvent, err := domain.NewThreadCreatedOutboxEvent(thread, message, workspace.ID, mentions)
	if err != nil {
		t.Fatalf("build thread outbox event: %v", err)
	}
	threadDetail, err := threadRepo.CreateThread(ctx, thread, message, mentionRows, threadOutboxEvent)
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}
	if threadDetail.Thread.ID != thread.ID || len(threadDetail.Messages) != 1 || threadDetail.Messages[0].Body != message.Body {
		t.Fatalf("create thread mismatch: %+v", threadDetail)
	}
	if len(threadDetail.Events) != 1 || threadDetail.Events[0].Type != domain.PageCommentThreadEventTypeCreated {
		t.Fatalf("expected created event in create response, got %+v", threadDetail.Events)
	}
	loadedThread, err := threadRepo.GetThread(ctx, thread.ID)
	if err != nil {
		t.Fatalf("get thread: %v", err)
	}
	if loadedThread.Thread.ID != thread.ID || loadedThread.Thread.ReplyCount != 1 || len(loadedThread.Messages) != 1 || loadedThread.Messages[0].Body != message.Body {
		t.Fatalf("get thread mismatch: %+v", loadedThread)
	}
	if len(loadedThread.Events) != 1 || loadedThread.Events[0].Type != domain.PageCommentThreadEventTypeCreated {
		t.Fatalf("expected created event, got %+v", loadedThread.Events)
	}
	var storedOutbox struct {
		Topic          string
		AggregateType  string
		AggregateID    string
		IdempotencyKey string
		Payload        []byte
		AvailableAt    time.Time
	}
	if err := pool.QueryRow(ctx, `
		SELECT topic, aggregate_type, aggregate_id::text, idempotency_key, payload, available_at
		FROM outbox_events
		WHERE id = $1
	`, threadOutboxEvent.ID).Scan(&storedOutbox.Topic, &storedOutbox.AggregateType, &storedOutbox.AggregateID, &storedOutbox.IdempotencyKey, &storedOutbox.Payload, &storedOutbox.AvailableAt); err != nil {
		t.Fatalf("get thread outbox event: %v", err)
	}
	if storedOutbox.Topic != string(domain.OutboxTopicThreadCreated) || storedOutbox.AggregateType != string(domain.OutboxAggregateTypeThread) || storedOutbox.AggregateID != thread.ID || storedOutbox.IdempotencyKey != threadOutboxEvent.IdempotencyKey || !storedOutbox.AvailableAt.Equal(thread.CreatedAt) {
		t.Fatalf("unexpected stored outbox metadata: %+v", storedOutbox)
	}
	var storedPayload struct {
		ThreadID    string    `json:"thread_id"`
		MessageID   string    `json:"message_id"`
		PageID      string    `json:"page_id"`
		WorkspaceID string    `json:"workspace_id"`
		ActorID     string    `json:"actor_id"`
		OccurredAt  time.Time `json:"occurred_at"`
	}
	if err := json.Unmarshal(storedOutbox.Payload, &storedPayload); err != nil {
		t.Fatalf("unmarshal stored outbox payload: %v", err)
	}
	if storedPayload.ThreadID != thread.ID || storedPayload.MessageID != message.ID || storedPayload.PageID != page.ID || storedPayload.WorkspaceID != workspace.ID || storedPayload.ActorID != member.ID || !storedPayload.OccurredAt.Equal(thread.CreatedAt) {
		t.Fatalf("unexpected stored outbox payload: %+v", storedPayload)
	}
	var storedMentionCount int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM page_comment_message_mentions WHERE message_id = $1`, message.ID).Scan(&storedMentionCount); err != nil {
		t.Fatalf("count thread mentions: %v", err)
	}
	if storedMentionCount != len(mentionRows) {
		t.Fatalf("expected %d mention rows, got %d", len(mentionRows), storedMentionCount)
	}
	var storedMentionPayload struct {
		MentionUserIDs []string `json:"mention_user_ids"`
	}
	if err := json.Unmarshal(storedOutbox.Payload, &storedMentionPayload); err != nil {
		t.Fatalf("unmarshal stored outbox mention payload: %v", err)
	}
	if len(storedMentionPayload.MentionUserIDs) != len(mentionRows) || storedMentionPayload.MentionUserIDs[0] != member.ID || storedMentionPayload.MentionUserIDs[1] != mentioned.ID {
		t.Fatalf("unexpected mention payload: %+v", storedMentionPayload.MentionUserIDs)
	}
	if _, err := threadRepo.GetThread(ctx, uuid.NewString()); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected thread not found, got %v", err)
	}
	secondBlockID := "block-2"
	secondThread := domain.PageCommentThread{
		ID:     uuid.NewString(),
		PageID: page.ID,
		Anchor: domain.PageCommentThreadAnchor{
			Type:            domain.PageCommentThreadAnchorTypeBlock,
			BlockID:         &secondBlockID,
			QuotedBlockText: "architecture notes",
		},
		ThreadState:    domain.PageCommentThreadStateResolved,
		AnchorState:    domain.PageCommentThreadAnchorStateMissing,
		CreatedBy:      owner.ID,
		CreatedAt:      now.Add(3 * time.Minute),
		LastActivityAt: now.Add(3 * time.Minute),
		ReplyCount:     1,
	}
	secondMessage := domain.PageCommentThreadMessage{
		ID:        uuid.NewString(),
		ThreadID:  secondThread.ID,
		Body:      "Archived discussion",
		CreatedBy: owner.ID,
		CreatedAt: now.Add(3 * time.Minute),
	}
	secondOutboxEvent, err := domain.NewThreadCreatedOutboxEvent(secondThread, secondMessage, workspace.ID, nil)
	if err != nil {
		t.Fatalf("build second thread outbox event: %v", err)
	}
	if _, err := threadRepo.CreateThread(ctx, secondThread, secondMessage, nil, secondOutboxEvent); err != nil {
		t.Fatalf("create second thread: %v", err)
	}
	thirdBlockID := "block-3"
	thirdThread := domain.PageCommentThread{
		ID:     uuid.NewString(),
		PageID: page.ID,
		Anchor: domain.PageCommentThreadAnchor{
			Type:            domain.PageCommentThreadAnchorTypeBlock,
			BlockID:         &thirdBlockID,
			QuotedBlockText: "stale text",
		},
		ThreadState:    domain.PageCommentThreadStateOpen,
		AnchorState:    domain.PageCommentThreadAnchorStateOutdated,
		CreatedBy:      owner.ID,
		CreatedAt:      now.Add(350 * time.Second),
		LastActivityAt: now.Add(350 * time.Second),
		ReplyCount:     1,
	}
	thirdMessage := domain.PageCommentThreadMessage{
		ID:        uuid.NewString(),
		ThreadID:  thirdThread.ID,
		Body:      "Outdated discussion",
		CreatedBy: owner.ID,
		CreatedAt: now.Add(350 * time.Second),
	}
	thirdOutboxEvent, err := domain.NewThreadCreatedOutboxEvent(thirdThread, thirdMessage, workspace.ID, nil)
	if err != nil {
		t.Fatalf("build third thread outbox event: %v", err)
	}
	if _, err := threadRepo.CreateThread(ctx, thirdThread, thirdMessage, nil, thirdOutboxEvent); err != nil {
		t.Fatalf("create third thread: %v", err)
	}
	listedThreads, err := threadRepo.ListThreads(ctx, page.ID, nil, nil, nil, nil, nil, "", "", 0, "")
	if err != nil {
		t.Fatalf("list threads: %v", err)
	}
	if len(listedThreads.Threads) != 3 || listedThreads.Threads[0].ID != thirdThread.ID {
		t.Fatalf("unexpected listed threads: %+v", listedThreads.Threads)
	}
	if listedThreads.Counts.Open != 2 || listedThreads.Counts.Resolved != 1 || listedThreads.Counts.Active != 1 || listedThreads.Counts.Outdated != 1 || listedThreads.Counts.Missing != 1 {
		t.Fatalf("unexpected thread counts: %+v", listedThreads.Counts)
	}
	newestThreads, err := threadRepo.ListThreads(ctx, page.ID, nil, nil, nil, nil, nil, "newest", "", 0, "")
	if err != nil {
		t.Fatalf("list threads newest: %v", err)
	}
	if len(newestThreads.Threads) != 3 || newestThreads.Threads[0].ID != thirdThread.ID || newestThreads.Threads[1].ID != secondThread.ID || newestThreads.Threads[2].ID != thread.ID {
		t.Fatalf("unexpected newest threads order: %+v", newestThreads.Threads)
	}
	oldestThreads, err := threadRepo.ListThreads(ctx, page.ID, nil, nil, nil, nil, nil, "oldest", "", 0, "")
	if err != nil {
		t.Fatalf("list threads oldest: %v", err)
	}
	if len(oldestThreads.Threads) != 3 || oldestThreads.Threads[0].ID != thread.ID || oldestThreads.Threads[1].ID != secondThread.ID || oldestThreads.Threads[2].ID != thirdThread.ID {
		t.Fatalf("unexpected oldest threads order: %+v", oldestThreads.Threads)
	}
	recentActivityThreads, err := threadRepo.ListThreads(ctx, page.ID, nil, nil, nil, nil, nil, "recent_activity", "", 0, "")
	if err != nil {
		t.Fatalf("list threads recent_activity: %v", err)
	}
	if len(recentActivityThreads.Threads) != 3 || recentActivityThreads.Threads[0].ID != thirdThread.ID || recentActivityThreads.Threads[1].ID != thread.ID || recentActivityThreads.Threads[2].ID != secondThread.ID {
		t.Fatalf("unexpected recent_activity threads order: %+v", recentActivityThreads.Threads)
	}

	collidingThread := domain.PageCommentThread{
		ID:     uuid.NewString(),
		PageID: page.ID,
		Anchor: domain.PageCommentThreadAnchor{
			Type:            domain.PageCommentThreadAnchorTypeBlock,
			BlockID:         &blockID,
			QuotedText:      &quotedText,
			QuotedBlockText: "duplicate outbox",
		},
		ThreadState:    domain.PageCommentThreadStateOpen,
		AnchorState:    domain.PageCommentThreadAnchorStateActive,
		CreatedBy:      member.ID,
		CreatedAt:      now.Add(4 * time.Minute),
		LastActivityAt: now.Add(4 * time.Minute),
		ReplyCount:     1,
	}
	collidingMessage := domain.PageCommentThreadMessage{
		ID:        uuid.NewString(),
		ThreadID:  collidingThread.ID,
		Body:      "Should roll back",
		CreatedBy: member.ID,
		CreatedAt: now.Add(4 * time.Minute),
	}
	collidingOutboxEvent, err := domain.NewThreadCreatedOutboxEvent(collidingThread, collidingMessage, workspace.ID, nil)
	if err != nil {
		t.Fatalf("build colliding thread outbox event: %v", err)
	}
	mustExec(t, pool, `
		INSERT INTO outbox_events (
			id, topic, aggregate_type, aggregate_id, idempotency_key, payload,
			status, attempt_count, max_attempts, available_at,
			claimed_by, claimed_at, lease_expires_at, last_error,
			processed_at, dead_lettered_at, created_at, updated_at
		)
		VALUES ($1,$2,$3,$4,$5,$6,'pending',0,25,$7,NULL,NULL,NULL,NULL,NULL,NULL,$8,$8)
	`, uuid.NewString(), domain.OutboxTopicThreadCreated, domain.OutboxAggregateTypeThread, uuid.NewString(), collidingOutboxEvent.IdempotencyKey, collidingOutboxEvent.Payload, collidingOutboxEvent.AvailableAt, collidingOutboxEvent.CreatedAt)
	if _, err := threadRepo.CreateThread(ctx, collidingThread, collidingMessage, nil, collidingOutboxEvent); err == nil {
		t.Fatal("expected thread create to fail when outbox idempotency key already exists")
	}
	var threadCount, messageCount, eventCount int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM page_comment_threads WHERE id = $1`, collidingThread.ID).Scan(&threadCount); err != nil {
		t.Fatalf("count colliding thread rows: %v", err)
	}
	if threadCount != 0 {
		t.Fatalf("expected no committed thread rows on outbox failure, got %d", threadCount)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM page_comment_messages WHERE thread_id = $1`, collidingThread.ID).Scan(&messageCount); err != nil {
		t.Fatalf("count colliding message rows: %v", err)
	}
	if messageCount != 0 {
		t.Fatalf("expected no committed message rows on outbox failure, got %d", messageCount)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM page_comment_thread_events WHERE thread_id = $1`, collidingThread.ID).Scan(&eventCount); err != nil {
		t.Fatalf("count colliding event rows: %v", err)
	}
	if eventCount != 0 {
		t.Fatalf("expected no committed thread event rows on outbox failure, got %d", eventCount)
	}

	mismatchedCreateMessage := domain.PageCommentThreadMessage{
		ID:        uuid.NewString(),
		ThreadID:  uuid.NewString(),
		Body:      "Wrong thread id",
		CreatedBy: member.ID,
		CreatedAt: now.Add(5 * time.Minute),
	}
	mismatchedCreateEvent, err := domain.NewThreadCreatedOutboxEvent(collidingThread, domain.PageCommentThreadMessage{
		ID:        mismatchedCreateMessage.ID,
		ThreadID:  collidingThread.ID,
		Body:      mismatchedCreateMessage.Body,
		CreatedBy: mismatchedCreateMessage.CreatedBy,
		CreatedAt: mismatchedCreateMessage.CreatedAt,
	}, workspace.ID, nil)
	if err != nil {
		t.Fatalf("build mismatched create outbox event: %v", err)
	}
	if _, err := threadRepo.CreateThread(ctx, collidingThread, mismatchedCreateMessage, nil, mismatchedCreateEvent); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("expected mismatched starter message thread id validation error, got %v", err)
	}

	rollbackThread := domain.PageCommentThread{
		ID:     uuid.NewString(),
		PageID: page.ID,
		Anchor: domain.PageCommentThreadAnchor{
			Type:            domain.PageCommentThreadAnchorTypeBlock,
			BlockID:         &blockID,
			QuotedText:      &quotedText,
			QuotedBlockText: "rollback mentions",
		},
		ThreadState:    domain.PageCommentThreadStateOpen,
		AnchorState:    domain.PageCommentThreadAnchorStateActive,
		CreatedBy:      member.ID,
		CreatedAt:      now.Add(5 * time.Minute),
		LastActivityAt: now.Add(5 * time.Minute),
		ReplyCount:     1,
	}
	rollbackMessage := domain.PageCommentThreadMessage{
		ID:        uuid.NewString(),
		ThreadID:  rollbackThread.ID,
		Body:      "Should roll back mentions",
		CreatedBy: member.ID,
		CreatedAt: now.Add(5 * time.Minute),
	}
	rollbackOutboxEvent, err := domain.NewThreadCreatedOutboxEvent(rollbackThread, rollbackMessage, workspace.ID, []string{member.ID, member.ID})
	if err != nil {
		t.Fatalf("build rollback outbox event: %v", err)
	}
	if _, err := threadRepo.CreateThread(ctx, rollbackThread, rollbackMessage, []domain.PageCommentMessageMention{
		{MessageID: rollbackMessage.ID, MentionedUserID: member.ID},
		{MessageID: rollbackMessage.ID, MentionedUserID: member.ID},
	}, rollbackOutboxEvent); err == nil {
		t.Fatal("expected thread create to fail when mention insert conflicts")
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM page_comment_threads WHERE id = $1`, rollbackThread.ID).Scan(&threadCount); err != nil {
		t.Fatalf("count rollback thread rows: %v", err)
	}
	if threadCount != 0 {
		t.Fatalf("expected no committed thread rows on mention failure, got %d", threadCount)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM page_comment_messages WHERE thread_id = $1`, rollbackThread.ID).Scan(&messageCount); err != nil {
		t.Fatalf("count rollback message rows: %v", err)
	}
	if messageCount != 0 {
		t.Fatalf("expected no committed message rows on mention failure, got %d", messageCount)
	}
	var mentionCount int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM page_comment_message_mentions WHERE message_id = $1`, rollbackMessage.ID).Scan(&mentionCount); err != nil {
		t.Fatalf("count rollback mention rows: %v", err)
	}
	if mentionCount != 0 {
		t.Fatalf("expected no committed mention rows on mention failure, got %d", mentionCount)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM outbox_events WHERE aggregate_id = $1`, rollbackThread.ID).Scan(&eventCount); err != nil {
		t.Fatalf("count rollback outbox rows: %v", err)
	}
	if eventCount != 0 {
		t.Fatalf("expected no committed outbox rows on mention failure, got %d", eventCount)
	}

	outsider := seedUser(t, pool, "combo-outsider@example.com")
	foreignMentionThread := domain.PageCommentThread{
		ID:     uuid.NewString(),
		PageID: page.ID,
		Anchor: domain.PageCommentThreadAnchor{
			Type:            domain.PageCommentThreadAnchorTypeBlock,
			BlockID:         &blockID,
			QuotedText:      &quotedText,
			QuotedBlockText: "foreign mention",
		},
		ThreadState:    domain.PageCommentThreadStateOpen,
		AnchorState:    domain.PageCommentThreadAnchorStateActive,
		CreatedBy:      member.ID,
		CreatedAt:      now.Add(6 * time.Minute),
		LastActivityAt: now.Add(6 * time.Minute),
		ReplyCount:     1,
	}
	foreignMentionMessage := domain.PageCommentThreadMessage{
		ID:        uuid.NewString(),
		ThreadID:  foreignMentionThread.ID,
		Body:      "Should reject foreign mention",
		CreatedBy: member.ID,
		CreatedAt: now.Add(6 * time.Minute),
	}
	foreignMentionOutboxEvent, err := domain.NewThreadCreatedOutboxEvent(foreignMentionThread, foreignMentionMessage, workspace.ID, []string{outsider.ID})
	if err != nil {
		t.Fatalf("build foreign mention outbox event: %v", err)
	}
	if _, err := threadRepo.CreateThread(ctx, foreignMentionThread, foreignMentionMessage, []domain.PageCommentMessageMention{
		{MessageID: foreignMentionMessage.ID, MentionedUserID: outsider.ID},
	}, foreignMentionOutboxEvent); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("expected foreign mention validation error, got %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM page_comment_threads WHERE id = $1`, foreignMentionThread.ID).Scan(&threadCount); err != nil {
		t.Fatalf("count foreign mention thread rows: %v", err)
	}
	if threadCount != 0 {
		t.Fatalf("expected no committed thread rows on foreign mention validation, got %d", threadCount)
	}
	paginatedThreads, err := threadRepo.ListThreads(ctx, page.ID, nil, nil, nil, nil, nil, "recent_activity", "", 2, "")
	if err != nil {
		t.Fatalf("list threads recent_activity paginated: %v", err)
	}
	if len(paginatedThreads.Threads) != 2 || paginatedThreads.Threads[0].ID != thirdThread.ID || paginatedThreads.Threads[1].ID != thread.ID {
		t.Fatalf("unexpected recent_activity paginated threads: %+v", paginatedThreads.Threads)
	}
	if !paginatedThreads.HasMore || paginatedThreads.NextCursor == nil {
		t.Fatalf("expected next cursor on paginated thread list, got %+v", paginatedThreads)
	}
	nextThreads, err := threadRepo.ListThreads(ctx, page.ID, nil, nil, nil, nil, nil, "recent_activity", "", 2, *paginatedThreads.NextCursor)
	if err != nil {
		t.Fatalf("list threads recent_activity next page: %v", err)
	}
	if len(nextThreads.Threads) != 1 || nextThreads.Threads[0].ID != secondThread.ID {
		t.Fatalf("unexpected next page threads: %+v", nextThreads.Threads)
	}
	if nextThreads.HasMore || nextThreads.NextCursor != nil {
		t.Fatalf("expected final thread page without next cursor, got %+v", nextThreads)
	}
	paginatedNewestThreads, err := threadRepo.ListThreads(ctx, page.ID, nil, nil, nil, nil, nil, "newest", "", 2, "")
	if err != nil {
		t.Fatalf("list threads newest paginated: %v", err)
	}
	if len(paginatedNewestThreads.Threads) != 2 || paginatedNewestThreads.Threads[0].ID != thirdThread.ID || paginatedNewestThreads.Threads[1].ID != secondThread.ID {
		t.Fatalf("unexpected newest paginated threads: %+v", paginatedNewestThreads.Threads)
	}
	if paginatedNewestThreads.NextCursor == nil {
		t.Fatalf("expected newest next cursor, got %+v", paginatedNewestThreads)
	}
	nextNewestThreads, err := threadRepo.ListThreads(ctx, page.ID, nil, nil, nil, nil, nil, "newest", "", 2, *paginatedNewestThreads.NextCursor)
	if err != nil {
		t.Fatalf("list threads newest next page: %v", err)
	}
	if len(nextNewestThreads.Threads) != 1 || nextNewestThreads.Threads[0].ID != thread.ID {
		t.Fatalf("unexpected newest next page threads: %+v", nextNewestThreads.Threads)
	}
	if _, err := threadRepo.ListThreads(ctx, page.ID, nil, nil, nil, nil, nil, "recent_activity", "", 2, "broken"); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("expected invalid cursor validation error, got %v", err)
	}
	openState := domain.PageCommentThreadStateOpen
	filteredThreads, err := threadRepo.ListThreads(ctx, page.ID, &openState, nil, nil, nil, nil, "", "revise", 0, "")
	if err != nil {
		t.Fatalf("list filtered threads: %v", err)
	}
	if len(filteredThreads.Threads) != 1 || filteredThreads.Threads[0].ID != thread.ID {
		t.Fatalf("unexpected filtered threads: %+v", filteredThreads.Threads)
	}
	createdByThreads, err := threadRepo.ListThreads(ctx, page.ID, nil, nil, &member.ID, nil, nil, "", "", 0, "")
	if err != nil {
		t.Fatalf("list threads by created_by: %v", err)
	}
	if len(createdByThreads.Threads) != 1 || createdByThreads.Threads[0].ID != thread.ID {
		t.Fatalf("expected created_by filter to match member-owned thread, got %+v", createdByThreads.Threads)
	}
	hasMissingAnchor := true
	onlyMissingThreads, err := threadRepo.ListThreads(ctx, page.ID, nil, nil, nil, &hasMissingAnchor, nil, "", "", 0, "")
	if err != nil {
		t.Fatalf("list threads by has_missing_anchor=true: %v", err)
	}
	if len(onlyMissingThreads.Threads) != 1 || onlyMissingThreads.Threads[0].ID != secondThread.ID {
		t.Fatalf("expected has_missing_anchor=true to match second thread, got %+v", onlyMissingThreads.Threads)
	}
	hasNoMissingAnchor := false
	noMissingThreads, err := threadRepo.ListThreads(ctx, page.ID, nil, nil, nil, &hasNoMissingAnchor, nil, "", "", 0, "")
	if err != nil {
		t.Fatalf("list threads by has_missing_anchor=false: %v", err)
	}
	if len(noMissingThreads.Threads) != 2 {
		t.Fatalf("expected has_missing_anchor=false to exclude missing thread, got %+v", noMissingThreads.Threads)
	}
	hasOutdatedAnchor := true
	onlyOutdatedThreads, err := threadRepo.ListThreads(ctx, page.ID, nil, nil, nil, nil, &hasOutdatedAnchor, "", "", 0, "")
	if err != nil {
		t.Fatalf("list threads by has_outdated_anchor=true: %v", err)
	}
	if len(onlyOutdatedThreads.Threads) != 1 || onlyOutdatedThreads.Threads[0].ID != thirdThread.ID {
		t.Fatalf("expected has_outdated_anchor=true to match third thread, got %+v", onlyOutdatedThreads.Threads)
	}
	hasNoOutdatedAnchor := false
	noOutdatedThreads, err := threadRepo.ListThreads(ctx, page.ID, nil, nil, nil, nil, &hasNoOutdatedAnchor, "", "", 0, "")
	if err != nil {
		t.Fatalf("list threads by has_outdated_anchor=false: %v", err)
	}
	if len(noOutdatedThreads.Threads) != 2 {
		t.Fatalf("expected has_outdated_anchor=false to exclude outdated thread, got %+v", noOutdatedThreads.Threads)
	}
	missingAnchorState := domain.PageCommentThreadAnchorStateMissing
	missingThreads, err := threadRepo.ListThreads(ctx, page.ID, nil, &missingAnchorState, nil, nil, nil, "", "", 0, "")
	if err != nil {
		t.Fatalf("list threads by anchor_state: %v", err)
	}
	if len(missingThreads.Threads) != 1 || missingThreads.Threads[0].ID != secondThread.ID {
		t.Fatalf("expected missing-anchor filter to match second thread, got %+v", missingThreads.Threads)
	}
	if missingThreads.Counts.Open != 2 || missingThreads.Counts.Resolved != 1 || missingThreads.Counts.Active != 1 || missingThreads.Counts.Outdated != 1 || missingThreads.Counts.Missing != 1 {
		t.Fatalf("expected counts to remain page-wide under filtering, got %+v", missingThreads.Counts)
	}
	quotedSearchThreads, err := threadRepo.ListThreads(ctx, page.ID, nil, nil, nil, nil, nil, "", "hello", 0, "")
	if err != nil {
		t.Fatalf("list threads by quoted text: %v", err)
	}
	if len(quotedSearchThreads.Threads) != 1 || quotedSearchThreads.Threads[0].ID != thread.ID {
		t.Fatalf("expected quoted text search to match first thread, got %+v", quotedSearchThreads.Threads)
	}
	workspaceThreads, err := threadRepo.ListWorkspaceThreads(ctx, workspace.ID, nil, nil, nil, nil, nil, "recent_activity", "", 0, "")
	if err != nil {
		t.Fatalf("list workspace threads: %v", err)
	}
	if len(workspaceThreads.Threads) != 3 || workspaceThreads.Threads[0].Thread.ID != thirdThread.ID || workspaceThreads.Threads[0].Page.ID != page.ID {
		t.Fatalf("unexpected workspace threads: %+v", workspaceThreads.Threads)
	}
	paginatedWorkspaceThreads, err := threadRepo.ListWorkspaceThreads(ctx, workspace.ID, nil, nil, nil, nil, nil, "recent_activity", "", 2, "")
	if err != nil {
		t.Fatalf("list workspace threads paginated: %v", err)
	}
	if len(paginatedWorkspaceThreads.Threads) != 2 || paginatedWorkspaceThreads.Threads[0].Thread.ID != thirdThread.ID || paginatedWorkspaceThreads.Threads[1].Thread.ID != thread.ID {
		t.Fatalf("unexpected paginated workspace threads: %+v", paginatedWorkspaceThreads.Threads)
	}
	if !paginatedWorkspaceThreads.HasMore || paginatedWorkspaceThreads.NextCursor == nil {
		t.Fatalf("expected workspace next cursor, got %+v", paginatedWorkspaceThreads)
	}
	nextWorkspaceThreads, err := threadRepo.ListWorkspaceThreads(ctx, workspace.ID, nil, nil, nil, nil, nil, "recent_activity", "", 2, *paginatedWorkspaceThreads.NextCursor)
	if err != nil {
		t.Fatalf("list workspace threads next page: %v", err)
	}
	if len(nextWorkspaceThreads.Threads) != 1 || nextWorkspaceThreads.Threads[0].Thread.ID != secondThread.ID {
		t.Fatalf("unexpected next workspace thread page: %+v", nextWorkspaceThreads.Threads)
	}
	if _, err := threadRepo.ListWorkspaceThreads(ctx, workspace.ID, nil, nil, nil, nil, nil, "recent_activity", "", 2, "broken"); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("expected invalid workspace cursor validation error, got %v", err)
	}
	workspaceSearchThreads, err := threadRepo.ListWorkspaceThreads(ctx, workspace.ID, nil, nil, nil, nil, nil, "", "architecture", 0, "")
	if err != nil {
		t.Fatalf("list workspace threads by search: %v", err)
	}
	if len(workspaceSearchThreads.Threads) != 1 || workspaceSearchThreads.Threads[0].Thread.ID != secondThread.ID {
		t.Fatalf("expected workspace search to match second thread by page title, got %+v", workspaceSearchThreads.Threads)
	}
	pageRepo := NewPageRepository(pool)
	trashItem := domain.TrashItem{
		ID:          uuid.NewString(),
		WorkspaceID: workspace.ID,
		PageID:      page.ID,
		PageTitle:   page.Title,
		DeletedBy:   owner.ID,
		DeletedAt:   now.Add(6 * time.Minute),
	}
	if err := pageRepo.SoftDelete(ctx, trashItem); err != nil {
		t.Fatalf("soft delete page for workspace threads: %v", err)
	}
	workspaceThreadsAfterTrash, err := threadRepo.ListWorkspaceThreads(ctx, workspace.ID, nil, nil, nil, nil, nil, "recent_activity", "", 0, "")
	if err != nil {
		t.Fatalf("list workspace threads after trash: %v", err)
	}
	if len(workspaceThreadsAfterTrash.Threads) != 0 {
		t.Fatalf("expected trashed page threads to be excluded from workspace inbox, got %+v", workspaceThreadsAfterTrash.Threads)
	}
	replyTime := now.Add(4 * time.Minute)
	replyMessage := domain.PageCommentThreadMessage{
		ID:        uuid.NewString(),
		ThreadID:  secondThread.ID,
		Body:      "Need another look",
		CreatedBy: member.ID,
		CreatedAt: replyTime,
	}
	replyMentions := []domain.PageCommentMessageMention{
		{MessageID: replyMessage.ID, MentionedUserID: owner.ID},
		{MessageID: replyMessage.ID, MentionedUserID: member.ID},
	}
	replyOutboxEvent, err := domain.NewThreadReplyCreatedOutboxEvent(secondThread, replyMessage, workspace.ID, []string{owner.ID, member.ID})
	if err != nil {
		t.Fatalf("build thread reply outbox event: %v", err)
	}
	invalidReplyMessage := domain.PageCommentThreadMessage{
		ID:        uuid.NewString(),
		ThreadID:  secondThread.ID,
		Body:      "Invalid outbox reply",
		CreatedBy: member.ID,
		CreatedAt: replyTime.Add(-time.Minute),
	}
	invalidReplyOutboxEvent, err := domain.NewThreadReplyCreatedOutboxEvent(secondThread, invalidReplyMessage, workspace.ID, nil)
	if err != nil {
		t.Fatalf("build invalid thread reply outbox event: %v", err)
	}
	invalidReplyOutboxEvent.AggregateID = uuid.NewString()
	if _, err := threadRepo.AddReply(ctx, secondThread.ID, invalidReplyMessage, nil, domain.PageCommentThread{
		ID:             secondThread.ID,
		PageID:         secondThread.PageID,
		Anchor:         secondThread.Anchor,
		ThreadState:    domain.PageCommentThreadStateOpen,
		AnchorState:    secondThread.AnchorState,
		CreatedBy:      secondThread.CreatedBy,
		CreatedAt:      secondThread.CreatedAt,
		ReopenedBy:     &member.ID,
		ReopenedAt:     &invalidReplyMessage.CreatedAt,
		LastActivityAt: invalidReplyMessage.CreatedAt,
	}, invalidReplyOutboxEvent); err == nil {
		t.Fatal("expected add reply to fail when outbox contract mismatches the reply")
	}
	unchangedThread, err := threadRepo.GetThread(ctx, secondThread.ID)
	if err != nil {
		t.Fatalf("get thread after invalid reply outbox validation failure: %v", err)
	}
	if unchangedThread.Thread.ThreadState != secondThread.ThreadState || unchangedThread.Thread.ReplyCount != secondThread.ReplyCount || len(unchangedThread.Messages) != 1 || len(unchangedThread.Events) != 1 {
		t.Fatalf("expected thread to remain unchanged after invalid reply outbox failure, got %+v", unchangedThread)
	}
	repliedThread, err := threadRepo.AddReply(ctx, secondThread.ID, replyMessage, replyMentions, domain.PageCommentThread{
		ID:             secondThread.ID,
		PageID:         secondThread.PageID,
		Anchor:         secondThread.Anchor,
		ThreadState:    domain.PageCommentThreadStateOpen,
		AnchorState:    secondThread.AnchorState,
		CreatedBy:      secondThread.CreatedBy,
		CreatedAt:      secondThread.CreatedAt,
		ReopenedBy:     &member.ID,
		ReopenedAt:     &replyTime,
		LastActivityAt: replyTime,
	}, replyOutboxEvent)
	if err != nil {
		t.Fatalf("add reply: %v", err)
	}
	if repliedThread.Thread.ThreadState != domain.PageCommentThreadStateOpen || repliedThread.Thread.ReopenedBy == nil || *repliedThread.Thread.ReopenedBy != member.ID {
		t.Fatalf("expected auto reopen markers after reply, got %+v", repliedThread.Thread)
	}
	if repliedThread.Thread.ReplyCount != 2 || len(repliedThread.Messages) != 2 || repliedThread.Messages[1].Body != "Need another look" {
		t.Fatalf("unexpected replied thread payload: %+v", repliedThread)
	}
	if !hasThreadEventType(repliedThread.Events, domain.PageCommentThreadEventTypeReplied) || !hasThreadEventType(repliedThread.Events, domain.PageCommentThreadEventTypeReopened) {
		t.Fatalf("expected reply and reopened events, got %+v", repliedThread.Events)
	}
	var replyMentionCount int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*)::int
		FROM page_comment_message_mentions
		WHERE message_id = $1
	`, replyMessage.ID).Scan(&replyMentionCount); err != nil {
		t.Fatalf("count reply mention rows: %v", err)
	}
	if replyMentionCount != len(replyMentions) {
		t.Fatalf("expected reply mention rows for reply message, got %d want %d", replyMentionCount, len(replyMentions))
	}
	var storedReplyOutbox struct {
		Topic          string
		AggregateType  string
		AggregateID    string
		IdempotencyKey string
		Payload        []byte
		AvailableAt    time.Time
	}
	if err := pool.QueryRow(ctx, `
		SELECT topic, aggregate_type, aggregate_id::text, idempotency_key, payload, available_at
		FROM outbox_events
		WHERE id = $1
	`, replyOutboxEvent.ID).Scan(&storedReplyOutbox.Topic, &storedReplyOutbox.AggregateType, &storedReplyOutbox.AggregateID, &storedReplyOutbox.IdempotencyKey, &storedReplyOutbox.Payload, &storedReplyOutbox.AvailableAt); err != nil {
		t.Fatalf("get thread reply outbox event: %v", err)
	}
	if storedReplyOutbox.Topic != string(domain.OutboxTopicThreadReplyCreated) || storedReplyOutbox.AggregateType != string(domain.OutboxAggregateTypeThreadMessage) || storedReplyOutbox.AggregateID != replyMessage.ID || storedReplyOutbox.IdempotencyKey != replyOutboxEvent.IdempotencyKey || !storedReplyOutbox.AvailableAt.Equal(replyMessage.CreatedAt) {
		t.Fatalf("unexpected reply outbox metadata: %+v", storedReplyOutbox)
	}
	var storedReplyPayload struct {
		ThreadID       string    `json:"thread_id"`
		MessageID      string    `json:"message_id"`
		PageID         string    `json:"page_id"`
		WorkspaceID    string    `json:"workspace_id"`
		ActorID        string    `json:"actor_id"`
		OccurredAt     time.Time `json:"occurred_at"`
		MentionUserIDs []string  `json:"mention_user_ids"`
	}
	if err := json.Unmarshal(storedReplyOutbox.Payload, &storedReplyPayload); err != nil {
		t.Fatalf("unmarshal thread reply outbox payload: %v", err)
	}
	if storedReplyPayload.ThreadID != secondThread.ID || storedReplyPayload.MessageID != replyMessage.ID || storedReplyPayload.PageID != secondThread.PageID || storedReplyPayload.WorkspaceID != workspace.ID || storedReplyPayload.ActorID != member.ID || !storedReplyPayload.OccurredAt.Equal(replyMessage.CreatedAt) {
		t.Fatalf("unexpected reply outbox payload: %+v", storedReplyPayload)
	}
	if len(storedReplyPayload.MentionUserIDs) != 2 || storedReplyPayload.MentionUserIDs[0] != owner.ID || storedReplyPayload.MentionUserIDs[1] != member.ID {
		t.Fatalf("unexpected reply outbox mention ids: %+v", storedReplyPayload.MentionUserIDs)
	}
	if _, err := threadRepo.AddReply(ctx, uuid.NewString(), domain.PageCommentThreadMessage{
		ID:        uuid.NewString(),
		ThreadID:  uuid.NewString(),
		Body:      "ghost reply",
		CreatedBy: member.ID,
		CreatedAt: replyTime,
	}, nil, domain.PageCommentThread{
		ID:             uuid.NewString(),
		PageID:         secondThread.PageID,
		Anchor:         secondThread.Anchor,
		ThreadState:    domain.PageCommentThreadStateOpen,
		AnchorState:    secondThread.AnchorState,
		CreatedBy:      secondThread.CreatedBy,
		CreatedAt:      secondThread.CreatedAt,
		LastActivityAt: replyTime,
	}, domain.OutboxEvent{}); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected add reply not found, got %v", err)
	}
	resolveTime := now.Add(5 * time.Minute)
	resolveNote := "Fixed in latest revision"
	resolvedThread, err := threadRepo.UpdateThreadState(ctx, secondThread.ID, domain.PageCommentThread{
		ID:             repliedThread.Thread.ID,
		PageID:         repliedThread.Thread.PageID,
		Anchor:         repliedThread.Thread.Anchor,
		ThreadState:    domain.PageCommentThreadStateResolved,
		AnchorState:    repliedThread.Thread.AnchorState,
		CreatedBy:      repliedThread.Thread.CreatedBy,
		CreatedAt:      repliedThread.Thread.CreatedAt,
		ResolvedBy:     &owner.ID,
		ResolvedAt:     &resolveTime,
		ResolveNote:    &resolveNote,
		ReopenedBy:     repliedThread.Thread.ReopenedBy,
		ReopenedAt:     repliedThread.Thread.ReopenedAt,
		LastActivityAt: resolveTime,
	}, nil)
	if err != nil {
		t.Fatalf("resolve thread state: %v", err)
	}
	if resolvedThread.Thread.ThreadState != domain.PageCommentThreadStateResolved || resolvedThread.Thread.ResolvedBy == nil || *resolvedThread.Thread.ResolvedBy != owner.ID {
		t.Fatalf("unexpected resolved thread payload: %+v", resolvedThread.Thread)
	}
	if resolvedThread.Thread.ResolvedAt == nil || !resolvedThread.Thread.LastActivityAt.Equal(*resolvedThread.Thread.ResolvedAt) {
		t.Fatalf("expected resolved thread to update last_activity_at, got %+v", resolvedThread.Thread)
	}
	if resolvedThread.Thread.ResolveNote == nil || *resolvedThread.Thread.ResolveNote != resolveNote {
		t.Fatalf("expected resolve note to persist, got %+v", resolvedThread.Thread)
	}
	if !hasThreadEventType(resolvedThread.Events, domain.PageCommentThreadEventTypeResolved) {
		t.Fatalf("expected resolved event, got %+v", resolvedThread.Events)
	}
	rollbackReplyMessage := domain.PageCommentThreadMessage{
		ID:        uuid.NewString(),
		ThreadID:  resolvedThread.Thread.ID,
		Body:      "Should roll back",
		CreatedBy: member.ID,
		CreatedAt: replyTime.Add(time.Minute),
	}
	rollbackReplyMentions := []domain.PageCommentMessageMention{
		{MessageID: rollbackReplyMessage.ID, MentionedUserID: member.ID},
		{MessageID: rollbackReplyMessage.ID, MentionedUserID: owner.ID},
	}
	rollbackOutboxEvent, err = domain.NewThreadReplyCreatedOutboxEvent(resolvedThread.Thread, rollbackReplyMessage, workspace.ID, []string{member.ID})
	if err != nil {
		t.Fatalf("build rollback reply outbox event: %v", err)
	}
	mustExec(t, pool, `
		INSERT INTO outbox_events (
			id, topic, aggregate_type, aggregate_id, idempotency_key, payload,
			status, attempt_count, max_attempts, available_at,
			claimed_by, claimed_at, lease_expires_at, last_error,
			processed_at, dead_lettered_at, created_at, updated_at
		)
		VALUES ($1,$2,$3,$4,$5,$6,'pending',0,25,$7,NULL,NULL,NULL,NULL,NULL,NULL,$8,$8)
	`, uuid.NewString(), rollbackOutboxEvent.Topic, rollbackOutboxEvent.AggregateType, uuid.NewString(), rollbackOutboxEvent.IdempotencyKey, rollbackOutboxEvent.Payload, rollbackOutboxEvent.AvailableAt, rollbackOutboxEvent.CreatedAt)
	_, err = threadRepo.AddReply(ctx, resolvedThread.Thread.ID, rollbackReplyMessage, rollbackReplyMentions, domain.PageCommentThread{
		ID:             resolvedThread.Thread.ID,
		PageID:         resolvedThread.Thread.PageID,
		Anchor:         resolvedThread.Thread.Anchor,
		ThreadState:    domain.PageCommentThreadStateOpen,
		AnchorState:    resolvedThread.Thread.AnchorState,
		CreatedBy:      resolvedThread.Thread.CreatedBy,
		CreatedAt:      resolvedThread.Thread.CreatedAt,
		ResolvedBy:     nil,
		ResolvedAt:     nil,
		ResolveNote:    nil,
		ReopenedBy:     &member.ID,
		ReopenedAt:     &rollbackReplyMessage.CreatedAt,
		LastActivityAt: rollbackReplyMessage.CreatedAt,
	}, rollbackOutboxEvent)
	if err == nil {
		t.Fatal("expected add reply to fail on outbox collision")
	}
	rolledBackThread, err := threadRepo.GetThread(ctx, resolvedThread.Thread.ID)
	if err != nil {
		t.Fatalf("get rolled back thread: %v", err)
	}
	if rolledBackThread.Thread.ThreadState != domain.PageCommentThreadStateResolved || len(rolledBackThread.Messages) != len(resolvedThread.Messages) || rolledBackThread.Thread.ReplyCount != resolvedThread.Thread.ReplyCount || !rolledBackThread.Thread.LastActivityAt.Equal(resolvedThread.Thread.LastActivityAt) {
		t.Fatalf("expected resolved thread to remain unchanged after rollback, got thread=%+v messages=%d", rolledBackThread.Thread, len(rolledBackThread.Messages))
	}
	var rollbackMentionCount int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*)::int
		FROM page_comment_message_mentions
		WHERE message_id = $1
	`, rollbackReplyMessage.ID).Scan(&rollbackMentionCount); err != nil {
		t.Fatalf("count rolled back reply mention rows: %v", err)
	}
	if rollbackMentionCount != 0 {
		t.Fatalf("expected no mention rows after rollback, got %d", rollbackMentionCount)
	}
	var rollbackEventCount int
	var rollbackRepliedCount int
	var rollbackReopenedCount int
	if err := pool.QueryRow(ctx, `
		SELECT
			COUNT(*)::int,
			COUNT(*) FILTER (WHERE event_type = 'replied')::int,
			COUNT(*) FILTER (WHERE event_type = 'reopened')::int
		FROM page_comment_thread_events
		WHERE thread_id = $1
	`, resolvedThread.Thread.ID).Scan(&rollbackEventCount, &rollbackRepliedCount, &rollbackReopenedCount); err != nil {
		t.Fatalf("count rolled back reply event rows: %v", err)
	}
	if rollbackEventCount != len(resolvedThread.Events) {
		t.Fatalf("expected event count to remain unchanged after rollback, got %d want %d", rollbackEventCount, len(resolvedThread.Events))
	}
	if rollbackRepliedCount != 1 || rollbackReopenedCount != 1 {
		t.Fatalf("expected no extra reopened/replied events after rollback, got replied=%d reopened=%d", rollbackRepliedCount, rollbackReopenedCount)
	}
	reopenReason := "Need another look"
	reopenedThread, err := threadRepo.UpdateThreadState(ctx, secondThread.ID, domain.PageCommentThread{
		ID:             resolvedThread.Thread.ID,
		PageID:         resolvedThread.Thread.PageID,
		Anchor:         resolvedThread.Thread.Anchor,
		ThreadState:    domain.PageCommentThreadStateOpen,
		AnchorState:    resolvedThread.Thread.AnchorState,
		CreatedBy:      resolvedThread.Thread.CreatedBy,
		CreatedAt:      resolvedThread.Thread.CreatedAt,
		ResolvedBy:     nil,
		ResolvedAt:     nil,
		ResolveNote:    nil,
		ReopenedBy:     &member.ID,
		ReopenedAt:     &replyTime,
		ReopenReason:   &reopenReason,
		LastActivityAt: replyTime,
	}, nil)
	if err != nil {
		t.Fatalf("reopen thread state: %v", err)
	}
	if reopenedThread.Thread.ThreadState != domain.PageCommentThreadStateOpen || reopenedThread.Thread.ReopenedBy == nil || *reopenedThread.Thread.ReopenedBy != member.ID {
		t.Fatalf("unexpected reopened thread payload: %+v", reopenedThread.Thread)
	}
	if reopenedThread.Thread.ReopenedAt == nil || !reopenedThread.Thread.LastActivityAt.Equal(*reopenedThread.Thread.ReopenedAt) {
		t.Fatalf("expected reopened thread to update last_activity_at, got %+v", reopenedThread.Thread)
	}
	if reopenedThread.Thread.ResolveNote != nil {
		t.Fatalf("expected resolve note cleared on reopen, got %+v", reopenedThread.Thread)
	}
	if reopenedThread.Thread.ReopenReason == nil || *reopenedThread.Thread.ReopenReason != reopenReason {
		t.Fatalf("expected reopen reason to persist, got %+v", reopenedThread.Thread)
	}
	if !hasThreadEventType(reopenedThread.Events, domain.PageCommentThreadEventTypeReopened) {
		t.Fatalf("expected reopened event, got %+v", reopenedThread.Events)
	}

	reason := domain.PageCommentThreadEventReasonDraftUpdated
	restoredRevisionID := rev.ID
	reevaluation := domain.ThreadAnchorReevaluationContext{Reason: reason, RevisionID: &restoredRevisionID}
	anchorChangedThread, err := threadRepo.UpdateThreadState(ctx, thirdThread.ID, domain.PageCommentThread{
		ID:             thirdThread.ID,
		PageID:         thirdThread.PageID,
		Anchor:         thirdThread.Anchor,
		ThreadState:    thirdThread.ThreadState,
		AnchorState:    domain.PageCommentThreadAnchorStateMissing,
		CreatedBy:      thirdThread.CreatedBy,
		CreatedAt:      thirdThread.CreatedAt,
		LastActivityAt: thirdThread.LastActivityAt,
	}, &reevaluation)
	if err != nil {
		t.Fatalf("anchor change thread state: %v", err)
	}
	if !hasThreadEventType(anchorChangedThread.Events, domain.PageCommentThreadEventTypeAnchorStateChanged) {
		t.Fatalf("expected anchor_state_changed event, got %+v", anchorChangedThread.Events)
	}
	if anchorChangedThread.Thread.AnchorState != domain.PageCommentThreadAnchorStateMissing {
		t.Fatalf("expected anchor state to persist as missing, got %+v", anchorChangedThread.Thread)
	}
	if !hasThreadEventReason(anchorChangedThread.Events, domain.PageCommentThreadEventReasonDraftUpdated) {
		t.Fatalf("expected draft_updated anchor event reason, got %+v", anchorChangedThread.Events)
	}
	if !hasEventRevisionID(anchorChangedThread.Events, rev.ID) {
		t.Fatalf("expected revision id on anchor change event, got %+v", anchorChangedThread.Events)
	}
	recoveredBlockID := uuid.NewString()
	reanchoredThread, err := threadRepo.UpdateThreadState(ctx, thirdThread.ID, domain.PageCommentThread{
		ID:     thirdThread.ID,
		PageID: thirdThread.PageID,
		Anchor: domain.PageCommentThreadAnchor{
			Type:            thirdThread.Anchor.Type,
			BlockID:         &recoveredBlockID,
			QuotedText:      thirdThread.Anchor.QuotedText,
			QuotedBlockText: thirdThread.Anchor.QuotedBlockText,
		},
		ThreadState:    anchorChangedThread.Thread.ThreadState,
		AnchorState:    anchorChangedThread.Thread.AnchorState,
		CreatedBy:      anchorChangedThread.Thread.CreatedBy,
		CreatedAt:      anchorChangedThread.Thread.CreatedAt,
		LastActivityAt: anchorChangedThread.Thread.LastActivityAt,
	}, &reevaluation)
	if err != nil {
		t.Fatalf("reanchor thread update: %v", err)
	}
	if reanchoredThread.Thread.Anchor.BlockID == nil || *reanchoredThread.Thread.Anchor.BlockID != recoveredBlockID {
		t.Fatalf("expected block_id update to persist, got %+v", reanchoredThread.Thread.Anchor.BlockID)
	}
	if !hasAnchorRecoveredEvent(reanchoredThread.Events, thirdBlockID, recoveredBlockID, reason) {
		t.Fatalf("expected anchor_recovered event with block ids, got %+v", reanchoredThread.Events)
	}
	if !hasEventRevisionID(reanchoredThread.Events, rev.ID) {
		t.Fatalf("expected revision id on anchor recovery event, got %+v", reanchoredThread.Events)
	}
	staleReopenReason := "stale reopen metadata"
	staleReopenAt := now.Add(7 * time.Minute)
	staleReplyMessage := domain.PageCommentThreadMessage{
		ID:        uuid.NewString(),
		ThreadID:  thread.ID,
		Body:      "Reply after stale reopen snapshot",
		CreatedBy: member.ID,
		CreatedAt: staleReopenAt,
	}
	staleReplyOutbox, err := domain.NewThreadReplyCreatedOutboxEvent(thread, staleReplyMessage, workspace.ID, nil)
	if err != nil {
		t.Fatalf("build stale reply outbox event: %v", err)
	}
	staleReopenThread := domain.PageCommentThread{
		ID:             thread.ID,
		PageID:         thread.PageID,
		Anchor:         thread.Anchor,
		ThreadState:    domain.PageCommentThreadStateOpen,
		AnchorState:    thread.AnchorState,
		CreatedBy:      thread.CreatedBy,
		CreatedAt:      thread.CreatedAt,
		ReopenedBy:     &owner.ID,
		ReopenedAt:     &staleReopenAt,
		ReopenReason:   &staleReopenReason,
		LastActivityAt: staleReopenAt,
		ReplyCount:     loadedThread.Thread.ReplyCount + 1,
	}
	staleReplyDetail, err := threadRepo.AddReply(ctx, thread.ID, staleReplyMessage, nil, staleReopenThread, staleReplyOutbox)
	if err != nil {
		t.Fatalf("add reply with stale reopen metadata: %v", err)
	}
	if staleReplyDetail.Thread.ReopenedBy != nil || staleReplyDetail.Thread.ReopenedAt != nil || staleReplyDetail.Thread.ReopenReason != nil {
		t.Fatalf("expected stale reopen metadata to be ignored for open thread, got %+v", staleReplyDetail.Thread)
	}
	mismatchedReplyMessage := domain.PageCommentThreadMessage{
		ID:        uuid.NewString(),
		ThreadID:  uuid.NewString(),
		Body:      "Wrong reply thread id",
		CreatedBy: member.ID,
		CreatedAt: now.Add(8 * time.Minute),
	}
	mismatchedReplyEvent, err := domain.NewThreadReplyCreatedOutboxEvent(thread, domain.PageCommentThreadMessage{
		ID:        mismatchedReplyMessage.ID,
		ThreadID:  thread.ID,
		Body:      mismatchedReplyMessage.Body,
		CreatedBy: mismatchedReplyMessage.CreatedBy,
		CreatedAt: mismatchedReplyMessage.CreatedAt,
	}, workspace.ID, nil)
	if err != nil {
		t.Fatalf("build mismatched reply outbox event: %v", err)
	}
	if _, err := threadRepo.AddReply(ctx, thread.ID, mismatchedReplyMessage, nil, staleReopenThread, mismatchedReplyEvent); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("expected mismatched reply thread id validation error, got %v", err)
	}
	if _, err := threadRepo.UpdateThreadState(ctx, uuid.NewString(), domain.PageCommentThread{
		ID:             uuid.NewString(),
		PageID:         secondThread.PageID,
		Anchor:         secondThread.Anchor,
		ThreadState:    domain.PageCommentThreadStateResolved,
		AnchorState:    secondThread.AnchorState,
		CreatedBy:      secondThread.CreatedBy,
		CreatedAt:      secondThread.CreatedAt,
		ResolvedBy:     &owner.ID,
		ResolvedAt:     &resolveTime,
		LastActivityAt: resolveTime,
	}, nil); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected update thread state not found, got %v", err)
	}

	notifRepo := NewNotificationRepository(pool)
	commentResourceType := domain.NotificationResourceTypePageComment
	notif := domain.Notification{
		ID:           uuid.NewString(),
		UserID:       member.ID,
		WorkspaceID:  workspace.ID,
		Type:         domain.NotificationTypeComment,
		EventID:      comment.ID,
		Message:      "commented",
		ActorID:      &owner.ID,
		Title:        "Comment activity",
		Content:      "commented",
		ResourceType: &commentResourceType,
		ResourceID:   &comment.ID,
		CreatedAt:    now,
	}
	createdNotif, err := notifRepo.Create(ctx, notif)
	if err != nil {
		t.Fatalf("create notification: %v", err)
	}
	if createdNotif.Title != "Comment activity" || createdNotif.Content != "commented" {
		t.Fatalf("expected comment notification v2 content fields, got %+v", createdNotif)
	}
	if createdNotif.IsRead || createdNotif.Actionable {
		t.Fatalf("expected unread non-actionable notification, got %+v", createdNotif)
	}
	if createdNotif.ResourceType == nil || *createdNotif.ResourceType != domain.NotificationResourceTypePageComment || createdNotif.ResourceID == nil || *createdNotif.ResourceID != comment.ID {
		t.Fatalf("expected page_comment resource metadata, got %+v", createdNotif)
	}
	if createdNotif.UpdatedAt.IsZero() || !createdNotif.UpdatedAt.Equal(createdNotif.CreatedAt) {
		t.Fatalf("expected created notification updated_at to equal created_at, got %+v", createdNotif)
	}
	if unreadCount, err := notifRepo.GetUnreadCount(ctx, member.ID); err != nil {
		t.Fatalf("get unread count after create: %v", err)
	} else if unreadCount != 1 {
		t.Fatalf("expected unread count 1 after single create, got %d", unreadCount)
	}
	if unreadCount, err := notifRepo.GetUnreadCount(ctx, owner.ID); err != nil {
		t.Fatalf("get unread count for owner without counter row: %v", err)
	} else if unreadCount != 0 {
		t.Fatalf("expected unread count 0 without counter row, got %d", unreadCount)
	}
	if err := notifRepo.CreateMany(ctx, []domain.Notification{
		{ID: uuid.NewString(), UserID: owner.ID, WorkspaceID: workspace.ID, Type: domain.NotificationTypeComment, EventID: uuid.NewString(), Message: "batch owner", CreatedAt: now.Add(30 * time.Second)},
		{ID: uuid.NewString(), UserID: member.ID, WorkspaceID: workspace.ID, Type: domain.NotificationTypeInvitation, EventID: uuid.NewString(), Message: "batch member", CreatedAt: now.Add(31 * time.Second)},
	}); err != nil {
		t.Fatalf("create notifications batch: %v", err)
	}
	if unreadCount, err := notifRepo.GetUnreadCount(ctx, owner.ID); err != nil {
		t.Fatalf("get unread count for owner after batch: %v", err)
	} else if unreadCount != 1 {
		t.Fatalf("expected unread count 1 for owner after batch, got %d", unreadCount)
	}
	if unreadCount, err := notifRepo.GetUnreadCount(ctx, member.ID); err != nil {
		t.Fatalf("get unread count for member after batch: %v", err)
	} else if unreadCount != 2 {
		t.Fatalf("expected unread count 2 for member after batch, got %d", unreadCount)
	}
	threadMsgResourceType := domain.NotificationResourceTypeThreadMsg
	replyNotificationEventID := uuid.NewString()
	threadCreatedNotificationEventID := uuid.NewString()
	commentNotifications := []domain.Notification{
		{
			ID:           uuid.NewString(),
			UserID:       member.ID,
			WorkspaceID:  workspace.ID,
			Type:         domain.NotificationTypeComment,
			EventID:      replyNotificationEventID,
			Message:      "A relevant comment thread has a new reply",
			Title:        "New thread reply",
			Content:      "A relevant comment thread has a new reply",
			ResourceType: &threadMsgResourceType,
			ResourceID:   &replyNotificationEventID,
			Payload:      json.RawMessage(`{"thread_id":"thread-1","message_id":"message-1","page_id":"page-1","workspace_id":"workspace-1","event_topic":"thread_reply_created"}`),
			CreatedAt:    now.Add(32 * time.Second),
		},
		{
			ID:           uuid.NewString(),
			UserID:       member.ID,
			WorkspaceID:  workspace.ID,
			Type:         domain.NotificationTypeComment,
			EventID:      replyNotificationEventID,
			Message:      "duplicate reply notification",
			Title:        "New thread reply",
			Content:      "A relevant comment thread has a new reply",
			ResourceType: &threadMsgResourceType,
			ResourceID:   &replyNotificationEventID,
			Payload:      json.RawMessage(`{"thread_id":"thread-1","message_id":"message-1","page_id":"page-1","workspace_id":"workspace-1","event_topic":"thread_reply_created"}`),
			CreatedAt:    now.Add(33 * time.Second),
		},
		{
			ID:           uuid.NewString(),
			UserID:       owner.ID,
			WorkspaceID:  workspace.ID,
			Type:         domain.NotificationTypeComment,
			EventID:      threadCreatedNotificationEventID,
			Message:      "A new relevant comment thread was created",
			Title:        "New comment thread",
			Content:      "A new relevant comment thread was created",
			ResourceType: &threadMsgResourceType,
			ResourceID:   &threadCreatedNotificationEventID,
			Payload:      json.RawMessage(`{"thread_id":"thread-2","message_id":"message-2","page_id":"page-1","workspace_id":"workspace-1","event_topic":"thread_created"}`),
			CreatedAt:    now.Add(34 * time.Second),
		},
	}
	insertedCommentCount, err := notifRepo.CreateCommentNotifications(ctx, commentNotifications)
	if err != nil {
		t.Fatalf("create comment notifications batch: %v", err)
	}
	if insertedCommentCount != 2 {
		t.Fatalf("expected 2 newly inserted comment notifications, got %d", insertedCommentCount)
	}
	if unreadCount, err := notifRepo.GetUnreadCount(ctx, member.ID); err != nil {
		t.Fatalf("get unread count for member after comment batch: %v", err)
	} else if unreadCount != 3 {
		t.Fatalf("expected unread count 3 for member after comment batch, got %d", unreadCount)
	}
	if unreadCount, err := notifRepo.GetUnreadCount(ctx, owner.ID); err != nil {
		t.Fatalf("get unread count for owner after comment batch: %v", err)
	} else if unreadCount != 2 {
		t.Fatalf("expected unread count 2 for owner after comment batch, got %d", unreadCount)
	}
	if _, err := notifRepo.Create(ctx, domain.Notification{ID: uuid.NewString(), UserID: member.ID, WorkspaceID: workspace.ID, Type: domain.NotificationTypeComment, EventID: comment.ID, Message: "dup", CreatedAt: now}); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("expected notification conflict, got %v", err)
	}
	if unreadCount, err := notifRepo.GetUnreadCount(ctx, member.ID); err != nil {
		t.Fatalf("get unread count after duplicate create: %v", err)
	} else if unreadCount != 3 {
		t.Fatalf("expected unread count unchanged after duplicate create, got %d", unreadCount)
	}
	allNotifs, err := notifRepo.ListByUserID(ctx, member.ID)
	if err != nil || len(allNotifs) != 3 || allNotifs[2].ID != createdNotif.ID {
		t.Fatalf("list notifications mismatch: err=%v len=%d", err, len(allNotifs))
	}
	marked, err := notifRepo.MarkRead(ctx, createdNotif.ID, member.ID, now.Add(2*time.Minute))
	if err != nil || marked.ReadAt == nil {
		t.Fatalf("mark read mismatch: err=%v", err)
	}
	if !marked.IsRead || !marked.UpdatedAt.Equal(*marked.ReadAt) {
		t.Fatalf("expected mark read to update is_read and updated_at, got %+v", marked)
	}
	if unreadCount, err := notifRepo.GetUnreadCount(ctx, member.ID); err != nil {
		t.Fatalf("get unread count after first mark read: %v", err)
	} else if unreadCount != 2 {
		t.Fatalf("expected unread count 2 after first mark read, got %d", unreadCount)
	}
	if again, err := notifRepo.MarkRead(ctx, createdNotif.ID, member.ID, now.Add(3*time.Minute)); err != nil {
		t.Fatalf("mark read idempotent mismatch: %v", err)
	} else if again.ReadAt == nil || !again.ReadAt.Equal(*marked.ReadAt) || !again.UpdatedAt.Equal(marked.UpdatedAt) {
		t.Fatalf("expected idempotent mark read to preserve timestamps, got %+v", again)
	}
	if unreadCount, err := notifRepo.GetUnreadCount(ctx, member.ID); err != nil {
		t.Fatalf("get unread count after idempotent mark read: %v", err)
	} else if unreadCount != 2 {
		t.Fatalf("expected unread count unchanged after idempotent mark read, got %d", unreadCount)
	}
	if _, err := notifRepo.MarkRead(ctx, uuid.NewString(), member.ID, now); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected mark read not found, got %v", err)
	}
	inboxMarked, err := notifRepo.MarkRead(ctx, createdNotif.ID, member.ID, now.Add(4*time.Minute))
	if err != nil {
		t.Fatalf("mark read inbox projection: %v", err)
	}
	if !inboxMarked.IsRead || inboxMarked.ReadAt == nil || !inboxMarked.UpdatedAt.Equal(*inboxMarked.ReadAt) {
		t.Fatalf("expected inbox mark-read state transition, got %+v", inboxMarked)
	}
	if inboxMarked.Actor == nil || inboxMarked.Actor.ID != owner.ID {
		t.Fatalf("expected actor metadata on mark-read response, got %+v", inboxMarked)
	}
	if _, err := notifRepo.CreateCommentNotifications(ctx, []domain.Notification{{ID: uuid.NewString(), UserID: "", WorkspaceID: workspace.ID, Type: domain.NotificationTypeComment, EventID: replyNotificationEventID, Message: "x", ResourceType: &threadMsgResourceType, ResourceID: &replyNotificationEventID, CreatedAt: now}}); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("expected comment notification validation error for blank user_id, got %v", err)
	}
	if _, err := notifRepo.CreateCommentNotifications(ctx, []domain.Notification{{ID: uuid.NewString(), UserID: member.ID, WorkspaceID: workspace.ID, Type: domain.NotificationTypeComment, EventID: "", Message: "x", ResourceType: &threadMsgResourceType, ResourceID: &replyNotificationEventID, CreatedAt: now}}); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("expected comment notification validation error for blank event_id, got %v", err)
	}
	if _, err := notifRepo.CreateCommentNotifications(ctx, []domain.Notification{{ID: uuid.NewString(), UserID: member.ID, WorkspaceID: workspace.ID, Type: domain.NotificationTypeComment, EventID: replyNotificationEventID, Message: "x", ResourceType: &threadMsgResourceType, ResourceID: &replyNotificationEventID, Payload: json.RawMessage(`[]`), CreatedAt: now}}); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("expected comment notification validation error for non-object payload, got %v", err)
	}
	inboxAgain, err := notifRepo.MarkRead(ctx, createdNotif.ID, member.ID, now.Add(5*time.Minute))
	if err != nil {
		t.Fatalf("mark read inbox idempotent projection: %v", err)
	}
	if inboxAgain.ReadAt == nil || !inboxAgain.ReadAt.Equal(*inboxMarked.ReadAt) || !inboxAgain.UpdatedAt.Equal(inboxMarked.UpdatedAt) {
		t.Fatalf("expected mark-read idempotency preservation, got %+v", inboxAgain)
	}
	if _, err := notifRepo.MarkRead(ctx, createdNotif.ID, owner.ID, now.Add(6*time.Minute)); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected mark read foreign owner not found, got %v", err)
	}
	unreadNotificationIDs := make([]string, 0, 2)
	for _, notification := range allNotifs {
		if notification.ID != createdNotif.ID {
			unreadNotificationIDs = append(unreadNotificationIDs, notification.ID)
		}
	}
	batchResult, err := notifRepo.BatchMarkRead(ctx, member.ID, unreadNotificationIDs, now.Add(7*time.Minute))
	if err != nil {
		t.Fatalf("batch mark read: %v", err)
	}
	if batchResult.UpdatedCount != 2 || batchResult.UnreadCount != 0 {
		t.Fatalf("expected batch mark-read result to update the unread row once, got %+v", batchResult)
	}
	repeatBatch, err := notifRepo.BatchMarkRead(ctx, member.ID, unreadNotificationIDs, now.Add(8*time.Minute))
	if err != nil {
		t.Fatalf("repeat batch mark read: %v", err)
	}
	if repeatBatch.UpdatedCount != 0 || repeatBatch.UnreadCount != 0 {
		t.Fatalf("expected repeat batch mark-read to be idempotent, got %+v", repeatBatch)
	}
	if _, err := notifRepo.BatchMarkRead(ctx, member.ID, []string{createdNotif.ID, uuid.NewString()}, now.Add(9*time.Minute)); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected batch mark read missing notification not found, got %v", err)
	}

	invitationResourceType := domain.NotificationResourceTypeInvitation
	invitationActionKind := domain.NotificationActionKindInvitationResponse
	invitationID := uuid.NewString()
	createdAt := now.Add(4 * time.Minute)
	liveCreate, err := notifRepo.UpsertInvitationLive(ctx, domain.Notification{
		ID:           uuid.NewString(),
		UserID:       member.ID,
		WorkspaceID:  workspace.ID,
		Type:         domain.NotificationTypeInvitation,
		EventID:      invitationID,
		Message:      "You have a new workspace invitation",
		ActorID:      &owner.ID,
		Title:        "Workspace invitation",
		Content:      "You have a new workspace invitation",
		Actionable:   true,
		ActionKind:   &invitationActionKind,
		ResourceType: &invitationResourceType,
		ResourceID:   &invitationID,
		Payload:      json.RawMessage(`{"invitation_id":"` + invitationID + `","workspace_id":"` + workspace.ID + `","email":"` + member.Email + `","role":"viewer","status":"pending","version":1,"can_accept":true,"can_reject":true}`),
		CreatedAt:    createdAt,
		UpdatedAt:    createdAt,
	})
	if err != nil {
		t.Fatalf("upsert invitation live create: %v", err)
	}
	if liveCreate.ID == "" || liveCreate.CreatedAt.IsZero() || !liveCreate.CreatedAt.Equal(createdAt) || liveCreate.IsRead {
		t.Fatalf("expected unread created live invitation row, got %+v", liveCreate)
	}
	if unreadCount, err := notifRepo.GetUnreadCount(ctx, member.ID); err != nil {
		t.Fatalf("get unread count after invitation live create: %v", err)
	} else if unreadCount != 1 {
		t.Fatalf("expected unread count 1 after invitation live create, got %d", unreadCount)
	}

	readAt := now.Add(5 * time.Minute)
	markedLive, err := notifRepo.MarkRead(ctx, liveCreate.ID, member.ID, readAt)
	if err != nil {
		t.Fatalf("mark live invitation read: %v", err)
	}
	var markedPayload map[string]any
	if err := json.Unmarshal(markedLive.Payload, &markedPayload); err != nil {
		t.Fatalf("unmarshal marked invitation payload: %v", err)
	}
	if markedPayload["invitation_id"] != invitationID || markedPayload["workspace_id"] != workspace.ID || markedPayload["email"] != member.Email || markedPayload["role"] != string(domain.RoleViewer) {
		t.Fatalf("expected mark-read to preserve invitation payload fields, got %+v", markedPayload)
	}
	if unreadCount, err := notifRepo.GetUnreadCount(ctx, member.ID); err != nil {
		t.Fatalf("get unread count after live invitation read: %v", err)
	} else if unreadCount != 0 {
		t.Fatalf("expected unread count 0 after live invitation read, got %d", unreadCount)
	}

	liveUpdate, err := notifRepo.UpsertInvitationLive(ctx, domain.Notification{
		ID:           uuid.NewString(),
		UserID:       member.ID,
		WorkspaceID:  workspace.ID,
		Type:         domain.NotificationTypeInvitation,
		EventID:      invitationID,
		Message:      "You accepted the workspace invitation",
		ActorID:      &owner.ID,
		Title:        "Invitation accepted",
		Content:      "You accepted the workspace invitation",
		Actionable:   false,
		ResourceType: &invitationResourceType,
		ResourceID:   &invitationID,
		Payload:      json.RawMessage(`{"invitation_id":"` + invitationID + `","workspace_id":"` + workspace.ID + `","email":"` + member.Email + `","role":"viewer","status":"accepted","version":2,"can_accept":false,"can_reject":false}`),
		CreatedAt:    now.Add(6 * time.Minute),
		UpdatedAt:    now.Add(6 * time.Minute),
	})
	if err != nil {
		t.Fatalf("upsert invitation live update: %v", err)
	}
	if liveUpdate.ID != liveCreate.ID || !liveUpdate.CreatedAt.Equal(liveCreate.CreatedAt) {
		t.Fatalf("expected same live row identity, got create=%+v update=%+v", liveCreate, liveUpdate)
	}
	if !liveUpdate.IsRead || liveUpdate.ReadAt == nil || !liveUpdate.ReadAt.Equal(readAt) {
		t.Fatalf("expected preserved read state, got %+v", liveUpdate)
	}
	if liveUpdate.Actionable || liveUpdate.ActionKind != nil || liveUpdate.Title != "Invitation accepted" || liveUpdate.Message != "You accepted the workspace invitation" {
		t.Fatalf("expected mutable invitation fields updated in place, got %+v", liveUpdate)
	}

	if _, err := notifRepo.UpsertInvitationLive(ctx, domain.Notification{
		ID:           uuid.NewString(),
		UserID:       member.ID,
		WorkspaceID:  workspace.ID,
		Type:         domain.NotificationTypeInvitation,
		EventID:      invitationID,
		Message:      "You accepted the workspace invitation",
		ActorID:      &owner.ID,
		Title:        "Invitation accepted",
		Content:      "You accepted the workspace invitation",
		Actionable:   false,
		ResourceType: &invitationResourceType,
		ResourceID:   &invitationID,
		Payload:      json.RawMessage(`{"invitation_id":"` + invitationID + `","workspace_id":"` + workspace.ID + `","email":"` + member.Email + `","role":"viewer","status":"accepted","version":2,"can_accept":false,"can_reject":false}`),
		CreatedAt:    now.Add(7 * time.Minute),
		UpdatedAt:    now.Add(7 * time.Minute),
	}); err != nil {
		t.Fatalf("repeat upsert invitation live: %v", err)
	}
	if unreadCount, err := notifRepo.GetUnreadCount(ctx, member.ID); err != nil {
		t.Fatalf("get unread count after invitation live updates: %v", err)
	} else if unreadCount != 0 {
		t.Fatalf("expected unread count unchanged after invitation live updates, got %d", unreadCount)
	}
	memberNotifs, err := notifRepo.ListByUserID(ctx, member.ID)
	if err != nil {
		t.Fatalf("list notifications after invitation live upsert: %v", err)
	}
	liveCount := 0
	for _, notification := range memberNotifs {
		if notification.Type == domain.NotificationTypeInvitation && notification.ResourceType != nil && *notification.ResourceType == domain.NotificationResourceTypeInvitation && notification.ResourceID != nil && *notification.ResourceID == invitationID {
			liveCount++
		}
	}
	if liveCount != 1 {
		t.Fatalf("expected one live invitation row, got %d", liveCount)
	}

	inboxPage, err := notifRepo.ListInbox(ctx, member.ID, domain.NotificationInboxFilter{
		Status: domain.NotificationInboxStatusAll,
		Type:   domain.NotificationInboxTypeAll,
		Limit:  2,
	})
	if err != nil {
		t.Fatalf("list notification inbox: %v", err)
	}
	if len(inboxPage.Items) != 2 || inboxPage.Items[0].ID != liveCreate.ID {
		t.Fatalf("expected newest-first inbox page, got %+v", inboxPage)
	}
	if inboxPage.UnreadCount != 0 {
		t.Fatalf("expected total unread_count=0, got %+v", inboxPage)
	}
	if !inboxPage.HasMore || inboxPage.NextCursor == nil {
		t.Fatalf("expected inbox pagination metadata, got %+v", inboxPage)
	}
	if inboxPage.Items[0].Actor == nil || inboxPage.Items[0].Actor.ID != owner.ID {
		t.Fatalf("expected actor metadata join, got %+v", inboxPage.Items[0])
	}

	unreadPage, err := notifRepo.ListInbox(ctx, member.ID, domain.NotificationInboxFilter{
		Status: domain.NotificationInboxStatusUnread,
		Type:   domain.NotificationInboxTypeAll,
		Limit:  10,
	})
	if err != nil {
		t.Fatalf("list unread notification inbox: %v", err)
	}
	if len(unreadPage.Items) != 0 {
		t.Fatalf("expected no unread rows after batch and live invitation read, got %+v", unreadPage.Items)
	}

	invitationPage, err := notifRepo.ListInbox(ctx, member.ID, domain.NotificationInboxFilter{
		Status: domain.NotificationInboxStatusAll,
		Type:   domain.NotificationInboxTypeInvitation,
		Limit:  10,
	})
	if err != nil {
		t.Fatalf("list invitation notification inbox: %v", err)
	}
	liveInvitationCount := 0
	for _, item := range invitationPage.Items {
		if item.ResourceType != nil && *item.ResourceType == domain.NotificationResourceTypeInvitation && item.ResourceID != nil && *item.ResourceID == invitationID {
			liveInvitationCount++
		}
	}
	if liveInvitationCount != 1 {
		t.Fatalf("expected one invitation live row, got %+v", invitationPage.Items)
	}

	firstPage, err := notifRepo.ListInbox(ctx, member.ID, domain.NotificationInboxFilter{
		Status: domain.NotificationInboxStatusAll,
		Type:   domain.NotificationInboxTypeAll,
		Limit:  1,
	})
	if err != nil {
		t.Fatalf("list first notification inbox page: %v", err)
	}
	if !firstPage.HasMore || firstPage.NextCursor == nil || len(firstPage.Items) != 1 {
		t.Fatalf("expected first inbox page cursor, got %+v", firstPage)
	}
	secondPage, err := notifRepo.ListInbox(ctx, member.ID, domain.NotificationInboxFilter{
		Status: domain.NotificationInboxStatusAll,
		Type:   domain.NotificationInboxTypeAll,
		Limit:  1,
		Cursor: *firstPage.NextCursor,
	})
	if err != nil {
		t.Fatalf("list second notification inbox page: %v", err)
	}
	if len(secondPage.Items) != 1 || secondPage.Items[0].ID == firstPage.Items[0].ID {
		t.Fatalf("expected second inbox page after cursor, got %+v", secondPage)
	}
	if _, err := notifRepo.ListInbox(ctx, member.ID, domain.NotificationInboxFilter{
		Status: domain.NotificationInboxStatusUnread,
		Type:   domain.NotificationInboxTypeInvitation,
		Limit:  1,
		Cursor: *firstPage.NextCursor,
	}); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("expected cursor filter mismatch validation error, got %v", err)
	}
}

func TestPageCommentMessageMentionSchemaIntegration(t *testing.T) {
	pool := integrationPool(t)
	ctx := context.Background()

	owner := seedUser(t, pool, "mention-owner@example.com")
	mentionedA := seedUser(t, pool, "mention-a@example.com")
	mentionedB := seedUser(t, pool, "mention-b@example.com")
	workspace, _ := seedWorkspaceWithOwner(t, pool, owner)
	page, _ := seedPageWithDraft(t, pool, workspace.ID, owner.ID, nil, "Mention Schema Page")
	threadRepo := NewThreadRepository(pool)
	now := time.Now().UTC().Truncate(time.Microsecond)

	createThreadMessage := func(suffix string, createdAt time.Time) domain.PageCommentThreadMessage {
		blockID := "block-" + suffix
		thread := domain.PageCommentThread{
			ID:     uuid.NewString(),
			PageID: page.ID,
			Anchor: domain.PageCommentThreadAnchor{
				Type:            domain.PageCommentThreadAnchorTypeBlock,
				BlockID:         &blockID,
				QuotedBlockText: "mention schema " + suffix,
			},
			ThreadState:    domain.PageCommentThreadStateOpen,
			AnchorState:    domain.PageCommentThreadAnchorStateActive,
			CreatedBy:      owner.ID,
			CreatedAt:      createdAt,
			LastActivityAt: createdAt,
			ReplyCount:     1,
		}
		message := domain.PageCommentThreadMessage{
			ID:        uuid.NewString(),
			ThreadID:  thread.ID,
			Body:      "Mention target " + suffix,
			CreatedBy: owner.ID,
			CreatedAt: createdAt,
		}
		outboxEvent, err := domain.NewThreadCreatedOutboxEvent(thread, message, workspace.ID, nil)
		if err != nil {
			t.Fatalf("build thread outbox event: %v", err)
		}
		createdThread, err := threadRepo.CreateThread(ctx, thread, message, nil, outboxEvent)
		if err != nil {
			t.Fatalf("create thread %s: %v", suffix, err)
		}
		if len(createdThread.Messages) != 1 {
			t.Fatalf("expected one starter message, got %+v", createdThread.Messages)
		}
		return createdThread.Messages[0]
	}

	firstMessage := createThreadMessage("one", now)
	secondMessage := createThreadMessage("two", now.Add(time.Minute))

	var tableExists bool
	if err := pool.QueryRow(ctx, `SELECT to_regclass('public.page_comment_message_mentions') IS NOT NULL`).Scan(&tableExists); err != nil {
		t.Fatalf("check mention table existence: %v", err)
	}
	if !tableExists {
		t.Fatal("expected page_comment_message_mentions table to exist")
	}

	var pkCount int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM pg_constraint c
		JOIN pg_class t ON t.oid = c.conrelid
		JOIN pg_namespace n ON n.oid = t.relnamespace
		WHERE n.nspname = 'public'
		  AND t.relname = 'page_comment_message_mentions'
		  AND c.contype = 'p'
	`).Scan(&pkCount); err != nil {
		t.Fatalf("check mention primary key existence: %v", err)
	}
	if pkCount != 1 {
		t.Fatalf("expected one primary key constraint on mention table, got %d", pkCount)
	}

	var indexExists bool
	if err := pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM pg_indexes
			WHERE schemaname = 'public'
			  AND tablename = 'page_comment_message_mentions'
			  AND indexname = 'page_comment_message_mentions_user_message_idx'
		)
	`).Scan(&indexExists); err != nil {
		t.Fatalf("check mention index existence: %v", err)
	}
	if !indexExists {
		t.Fatal("expected page_comment_message_mentions_user_message_idx to exist")
	}

	mustExec(t, pool, `
		INSERT INTO page_comment_message_mentions (message_id, mentioned_user_id)
		VALUES ($1, $2)
	`, firstMessage.ID, mentionedA.ID)
	mustExec(t, pool, `
		INSERT INTO page_comment_message_mentions (message_id, mentioned_user_id)
		VALUES ($1, $2)
	`, firstMessage.ID, mentionedB.ID)
	mustExec(t, pool, `
		INSERT INTO page_comment_message_mentions (message_id, mentioned_user_id)
		VALUES ($1, $2)
	`, secondMessage.ID, mentionedA.ID)

	var mentionCount int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM page_comment_message_mentions WHERE message_id = $1`, firstMessage.ID).Scan(&mentionCount); err != nil {
		t.Fatalf("count mentions for first message: %v", err)
	}
	if mentionCount != 2 {
		t.Fatalf("expected two mentions on first message, got %d", mentionCount)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM page_comment_message_mentions WHERE mentioned_user_id = $1`, mentionedA.ID).Scan(&mentionCount); err != nil {
		t.Fatalf("count mentions for first user across messages: %v", err)
	}
	if mentionCount != 2 {
		t.Fatalf("expected same user to be mentioned in two messages, got %d", mentionCount)
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO page_comment_message_mentions (message_id, mentioned_user_id)
		VALUES ($1, $2)
	`, firstMessage.ID, mentionedA.ID); err == nil || !isUniqueViolation(err) {
		t.Fatalf("expected duplicate mention rejection, got %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO page_comment_message_mentions (message_id, mentioned_user_id)
		VALUES ($1, $2)
	`, uuid.NewString(), mentionedA.ID); err == nil {
		t.Fatal("expected missing message foreign key failure")
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO page_comment_message_mentions (message_id, mentioned_user_id)
		VALUES ($1, $2)
	`, firstMessage.ID, uuid.NewString()); err == nil {
		t.Fatal("expected missing user foreign key failure")
	}
	if _, err := pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, mentionedA.ID); err == nil {
		t.Fatal("expected deleting a mentioned user to be restricted")
	}

	mustExec(t, pool, `DELETE FROM page_comment_messages WHERE id = $1`, firstMessage.ID)
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM page_comment_message_mentions WHERE message_id = $1`, firstMessage.ID).Scan(&mentionCount); err != nil {
		t.Fatalf("count mentions after cascade delete: %v", err)
	}
	if mentionCount != 0 {
		t.Fatalf("expected cascade delete to remove mention rows, got %d", mentionCount)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM page_comment_message_mentions WHERE message_id = $1`, secondMessage.ID).Scan(&mentionCount); err != nil {
		t.Fatalf("count mentions for remaining message: %v", err)
	}
	if mentionCount != 1 {
		t.Fatalf("expected remaining message mentions to stay intact, got %d", mentionCount)
	}
}

func TestNotificationRepositoryMentionNotificationsIntegration(t *testing.T) {
	pool := integrationPool(t)
	ctx := context.Background()

	owner := seedUser(t, pool, "mention-notif-owner@example.com")
	member := seedUser(t, pool, "mention-notif-member@example.com")
	workspace, _ := seedWorkspaceWithOwner(t, pool, owner)
	if _, err := pool.Exec(ctx, `INSERT INTO workspace_members (id, workspace_id, user_id, role, created_at) VALUES ($1, $2, $3, $4, $5)`, uuid.NewString(), workspace.ID, member.ID, domain.RoleEditor, time.Now().UTC()); err != nil {
		t.Fatalf("seed member membership: %v", err)
	}

	notifRepo := NewNotificationRepository(pool)
	threadMsgResourceType := domain.NotificationResourceTypeThreadMsg
	messageID := uuid.NewString()
	now := time.Now().UTC().Truncate(time.Microsecond)

	commentNotification := domain.Notification{
		ID:           uuid.NewString(),
		UserID:       member.ID,
		WorkspaceID:  workspace.ID,
		Type:         domain.NotificationTypeComment,
		EventID:      messageID,
		Message:      "A relevant comment thread has a new reply",
		Title:        "New thread reply",
		Content:      "A relevant comment thread has a new reply",
		ResourceType: &threadMsgResourceType,
		ResourceID:   &messageID,
		Payload:      json.RawMessage(`{"thread_id":"thread-1","message_id":"message-1","page_id":"page-1","workspace_id":"workspace-1","event_topic":"thread_reply_created"}`),
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if _, err := notifRepo.CreateCommentNotifications(ctx, []domain.Notification{commentNotification}); err != nil {
		t.Fatalf("create comment notification: %v", err)
	}

	mentionNotification := domain.Notification{
		ID:           uuid.NewString(),
		UserID:       member.ID,
		WorkspaceID:  workspace.ID,
		Type:         domain.NotificationTypeMention,
		EventID:      messageID,
		Message:      "You were mentioned in a thread reply",
		Title:        "Mentioned in a thread reply",
		Content:      "You were mentioned in a thread reply",
		ActorID:      &owner.ID,
		ResourceType: &threadMsgResourceType,
		ResourceID:   &messageID,
		Payload:      json.RawMessage(`{"thread_id":"thread-1","message_id":"message-1","page_id":"page-1","workspace_id":"workspace-1","event_topic":"thread_reply_created","mention_source":"explicit"}`),
		CreatedAt:    now.Add(time.Second),
		UpdatedAt:    now.Add(time.Second),
	}
	otherMention := domain.Notification{
		ID:           uuid.NewString(),
		UserID:       owner.ID,
		WorkspaceID:  workspace.ID,
		Type:         domain.NotificationTypeMention,
		EventID:      messageID,
		Message:      "You were mentioned in a thread reply",
		Title:        "Mentioned in a thread reply",
		Content:      "You were mentioned in a thread reply",
		ActorID:      &member.ID,
		ResourceType: &threadMsgResourceType,
		ResourceID:   &messageID,
		Payload:      json.RawMessage(`{"thread_id":"thread-1","message_id":"message-1","page_id":"page-1","workspace_id":"workspace-1","event_topic":"thread_reply_created","mention_source":"explicit"}`),
		CreatedAt:    now.Add(2 * time.Second),
		UpdatedAt:    now.Add(2 * time.Second),
	}

	insertedMentionCount, err := notifRepo.CreateMentionNotifications(ctx, []domain.Notification{mentionNotification, otherMention, mentionNotification})
	if err != nil {
		t.Fatalf("create mention notifications batch: %v", err)
	}
	if insertedMentionCount != 2 {
		t.Fatalf("expected 2 newly inserted mention notifications, got %d", insertedMentionCount)
	}

	if unreadCount, err := notifRepo.GetUnreadCount(ctx, member.ID); err != nil {
		t.Fatalf("get unread count after mention create: %v", err)
	} else if unreadCount != 2 {
		t.Fatalf("expected unread count 2 after comment and mention rows for member, got %d", unreadCount)
	}
	if unreadCount, err := notifRepo.GetUnreadCount(ctx, owner.ID); err != nil {
		t.Fatalf("get unread count after owner mention create: %v", err)
	} else if unreadCount != 1 {
		t.Fatalf("expected unread count 1 after owner mention row, got %d", unreadCount)
	}

	retryCount, err := notifRepo.CreateMentionNotifications(ctx, []domain.Notification{mentionNotification, otherMention})
	if err != nil {
		t.Fatalf("repeat mention notifications batch: %v", err)
	}
	if retryCount != 0 {
		t.Fatalf("expected zero newly inserted mention notifications on retry, got %d", retryCount)
	}

	memberNotifs, err := notifRepo.ListByUserID(ctx, member.ID)
	if err != nil {
		t.Fatalf("list member notifications after mention create: %v", err)
	}
	hasComment := false
	hasMention := false
	for _, notification := range memberNotifs {
		switch notification.Type {
		case domain.NotificationTypeComment:
			if notification.EventID == messageID {
				hasComment = true
			}
		case domain.NotificationTypeMention:
			if notification.EventID == messageID && notification.ResourceType != nil && *notification.ResourceType == domain.NotificationResourceTypeThreadMsg {
				hasMention = true
				if notification.Payload == nil || !json.Valid(notification.Payload) {
					t.Fatalf("expected mention payload JSON, got %+v", notification)
				}
			}
		}
	}
	if !hasComment || !hasMention {
		t.Fatalf("expected both comment and mention rows for the same message, got %+v", memberNotifs)
	}

	if _, err := notifRepo.CreateMentionNotifications(ctx, []domain.Notification{{ID: uuid.NewString(), UserID: "", WorkspaceID: workspace.ID, Type: domain.NotificationTypeMention, EventID: messageID, Message: "x", ResourceType: &threadMsgResourceType, ResourceID: &messageID, CreatedAt: now}}); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("expected mention notification validation error for blank user_id, got %v", err)
	}
	if _, err := notifRepo.CreateMentionNotifications(ctx, []domain.Notification{{ID: uuid.NewString(), UserID: member.ID, WorkspaceID: workspace.ID, Type: domain.NotificationTypeMention, EventID: "", Message: "x", ResourceType: &threadMsgResourceType, ResourceID: &messageID, CreatedAt: now}}); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("expected mention notification validation error for blank event_id, got %v", err)
	}
}

type capturingNotificationStreamPublisher struct {
	signals []domain.NotificationStreamSignal
	err     error
}

func (p *capturingNotificationStreamPublisher) Publish(_ context.Context, signal domain.NotificationStreamSignal) error {
	p.signals = append(p.signals, signal)
	return p.err
}

func TestNotificationRepositoryPublishesStreamInvalidationsIntegration(t *testing.T) {
	pool := integrationPool(t)
	ctx := context.Background()

	owner := seedUser(t, pool, "stream-owner@example.com")
	member := seedUser(t, pool, "stream-member@example.com")
	workspace, _ := seedWorkspaceWithOwner(t, pool, owner)
	now := time.Now().UTC().Truncate(time.Microsecond)

	publisher := &capturingNotificationStreamPublisher{}
	notifRepo := NewNotificationRepository(pool).WithStreamPublisher(publisher)

	resourceType := domain.NotificationResourceTypePageComment
	created, err := notifRepo.Create(ctx, domain.Notification{
		ID:           uuid.NewString(),
		UserID:       member.ID,
		WorkspaceID:  workspace.ID,
		Type:         domain.NotificationTypeComment,
		EventID:      uuid.NewString(),
		Message:      "stream create",
		ResourceType: &resourceType,
		ResourceID:   func() *string { v := uuid.NewString(); return &v }(),
		CreatedAt:    now,
	})
	if err != nil {
		t.Fatalf("create notification: %v", err)
	}
	if created.ID == "" {
		t.Fatal("expected created notification ID")
	}
	if len(publisher.signals) != 1 {
		t.Fatalf("expected one publish signal, got %d", len(publisher.signals))
	}
	if publisher.signals[0].UserID != member.ID {
		t.Fatalf("unexpected publish user: %+v", publisher.signals[0])
	}

	if _, err := notifRepo.Create(ctx, domain.Notification{
		ID:           uuid.NewString(),
		UserID:       member.ID,
		WorkspaceID:  workspace.ID,
		Type:         domain.NotificationTypeComment,
		EventID:      created.EventID,
		Message:      "duplicate stream create",
		ResourceType: &resourceType,
		ResourceID:   func() *string { v := uuid.NewString(); return &v }(),
		CreatedAt:    now.Add(time.Second),
	}); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("expected duplicate create conflict, got %v", err)
	}
	if len(publisher.signals) != 1 {
		t.Fatalf("expected no publish on conflict, got %d signals", len(publisher.signals))
	}

	marked, err := notifRepo.MarkRead(ctx, created.ID, member.ID, now.Add(2*time.Second))
	if err != nil {
		t.Fatalf("mark read: %v", err)
	}
	if !marked.IsRead {
		t.Fatal("expected marked notification to be read")
	}
	if len(publisher.signals) != 2 {
		t.Fatalf("expected publish on first read transition, got %d signals", len(publisher.signals))
	}

	if _, err := notifRepo.MarkRead(ctx, created.ID, member.ID, now.Add(3*time.Second)); err != nil {
		t.Fatalf("repeat mark read: %v", err)
	}
	if len(publisher.signals) != 2 {
		t.Fatalf("expected no publish on repeated mark read, got %d signals", len(publisher.signals))
	}

	batchPublisher := &capturingNotificationStreamPublisher{}
	batchRepo := NewNotificationRepository(pool).WithStreamPublisher(batchPublisher)
	second, err := batchRepo.Create(ctx, domain.Notification{
		ID:           uuid.NewString(),
		UserID:       member.ID,
		WorkspaceID:  workspace.ID,
		Type:         domain.NotificationTypeComment,
		EventID:      uuid.NewString(),
		Message:      "batch one",
		ResourceType: &resourceType,
		ResourceID:   func() *string { v := uuid.NewString(); return &v }(),
		CreatedAt:    now.Add(4 * time.Second),
	})
	if err != nil {
		t.Fatalf("create batch notification: %v", err)
	}
	otherUserNotif, err := batchRepo.Create(ctx, domain.Notification{
		ID:           uuid.NewString(),
		UserID:       owner.ID,
		WorkspaceID:  workspace.ID,
		Type:         domain.NotificationTypeComment,
		EventID:      uuid.NewString(),
		Message:      "batch two",
		ResourceType: &resourceType,
		ResourceID:   func() *string { v := uuid.NewString(); return &v }(),
		CreatedAt:    now.Add(5 * time.Second),
	})
	if err != nil {
		t.Fatalf("create second batch notification: %v", err)
	}

	if _, err := batchRepo.BatchMarkRead(ctx, member.ID, []string{second.ID}, now.Add(6*time.Second)); err != nil {
		t.Fatalf("batch mark read: %v", err)
	}
	if len(batchPublisher.signals) != 3 {
		t.Fatalf("expected publish after batch read transition, got %d signals", len(batchPublisher.signals))
	}

	if _, err := batchRepo.BatchMarkRead(ctx, member.ID, []string{second.ID}, now.Add(7*time.Second)); err != nil {
		t.Fatalf("repeat batch mark read: %v", err)
	}
	if len(batchPublisher.signals) != 3 {
		t.Fatalf("expected no publish on repeat batch mark read, got %d signals", len(batchPublisher.signals))
	}

	failingPublisher := &capturingNotificationStreamPublisher{err: errors.New("publish failed")}
	failingRepo := NewNotificationRepository(pool).WithStreamPublisher(failingPublisher)
	_, err = failingRepo.Create(ctx, domain.Notification{
		ID:           uuid.NewString(),
		UserID:       owner.ID,
		WorkspaceID:  workspace.ID,
		Type:         domain.NotificationTypeComment,
		EventID:      uuid.NewString(),
		Message:      "publish failure should not fail write",
		ResourceType: &resourceType,
		ResourceID:   &otherUserNotif.ID,
		CreatedAt:    now.Add(8 * time.Second),
	})
	if err != nil {
		t.Fatalf("expected publish failure to stay best-effort, got %v", err)
	}
}

func TestNotificationRepositoryCombinedCommentMentionNotificationsAtomicityIntegration(t *testing.T) {
	pool := integrationPool(t)
	ctx := context.Background()

	owner := seedUser(t, pool, "atomic-comment-owner@example.com")
	member := seedUser(t, pool, "atomic-comment-member@example.com")
	workspace, _ := seedWorkspaceWithOwner(t, pool, owner)
	if _, err := pool.Exec(ctx, `INSERT INTO workspace_members (id, workspace_id, user_id, role, created_at) VALUES ($1, $2, $3, $4, $5)`, uuid.NewString(), workspace.ID, member.ID, domain.RoleEditor, time.Now().UTC()); err != nil {
		t.Fatalf("seed member membership: %v", err)
	}

	notifRepo := NewNotificationRepository(pool)
	threadMsgResourceType := domain.NotificationResourceTypeThreadMsg
	messageID := uuid.NewString()
	now := time.Now().UTC().Truncate(time.Microsecond)

	commentNotification := domain.Notification{
		ID:           uuid.NewString(),
		UserID:       member.ID,
		WorkspaceID:  workspace.ID,
		Type:         domain.NotificationTypeComment,
		EventID:      messageID,
		Message:      "A relevant comment thread has a new reply",
		Title:        "New thread reply",
		Content:      "A relevant comment thread has a new reply",
		ResourceType: &threadMsgResourceType,
		ResourceID:   &messageID,
		Payload:      json.RawMessage(`{"thread_id":"thread-1","message_id":"message-1","page_id":"page-1","workspace_id":"workspace-1","event_topic":"thread_reply_created"}`),
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	invalidMentionNotification := domain.Notification{
		ID:           uuid.NewString(),
		UserID:       "",
		WorkspaceID:  workspace.ID,
		Type:         domain.NotificationTypeMention,
		EventID:      messageID,
		Message:      "You were mentioned in a thread reply",
		Title:        "Mentioned in a thread reply",
		Content:      "You were mentioned in a thread reply",
		ResourceType: &threadMsgResourceType,
		ResourceID:   &messageID,
		Payload:      json.RawMessage(`{"thread_id":"thread-1","message_id":"message-1","page_id":"page-1","workspace_id":"workspace-1","event_topic":"thread_reply_created","mention_source":"explicit"}`),
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if _, _, err := notifRepo.CreateCommentAndMentionNotifications(ctx, []domain.Notification{commentNotification}, []domain.Notification{invalidMentionNotification}); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("expected combined notification validation error, got %v", err)
	}

	var notificationCount int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM notifications WHERE event_id = $1`, messageID).Scan(&notificationCount); err != nil {
		t.Fatalf("count notifications after combined validation failure: %v", err)
	}
	if notificationCount != 0 {
		t.Fatalf("expected no comment or mention notifications committed after validation failure, got %d", notificationCount)
	}

	if unreadCount, err := notifRepo.GetUnreadCount(ctx, member.ID); err != nil {
		t.Fatalf("get unread count after combined validation failure: %v", err)
	} else if unreadCount != 0 {
		t.Fatalf("expected unread count unchanged after combined validation failure, got %d", unreadCount)
	}
}
