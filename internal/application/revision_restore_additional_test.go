package application

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"note-app/internal/domain"
)

type restorePageRepoStub struct {
	getByIDFn     func(context.Context, string) (domain.Page, domain.PageDraft, error)
	updateDraftFn func(context.Context, string, json.RawMessage, string, time.Time) (domain.PageDraft, error)
}

func (s restorePageRepoStub) GetByID(ctx context.Context, pageID string) (domain.Page, domain.PageDraft, error) {
	if s.getByIDFn != nil {
		return s.getByIDFn(ctx, pageID)
	}
	return domain.Page{}, domain.PageDraft{}, domain.ErrNotFound
}

func (s restorePageRepoStub) UpdateDraft(ctx context.Context, pageID string, content json.RawMessage, actorID string, updatedAt time.Time) (domain.PageDraft, error) {
	if s.updateDraftFn != nil {
		return s.updateDraftFn(ctx, pageID, content, actorID, updatedAt)
	}
	return domain.PageDraft{}, domain.ErrNotFound
}

type restoreRevisionRepoStub struct {
	getByIDFn func(context.Context, string) (domain.Revision, error)
	createFn  func(context.Context, domain.Revision) (domain.Revision, error)
}

func (s restoreRevisionRepoStub) Create(ctx context.Context, revision domain.Revision) (domain.Revision, error) {
	if s.createFn != nil {
		return s.createFn(ctx, revision)
	}
	return revision, nil
}
func (s restoreRevisionRepoStub) GetByID(ctx context.Context, revisionID string) (domain.Revision, error) {
	if s.getByIDFn != nil {
		return s.getByIDFn(ctx, revisionID)
	}
	return domain.Revision{}, domain.ErrNotFound
}
func (s restoreRevisionRepoStub) ListByPageID(context.Context, string) ([]domain.Revision, error) {
	return nil, nil
}

type restoreMembershipStub struct {
	getFn func(context.Context, string, string) (domain.WorkspaceMember, error)
}

func (s restoreMembershipStub) GetMembershipByUserID(ctx context.Context, workspaceID, userID string) (domain.WorkspaceMember, error) {
	if s.getFn != nil {
		return s.getFn(ctx, workspaceID, userID)
	}
	return domain.WorkspaceMember{}, domain.ErrForbidden
}
func (s restoreMembershipStub) ListMembers(context.Context, string) ([]domain.WorkspaceMember, error) {
	return nil, nil
}

type restoreThreadReevaluatorStub struct {
	called  bool
	pageID  string
	content json.RawMessage
	context domain.ThreadAnchorReevaluationContext
	err     error
}

func (s *restoreThreadReevaluatorStub) ReevaluatePageAnchors(_ context.Context, pageID string, content json.RawMessage, reevaluation domain.ThreadAnchorReevaluationContext) error {
	s.called = true
	s.pageID = pageID
	s.content = append(json.RawMessage(nil), content...)
	s.context = reevaluation
	return s.err
}

