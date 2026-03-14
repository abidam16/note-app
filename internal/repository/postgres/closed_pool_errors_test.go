package postgres

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"note-app/internal/domain"
	"note-app/internal/infrastructure/database"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func closedPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	dsn := os.Getenv("POSTGRES_DSN")
	if dsn == "" {
		dsn = defaultTestDSN
	}
	pool, err := database.NewPool(ctx, dsn)
	if err != nil {
		t.Fatalf("create pool for closed-pool tests: %v", err)
	}
	pool.Close()
	return pool
}
func TestRepositoriesReturnErrorsWhenPoolClosed(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	pool := closedPool(t)

	userRepo := NewUserRepository(pool)
	if _, err := userRepo.Create(ctx, domain.User{ID: uuid.NewString(), Email: "a@b.com", FullName: "A", PasswordHash: "x", CreatedAt: now, UpdatedAt: now}); err == nil {
		t.Fatal("expected user create error on closed pool")
	}
	if _, err := userRepo.GetByEmail(ctx, "a@b.com"); err == nil {
		t.Fatal("expected user get by email error on closed pool")
	}
	if _, err := userRepo.GetByID(ctx, uuid.NewString()); err == nil {
		t.Fatal("expected user get by id error on closed pool")
	}

	refreshRepo := NewRefreshTokenRepository(pool)
	if _, err := refreshRepo.Create(ctx, domain.RefreshToken{ID: uuid.NewString(), UserID: uuid.NewString(), TokenHash: "h", ExpiresAt: now.Add(time.Hour), CreatedAt: now}); err == nil {
		t.Fatal("expected refresh create error on closed pool")
	}
	if _, err := refreshRepo.GetByHash(ctx, "h"); err == nil {
		t.Fatal("expected refresh get error on closed pool")
	}
	if err := refreshRepo.RevokeByID(ctx, uuid.NewString(), now); err == nil {
		t.Fatal("expected refresh revoke error on closed pool")
	}

	workspaceRepo := NewWorkspaceRepository(pool)
	if _, _, err := workspaceRepo.CreateWithOwner(ctx, domain.Workspace{ID: uuid.NewString(), Name: "W", CreatedAt: now, UpdatedAt: now}, domain.WorkspaceMember{ID: uuid.NewString(), WorkspaceID: uuid.NewString(), UserID: uuid.NewString(), Role: domain.RoleOwner, CreatedAt: now}); err == nil {
		t.Fatal("expected workspace create error on closed pool")
	}
	if _, err := workspaceRepo.GetMembershipByUserID(ctx, uuid.NewString(), uuid.NewString()); err == nil {
		t.Fatal("expected get membership error on closed pool")
	}
	if _, err := workspaceRepo.CreateInvitation(ctx, domain.WorkspaceInvitation{ID: uuid.NewString(), WorkspaceID: uuid.NewString(), Email: "a@b.com", Role: domain.RoleEditor, InvitedBy: uuid.NewString(), CreatedAt: now}); err == nil {
		t.Fatal("expected create invitation error on closed pool")
	}
	if _, err := workspaceRepo.GetActiveInvitationByEmail(ctx, uuid.NewString(), "a@b.com"); err == nil {
		t.Fatal("expected get active invitation error on closed pool")
	}
	if _, err := workspaceRepo.GetInvitationByID(ctx, uuid.NewString()); err == nil {
		t.Fatal("expected get invitation by id error on closed pool")
	}
	if _, err := workspaceRepo.AcceptInvitation(ctx, uuid.NewString(), uuid.NewString(), now); err == nil {
		t.Fatal("expected accept invitation error on closed pool")
	}
	if _, err := workspaceRepo.ListMembers(ctx, uuid.NewString()); err == nil {
		t.Fatal("expected list members error on closed pool")
	}
	if _, err := workspaceRepo.UpdateMemberRole(ctx, uuid.NewString(), uuid.NewString(), domain.RoleViewer); err == nil {
		t.Fatal("expected update member role error on closed pool")
	}
	if _, err := workspaceRepo.CountOwners(ctx, uuid.NewString()); err == nil {
		t.Fatal("expected count owners error on closed pool")
	}

	folderRepo := NewFolderRepository(pool)
	if _, err := folderRepo.Create(ctx, domain.Folder{ID: uuid.NewString(), WorkspaceID: uuid.NewString(), Name: "F", CreatedAt: now, UpdatedAt: now}); err == nil {
		t.Fatal("expected folder create error on closed pool")
	}
	if _, err := folderRepo.GetByID(ctx, uuid.NewString()); err == nil {
		t.Fatal("expected folder get error on closed pool")
	}
	if _, err := folderRepo.ListByWorkspaceID(ctx, uuid.NewString()); err == nil {
		t.Fatal("expected folder list error on closed pool")
	}

	pageRepo := NewPageRepository(pool)
	if _, _, err := pageRepo.CreateWithDraft(ctx, domain.Page{ID: uuid.NewString(), WorkspaceID: uuid.NewString(), Title: "P", CreatedBy: uuid.NewString(), CreatedAt: now, UpdatedAt: now}, domain.PageDraft{PageID: uuid.NewString(), Content: json.RawMessage(`[]`), LastEditedBy: uuid.NewString(), CreatedAt: now, UpdatedAt: now}); err == nil {
		t.Fatal("expected page create error on closed pool")
	}
	if _, _, err := pageRepo.GetByID(ctx, uuid.NewString()); err == nil {
		t.Fatal("expected page get error on closed pool")
	}
	if _, err := pageRepo.UpdateMetadata(ctx, uuid.NewString(), "x", nil, now); err == nil {
		t.Fatal("expected page update metadata error on closed pool")
	}
	if _, err := pageRepo.UpdateDraft(ctx, uuid.NewString(), json.RawMessage(`[]`), uuid.NewString(), now); err == nil {
		t.Fatal("expected page update draft error on closed pool")
	}
	if _, err := pageRepo.SearchPages(ctx, uuid.NewString(), "q"); err == nil {
		t.Fatal("expected page search error on closed pool")
	}
	if err := pageRepo.SoftDelete(ctx, domain.TrashItem{ID: uuid.NewString(), WorkspaceID: uuid.NewString(), PageID: uuid.NewString(), PageTitle: "x", DeletedBy: uuid.NewString(), DeletedAt: now}); err == nil {
		t.Fatal("expected page soft delete error on closed pool")
	}
	if _, err := pageRepo.ListTrashByWorkspaceID(ctx, uuid.NewString()); err == nil {
		t.Fatal("expected list trash error on closed pool")
	}
	if _, err := pageRepo.GetTrashItemByID(ctx, uuid.NewString()); err == nil {
		t.Fatal("expected get trash item error on closed pool")
	}
	if _, err := pageRepo.RestoreTrashItem(ctx, uuid.NewString(), uuid.NewString(), now); err == nil {
		t.Fatal("expected restore trash item error on closed pool")
	}

	revisionRepo := NewRevisionRepository(pool)
	if _, err := revisionRepo.Create(ctx, domain.Revision{ID: uuid.NewString(), PageID: uuid.NewString(), Content: json.RawMessage(`[]`), CreatedBy: uuid.NewString(), CreatedAt: now}); err == nil {
		t.Fatal("expected revision create error on closed pool")
	}
	if _, err := revisionRepo.GetByID(ctx, uuid.NewString()); err == nil {
		t.Fatal("expected revision get error on closed pool")
	}
	if _, err := revisionRepo.ListByPageID(ctx, uuid.NewString()); err == nil {
		t.Fatal("expected revision list error on closed pool")
	}

	commentRepo := NewCommentRepository(pool)
	if _, err := commentRepo.Create(ctx, domain.PageComment{ID: uuid.NewString(), PageID: uuid.NewString(), Body: "x", CreatedBy: uuid.NewString(), CreatedAt: now}); err == nil {
		t.Fatal("expected comment create error on closed pool")
	}
	if _, err := commentRepo.GetByID(ctx, uuid.NewString()); err == nil {
		t.Fatal("expected comment get error on closed pool")
	}
	if _, err := commentRepo.ListByPageID(ctx, uuid.NewString()); err == nil {
		t.Fatal("expected comment list error on closed pool")
	}
	if _, err := commentRepo.Resolve(ctx, uuid.NewString(), uuid.NewString(), now); err == nil {
		t.Fatal("expected comment resolve error on closed pool")
	}

	notificationRepo := NewNotificationRepository(pool)
	if _, err := notificationRepo.Create(ctx, domain.Notification{ID: uuid.NewString(), UserID: uuid.NewString(), WorkspaceID: uuid.NewString(), Type: domain.NotificationTypeComment, EventID: uuid.NewString(), Message: "x", CreatedAt: now}); err == nil {
		t.Fatal("expected notification create error on closed pool")
	}
	if _, err := notificationRepo.ListByUserID(ctx, uuid.NewString()); err == nil {
		t.Fatal("expected notification list error on closed pool")
	}
	if _, err := notificationRepo.MarkRead(ctx, uuid.NewString(), uuid.NewString(), now); err == nil {
		t.Fatal("expected notification mark read error on closed pool")
	}
}
