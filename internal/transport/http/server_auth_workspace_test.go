package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"note-app/internal/application"
	"note-app/internal/domain"
	appauth "note-app/internal/infrastructure/auth"
	"note-app/internal/infrastructure/storage"

	"github.com/google/uuid"
)

type httpUserRepo struct {
	byID    map[string]domain.User
	byEmail map[string]domain.User
}

func assertAPISecurityHeaders(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("expected X-Content-Type-Options nosniff, got %q", got)
	}
	if got := rec.Header().Get("X-Frame-Options"); got != "DENY" {
		t.Fatalf("expected X-Frame-Options DENY, got %q", got)
	}
	if got := rec.Header().Get("Referrer-Policy"); got != "no-referrer" {
		t.Fatalf("expected Referrer-Policy no-referrer, got %q", got)
	}
}

func assertNoStoreHeaders(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("expected Cache-Control no-store, got %q", got)
	}
	if got := rec.Header().Get("Pragma"); got != "no-cache" {
		t.Fatalf("expected Pragma no-cache, got %q", got)
	}
}

func (r *httpUserRepo) Create(_ context.Context, user domain.User) (domain.User, error) {
	if r.byID == nil {
		r.byID = map[string]domain.User{}
	}
	if r.byEmail == nil {
		r.byEmail = map[string]domain.User{}
	}
	if _, exists := r.byEmail[user.Email]; exists {
		return domain.User{}, domain.ErrEmailAlreadyUsed
	}
	r.byID[user.ID] = user
	r.byEmail[user.Email] = user
	return user, nil
}

func (r *httpUserRepo) GetByEmail(_ context.Context, email string) (domain.User, error) {
	user, ok := r.byEmail[email]
	if !ok {
		return domain.User{}, domain.ErrNotFound
	}
	return user, nil
}

func (r *httpUserRepo) GetByID(_ context.Context, userID string) (domain.User, error) {
	user, ok := r.byID[userID]
	if !ok {
		return domain.User{}, domain.ErrNotFound
	}
	return user, nil
}

type httpRefreshTokenRepo struct {
	byHash map[string]domain.RefreshToken
}

func (r *httpRefreshTokenRepo) Create(_ context.Context, token domain.RefreshToken) (domain.RefreshToken, error) {
	if r.byHash == nil {
		r.byHash = map[string]domain.RefreshToken{}
	}
	r.byHash[token.TokenHash] = token
	return token, nil
}

func (r *httpRefreshTokenRepo) GetByHash(_ context.Context, hash string) (domain.RefreshToken, error) {
	token, ok := r.byHash[hash]
	if !ok {
		return domain.RefreshToken{}, domain.ErrNotFound
	}
	return token, nil
}

func (r *httpRefreshTokenRepo) RevokeByID(_ context.Context, tokenID string, revokedAt time.Time) error {
	for hash, token := range r.byHash {
		if token.ID == tokenID {
			token.RevokedAt = &revokedAt
			r.byHash[hash] = token
			return nil
		}
	}
	return domain.ErrNotFound
}

type httpWorkspaceRepo struct {
	memberships map[string][]domain.WorkspaceMember
	invitations map[string]domain.WorkspaceInvitation
	workspaces  map[string]domain.Workspace
}

func httpWorkspaceInvitationIsPending(invitation domain.WorkspaceInvitation) bool {
	if invitation.Status != "" {
		return invitation.Status == domain.WorkspaceInvitationStatusPending
	}
	return invitation.AcceptedAt == nil
}

func (r *httpWorkspaceRepo) CreateWithOwner(_ context.Context, workspace domain.Workspace, member domain.WorkspaceMember) (domain.Workspace, domain.WorkspaceMember, error) {
	if r.memberships == nil {
		r.memberships = map[string][]domain.WorkspaceMember{}
	}
	if r.workspaces == nil {
		r.workspaces = map[string]domain.Workspace{}
	}
	r.memberships[workspace.ID] = append(r.memberships[workspace.ID], member)
	r.workspaces[workspace.ID] = workspace
	return workspace, member, nil
}

func (r *httpWorkspaceRepo) HasWorkspaceWithNameForUser(_ context.Context, userID, workspaceName string) (bool, error) {
	for workspaceID, members := range r.memberships {
		for _, member := range members {
			if member.UserID == userID && strings.EqualFold(strings.TrimSpace(r.workspaces[workspaceID].Name), strings.TrimSpace(workspaceName)) {
				return true, nil
			}
		}
	}
	return false, nil
}

func (r *httpWorkspaceRepo) HasWorkspaceWithNameForUserExcludingID(_ context.Context, userID, workspaceName, excludeWorkspaceID string) (bool, error) {
	for workspaceID, members := range r.memberships {
		if workspaceID == excludeWorkspaceID {
			continue
		}
		for _, member := range members {
			if member.UserID == userID && strings.EqualFold(strings.TrimSpace(r.workspaces[workspaceID].Name), strings.TrimSpace(workspaceName)) {
				return true, nil
			}
		}
	}
	return false, nil
}

func (r *httpWorkspaceRepo) GetByID(_ context.Context, workspaceID string) (domain.Workspace, error) {
	workspace, ok := r.workspaces[workspaceID]
	if !ok {
		return domain.Workspace{}, domain.ErrNotFound
	}
	return workspace, nil
}

func (r *httpWorkspaceRepo) UpdateName(_ context.Context, workspaceID, name string, updatedAt time.Time) (domain.Workspace, error) {
	workspace, ok := r.workspaces[workspaceID]
	if !ok {
		return domain.Workspace{}, domain.ErrNotFound
	}
	workspace.Name = name
	workspace.UpdatedAt = updatedAt
	r.workspaces[workspaceID] = workspace
	return workspace, nil
}

func (r *httpWorkspaceRepo) ListByUserID(_ context.Context, userID string) ([]domain.Workspace, error) {
	workspaces := make([]domain.Workspace, 0)
	for workspaceID, members := range r.memberships {
		for _, member := range members {
			if member.UserID == userID {
				workspaces = append(workspaces, r.workspaces[workspaceID])
				break
			}
		}
	}
	return workspaces, nil
}

func (r *httpWorkspaceRepo) GetMembershipByUserID(_ context.Context, workspaceID, userID string) (domain.WorkspaceMember, error) {
	for _, member := range r.memberships[workspaceID] {
		if member.UserID == userID {
			return member, nil
		}
	}
	return domain.WorkspaceMember{}, domain.ErrForbidden
}

func (r *httpWorkspaceRepo) GetMembershipByID(_ context.Context, workspaceID, memberID string) (domain.WorkspaceMember, error) {
	for _, member := range r.memberships[workspaceID] {
		if member.ID == memberID {
			return member, nil
		}
	}
	return domain.WorkspaceMember{}, domain.ErrNotFound
}

func (r *httpWorkspaceRepo) CreateInvitation(_ context.Context, invitation domain.WorkspaceInvitation) (domain.WorkspaceInvitation, error) {
	if r.invitations == nil {
		r.invitations = map[string]domain.WorkspaceInvitation{}
	}
	for _, existing := range r.invitations {
		if existing.WorkspaceID == invitation.WorkspaceID && existing.Email == invitation.Email && httpWorkspaceInvitationIsPending(existing) {
			return domain.WorkspaceInvitation{}, domain.ErrConflict
		}
	}
	if invitation.Status == "" {
		invitation.Status = domain.WorkspaceInvitationStatusPending
	}
	if invitation.Version == 0 {
		invitation.Version = 1
	}
	if invitation.UpdatedAt.IsZero() {
		invitation.UpdatedAt = invitation.CreatedAt
	}
	r.invitations[invitation.ID] = invitation
	return invitation, nil
}

func (r *httpWorkspaceRepo) GetActiveInvitationByEmail(_ context.Context, workspaceID, email string) (domain.WorkspaceInvitation, error) {
	for _, invitation := range r.invitations {
		if invitation.WorkspaceID == workspaceID && invitation.Email == email && httpWorkspaceInvitationIsPending(invitation) {
			return invitation, nil
		}
	}
	return domain.WorkspaceInvitation{}, domain.ErrNotFound
}

func (r *httpWorkspaceRepo) GetInvitationByID(_ context.Context, invitationID string) (domain.WorkspaceInvitation, error) {
	invitation, ok := r.invitations[invitationID]
	if !ok {
		return domain.WorkspaceInvitation{}, domain.ErrNotFound
	}
	return invitation, nil
}

func (r *httpWorkspaceRepo) AcceptInvitation(_ context.Context, invitationID, userID string, version int64, acceptedAt time.Time) (domain.AcceptInvitationResult, error) {
	invitation, ok := r.invitations[invitationID]
	if !ok {
		return domain.AcceptInvitationResult{}, domain.ErrNotFound
	}
	if !httpWorkspaceInvitationIsPending(invitation) {
		return domain.AcceptInvitationResult{}, domain.ErrConflict
	}
	if invitation.Version != version {
		return domain.AcceptInvitationResult{}, domain.ErrConflict
	}
	invitation.AcceptedAt = &acceptedAt
	invitation.Status = domain.WorkspaceInvitationStatusAccepted
	invitation.Version++
	if invitation.Version == 0 {
		invitation.Version = 2
	}
	invitation.UpdatedAt = acceptedAt
	invitation.RespondedBy = &userID
	invitation.RespondedAt = &acceptedAt
	r.invitations[invitationID] = invitation
	member := domain.WorkspaceMember{ID: uuid.NewString(), WorkspaceID: invitation.WorkspaceID, UserID: userID, Role: invitation.Role, CreatedAt: acceptedAt}
	r.memberships[invitation.WorkspaceID] = append(r.memberships[invitation.WorkspaceID], member)
	return domain.AcceptInvitationResult{Invitation: invitation, Membership: member}, nil
}

func (r *httpWorkspaceRepo) UpdateInvitation(_ context.Context, invitationID string, role domain.WorkspaceRole, version int64, updatedAt time.Time) (domain.WorkspaceInvitation, error) {
	invitation, ok := r.invitations[invitationID]
	if !ok {
		return domain.WorkspaceInvitation{}, domain.ErrNotFound
	}
	if !httpWorkspaceInvitationIsPending(invitation) {
		return domain.WorkspaceInvitation{}, domain.ErrConflict
	}
	if invitation.Version != version {
		return domain.WorkspaceInvitation{}, domain.ErrConflict
	}
	if invitation.Role == role {
		return invitation, nil
	}
	invitation.Role = role
	invitation.Version++
	invitation.UpdatedAt = updatedAt
	r.invitations[invitationID] = invitation
	return invitation, nil
}

func (r *httpWorkspaceRepo) RejectInvitation(_ context.Context, invitationID, userID string, version int64, rejectedAt time.Time) (domain.WorkspaceInvitation, error) {
	invitation, ok := r.invitations[invitationID]
	if !ok {
		return domain.WorkspaceInvitation{}, domain.ErrNotFound
	}
	if !httpWorkspaceInvitationIsPending(invitation) {
		return domain.WorkspaceInvitation{}, domain.ErrConflict
	}
	if invitation.Version != version {
		return domain.WorkspaceInvitation{}, domain.ErrConflict
	}
	invitation.Status = domain.WorkspaceInvitationStatusRejected
	invitation.Version++
	invitation.UpdatedAt = rejectedAt
	invitation.RespondedBy = &userID
	invitation.RespondedAt = &rejectedAt
	invitation.AcceptedAt = nil
	r.invitations[invitationID] = invitation
	return invitation, nil
}

func (r *httpWorkspaceRepo) CancelInvitation(_ context.Context, invitationID, userID string, version int64, cancelledAt time.Time) (domain.WorkspaceInvitation, error) {
	invitation, ok := r.invitations[invitationID]
	if !ok {
		return domain.WorkspaceInvitation{}, domain.ErrNotFound
	}
	if !httpWorkspaceInvitationIsPending(invitation) {
		return domain.WorkspaceInvitation{}, domain.ErrConflict
	}
	if invitation.Version != version {
		return domain.WorkspaceInvitation{}, domain.ErrConflict
	}
	invitation.Status = domain.WorkspaceInvitationStatusCancelled
	invitation.Version++
	invitation.UpdatedAt = cancelledAt
	invitation.CancelledBy = &userID
	invitation.CancelledAt = &cancelledAt
	invitation.AcceptedAt = nil
	invitation.RespondedBy = nil
	invitation.RespondedAt = nil
	r.invitations[invitationID] = invitation
	return invitation, nil
}

