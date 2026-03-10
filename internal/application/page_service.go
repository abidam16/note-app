package application

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"note-app/internal/domain"

	"github.com/google/uuid"
)

type PageRepository interface {
	CreateWithDraft(ctx context.Context, page domain.Page, draft domain.PageDraft) (domain.Page, domain.PageDraft, error)
	GetByID(ctx context.Context, pageID string) (domain.Page, domain.PageDraft, error)
	UpdateMetadata(ctx context.Context, pageID string, title string, folderID *string, updatedAt time.Time) (domain.Page, error)
	UpdateDraft(ctx context.Context, pageID string, content json.RawMessage, lastEditedBy string, updatedAt time.Time) (domain.PageDraft, error)
	SoftDelete(ctx context.Context, trashItem domain.TrashItem) error
	ListTrashByWorkspaceID(ctx context.Context, workspaceID string) ([]domain.TrashItem, error)
	GetTrashItemByID(ctx context.Context, trashItemID string) (domain.TrashItem, error)
	RestoreTrashItem(ctx context.Context, trashItemID string, restoredBy string, restoredAt time.Time) (domain.Page, error)
}

type CreatePageInput struct {
	WorkspaceID string
	FolderID    *string
	Title       string
}

type UpdatePageInput struct {
	PageID    string
	Title     *string
	FolderID  *string
	FolderSet bool
}

type UpdateDraftInput struct {
	PageID  string
	Content json.RawMessage
}

type DeletePageInput struct {
	PageID string
}

type RestoreTrashItemInput struct {
	TrashItemID string
}

type PageService struct {
	pages       PageRepository
	memberships WorkspaceMembershipReader
	folders     FolderRepository
}

func NewPageService(pages PageRepository, memberships WorkspaceMembershipReader, folders FolderRepository) PageService {
	return PageService{
		pages:       pages,
		memberships: memberships,
		folders:     folders,
	}
}

