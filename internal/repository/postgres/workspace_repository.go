package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"note-app/internal/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type WorkspaceRepository struct {
	db *pgxpool.Pool
}

func NewWorkspaceRepository(db *pgxpool.Pool) WorkspaceRepository {
	return WorkspaceRepository{db: db}
}

func (r WorkspaceRepository) CreateWithOwner(ctx context.Context, workspace domain.Workspace, member domain.WorkspaceMember) (domain.Workspace, domain.WorkspaceMember, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return domain.Workspace{}, domain.WorkspaceMember{}, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `INSERT INTO workspaces (id, name, created_at, updated_at) VALUES ($1, $2, $3, $4)`, workspace.ID, workspace.Name, workspace.CreatedAt, workspace.UpdatedAt); err != nil {
		return domain.Workspace{}, domain.WorkspaceMember{}, fmt.Errorf("insert workspace: %w", err)
	}

	if _, err := tx.Exec(ctx, `INSERT INTO workspace_members (id, workspace_id, user_id, role, created_at) VALUES ($1, $2, $3, $4, $5)`, member.ID, member.WorkspaceID, member.UserID, member.Role, member.CreatedAt); err != nil {
		return domain.Workspace{}, domain.WorkspaceMember{}, fmt.Errorf("insert workspace owner: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return domain.Workspace{}, domain.WorkspaceMember{}, fmt.Errorf("commit workspace creation: %w", err)
	}

	return workspace, member, nil
}

func (r WorkspaceRepository) GetMembershipByUserID(ctx context.Context, workspaceID, userID string) (domain.WorkspaceMember, error) {
	query := `
        SELECT wm.id, wm.workspace_id, wm.user_id, wm.role, wm.created_at,
               u.id, u.email, u.full_name, u.password_hash, u.created_at, u.updated_at
        FROM workspace_members wm
        JOIN users u ON u.id = wm.user_id
        WHERE wm.workspace_id = $1 AND wm.user_id = $2
    `

	member, err := scanWorkspaceMember(r.db.QueryRow(ctx, query, workspaceID, userID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.WorkspaceMember{}, domain.ErrForbidden
		}
		return domain.WorkspaceMember{}, fmt.Errorf("select membership: %w", err)
	}

	return member, nil
}

func (r WorkspaceRepository) CreateInvitation(ctx context.Context, invitation domain.WorkspaceInvitation) (domain.WorkspaceInvitation, error) {
	query := `
        INSERT INTO workspace_invitations (id, workspace_id, email, role, invited_by, accepted_at, created_at)
        VALUES ($1, $2, $3, $4, $5, $6, $7)
        RETURNING id, workspace_id, email, role, invited_by, accepted_at, created_at
    `

	var created domain.WorkspaceInvitation
	if err := r.db.QueryRow(ctx, query, invitation.ID, invitation.WorkspaceID, invitation.Email, invitation.Role, invitation.InvitedBy, invitation.AcceptedAt, invitation.CreatedAt).Scan(
		&created.ID,
		&created.WorkspaceID,
		&created.Email,
		&created.Role,
		&created.InvitedBy,
		&created.AcceptedAt,
		&created.CreatedAt,
	); err != nil {
		if isUniqueViolation(err) {
			return domain.WorkspaceInvitation{}, domain.ErrConflict
		}
		return domain.WorkspaceInvitation{}, fmt.Errorf("insert invitation: %w", err)
	}

	return created, nil
}

func (r WorkspaceRepository) GetActiveInvitationByEmail(ctx context.Context, workspaceID, email string) (domain.WorkspaceInvitation, error) {
	query := `
        SELECT id, workspace_id, email, role, invited_by, accepted_at, created_at
        FROM workspace_invitations
        WHERE workspace_id = $1 AND email = $2 AND accepted_at IS NULL
        ORDER BY created_at DESC
        LIMIT 1
    `

	var invitation domain.WorkspaceInvitation
	if err := r.db.QueryRow(ctx, query, workspaceID, email).Scan(
		&invitation.ID,
		&invitation.WorkspaceID,
		&invitation.Email,
		&invitation.Role,
		&invitation.InvitedBy,
		&invitation.AcceptedAt,
		&invitation.CreatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.WorkspaceInvitation{}, domain.ErrNotFound
		}
		return domain.WorkspaceInvitation{}, fmt.Errorf("select active invitation: %w", err)
	}

	return invitation, nil
}

func (r WorkspaceRepository) GetInvitationByID(ctx context.Context, invitationID string) (domain.WorkspaceInvitation, error) {
	query := `SELECT id, workspace_id, email, role, invited_by, accepted_at, created_at FROM workspace_invitations WHERE id = $1`

	var invitation domain.WorkspaceInvitation
	if err := r.db.QueryRow(ctx, query, invitationID).Scan(
		&invitation.ID,
		&invitation.WorkspaceID,
		&invitation.Email,
		&invitation.Role,
		&invitation.InvitedBy,
		&invitation.AcceptedAt,
		&invitation.CreatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.WorkspaceInvitation{}, domain.ErrNotFound
		}
		return domain.WorkspaceInvitation{}, fmt.Errorf("select invitation: %w", err)
	}

	return invitation, nil
}

