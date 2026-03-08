package application

import (
	"context"
	"fmt"
	"strings"
	"time"

	"note-app/internal/domain"

	"github.com/google/uuid"
)

type CommentPageRepository interface {
	GetByID(ctx context.Context, pageID string) (domain.Page, domain.PageDraft, error)
}

type CommentRepository interface {
	Create(ctx context.Context, comment domain.PageComment) (domain.PageComment, error)
	GetByID(ctx context.Context, commentID string) (domain.PageComment, error)
	ListByPageID(ctx context.Context, pageID string) ([]domain.PageComment, error)
	Resolve(ctx context.Context, commentID string, resolvedBy string, resolvedAt time.Time) (domain.PageComment, error)
}

type CreateCommentInput struct {
	PageID string
	Body   string
}

type ResolveCommentInput struct {
	CommentID string
}

type CommentService struct {
	comments    CommentRepository
	pages       CommentPageRepository
	memberships WorkspaceMembershipReader
}

func NewCommentService(comments CommentRepository, pages CommentPageRepository, memberships WorkspaceMembershipReader) CommentService {
	return CommentService{
		comments:    comments,
		pages:       pages,
		memberships: memberships,
	}
}

func (s CommentService) CreateComment(ctx context.Context, actorID string, input CreateCommentInput) (domain.PageComment, error) {
	page, _, err := s.pages.GetByID(ctx, input.PageID)
	if err != nil {
		return domain.PageComment{}, err
	}

	if _, err := s.memberships.GetMembershipByUserID(ctx, page.WorkspaceID, actorID); err != nil {
		return domain.PageComment{}, err
	}

	body := strings.TrimSpace(input.Body)
	if body == "" {
		return domain.PageComment{}, fmt.Errorf("%w: comment body is required", domain.ErrValidation)
	}

	comment := domain.PageComment{
		ID:        uuid.NewString(),
		PageID:    page.ID,
		Body:      body,
		CreatedBy: actorID,
		CreatedAt: time.Now().UTC(),
	}

	return s.comments.Create(ctx, comment)
}

func (s CommentService) ListComments(ctx context.Context, actorID, pageID string) ([]domain.PageComment, error) {
	page, _, err := s.pages.GetByID(ctx, pageID)
	if err != nil {
		return nil, err
	}

	if _, err := s.memberships.GetMembershipByUserID(ctx, page.WorkspaceID, actorID); err != nil {
		return nil, err
	}

	return s.comments.ListByPageID(ctx, pageID)
}

func (s CommentService) ResolveComment(ctx context.Context, actorID string, input ResolveCommentInput) (domain.PageComment, error) {
	comment, err := s.comments.GetByID(ctx, input.CommentID)
	if err != nil {
		return domain.PageComment{}, err
	}

	page, _, err := s.pages.GetByID(ctx, comment.PageID)
	if err != nil {
		return domain.PageComment{}, err
	}

	membership, err := s.memberships.GetMembershipByUserID(ctx, page.WorkspaceID, actorID)
	if err != nil {
		return domain.PageComment{}, err
	}
	if membership.Role == domain.RoleViewer {
		return domain.PageComment{}, domain.ErrForbidden
	}

	if comment.ResolvedAt != nil {
		return comment, nil
	}

	return s.comments.Resolve(ctx, comment.ID, actorID, time.Now().UTC())
}
