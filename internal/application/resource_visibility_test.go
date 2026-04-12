package application

import (
	"context"
	"errors"
	"testing"

	"note-app/internal/domain"
)

type visiblePageRepoStub struct {
	getByIDFn          func(ctx context.Context, pageID string) (domain.Page, domain.PageDraft, error)
	getVisibleByUserFn func(ctx context.Context, pageID string, userID string) (domain.Page, domain.PageDraft, error)
}

func (s visiblePageRepoStub) GetByID(ctx context.Context, pageID string) (domain.Page, domain.PageDraft, error) {
	if s.getByIDFn != nil {
		return s.getByIDFn(ctx, pageID)
	}
	return domain.Page{}, domain.PageDraft{}, domain.ErrNotFound
}

func (s visiblePageRepoStub) GetVisibleByUserID(ctx context.Context, pageID string, userID string) (domain.Page, domain.PageDraft, error) {
	if s.getVisibleByUserFn != nil {
		return s.getVisibleByUserFn(ctx, pageID, userID)
	}
	return domain.Page{}, domain.PageDraft{}, domain.ErrNotFound
}

func TestLoadVisiblePageForActor(t *testing.T) {
	t.Run("uses repository scoped lookup when available", func(t *testing.T) {
		page := domain.Page{ID: "page-1", WorkspaceID: "workspace-1"}
		draft := domain.PageDraft{PageID: page.ID}
		repo := visiblePageRepoStub{
			getByIDFn: func(context.Context, string) (domain.Page, domain.PageDraft, error) {
				t.Fatal("fallback GetByID should not be used when scoped lookup exists")
				return domain.Page{}, domain.PageDraft{}, nil
			},
			getVisibleByUserFn: func(_ context.Context, pageID string, userID string) (domain.Page, domain.PageDraft, error) {
				if pageID != page.ID || userID != "user-1" {
					t.Fatalf("unexpected scoped lookup args: pageID=%s userID=%s", pageID, userID)
				}
				return page, draft, nil
			},
		}

		gotPage, gotDraft, err := loadVisiblePageForActor(context.Background(), repo, workspaceMembershipReaderStub{}, "user-1", page.ID)
		if err != nil {
			t.Fatalf("loadVisiblePageForActor() error = %v", err)
		}
		if gotPage.ID != page.ID || gotDraft.PageID != draft.PageID {
			t.Fatalf("unexpected page/draft: %+v %+v", gotPage, gotDraft)
		}
	})

	t.Run("fallback hides foreign membership as not found", func(t *testing.T) {
		page := domain.Page{ID: "page-1", WorkspaceID: "workspace-1"}
		draft := domain.PageDraft{PageID: page.ID}
		repo := visiblePageRepoStub{
			getByIDFn: func(_ context.Context, pageID string) (domain.Page, domain.PageDraft, error) {
				if pageID != page.ID {
					t.Fatalf("unexpected page lookup: %s", pageID)
				}
				return page, draft, nil
			},
		}
		memberships := workspaceMembershipReaderStub{
			getMembershipByUserIDFn: func(context.Context, string, string) (domain.WorkspaceMember, error) {
				return domain.WorkspaceMember{}, domain.ErrForbidden
			},
		}

		_, _, err := loadVisiblePageForActor(context.Background(), repo, memberships, "outsider-1", page.ID)
		if !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("expected not found, got %v", err)
		}
	})
}

type workspaceMembershipReaderStub struct {
	getMembershipByUserIDFn func(ctx context.Context, workspaceID string, userID string) (domain.WorkspaceMember, error)
}

func (s workspaceMembershipReaderStub) GetMembershipByUserID(ctx context.Context, workspaceID string, userID string) (domain.WorkspaceMember, error) {
	if s.getMembershipByUserIDFn != nil {
		return s.getMembershipByUserIDFn(ctx, workspaceID, userID)
	}
	return domain.WorkspaceMember{}, domain.ErrForbidden
}

func (workspaceMembershipReaderStub) ListMembers(context.Context, string) ([]domain.WorkspaceMember, error) {
	return nil, nil
}
