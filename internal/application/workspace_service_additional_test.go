package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"note-app/internal/domain"
)

type workspaceRepoStub struct {
	createWithOwnerFn             func(context.Context, domain.Workspace, domain.WorkspaceMember) (domain.Workspace, domain.WorkspaceMember, error)
	hasWorkspaceWithNameForUserFn func(context.Context, string, string) (bool, error)
	getByIDFn                     func(context.Context, string) (domain.Workspace, error)
	updateNameFn                  func(context.Context, string, string, time.Time) (domain.Workspace, error)
	listByUserIDFn                func(context.Context, string) ([]domain.Workspace, error)
	getMembershipByUserIDFn       func(context.Context, string, string) (domain.WorkspaceMember, error)
	createInvitationFn            func(context.Context, domain.WorkspaceInvitation) (domain.WorkspaceInvitation, error)
	getActiveInvitationByEmailFn  func(context.Context, string, string) (domain.WorkspaceInvitation, error)
	getInvitationByIDFn           func(context.Context, string) (domain.WorkspaceInvitation, error)
	acceptInvitationFn            func(context.Context, string, string, time.Time) (domain.WorkspaceMember, error)
	listMembersFn                 func(context.Context, string) ([]domain.WorkspaceMember, error)
	updateMemberRoleFn            func(context.Context, string, string, domain.WorkspaceRole) (domain.WorkspaceMember, error)
	countOwnersFn                 func(context.Context, string) (int, error)
}

func (s workspaceRepoStub) CreateWithOwner(ctx context.Context, w domain.Workspace, m domain.WorkspaceMember) (domain.Workspace, domain.WorkspaceMember, error) {
	if s.createWithOwnerFn != nil {
		return s.createWithOwnerFn(ctx, w, m)
	}
	return w, m, nil
}
func (s workspaceRepoStub) HasWorkspaceWithNameForUser(ctx context.Context, userID, workspaceName string) (bool, error) {
	if s.hasWorkspaceWithNameForUserFn != nil {
		return s.hasWorkspaceWithNameForUserFn(ctx, userID, workspaceName)
	}
	return false, nil
}
func (s workspaceRepoStub) GetByID(ctx context.Context, workspaceID string) (domain.Workspace, error) {
	if s.getByIDFn != nil {
		return s.getByIDFn(ctx, workspaceID)
	}
	return domain.Workspace{}, domain.ErrNotFound
}
func (s workspaceRepoStub) UpdateName(ctx context.Context, workspaceID, name string, updatedAt time.Time) (domain.Workspace, error) {
	if s.updateNameFn != nil {
		return s.updateNameFn(ctx, workspaceID, name, updatedAt)
	}
	return domain.Workspace{ID: workspaceID, Name: name, UpdatedAt: updatedAt}, nil
}
func (s workspaceRepoStub) ListByUserID(ctx context.Context, userID string) ([]domain.Workspace, error) {
	if s.listByUserIDFn != nil {
		return s.listByUserIDFn(ctx, userID)
	}
	return []domain.Workspace{}, nil
}
func (s workspaceRepoStub) GetMembershipByUserID(ctx context.Context, wid, uid string) (domain.WorkspaceMember, error) {
	if s.getMembershipByUserIDFn != nil {
		return s.getMembershipByUserIDFn(ctx, wid, uid)
	}
	return domain.WorkspaceMember{}, domain.ErrForbidden
}
func (s workspaceRepoStub) CreateInvitation(ctx context.Context, i domain.WorkspaceInvitation) (domain.WorkspaceInvitation, error) {
	if s.createInvitationFn != nil {
		return s.createInvitationFn(ctx, i)
	}
	return i, nil
}
func (s workspaceRepoStub) GetActiveInvitationByEmail(ctx context.Context, wid, email string) (domain.WorkspaceInvitation, error) {
	if s.getActiveInvitationByEmailFn != nil {
		return s.getActiveInvitationByEmailFn(ctx, wid, email)
	}
	return domain.WorkspaceInvitation{}, domain.ErrNotFound
}
func (s workspaceRepoStub) GetInvitationByID(ctx context.Context, id string) (domain.WorkspaceInvitation, error) {
	if s.getInvitationByIDFn != nil {
		return s.getInvitationByIDFn(ctx, id)
	}
	return domain.WorkspaceInvitation{}, domain.ErrNotFound
}
func (s workspaceRepoStub) AcceptInvitation(ctx context.Context, invitationID, userID string, acceptedAt time.Time) (domain.WorkspaceMember, error) {
	if s.acceptInvitationFn != nil {
		return s.acceptInvitationFn(ctx, invitationID, userID, acceptedAt)
	}
	return domain.WorkspaceMember{ID: "m2", WorkspaceID: "w1", UserID: userID, Role: domain.RoleEditor, CreatedAt: acceptedAt}, nil
}
func (s workspaceRepoStub) ListMembers(ctx context.Context, workspaceID string) ([]domain.WorkspaceMember, error) {
	if s.listMembersFn != nil {
		return s.listMembersFn(ctx, workspaceID)
	}
	return nil, nil
}
func (s workspaceRepoStub) UpdateMemberRole(ctx context.Context, workspaceID, memberID string, role domain.WorkspaceRole) (domain.WorkspaceMember, error) {
	if s.updateMemberRoleFn != nil {
		return s.updateMemberRoleFn(ctx, workspaceID, memberID, role)
	}
	return domain.WorkspaceMember{ID: memberID, WorkspaceID: workspaceID, Role: role}, nil
}
func (s workspaceRepoStub) CountOwners(ctx context.Context, workspaceID string) (int, error) {
	if s.countOwnersFn != nil {
		return s.countOwnersFn(ctx, workspaceID)
	}
	return 1, nil
}

