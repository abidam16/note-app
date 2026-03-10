package application

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"note-app/internal/domain"

	"github.com/google/uuid"
)

type RevisionPageRepository interface {
	GetByID(ctx context.Context, pageID string) (domain.Page, domain.PageDraft, error)
	UpdateDraft(ctx context.Context, pageID string, content json.RawMessage, lastEditedBy string, updatedAt time.Time) (domain.PageDraft, error)
}

type RevisionRepository interface {
	Create(ctx context.Context, revision domain.Revision) (domain.Revision, error)
	GetByID(ctx context.Context, revisionID string) (domain.Revision, error)
	ListByPageID(ctx context.Context, pageID string) ([]domain.Revision, error)
}

type CreateRevisionInput struct {
	PageID string
	Label  *string
	Note   *string
}

type RestoreRevisionResult struct {
	Draft    domain.PageDraft
	Revision domain.Revision
}

type RevisionService struct {
	revisions   RevisionRepository
	pages       RevisionPageRepository
	memberships WorkspaceMembershipReader
}

func NewRevisionService(revisions RevisionRepository, pages RevisionPageRepository, memberships WorkspaceMembershipReader) RevisionService {
	return RevisionService{
		revisions:   revisions,
		pages:       pages,
		memberships: memberships,
	}
}

func (s RevisionService) CreateRevision(ctx context.Context, actorID string, input CreateRevisionInput) (domain.Revision, error) {
	page, draft, err := s.pages.GetByID(ctx, input.PageID)
	if err != nil {
		return domain.Revision{}, err
	}

	membership, err := s.memberships.GetMembershipByUserID(ctx, page.WorkspaceID, actorID)
	if err != nil {
		return domain.Revision{}, err
	}
	if membership.Role == domain.RoleViewer {
		return domain.Revision{}, domain.ErrForbidden
	}
	if err := ValidateDocument(draft.Content); err != nil {
		return domain.Revision{}, err
	}

	now := time.Now().UTC()
	revision := domain.Revision{
		ID:        uuid.NewString(),
		PageID:    page.ID,
		Label:     normalizeOptionalText(input.Label),
		Note:      normalizeOptionalText(input.Note),
		Content:   cloneRawMessage(draft.Content),
		CreatedBy: actorID,
		CreatedAt: now,
	}

	return s.revisions.Create(ctx, revision)
}

func (s RevisionService) ListRevisions(ctx context.Context, actorID, pageID string) ([]domain.Revision, error) {
	page, _, err := s.pages.GetByID(ctx, pageID)
	if err != nil {
		return nil, err
	}

	if _, err := s.memberships.GetMembershipByUserID(ctx, page.WorkspaceID, actorID); err != nil {
		return nil, err
	}

	return s.revisions.ListByPageID(ctx, pageID)
}

func normalizeOptionalText(value *string) *string {
	if value == nil {
		return nil
	}

	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}

	return &trimmed
}

func cloneRawMessage(value json.RawMessage) json.RawMessage {
	if len(value) == 0 {
		return nil
	}

	clone := make(json.RawMessage, len(value))
	copy(clone, value)
	return clone
}
