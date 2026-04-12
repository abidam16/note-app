package application

import (
	"context"
	"errors"

	"note-app/internal/domain"
)

// Once a service has already resolved a concrete resource by id, returning
// not_found for non-members avoids leaking whether that foreign resource exists.
// Same-workspace role checks still happen after successful membership lookup and
// should continue returning forbidden where appropriate.
func hideForeignResourceMembershipError(err error) error {
	if errors.Is(err, domain.ErrForbidden) {
		return domain.ErrNotFound
	}
	return err
}

type pageByIDReader interface {
	GetByID(ctx context.Context, pageID string) (domain.Page, domain.PageDraft, error)
}

type scopedPageVisibilityReader interface {
	GetVisibleByUserID(ctx context.Context, pageID string, userID string) (domain.Page, domain.PageDraft, error)
}

func loadVisiblePageForActor(ctx context.Context, pages pageByIDReader, memberships WorkspaceMembershipReader, actorID, pageID string) (domain.Page, domain.PageDraft, error) {
	if scopedPages, ok := any(pages).(scopedPageVisibilityReader); ok {
		return scopedPages.GetVisibleByUserID(ctx, pageID, actorID)
	}

	page, draft, err := pages.GetByID(ctx, pageID)
	if err != nil {
		return domain.Page{}, domain.PageDraft{}, err
	}

	if _, err := memberships.GetMembershipByUserID(ctx, page.WorkspaceID, actorID); err != nil {
		return domain.Page{}, domain.PageDraft{}, hideForeignResourceMembershipError(err)
	}

	return page, draft, nil
}