func (r *httpWorkspaceRepo) ListWorkspaceInvitations(_ context.Context, workspaceID string, status domain.WorkspaceInvitationStatusFilter, limit int, cursor string) (domain.WorkspaceInvitationList, error) {
	if strings.TrimSpace(cursor) == "broken" {
		return domain.WorkspaceInvitationList{}, fmt.Errorf("%w: invalid cursor", domain.ErrValidation)
	}

	items := make([]domain.WorkspaceInvitation, 0)
	for _, invitation := range r.invitations {
		if invitation.WorkspaceID != workspaceID {
			continue
		}
		if status != domain.WorkspaceInvitationStatusFilterAll && invitation.Status != domain.WorkspaceInvitationStatus(status) {
			continue
		}
		items = append(items, invitation)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].CreatedAt.Equal(items[j].CreatedAt) {
			return items[i].ID > items[j].ID
		}
		return items[i].CreatedAt.After(items[j].CreatedAt)
	})

	start := 0
	if cursor != "" {
		for idx := range items {
			if items[idx].ID == cursor {
				start = idx + 1
				break
			}
		}
	}
	if start > len(items) {
		start = len(items)
	}
	items = items[start:]

	result := domain.WorkspaceInvitationList{}
	if len(items) > limit {
		result.Items = append(result.Items, items[:limit]...)
		result.HasMore = true
		next := result.Items[len(result.Items)-1].ID
		result.NextCursor = &next
		return result, nil
	}
	result.Items = append(result.Items, items...)
	return result, nil
}

func (r *httpWorkspaceRepo) ListMyInvitations(_ context.Context, email string, status domain.WorkspaceInvitationStatusFilter, limit int, cursor string) (domain.WorkspaceInvitationList, error) {
	if strings.TrimSpace(cursor) == "broken" {
		return domain.WorkspaceInvitationList{}, fmt.Errorf("%w: invalid cursor", domain.ErrValidation)
	}

	items := make([]domain.WorkspaceInvitation, 0)
	for _, invitation := range r.invitations {
		if !strings.EqualFold(invitation.Email, email) {
			continue
		}
		if status != domain.WorkspaceInvitationStatusFilterAll && invitation.Status != domain.WorkspaceInvitationStatus(status) {
			continue
		}
		items = append(items, invitation)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].CreatedAt.Equal(items[j].CreatedAt) {
			return items[i].ID > items[j].ID
		}
		return items[i].CreatedAt.After(items[j].CreatedAt)
	})

	start := 0
	if cursor != "" {
		for idx := range items {
			if items[idx].ID == cursor {
				start = idx + 1
				break
			}
		}
	}
	if start > len(items) {
		start = len(items)
	}
	items = items[start:]

	result := domain.WorkspaceInvitationList{}
	if len(items) > limit {
		result.Items = append(result.Items, items[:limit]...)
		result.HasMore = true
		next := result.Items[len(result.Items)-1].ID
		result.NextCursor = &next
		return result, nil
	}
	result.Items = append(result.Items, items...)
	return result, nil
}

func (r *httpWorkspaceRepo) ListMembers(_ context.Context, workspaceID string) ([]domain.WorkspaceMember, error) {
	return r.memberships[workspaceID], nil
}

func (r *httpWorkspaceRepo) UpdateMemberRole(_ context.Context, workspaceID, memberID string, role domain.WorkspaceRole) (domain.WorkspaceMember, error) {
	members := r.memberships[workspaceID]
	for i := range members {
		if members[i].ID == memberID {
			members[i].Role = role
			r.memberships[workspaceID] = members
			return members[i], nil
		}
	}
	return domain.WorkspaceMember{}, domain.ErrNotFound
}

func (r *httpWorkspaceRepo) CountOwners(_ context.Context, workspaceID string) (int, error) {
	count := 0
	for _, member := range r.memberships[workspaceID] {
		if member.Role == domain.RoleOwner {
			count++
		}
	}
	return count, nil
}

