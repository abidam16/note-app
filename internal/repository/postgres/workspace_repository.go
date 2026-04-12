package postgres

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"note-app/internal/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type WorkspaceRepository struct {
	db *pgxpool.Pool
}

type workspaceInvitationListCursor struct {
	Status    domain.WorkspaceInvitationStatusFilter `json:"status"`
	CreatedAt time.Time                              `json:"created_at"`
	ID        string                                 `json:"id"`
}

func NewWorkspaceRepository(db *pgxpool.Pool) WorkspaceRepository {
	return WorkspaceRepository{db: db}
}

func encodeWorkspaceInvitationListCursor(status domain.WorkspaceInvitationStatusFilter, invitation domain.WorkspaceInvitation) (string, error) {
	encoded, err := json.Marshal(workspaceInvitationListCursor{
		Status:    status,
		CreatedAt: invitation.CreatedAt.UTC(),
		ID:        invitation.ID,
	})
	if err != nil {
		return "", fmt.Errorf("marshal workspace invitation cursor: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(encoded), nil
}

func decodeWorkspaceInvitationListCursor(raw string, status domain.WorkspaceInvitationStatusFilter) (*workspaceInvitationListCursor, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}

	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid cursor", domain.ErrValidation)
	}

	var cursor workspaceInvitationListCursor
	if err := json.Unmarshal(decoded, &cursor); err != nil {
		return nil, fmt.Errorf("%w: invalid cursor", domain.ErrValidation)
	}
	if cursor.Status != status || cursor.CreatedAt.IsZero() || strings.TrimSpace(cursor.ID) == "" {
		return nil, fmt.Errorf("%w: invalid cursor", domain.ErrValidation)
	}

	return &cursor, nil
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

func (r WorkspaceRepository) HasWorkspaceWithNameForUser(ctx context.Context, userID, workspaceName string) (bool, error) {
	query := `
		SELECT EXISTS (
			SELECT 1
			FROM workspaces w
			JOIN workspace_members wm ON wm.workspace_id = w.id
			WHERE wm.user_id = $1
			  AND LOWER(TRIM(w.name)) = LOWER(TRIM($2))
		)
	`

	var exists bool
	if err := r.db.QueryRow(ctx, query, userID, workspaceName).Scan(&exists); err != nil {
		return false, fmt.Errorf("check workspace name existence for user: %w", err)
	}

	return exists, nil
}

func (r WorkspaceRepository) HasWorkspaceWithNameForUserExcludingID(ctx context.Context, userID, workspaceName, excludeWorkspaceID string) (bool, error) {
	query := `
		SELECT EXISTS (
			SELECT 1
			FROM workspaces w
			JOIN workspace_members wm ON wm.workspace_id = w.id
			WHERE wm.user_id = $1
			  AND LOWER(TRIM(w.name)) = LOWER(TRIM($2))
			  AND w.id <> $3
		)
	`

	var exists bool
	if err := r.db.QueryRow(ctx, query, userID, workspaceName, excludeWorkspaceID).Scan(&exists); err != nil {
		return false, fmt.Errorf("check workspace name existence for user with exclusion: %w", err)
	}

	return exists, nil
}

func (r WorkspaceRepository) GetByID(ctx context.Context, workspaceID string) (domain.Workspace, error) {
	query := `SELECT id, name, created_at, updated_at FROM workspaces WHERE id = $1`

	var workspace domain.Workspace
	if err := r.db.QueryRow(ctx, query, workspaceID).Scan(
		&workspace.ID,
		&workspace.Name,
		&workspace.CreatedAt,
		&workspace.UpdatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Workspace{}, domain.ErrNotFound
		}
		return domain.Workspace{}, fmt.Errorf("select workspace by id: %w", err)
	}

	return workspace, nil
}

func (r WorkspaceRepository) UpdateName(ctx context.Context, workspaceID, name string, updatedAt time.Time) (domain.Workspace, error) {
	query := `
		UPDATE workspaces
		SET name = $2, updated_at = $3
		WHERE id = $1
		RETURNING id, name, created_at, updated_at
	`

	var workspace domain.Workspace
	if err := r.db.QueryRow(ctx, query, workspaceID, name, updatedAt).Scan(
		&workspace.ID,
		&workspace.Name,
		&workspace.CreatedAt,
		&workspace.UpdatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Workspace{}, domain.ErrNotFound
		}
		return domain.Workspace{}, fmt.Errorf("update workspace name: %w", err)
	}

	return workspace, nil
}

