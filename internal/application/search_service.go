package application

import (
	"context"
	"fmt"
	"strings"

	"note-app/internal/domain"
)

type SearchRepository interface {
	SearchPages(ctx context.Context, workspaceID string, query string) ([]domain.PageSearchResult, error)
}

type SearchInput struct {
	WorkspaceID string
	Query       string
}

type SearchService struct {
	searches    SearchRepository
	memberships WorkspaceMembershipReader
}

func NewSearchService(searches SearchRepository, memberships WorkspaceMembershipReader) SearchService {
	return SearchService{
		searches:    searches,
		memberships: memberships,
	}
}

func (s SearchService) SearchPages(ctx context.Context, actorID string, input SearchInput) ([]domain.PageSearchResult, error) {
	query := strings.TrimSpace(input.Query)
	if query == "" {
		return nil, fmt.Errorf("%w: query is required", domain.ErrValidation)
	}

	if _, err := s.memberships.GetMembershipByUserID(ctx, input.WorkspaceID, actorID); err != nil {
		return nil, err
	}

	return s.searches.SearchPages(ctx, input.WorkspaceID, query)
}