func TestAuthAndWorkspaceEndpoints(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	tokenManager := appauth.NewTokenManager("super-secret-token", "note-app", 15*time.Minute)
	userRepo := &httpUserRepo{byID: map[string]domain.User{}, byEmail: map[string]domain.User{}}
	refreshRepo := &httpRefreshTokenRepo{byHash: map[string]domain.RefreshToken{}}
	workspaceRepo := &httpWorkspaceRepo{memberships: map[string][]domain.WorkspaceMember{}, invitations: map[string]domain.WorkspaceInvitation{}, workspaces: map[string]domain.Workspace{}}

	authService := application.NewAuthService(userRepo, refreshRepo, appauth.NewPasswordManager(), tokenManager, 24*time.Hour)
	workspaceService := application.NewWorkspaceService(workspaceRepo, userRepo)
	server := NewServer(logger, authService, workspaceService, application.FolderService{}, application.PageService{}, application.RevisionService{}, tokenManager, storage.NewLocal(t.TempDir()))

	invalidJSON := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewBufferString(`{"email":`))
	invalidJSON.Header.Set("Content-Type", "application/json")
	invalidJSONRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(invalidJSONRec, invalidJSON)
	if invalidJSONRec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 invalid json, got %d", invalidJSONRec.Code)
	}
	assertAPISecurityHeaders(t, invalidJSONRec)
	assertNoStoreHeaders(t, invalidJSONRec)

	registerBody := `{"email":"owner@example.com","password":"Password1","full_name":"Owner"}`
	registerReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewBufferString(registerBody))
	registerReq.Header.Set("Content-Type", "application/json")
	registerRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(registerRec, registerReq)
	if registerRec.Code != http.StatusCreated {
		t.Fatalf("expected register 201, got %d body=%s", registerRec.Code, registerRec.Body.String())
	}
	assertAPISecurityHeaders(t, registerRec)
	assertNoStoreHeaders(t, registerRec)

	loginReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(`{"email":"owner@example.com","password":"Password1"}`))
	loginReq.Header.Set("Content-Type", "application/json")
	loginRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(loginRec, loginReq)
	if loginRec.Code != http.StatusOK {
		t.Fatalf("expected login 200, got %d body=%s", loginRec.Code, loginRec.Body.String())
	}
	assertAPISecurityHeaders(t, loginRec)
	assertNoStoreHeaders(t, loginRec)
	var loginPayload struct {
		Data application.AuthResult `json:"data"`
	}
	if err := json.Unmarshal(loginRec.Body.Bytes(), &loginPayload); err != nil {
		t.Fatalf("unmarshal login payload: %v", err)
	}

	refreshReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", bytes.NewBufferString(`{"refresh_token":"`+loginPayload.Data.Tokens.RefreshToken+`"}`))
	refreshReq.Header.Set("Content-Type", "application/json")
	refreshRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(refreshRec, refreshReq)
	if refreshRec.Code != http.StatusOK {
		t.Fatalf("expected refresh 200, got %d body=%s", refreshRec.Code, refreshRec.Body.String())
	}
	assertAPISecurityHeaders(t, refreshRec)
	assertNoStoreHeaders(t, refreshRec)

	var refreshPayload struct {
		Data application.AuthResult `json:"data"`
	}
	if err := json.Unmarshal(refreshRec.Body.Bytes(), &refreshPayload); err != nil {
		t.Fatalf("unmarshal refresh payload: %v", err)
	}

	logoutReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", bytes.NewBufferString(`{"refresh_token":"`+refreshPayload.Data.Tokens.RefreshToken+`"}`))
	logoutReq.Header.Set("Content-Type", "application/json")
	logoutRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(logoutRec, logoutReq)
	if logoutRec.Code != http.StatusNoContent {
		t.Fatalf("expected logout 204, got %d body=%s", logoutRec.Code, logoutRec.Body.String())
	}
	assertAPISecurityHeaders(t, logoutRec)
	assertNoStoreHeaders(t, logoutRec)

	meReq := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	meReq.Header.Set("Authorization", "Bearer "+refreshPayload.Data.Tokens.AccessToken)
	meRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(meRec, meReq)
	if meRec.Code != http.StatusOK {
		t.Fatalf("expected me 200, got %d body=%s", meRec.Code, meRec.Body.String())
	}
	assertAPISecurityHeaders(t, meRec)
	assertNoStoreHeaders(t, meRec)

	createWorkspaceReq := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces", bytes.NewBufferString(`{"name":"Engineering"}`))
	createWorkspaceReq.Header.Set("Authorization", "Bearer "+refreshPayload.Data.Tokens.AccessToken)
	createWorkspaceReq.Header.Set("Content-Type", "application/json")
	createWorkspaceRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(createWorkspaceRec, createWorkspaceReq)
	if createWorkspaceRec.Code != http.StatusCreated {
		t.Fatalf("expected workspace create 201, got %d body=%s", createWorkspaceRec.Code, createWorkspaceRec.Body.String())
	}
	assertAPISecurityHeaders(t, createWorkspaceRec)

	var workspacePayload struct {
		Data struct {
			Workspace  domain.Workspace       `json:"workspace"`
			Membership domain.WorkspaceMember `json:"membership"`
		} `json:"data"`
	}
	if err := json.Unmarshal(createWorkspaceRec.Body.Bytes(), &workspacePayload); err != nil {
		t.Fatalf("unmarshal workspace payload: %v", err)
	}

	inviteUnknownReq := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces/"+workspacePayload.Data.Workspace.ID+"/invitations", bytes.NewBufferString(`{"email":"unknown@example.com","role":"viewer"}`))
	inviteUnknownReq.Header.Set("Authorization", "Bearer "+refreshPayload.Data.Tokens.AccessToken)
	inviteUnknownReq.Header.Set("Content-Type", "application/json")
	inviteUnknownRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(inviteUnknownRec, inviteUnknownReq)
	if inviteUnknownRec.Code != http.StatusCreated {
		t.Fatalf("expected unknown invitee invitation 201, got %d body=%s", inviteUnknownRec.Code, inviteUnknownRec.Body.String())
	}
	var inviteUnknownPayload struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(inviteUnknownRec.Body.Bytes(), &inviteUnknownPayload); err != nil {
		t.Fatalf("unmarshal unknown invite payload: %v", err)
	}
	if inviteUnknownPayload.Data["status"] != "pending" {
		t.Fatalf("expected pending status in invite response, got %+v", inviteUnknownPayload.Data)
	}
	if inviteUnknownPayload.Data["version"] != float64(1) {
		t.Fatalf("expected version 1 in invite response, got %+v", inviteUnknownPayload.Data)
	}
	if inviteUnknownPayload.Data["updated_at"] == nil {
		t.Fatalf("expected updated_at in invite response, got %+v", inviteUnknownPayload.Data)
	}

	ownerListReq := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces", nil)
	ownerListReq.Header.Set("Authorization", "Bearer "+refreshPayload.Data.Tokens.AccessToken)
	ownerListRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(ownerListRec, ownerListReq)
	if ownerListRec.Code != http.StatusOK {
		t.Fatalf("expected owner workspace list 200, got %d body=%s", ownerListRec.Code, ownerListRec.Body.String())
	}
	var ownerListPayload struct {
		Data []domain.Workspace `json:"data"`
	}
	if err := json.Unmarshal(ownerListRec.Body.Bytes(), &ownerListPayload); err != nil {
		t.Fatalf("unmarshal owner workspace list: %v", err)
	}
	if len(ownerListPayload.Data) != 1 || ownerListPayload.Data[0].ID != workspacePayload.Data.Workspace.ID {
		t.Fatalf("unexpected owner workspace list: %+v", ownerListPayload.Data)
	}

	renameWorkspaceReq := httptest.NewRequest(http.MethodPatch, "/api/v1/workspaces/"+workspacePayload.Data.Workspace.ID, bytes.NewBufferString(`{"name":"Platform"}`))
	renameWorkspaceReq.Header.Set("Authorization", "Bearer "+refreshPayload.Data.Tokens.AccessToken)
	renameWorkspaceReq.Header.Set("Content-Type", "application/json")
	renameWorkspaceRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(renameWorkspaceRec, renameWorkspaceReq)
	if renameWorkspaceRec.Code != http.StatusOK {
		t.Fatalf("expected workspace rename 200, got %d body=%s", renameWorkspaceRec.Code, renameWorkspaceRec.Body.String())
	}

	duplicateWorkspaceReq := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces", bytes.NewBufferString(`{"name":"Product"}`))
	duplicateWorkspaceReq.Header.Set("Authorization", "Bearer "+refreshPayload.Data.Tokens.AccessToken)
	duplicateWorkspaceReq.Header.Set("Content-Type", "application/json")
	duplicateWorkspaceRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(duplicateWorkspaceRec, duplicateWorkspaceReq)
	if duplicateWorkspaceRec.Code != http.StatusCreated {
		t.Fatalf("expected second workspace create 201, got %d body=%s", duplicateWorkspaceRec.Code, duplicateWorkspaceRec.Body.String())
	}

	renameToDuplicateReq := httptest.NewRequest(http.MethodPatch, "/api/v1/workspaces/"+workspacePayload.Data.Workspace.ID, bytes.NewBufferString(`{"name":" product "}`))
	renameToDuplicateReq.Header.Set("Authorization", "Bearer "+refreshPayload.Data.Tokens.AccessToken)
	renameToDuplicateReq.Header.Set("Content-Type", "application/json")
	renameToDuplicateRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(renameToDuplicateRec, renameToDuplicateReq)
	if renameToDuplicateRec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected duplicate workspace rename 422, got %d body=%s", renameToDuplicateRec.Code, renameToDuplicateRec.Body.String())
	}

	invitedRegisterReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewBufferString(`{"email":"member@example.com","password":"Password1","full_name":"Member"}`))
	invitedRegisterReq.Header.Set("Content-Type", "application/json")
	invitedRegisterRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(invitedRegisterRec, invitedRegisterReq)
	if invitedRegisterRec.Code != http.StatusCreated {
		t.Fatalf("expected invited register 201, got %d body=%s", invitedRegisterRec.Code, invitedRegisterRec.Body.String())
	}

	invitedLoginReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(`{"email":"member@example.com","password":"Password1"}`))
	invitedLoginReq.Header.Set("Content-Type", "application/json")
	invitedLoginRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(invitedLoginRec, invitedLoginReq)
	if invitedLoginRec.Code != http.StatusOK {
		t.Fatalf("expected invited login 200, got %d body=%s", invitedLoginRec.Code, invitedLoginRec.Body.String())
	}
	var invitedLoginPayload struct {
		Data application.AuthResult `json:"data"`
	}
	if err := json.Unmarshal(invitedLoginRec.Body.Bytes(), &invitedLoginPayload); err != nil {
		t.Fatalf("unmarshal invited login payload: %v", err)
	}

	badInviteReq := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces/"+workspacePayload.Data.Workspace.ID+"/invitations", bytes.NewBufferString(`{"email":`))
	badInviteReq.Header.Set("Authorization", "Bearer "+refreshPayload.Data.Tokens.AccessToken)
	badInviteReq.Header.Set("Content-Type", "application/json")
	badInviteRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(badInviteRec, badInviteReq)
	if badInviteRec.Code != http.StatusBadRequest {
		t.Fatalf("expected bad invite json 400, got %d", badInviteRec.Code)
	}

	inviteReq := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces/"+workspacePayload.Data.Workspace.ID+"/invitations", bytes.NewBufferString(`{"email":"member@example.com","role":"editor"}`))
	inviteReq.Header.Set("Authorization", "Bearer "+refreshPayload.Data.Tokens.AccessToken)
	inviteReq.Header.Set("Content-Type", "application/json")
	inviteRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(inviteRec, inviteReq)
	if inviteRec.Code != http.StatusCreated {
		t.Fatalf("expected invite 201, got %d body=%s", inviteRec.Code, inviteRec.Body.String())
	}
	var invitePayloadMap struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(inviteRec.Body.Bytes(), &invitePayloadMap); err != nil {
		t.Fatalf("unmarshal invite response map: %v", err)
	}
	if invitePayloadMap.Data["status"] != "pending" || invitePayloadMap.Data["version"] != float64(1) {
		t.Fatalf("expected public invitation state fields in response, got %+v", invitePayloadMap.Data)
	}

	viewerInviteReq := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces/"+workspacePayload.Data.Workspace.ID+"/invitations", bytes.NewBufferString(`{"email":"x@example.com","role":"viewer"}`))
	viewerInviteReq.Header.Set("Authorization", "Bearer "+invitedLoginPayload.Data.Tokens.AccessToken)
	viewerInviteReq.Header.Set("Content-Type", "application/json")
	viewerInviteRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(viewerInviteRec, viewerInviteReq)
	if viewerInviteRec.Code != http.StatusForbidden {
		t.Fatalf("expected non-owner invite forbidden, got %d body=%s", viewerInviteRec.Code, viewerInviteRec.Body.String())
	}

	var invitationPayload struct {
		Data domain.WorkspaceInvitation `json:"data"`
	}
	if err := json.Unmarshal(inviteRec.Body.Bytes(), &invitationPayload); err != nil {
		t.Fatalf("unmarshal invitation payload: %v", err)
	}

	listMembersReq := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces/"+workspacePayload.Data.Workspace.ID+"/members", nil)
	listMembersReq.Header.Set("Authorization", "Bearer "+refreshPayload.Data.Tokens.AccessToken)
	listMembersRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(listMembersRec, listMembersReq)
	if listMembersRec.Code != http.StatusOK {
		t.Fatalf("expected list members 200, got %d body=%s", listMembersRec.Code, listMembersRec.Body.String())
	}

	acceptReq := httptest.NewRequest(http.MethodPost, "/api/v1/workspace-invitations/"+invitationPayload.Data.ID+"/accept", bytes.NewBufferString(`{"version":1}`))
	acceptReq.Header.Set("Authorization", "Bearer "+invitedLoginPayload.Data.Tokens.AccessToken)
	acceptReq.Header.Set("Content-Type", "application/json")
	acceptRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(acceptRec, acceptReq)
	if acceptRec.Code != http.StatusOK {
		t.Fatalf("expected accept 200, got %d body=%s", acceptRec.Code, acceptRec.Body.String())
	}

	var acceptedMemberPayload struct {
		Data domain.AcceptInvitationResult `json:"data"`
	}
	if err := json.Unmarshal(acceptRec.Body.Bytes(), &acceptedMemberPayload); err != nil {
		t.Fatalf("unmarshal accepted member payload: %v", err)
	}
	if acceptedMemberPayload.Data.Invitation.Status != domain.WorkspaceInvitationStatusAccepted || acceptedMemberPayload.Data.Invitation.Version != 2 {
		t.Fatalf("unexpected accepted invitation payload: %+v", acceptedMemberPayload.Data.Invitation)
	}
	if acceptedMemberPayload.Data.Membership.UserID != invitedLoginPayload.Data.User.ID {
		t.Fatalf("unexpected accepted membership payload: %+v", acceptedMemberPayload.Data.Membership)
	}

	reinviteMemberReq := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces/"+workspacePayload.Data.Workspace.ID+"/invitations", bytes.NewBufferString(`{"email":"member@example.com","role":"viewer"}`))
	reinviteMemberReq.Header.Set("Authorization", "Bearer "+refreshPayload.Data.Tokens.AccessToken)
	reinviteMemberReq.Header.Set("Content-Type", "application/json")
	reinviteMemberRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(reinviteMemberRec, reinviteMemberReq)
	if reinviteMemberRec.Code != http.StatusConflict {
		t.Fatalf("expected existing member invite conflict 409, got %d body=%s", reinviteMemberRec.Code, reinviteMemberRec.Body.String())
	}

	listInvitationsReq := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces/"+workspacePayload.Data.Workspace.ID+"/invitations?status=all&limit=1", nil)
	listInvitationsReq.Header.Set("Authorization", "Bearer "+refreshPayload.Data.Tokens.AccessToken)
	listInvitationsRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(listInvitationsRec, listInvitationsReq)
	if listInvitationsRec.Code != http.StatusOK {
		t.Fatalf("expected invitation list 200, got %d body=%s", listInvitationsRec.Code, listInvitationsRec.Body.String())
	}
	var invitationListPayload struct {
		Data domain.WorkspaceInvitationList `json:"data"`
	}
	if err := json.Unmarshal(listInvitationsRec.Body.Bytes(), &invitationListPayload); err != nil {
		t.Fatalf("unmarshal invitation list payload: %v", err)
	}
	if len(invitationListPayload.Data.Items) != 1 || !invitationListPayload.Data.HasMore || invitationListPayload.Data.NextCursor == nil {
		t.Fatalf("unexpected invitation list page: %+v", invitationListPayload.Data)
	}

	nextInvitationsReq := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces/"+workspacePayload.Data.Workspace.ID+"/invitations?status=all&limit=1&cursor="+*invitationListPayload.Data.NextCursor, nil)
	nextInvitationsReq.Header.Set("Authorization", "Bearer "+refreshPayload.Data.Tokens.AccessToken)
	nextInvitationsRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(nextInvitationsRec, nextInvitationsReq)
	if nextInvitationsRec.Code != http.StatusOK {
		t.Fatalf("expected invitation cursor page 200, got %d body=%s", nextInvitationsRec.Code, nextInvitationsRec.Body.String())
	}
	var nextInvitationListPayload struct {
		Data domain.WorkspaceInvitationList `json:"data"`
	}
	if err := json.Unmarshal(nextInvitationsRec.Body.Bytes(), &nextInvitationListPayload); err != nil {
		t.Fatalf("unmarshal next invitation list payload: %v", err)
	}
	if len(nextInvitationListPayload.Data.Items) != 1 || nextInvitationListPayload.Data.HasMore || nextInvitationListPayload.Data.NextCursor != nil {
		t.Fatalf("unexpected final invitation list page: %+v", nextInvitationListPayload.Data)
	}

	myInvitationsReq := httptest.NewRequest(http.MethodGet, "/api/v1/my/invitations?status=all&limit=1", nil)
	myInvitationsReq.Header.Set("Authorization", "Bearer "+invitedLoginPayload.Data.Tokens.AccessToken)
	myInvitationsRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(myInvitationsRec, myInvitationsReq)
	if myInvitationsRec.Code != http.StatusOK {
		t.Fatalf("expected my invitations 200, got %d body=%s", myInvitationsRec.Code, myInvitationsRec.Body.String())
	}
	var myInvitationsPayload struct {
		Data domain.WorkspaceInvitationList `json:"data"`
	}
	if err := json.Unmarshal(myInvitationsRec.Body.Bytes(), &myInvitationsPayload); err != nil {
		t.Fatalf("unmarshal my invitations payload: %v", err)
	}
	if len(myInvitationsPayload.Data.Items) != 1 || myInvitationsPayload.Data.HasMore || myInvitationsPayload.Data.NextCursor != nil {
		t.Fatalf("unexpected my invitations page: %+v", myInvitationsPayload.Data)
	}

	viewerListInvitationsReq := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces/"+workspacePayload.Data.Workspace.ID+"/invitations", nil)
	viewerListInvitationsReq.Header.Set("Authorization", "Bearer "+invitedLoginPayload.Data.Tokens.AccessToken)
	viewerListInvitationsRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(viewerListInvitationsRec, viewerListInvitationsReq)
	if viewerListInvitationsRec.Code != http.StatusForbidden {
		t.Fatalf("expected non-owner invitation list forbidden, got %d body=%s", viewerListInvitationsRec.Code, viewerListInvitationsRec.Body.String())
	}

	invitedListReq := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces", nil)
	invitedListReq.Header.Set("Authorization", "Bearer "+invitedLoginPayload.Data.Tokens.AccessToken)
	invitedListRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(invitedListRec, invitedListReq)
	if invitedListRec.Code != http.StatusOK {
		t.Fatalf("expected invited workspace list 200, got %d body=%s", invitedListRec.Code, invitedListRec.Body.String())
	}
	var invitedListPayload struct {
		Data []domain.Workspace `json:"data"`
	}
	if err := json.Unmarshal(invitedListRec.Body.Bytes(), &invitedListPayload); err != nil {
		t.Fatalf("unmarshal invited workspace list: %v", err)
	}
	if len(invitedListPayload.Data) != 1 || invitedListPayload.Data[0].ID != workspacePayload.Data.Workspace.ID {
		t.Fatalf("unexpected invited workspace list: %+v", invitedListPayload.Data)
	}

	badUpdateReq := httptest.NewRequest(http.MethodPatch, "/api/v1/workspaces/"+workspacePayload.Data.Workspace.ID+"/members/"+acceptedMemberPayload.Data.Membership.ID+"/role", bytes.NewBufferString(`{"role":`))
	badUpdateReq.Header.Set("Authorization", "Bearer "+refreshPayload.Data.Tokens.AccessToken)
	badUpdateReq.Header.Set("Content-Type", "application/json")
	badUpdateRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(badUpdateRec, badUpdateReq)
	if badUpdateRec.Code != http.StatusBadRequest {
		t.Fatalf("expected bad update json 400, got %d", badUpdateRec.Code)
	}

	updateReq := httptest.NewRequest(http.MethodPatch, "/api/v1/workspaces/"+workspacePayload.Data.Workspace.ID+"/members/"+acceptedMemberPayload.Data.Membership.ID+"/role", bytes.NewBufferString(`{"role":"viewer"}`))
	updateReq.Header.Set("Authorization", "Bearer "+refreshPayload.Data.Tokens.AccessToken)
	updateReq.Header.Set("Content-Type", "application/json")
	updateRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(updateRec, updateReq)
	if updateRec.Code != http.StatusOK {
		t.Fatalf("expected update role 200, got %d body=%s", updateRec.Code, updateRec.Body.String())
	}
}