func (s PageService) CreatePage(ctx context.Context, actorID string, input CreatePageInput) (domain.Page, domain.PageDraft, error) {
	title := strings.TrimSpace(input.Title)
	if title == "" {
		return domain.Page{}, domain.PageDraft{}, fmt.Errorf("%w: page title is required", domain.ErrValidation)
	}

	membership, err := s.memberships.GetMembershipByUserID(ctx, input.WorkspaceID, actorID)
	if err != nil {
		return domain.Page{}, domain.PageDraft{}, err
	}
	if membership.Role == domain.RoleViewer {
		return domain.Page{}, domain.PageDraft{}, domain.ErrForbidden
	}

	folderID, err := s.resolveFolderID(ctx, input.WorkspaceID, input.FolderID)
	if err != nil {
		return domain.Page{}, domain.PageDraft{}, err
	}

	now := time.Now().UTC()
	page := domain.Page{
		ID:          uuid.NewString(),
		WorkspaceID: input.WorkspaceID,
		FolderID:    folderID,
		Title:       title,
		CreatedBy:   actorID,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	draft := domain.PageDraft{
		PageID:       page.ID,
		Content:      json.RawMessage("[]"),
		LastEditedBy: actorID,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	return s.pages.CreateWithDraft(ctx, page, draft)
}

func (s PageService) GetPage(ctx context.Context, actorID, pageID string) (domain.Page, domain.PageDraft, error) {
	page, draft, err := s.pages.GetByID(ctx, pageID)
	if err != nil {
		return domain.Page{}, domain.PageDraft{}, err
	}

	if _, err := s.memberships.GetMembershipByUserID(ctx, page.WorkspaceID, actorID); err != nil {
		return domain.Page{}, domain.PageDraft{}, err
	}

	return page, draft, nil
}

func (s PageService) UpdatePage(ctx context.Context, actorID string, input UpdatePageInput) (domain.Page, error) {
	page, _, err := s.pages.GetByID(ctx, input.PageID)
	if err != nil {
		return domain.Page{}, err
	}

	membership, err := s.memberships.GetMembershipByUserID(ctx, page.WorkspaceID, actorID)
	if err != nil {
		return domain.Page{}, err
	}
	if membership.Role == domain.RoleViewer {
		return domain.Page{}, domain.ErrForbidden
	}

	updatedTitle := page.Title
	if input.Title != nil {
		trimmedTitle := strings.TrimSpace(*input.Title)
		if trimmedTitle == "" {
			return domain.Page{}, fmt.Errorf("%w: page title is required", domain.ErrValidation)
		}
		updatedTitle = trimmedTitle
	}

	updatedFolderID := page.FolderID
	if input.FolderSet {
		resolvedFolderID, err := s.resolveFolderID(ctx, page.WorkspaceID, input.FolderID)
		if err != nil {
			return domain.Page{}, err
		}
		updatedFolderID = resolvedFolderID
	}

	return s.pages.UpdateMetadata(ctx, page.ID, updatedTitle, updatedFolderID, time.Now().UTC())
}

func (s PageService) UpdateDraft(ctx context.Context, actorID string, input UpdateDraftInput) (domain.PageDraft, error) {
	page, _, err := s.pages.GetByID(ctx, input.PageID)
	if err != nil {
		return domain.PageDraft{}, err
	}

	membership, err := s.memberships.GetMembershipByUserID(ctx, page.WorkspaceID, actorID)
	if err != nil {
		return domain.PageDraft{}, err
	}
	if membership.Role == domain.RoleViewer {
		return domain.PageDraft{}, domain.ErrForbidden
	}
	if len(input.Content) == 0 {
		return domain.PageDraft{}, fmt.Errorf("%w: content is required", domain.ErrValidation)
	}
	if err := ValidateDocument(input.Content); err != nil {
		return domain.PageDraft{}, err
	}

	return s.pages.UpdateDraft(ctx, page.ID, input.Content, actorID, time.Now().UTC())
}

func (s PageService) DeletePage(ctx context.Context, actorID string, input DeletePageInput) error {
	page, _, err := s.pages.GetByID(ctx, input.PageID)
	if err != nil {
		return err
	}

	membership, err := s.memberships.GetMembershipByUserID(ctx, page.WorkspaceID, actorID)
	if err != nil {
		return err
	}
	if membership.Role == domain.RoleViewer {
		return domain.ErrForbidden
	}

	trashItem := domain.TrashItem{
		ID:          uuid.NewString(),
		WorkspaceID: page.WorkspaceID,
		PageID:      page.ID,
		PageTitle:   page.Title,
		DeletedBy:   actorID,
		DeletedAt:   time.Now().UTC(),
	}

	return s.pages.SoftDelete(ctx, trashItem)
}

func (s PageService) ListTrash(ctx context.Context, actorID, workspaceID string) ([]domain.TrashItem, error) {
	if _, err := s.memberships.GetMembershipByUserID(ctx, workspaceID, actorID); err != nil {
		return nil, err
	}

	return s.pages.ListTrashByWorkspaceID(ctx, workspaceID)
}

func (s PageService) RestoreTrashItem(ctx context.Context, actorID string, input RestoreTrashItemInput) (domain.Page, error) {
	trashItem, err := s.pages.GetTrashItemByID(ctx, input.TrashItemID)
	if err != nil {
		return domain.Page{}, err
	}

	membership, err := s.memberships.GetMembershipByUserID(ctx, trashItem.WorkspaceID, actorID)
	if err != nil {
		return domain.Page{}, err
	}
	if membership.Role == domain.RoleViewer {
		return domain.Page{}, domain.ErrForbidden
	}

	return s.pages.RestoreTrashItem(ctx, input.TrashItemID, actorID, time.Now().UTC())
}

func (s PageService) resolveFolderID(ctx context.Context, workspaceID string, inputFolderID *string) (*string, error) {
	if inputFolderID == nil {
		return nil, nil
	}

	trimmedFolderID := strings.TrimSpace(*inputFolderID)
	if trimmedFolderID == "" {
		return nil, nil
	}

	folder, err := s.folders.GetByID(ctx, trimmedFolderID)
	if err != nil {
		return nil, err
	}
	if folder.WorkspaceID != workspaceID {
		return nil, fmt.Errorf("%w: folder must belong to the same workspace", domain.ErrValidation)
	}

	return &trimmedFolderID, nil
}