func (r WorkspaceRepository) ListByUserID(ctx context.Context, userID string) ([]domain.Workspace, error) {
	query := `
        SELECT w.id, w.name, w.created_at, w.updated_at
        FROM workspaces w
        JOIN workspace_members wm ON wm.workspace_id = w.id
        WHERE wm.user_id = $1
        ORDER BY w.updated_at DESC, w.created_at DESC
    `

	rows, err := r.db.Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("query workspaces by user: %w", err)
	}
	defer rows.Close()

	workspaces := make([]domain.Workspace, 0)
	for rows.Next() {
		var workspace domain.Workspace
		if err := rows.Scan(&workspace.ID, &workspace.Name, &workspace.CreatedAt, &workspace.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan workspace by user: %w", err)
		}
		workspaces = append(workspaces, workspace)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate workspaces by user: %w", err)
	}

	return workspaces, nil
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

func (r WorkspaceRepository) GetMembershipByID(ctx context.Context, workspaceID, memberID string) (domain.WorkspaceMember, error) {
	query := `
		SELECT id, workspace_id, user_id, role, created_at
		FROM workspace_members
		WHERE workspace_id = $1
		  AND id = $2
	`

	var member domain.WorkspaceMember
	if err := r.db.QueryRow(ctx, query, workspaceID, memberID).Scan(
		&member.ID,
		&member.WorkspaceID,
		&member.UserID,
		&member.Role,
		&member.CreatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.WorkspaceMember{}, domain.ErrNotFound
		}
		return domain.WorkspaceMember{}, fmt.Errorf("select membership by id: %w", err)
	}

	return member, nil
}

func (r WorkspaceRepository) CreateInvitation(ctx context.Context, invitation domain.WorkspaceInvitation) (domain.WorkspaceInvitation, error) {
	query := `
        INSERT INTO workspace_invitations (
			id,
			workspace_id,
			email,
			role,
			invited_by,
			accepted_at,
			created_at,
			status,
			version,
			updated_at,
			responded_by,
			responded_at,
			cancelled_by,
			cancelled_at
		)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
        RETURNING id, workspace_id, email, role, invited_by, accepted_at, created_at, status, version, updated_at, responded_by, responded_at, cancelled_by, cancelled_at
    `

	status := invitation.Status
	if status == "" {
		status = domain.WorkspaceInvitationStatusPending
	}
	version := invitation.Version
	if version == 0 {
		version = 1
	}
	updatedAt := invitation.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = invitation.CreatedAt
	}

	var created domain.WorkspaceInvitation
	if err := scanWorkspaceInvitation(r.db.QueryRow(
		ctx,
		query,
		invitation.ID,
		invitation.WorkspaceID,
		invitation.Email,
		invitation.Role,
		invitation.InvitedBy,
		invitation.AcceptedAt,
		invitation.CreatedAt,
		status,
		version,
		updatedAt,
		invitation.RespondedBy,
		invitation.RespondedAt,
		invitation.CancelledBy,
		invitation.CancelledAt,
	), &created); err != nil {
		if isUniqueViolation(err) {
			return domain.WorkspaceInvitation{}, domain.ErrConflict
		}
		return domain.WorkspaceInvitation{}, fmt.Errorf("insert invitation: %w", err)
	}

	return created, nil
}

func (r WorkspaceRepository) GetActiveInvitationByEmail(ctx context.Context, workspaceID, email string) (domain.WorkspaceInvitation, error) {
	query := `
        SELECT id, workspace_id, email, role, invited_by, accepted_at, created_at, status, version, updated_at, responded_by, responded_at, cancelled_by, cancelled_at
        FROM workspace_invitations
        WHERE workspace_id = $1 AND email = $2 AND status = $3
        ORDER BY updated_at DESC, created_at DESC
        LIMIT 1
    `

	var invitation domain.WorkspaceInvitation
	if err := scanWorkspaceInvitation(r.db.QueryRow(ctx, query, workspaceID, email, domain.WorkspaceInvitationStatusPending), &invitation); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.WorkspaceInvitation{}, domain.ErrNotFound
		}
		return domain.WorkspaceInvitation{}, fmt.Errorf("select active invitation: %w", err)
	}

	return invitation, nil
}