func TestMapErrorDefaultBranch(t *testing.T) {
	status, apiErr := mapError(errors.New("boom"))
	if status != http.StatusInternalServerError || apiErr.Code != "internal_error" {
		t.Fatalf("expected internal error mapping, got status=%d code=%s", status, apiErr.Code)
	}
}

func TestMapErrorNormalizesValidationMessage(t *testing.T) {
	status, apiErr := mapError(fmt.Errorf("%w: workspace name is required", domain.ErrValidation))
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d", status)
	}
	if apiErr.Code != "validation_failed" {
		t.Fatalf("expected validation_failed code, got %s", apiErr.Code)
	}
	if apiErr.Message != "workspace name is required" {
		t.Fatalf("expected normalized validation message, got %q", apiErr.Message)
	}
}

func TestAuthRequestLogsDoNotContainSecrets(t *testing.T) {
	var output bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&output, nil))
	tokenManager := appauth.NewTokenManager("super-secret-token", "note-app", 15*time.Minute)
	userRepo := &httpUserRepo{byID: map[string]domain.User{}, byEmail: map[string]domain.User{}}
	refreshRepo := &httpRefreshTokenRepo{byHash: map[string]domain.RefreshToken{}}

	authService := application.NewAuthService(userRepo, refreshRepo, appauth.NewPasswordManager(), tokenManager, 24*time.Hour)
	server := NewServer(logger, authService, application.WorkspaceService{}, application.FolderService{}, application.PageService{}, application.RevisionService{}, tokenManager, storage.NewLocal(t.TempDir()))

	registerReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewBufferString(`{"email":"owner@example.com","password":"Password1","full_name":"Owner"}`))
	registerReq.Header.Set("Content-Type", "application/json")
	registerRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(registerRec, registerReq)
	if registerRec.Code != http.StatusCreated {
		t.Fatalf("expected register 201, got %d body=%s", registerRec.Code, registerRec.Body.String())
	}

	loginReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(`{"email":"owner@example.com","password":"Password1"}`))
	loginReq.Header.Set("Content-Type", "application/json")
	loginRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(loginRec, loginReq)
	if loginRec.Code != http.StatusOK {
		t.Fatalf("expected login 200, got %d body=%s", loginRec.Code, loginRec.Body.String())
	}

	var loginPayload struct {
		Data application.AuthResult `json:"data"`
	}
	if err := json.Unmarshal(loginRec.Body.Bytes(), &loginPayload); err != nil {
		t.Fatalf("unmarshal login payload: %v", err)
	}

	refreshReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", bytes.NewBufferString(`{"refresh_token":"`+loginPayload.Data.Tokens.RefreshToken+`"}`))
	refreshReq.Header.Set("Content-Type", "application/json")
	refreshRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(refreshRec, refreshReq)
	if refreshRec.Code != http.StatusOK {
		t.Fatalf("expected refresh 200, got %d body=%s", refreshRec.Code, refreshRec.Body.String())
	}

	meReq := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	meReq.Header.Set("Authorization", "Bearer "+loginPayload.Data.Tokens.AccessToken)
	meRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(meRec, meReq)
	if meRec.Code != http.StatusOK {
		t.Fatalf("expected me 200, got %d body=%s", meRec.Code, meRec.Body.String())
	}

	logs := output.String()
	if !strings.Contains(logs, `"msg":"http request"`) {
		t.Fatalf("expected request logs, got %s", logs)
	}

	storedUser := userRepo.byEmail["owner@example.com"]
	secrets := []string{
		"Password1",
		loginPayload.Data.Tokens.AccessToken,
		loginPayload.Data.Tokens.RefreshToken,
		"Bearer " + loginPayload.Data.Tokens.AccessToken,
		storedUser.PasswordHash,
	}
	for _, secret := range secrets {
		if secret == "" {
			continue
		}
		if strings.Contains(logs, secret) {
			t.Fatalf("expected logs to omit secret %q, got %s", secret, logs)
		}
	}
}

func TestAuthAndWorkspaceErrorMappings(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	tokenManager := appauth.NewTokenManager("super-secret-token", "note-app", 15*time.Minute)
	userRepo := &httpUserRepo{byID: map[string]domain.User{}, byEmail: map[string]domain.User{}}
	refreshRepo := &httpRefreshTokenRepo{byHash: map[string]domain.RefreshToken{}}
	workspaceRepo := &httpWorkspaceRepo{memberships: map[string][]domain.WorkspaceMember{}, invitations: map[string]domain.WorkspaceInvitation{}, workspaces: map[string]domain.Workspace{}}

	authService := application.NewAuthService(userRepo, refreshRepo, appauth.NewPasswordManager(), tokenManager, 24*time.Hour)
	workspaceService := application.NewWorkspaceService(workspaceRepo, userRepo)
	server := NewServer(logger, authService, workspaceService, application.FolderService{}, application.PageService{}, application.RevisionService{}, tokenManager, storage.NewLocal(t.TempDir()))

	register := func(email string) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewBufferString(`{"email":"`+email+`","password":"Password1","full_name":"X"}`))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		server.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("register failed: %d %s", rec.Code, rec.Body.String())
		}
	}
	register("owner@example.com")
	validOwnerToken, _, err := tokenManager.GenerateAccessToken(userRepo.byEmail["owner@example.com"].ID, "owner@example.com", time.Now().UTC())
	if err != nil {
		t.Fatalf("GenerateAccessToken valid owner token error: %v", err)
	}

	dupReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewBufferString(`{"email":"owner@example.com","password":"Password1","full_name":"X"}`))
	dupReq.Header.Set("Content-Type", "application/json")
	dupRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(dupRec, dupReq)
	if dupRec.Code != http.StatusConflict {
		t.Fatalf("expected duplicate register conflict, got %d", dupRec.Code)
	}

	wrongLoginReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(`{"email":"owner@example.com","password":"Wrong123"}`))
	wrongLoginReq.Header.Set("Content-Type", "application/json")
	wrongLoginRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(wrongLoginRec, wrongLoginReq)
	if wrongLoginRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected wrong login unauthorized, got %d", wrongLoginRec.Code)
	}

	missingRefreshReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", bytes.NewBufferString(`{"refresh_token":"missing"}`))
	missingRefreshReq.Header.Set("Content-Type", "application/json")
	missingRefreshRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(missingRefreshRec, missingRefreshReq)
	if missingRefreshRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected missing refresh unauthorized, got %d", missingRefreshRec.Code)
	}

	ownerToken, _, err := tokenManager.GenerateAccessToken("owner-id", "owner@example.com", time.Now().UTC())
	if err != nil {
		t.Fatalf("GenerateAccessToken error: %v", err)
	}
	meReq := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	meReq.Header.Set("Authorization", "Bearer "+ownerToken)
	meRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(meRec, meReq)
	if meRec.Code != http.StatusNotFound {
		t.Fatalf("expected me not found, got %d", meRec.Code)
	}

	workspaceReq := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces", bytes.NewBufferString(`{"name":"   "}`))
	workspaceReq.Header.Set("Authorization", "Bearer "+ownerToken)
	workspaceReq.Header.Set("Content-Type", "application/json")
	workspaceRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(workspaceRec, workspaceReq)
	if workspaceRec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected workspace validation failure, got %d", workspaceRec.Code)
	}
	var workspacePayload map[string]map[string]string
	if err := json.Unmarshal(workspaceRec.Body.Bytes(), &workspacePayload); err != nil {
		t.Fatalf("unmarshal workspace validation failure: %v", err)
	}
	if workspacePayload["error"]["message"] != "workspace name is required" {
		t.Fatalf("expected normalized workspace validation message, got %q", workspacePayload["error"]["message"])
	}

	unknownActorReq := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces", bytes.NewBufferString(`{"name":"Engineering"}`))
	unknownActorReq.Header.Set("Authorization", "Bearer "+ownerToken)
	unknownActorReq.Header.Set("Content-Type", "application/json")
	unknownActorRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(unknownActorRec, unknownActorReq)
	if unknownActorRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected workspace unauthorized for unknown actor, got %d body=%s", unknownActorRec.Code, unknownActorRec.Body.String())
	}

	inviteReq := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces/w1/invitations", bytes.NewBufferString(`{"email":"x@example.com","role":"bad"}`))
	inviteReq.Header.Set("Authorization", "Bearer "+ownerToken)
	inviteReq.Header.Set("Content-Type", "application/json")
	inviteRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(inviteRec, inviteReq)
	if inviteRec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected invite validation failure, got %d", inviteRec.Code)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces/w1/members", nil)
	listReq.Header.Set("Authorization", "Bearer "+ownerToken)
	listRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusForbidden {
		t.Fatalf("expected list members forbidden, got %d", listRec.Code)
	}

	invitationListReq := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces/w1/invitations?status=bad", nil)
	invitationListReq.Header.Set("Authorization", "Bearer "+ownerToken)
	invitationListRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(invitationListRec, invitationListReq)
	if invitationListRec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected invalid invitation list filter failure, got %d body=%s", invitationListRec.Code, invitationListRec.Body.String())
	}

	myInvitationListReq := httptest.NewRequest(http.MethodGet, "/api/v1/my/invitations?status=bad", nil)
	myInvitationListReq.Header.Set("Authorization", "Bearer "+validOwnerToken)
	myInvitationListRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(myInvitationListRec, myInvitationListReq)
	if myInvitationListRec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected invalid my invitation list filter failure, got %d body=%s", myInvitationListRec.Code, myInvitationListRec.Body.String())
	}

	updateReq := httptest.NewRequest(http.MethodPatch, "/api/v1/workspaces/w1/members/m1/role", bytes.NewBufferString(`{"role":"bad"}`))
	updateReq.Header.Set("Authorization", "Bearer "+ownerToken)
	updateReq.Header.Set("Content-Type", "application/json")
	updateRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(updateRec, updateReq)
	if updateRec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected update role validation failure, got %d", updateRec.Code)
	}

	renameReq := httptest.NewRequest(http.MethodPatch, "/api/v1/workspaces/w1", bytes.NewBufferString(`{"name":"Renamed"}`))
	renameReq.Header.Set("Authorization", "Bearer "+ownerToken)
	renameReq.Header.Set("Content-Type", "application/json")
	renameRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(renameRec, renameReq)
	if renameRec.Code != http.StatusForbidden {
		t.Fatalf("expected workspace rename forbidden, got %d", renameRec.Code)
	}

	acceptReq := httptest.NewRequest(http.MethodPost, "/api/v1/workspace-invitations/missing/accept", bytes.NewBufferString(`{"version":1}`))
	acceptReq.Header.Set("Authorization", "Bearer "+ownerToken)
	acceptReq.Header.Set("Content-Type", "application/json")
	acceptRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(acceptRec, acceptReq)
	if acceptRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected accept invitation unauthorized for unknown actor, got %d", acceptRec.Code)
	}
}

