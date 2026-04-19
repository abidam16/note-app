package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"note-app/internal/domain"
)

type workspaceRepoStub struct {
	createWithOwnerFn                        func(context.Context, domain.Workspace, domain.WorkspaceMember) (domain.Workspace, domain.WorkspaceMember, error)
	hasWorkspaceWithNameForUserFn            func(context.Context, string, string) (bool, error)
	hasWorkspaceWithNameForUserExcludingIDFn func(context.Context, string, string, string) (bool, error)
	getByIDFn                                func(context.Context, string) (domain.Workspace, error)
	updateNameFn                             func(context.Context, string, string, time.Time) (domain.Workspace, error)
	listByUserIDFn                           func(context.Context, string) ([]domain.Workspace, error)
	getMembershipByUserIDFn                  func(context.Context, string, string) (domain.WorkspaceMember, error)
	getMembershipByIDFn                      func(context.Context, string, string) (domain.WorkspaceMember, error)
	createInvitationFn                       func(context.Context, domain.WorkspaceInvitation) (domain.WorkspaceInvitation, error)
	getActiveInvitationByEmailFn             func(context.Context, string, string) (domain.WorkspaceInvitation, error)
	getInvitationByIDFn                      func(context.Context, string) (domain.WorkspaceInvitation, error)
	acceptInvitationFn                       func(context.Context, string, string, int64, time.Time) (domain.AcceptInvitationResult, error)
	rejectInvitationFn                       func(context.Context, string, string, int64, time.Time) (domain.WorkspaceInvitation, error)
	cancelInvitationFn                       func(context.Context, string, string, int64, time.Time) (domain.WorkspaceInvitation, error)
	updateInvitationFn                       func(context.Context, string, domain.WorkspaceRole, int64, time.Time) (domain.WorkspaceInvitation, error)
	listWorkspaceInvitationsFn               func(context.Context, string, domain.WorkspaceInvitationStatusFilter, int, string) (domain.WorkspaceInvitationList, error)
	listMyInvitationsFn                      func(context.Context, string, domain.WorkspaceInvitationStatusFilter, int, string) (domain.WorkspaceInvitationList, error)
	listMembersFn                            func(context.Context, string) ([]domain.WorkspaceMember, error)
	updateMemberRoleFn                       func(context.Context, string, string, domain.WorkspaceRole) (domain.WorkspaceMember, error)
	countOwnersFn                            func(context.Context, string) (int, error)
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
func (s workspaceRepoStub) HasWorkspaceWithNameForUserExcludingID(ctx context.Context, userID, workspaceName, excludeWorkspaceID string) (bool, error) {
	if s.hasWorkspaceWithNameForUserExcludingIDFn != nil {
		return s.hasWorkspaceWithNameForUserExcludingIDFn(ctx, userID, workspaceName, excludeWorkspaceID)
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
func (s workspaceRepoStub) GetMembershipByID(ctx context.Context, wid, mid string) (domain.WorkspaceMember, error) {
	if s.getMembershipByIDFn != nil {
		return s.getMembershipByIDFn(ctx, wid, mid)
	}
	return domain.WorkspaceMember{}, domain.ErrNotFound
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
func (s workspaceRepoStub) AcceptInvitation(ctx context.Context, invitationID, userID string, version int64, acceptedAt time.Time) (domain.AcceptInvitationResult, error) {
	if s.acceptInvitationFn != nil {
		return s.acceptInvitationFn(ctx, invitationID, userID, version, acceptedAt)
	}
	return domain.AcceptInvitationResult{
		Invitation: domain.WorkspaceInvitation{
			ID:          invitationID,
			WorkspaceID: "w1",
			Email:       "owner@example.com",
			Role:        domain.RoleEditor,
			Status:      domain.WorkspaceInvitationStatusAccepted,
			Version:     version + 1,
			CreatedAt:   acceptedAt.Add(-time.Minute),
			UpdatedAt:   acceptedAt,
			AcceptedAt:  &acceptedAt,
			RespondedBy: &userID,
			RespondedAt: &acceptedAt,
		},
		Membership: domain.WorkspaceMember{ID: "m2", WorkspaceID: "w1", UserID: userID, Role: domain.RoleEditor, CreatedAt: acceptedAt},
	}, nil
}
func (s workspaceRepoStub) RejectInvitation(ctx context.Context, invitationID, userID string, version int64, rejectedAt time.Time) (domain.WorkspaceInvitation, error) {
	if s.rejectInvitationFn != nil {
		return s.rejectInvitationFn(ctx, invitationID, userID, version, rejectedAt)
	}
	return domain.WorkspaceInvitation{
		ID:          invitationID,
		WorkspaceID: "w1",
		Email:       "owner@example.com",
		Role:        domain.RoleEditor,
		Status:      domain.WorkspaceInvitationStatusRejected,
		Version:     version + 1,
		CreatedAt:   rejectedAt.Add(-time.Minute),
		UpdatedAt:   rejectedAt,
		RespondedBy: &userID,
		RespondedAt: &rejectedAt,
	}, nil
}
func (s workspaceRepoStub) CancelInvitation(ctx context.Context, invitationID, userID string, version int64, cancelledAt time.Time) (domain.WorkspaceInvitation, error) {
	if s.cancelInvitationFn != nil {
		return s.cancelInvitationFn(ctx, invitationID, userID, version, cancelledAt)
	}
	return domain.WorkspaceInvitation{
		ID:          invitationID,
		WorkspaceID: "w1",
		Email:       "owner@example.com",
		Role:        domain.RoleEditor,
		Status:      domain.WorkspaceInvitationStatusCancelled,
		Version:     version + 1,
		CreatedAt:   cancelledAt.Add(-time.Minute),
		UpdatedAt:   cancelledAt,
		CancelledBy: &userID,
		CancelledAt: &cancelledAt,
	}, nil
}
func (s workspaceRepoStub) UpdateInvitation(ctx context.Context, invitationID string, role domain.WorkspaceRole, version int64, updatedAt time.Time) (domain.WorkspaceInvitation, error) {
	if s.updateInvitationFn != nil {
		return s.updateInvitationFn(ctx, invitationID, role, version, updatedAt)
	}
	return domain.WorkspaceInvitation{}, domain.ErrNotFound
}
func (s workspaceRepoStub) ListWorkspaceInvitations(ctx context.Context, workspaceID string, status domain.WorkspaceInvitationStatusFilter, limit int, cursor string) (domain.WorkspaceInvitationList, error) {
	if s.listWorkspaceInvitationsFn != nil {
		return s.listWorkspaceInvitationsFn(ctx, workspaceID, status, limit, cursor)
	}
	return domain.WorkspaceInvitationList{}, nil
}
func (s workspaceRepoStub) ListMyInvitations(ctx context.Context, email string, status domain.WorkspaceInvitationStatusFilter, limit int, cursor string) (domain.WorkspaceInvitationList, error) {
	if s.listMyInvitationsFn != nil {
		return s.listMyInvitationsFn(ctx, email, status, limit, cursor)
	}
	return domain.WorkspaceInvitationList{}, nil
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
			hasWorkspaceWithNameForUserExcludingIDFn: func(context.Context, string, string, string) (bool, error) {
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

		svc = NewWorkspaceService(workspaceRepoStub{
			getMembershipByUserIDFn: func(context.Context, string, string) (domain.WorkspaceMember, error) {
				return domain.WorkspaceMember{Role: domain.RoleOwner}, nil
			},
		}, authUserRepoStub{getByIDFn: func(context.Context, string) (domain.User, error) {
			return domain.User{ID: "user-1", Email: "owner@example.com"}, nil
		}})
		if _, err := svc.InviteMember(context.Background(), "user-1", InviteMemberInput{WorkspaceID: "w1", Email: "missing@example.com", Role: domain.RoleViewer}); !errors.Is(err, domain.ErrInvitationUnregistered) {
			t.Fatalf("expected unregistered invitee conflict, got %v", err)
		}

		svc = NewWorkspaceService(workspaceRepoStub{
			getMembershipByUserIDFn: func(context.Context, string, string) (domain.WorkspaceMember, error) {
				return domain.WorkspaceMember{Role: domain.RoleOwner}, nil
			},
		}, authUserRepoStub{getByIDFn: func(context.Context, string) (domain.User, error) {
			return domain.User{ID: "user-1", Email: "owner@example.com"}, nil
		}})
		if _, err := svc.InviteMember(context.Background(), "user-1", InviteMemberInput{WorkspaceID: "w1", Email: " OWNER@example.com ", Role: domain.RoleViewer}); !errors.Is(err, domain.ErrInvitationSelfEmail) {
			t.Fatalf("expected self invite conflict, got %v", err)
		}

		svc = NewWorkspaceService(workspaceRepoStub{
			getMembershipByUserIDFn: func(_ context.Context, workspaceID, userID string) (domain.WorkspaceMember, error) {
				if workspaceID == "w1" && userID == "user-1" {
					return domain.WorkspaceMember{Role: domain.RoleOwner}, nil
				}
				return domain.WorkspaceMember{}, domain.ErrForbidden
			},
			getActiveInvitationByEmailFn: func(context.Context, string, string) (domain.WorkspaceInvitation, error) {
				return domain.WorkspaceInvitation{ID: "inv-1"}, nil
			},
		}, authUserRepoStub{
			getByIDFn: func(context.Context, string) (domain.User, error) {
				return domain.User{ID: "user-1", Email: "owner@example.com"}, nil
			},
			getByEmailFn: func(context.Context, string) (domain.User, error) {
				return domain.User{ID: "registered-1", Email: "a@b.com"}, nil
			},
		})
		if _, err := svc.InviteMember(context.Background(), "user-1", InviteMemberInput{WorkspaceID: "w1", Email: "a@b.com", Role: domain.RoleViewer}); !errors.Is(err, domain.ErrInvitationDuplicatePending) {
			t.Fatalf("expected conflict for duplicate invitation, got %v", err)
		}

		svc = NewWorkspaceService(workspaceRepoStub{
			getMembershipByUserIDFn: func(_ context.Context, workspaceID, userID string) (domain.WorkspaceMember, error) {
				switch {
				case workspaceID == "w1" && userID == "user-1":
					return domain.WorkspaceMember{Role: domain.RoleOwner}, nil
				case workspaceID == "w1" && userID == "member-1":
					return domain.WorkspaceMember{Role: domain.RoleViewer}, nil
				default:
					return domain.WorkspaceMember{}, domain.ErrForbidden
				}
			},
		}, authUserRepoStub{getByEmailFn: func(_ context.Context, email string) (domain.User, error) {
			if email != "member@example.com" {
				t.Fatalf("unexpected email lookup %q", email)
			}
			return domain.User{ID: "member-1", Email: email}, nil
		}, getByIDFn: func(context.Context, string) (domain.User, error) {
			return domain.User{ID: "user-1", Email: "owner@example.com"}, nil
		}})
		if _, err := svc.InviteMember(context.Background(), "user-1", InviteMemberInput{WorkspaceID: "w1", Email: "member@example.com", Role: domain.RoleViewer}); !errors.Is(err, domain.ErrInvitationExistingMember) {
			t.Fatalf("expected conflict for existing workspace member, got %v", err)
		}

		svc = NewWorkspaceService(workspaceRepoStub{
			getMembershipByUserIDFn: func(_ context.Context, workspaceID, userID string) (domain.WorkspaceMember, error) {
				if workspaceID == "w1" && userID == "user-1" {
					return domain.WorkspaceMember{Role: domain.RoleOwner}, nil
				}
				return domain.WorkspaceMember{}, domain.ErrForbidden
			},
			createInvitationFn: func(_ context.Context, invitation domain.WorkspaceInvitation) (domain.WorkspaceInvitation, error) {
				return invitation, nil
			},
		}, authUserRepoStub{
			getByIDFn: func(context.Context, string) (domain.User, error) {
				return domain.User{ID: "user-1", Email: "owner@example.com"}, nil
			},
			getByEmailFn: func(_ context.Context, email string) (domain.User, error) {
				if email != "registered@example.com" {
					t.Fatalf("unexpected registered invite lookup %q", email)
				}
				return domain.User{ID: "registered-1", Email: email}, nil
			},
		})
		invitation, err := svc.InviteMember(context.Background(), "user-1", InviteMemberInput{WorkspaceID: "w1", Email: "Registered@example.com", Role: domain.RoleViewer})
		if err != nil {
			t.Fatalf("expected registered invitee success, got %v", err)
		}
		if invitation.Email != "registered@example.com" {
			t.Fatalf("expected normalized invitation email, got %+v", invitation)
		}
		if invitation.Status != domain.WorkspaceInvitationStatusPending {
			t.Fatalf("expected pending invitation status, got %+v", invitation)
		}
		if invitation.Version != 1 {
			t.Fatalf("expected invitation version 1, got %+v", invitation)
		}
		if !invitation.UpdatedAt.Equal(invitation.CreatedAt) {
			t.Fatalf("expected invitation updated_at to equal created_at, got %+v", invitation)
		}
	})

	t.Run("accept invitation requires valid version and hides foreign invitations", func(t *testing.T) {
		svc := NewWorkspaceService(workspaceRepoStub{}, users)
		if _, err := svc.AcceptInvitation(context.Background(), "user-1", AcceptInvitationInput{InvitationID: "inv-1", Version: 0}); !errors.Is(err, domain.ErrValidation) {
			t.Fatalf("expected version validation, got %v", err)
		}

		svc = NewWorkspaceService(workspaceRepoStub{getInvitationByIDFn: func(context.Context, string) (domain.WorkspaceInvitation, error) {
			return domain.WorkspaceInvitation{ID: "inv-1", WorkspaceID: "w1", Email: "other@example.com", Status: domain.WorkspaceInvitationStatusPending, Version: 2}, nil
		}}, users)
		if _, err := svc.AcceptInvitation(context.Background(), "user-1", AcceptInvitationInput{InvitationID: "inv-1", Version: 2}); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("expected mismatched invitation to appear not found, got %v", err)
		}
	})

	t.Run("accept invitation handles terminal state and stale version", func(t *testing.T) {
		svc := NewWorkspaceService(workspaceRepoStub{getInvitationByIDFn: func(context.Context, string) (domain.WorkspaceInvitation, error) {
			acceptedAt := time.Now().UTC()
			return domain.WorkspaceInvitation{ID: "inv-1", WorkspaceID: "w1", Email: "owner@example.com", AcceptedAt: &acceptedAt, Status: domain.WorkspaceInvitationStatusAccepted, Version: 3}, nil
		}}, users)
		if _, err := svc.AcceptInvitation(context.Background(), "user-1", AcceptInvitationInput{InvitationID: "inv-1", Version: 3}); !errors.Is(err, domain.ErrConflict) {
			t.Fatalf("expected conflict for accepted invitation, got %v", err)
		}

		svc = NewWorkspaceService(workspaceRepoStub{
			getInvitationByIDFn: func(context.Context, string) (domain.WorkspaceInvitation, error) {
				return domain.WorkspaceInvitation{ID: "inv-1", WorkspaceID: "w1", Email: "owner@example.com", Status: domain.WorkspaceInvitationStatusPending, Version: 4}, nil
			},
			acceptInvitationFn: func(context.Context, string, string, int64, time.Time) (domain.AcceptInvitationResult, error) {
				return domain.AcceptInvitationResult{}, domain.ErrConflict
			},
		}, users)
		if _, err := svc.AcceptInvitation(context.Background(), "user-1", AcceptInvitationInput{InvitationID: "inv-1", Version: 3}); !errors.Is(err, domain.ErrConflict) {
			t.Fatalf("expected stale version conflict, got %v", err)
		}
	})

	t.Run("accept invitation returns invitation and membership", func(t *testing.T) {
		now := time.Now().UTC().Truncate(time.Second)
		svc := NewWorkspaceService(workspaceRepoStub{
			getInvitationByIDFn: func(context.Context, string) (domain.WorkspaceInvitation, error) {
				return domain.WorkspaceInvitation{
					ID:          "inv-1",
					WorkspaceID: "w1",
					Email:       "owner@example.com",
					Role:        domain.RoleEditor,
					Status:      domain.WorkspaceInvitationStatusPending,
					Version:     2,
					CreatedAt:   now.Add(-time.Hour),
					UpdatedAt:   now.Add(-30 * time.Minute),
				}, nil
			},
			acceptInvitationFn: func(_ context.Context, invitationID, userID string, version int64, acceptedAt time.Time) (domain.AcceptInvitationResult, error) {
				if invitationID != "inv-1" || userID != "user-1" || version != 2 {
					t.Fatalf("unexpected accept args id=%s userID=%s version=%d", invitationID, userID, version)
				}
				return domain.AcceptInvitationResult{
					Invitation: domain.WorkspaceInvitation{
						ID:          invitationID,
						WorkspaceID: "w1",
						Email:       "owner@example.com",
						Role:        domain.RoleEditor,
						Status:      domain.WorkspaceInvitationStatusAccepted,
						Version:     3,
						CreatedAt:   now.Add(-time.Hour),
						UpdatedAt:   acceptedAt,
						AcceptedAt:  &acceptedAt,
						RespondedBy: &userID,
						RespondedAt: &acceptedAt,
					},
					Membership: domain.WorkspaceMember{
						ID:          "m1",
						WorkspaceID: "w1",
						UserID:      userID,
						Role:        domain.RoleEditor,
						CreatedAt:   acceptedAt,
					},
				}, nil
			},
		}, users)
		result, err := svc.AcceptInvitation(context.Background(), "user-1", AcceptInvitationInput{InvitationID: "inv-1", Version: 2})
		if err != nil {
			t.Fatalf("expected accept success, got %v", err)
		}
		if result.Invitation.Status != domain.WorkspaceInvitationStatusAccepted || result.Invitation.Version != 3 {
			t.Fatalf("unexpected accepted invitation: %+v", result.Invitation)
		}
		if result.Membership.UserID != "user-1" || result.Membership.Role != domain.RoleEditor {
			t.Fatalf("unexpected membership: %+v", result.Membership)
		}
	})

	t.Run("reject invitation requires valid version and hides foreign invitations", func(t *testing.T) {
		svc := NewWorkspaceService(workspaceRepoStub{}, users)
		if _, err := svc.RejectInvitation(context.Background(), "user-1", RejectInvitationInput{InvitationID: "inv-1", Version: 0}); !errors.Is(err, domain.ErrValidation) {
			t.Fatalf("expected version validation, got %v", err)
		}

		svc = NewWorkspaceService(workspaceRepoStub{getInvitationByIDFn: func(context.Context, string) (domain.WorkspaceInvitation, error) {
			return domain.WorkspaceInvitation{ID: "inv-1", WorkspaceID: "w1", Email: "other@example.com", Status: domain.WorkspaceInvitationStatusPending, Version: 2}, nil
		}}, users)
		if _, err := svc.RejectInvitation(context.Background(), "user-1", RejectInvitationInput{InvitationID: "inv-1", Version: 2}); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("expected mismatched invitation to appear not found, got %v", err)
		}
	})

	t.Run("reject invitation handles terminal state and stale version", func(t *testing.T) {
		terminalStatuses := []domain.WorkspaceInvitationStatus{
			domain.WorkspaceInvitationStatusAccepted,
			domain.WorkspaceInvitationStatusRejected,
			domain.WorkspaceInvitationStatusCancelled,
		}
		for _, status := range terminalStatuses {
			t.Run(string(status), func(t *testing.T) {
				svc := NewWorkspaceService(workspaceRepoStub{getInvitationByIDFn: func(context.Context, string) (domain.WorkspaceInvitation, error) {
					invitation := domain.WorkspaceInvitation{ID: "inv-1", WorkspaceID: "w1", Email: "owner@example.com", Status: status, Version: 3}
					if status == domain.WorkspaceInvitationStatusAccepted {
						acceptedAt := time.Now().UTC()
						invitation.AcceptedAt = &acceptedAt
					}
					return invitation, nil
				}}, users)
				if _, err := svc.RejectInvitation(context.Background(), "user-1", RejectInvitationInput{InvitationID: "inv-1", Version: 3}); !errors.Is(err, domain.ErrConflict) {
					t.Fatalf("expected conflict for %s invitation, got %v", status, err)
				}
			})
		}

		svc := NewWorkspaceService(workspaceRepoStub{
			getInvitationByIDFn: func(context.Context, string) (domain.WorkspaceInvitation, error) {
				return domain.WorkspaceInvitation{ID: "inv-1", WorkspaceID: "w1", Email: "owner@example.com", Status: domain.WorkspaceInvitationStatusPending, Version: 4}, nil
			},
			rejectInvitationFn: func(context.Context, string, string, int64, time.Time) (domain.WorkspaceInvitation, error) {
				return domain.WorkspaceInvitation{}, domain.ErrConflict
			},
		}, users)
		if _, err := svc.RejectInvitation(context.Background(), "user-1", RejectInvitationInput{InvitationID: "inv-1", Version: 3}); !errors.Is(err, domain.ErrConflict) {
			t.Fatalf("expected stale version conflict, got %v", err)
		}
	})

	t.Run("reject invitation returns rejected invitation", func(t *testing.T) {
		now := time.Now().UTC().Truncate(time.Second)
		svc := NewWorkspaceService(workspaceRepoStub{
			getInvitationByIDFn: func(context.Context, string) (domain.WorkspaceInvitation, error) {
				return domain.WorkspaceInvitation{
					ID:          "inv-1",
					WorkspaceID: "w1",
					Email:       "owner@example.com",
					Role:        domain.RoleEditor,
					Status:      domain.WorkspaceInvitationStatusPending,
					Version:     2,
					CreatedAt:   now.Add(-time.Hour),
					UpdatedAt:   now.Add(-30 * time.Minute),
				}, nil
			},
			rejectInvitationFn: func(_ context.Context, invitationID, userID string, version int64, rejectedAt time.Time) (domain.WorkspaceInvitation, error) {
				if invitationID != "inv-1" || userID != "user-1" || version != 2 {
					t.Fatalf("unexpected reject args id=%s userID=%s version=%d", invitationID, userID, version)
				}
				return domain.WorkspaceInvitation{
					ID:          invitationID,
					WorkspaceID: "w1",
					Email:       "owner@example.com",
					Role:        domain.RoleEditor,
					Status:      domain.WorkspaceInvitationStatusRejected,
					Version:     3,
					CreatedAt:   now.Add(-time.Hour),
					UpdatedAt:   rejectedAt,
					RespondedBy: &userID,
					RespondedAt: &rejectedAt,
				}, nil
			},
		}, users)
		result, err := svc.RejectInvitation(context.Background(), "user-1", RejectInvitationInput{InvitationID: "inv-1", Version: 2})
		if err != nil {
			t.Fatalf("expected reject success, got %v", err)
		}
		if result.Status != domain.WorkspaceInvitationStatusRejected || result.Version != 3 {
			t.Fatalf("unexpected rejected invitation: %+v", result)
		}
		if result.RespondedBy == nil || *result.RespondedBy != "user-1" || result.RespondedAt == nil {
			t.Fatalf("unexpected rejection metadata: %+v", result)
		}
		if result.AcceptedAt != nil {
			t.Fatalf("expected accepted_at to remain nil, got %+v", result)
		}
	})

	t.Run("cancel invitation validates version and membership visibility", func(t *testing.T) {
		svc := NewWorkspaceService(workspaceRepoStub{}, users)
		if _, err := svc.CancelInvitation(context.Background(), "user-1", CancelInvitationInput{InvitationID: "inv-1", Version: 0}); !errors.Is(err, domain.ErrValidation) {
			t.Fatalf("expected version validation, got %v", err)
		}

		svc = NewWorkspaceService(workspaceRepoStub{
			getInvitationByIDFn: func(context.Context, string) (domain.WorkspaceInvitation, error) {
				return domain.WorkspaceInvitation{ID: "inv-1", WorkspaceID: "w1", Email: "member@example.com", Status: domain.WorkspaceInvitationStatusPending, Version: 2}, nil
			},
			getMembershipByUserIDFn: func(context.Context, string, string) (domain.WorkspaceMember, error) {
				return domain.WorkspaceMember{}, domain.ErrForbidden
			},
		}, users)
		if _, err := svc.CancelInvitation(context.Background(), "user-1", CancelInvitationInput{InvitationID: "inv-1", Version: 2}); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("expected outsider to see not found, got %v", err)
		}

		svc = NewWorkspaceService(workspaceRepoStub{
			getInvitationByIDFn: func(context.Context, string) (domain.WorkspaceInvitation, error) {
				return domain.WorkspaceInvitation{ID: "inv-1", WorkspaceID: "w1", Email: "member@example.com", Status: domain.WorkspaceInvitationStatusPending, Version: 2}, nil
			},
			getMembershipByUserIDFn: func(context.Context, string, string) (domain.WorkspaceMember, error) {
				return domain.WorkspaceMember{Role: domain.RoleEditor}, nil
			},
		}, users)
		if _, err := svc.CancelInvitation(context.Background(), "user-1", CancelInvitationInput{InvitationID: "inv-1", Version: 2}); !errors.Is(err, domain.ErrForbidden) {
			t.Fatalf("expected non-owner member forbidden, got %v", err)
		}
	})

	t.Run("cancel invitation handles terminal state and stale version", func(t *testing.T) {
		terminalStatuses := []domain.WorkspaceInvitationStatus{
			domain.WorkspaceInvitationStatusAccepted,
			domain.WorkspaceInvitationStatusRejected,
			domain.WorkspaceInvitationStatusCancelled,
		}
		for _, status := range terminalStatuses {
			t.Run(string(status), func(t *testing.T) {
				svc := NewWorkspaceService(workspaceRepoStub{
					getInvitationByIDFn: func(context.Context, string) (domain.WorkspaceInvitation, error) {
						invitation := domain.WorkspaceInvitation{ID: "inv-1", WorkspaceID: "w1", Email: "member@example.com", Status: status, Version: 3}
						if status == domain.WorkspaceInvitationStatusAccepted {
							acceptedAt := time.Now().UTC()
							invitation.AcceptedAt = &acceptedAt
						}
						return invitation, nil
					},
					getMembershipByUserIDFn: func(context.Context, string, string) (domain.WorkspaceMember, error) {
						return domain.WorkspaceMember{Role: domain.RoleOwner}, nil
					},
				}, users)
				if _, err := svc.CancelInvitation(context.Background(), "user-1", CancelInvitationInput{InvitationID: "inv-1", Version: 3}); !errors.Is(err, domain.ErrConflict) {
					t.Fatalf("expected conflict for %s invitation, got %v", status, err)
				}
			})
		}

		svc := NewWorkspaceService(workspaceRepoStub{
			getInvitationByIDFn: func(context.Context, string) (domain.WorkspaceInvitation, error) {
				return domain.WorkspaceInvitation{ID: "inv-1", WorkspaceID: "w1", Email: "member@example.com", Status: domain.WorkspaceInvitationStatusPending, Version: 4}, nil
			},
			getMembershipByUserIDFn: func(context.Context, string, string) (domain.WorkspaceMember, error) {
				return domain.WorkspaceMember{Role: domain.RoleOwner}, nil
			},
			cancelInvitationFn: func(context.Context, string, string, int64, time.Time) (domain.WorkspaceInvitation, error) {
				return domain.WorkspaceInvitation{}, domain.ErrConflict
			},
		}, users)
		if _, err := svc.CancelInvitation(context.Background(), "user-1", CancelInvitationInput{InvitationID: "inv-1", Version: 3}); !errors.Is(err, domain.ErrConflict) {
			t.Fatalf("expected stale version conflict, got %v", err)
		}
	})

	t.Run("cancel invitation returns cancelled invitation", func(t *testing.T) {
		now := time.Now().UTC().Truncate(time.Second)
		svc := NewWorkspaceService(workspaceRepoStub{
			getInvitationByIDFn: func(context.Context, string) (domain.WorkspaceInvitation, error) {
				return domain.WorkspaceInvitation{
					ID:          "inv-1",
					WorkspaceID: "w1",
					Email:       "member@example.com",
					Role:        domain.RoleEditor,
					Status:      domain.WorkspaceInvitationStatusPending,
					Version:     2,
					CreatedAt:   now.Add(-time.Hour),
					UpdatedAt:   now.Add(-30 * time.Minute),
				}, nil
			},
			getMembershipByUserIDFn: func(context.Context, string, string) (domain.WorkspaceMember, error) {
				return domain.WorkspaceMember{Role: domain.RoleOwner}, nil
			},
			cancelInvitationFn: func(_ context.Context, invitationID, userID string, version int64, cancelledAt time.Time) (domain.WorkspaceInvitation, error) {
				if invitationID != "inv-1" || userID != "user-1" || version != 2 {
					t.Fatalf("unexpected cancel args id=%s userID=%s version=%d", invitationID, userID, version)
				}
				return domain.WorkspaceInvitation{
					ID:          invitationID,
					WorkspaceID: "w1",
					Email:       "member@example.com",
					Role:        domain.RoleEditor,
					Status:      domain.WorkspaceInvitationStatusCancelled,
					Version:     3,
					CreatedAt:   now.Add(-time.Hour),
					UpdatedAt:   cancelledAt,
					CancelledBy: &userID,
					CancelledAt: &cancelledAt,
				}, nil
			},
		}, users)
		result, err := svc.CancelInvitation(context.Background(), "user-1", CancelInvitationInput{InvitationID: "inv-1", Version: 2})
		if err != nil {
			t.Fatalf("expected cancel success, got %v", err)
		}
		if result.Status != domain.WorkspaceInvitationStatusCancelled || result.Version != 3 {
			t.Fatalf("unexpected cancelled invitation: %+v", result)
		}
		if result.CancelledBy == nil || *result.CancelledBy != "user-1" || result.CancelledAt == nil {
			t.Fatalf("unexpected cancel metadata: %+v", result)
		}
		if result.AcceptedAt != nil || result.RespondedAt != nil || result.RespondedBy != nil {
			t.Fatalf("expected accept/respond fields to remain nil, got %+v", result)
		}
	})

	t.Run("update invitation validates role and version", func(t *testing.T) {
		svc := NewWorkspaceService(workspaceRepoStub{}, users)
		if _, err := svc.UpdateInvitation(context.Background(), "user-1", UpdateInvitationInput{
			InvitationID: "inv-1",
			Role:         domain.WorkspaceRole("bad"),
			Version:      1,
		}); !errors.Is(err, domain.ErrValidation) {
			t.Fatalf("expected invalid role validation, got %v", err)
		}
		if _, err := svc.UpdateInvitation(context.Background(), "user-1", UpdateInvitationInput{
			InvitationID: "inv-1",
			Role:         domain.RoleEditor,
			Version:      0,
		}); !errors.Is(err, domain.ErrValidation) {
			t.Fatalf("expected version validation, got %v", err)
		}
	})

	t.Run("update invitation requires owner and pending invitation", func(t *testing.T) {
		pending := domain.WorkspaceInvitation{
			ID:          "inv-1",
			WorkspaceID: "w1",
			Email:       "member@example.com",
			Role:        domain.RoleViewer,
			Status:      domain.WorkspaceInvitationStatusPending,
			Version:     3,
			CreatedAt:   time.Now().UTC().Add(-time.Hour),
			UpdatedAt:   time.Now().UTC().Add(-30 * time.Minute),
		}
		svc := NewWorkspaceService(workspaceRepoStub{
			getInvitationByIDFn: func(context.Context, string) (domain.WorkspaceInvitation, error) {
				return pending, nil
			},
			getMembershipByUserIDFn: func(context.Context, string, string) (domain.WorkspaceMember, error) {
				return domain.WorkspaceMember{Role: domain.RoleEditor}, nil
			},
		}, users)
		if _, err := svc.UpdateInvitation(context.Background(), "user-1", UpdateInvitationInput{
			InvitationID: "inv-1",
			Role:         domain.RoleEditor,
			Version:      3,
		}); !errors.Is(err, domain.ErrForbidden) {
			t.Fatalf("expected forbidden for non-owner, got %v", err)
		}

		terminalCases := []domain.WorkspaceInvitationStatus{
			domain.WorkspaceInvitationStatusAccepted,
			domain.WorkspaceInvitationStatusRejected,
			domain.WorkspaceInvitationStatusCancelled,
		}
		for _, status := range terminalCases {
			t.Run(string(status), func(t *testing.T) {
				svc := NewWorkspaceService(workspaceRepoStub{
					getInvitationByIDFn: func(context.Context, string) (domain.WorkspaceInvitation, error) {
						invitation := pending
						invitation.Status = status
						if status == domain.WorkspaceInvitationStatusAccepted {
							acceptedAt := time.Now().UTC()
							invitation.AcceptedAt = &acceptedAt
						}
						return invitation, nil
					},
					getMembershipByUserIDFn: func(context.Context, string, string) (domain.WorkspaceMember, error) {
						return domain.WorkspaceMember{Role: domain.RoleOwner}, nil
					},
				}, users)
				if _, err := svc.UpdateInvitation(context.Background(), "user-1", UpdateInvitationInput{
					InvitationID: "inv-1",
					Role:         domain.RoleEditor,
					Version:      3,
				}); !errors.Is(err, domain.ErrConflict) {
					t.Fatalf("expected conflict for %s invitation, got %v", status, err)
				}
			})
		}
	})

	t.Run("update invitation handles stale version, no-op, and success", func(t *testing.T) {
		now := time.Now().UTC().Truncate(time.Second)
		current := domain.WorkspaceInvitation{
			ID:          "inv-1",
			WorkspaceID: "w1",
			Email:       "member@example.com",
			Role:        domain.RoleViewer,
			Status:      domain.WorkspaceInvitationStatusPending,
			Version:     6,
			CreatedAt:   now.Add(-time.Hour),
			UpdatedAt:   now.Add(-20 * time.Minute),
		}

		svc := NewWorkspaceService(workspaceRepoStub{
			getInvitationByIDFn: func(context.Context, string) (domain.WorkspaceInvitation, error) {
				return current, nil
			},
			getMembershipByUserIDFn: func(context.Context, string, string) (domain.WorkspaceMember, error) {
				return domain.WorkspaceMember{Role: domain.RoleOwner}, nil
			},
			updateInvitationFn: func(context.Context, string, domain.WorkspaceRole, int64, time.Time) (domain.WorkspaceInvitation, error) {
				return domain.WorkspaceInvitation{}, domain.ErrConflict
			},
		}, users)
		if _, err := svc.UpdateInvitation(context.Background(), "user-1", UpdateInvitationInput{
			InvitationID: "inv-1",
			Role:         domain.RoleEditor,
			Version:      5,
		}); !errors.Is(err, domain.ErrConflict) {
			t.Fatalf("expected stale version conflict, got %v", err)
		}

		called := false
		svc = NewWorkspaceService(workspaceRepoStub{
			getInvitationByIDFn: func(context.Context, string) (domain.WorkspaceInvitation, error) {
				return current, nil
			},
			getMembershipByUserIDFn: func(context.Context, string, string) (domain.WorkspaceMember, error) {
				return domain.WorkspaceMember{Role: domain.RoleOwner}, nil
			},
			updateInvitationFn: func(context.Context, string, domain.WorkspaceRole, int64, time.Time) (domain.WorkspaceInvitation, error) {
				called = true
				return domain.WorkspaceInvitation{}, nil
			},
		}, users)
		unchanged, err := svc.UpdateInvitation(context.Background(), "user-1", UpdateInvitationInput{
			InvitationID: "inv-1",
			Role:         domain.RoleViewer,
			Version:      6,
		})
		if err != nil {
			t.Fatalf("expected no-op success, got %v", err)
		}
		if called {
			t.Fatal("expected no repository update for same-role no-op")
		}
		if unchanged.Version != current.Version || !unchanged.UpdatedAt.Equal(current.UpdatedAt) || unchanged.Role != current.Role {
			t.Fatalf("expected unchanged invitation, got %+v", unchanged)
		}

		svc = NewWorkspaceService(workspaceRepoStub{
			getInvitationByIDFn: func(context.Context, string) (domain.WorkspaceInvitation, error) {
				return current, nil
			},
			getMembershipByUserIDFn: func(context.Context, string, string) (domain.WorkspaceMember, error) {
				return domain.WorkspaceMember{Role: domain.RoleOwner}, nil
			},
			updateInvitationFn: func(_ context.Context, invitationID string, role domain.WorkspaceRole, version int64, updatedAt time.Time) (domain.WorkspaceInvitation, error) {
				if invitationID != "inv-1" || role != domain.RoleEditor || version != 6 {
					t.Fatalf("unexpected update args id=%s role=%s version=%d", invitationID, role, version)
				}
				return domain.WorkspaceInvitation{
					ID:          invitationID,
					WorkspaceID: "w1",
					Email:       "member@example.com",
					Role:        role,
					Status:      domain.WorkspaceInvitationStatusPending,
					Version:     7,
					CreatedAt:   current.CreatedAt,
					UpdatedAt:   updatedAt,
				}, nil
			},
		}, users)
		updated, err := svc.UpdateInvitation(context.Background(), "user-1", UpdateInvitationInput{
			InvitationID: "inv-1",
			Role:         domain.RoleEditor,
			Version:      6,
		})
		if err != nil {
			t.Fatalf("expected update success, got %v", err)
		}
		if updated.Role != domain.RoleEditor || updated.Version != 7 || !updated.UpdatedAt.After(current.UpdatedAt) {
			t.Fatalf("unexpected updated invitation: %+v", updated)
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
			getMembershipByIDFn: func(context.Context, string, string) (domain.WorkspaceMember, error) {
				return domain.WorkspaceMember{}, errors.New("target failed")
			},
		}, users)
		if _, err := svc.UpdateMemberRole(context.Background(), "u1", UpdateMemberRoleInput{WorkspaceID: "w1", MemberID: "m1", Role: domain.RoleViewer}); err == nil || err.Error() != "target failed" {
			t.Fatalf("expected target lookup failure propagation, got %v", err)
		}

		svc = NewWorkspaceService(workspaceRepoStub{
			getMembershipByUserIDFn: func(context.Context, string, string) (domain.WorkspaceMember, error) {
				return domain.WorkspaceMember{Role: domain.RoleOwner}, nil
			},
			getMembershipByIDFn: func(context.Context, string, string) (domain.WorkspaceMember, error) {
				return domain.WorkspaceMember{ID: "m1", Role: domain.RoleOwner}, nil
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
			getMembershipByIDFn: func(context.Context, string, string) (domain.WorkspaceMember, error) {
				return domain.WorkspaceMember{ID: "m1", Role: domain.RoleOwner}, nil
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

	t.Run("list workspace invitations validation and auth", func(t *testing.T) {
		svc := NewWorkspaceService(workspaceRepoStub{}, users)
		if _, err := svc.ListWorkspaceInvitations(context.Background(), "user-1", ListWorkspaceInvitationsInput{
			WorkspaceID: "w1",
			Status:      "bad",
		}); !errors.Is(err, domain.ErrValidation) {
			t.Fatalf("expected invalid status validation, got %v", err)
		}

		if _, err := svc.ListWorkspaceInvitations(context.Background(), "user-1", ListWorkspaceInvitationsInput{
			WorkspaceID: "w1",
			Limit:       0,
		}); !errors.Is(err, domain.ErrValidation) {
			t.Fatalf("expected invalid limit validation, got %v", err)
		}

		svc = NewWorkspaceService(workspaceRepoStub{
			getMembershipByUserIDFn: func(context.Context, string, string) (domain.WorkspaceMember, error) {
				return domain.WorkspaceMember{Role: domain.RoleEditor}, nil
			},
		}, users)
		if _, err := svc.ListWorkspaceInvitations(context.Background(), "user-1", ListWorkspaceInvitationsInput{
			WorkspaceID: "w1",
			Limit:       -1,
		}); !errors.Is(err, domain.ErrForbidden) {
			t.Fatalf("expected non-owner list forbidden, got %v", err)
		}

		expected := domain.WorkspaceInvitationList{
			Items: []domain.WorkspaceInvitation{
				{ID: "inv-2", WorkspaceID: "w1", Email: "second@example.com", Status: domain.WorkspaceInvitationStatusPending, Version: 1},
				{ID: "inv-1", WorkspaceID: "w1", Email: "first@example.com", Status: domain.WorkspaceInvitationStatusAccepted, Version: 2},
			},
			HasMore: true,
		}
		next := "cursor-2"
		expected.NextCursor = &next
		svc = NewWorkspaceService(workspaceRepoStub{
			getMembershipByUserIDFn: func(context.Context, string, string) (domain.WorkspaceMember, error) {
				return domain.WorkspaceMember{Role: domain.RoleOwner}, nil
			},
			listWorkspaceInvitationsFn: func(_ context.Context, workspaceID string, status domain.WorkspaceInvitationStatusFilter, limit int, cursor string) (domain.WorkspaceInvitationList, error) {
				if workspaceID != "w1" || status != domain.WorkspaceInvitationStatusFilterPending || limit != 25 || cursor != "cursor-1" {
					t.Fatalf("unexpected list args workspaceID=%s status=%s limit=%d cursor=%q", workspaceID, status, limit, cursor)
				}
				return expected, nil
			},
		}, users)
		result, err := svc.ListWorkspaceInvitations(context.Background(), "user-1", ListWorkspaceInvitationsInput{
			WorkspaceID: "w1",
			Status:      domain.WorkspaceInvitationStatusFilterPending,
			Limit:       25,
			Cursor:      "cursor-1",
		})
		if err != nil {
			t.Fatalf("expected list success, got %v", err)
		}
		if len(result.Items) != 2 || !result.HasMore || result.NextCursor == nil || *result.NextCursor != next {
			t.Fatalf("unexpected invitation list result: %+v", result)
		}
	})

	t.Run("list my invitations validation and auth", func(t *testing.T) {
		svc := NewWorkspaceService(workspaceRepoStub{}, authUserRepoStub{})
		if _, err := svc.ListMyInvitations(context.Background(), "missing-user", ListMyInvitationsInput{}); !errors.Is(err, domain.ErrUnauthorized) {
			t.Fatalf("expected unauthorized for unknown actor, got %v", err)
		}

		svc = NewWorkspaceService(workspaceRepoStub{}, users)
		if _, err := svc.ListMyInvitations(context.Background(), "user-1", ListMyInvitationsInput{
			Status: "bad",
		}); !errors.Is(err, domain.ErrValidation) {
			t.Fatalf("expected invalid status validation, got %v", err)
		}
		if _, err := svc.ListMyInvitations(context.Background(), "user-1", ListMyInvitationsInput{
			Limit: 0,
		}); !errors.Is(err, domain.ErrValidation) {
			t.Fatalf("expected invalid limit validation, got %v", err)
		}

		expected := domain.WorkspaceInvitationList{
			Items: []domain.WorkspaceInvitation{
				{ID: "inv-2", WorkspaceID: "w2", Email: "owner@example.com", Status: domain.WorkspaceInvitationStatusPending, Version: 1},
				{ID: "inv-1", WorkspaceID: "w1", Email: "owner@example.com", Status: domain.WorkspaceInvitationStatusAccepted, Version: 2},
			},
			HasMore: true,
		}
		next := "cursor-2"
		expected.NextCursor = &next
		svc = NewWorkspaceService(workspaceRepoStub{
			listMyInvitationsFn: func(_ context.Context, email string, status domain.WorkspaceInvitationStatusFilter, limit int, cursor string) (domain.WorkspaceInvitationList, error) {
				if email != "owner@example.com" || status != domain.WorkspaceInvitationStatusFilterPending || limit != 25 || cursor != "cursor-1" {
					t.Fatalf("unexpected my-invitation args email=%s status=%s limit=%d cursor=%q", email, status, limit, cursor)
				}
				return expected, nil
			},
		}, users)
		result, err := svc.ListMyInvitations(context.Background(), "user-1", ListMyInvitationsInput{
			Status: domain.WorkspaceInvitationStatusFilterPending,
			Limit:  25,
			Cursor: "cursor-1",
		})
		if err != nil {
			t.Fatalf("expected my invitation list success, got %v", err)
		}
		if len(result.Items) != 2 || !result.HasMore || result.NextCursor == nil || *result.NextCursor != next {
			t.Fatalf("unexpected my invitation list result: %+v", result)
		}
	})

	t.Run("update invitation validation and auth", func(t *testing.T) {
		svc := NewWorkspaceService(workspaceRepoStub{}, users)
		if _, err := svc.UpdateInvitation(context.Background(), "user-1", UpdateInvitationInput{
			InvitationID: "inv-1",
			Role:         domain.WorkspaceRole("bad"),
			Version:      1,
		}); !errors.Is(err, domain.ErrValidation) {
			t.Fatalf("expected invalid role validation, got %v", err)
		}

		if _, err := svc.UpdateInvitation(context.Background(), "user-1", UpdateInvitationInput{
			InvitationID: "inv-1",
			Role:         domain.RoleEditor,
			Version:      0,
		}); !errors.Is(err, domain.ErrValidation) {
			t.Fatalf("expected non-positive version validation, got %v", err)
		}

		svc = NewWorkspaceService(workspaceRepoStub{
			getInvitationByIDFn: func(context.Context, string) (domain.WorkspaceInvitation, error) {
				return domain.WorkspaceInvitation{}, domain.ErrNotFound
			},
		}, users)
		if _, err := svc.UpdateInvitation(context.Background(), "user-1", UpdateInvitationInput{
			InvitationID: "missing",
			Role:         domain.RoleEditor,
			Version:      1,
		}); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("expected invitation not found, got %v", err)
		}

		svc = NewWorkspaceService(workspaceRepoStub{
			getInvitationByIDFn: func(context.Context, string) (domain.WorkspaceInvitation, error) {
				return domain.WorkspaceInvitation{ID: "inv-1", WorkspaceID: "w1", Role: domain.RoleViewer, Status: domain.WorkspaceInvitationStatusPending, Version: 3}, nil
			},
			getMembershipByUserIDFn: func(context.Context, string, string) (domain.WorkspaceMember, error) {
				return domain.WorkspaceMember{Role: domain.RoleEditor}, nil
			},
		}, users)
		if _, err := svc.UpdateInvitation(context.Background(), "user-1", UpdateInvitationInput{
			InvitationID: "inv-1",
			Role:         domain.RoleEditor,
			Version:      3,
		}); !errors.Is(err, domain.ErrForbidden) {
			t.Fatalf("expected non-owner update forbidden, got %v", err)
		}

		svc = NewWorkspaceService(workspaceRepoStub{
			getInvitationByIDFn: func(context.Context, string) (domain.WorkspaceInvitation, error) {
				return domain.WorkspaceInvitation{ID: "inv-1", WorkspaceID: "w1", Role: domain.RoleViewer, Status: domain.WorkspaceInvitationStatusAccepted, Version: 3}, nil
			},
			getMembershipByUserIDFn: func(context.Context, string, string) (domain.WorkspaceMember, error) {
				return domain.WorkspaceMember{Role: domain.RoleOwner}, nil
			},
		}, users)
		if _, err := svc.UpdateInvitation(context.Background(), "user-1", UpdateInvitationInput{
			InvitationID: "inv-1",
			Role:         domain.RoleEditor,
			Version:      3,
		}); !errors.Is(err, domain.ErrConflict) {
			t.Fatalf("expected terminal invitation conflict, got %v", err)
		}

		sameTime := time.Now().UTC().Truncate(time.Microsecond)
		expected := domain.WorkspaceInvitation{
			ID:          "inv-1",
			WorkspaceID: "w1",
			Email:       "invitee@example.com",
			Role:        domain.RoleEditor,
			Status:      domain.WorkspaceInvitationStatusPending,
			Version:     4,
			CreatedAt:   sameTime.Add(-time.Hour),
			UpdatedAt:   sameTime,
		}
		svc = NewWorkspaceService(workspaceRepoStub{
			getInvitationByIDFn: func(context.Context, string) (domain.WorkspaceInvitation, error) {
				return domain.WorkspaceInvitation{ID: "inv-1", WorkspaceID: "w1", Role: domain.RoleViewer, Status: domain.WorkspaceInvitationStatusPending, Version: 3}, nil
			},
			getMembershipByUserIDFn: func(context.Context, string, string) (domain.WorkspaceMember, error) {
				return domain.WorkspaceMember{Role: domain.RoleOwner}, nil
			},
			updateInvitationFn: func(_ context.Context, invitationID string, role domain.WorkspaceRole, version int64, updatedAt time.Time) (domain.WorkspaceInvitation, error) {
				if invitationID != "inv-1" || role != domain.RoleEditor || version != 3 {
					t.Fatalf("unexpected update args invitationID=%s role=%s version=%d", invitationID, role, version)
				}
				expected.UpdatedAt = updatedAt
				return expected, nil
			},
		}, users)
		updated, err := svc.UpdateInvitation(context.Background(), "user-1", UpdateInvitationInput{
			InvitationID: "inv-1",
			Role:         domain.RoleEditor,
			Version:      3,
		})
		if err != nil {
			t.Fatalf("expected update success, got %v", err)
		}
		if updated.Role != domain.RoleEditor || updated.Version != 4 || updated.Status != domain.WorkspaceInvitationStatusPending {
			t.Fatalf("unexpected updated invitation: %+v", updated)
		}
	})
}