func (r WorkspaceRepository) GetInvitationByID(ctx context.Context, invitationID string) (domain.WorkspaceInvitation, error) {
	query := `
		SELECT id, workspace_id, email, role, invited_by, accepted_at, created_at, status, version, updated_at, responded_by, responded_at, cancelled_by, cancelled_at
		FROM workspace_invitations
		WHERE id = $1
	`

	var invitation domain.WorkspaceInvitation
	if err := scanWorkspaceInvitation(r.db.QueryRow(ctx, query, invitationID), &invitation); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.WorkspaceInvitation{}, domain.ErrNotFound
		}
		return domain.WorkspaceInvitation{}, fmt.Errorf("select invitation: %w", err)
	}

	return invitation, nil
}

func (r WorkspaceRepository) AcceptInvitation(ctx context.Context, invitationID, userID string, version int64, acceptedAt time.Time) (domain.AcceptInvitationResult, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return domain.AcceptInvitationResult{}, fmt.Errorf("begin invitation acceptance transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	invitation, err := r.getInvitationForUpdate(ctx, tx, invitationID)
	if err != nil {
		return domain.AcceptInvitationResult{}, err
	}
	if invitation.Status != domain.WorkspaceInvitationStatusPending {
		return domain.AcceptInvitationResult{}, domain.ErrConflict
	}
	if invitation.Version != version {
		return domain.AcceptInvitationResult{}, domain.ErrConflict
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
			return domain.AcceptInvitationResult{}, domain.ErrConflict
		}
		return domain.AcceptInvitationResult{}, fmt.Errorf("insert workspace member: %w", err)
	}

	if _, err := tx.Exec(
		ctx,
		`
			UPDATE workspace_invitations
			SET accepted_at = $2,
			    status = $3,
			    version = $4,
			    updated_at = $2,
			    responded_by = $5,
			    responded_at = $2
			WHERE id = $1
		`,
		invitationID,
		acceptedAt,
		domain.WorkspaceInvitationStatusAccepted,
		version+1,
		userID,
	); err != nil {
		return domain.AcceptInvitationResult{}, fmt.Errorf("mark invitation accepted: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return domain.AcceptInvitationResult{}, fmt.Errorf("commit invitation acceptance: %w", err)
	}

	acceptedInvitation, err := r.GetInvitationByID(ctx, invitationID)
	if err != nil {
		return domain.AcceptInvitationResult{}, err
	}
	acceptedMember, err := r.GetMembershipByUserID(ctx, invitation.WorkspaceID, userID)
	if err != nil {
		return domain.AcceptInvitationResult{}, err
	}

	return domain.AcceptInvitationResult{
		Invitation: acceptedInvitation,
		Membership: acceptedMember,
	}, nil
}

func (r WorkspaceRepository) RejectInvitation(ctx context.Context, invitationID, userID string, version int64, rejectedAt time.Time) (domain.WorkspaceInvitation, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return domain.WorkspaceInvitation{}, fmt.Errorf("begin invitation rejection transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	invitation, err := r.getInvitationForUpdate(ctx, tx, invitationID)
	if err != nil {
		return domain.WorkspaceInvitation{}, err
	}
	if invitation.Status != domain.WorkspaceInvitationStatusPending {
		return domain.WorkspaceInvitation{}, domain.ErrConflict
	}
	if invitation.Version != version {
		return domain.WorkspaceInvitation{}, domain.ErrConflict
	}

	query := `
		UPDATE workspace_invitations
		SET status = $2,
		    version = $3,
		    updated_at = $4,
		    responded_by = $5,
		    responded_at = $4
		WHERE id = $1
		RETURNING id, workspace_id, email, role, invited_by, accepted_at, created_at, status, version, updated_at, responded_by, responded_at, cancelled_by, cancelled_at
	`

	var rejected domain.WorkspaceInvitation
	if err := scanWorkspaceInvitation(tx.QueryRow(ctx, query, invitationID, domain.WorkspaceInvitationStatusRejected, version+1, rejectedAt, userID), &rejected); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.WorkspaceInvitation{}, domain.ErrNotFound
		}
		return domain.WorkspaceInvitation{}, fmt.Errorf("reject invitation: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return domain.WorkspaceInvitation{}, fmt.Errorf("commit invitation rejection: %w", err)
	}

	return rejected, nil
}

func (r WorkspaceRepository) CancelInvitation(ctx context.Context, invitationID, userID string, version int64, cancelledAt time.Time) (domain.WorkspaceInvitation, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return domain.WorkspaceInvitation{}, fmt.Errorf("begin invitation cancellation transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	invitation, err := r.getInvitationForUpdate(ctx, tx, invitationID)
	if err != nil {
		return domain.WorkspaceInvitation{}, err
	}
	if invitation.Status != domain.WorkspaceInvitationStatusPending {
		return domain.WorkspaceInvitation{}, domain.ErrConflict
	}
	if invitation.Version != version {
		return domain.WorkspaceInvitation{}, domain.ErrConflict
	}

	query := `
		UPDATE workspace_invitations
		SET status = $2,
		    version = $3,
		    updated_at = $4,
		    cancelled_by = $5,
		    cancelled_at = $4
		WHERE id = $1
		RETURNING id, workspace_id, email, role, invited_by, accepted_at, created_at, status, version, updated_at, responded_by, responded_at, cancelled_by, cancelled_at
	`

	var cancelled domain.WorkspaceInvitation
	if err := scanWorkspaceInvitation(tx.QueryRow(ctx, query, invitationID, domain.WorkspaceInvitationStatusCancelled, version+1, cancelledAt, userID), &cancelled); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.WorkspaceInvitation{}, domain.ErrNotFound
		}
		return domain.WorkspaceInvitation{}, fmt.Errorf("cancel invitation: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return domain.WorkspaceInvitation{}, fmt.Errorf("commit invitation cancellation: %w", err)
	}

	return cancelled, nil
}

func (r WorkspaceRepository) UpdateInvitation(ctx context.Context, invitationID string, role domain.WorkspaceRole, version int64, updatedAt time.Time) (domain.WorkspaceInvitation, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return domain.WorkspaceInvitation{}, fmt.Errorf("begin invitation update transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	invitation, err := r.getInvitationForUpdate(ctx, tx, invitationID)
	if err != nil {
		return domain.WorkspaceInvitation{}, err
	}
	if invitation.Status != domain.WorkspaceInvitationStatusPending {
		return domain.WorkspaceInvitation{}, domain.ErrConflict
	}
	if invitation.Version != version {
		return domain.WorkspaceInvitation{}, domain.ErrConflict
	}
	if invitation.Role == role {
		if err := tx.Commit(ctx); err != nil {
			return domain.WorkspaceInvitation{}, fmt.Errorf("commit invitation update no-op: %w", err)
		}
		return invitation, nil
	}

	query := `
		UPDATE workspace_invitations
		SET role = $2,
		    version = $3,
		    updated_at = $4
		WHERE id = $1
		RETURNING id, workspace_id, email, role, invited_by, accepted_at, created_at, status, version, updated_at, responded_by, responded_at, cancelled_by, cancelled_at
	`

	var updated domain.WorkspaceInvitation
	if err := scanWorkspaceInvitation(tx.QueryRow(ctx, query, invitationID, role, invitation.Version+1, updatedAt), &updated); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.WorkspaceInvitation{}, domain.ErrNotFound
		}
		return domain.WorkspaceInvitation{}, fmt.Errorf("update invitation: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return domain.WorkspaceInvitation{}, fmt.Errorf("commit invitation update: %w", err)
	}

	return updated, nil
}

func (r WorkspaceRepository) ListWorkspaceInvitations(ctx context.Context, workspaceID string, status domain.WorkspaceInvitationStatusFilter, limit int, rawCursor string) (domain.WorkspaceInvitationList, error) {
	cursor, err := decodeWorkspaceInvitationListCursor(rawCursor, status)
	if err != nil {
		return domain.WorkspaceInvitationList{}, err
	}

	args := []any{workspaceID}
	query := `
		SELECT id, workspace_id, email, role, invited_by, accepted_at, created_at, status, version, updated_at, responded_by, responded_at, cancelled_by, cancelled_at
		FROM workspace_invitations
		WHERE workspace_id = $1
	`
	nextArg := 2

	if status != domain.WorkspaceInvitationStatusFilterAll {
		query += fmt.Sprintf(" AND status = $%d", nextArg)
		args = append(args, status)
		nextArg++
	}
	if cursor != nil {
		query += fmt.Sprintf(" AND (created_at, id) < ($%d, $%d)", nextArg, nextArg+1)
		args = append(args, cursor.CreatedAt, cursor.ID)
		nextArg += 2
	}
	query += fmt.Sprintf(" ORDER BY created_at DESC, id DESC LIMIT $%d", nextArg)
	args = append(args, limit+1)

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return domain.WorkspaceInvitationList{}, fmt.Errorf("query workspace invitations: %w", err)
	}
	defer rows.Close()

	items := make([]domain.WorkspaceInvitation, 0, limit+1)
	for rows.Next() {
		var invitation domain.WorkspaceInvitation
		if err := scanWorkspaceInvitation(rows, &invitation); err != nil {
			return domain.WorkspaceInvitationList{}, fmt.Errorf("scan workspace invitation list item: %w", err)
		}
		items = append(items, invitation)
	}
	if err := rows.Err(); err != nil {
		return domain.WorkspaceInvitationList{}, fmt.Errorf("iterate workspace invitations: %w", err)
	}

	result := domain.WorkspaceInvitationList{Items: items}
	if len(items) > limit {
		result.HasMore = true
		result.Items = items[:limit]
		nextCursor, err := encodeWorkspaceInvitationListCursor(status, result.Items[len(result.Items)-1])
		if err != nil {
			return domain.WorkspaceInvitationList{}, err
		}
		result.NextCursor = &nextCursor
	}

	return result, nil
}