func TestWorkspaceServiceAdditionalBranches(t *testing.T) {
	users := authUserRepoStub{getByIDFn: func(context.Context, string) (domain.User, error) {
		return domain.User{ID: "user-1", Email: "owner@example.com"}, nil
	}, getByEmailFn: func(_ context.Context, email string) (domain.User, error) {
		return domain.User{ID: "member-1", Email: email}, nil
	}}

	t.Run("create workspace validation", func(t *testing.T) {
		svc := NewWorkspaceService(workspaceRepoStub{}, users)
		if _, _, err := svc.CreateWorkspace(context.Background(), "user-1", CreateWorkspaceInput{Name: "   "}); !errors.Is(err, domain.ErrValidation) {
			t.Fatalf("expected validation error, got %v", err)
		}
	})

	t.Run("create workspace requires valid actor", func(t *testing.T) {
		svc := NewWorkspaceService(workspaceRepoStub{}, authUserRepoStub{})
		if _, _, err := svc.CreateWorkspace(context.Background(), "missing-user", CreateWorkspaceInput{Name: "Team"}); !errors.Is(err, domain.ErrUnauthorized) {
			t.Fatalf("expected unauthorized for unknown actor, got %v", err)
		}
	})

	t.Run("create workspace rejects duplicate name for actor", func(t *testing.T) {
		svc := NewWorkspaceService(workspaceRepoStub{hasWorkspaceWithNameForUserFn: func(_ context.Context, userID, workspaceName string) (bool, error) {
			if userID != "user-1" || workspaceName != "Team" {
				t.Fatalf("unexpected duplicate check inputs userID=%s workspaceName=%s", userID, workspaceName)
			}
			return true, nil
		}}, users)
		if _, _, err := svc.CreateWorkspace(context.Background(), "user-1", CreateWorkspaceInput{Name: "Team"}); !errors.Is(err, domain.ErrValidation) {
			t.Fatalf("expected validation for duplicate workspace name, got %v", err)
		}
	})

	t.Run("list workspaces only for actor", func(t *testing.T) {
		svc := NewWorkspaceService(workspaceRepoStub{
			listByUserIDFn: func(_ context.Context, userID string) ([]domain.Workspace, error) {
				if userID != "user-1" {
					t.Fatalf("expected list call for actor user-1, got %s", userID)
				}
				return []domain.Workspace{{ID: "w1", Name: "Engineering"}}, nil
			},
		}, users)
		items, err := svc.ListWorkspaces(context.Background(), "user-1")
		if err != nil {
			t.Fatalf("expected list success, got %v", err)
		}
		if len(items) != 1 || items[0].ID != "w1" {
			t.Fatalf("unexpected workspace list: %+v", items)
		}
	})

	t.Run("rename workspace validation and auth", func(t *testing.T) {
		svc := NewWorkspaceService(workspaceRepoStub{}, users)
		if _, err := svc.RenameWorkspace(context.Background(), "user-1", RenameWorkspaceInput{WorkspaceID: "w1", Name: "   "}); !errors.Is(err, domain.ErrValidation) {
			t.Fatalf("expected rename validation error, got %v", err)
		}

		svc = NewWorkspaceService(workspaceRepoStub{
			getMembershipByUserIDFn: func(context.Context, string, string) (domain.WorkspaceMember, error) {
				return domain.WorkspaceMember{Role: domain.RoleEditor}, nil
			},
		}, users)
		if _, err := svc.RenameWorkspace(context.Background(), "user-1", RenameWorkspaceInput{WorkspaceID: "w1", Name: "New Name"}); !errors.Is(err, domain.ErrForbidden) {
			t.Fatalf("expected forbidden for non-owner rename, got %v", err)
		}

		svc = NewWorkspaceService(workspaceRepoStub{
			getMembershipByUserIDFn: func(context.Context, string, string) (domain.WorkspaceMember, error) {
				return domain.WorkspaceMember{Role: domain.RoleOwner}, nil
			},
			getByIDFn: func(context.Context, string) (domain.Workspace, error) {
				return domain.Workspace{ID: "w1", Name: "Engineering"}, nil
			},
			hasWorkspaceWithNameForUserFn: func(context.Context, string, string) (bool, error) {
				return true, nil
			},
		}, users)
		if _, err := svc.RenameWorkspace(context.Background(), "user-1", RenameWorkspaceInput{WorkspaceID: "w1", Name: "Product"}); !errors.Is(err, domain.ErrValidation) {
			t.Fatalf("expected duplicate rename validation, got %v", err)
		}

		svc = NewWorkspaceService(workspaceRepoStub{
			getMembershipByUserIDFn: func(context.Context, string, string) (domain.WorkspaceMember, error) {
				return domain.WorkspaceMember{Role: domain.RoleOwner}, nil
			},
			getByIDFn: func(context.Context, string) (domain.Workspace, error) {
				return domain.Workspace{ID: "w1", Name: "Engineering"}, nil
			},
			updateNameFn: func(_ context.Context, workspaceID, name string, updatedAt time.Time) (domain.Workspace, error) {
				return domain.Workspace{ID: workspaceID, Name: name, UpdatedAt: updatedAt}, nil
			},
		}, users)
		renamed, err := svc.RenameWorkspace(context.Background(), "user-1", RenameWorkspaceInput{WorkspaceID: "w1", Name: "  engineering  "})
		if err != nil {
			t.Fatalf("expected same-normalized rename to succeed, got %v", err)
		}
		if renamed.Name != "engineering" {
			t.Fatalf("expected trimmed rename result, got %q", renamed.Name)
		}
	})

	t.Run("invite member validation and auth", func(t *testing.T) {
		svc := NewWorkspaceService(workspaceRepoStub{}, users)
		if _, err := svc.InviteMember(context.Background(), "user-1", InviteMemberInput{WorkspaceID: "w1", Email: "a@b.com", Role: domain.WorkspaceRole("bad")}); !errors.Is(err, domain.ErrValidation) {
			t.Fatalf("expected invalid role validation, got %v", err)
		}

		svc = NewWorkspaceService(workspaceRepoStub{getMembershipByUserIDFn: func(context.Context, string, string) (domain.WorkspaceMember, error) {
			return domain.WorkspaceMember{Role: domain.RoleEditor}, nil
		}}, users)
		if _, err := svc.InviteMember(context.Background(), "user-1", InviteMemberInput{WorkspaceID: "w1", Email: "a@b.com", Role: domain.RoleViewer}); !errors.Is(err, domain.ErrForbidden) {
			t.Fatalf("expected forbidden for non-owner, got %v", err)
		}

		svc = NewWorkspaceService(workspaceRepoStub{getMembershipByUserIDFn: func(context.Context, string, string) (domain.WorkspaceMember, error) {
			return domain.WorkspaceMember{Role: domain.RoleOwner}, nil
		}}, users)
		if _, err := svc.InviteMember(context.Background(), "user-1", InviteMemberInput{WorkspaceID: "w1", Email: "bad-email", Role: domain.RoleViewer}); !errors.Is(err, domain.ErrValidation) {
			t.Fatalf("expected invalid email validation, got %v", err)
		}

		svc = NewWorkspaceService(workspaceRepoStub{getMembershipByUserIDFn: func(context.Context, string, string) (domain.WorkspaceMember, error) {
			return domain.WorkspaceMember{Role: domain.RoleOwner}, nil
		}}, authUserRepoStub{})
		if _, err := svc.InviteMember(context.Background(), "user-1", InviteMemberInput{WorkspaceID: "w1", Email: "missing@example.com", Role: domain.RoleViewer}); !errors.Is(err, domain.ErrValidation) {
			t.Fatalf("expected unregistered invitee validation, got %v", err)
		}

		svc = NewWorkspaceService(workspaceRepoStub{
			getMembershipByUserIDFn: func(context.Context, string, string) (domain.WorkspaceMember, error) {
				return domain.WorkspaceMember{Role: domain.RoleOwner}, nil
			},
			getActiveInvitationByEmailFn: func(context.Context, string, string) (domain.WorkspaceInvitation, error) {
				return domain.WorkspaceInvitation{ID: "inv-1"}, nil
			},
		}, users)
		if _, err := svc.InviteMember(context.Background(), "user-1", InviteMemberInput{WorkspaceID: "w1", Email: "a@b.com", Role: domain.RoleViewer}); !errors.Is(err, domain.ErrConflict) {
			t.Fatalf("expected conflict for duplicate invitation, got %v", err)
		}
	})

	t.Run("accept invitation mismatch and conflict", func(t *testing.T) {
		svc := NewWorkspaceService(workspaceRepoStub{getInvitationByIDFn: func(context.Context, string) (domain.WorkspaceInvitation, error) {
			acceptedAt := time.Now().UTC()
			return domain.WorkspaceInvitation{ID: "inv-1", WorkspaceID: "w1", Email: "owner@example.com", AcceptedAt: &acceptedAt}, nil
		}}, users)
		if _, err := svc.AcceptInvitation(context.Background(), "user-1", "inv-1"); !errors.Is(err, domain.ErrConflict) {
			t.Fatalf("expected conflict for accepted invitation, got %v", err)
		}

		svc = NewWorkspaceService(workspaceRepoStub{getInvitationByIDFn: func(context.Context, string) (domain.WorkspaceInvitation, error) {
			return domain.WorkspaceInvitation{ID: "inv-1", WorkspaceID: "w1", Email: "other@example.com"}, nil
		}}, users)
		if _, err := svc.AcceptInvitation(context.Background(), "user-1", "inv-1"); !errors.Is(err, domain.ErrInvitationEmailMismatch) {
			t.Fatalf("expected email mismatch error, got %v", err)
		}
	})

	t.Run("list members and update role branches", func(t *testing.T) {
		svc := NewWorkspaceService(workspaceRepoStub{getMembershipByUserIDFn: func(context.Context, string, string) (domain.WorkspaceMember, error) {
			return domain.WorkspaceMember{}, domain.ErrForbidden
		}}, users)
		if _, err := svc.ListMembers(context.Background(), "u1", "w1"); !errors.Is(err, domain.ErrForbidden) {
			t.Fatalf("expected forbidden, got %v", err)
		}

		svc = NewWorkspaceService(workspaceRepoStub{}, users)
		if _, err := svc.UpdateMemberRole(context.Background(), "u1", UpdateMemberRoleInput{WorkspaceID: "w1", MemberID: "m1", Role: domain.WorkspaceRole("bad")}); !errors.Is(err, domain.ErrValidation) {
			t.Fatalf("expected invalid role validation, got %v", err)
		}

		svc = NewWorkspaceService(workspaceRepoStub{getMembershipByUserIDFn: func(context.Context, string, string) (domain.WorkspaceMember, error) {
			return domain.WorkspaceMember{Role: domain.RoleEditor}, nil
		}}, users)
		if _, err := svc.UpdateMemberRole(context.Background(), "u1", UpdateMemberRoleInput{WorkspaceID: "w1", MemberID: "m1", Role: domain.RoleViewer}); !errors.Is(err, domain.ErrForbidden) {
			t.Fatalf("expected forbidden for non-owner actor, got %v", err)
		}

		svc = NewWorkspaceService(workspaceRepoStub{
			getMembershipByUserIDFn: func(context.Context, string, string) (domain.WorkspaceMember, error) {
				return domain.WorkspaceMember{Role: domain.RoleOwner}, nil
			},
			listMembersFn: func(context.Context, string) ([]domain.WorkspaceMember, error) {
				return nil, errors.New("list failed")
			},
		}, users)
		if _, err := svc.UpdateMemberRole(context.Background(), "u1", UpdateMemberRoleInput{WorkspaceID: "w1", MemberID: "m1", Role: domain.RoleViewer}); err == nil || err.Error() != "list failed" {
			t.Fatalf("expected list failure propagation, got %v", err)
		}

		svc = NewWorkspaceService(workspaceRepoStub{
			getMembershipByUserIDFn: func(context.Context, string, string) (domain.WorkspaceMember, error) {
				return domain.WorkspaceMember{Role: domain.RoleOwner}, nil
			},
			listMembersFn: func(context.Context, string) ([]domain.WorkspaceMember, error) {
				return []domain.WorkspaceMember{{ID: "m1", Role: domain.RoleOwner}}, nil
			},
			countOwnersFn: func(context.Context, string) (int, error) {
				return 0, errors.New("count failed")
			},
		}, users)
		if _, err := svc.UpdateMemberRole(context.Background(), "u1", UpdateMemberRoleInput{WorkspaceID: "w1", MemberID: "m1", Role: domain.RoleViewer}); err == nil || err.Error() != "count failed" {
			t.Fatalf("expected count failure propagation, got %v", err)
		}

		svc = NewWorkspaceService(workspaceRepoStub{
			getMembershipByUserIDFn: func(context.Context, string, string) (domain.WorkspaceMember, error) {
				return domain.WorkspaceMember{Role: domain.RoleOwner}, nil
			},
			listMembersFn: func(context.Context, string) ([]domain.WorkspaceMember, error) {
				return []domain.WorkspaceMember{{ID: "m1", Role: domain.RoleOwner}}, nil
			},
			countOwnersFn: func(context.Context, string) (int, error) {
				return 2, nil
			},
			updateMemberRoleFn: func(context.Context, string, string, domain.WorkspaceRole) (domain.WorkspaceMember, error) {
				return domain.WorkspaceMember{ID: "m1", Role: domain.RoleViewer}, nil
			},
		}, users)
		updated, err := svc.UpdateMemberRole(context.Background(), "u1", UpdateMemberRoleInput{WorkspaceID: "w1", MemberID: "m1", Role: domain.RoleViewer})
		if err != nil || updated.Role != domain.RoleViewer {
			t.Fatalf("expected successful owner demotion when owners remain, err=%v role=%s", err, updated.Role)
		}
	})
}
