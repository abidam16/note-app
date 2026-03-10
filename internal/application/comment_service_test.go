package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"note-app/internal/domain"
)

type fakeCommentRepo struct {
	comments map[string]domain.PageComment
	ordered  []domain.PageComment
}

func (r *fakeCommentRepo) Create(_ context.Context, comment domain.PageComment) (domain.PageComment, error) {
	r.comments[comment.ID] = comment
	r.ordered = append(r.ordered, comment)
	return comment, nil
}

func (r *fakeCommentRepo) GetByID(_ context.Context, commentID string) (domain.PageComment, error) {
	comment, ok := r.comments[commentID]
	if !ok {
		return domain.PageComment{}, domain.ErrNotFound
	}
	return comment, nil
}

func (r *fakeCommentRepo) ListByPageID(_ context.Context, pageID string) ([]domain.PageComment, error) {
	comments := make([]domain.PageComment, 0)
	for _, comment := range r.ordered {
		if comment.PageID == pageID {
			comments = append(comments, r.comments[comment.ID])
		}
	}
	return comments, nil
}

func (r *fakeCommentRepo) Resolve(_ context.Context, commentID string, resolvedBy string, resolvedAt time.Time) (domain.PageComment, error) {
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

func TestCommentServiceCreateListAndResolve(t *testing.T) {
	memberships := &fakeWorkspaceRepo{memberships: map[string][]domain.WorkspaceMember{
		"workspace-1": {
			{ID: "member-1", WorkspaceID: "workspace-1", UserID: "viewer-1", Role: domain.RoleViewer},
			{ID: "member-2", WorkspaceID: "workspace-1", UserID: "editor-1", Role: domain.RoleEditor},
		},
	}, invitations: map[string]domain.WorkspaceInvitation{}, owners: map[string]int{}}
	pages := &fakePageRepo{
		pages: map[string]domain.Page{
			"page-1": {ID: "page-1", WorkspaceID: "workspace-1", Title: "Doc"},
		},
		drafts: map[string]domain.PageDraft{
			"page-1": {PageID: "page-1"},
		},
	}
	comments := &fakeCommentRepo{comments: map[string]domain.PageComment{}, ordered: []domain.PageComment{}}
	service := NewCommentService(comments, pages, memberships)

	created, err := service.CreateComment(context.Background(), "viewer-1", CreateCommentInput{
		PageID: "page-1",
		Body:   "  needs review before publish  ",
	})
	if err != nil {
		t.Fatalf("CreateComment() error = %v", err)
	}
	if created.Body != "needs review before publish" {
		t.Fatalf("expected trimmed body, got %q", created.Body)
	}
	if created.ResolvedAt != nil || created.ResolvedBy != nil {
		t.Fatalf("expected new comment to be unresolved")
	}

	listed, err := service.ListComments(context.Background(), "viewer-1", "page-1")
	if err != nil {
		t.Fatalf("ListComments() error = %v", err)
	}
	if len(listed) != 1 {
		t.Fatalf("expected one comment, got %d", len(listed))
	}

	resolved, err := service.ResolveComment(context.Background(), "editor-1", ResolveCommentInput{CommentID: created.ID})
	if err != nil {
		t.Fatalf("ResolveComment() error = %v", err)
	}
	if resolved.ResolvedBy == nil || *resolved.ResolvedBy != "editor-1" {
		t.Fatalf("expected resolved_by editor-1, got %+v", resolved.ResolvedBy)
	}
	if resolved.ResolvedAt == nil {
		t.Fatalf("expected resolved_at to be set")
	}

	listedAfterResolve, err := service.ListComments(context.Background(), "viewer-1", "page-1")
	if err != nil {
		t.Fatalf("ListComments() after resolve error = %v", err)
	}
	if len(listedAfterResolve) != 1 {
		t.Fatalf("expected one comment after resolve, got %d", len(listedAfterResolve))
	}
	if listedAfterResolve[0].ResolvedAt == nil {
		t.Fatalf("expected resolved comment to remain in history")
	}
}

func TestCommentServiceRejectsInvalidBodyViewerResolveAndMissingResources(t *testing.T) {
	viewerMemberships := &fakeWorkspaceRepo{memberships: map[string][]domain.WorkspaceMember{
		"workspace-1": {
			{ID: "member-1", WorkspaceID: "workspace-1", UserID: "viewer-1", Role: domain.RoleViewer},
		},
	}, invitations: map[string]domain.WorkspaceInvitation{}, owners: map[string]int{}}
	pages := &fakePageRepo{
		pages: map[string]domain.Page{
			"page-1": {ID: "page-1", WorkspaceID: "workspace-1", Title: "Doc"},
		},
		drafts: map[string]domain.PageDraft{
			"page-1": {PageID: "page-1"},
		},
	}
	now := time.Now().UTC()
	comments := &fakeCommentRepo{comments: map[string]domain.PageComment{
		"comment-1": {ID: "comment-1", PageID: "page-1", Body: "hello", CreatedBy: "viewer-1", CreatedAt: now},
	}, ordered: []domain.PageComment{
		{ID: "comment-1", PageID: "page-1", Body: "hello", CreatedBy: "viewer-1", CreatedAt: now},
	}}
	service := NewCommentService(comments, pages, viewerMemberships)

	_, err := service.CreateComment(context.Background(), "viewer-1", CreateCommentInput{PageID: "page-1", Body: "   "})
	if !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("expected validation error, got %v", err)
	}

	_, err = service.ResolveComment(context.Background(), "viewer-1", ResolveCommentInput{CommentID: "comment-1"})
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected forbidden error, got %v", err)
	}

	_, err = service.CreateComment(context.Background(), "viewer-1", CreateCommentInput{PageID: "missing-page", Body: "hello"})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected not found error, got %v", err)
	}

	_, err = service.ResolveComment(context.Background(), "viewer-1", ResolveCommentInput{CommentID: "missing-comment"})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected not found error, got %v", err)
	}
}
