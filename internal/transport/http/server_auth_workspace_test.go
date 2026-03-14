package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
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

func (r *httpWorkspaceRepo) CreateInvitation(_ context.Context, invitation domain.WorkspaceInvitation) (domain.WorkspaceInvitation, error) {
	if r.invitations == nil {
		r.invitations = map[string]domain.WorkspaceInvitation{}
	}
	for _, existing := range r.invitations {
		if existing.WorkspaceID == invitation.WorkspaceID && existing.Email == invitation.Email && existing.AcceptedAt == nil {
			return domain.WorkspaceInvitation{}, domain.ErrConflict
		}
	}
	r.invitations[invitation.ID] = invitation
	return invitation, nil
}

func (r *httpWorkspaceRepo) GetActiveInvitationByEmail(_ context.Context, workspaceID, email string) (domain.WorkspaceInvitation, error) {
	for _, invitation := range r.invitations {
		if invitation.WorkspaceID == workspaceID && invitation.Email == email && invitation.AcceptedAt == nil {
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

func (r *httpWorkspaceRepo) AcceptInvitation(_ context.Context, invitationID, userID string, acceptedAt time.Time) (domain.WorkspaceMember, error) {
	invitation, ok := r.invitations[invitationID]
	if !ok {
		return domain.WorkspaceMember{}, domain.ErrNotFound
	}
	if invitation.AcceptedAt != nil {
		return domain.WorkspaceMember{}, domain.ErrConflict
	}
	invitation.AcceptedAt = &acceptedAt
	r.invitations[invitationID] = invitation
	member := domain.WorkspaceMember{ID: uuid.NewString(), WorkspaceID: invitation.WorkspaceID, UserID: userID, Role: invitation.Role, CreatedAt: acceptedAt}
	r.memberships[invitation.WorkspaceID] = append(r.memberships[invitation.WorkspaceID], member)
	return member, nil
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

	registerBody := `{"email":"owner@example.com","password":"Password1","full_name":"Owner"}`
	registerReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewBufferString(registerBody))
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

	meReq := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	meReq.Header.Set("Authorization", "Bearer "+refreshPayload.Data.Tokens.AccessToken)
	meRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(meRec, meReq)
	if meRec.Code != http.StatusOK {
		t.Fatalf("expected me 200, got %d body=%s", meRec.Code, meRec.Body.String())
	}

	createWorkspaceReq := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces", bytes.NewBufferString(`{"name":"Engineering"}`))
	createWorkspaceReq.Header.Set("Authorization", "Bearer "+refreshPayload.Data.Tokens.AccessToken)
	createWorkspaceReq.Header.Set("Content-Type", "application/json")
	createWorkspaceRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(createWorkspaceRec, createWorkspaceReq)
	if createWorkspaceRec.Code != http.StatusCreated {
		t.Fatalf("expected workspace create 201, got %d body=%s", createWorkspaceRec.Code, createWorkspaceRec.Body.String())
	}

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
	if inviteUnknownRec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected unknown invitee validation 422, got %d body=%s", inviteUnknownRec.Code, inviteUnknownRec.Body.String())
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

	acceptReq := httptest.NewRequest(http.MethodPost, "/api/v1/workspace-invitations/"+invitationPayload.Data.ID+"/accept", nil)
	acceptReq.Header.Set("Authorization", "Bearer "+invitedLoginPayload.Data.Tokens.AccessToken)
	acceptRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(acceptRec, acceptReq)
	if acceptRec.Code != http.StatusOK {
		t.Fatalf("expected accept 200, got %d body=%s", acceptRec.Code, acceptRec.Body.String())
	}

	var acceptedMemberPayload struct {
		Data domain.WorkspaceMember `json:"data"`
	}
	if err := json.Unmarshal(acceptRec.Body.Bytes(), &acceptedMemberPayload); err != nil {
		t.Fatalf("unmarshal accepted member payload: %v", err)
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

	badUpdateReq := httptest.NewRequest(http.MethodPatch, "/api/v1/workspaces/"+workspacePayload.Data.Workspace.ID+"/members/"+acceptedMemberPayload.Data.ID+"/role", bytes.NewBufferString(`{"role":`))
	badUpdateReq.Header.Set("Authorization", "Bearer "+refreshPayload.Data.Tokens.AccessToken)
	badUpdateReq.Header.Set("Content-Type", "application/json")
	badUpdateRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(badUpdateRec, badUpdateReq)
	if badUpdateRec.Code != http.StatusBadRequest {
		t.Fatalf("expected bad update json 400, got %d", badUpdateRec.Code)
	}

	updateReq := httptest.NewRequest(http.MethodPatch, "/api/v1/workspaces/"+workspacePayload.Data.Workspace.ID+"/members/"+acceptedMemberPayload.Data.ID+"/role", bytes.NewBufferString(`{"role":"viewer"}`))
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

	acceptReq := httptest.NewRequest(http.MethodPost, "/api/v1/workspace-invitations/missing/accept", nil)
	acceptReq.Header.Set("Authorization", "Bearer "+ownerToken)
	acceptRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(acceptRec, acceptReq)
	if acceptRec.Code != http.StatusNotFound {
		t.Fatalf("expected accept invitation not found, got %d", acceptRec.Code)
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