func TestAcceptInvitationReturnsNotFoundForMismatchedEmail(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	tokenManager := appauth.NewTokenManager("super-secret-token", "note-app", 15*time.Minute)
	userRepo := &httpUserRepo{byID: map[string]domain.User{}, byEmail: map[string]domain.User{}}
	refreshRepo := &httpRefreshTokenRepo{byHash: map[string]domain.RefreshToken{}}
	workspaceRepo := &httpWorkspaceRepo{memberships: map[string][]domain.WorkspaceMember{}, invitations: map[string]domain.WorkspaceInvitation{}, workspaces: map[string]domain.Workspace{}}

	authService := application.NewAuthService(userRepo, refreshRepo, appauth.NewPasswordManager(), tokenManager, 24*time.Hour)
	workspaceService := application.NewWorkspaceService(workspaceRepo, userRepo)
	server := NewServer(logger, authService, workspaceService, application.FolderService{}, application.PageService{}, application.RevisionService{}, tokenManager, storage.NewLocal(t.TempDir()))

	registerAndLogin := func(email string) application.AuthResult {
		registerReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewBufferString(`{"email":"`+email+`","password":"Password1","full_name":"User"}`))
		registerReq.Header.Set("Content-Type", "application/json")
		registerRec := httptest.NewRecorder()
		server.Handler().ServeHTTP(registerRec, registerReq)
		if registerRec.Code != http.StatusCreated {
			t.Fatalf("register failed for %s: %d %s", email, registerRec.Code, registerRec.Body.String())
		}

		loginReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(`{"email":"`+email+`","password":"Password1"}`))
		loginReq.Header.Set("Content-Type", "application/json")
		loginRec := httptest.NewRecorder()
		server.Handler().ServeHTTP(loginRec, loginReq)
		if loginRec.Code != http.StatusOK {
			t.Fatalf("login failed for %s: %d %s", email, loginRec.Code, loginRec.Body.String())
		}

		var payload struct {
			Data application.AuthResult `json:"data"`
		}
		if err := json.Unmarshal(loginRec.Body.Bytes(), &payload); err != nil {
			t.Fatalf("unmarshal login payload for %s: %v", email, err)
		}
		return payload.Data
	}

	ownerAuth := registerAndLogin("owner@example.com")
	outsiderAuth := registerAndLogin("outsider@example.com")

	createWorkspaceReq := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces", bytes.NewBufferString(`{"name":"Engineering"}`))
	createWorkspaceReq.Header.Set("Authorization", "Bearer "+ownerAuth.Tokens.AccessToken)
	createWorkspaceReq.Header.Set("Content-Type", "application/json")
	createWorkspaceRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(createWorkspaceRec, createWorkspaceReq)
	if createWorkspaceRec.Code != http.StatusCreated {
		t.Fatalf("workspace create failed: %d %s", createWorkspaceRec.Code, createWorkspaceRec.Body.String())
	}
	var workspacePayload struct {
		Data struct {
			Workspace domain.Workspace `json:"workspace"`
		} `json:"data"`
	}
	if err := json.Unmarshal(createWorkspaceRec.Body.Bytes(), &workspacePayload); err != nil {
		t.Fatalf("unmarshal workspace payload: %v", err)
	}

	inviteReq := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces/"+workspacePayload.Data.Workspace.ID+"/invitations", bytes.NewBufferString(`{"email":"member@example.com","role":"viewer"}`))
	inviteReq.Header.Set("Authorization", "Bearer "+ownerAuth.Tokens.AccessToken)
	inviteReq.Header.Set("Content-Type", "application/json")
	inviteRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(inviteRec, inviteReq)
	if inviteRec.Code != http.StatusCreated {
		t.Fatalf("invite create failed: %d %s", inviteRec.Code, inviteRec.Body.String())
	}
	var invitationPayload struct {
		Data domain.WorkspaceInvitation `json:"data"`
	}
	if err := json.Unmarshal(inviteRec.Body.Bytes(), &invitationPayload); err != nil {
		t.Fatalf("unmarshal invitation payload: %v", err)
	}

	acceptReq := httptest.NewRequest(http.MethodPost, "/api/v1/workspace-invitations/"+invitationPayload.Data.ID+"/accept", bytes.NewBufferString(`{"version":1}`))
	acceptReq.Header.Set("Authorization", "Bearer "+outsiderAuth.Tokens.AccessToken)
	acceptReq.Header.Set("Content-Type", "application/json")
	acceptRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(acceptRec, acceptReq)
	if acceptRec.Code != http.StatusNotFound {
		t.Fatalf("expected mismatched invitation accept to return 404, got %d body=%s", acceptRec.Code, acceptRec.Body.String())
	}
}

func TestAcceptInvitationEndpointRequiresVersion(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	tokenManager := appauth.NewTokenManager("super-secret-token", "note-app", 15*time.Minute)
	userRepo := &httpUserRepo{byID: map[string]domain.User{}, byEmail: map[string]domain.User{}}
	refreshRepo := &httpRefreshTokenRepo{byHash: map[string]domain.RefreshToken{}}
	workspaceRepo := &httpWorkspaceRepo{memberships: map[string][]domain.WorkspaceMember{}, invitations: map[string]domain.WorkspaceInvitation{}, workspaces: map[string]domain.Workspace{}}

	authService := application.NewAuthService(userRepo, refreshRepo, appauth.NewPasswordManager(), tokenManager, 24*time.Hour)
	workspaceService := application.NewWorkspaceService(workspaceRepo, userRepo)
	server := NewServer(logger, authService, workspaceService, application.FolderService{}, application.PageService{}, application.RevisionService{}, tokenManager, storage.NewLocal(t.TempDir()))

	registerAndLogin := func(email string) application.AuthResult {
		registerReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewBufferString(`{"email":"`+email+`","password":"Password1","full_name":"User"}`))
		registerReq.Header.Set("Content-Type", "application/json")
		registerRec := httptest.NewRecorder()
		server.Handler().ServeHTTP(registerRec, registerReq)
		if registerRec.Code != http.StatusCreated {
			t.Fatalf("register failed for %s: %d %s", email, registerRec.Code, registerRec.Body.String())
		}
		loginReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(`{"email":"`+email+`","password":"Password1"}`))
		loginReq.Header.Set("Content-Type", "application/json")
		loginRec := httptest.NewRecorder()
		server.Handler().ServeHTTP(loginRec, loginReq)
		if loginRec.Code != http.StatusOK {
			t.Fatalf("login failed for %s: %d %s", email, loginRec.Code, loginRec.Body.String())
		}
		var payload struct {
			Data application.AuthResult `json:"data"`
		}
		if err := json.Unmarshal(loginRec.Body.Bytes(), &payload); err != nil {
			t.Fatalf("unmarshal login payload for %s: %v", email, err)
		}
		return payload.Data
	}

	ownerAuth := registerAndLogin("accept-owner@example.com")
	memberAuth := registerAndLogin("accept-member@example.com")

	createWorkspaceReq := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces", bytes.NewBufferString(`{"name":"Engineering"}`))
	createWorkspaceReq.Header.Set("Authorization", "Bearer "+ownerAuth.Tokens.AccessToken)
	createWorkspaceReq.Header.Set("Content-Type", "application/json")
	createWorkspaceRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(createWorkspaceRec, createWorkspaceReq)
	if createWorkspaceRec.Code != http.StatusCreated {
		t.Fatalf("workspace create failed: %d %s", createWorkspaceRec.Code, createWorkspaceRec.Body.String())
	}
	var workspacePayload struct {
		Data struct {
			Workspace domain.Workspace `json:"workspace"`
		} `json:"data"`
	}
	if err := json.Unmarshal(createWorkspaceRec.Body.Bytes(), &workspacePayload); err != nil {
		t.Fatalf("unmarshal workspace payload: %v", err)
	}

	inviteReq := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces/"+workspacePayload.Data.Workspace.ID+"/invitations", bytes.NewBufferString(`{"email":"accept-member@example.com","role":"viewer"}`))
	inviteReq.Header.Set("Authorization", "Bearer "+ownerAuth.Tokens.AccessToken)
	inviteReq.Header.Set("Content-Type", "application/json")
	inviteRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(inviteRec, inviteReq)
	if inviteRec.Code != http.StatusCreated {
		t.Fatalf("invite create failed: %d %s", inviteRec.Code, inviteRec.Body.String())
	}
	var invitationPayload struct {
		Data domain.WorkspaceInvitation `json:"data"`
	}
	if err := json.Unmarshal(inviteRec.Body.Bytes(), &invitationPayload); err != nil {
		t.Fatalf("unmarshal invitation payload: %v", err)
	}

	missingVersionReq := httptest.NewRequest(http.MethodPost, "/api/v1/workspace-invitations/"+invitationPayload.Data.ID+"/accept", bytes.NewBufferString(`{}`))
	missingVersionReq.Header.Set("Authorization", "Bearer "+memberAuth.Tokens.AccessToken)
	missingVersionReq.Header.Set("Content-Type", "application/json")
	missingVersionRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(missingVersionRec, missingVersionReq)
	if missingVersionRec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected accept invitation missing version 422, got %d body=%s", missingVersionRec.Code, missingVersionRec.Body.String())
	}

	unknownFieldReq := httptest.NewRequest(http.MethodPost, "/api/v1/workspace-invitations/"+invitationPayload.Data.ID+"/accept", bytes.NewBufferString(`{"version":1,"role":"viewer"}`))
	unknownFieldReq.Header.Set("Authorization", "Bearer "+memberAuth.Tokens.AccessToken)
	unknownFieldReq.Header.Set("Content-Type", "application/json")
	unknownFieldRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(unknownFieldRec, unknownFieldReq)
	if unknownFieldRec.Code != http.StatusBadRequest {
		t.Fatalf("expected accept invitation unknown field 400, got %d body=%s", unknownFieldRec.Code, unknownFieldRec.Body.String())
	}

	staleReq := httptest.NewRequest(http.MethodPost, "/api/v1/workspace-invitations/"+invitationPayload.Data.ID+"/accept", bytes.NewBufferString(`{"version":2}`))
	staleReq.Header.Set("Authorization", "Bearer "+memberAuth.Tokens.AccessToken)
	staleReq.Header.Set("Content-Type", "application/json")
	staleRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(staleRec, staleReq)
	if staleRec.Code != http.StatusConflict {
		t.Fatalf("expected accept invitation stale version 409, got %d body=%s", staleRec.Code, staleRec.Body.String())
	}
}

