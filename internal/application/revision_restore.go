package application

import (
	"context"
	"fmt"
	"time"

	"note-app/internal/domain"

	"github.com/google/uuid"
)

type RestoreRevisionInput struct {
	PageID     string
	RevisionID string
}

func (s RevisionService) RestoreRevision(ctx context.Context, actorID string, input RestoreRevisionInput) (RestoreRevisionResult, error) {
	page, _, err := s.pages.GetByID(ctx, input.PageID)
	if err != nil {
		return RestoreRevisionResult{}, err
	}

	membership, err := s.memberships.GetMembershipByUserID(ctx, page.WorkspaceID, actorID)
	if err != nil {
		return RestoreRevisionResult{}, err
	}
	if membership.Role == domain.RoleViewer {
		return RestoreRevisionResult{}, domain.ErrForbidden
	}

	revision, err := s.revisions.GetByID(ctx, input.RevisionID)
	if err != nil {
		return RestoreRevisionResult{}, err
	}
	if revision.PageID != input.PageID {
		return RestoreRevisionResult{}, fmt.Errorf("%w: revision must belong to the requested page", domain.ErrValidation)
	}
	if err := ValidateDocument(revision.Content); err != nil {
		return RestoreRevisionResult{}, err
	}

	now := time.Now().UTC()
	draft, err := s.pages.UpdateDraft(ctx, input.PageID, cloneRawMessage(revision.Content), actorID, now)
	if err != nil {
		return RestoreRevisionResult{}, err
	}

	restoreRevision := domain.Revision{
		ID:        uuid.NewString(),
		PageID:    input.PageID,
		Content:   cloneRawMessage(revision.Content),
		CreatedBy: actorID,
		CreatedAt: now,
	}
	restoreRevision, err = s.revisions.Create(ctx, restoreRevision)
	if err != nil {
		return RestoreRevisionResult{}, err
	}

	return RestoreRevisionResult{Draft: draft, Revision: restoreRevision}, nil
}
