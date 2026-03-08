package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"note-app/internal/domain"
)

func TestCommentServiceResolveMembershipError(t *testing.T) {
	repo := &commentRepoStub{getByIDFn: func(context.Context, string) (domain.PageComment, error) {
		return domain.PageComment{ID: "c1", PageID: "p1", Body: "x", CreatedBy: "u1", CreatedAt: time.Now().UTC()}, nil
	}}
	svc := NewCommentService(repo, commentPageRepoStub{getFn: func(context.Context, string) (domain.Page, domain.PageDraft, error) {
		return domain.Page{ID: "p1", WorkspaceID: "w1"}, domain.PageDraft{}, nil
	}}, commentMembershipStub{getFn: func(context.Context, string, string) (domain.WorkspaceMember, error) {
		return domain.WorkspaceMember{}, errors.New("membership failed")
	}})

	if _, err := svc.ResolveComment(context.Background(), "u1", ResolveCommentInput{CommentID: "c1"}); err == nil || err.Error() != "membership failed" {
		t.Fatalf("expected membership failure, got %v", err)
	}
}