func (r WorkspaceRepository) ListMyInvitations(ctx context.Context, email string, status domain.WorkspaceInvitationStatusFilter, rawLimit int, rawCursor string) (domain.WorkspaceInvitationList, error) {
	cursor, err := decodeWorkspaceInvitationListCursor(rawCursor, status)
	if err != nil {
		return domain.WorkspaceInvitationList{}, err
	}

	args := []any{email}
	query := `
		SELECT id, workspace_id, email, role, invited_by, accepted_at, created_at, status, version, updated_at, responded_by, responded_at, cancelled_by, cancelled_at
		FROM workspace_invitations
		WHERE LOWER(email) = LOWER($1)
	`
	nextArg := 2

	if status != domain.WorkspaceInvitationStatusFilterAll {
		query += fmt.Sprintf(" AND status = $%d", nextArg)
		args = append(args, status)
		nextArg++
	}
	if cursor != nil {
		query += fmt.Sprintf(" AND (created_at, id) < ($%d, $%d)", nextArg, nextArg+1)
		args = append(args, cursor.CreatedAt, cursor.ID)
		nextArg += 2
	}
	query += fmt.Sprintf(" ORDER BY created_at DESC, id DESC LIMIT $%d", nextArg)
	args = append(args, rawLimit+1)

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return domain.WorkspaceInvitationList{}, fmt.Errorf("query my invitations: %w", err)
	}
	defer rows.Close()

	items := make([]domain.WorkspaceInvitation, 0, rawLimit+1)
	for rows.Next() {
		var invitation domain.WorkspaceInvitation
		if err := scanWorkspaceInvitation(rows, &invitation); err != nil {
			return domain.WorkspaceInvitationList{}, fmt.Errorf("scan my invitation list item: %w", err)
		}
		items = append(items, invitation)
	}
	if err := rows.Err(); err != nil {
		return domain.WorkspaceInvitationList{}, fmt.Errorf("iterate my invitations: %w", err)
	}

	result := domain.WorkspaceInvitationList{Items: items}
	if len(items) > rawLimit {
		result.HasMore = true
		result.Items = items[:rawLimit]
		nextCursor, err := encodeWorkspaceInvitationListCursor(status, result.Items[len(result.Items)-1])
		if err != nil {
			return domain.WorkspaceInvitationList{}, err
		}
		result.NextCursor = &nextCursor
	}

	return result, nil
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
        SELECT id, workspace_id, email, role, invited_by, accepted_at, created_at, status, version, updated_at, responded_by, responded_at, cancelled_by, cancelled_at
        FROM workspace_invitations
        WHERE id = $1
        FOR UPDATE
    `

	var invitation domain.WorkspaceInvitation
	if err := scanWorkspaceInvitation(tx.QueryRow(ctx, query, invitationID), &invitation); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.WorkspaceInvitation{}, domain.ErrNotFound
		}
		return domain.WorkspaceInvitation{}, fmt.Errorf("lock invitation: %w", err)
	}

	return invitation, nil
}

type workspaceInvitationScanner interface {
	Scan(dest ...any) error
}

func scanWorkspaceInvitation(row workspaceInvitationScanner, invitation *domain.WorkspaceInvitation) error {
	return row.Scan(
		&invitation.ID,
		&invitation.WorkspaceID,
		&invitation.Email,
		&invitation.Role,
		&invitation.InvitedBy,
		&invitation.AcceptedAt,
		&invitation.CreatedAt,
		&invitation.Status,
		&invitation.Version,
		&invitation.UpdatedAt,
		&invitation.RespondedBy,
		&invitation.RespondedAt,
		&invitation.CancelledBy,
		&invitation.CancelledAt,
	)
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
