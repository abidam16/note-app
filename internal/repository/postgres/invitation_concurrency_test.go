package postgres

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"note-app/internal/domain"

	"github.com/google/uuid"
)

type invitationRaceAction func(context.Context, WorkspaceRepository, domain.WorkspaceInvitation, domain.User, domain.User, time.Time) error

type invitationRaceResult struct {
	slot int
	err  error
}

func TestInvitationConcurrencyUpdateVsAccept(t *testing.T) {
	testInvitationConcurrencyRace(t, "update-vs-accept",
		func(ctx context.Context, repo WorkspaceRepository, invitation domain.WorkspaceInvitation, owner, invited domain.User, now time.Time) error {
			_, err := repo.UpdateInvitation(ctx, invitation.ID, domain.RoleEditor, invitation.Version, now.Add(time.Minute))
			return err
		},
		func(ctx context.Context, repo WorkspaceRepository, invitation domain.WorkspaceInvitation, owner, invited domain.User, now time.Time) error {
			_, err := repo.AcceptInvitation(ctx, invitation.ID, invited.ID, invitation.Version, now.Add(2*time.Minute))
			return err
		},
	)
}

func TestInvitationConcurrencyUpdateVsReject(t *testing.T) {
	testInvitationConcurrencyRace(t, "update-vs-reject",
		func(ctx context.Context, repo WorkspaceRepository, invitation domain.WorkspaceInvitation, owner, invited domain.User, now time.Time) error {
			_, err := repo.UpdateInvitation(ctx, invitation.ID, domain.RoleEditor, invitation.Version, now.Add(time.Minute))
			return err
		},
		func(ctx context.Context, repo WorkspaceRepository, invitation domain.WorkspaceInvitation, owner, invited domain.User, now time.Time) error {
			_, err := repo.RejectInvitation(ctx, invitation.ID, invited.ID, invitation.Version, now.Add(2*time.Minute))
			return err
		},
	)
}

func TestInvitationConcurrencyCancelVsAccept(t *testing.T) {
	testInvitationConcurrencyRace(t, "cancel-vs-accept",
		func(ctx context.Context, repo WorkspaceRepository, invitation domain.WorkspaceInvitation, owner, invited domain.User, now time.Time) error {
			_, err := repo.CancelInvitation(ctx, invitation.ID, owner.ID, invitation.Version, now.Add(time.Minute))
			return err
		},
		func(ctx context.Context, repo WorkspaceRepository, invitation domain.WorkspaceInvitation, owner, invited domain.User, now time.Time) error {
			_, err := repo.AcceptInvitation(ctx, invitation.ID, invited.ID, invitation.Version, now.Add(2*time.Minute))
			return err
		},
	)
}

func TestInvitationConcurrencyCancelVsReject(t *testing.T) {
	testInvitationConcurrencyRace(t, "cancel-vs-reject",
		func(ctx context.Context, repo WorkspaceRepository, invitation domain.WorkspaceInvitation, owner, invited domain.User, now time.Time) error {
			_, err := repo.CancelInvitation(ctx, invitation.ID, owner.ID, invitation.Version, now.Add(time.Minute))
			return err
		},
		func(ctx context.Context, repo WorkspaceRepository, invitation domain.WorkspaceInvitation, owner, invited domain.User, now time.Time) error {
			_, err := repo.RejectInvitation(ctx, invitation.ID, invited.ID, invitation.Version, now.Add(2*time.Minute))
			return err
		},
	)
}

func testInvitationConcurrencyRace(t *testing.T, name string, first invitationRaceAction, second invitationRaceAction) {
	t.Helper()

	pool := integrationPool(t)
	repo := NewWorkspaceRepository(pool)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	owner := seedUser(t, pool, name+"-owner@example.com")
	invited := seedUser(t, pool, name+"-invitee@example.com")
	workspace, _ := seedWorkspaceWithOwner(t, pool, owner)

	invitation, err := repo.CreateInvitation(ctx, domain.WorkspaceInvitation{
		ID:          uuid.NewString(),
		WorkspaceID: workspace.ID,
		Email:       invited.Email,
		Role:        domain.RoleViewer,
		InvitedBy:   owner.ID,
		CreatedAt:   now,
		Status:      domain.WorkspaceInvitationStatusPending,
		Version:     1,
		UpdatedAt:   now,
	})
	if err != nil {
		t.Fatalf("CreateInvitation() error = %v", err)
	}

	errs := runInvitationRace(t, repo, invitation, owner, invited, now, first, second)
	assertOneSuccessOneConflict(t, errs[0], errs[1])

	updatedInvitation, err := repo.GetInvitationByID(ctx, invitation.ID)
	if err != nil {
		t.Fatalf("GetInvitationByID() error = %v", err)
	}
	if updatedInvitation.Version != invitation.Version+1 {
		t.Fatalf("expected invitation version %d, got %d", invitation.Version+1, updatedInvitation.Version)
	}

	switch {
	case errs[0] == nil:
		assertFirstWinnerFinalState(t, repo, name, updatedInvitation, invited)
	case errs[1] == nil:
		assertSecondWinnerFinalState(t, repo, name, updatedInvitation, invited)
	default:
		t.Fatal("expected one successful transition")
	}
}

