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
	if _, err := repo.UpdateName(ctx, child.ID, "platform", now.Add(7*time.Second)); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("expected duplicate rename validation, got %v", err)
	}

	if _, err := repo.GetByID(ctx, uuid.NewString()); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected folder not found, got %v", err)
	}
	if _, err := repo.UpdateName(ctx, uuid.NewString(), "Missing", now); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected update missing folder not found, got %v", err)
	}
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

	fetchedPage, fetchedDraft, err := repo.GetByID(ctx, page.ID)
	if err != nil || fetchedPage.Title != page.Title || len(fetchedDraft.Content) == 0 {
		t.Fatalf("get page mismatch: err=%v title=%s", err, fetchedPage.Title)
	}

	updatedPage, err := repo.UpdateMetadata(ctx, page.ID, "Hello Updated", nil, now.Add(time.Minute))
	if err != nil || updatedPage.Title != "Hello Updated" || updatedPage.FolderID != nil {
		t.Fatalf("update metadata mismatch: err=%v title=%s", err, updatedPage.Title)
	}

	updatedDraft, err := repo.UpdateDraft(ctx, page.ID, json.RawMessage(`[{"type":"paragraph","text":"searchable token"}]`), owner.ID, now.Add(2*time.Minute))
	if err != nil {
		t.Fatalf("update draft: %v", err)
	}
	if updatedDraft.SearchBody == "" {
		t.Fatal("expected search body to be derived")
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

	trashList, err := repo.ListTrashByWorkspaceID(ctx, workspace.ID)
	if err != nil || len(trashList) != 1 || trashList[0].ID != trash.ID {
		t.Fatalf("list trash mismatch: err=%v len=%d", err, len(trashList))
	}

	trashItem, err := repo.GetTrashItemByID(ctx, trash.ID)
	if err != nil || trashItem.PageID != page.ID {
		t.Fatalf("get trash item mismatch: err=%v page=%s", err, trashItem.PageID)
	}

	restoredPage, err := repo.RestoreTrashItem(ctx, trash.ID, owner.ID, now.Add(4*time.Minute))
	if err != nil || restoredPage.ID != page.ID {
		t.Fatalf("restore page mismatch: err=%v id=%s", err, restoredPage.ID)
	}

	if _, err := repo.GetTrashItemByID(ctx, trash.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected restored trash item hidden, got %v", err)
	}

	if _, err := repo.RestoreTrashItem(ctx, trash.ID, owner.ID, now.Add(5*time.Minute)); !errors.Is(err, domain.ErrNotFound) {
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
}

func TestRevisionCommentNotificationRepositoriesIntegration(t *testing.T) {
	pool := integrationPool(t)
	ctx := context.Background()

	owner := seedUser(t, pool, "combo-owner@example.com")
	member := seedUser(t, pool, "combo-member@example.com")
	workspace, _ := seedWorkspaceWithOwner(t, pool, owner)
	page, _ := seedPageWithDraft(t, pool, workspace.ID, owner.ID, nil, "Doc")
	now := time.Now().UTC().Truncate(time.Microsecond)

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

	notifRepo := NewNotificationRepository(pool)
	notif := domain.Notification{ID: uuid.NewString(), UserID: member.ID, WorkspaceID: workspace.ID, Type: domain.NotificationTypeComment, EventID: comment.ID, Message: "commented", CreatedAt: now}
	createdNotif, err := notifRepo.Create(ctx, notif)
	if err != nil {
		t.Fatalf("create notification: %v", err)
	}
	if _, err := notifRepo.Create(ctx, domain.Notification{ID: uuid.NewString(), UserID: member.ID, WorkspaceID: workspace.ID, Type: domain.NotificationTypeComment, EventID: comment.ID, Message: "dup", CreatedAt: now}); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("expected notification conflict, got %v", err)
	}
	allNotifs, err := notifRepo.ListByUserID(ctx, member.ID)
	if err != nil || len(allNotifs) != 1 || allNotifs[0].ID != createdNotif.ID {
		t.Fatalf("list notifications mismatch: err=%v len=%d", err, len(allNotifs))
	}
	marked, err := notifRepo.MarkRead(ctx, createdNotif.ID, member.ID, now.Add(2*time.Minute))
	if err != nil || marked.ReadAt == nil {
		t.Fatalf("mark read mismatch: err=%v", err)
	}
	if _, err := notifRepo.MarkRead(ctx, uuid.NewString(), member.ID, now); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected mark read not found, got %v", err)
	}
}