func TestRejectInvitationEndpointRequiresVersion(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	tokenManager := appauth.NewTokenManager("super-secret-token", "note-app", 15*time.Minute)
	userRepo := &httpUserRepo{byID: map[string]domain.User{}, byEmail: map[string]domain.User{}}
	refreshRepo := &httpRefreshTokenRepo{byHash: map[string]domain.RefreshToken{}}
	workspaceRepo := &httpWorkspaceRepo{memberships: map[string][]domain.WorkspaceMember{}, invitations: map[string]domain.WorkspaceInvitation{}, workspaces: map[string]domain.Workspace{}}

	authService := application.NewAuthService(userRepo, refreshRepo, appauth.NewPasswordManager(), tokenManager, 24*time.Hour)
	workspaceService := application.NewWorkspaceService(workspaceRepo, userRepo)
	server := NewServer(logger, authService, workspaceService, application.FolderService{}, application.PageService{}, application.RevisionService{}, tokenManager, storage.NewLocal(t.TempDir()))

	registerAndLogin := func(email string) application.AuthResult {
		registerReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewBufferString(`{"email":"`+email+`","password":"Password1","full_name":"User"}`))
		registerReq.Header.Set("Content-Type", "application/json")
		registerRec := httptest.NewRecorder()
		server.Handler().ServeHTTP(registerRec, registerReq)
		if registerRec.Code != http.StatusCreated {
			t.Fatalf("register failed for %s: %d %s", email, registerRec.Code, registerRec.Body.String())
		}
		loginReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(`{"email":"`+email+`","password":"Password1"}`))
		loginReq.Header.Set("Content-Type", "application/json")
		loginRec := httptest.NewRecorder()
		server.Handler().ServeHTTP(loginRec, loginReq)
		if loginRec.Code != http.StatusOK {
			t.Fatalf("login failed for %s: %d %s", email, loginRec.Code, loginRec.Body.String())
		}
		var payload struct {
			Data application.AuthResult `json:"data"`
		}
		if err := json.Unmarshal(loginRec.Body.Bytes(), &payload); err != nil {
			t.Fatalf("unmarshal login payload for %s: %v", email, err)
		}
		return payload.Data
	}

	ownerAuth := registerAndLogin("reject-owner@example.com")
	memberAuth := registerAndLogin("reject-member@example.com")
	outsiderAuth := registerAndLogin("reject-outsider@example.com")

	createWorkspaceReq := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces", bytes.NewBufferString(`{"name":"Engineering"}`))
	createWorkspaceReq.Header.Set("Authorization", "Bearer "+ownerAuth.Tokens.AccessToken)
	createWorkspaceReq.Header.Set("Content-Type", "application/json")
	createWorkspaceRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(createWorkspaceRec, createWorkspaceReq)
	if createWorkspaceRec.Code != http.StatusCreated {
		t.Fatalf("workspace create failed: %d %s", createWorkspaceRec.Code, createWorkspaceRec.Body.String())
	}
	var workspacePayload struct {
		Data struct {
			Workspace domain.Workspace `json:"workspace"`
		} `json:"data"`
	}
	if err := json.Unmarshal(createWorkspaceRec.Body.Bytes(), &workspacePayload); err != nil {
		t.Fatalf("unmarshal workspace payload: %v", err)
	}

	inviteReq := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces/"+workspacePayload.Data.Workspace.ID+"/invitations", bytes.NewBufferString(`{"email":"reject-member@example.com","role":"viewer"}`))
	inviteReq.Header.Set("Authorization", "Bearer "+ownerAuth.Tokens.AccessToken)
	inviteReq.Header.Set("Content-Type", "application/json")
	inviteRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(inviteRec, inviteReq)
	if inviteRec.Code != http.StatusCreated {
		t.Fatalf("invite create failed: %d %s", inviteRec.Code, inviteRec.Body.String())
	}
	var invitationPayload struct {
		Data domain.WorkspaceInvitation `json:"data"`
	}
	if err := json.Unmarshal(inviteRec.Body.Bytes(), &invitationPayload); err != nil {
		t.Fatalf("unmarshal invitation payload: %v", err)
	}

	missingVersionReq := httptest.NewRequest(http.MethodPost, "/api/v1/workspace-invitations/"+invitationPayload.Data.ID+"/reject", bytes.NewBufferString(`{}`))
	missingVersionReq.Header.Set("Authorization", "Bearer "+memberAuth.Tokens.AccessToken)
	missingVersionReq.Header.Set("Content-Type", "application/json")
	missingVersionRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(missingVersionRec, missingVersionReq)
	if missingVersionRec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected reject invitation missing version 422, got %d body=%s", missingVersionRec.Code, missingVersionRec.Body.String())
	}

	unknownFieldReq := httptest.NewRequest(http.MethodPost, "/api/v1/workspace-invitations/"+invitationPayload.Data.ID+"/reject", bytes.NewBufferString(`{"version":1,"role":"viewer"}`))
	unknownFieldReq.Header.Set("Authorization", "Bearer "+memberAuth.Tokens.AccessToken)
	unknownFieldReq.Header.Set("Content-Type", "application/json")
	unknownFieldRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(unknownFieldRec, unknownFieldReq)
	if unknownFieldRec.Code != http.StatusBadRequest {
		t.Fatalf("expected reject invitation unknown field 400, got %d body=%s", unknownFieldRec.Code, unknownFieldRec.Body.String())
	}

	foreignReq := httptest.NewRequest(http.MethodPost, "/api/v1/workspace-invitations/"+invitationPayload.Data.ID+"/reject", bytes.NewBufferString(`{"version":1}`))
	foreignReq.Header.Set("Authorization", "Bearer "+outsiderAuth.Tokens.AccessToken)
	foreignReq.Header.Set("Content-Type", "application/json")
	foreignRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(foreignRec, foreignReq)
	if foreignRec.Code != http.StatusNotFound {
		t.Fatalf("expected reject invitation foreign user 404, got %d body=%s", foreignRec.Code, foreignRec.Body.String())
	}

	staleReq := httptest.NewRequest(http.MethodPost, "/api/v1/workspace-invitations/"+invitationPayload.Data.ID+"/reject", bytes.NewBufferString(`{"version":2}`))
	staleReq.Header.Set("Authorization", "Bearer "+memberAuth.Tokens.AccessToken)
	staleReq.Header.Set("Content-Type", "application/json")
	staleRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(staleRec, staleReq)
	if staleRec.Code != http.StatusConflict {
		t.Fatalf("expected reject invitation stale version 409, got %d body=%s", staleRec.Code, staleRec.Body.String())
	}

	successReq := httptest.NewRequest(http.MethodPost, "/api/v1/workspace-invitations/"+invitationPayload.Data.ID+"/reject", bytes.NewBufferString(`{"version":1}`))
	successReq.Header.Set("Authorization", "Bearer "+memberAuth.Tokens.AccessToken)
	successReq.Header.Set("Content-Type", "application/json")
	successRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(successRec, successReq)
	if successRec.Code != http.StatusOK {
		t.Fatalf("expected reject invitation 200, got %d body=%s", successRec.Code, successRec.Body.String())
	}
	var successPayload struct {
		Data domain.WorkspaceInvitation `json:"data"`
	}
	if err := json.Unmarshal(successRec.Body.Bytes(), &successPayload); err != nil {
		t.Fatalf("unmarshal reject invitation payload: %v", err)
	}
	if successPayload.Data.Status != domain.WorkspaceInvitationStatusRejected || successPayload.Data.Version != 2 {
		t.Fatalf("unexpected rejected invitation payload: %+v", successPayload.Data)
	}
	if successPayload.Data.RespondedBy == nil || *successPayload.Data.RespondedBy != memberAuth.User.ID || successPayload.Data.RespondedAt == nil {
		t.Fatalf("unexpected reject metadata payload: %+v", successPayload.Data)
	}
	if successPayload.Data.AcceptedAt != nil {
		t.Fatalf("expected accepted_at to remain nil after reject, got %+v", successPayload.Data)
	}
}