func runInvitationRace(t *testing.T, repo WorkspaceRepository, invitation domain.WorkspaceInvitation, owner, invited domain.User, now time.Time, first invitationRaceAction, second invitationRaceAction) [2]error {
	t.Helper()

	actionCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ready := make(chan struct{}, 2)
	start := make(chan struct{})
	results := make(chan invitationRaceResult, 2)

	var wg sync.WaitGroup
	launch := func(slot int, action invitationRaceAction) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ready <- struct{}{}
			<-start
			results <- invitationRaceResult{slot: slot, err: action(actionCtx, repo, invitation, owner, invited, now)}
		}()
	}

	launch(0, first)
	launch(1, second)

	<-ready
	<-ready
	close(start)
	wg.Wait()
	close(results)

	var errs [2]error
	for result := range results {
		errs[result.slot] = result.err
	}
	return errs
}

func assertOneSuccessOneConflict(t *testing.T, firstErr, secondErr error) {
	t.Helper()

	successes := 0
	conflicts := 0
	for _, err := range []error{firstErr, secondErr} {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, domain.ErrConflict):
			conflicts++
		default:
			t.Fatalf("expected success or conflict, got %v", err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("expected one success and one conflict, got successes=%d conflicts=%d", successes, conflicts)
	}
}

func assertFirstWinnerFinalState(t *testing.T, repo WorkspaceRepository, name string, invitation domain.WorkspaceInvitation, invited domain.User) {
	t.Helper()

	switch name {
	case "update-vs-accept":
		if invitation.Status != domain.WorkspaceInvitationStatusPending || invitation.Role != domain.RoleEditor {
			t.Fatalf("expected updated pending invitation, got %+v", invitation)
		}
		assertNoMembership(t, repo, invitation.WorkspaceID, invited.ID)
	case "update-vs-reject":
		if invitation.Status != domain.WorkspaceInvitationStatusPending || invitation.Role != domain.RoleEditor {
			t.Fatalf("expected updated pending invitation, got %+v", invitation)
		}
		assertNoMembership(t, repo, invitation.WorkspaceID, invited.ID)
	case "cancel-vs-accept":
		if invitation.Status != domain.WorkspaceInvitationStatusCancelled || invitation.Role != domain.RoleViewer {
			t.Fatalf("expected cancelled invitation, got %+v", invitation)
		}
		assertNoMembership(t, repo, invitation.WorkspaceID, invited.ID)
	case "cancel-vs-reject":
		if invitation.Status != domain.WorkspaceInvitationStatusCancelled || invitation.Role != domain.RoleViewer {
			t.Fatalf("expected cancelled invitation, got %+v", invitation)
		}
		assertNoMembership(t, repo, invitation.WorkspaceID, invited.ID)
	default:
		t.Fatalf("unexpected race case %q", name)
	}
}

func assertSecondWinnerFinalState(t *testing.T, repo WorkspaceRepository, name string, invitation domain.WorkspaceInvitation, invited domain.User) {
	t.Helper()

	switch name {
	case "update-vs-accept":
		if invitation.Status != domain.WorkspaceInvitationStatusAccepted || invitation.Role != domain.RoleViewer {
			t.Fatalf("expected accepted invitation, got %+v", invitation)
		}
		assertMembership(t, repo, invitation.WorkspaceID, invited.ID, domain.RoleViewer)
	case "update-vs-reject":
		if invitation.Status != domain.WorkspaceInvitationStatusRejected || invitation.Role != domain.RoleViewer {
			t.Fatalf("expected rejected invitation, got %+v", invitation)
		}
		assertNoMembership(t, repo, invitation.WorkspaceID, invited.ID)
	case "cancel-vs-accept":
		if invitation.Status != domain.WorkspaceInvitationStatusAccepted || invitation.Role != domain.RoleViewer {
			t.Fatalf("expected accepted invitation, got %+v", invitation)
		}
		assertMembership(t, repo, invitation.WorkspaceID, invited.ID, domain.RoleViewer)
	case "cancel-vs-reject":
		if invitation.Status != domain.WorkspaceInvitationStatusRejected || invitation.Role != domain.RoleViewer {
			t.Fatalf("expected rejected invitation, got %+v", invitation)
		}
		assertNoMembership(t, repo, invitation.WorkspaceID, invited.ID)
	default:
		t.Fatalf("unexpected race case %q", name)
	}
}

func assertMembership(t *testing.T, repo WorkspaceRepository, workspaceID, userID string, wantRole domain.WorkspaceRole) {
	t.Helper()

	member, err := repo.GetMembershipByUserID(context.Background(), workspaceID, userID)
	if err != nil {
		t.Fatalf("expected membership for %s: %v", userID, err)
	}
	if member.Role != wantRole {
		t.Fatalf("expected membership role %s, got %+v", wantRole, member)
	}
}

func assertNoMembership(t *testing.T, repo WorkspaceRepository, workspaceID, userID string) {
	t.Helper()

	member, err := repo.GetMembershipByUserID(context.Background(), workspaceID, userID)
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected no membership for %s, got member=%+v err=%v", userID, member, err)
	}
}
