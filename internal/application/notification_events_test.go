package application

import (
	"context"
	"testing"

	"note-app/internal/domain"
)

type fakeNotificationPublisher struct {
	invitations []domain.WorkspaceInvitation
	comments    []domain.PageComment
}

func (p *fakeNotificationPublisher) NotifyInvitationCreated(_ context.Context, invitation domain.WorkspaceInvitation) error {
	p.invitations = append(p.invitations, invitation)
	return nil
}

func (p *fakeNotificationPublisher) NotifyCommentCreated(_ context.Context, _ domain.Page, comment domain.PageComment) error {
	p.comments = append(p.comments, comment)
	return nil
}

func TestWorkspaceServiceInvitePublishesNotificationEvent(t *testing.T) {
	workspaces := &fakeWorkspaceRepo{
		memberships: map[string][]domain.WorkspaceMember{
			"workspace-1": {{ID: "member-1", WorkspaceID: "workspace-1", UserID: "owner-1", Role: domain.RoleOwner}},
		},
		invitations: map[string]domain.WorkspaceInvitation{},
		owners:      map[string]int{"workspace-1": 1},
	}
	users := &fakeUserRepo{
		byEmail: map[string]domain.User{"invitee@example.com": {ID: "invitee-1", Email: "invitee@example.com", FullName: "Invitee"}},
		byID:    map[string]domain.User{"owner-1": {ID: "owner-1", Email: "owner@example.com", FullName: "Owner"}},
	}
	publisher := &fakeNotificationPublisher{}
	service := NewWorkspaceService(workspaces, users, publisher)

	invitation, err := service.InviteMember(context.Background(), "owner-1", InviteMemberInput{
		WorkspaceID: "workspace-1",
		Email:       "invitee@example.com",
		Role:        domain.RoleViewer,
	})
	if err != nil {
		t.Fatalf("InviteMember() error = %v", err)
	}
	if len(publisher.invitations) != 1 || publisher.invitations[0].ID != invitation.ID {
		t.Fatalf("expected invitation publish event, got %+v", publisher.invitations)
	}
}

func TestCommentServiceCreatePublishesNotificationEvent(t *testing.T) {
	memberships := &fakeWorkspaceRepo{memberships: map[string][]domain.WorkspaceMember{
		"workspace-1": {
			{ID: "member-1", WorkspaceID: "workspace-1", UserID: "viewer-1", Role: domain.RoleViewer},
			{ID: "member-2", WorkspaceID: "workspace-1", UserID: "editor-1", Role: domain.RoleEditor},
		},
	}, invitations: map[string]domain.WorkspaceInvitation{}, owners: map[string]int{}}
	pages := &fakePageRepo{
		pages: map[string]domain.Page{
			"page-1": {ID: "page-1", WorkspaceID: "workspace-1", Title: "Doc"},
		},
		drafts: map[string]domain.PageDraft{
			"page-1": {PageID: "page-1"},
		},
	}
	comments := &fakeCommentRepo{comments: map[string]domain.PageComment{}, ordered: []domain.PageComment{}}
	publisher := &fakeNotificationPublisher{}
	service := NewCommentService(comments, pages, memberships, publisher)

	created, err := service.CreateComment(context.Background(), "viewer-1", CreateCommentInput{PageID: "page-1", Body: "looks good"})
	if err != nil {
		t.Fatalf("CreateComment() error = %v", err)
	}
	if len(publisher.comments) != 1 || publisher.comments[0].ID != created.ID {
		t.Fatalf("expected comment publish event, got %+v", publisher.comments)
	}
}