func TestRestoreRevisionAdditionalBranches(t *testing.T) {
	page := domain.Page{ID: "p1", WorkspaceID: "w1", Title: "Doc"}
	validRevision := domain.Revision{ID: "r1", PageID: "p1", Content: json.RawMessage(`[{"type":"paragraph","text":"old"}]`)}

	t.Run("page and revision lookup errors", func(t *testing.T) {
		svc := NewRevisionService(restoreRevisionRepoStub{}, restorePageRepoStub{}, restoreMembershipStub{})
		if _, err := svc.RestoreRevision(context.Background(), "u1", RestoreRevisionInput{PageID: "p1", RevisionID: "r1"}); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("expected page not found, got %v", err)
		}
	})

	t.Run("membership and validation errors", func(t *testing.T) {
		svc := NewRevisionService(
			restoreRevisionRepoStub{getByIDFn: func(context.Context, string) (domain.Revision, error) {
				return domain.Revision{ID: "r1", PageID: "p1", Content: json.RawMessage(`[{"type":"unsupported"}]`)}, nil
			}},
			restorePageRepoStub{getByIDFn: func(context.Context, string) (domain.Page, domain.PageDraft, error) {
				return page, domain.PageDraft{}, nil
			}},
			restoreMembershipStub{getFn: func(context.Context, string, string) (domain.WorkspaceMember, error) {
				return domain.WorkspaceMember{Role: domain.RoleEditor}, nil
			}},
		)
		if _, err := svc.RestoreRevision(context.Background(), "u1", RestoreRevisionInput{PageID: "p1", RevisionID: "r1"}); !errors.Is(err, domain.ErrValidation) {
			t.Fatalf("expected validation for invalid revision content, got %v", err)
		}

		svc = NewRevisionService(
			restoreRevisionRepoStub{getByIDFn: func(context.Context, string) (domain.Revision, error) { return validRevision, nil }},
			restorePageRepoStub{getByIDFn: func(context.Context, string) (domain.Page, domain.PageDraft, error) {
				return page, domain.PageDraft{}, nil
			}},
			restoreMembershipStub{getFn: func(context.Context, string, string) (domain.WorkspaceMember, error) {
				return domain.WorkspaceMember{Role: domain.RoleViewer}, nil
			}},
		)
		if _, err := svc.RestoreRevision(context.Background(), "u1", RestoreRevisionInput{PageID: "p1", RevisionID: "r1"}); !errors.Is(err, domain.ErrForbidden) {
			t.Fatalf("expected forbidden for viewer, got %v", err)
		}
	})

	t.Run("update draft and create revision errors", func(t *testing.T) {
		svc := NewRevisionService(
			restoreRevisionRepoStub{getByIDFn: func(context.Context, string) (domain.Revision, error) { return validRevision, nil }},
			restorePageRepoStub{
				getByIDFn: func(context.Context, string) (domain.Page, domain.PageDraft, error) {
					return page, domain.PageDraft{}, nil
				},
				updateDraftFn: func(context.Context, string, json.RawMessage, string, time.Time) (domain.PageDraft, error) {
					return domain.PageDraft{}, errors.New("update failed")
				},
			},
			restoreMembershipStub{getFn: func(context.Context, string, string) (domain.WorkspaceMember, error) {
				return domain.WorkspaceMember{Role: domain.RoleEditor}, nil
			}},
		)
		if _, err := svc.RestoreRevision(context.Background(), "u1", RestoreRevisionInput{PageID: "p1", RevisionID: "r1"}); err == nil || err.Error() != "update failed" {
			t.Fatalf("expected update draft error, got %v", err)
		}

		svc = NewRevisionService(
			restoreRevisionRepoStub{
				getByIDFn: func(context.Context, string) (domain.Revision, error) { return validRevision, nil },
				createFn: func(context.Context, domain.Revision) (domain.Revision, error) {
					return domain.Revision{}, errors.New("create failed")
				},
			},
			restorePageRepoStub{
				getByIDFn: func(context.Context, string) (domain.Page, domain.PageDraft, error) {
					return page, domain.PageDraft{}, nil
				},
				updateDraftFn: func(_ context.Context, pageID string, content json.RawMessage, actor string, updatedAt time.Time) (domain.PageDraft, error) {
					return domain.PageDraft{PageID: pageID, Content: content, LastEditedBy: actor, UpdatedAt: updatedAt}, nil
				},
			},
			restoreMembershipStub{getFn: func(context.Context, string, string) (domain.WorkspaceMember, error) {
				return domain.WorkspaceMember{Role: domain.RoleEditor}, nil
			}},
		)
		if _, err := svc.RestoreRevision(context.Background(), "u1", RestoreRevisionInput{PageID: "p1", RevisionID: "r1"}); err == nil || err.Error() != "create failed" {
			t.Fatalf("expected create revision error, got %v", err)
		}
	})

	t.Run("reevaluates anchors after restore update", func(t *testing.T) {
		reevaluator := &restoreThreadReevaluatorStub{}
		svc := NewRevisionService(
			restoreRevisionRepoStub{
				getByIDFn: func(context.Context, string) (domain.Revision, error) { return validRevision, nil },
				createFn:  func(_ context.Context, revision domain.Revision) (domain.Revision, error) { return revision, nil },
			},
			restorePageRepoStub{
				getByIDFn: func(context.Context, string) (domain.Page, domain.PageDraft, error) {
					return page, domain.PageDraft{}, nil
				},
				updateDraftFn: func(_ context.Context, pageID string, content json.RawMessage, actor string, updatedAt time.Time) (domain.PageDraft, error) {
					return domain.PageDraft{PageID: pageID, Content: content, LastEditedBy: actor, UpdatedAt: updatedAt}, nil
				},
			},
			restoreMembershipStub{getFn: func(context.Context, string, string) (domain.WorkspaceMember, error) {
				return domain.WorkspaceMember{Role: domain.RoleEditor}, nil
			}},
			reevaluator,
		)
		if _, err := svc.RestoreRevision(context.Background(), "u1", RestoreRevisionInput{PageID: "p1", RevisionID: "r1"}); err != nil {
			t.Fatalf("expected restore success, got %v", err)
		}
		if !reevaluator.called || reevaluator.pageID != "p1" || string(reevaluator.content) != string(validRevision.Content) {
			t.Fatalf("expected reevaluator call with restored content, got called=%t page=%s content=%s", reevaluator.called, reevaluator.pageID, string(reevaluator.content))
		}
		if reevaluator.context.Reason != domain.PageCommentThreadEventReasonRevisionRestore {
			t.Fatalf("expected revision_restored reason, got %s", reevaluator.context.Reason)
		}
		if reevaluator.context.RevisionID == nil || *reevaluator.context.RevisionID != "r1" {
			t.Fatalf("expected revision id linkage, got %+v", reevaluator.context.RevisionID)
		}
	})
}