func TestCancelInvitationEndpointRequiresVersion(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	tokenManager := appauth.NewTokenManager("super-secret-token", "note-app", 15*time.Minute)
	userRepo := &httpUserRepo{byID: map[string]domain.User{}, byEmail: map[string]domain.User{}}
	refreshRepo := &httpRefreshTokenRepo{byHash: map[string]domain.RefreshToken{}}
	workspaceRepo := &httpWorkspaceRepo{memberships: map[string][]domain.WorkspaceMember{}, invitations: map[string]domain.WorkspaceInvitation{}, workspaces: map[string]domain.Workspace{}}

	authService := application.NewAuthService(userRepo, refreshRepo, appauth.NewPasswordManager(), tokenManager, 24*time.Hour)
	workspaceService := application.NewWorkspaceService(workspaceRepo, userRepo)
	server := NewServer(logger, authService, workspaceService, application.FolderService{}, application.PageService{}, application.RevisionService{}, tokenManager, storage.NewLocal(t.TempDir()))

	registerAndLogin := func(email string) application.AuthResult {
		registerReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewBufferString(`{"email":"`+email+`","password":"Password1","full_name":"User"}`))
		registerReq.Header.Set("Content-Type", "application/json")
		registerRec := httptest.NewRecorder()
		server.Handler().ServeHTTP(registerRec, registerReq)
		if registerRec.Code != http.StatusCreated {
			t.Fatalf("register failed for %s: %d %s", email, registerRec.Code, registerRec.Body.String())
		}
		loginReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(`{"email":"`+email+`","password":"Password1"}`))
		loginReq.Header.Set("Content-Type", "application/json")
		loginRec := httptest.NewRecorder()
		server.Handler().ServeHTTP(loginRec, loginReq)
		if loginRec.Code != http.StatusOK {
			t.Fatalf("login failed for %s: %d %s", email, loginRec.Code, loginRec.Body.String())
		}
		var payload struct {
			Data application.AuthResult `json:"data"`
		}
		if err := json.Unmarshal(loginRec.Body.Bytes(), &payload); err != nil {
			t.Fatalf("unmarshal login payload for %s: %v", email, err)
		}
		return payload.Data
	}

	ownerAuth := registerAndLogin("cancel-owner@example.com")
	secondOwnerAuth := registerAndLogin("cancel-second-owner@example.com")
	memberAuth := registerAndLogin("cancel-member@example.com")
	targetAuth := registerAndLogin("cancel-target@example.com")
	outsiderAuth := registerAndLogin("cancel-outsider@example.com")

	createWorkspaceReq := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces", bytes.NewBufferString(`{"name":"Engineering"}`))
	createWorkspaceReq.Header.Set("Authorization", "Bearer "+ownerAuth.Tokens.AccessToken)
	createWorkspaceReq.Header.Set("Content-Type", "application/json")
	createWorkspaceRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(createWorkspaceRec, createWorkspaceReq)
	if createWorkspaceRec.Code != http.StatusCreated {
		t.Fatalf("workspace create failed: %d %s", createWorkspaceRec.Code, createWorkspaceRec.Body.String())
	}
	var workspacePayload struct {
		Data struct {
			Workspace domain.Workspace `json:"workspace"`
		} `json:"data"`
	}
	if err := json.Unmarshal(createWorkspaceRec.Body.Bytes(), &workspacePayload); err != nil {
		t.Fatalf("unmarshal workspace payload: %v", err)
	}

	joinAsEditor := func(userID string) {
		workspaceRepo.memberships[workspacePayload.Data.Workspace.ID] = append(workspaceRepo.memberships[workspacePayload.Data.Workspace.ID], domain.WorkspaceMember{
			ID:          uuid.NewString(),
			WorkspaceID: workspacePayload.Data.Workspace.ID,
			UserID:      userID,
			Role:        domain.RoleEditor,
			CreatedAt:   time.Now().UTC(),
		})
	}
	joinAsOwner := func(userID string) {
		workspaceRepo.memberships[workspacePayload.Data.Workspace.ID] = append(workspaceRepo.memberships[workspacePayload.Data.Workspace.ID], domain.WorkspaceMember{
			ID:          uuid.NewString(),
			WorkspaceID: workspacePayload.Data.Workspace.ID,
			UserID:      userID,
			Role:        domain.RoleOwner,
			CreatedAt:   time.Now().UTC(),
		})
	}
	joinAsEditor(memberAuth.User.ID)
	joinAsOwner(secondOwnerAuth.User.ID)

	createInvitation := func(email string) domain.WorkspaceInvitation {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces/"+workspacePayload.Data.Workspace.ID+"/invitations", bytes.NewBufferString(`{"email":"`+email+`","role":"viewer"}`))
		req.Header.Set("Authorization", "Bearer "+ownerAuth.Tokens.AccessToken)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		server.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("invite create failed for %s: %d %s", email, rec.Code, rec.Body.String())
		}
		var payload struct {
			Data domain.WorkspaceInvitation `json:"data"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
			t.Fatalf("unmarshal invitation payload for %s: %v", email, err)
		}
		return payload.Data
	}

	missingVersionInvitation := createInvitation("cancel-target-1@example.com")
	missingVersionReq := httptest.NewRequest(http.MethodPost, "/api/v1/workspace-invitations/"+missingVersionInvitation.ID+"/cancel", bytes.NewBufferString(`{}`))
	missingVersionReq.Header.Set("Authorization", "Bearer "+ownerAuth.Tokens.AccessToken)
	missingVersionReq.Header.Set("Content-Type", "application/json")
	missingVersionRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(missingVersionRec, missingVersionReq)
	if missingVersionRec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected cancel invitation missing version 422, got %d body=%s", missingVersionRec.Code, missingVersionRec.Body.String())
	}

	unknownFieldInvitation := createInvitation("cancel-target-2@example.com")
	unknownFieldReq := httptest.NewRequest(http.MethodPost, "/api/v1/workspace-invitations/"+unknownFieldInvitation.ID+"/cancel", bytes.NewBufferString(`{"version":1,"role":"viewer"}`))
	unknownFieldReq.Header.Set("Authorization", "Bearer "+ownerAuth.Tokens.AccessToken)
	unknownFieldReq.Header.Set("Content-Type", "application/json")
	unknownFieldRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(unknownFieldRec, unknownFieldReq)
	if unknownFieldRec.Code != http.StatusBadRequest {
		t.Fatalf("expected cancel invitation unknown field 400, got %d body=%s", unknownFieldRec.Code, unknownFieldRec.Body.String())
	}

	outsiderInvitation := createInvitation("cancel-target-3@example.com")
	outsiderReq := httptest.NewRequest(http.MethodPost, "/api/v1/workspace-invitations/"+outsiderInvitation.ID+"/cancel", bytes.NewBufferString(`{"version":1}`))
	outsiderReq.Header.Set("Authorization", "Bearer "+outsiderAuth.Tokens.AccessToken)
	outsiderReq.Header.Set("Content-Type", "application/json")
	outsiderRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(outsiderRec, outsiderReq)
	if outsiderRec.Code != http.StatusNotFound {
		t.Fatalf("expected cancel invitation outsider 404, got %d body=%s", outsiderRec.Code, outsiderRec.Body.String())
	}

	nonOwnerInvitation := createInvitation("cancel-target-4@example.com")
	nonOwnerReq := httptest.NewRequest(http.MethodPost, "/api/v1/workspace-invitations/"+nonOwnerInvitation.ID+"/cancel", bytes.NewBufferString(`{"version":1}`))
	nonOwnerReq.Header.Set("Authorization", "Bearer "+memberAuth.Tokens.AccessToken)
	nonOwnerReq.Header.Set("Content-Type", "application/json")
	nonOwnerRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(nonOwnerRec, nonOwnerReq)
	if nonOwnerRec.Code != http.StatusForbidden {
		t.Fatalf("expected cancel invitation non-owner member 403, got %d body=%s", nonOwnerRec.Code, nonOwnerRec.Body.String())
	}

	staleInvitation := createInvitation("cancel-target-5@example.com")
	staleReq := httptest.NewRequest(http.MethodPost, "/api/v1/workspace-invitations/"+staleInvitation.ID+"/cancel", bytes.NewBufferString(`{"version":2}`))
	staleReq.Header.Set("Authorization", "Bearer "+ownerAuth.Tokens.AccessToken)
	staleReq.Header.Set("Content-Type", "application/json")
	staleRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(staleRec, staleReq)
	if staleRec.Code != http.StatusConflict {
		t.Fatalf("expected cancel invitation stale version 409, got %d body=%s", staleRec.Code, staleRec.Body.String())
	}

	successInvitation := createInvitation("cancel-target-6@example.com")
	successReq := httptest.NewRequest(http.MethodPost, "/api/v1/workspace-invitations/"+successInvitation.ID+"/cancel", bytes.NewBufferString(`{"version":1}`))
	successReq.Header.Set("Authorization", "Bearer "+secondOwnerAuth.Tokens.AccessToken)
	successReq.Header.Set("Content-Type", "application/json")
	successRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(successRec, successReq)
	if successRec.Code != http.StatusOK {
		t.Fatalf("expected cancel invitation 200, got %d body=%s", successRec.Code, successRec.Body.String())
	}
	var successPayload struct {
		Data domain.WorkspaceInvitation `json:"data"`
	}
	if err := json.Unmarshal(successRec.Body.Bytes(), &successPayload); err != nil {
		t.Fatalf("unmarshal cancel invitation payload: %v", err)
	}
	if successPayload.Data.Status != domain.WorkspaceInvitationStatusCancelled || successPayload.Data.Version != 2 {
		t.Fatalf("unexpected cancelled invitation payload: %+v", successPayload.Data)
	}
	if successPayload.Data.CancelledBy == nil || *successPayload.Data.CancelledBy != secondOwnerAuth.User.ID || successPayload.Data.CancelledAt == nil {
		t.Fatalf("unexpected cancel metadata payload: %+v", successPayload.Data)
	}
	if successPayload.Data.RespondedBy != nil || successPayload.Data.RespondedAt != nil || successPayload.Data.AcceptedAt != nil {
		t.Fatalf("expected respond/accept metadata to remain nil after cancel, got %+v", successPayload.Data)
	}

	_ = targetAuth
}

func TestPatchWorkspaceInvitationEndpoint(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	tokenManager := appauth.NewTokenManager("super-secret-token", "note-app", 15*time.Minute)
	userRepo := &httpUserRepo{byID: map[string]domain.User{}, byEmail: map[string]domain.User{}}
	refreshRepo := &httpRefreshTokenRepo{byHash: map[string]domain.RefreshToken{}}
	workspaceRepo := &httpWorkspaceRepo{memberships: map[string][]domain.WorkspaceMember{}, invitations: map[string]domain.WorkspaceInvitation{}, workspaces: map[string]domain.Workspace{}}

	authService := application.NewAuthService(userRepo, refreshRepo, appauth.NewPasswordManager(), tokenManager, 24*time.Hour)
	workspaceService := application.NewWorkspaceService(workspaceRepo, userRepo)
	server := NewServer(logger, authService, workspaceService, application.FolderService{}, application.PageService{}, application.RevisionService{}, tokenManager, storage.NewLocal(t.TempDir()))

	registerAndLogin := func(email string) application.AuthResult {
		registerReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewBufferString(`{"email":"`+email+`","password":"Password1","full_name":"User"}`))
		registerReq.Header.Set("Content-Type", "application/json")
		registerRec := httptest.NewRecorder()
		server.Handler().ServeHTTP(registerRec, registerReq)
		if registerRec.Code != http.StatusCreated {
			t.Fatalf("register failed for %s: %d %s", email, registerRec.Code, registerRec.Body.String())
		}

		loginReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(`{"email":"`+email+`","password":"Password1"}`))
		loginReq.Header.Set("Content-Type", "application/json")
		loginRec := httptest.NewRecorder()
		server.Handler().ServeHTTP(loginRec, loginReq)
		if loginRec.Code != http.StatusOK {
			t.Fatalf("login failed for %s: %d %s", email, loginRec.Code, loginRec.Body.String())
		}

		var payload struct {
			Data application.AuthResult `json:"data"`
		}
		if err := json.Unmarshal(loginRec.Body.Bytes(), &payload); err != nil {
			t.Fatalf("unmarshal login payload for %s: %v", email, err)
		}
		return payload.Data
	}

	ownerAuth := registerAndLogin("patch-owner@example.com")
	nonOwnerAuth := registerAndLogin("patch-non-owner@example.com")

	createWorkspaceReq := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces", bytes.NewBufferString(`{"name":"Engineering"}`))
	createWorkspaceReq.Header.Set("Authorization", "Bearer "+ownerAuth.Tokens.AccessToken)
	createWorkspaceReq.Header.Set("Content-Type", "application/json")
	createWorkspaceRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(createWorkspaceRec, createWorkspaceReq)
	if createWorkspaceRec.Code != http.StatusCreated {
		t.Fatalf("workspace create failed: %d %s", createWorkspaceRec.Code, createWorkspaceRec.Body.String())
	}
	var workspacePayload struct {
		Data struct {
			Workspace domain.Workspace `json:"workspace"`
		} `json:"data"`
	}
	if err := json.Unmarshal(createWorkspaceRec.Body.Bytes(), &workspacePayload); err != nil {
		t.Fatalf("unmarshal workspace payload: %v", err)
	}

	createInvitation := func(email string, role string) domain.WorkspaceInvitation {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces/"+workspacePayload.Data.Workspace.ID+"/invitations", bytes.NewBufferString(`{"email":"`+email+`","role":"`+role+`"}`))
		req.Header.Set("Authorization", "Bearer "+ownerAuth.Tokens.AccessToken)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		server.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("invite create failed for %s: %d %s", email, rec.Code, rec.Body.String())
		}
		var payload struct {
			Data domain.WorkspaceInvitation `json:"data"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
			t.Fatalf("unmarshal invitation payload for %s: %v", email, err)
		}
		return payload.Data
	}

	pending := createInvitation("patch-member@example.com", "viewer")

	successReq := httptest.NewRequest(http.MethodPatch, "/api/v1/workspace-invitations/"+pending.ID, bytes.NewBufferString(fmt.Sprintf(`{"role":"editor","version":%d}`, pending.Version)))
	successReq.Header.Set("Authorization", "Bearer "+ownerAuth.Tokens.AccessToken)
	successReq.Header.Set("Content-Type", "application/json")
	successRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(successRec, successReq)
	if successRec.Code != http.StatusOK {
		t.Fatalf("expected patch invitation 200, got %d body=%s", successRec.Code, successRec.Body.String())
	}
	var successPayload struct {
		Data domain.WorkspaceInvitation `json:"data"`
	}
	if err := json.Unmarshal(successRec.Body.Bytes(), &successPayload); err != nil {
		t.Fatalf("unmarshal patch invitation payload: %v", err)
	}
	if successPayload.Data.Role != domain.RoleEditor || successPayload.Data.Version != pending.Version+1 || successPayload.Data.Status != domain.WorkspaceInvitationStatusPending {
		t.Fatalf("unexpected patched invitation payload: %+v", successPayload.Data)
	}

	noopReq := httptest.NewRequest(http.MethodPatch, "/api/v1/workspace-invitations/"+pending.ID, bytes.NewBufferString(fmt.Sprintf(`{"role":"editor","version":%d}`, successPayload.Data.Version)))
	noopReq.Header.Set("Authorization", "Bearer "+ownerAuth.Tokens.AccessToken)
	noopReq.Header.Set("Content-Type", "application/json")
	noopRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(noopRec, noopReq)
	if noopRec.Code != http.StatusOK {
		t.Fatalf("expected patch invitation no-op 200, got %d body=%s", noopRec.Code, noopRec.Body.String())
	}
	var noopPayload struct {
		Data domain.WorkspaceInvitation `json:"data"`
	}
	if err := json.Unmarshal(noopRec.Body.Bytes(), &noopPayload); err != nil {
		t.Fatalf("unmarshal noop patch invitation payload: %v", err)
	}
	if noopPayload.Data.Version != successPayload.Data.Version || !noopPayload.Data.UpdatedAt.Equal(successPayload.Data.UpdatedAt) {
		t.Fatalf("expected no-op invitation unchanged, got %+v", noopPayload.Data)
	}

	notFoundReq := httptest.NewRequest(http.MethodPatch, "/api/v1/workspace-invitations/missing", bytes.NewBufferString(`{"role":"viewer","version":1}`))
	notFoundReq.Header.Set("Authorization", "Bearer "+ownerAuth.Tokens.AccessToken)
	notFoundReq.Header.Set("Content-Type", "application/json")
	notFoundRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(notFoundRec, notFoundReq)
	if notFoundRec.Code != http.StatusNotFound {
		t.Fatalf("expected patch invitation not found 404, got %d body=%s", notFoundRec.Code, notFoundRec.Body.String())
	}

	nonOwnerReq := httptest.NewRequest(http.MethodPatch, "/api/v1/workspace-invitations/"+pending.ID, bytes.NewBufferString(fmt.Sprintf(`{"role":"viewer","version":%d}`, successPayload.Data.Version)))
	nonOwnerReq.Header.Set("Authorization", "Bearer "+nonOwnerAuth.Tokens.AccessToken)
	nonOwnerReq.Header.Set("Content-Type", "application/json")
	nonOwnerRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(nonOwnerRec, nonOwnerReq)
	if nonOwnerRec.Code != http.StatusForbidden {
		t.Fatalf("expected patch invitation forbidden 403, got %d body=%s", nonOwnerRec.Code, nonOwnerRec.Body.String())
	}

	staleReq := httptest.NewRequest(http.MethodPatch, "/api/v1/workspace-invitations/"+pending.ID, bytes.NewBufferString(`{"role":"viewer","version":1}`))
	staleReq.Header.Set("Authorization", "Bearer "+ownerAuth.Tokens.AccessToken)
	staleReq.Header.Set("Content-Type", "application/json")
	staleRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(staleRec, staleReq)
	if staleRec.Code != http.StatusConflict {
		t.Fatalf("expected patch invitation conflict 409, got %d body=%s", staleRec.Code, staleRec.Body.String())
	}

	accepted := createInvitation("patch-accepted@example.com", "viewer")
	acceptedUserAuth := registerAndLogin("patch-accepted@example.com")
	acceptReq := httptest.NewRequest(http.MethodPost, "/api/v1/workspace-invitations/"+accepted.ID+"/accept", bytes.NewBufferString(`{"version":1}`))
	acceptReq.Header.Set("Authorization", "Bearer "+acceptedUserAuth.Tokens.AccessToken)
	acceptReq.Header.Set("Content-Type", "application/json")
	acceptRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(acceptRec, acceptReq)
	if acceptRec.Code != http.StatusOK {
		t.Fatalf("accept invitation setup failed: %d %s", acceptRec.Code, acceptRec.Body.String())
	}

	terminalReq := httptest.NewRequest(http.MethodPatch, "/api/v1/workspace-invitations/"+accepted.ID, bytes.NewBufferString(`{"role":"editor","version":1}`))
	terminalReq.Header.Set("Authorization", "Bearer "+ownerAuth.Tokens.AccessToken)
	terminalReq.Header.Set("Content-Type", "application/json")
	terminalRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(terminalRec, terminalReq)
	if terminalRec.Code != http.StatusConflict {
		t.Fatalf("expected patch accepted invitation conflict 409, got %d body=%s", terminalRec.Code, terminalRec.Body.String())
	}

	invalidRoleReq := httptest.NewRequest(http.MethodPatch, "/api/v1/workspace-invitations/"+pending.ID, bytes.NewBufferString(`{"role":"bad","version":2}`))
	invalidRoleReq.Header.Set("Authorization", "Bearer "+ownerAuth.Tokens.AccessToken)
	invalidRoleReq.Header.Set("Content-Type", "application/json")
	invalidRoleRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(invalidRoleRec, invalidRoleReq)
	if invalidRoleRec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected patch invitation invalid role 422, got %d body=%s", invalidRoleRec.Code, invalidRoleRec.Body.String())
	}

	invalidVersionReq := httptest.NewRequest(http.MethodPatch, "/api/v1/workspace-invitations/"+pending.ID, bytes.NewBufferString(`{"role":"viewer","version":0}`))
	invalidVersionReq.Header.Set("Authorization", "Bearer "+ownerAuth.Tokens.AccessToken)
	invalidVersionReq.Header.Set("Content-Type", "application/json")
	invalidVersionRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(invalidVersionRec, invalidVersionReq)
	if invalidVersionRec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected patch invitation invalid version 422, got %d body=%s", invalidVersionRec.Code, invalidVersionRec.Body.String())
	}

	unknownFieldReq := httptest.NewRequest(http.MethodPatch, "/api/v1/workspace-invitations/"+pending.ID, bytes.NewBufferString(`{"role":"viewer","version":2,"email":"other@example.com"}`))
	unknownFieldReq.Header.Set("Authorization", "Bearer "+ownerAuth.Tokens.AccessToken)
	unknownFieldReq.Header.Set("Content-Type", "application/json")
	unknownFieldRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(unknownFieldRec, unknownFieldReq)
	if unknownFieldRec.Code != http.StatusBadRequest {
		t.Fatalf("expected patch invitation unknown field 400, got %d body=%s", unknownFieldRec.Code, unknownFieldRec.Body.String())
	}
}