func (r WorkspaceRepository) AcceptInvitation(ctx context.Context, invitationID, userID string, acceptedAt time.Time) (domain.WorkspaceMember, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return domain.WorkspaceMember{}, fmt.Errorf("begin invitation acceptance transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	invitation, err := r.getInvitationForUpdate(ctx, tx, invitationID)
	if err != nil {
		return domain.WorkspaceMember{}, err
	}
	if invitation.AcceptedAt != nil {
		return domain.WorkspaceMember{}, domain.ErrConflict
	}

	member := domain.WorkspaceMember{
		ID:          uuid.NewString(),
		WorkspaceID: invitation.WorkspaceID,
		UserID:      userID,
		Role:        invitation.Role,
		CreatedAt:   acceptedAt,
	}

	if _, err := tx.Exec(ctx, `INSERT INTO workspace_members (id, workspace_id, user_id, role, created_at) VALUES ($1, $2, $3, $4, $5)`, member.ID, member.WorkspaceID, member.UserID, member.Role, member.CreatedAt); err != nil {
		if isUniqueViolation(err) {
			return domain.WorkspaceMember{}, domain.ErrConflict
		}
		return domain.WorkspaceMember{}, fmt.Errorf("insert workspace member: %w", err)
	}

	if _, err := tx.Exec(ctx, `UPDATE workspace_invitations SET accepted_at = $2 WHERE id = $1`, invitationID, acceptedAt); err != nil {
		return domain.WorkspaceMember{}, fmt.Errorf("mark invitation accepted: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return domain.WorkspaceMember{}, fmt.Errorf("commit invitation acceptance: %w", err)
	}

	return r.GetMembershipByUserID(ctx, invitation.WorkspaceID, userID)
}

func (r WorkspaceRepository) ListMembers(ctx context.Context, workspaceID string) ([]domain.WorkspaceMember, error) {
	query := `
        SELECT wm.id, wm.workspace_id, wm.user_id, wm.role, wm.created_at,
               u.id, u.email, u.full_name, u.password_hash, u.created_at, u.updated_at
        FROM workspace_members wm
        JOIN users u ON u.id = wm.user_id
        WHERE wm.workspace_id = $1
        ORDER BY wm.created_at ASC
    `

	rows, err := r.db.Query(ctx, query, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("query workspace members: %w", err)
	}
	defer rows.Close()

	members := make([]domain.WorkspaceMember, 0)
	for rows.Next() {
		member, err := scanWorkspaceMember(rows)
		if err != nil {
			return nil, fmt.Errorf("scan workspace member: %w", err)
		}
		members = append(members, member)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate workspace members: %w", err)
	}

	return members, nil
}

func (r WorkspaceRepository) UpdateMemberRole(ctx context.Context, workspaceID, memberID string, role domain.WorkspaceRole) (domain.WorkspaceMember, error) {
	query := `
        UPDATE workspace_members
        SET role = $3
        WHERE workspace_id = $1 AND id = $2
        RETURNING id, workspace_id, user_id, role, created_at
    `

	var member domain.WorkspaceMember
	if err := r.db.QueryRow(ctx, query, workspaceID, memberID, role).Scan(
		&member.ID,
		&member.WorkspaceID,
		&member.UserID,
		&member.Role,
		&member.CreatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.WorkspaceMember{}, domain.ErrNotFound
		}
		return domain.WorkspaceMember{}, fmt.Errorf("update workspace member role: %w", err)
	}

	user, err := r.getUser(ctx, member.UserID)
	if err != nil {
		return domain.WorkspaceMember{}, err
	}
	member.User = &user
	return member, nil
}

func (r WorkspaceRepository) CountOwners(ctx context.Context, workspaceID string) (int, error) {
	query := `SELECT COUNT(*) FROM workspace_members WHERE workspace_id = $1 AND role = $2`

	var count int
	if err := r.db.QueryRow(ctx, query, workspaceID, domain.RoleOwner).Scan(&count); err != nil {
		return 0, fmt.Errorf("count workspace owners: %w", err)
	}
	return count, nil
}

func (r WorkspaceRepository) getInvitationForUpdate(ctx context.Context, tx pgx.Tx, invitationID string) (domain.WorkspaceInvitation, error) {
	query := `
        SELECT id, workspace_id, email, role, invited_by, accepted_at, created_at
        FROM workspace_invitations
        WHERE id = $1
        FOR UPDATE
    `

	var invitation domain.WorkspaceInvitation
	if err := tx.QueryRow(ctx, query, invitationID).Scan(
		&invitation.ID,
		&invitation.WorkspaceID,
		&invitation.Email,
		&invitation.Role,
		&invitation.InvitedBy,
		&invitation.AcceptedAt,
		&invitation.CreatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.WorkspaceInvitation{}, domain.ErrNotFound
		}
		return domain.WorkspaceInvitation{}, fmt.Errorf("lock invitation: %w", err)
	}

	return invitation, nil
}

func (r WorkspaceRepository) getUser(ctx context.Context, userID string) (domain.User, error) {
	query := `SELECT id, email, full_name, password_hash, created_at, updated_at FROM users WHERE id = $1`

	var user domain.User
	if err := r.db.QueryRow(ctx, query, userID).Scan(
		&user.ID,
		&user.Email,
		&user.FullName,
		&user.PasswordHash,
		&user.CreatedAt,
		&user.UpdatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.User{}, domain.ErrNotFound
		}
		return domain.User{}, fmt.Errorf("select user by id: %w", err)
	}

	user.PasswordHash = ""
	return user, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanWorkspaceMember(scanner rowScanner) (domain.WorkspaceMember, error) {
	var member domain.WorkspaceMember
	var user domain.User
	if err := scanner.Scan(
		&member.ID,
		&member.WorkspaceID,
		&member.UserID,
		&member.Role,
		&member.CreatedAt,
		&user.ID,
		&user.Email,
		&user.FullName,
		&user.PasswordHash,
		&user.CreatedAt,
		&user.UpdatedAt,
	); err != nil {
		return domain.WorkspaceMember{}, err
	}
	user.PasswordHash = ""
	member.User = &user
	return member, nil
}
