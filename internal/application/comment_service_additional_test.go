package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"note-app/internal/domain"
)

type commentRepoStub struct {
	createFn  func(context.Context, domain.PageComment) (domain.PageComment, error)
	getByIDFn func(context.Context, string) (domain.PageComment, error)
	listFn    func(context.Context, string) ([]domain.PageComment, error)
	resolveFn func(context.Context, string, string, time.Time) (domain.PageComment, error)
	calls     int
}

func (s *commentRepoStub) Create(ctx context.Context, comment domain.PageComment) (domain.PageComment, error) {
	if s.createFn != nil {
		return s.createFn(ctx, comment)
	}
	return comment, nil
}
func (s *commentRepoStub) GetByID(ctx context.Context, commentID string) (domain.PageComment, error) {
	if s.getByIDFn != nil {
		return s.getByIDFn(ctx, commentID)
	}
	return domain.PageComment{}, domain.ErrNotFound
}
func (s *commentRepoStub) ListByPageID(ctx context.Context, pageID string) ([]domain.PageComment, error) {
	if s.listFn != nil {
		return s.listFn(ctx, pageID)
	}
	return nil, nil
}
func (s *commentRepoStub) Resolve(ctx context.Context, commentID string, resolvedBy string, resolvedAt time.Time) (domain.PageComment, error) {
	s.calls++
	if s.resolveFn != nil {
		return s.resolveFn(ctx, commentID, resolvedBy, resolvedAt)
	}
	return domain.PageComment{ID: commentID}, nil
}

type commentPageRepoStub struct {
	getFn func(context.Context, string) (domain.Page, domain.PageDraft, error)
}

func (s commentPageRepoStub) GetByID(ctx context.Context, pageID string) (domain.Page, domain.PageDraft, error) {
	if s.getFn != nil {
		return s.getFn(ctx, pageID)
	}
	return domain.Page{}, domain.PageDraft{}, domain.ErrNotFound
}

type commentMembershipStub struct {
	getFn func(context.Context, string, string) (domain.WorkspaceMember, error)
}

func (s commentMembershipStub) GetMembershipByUserID(ctx context.Context, workspaceID, userID string) (domain.WorkspaceMember, error) {
	if s.getFn != nil {
		return s.getFn(ctx, workspaceID, userID)
	}
	return domain.WorkspaceMember{}, domain.ErrForbidden
}
func (s commentMembershipStub) ListMembers(context.Context, string) ([]domain.WorkspaceMember, error) {
	return nil, nil
}

type commentNotificationStub struct {
	err error
}

func (s commentNotificationStub) NotifyInvitationCreated(context.Context, domain.WorkspaceInvitation) error {
	return nil
}
func (s commentNotificationStub) NotifyCommentCreated(context.Context, domain.Page, domain.PageComment) error {
	return s.err
}

func TestCommentServiceAdditionalBranches(t *testing.T) {
	page := domain.Page{ID: "p1", WorkspaceID: "w1"}
	membership := commentMembershipStub{getFn: func(context.Context, string, string) (domain.WorkspaceMember, error) {
		return domain.WorkspaceMember{Role: domain.RoleEditor}, nil
	}}

	t.Run("create comment notification error", func(t *testing.T) {
		repo := &commentRepoStub{}
		svc := NewCommentService(repo, commentPageRepoStub{getFn: func(context.Context, string) (domain.Page, domain.PageDraft, error) {
			return page, domain.PageDraft{}, nil
		}}, membership, commentNotificationStub{err: errors.New("notify failed")})

		_, err := svc.CreateComment(context.Background(), "u1", CreateCommentInput{PageID: "p1", Body: "hello"})
		if err == nil || err.Error() != "notify failed" {
			t.Fatalf("expected notification failure, got %v", err)
		}
	})

	t.Run("list and resolve branches", func(t *testing.T) {
		repo := &commentRepoStub{
			getByIDFn: func(context.Context, string) (domain.PageComment, error) {
				resolvedAt := time.Now().UTC()
				return domain.PageComment{ID: "c1", PageID: "p1", Body: "x", CreatedBy: "u1", CreatedAt: resolvedAt, ResolvedAt: &resolvedAt}, nil
			},
			listFn: func(context.Context, string) ([]domain.PageComment, error) {
				return []domain.PageComment{{ID: "c1", PageID: "p1", Body: "x", CreatedBy: "u1", CreatedAt: time.Now().UTC()}}, nil
			},
		}

		svc := NewCommentService(repo, commentPageRepoStub{getFn: func(context.Context, string) (domain.Page, domain.PageDraft, error) {
			return page, domain.PageDraft{}, nil
		}}, membership)

		list, err := svc.ListComments(context.Background(), "u1", "p1")
		if err != nil || len(list) != 1 {
			t.Fatalf("expected list success, err=%v len=%d", err, len(list))
		}

		resolved, err := svc.ResolveComment(context.Background(), "u1", ResolveCommentInput{CommentID: "c1"})
		if err != nil {
			t.Fatalf("expected resolve to return already resolved comment, got %v", err)
		}
		if resolved.ResolvedAt == nil {
			t.Fatal("expected comment to remain resolved")
		}
		if repo.calls != 0 {
			t.Fatalf("expected no repo resolve call for already resolved comment, got %d", repo.calls)
		}
	})

	t.Run("resolve page lookup error", func(t *testing.T) {
		repo := &commentRepoStub{getByIDFn: func(context.Context, string) (domain.PageComment, error) {
			return domain.PageComment{ID: "c1", PageID: "missing", Body: "x", CreatedBy: "u1", CreatedAt: time.Now().UTC()}, nil
		}}
		svc := NewCommentService(repo, commentPageRepoStub{}, membership)
		if _, err := svc.ResolveComment(context.Background(), "u1", ResolveCommentInput{CommentID: "c1"}); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("expected page not found while resolving, got %v", err)
		}
	})
}