func TestAuthRoutesUseStricterRateLimit(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	tokenManager := appauth.NewTokenManager("super-secret-token", "note-app", 15*time.Minute)
	userRepo := &httpUserRepo{byID: map[string]domain.User{}, byEmail: map[string]domain.User{}}
	refreshRepo := &httpRefreshTokenRepo{byHash: map[string]domain.RefreshToken{}}
	authService := application.NewAuthService(userRepo, refreshRepo, appauth.NewPasswordManager(), tokenManager, 24*time.Hour)
	server := NewServer(logger, authService, application.WorkspaceService{}, application.FolderService{}, application.PageService{}, application.RevisionService{}, tokenManager, storage.NewLocal(t.TempDir()))
	handler := server.Handler()

	registerReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewBufferString(`{"email":"owner@example.com","password":"Password1","full_name":"Owner"}`))
	registerReq.RemoteAddr = "203.0.113.50:1234"
	registerReq.Header.Set("Content-Type", "application/json")
	registerRec := httptest.NewRecorder()
	handler.ServeHTTP(registerRec, registerReq)
	if registerRec.Code != http.StatusCreated {
		t.Fatalf("expected register status 201, got %d body=%s", registerRec.Code, registerRec.Body.String())
	}

	for i := 0; i < 5; i++ {
		loginReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(`{"email":"owner@example.com","password":"Wrong123"}`))
		loginReq.RemoteAddr = "203.0.113.50:5678"
		loginReq.Header.Set("Content-Type", "application/json")
		loginRec := httptest.NewRecorder()
		handler.ServeHTTP(loginRec, loginReq)
		if loginRec.Code != http.StatusUnauthorized {
			t.Fatalf("login request %d: expected 401 before auth limit, got %d body=%s", i+1, loginRec.Code, loginRec.Body.String())
		}
	}

	limitedLoginReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(`{"email":"owner@example.com","password":"Wrong123"}`))
	limitedLoginReq.RemoteAddr = "203.0.113.50:9999"
	limitedLoginReq.Header.Set("Content-Type", "application/json")
	limitedLoginRec := httptest.NewRecorder()
	handler.ServeHTTP(limitedLoginRec, limitedLoginReq)
	if limitedLoginRec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected login rate limit status 429, got %d body=%s", limitedLoginRec.Code, limitedLoginRec.Body.String())
	}
	if got := limitedLoginRec.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("expected login rate limit content-type application/json, got %q", got)
	}
	if got := limitedLoginRec.Header().Get("Retry-After"); got == "" {
		t.Fatal("expected login rate limit retry-after header")
	} else if seconds, err := strconv.Atoi(got); err != nil || seconds < 1 || seconds > 60 {
		t.Fatalf("expected login rate limit retry-after between 1 and 60 seconds, got %q", got)
	}
	var limitedLoginPayload map[string]map[string]string
	if err := json.Unmarshal(limitedLoginRec.Body.Bytes(), &limitedLoginPayload); err != nil {
		t.Fatalf("parse login rate limit body: %v", err)
	}
	if limitedLoginPayload["error"]["code"] != "rate_limited" || limitedLoginPayload["error"]["message"] != "too many requests" {
		t.Fatalf("unexpected login rate limit payload: %+v", limitedLoginPayload)
	}

	for i := 0; i < 5; i++ {
		refreshReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", bytes.NewBufferString(`{"refresh_token":"missing"}`))
		refreshReq.RemoteAddr = "203.0.113.51:1234"
		refreshReq.Header.Set("Content-Type", "application/json")
		refreshRec := httptest.NewRecorder()
		handler.ServeHTTP(refreshRec, refreshReq)
		if refreshRec.Code != http.StatusUnauthorized {
			t.Fatalf("refresh request %d: expected 401 before auth limit, got %d body=%s", i+1, refreshRec.Code, refreshRec.Body.String())
		}
	}

	limitedRefreshReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", bytes.NewBufferString(`{"refresh_token":"missing"}`))
	limitedRefreshReq.RemoteAddr = "203.0.113.51:9999"
	limitedRefreshReq.Header.Set("Content-Type", "application/json")
	limitedRefreshRec := httptest.NewRecorder()
	handler.ServeHTTP(limitedRefreshRec, limitedRefreshReq)
	if limitedRefreshRec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected refresh rate limit status 429, got %d body=%s", limitedRefreshRec.Code, limitedRefreshRec.Body.String())
	}
	if got := limitedRefreshRec.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("expected refresh rate limit content-type application/json, got %q", got)
	}
	if got := limitedRefreshRec.Header().Get("Retry-After"); got == "" {
		t.Fatal("expected refresh rate limit retry-after header")
	} else if seconds, err := strconv.Atoi(got); err != nil || seconds < 1 || seconds > 60 {
		t.Fatalf("expected refresh rate limit retry-after between 1 and 60 seconds, got %q", got)
	}
	var limitedRefreshPayload map[string]map[string]string
	if err := json.Unmarshal(limitedRefreshRec.Body.Bytes(), &limitedRefreshPayload); err != nil {
		t.Fatalf("parse refresh rate limit body: %v", err)
	}
	if limitedRefreshPayload["error"]["code"] != "rate_limited" || limitedRefreshPayload["error"]["message"] != "too many requests" {
		t.Fatalf("unexpected refresh rate limit payload: %+v", limitedRefreshPayload)
	}

	registerOtherReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewBufferString(`{"email":"other@example.com","password":"Password1","full_name":"Other"}`))
	registerOtherReq.RemoteAddr = "203.0.113.50:4444"
	registerOtherReq.Header.Set("Content-Type", "application/json")
	registerOtherRec := httptest.NewRecorder()
	handler.ServeHTTP(registerOtherRec, registerOtherReq)
	if registerOtherRec.Code != http.StatusCreated {
		t.Fatalf("expected register to stay outside strict auth limit, got %d body=%s", registerOtherRec.Code, registerOtherRec.Body.String())
	}
}

func TestListWorkspacesIsUserScoped(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	tokenManager := appauth.NewTokenManager("super-secret-token", "note-app", 15*time.Minute)
	userRepo := &httpUserRepo{byID: map[string]domain.User{}, byEmail: map[string]domain.User{}}
	refreshRepo := &httpRefreshTokenRepo{byHash: map[string]domain.RefreshToken{}}
	workspaceRepo := &httpWorkspaceRepo{memberships: map[string][]domain.WorkspaceMember{}, invitations: map[string]domain.WorkspaceInvitation{}, workspaces: map[string]domain.Workspace{}}

	authService := application.NewAuthService(userRepo, refreshRepo, appauth.NewPasswordManager(), tokenManager, 24*time.Hour)
	workspaceService := application.NewWorkspaceService(workspaceRepo, userRepo)
	server := NewServer(logger, authService, workspaceService, application.FolderService{}, application.PageService{}, application.RevisionService{}, tokenManager, storage.NewLocal(t.TempDir()))

	registerAndLogin := func(email string) string {
		registerReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewBufferString(`{"email":"`+email+`","password":"Password1","full_name":"User"}`))
		registerReq.Header.Set("Content-Type", "application/json")
		registerRec := httptest.NewRecorder()
		server.Handler().ServeHTTP(registerRec, registerReq)
		if registerRec.Code != http.StatusCreated {
			t.Fatalf("register failed for %s: %d %s", email, registerRec.Code, registerRec.Body.String())
		}

		loginReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(`{"email":"`+email+`","password":"Password1"}`))
		loginReq.Header.Set("Content-Type", "application/json")
		loginRec := httptest.NewRecorder()
		server.Handler().ServeHTTP(loginRec, loginReq)
		if loginRec.Code != http.StatusOK {
			t.Fatalf("login failed for %s: %d %s", email, loginRec.Code, loginRec.Body.String())
		}

		var loginPayload struct {
			Data application.AuthResult `json:"data"`
		}
		if err := json.Unmarshal(loginRec.Body.Bytes(), &loginPayload); err != nil {
			t.Fatalf("unmarshal login payload for %s: %v", email, err)
		}
		return loginPayload.Data.Tokens.AccessToken
	}

	ownerToken := registerAndLogin("owner@example.com")
	otherToken := registerAndLogin("other@example.com")

	createWorkspace := func(accessToken, name string) string {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces", bytes.NewBufferString(`{"name":"`+name+`"}`))
		req.Header.Set("Authorization", "Bearer "+accessToken)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		server.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("create workspace failed: %d %s", rec.Code, rec.Body.String())
		}
		var payload struct {
			Data struct {
				Workspace domain.Workspace `json:"workspace"`
			} `json:"data"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
			t.Fatalf("unmarshal workspace create payload: %v", err)
		}
		return payload.Data.Workspace.ID
	}

	ownerWorkspaceID := createWorkspace(ownerToken, "Owner Workspace")
	otherWorkspaceID := createWorkspace(otherToken, "Other Workspace")

	listForUser := func(accessToken string) []domain.Workspace {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces", nil)
		req.Header.Set("Authorization", "Bearer "+accessToken)
		rec := httptest.NewRecorder()
		server.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("list workspace failed: %d %s", rec.Code, rec.Body.String())
		}

		var payload struct {
			Data []domain.Workspace `json:"data"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
			t.Fatalf("unmarshal workspace list payload: %v", err)
		}
		return payload.Data
	}

	ownerWorkspaces := listForUser(ownerToken)
	if len(ownerWorkspaces) != 1 || ownerWorkspaces[0].ID != ownerWorkspaceID {
		t.Fatalf("owner should only see own workspace, got %+v", ownerWorkspaces)
	}

	otherWorkspaces := listForUser(otherToken)
	if len(otherWorkspaces) != 1 || otherWorkspaces[0].ID != otherWorkspaceID {
		t.Fatalf("other user should only see own workspace, got %+v", otherWorkspaces)
	}
}
