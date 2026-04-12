package http

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"note-app/internal/application"
	"note-app/internal/domain"

	"github.com/go-chi/chi/v5"
)

type registerRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	FullName string `json:"full_name"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type createWorkspaceRequest struct {
	Name string `json:"name"`
}

type updateWorkspaceRequest struct {
	Name string `json:"name"`
}

type inviteMemberRequest struct {
	Email string `json:"email"`
	Role  string `json:"role"`
}

type updateInvitationRequest struct {
	Role    string `json:"role"`
	Version int64  `json:"version"`
}

type acceptInvitationRequest struct {
	Version int64 `json:"version"`
}

type rejectInvitationRequest struct {
	Version int64 `json:"version"`
}

type cancelInvitationRequest struct {
	Version int64 `json:"version"`
}

type updateMemberRoleRequest struct {
	Role string `json:"role"`
}

type createFolderRequest struct {
	Name     string  `json:"name"`
	ParentID *string `json:"parent_id"`
}

type updateFolderRequest struct {
	Name string `json:"name"`
}

type createPageRequest struct {
	Title    string  `json:"title"`
	FolderID *string `json:"folder_id"`
}

type updatePageRequest struct {
	Title    *string         `json:"title"`
	FolderID json.RawMessage `json:"folder_id"`
}

type updateDraftRequest struct {
	Content json.RawMessage `json:"content"`
}

type createRevisionRequest struct {
	Label *string `json:"label"`
	Note  *string `json:"note"`
}

type createCommentRequest struct {
	Body string `json:"body"`
}

type createThreadAnchorRequest struct {
	Type            string  `json:"type"`
	BlockID         string  `json:"block_id"`
	QuotedText      *string `json:"quoted_text"`
	QuotedBlockText string  `json:"quoted_block_text"`
}

type createThreadRequest struct {
	Body     string                    `json:"body"`
	Mentions []string                  `json:"mentions"`
	Anchor   createThreadAnchorRequest `json:"anchor"`
}

type createThreadReplyRequest struct {
	Body     string   `json:"body"`
	Mentions []string `json:"mentions"`
}

type resolveThreadRequest struct {
	ResolveNote string `json:"resolve_note"`
}

type reopenThreadRequest struct {
	ReopenReason string `json:"reopen_reason"`
}

type updateThreadNotificationPreferenceRequest struct {
	Mode string `json:"mode"`
}

type batchMarkNotificationsReadRequest struct {
	NotificationIDs []string `json:"notification_ids"`
}

func (s Server) handleRegister() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req registerRequest
		if err := DecodeJSON(w, r, &req); err != nil {
			WriteError(w, r, http.StatusBadRequest, NewAPIError("invalid_json", "request body must be valid JSON"))
			return
		}

		user, err := s.authService.Register(r.Context(), application.RegisterInput{
			Email:    req.Email,
			Password: req.Password,
			FullName: req.FullName,
		})
		if err != nil {
			s.writeMappedError(w, r, err)
			return
		}

		WriteJSON(w, http.StatusCreated, user)
	}
}

func (s Server) handleLogin() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req loginRequest
		if err := DecodeJSON(w, r, &req); err != nil {
			WriteError(w, r, http.StatusBadRequest, NewAPIError("invalid_json", "request body must be valid JSON"))
			return
		}

		result, err := s.authService.Login(r.Context(), application.LoginInput{Email: req.Email, Password: req.Password})
		if err != nil {
			s.writeMappedError(w, r, err)
			return
		}

		WriteJSON(w, http.StatusOK, result)
	}
}

func (s Server) handleRefresh() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req refreshRequest
		if err := DecodeJSON(w, r, &req); err != nil {
			WriteError(w, r, http.StatusBadRequest, NewAPIError("invalid_json", "request body must be valid JSON"))
			return
		}

		result, err := s.authService.Refresh(r.Context(), req.RefreshToken)
		if err != nil {
			s.writeMappedError(w, r, err)
			return
		}

		WriteJSON(w, http.StatusOK, result)
	}
}

func (s Server) handleLogout() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req refreshRequest
		if err := DecodeJSON(w, r, &req); err != nil {
			WriteError(w, r, http.StatusBadRequest, NewAPIError("invalid_json", "request body must be valid JSON"))
			return
		}

		if err := s.authService.Logout(r.Context(), req.RefreshToken); err != nil {
			s.writeMappedError(w, r, err)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

func (s Server) handleCurrentUser() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, err := s.authService.CurrentUser(r.Context(), requestContextUserID(r.Context()))
		if err != nil {
			s.writeMappedError(w, r, err)
			return
		}

		WriteJSON(w, http.StatusOK, user)
	}
}

func (s Server) handleCreateWorkspace() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req createWorkspaceRequest
		if err := DecodeJSON(w, r, &req); err != nil {
			WriteError(w, r, http.StatusBadRequest, NewAPIError("invalid_json", "request body must be valid JSON"))
			return
		}

		workspace, member, err := s.workspaceService.CreateWorkspace(r.Context(), requestContextUserID(r.Context()), application.CreateWorkspaceInput{Name: req.Name})
		if err != nil {
			s.writeMappedError(w, r, err)
			return
		}

		WriteJSON(w, http.StatusCreated, map[string]any{"workspace": workspace, "membership": member})
	}
}

func (s Server) handleListWorkspaces() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		workspaces, err := s.workspaceService.ListWorkspaces(r.Context(), requestContextUserID(r.Context()))
		if err != nil {
			s.writeMappedError(w, r, err)
			return
		}
		WriteJSON(w, http.StatusOK, workspaces)
	}
}

func (s Server) handleRenameWorkspace() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req updateWorkspaceRequest
		if err := DecodeJSON(w, r, &req); err != nil {
			WriteError(w, r, http.StatusBadRequest, NewAPIError("invalid_json", "request body must be valid JSON"))
			return
		}

		workspace, err := s.workspaceService.RenameWorkspace(r.Context(), requestContextUserID(r.Context()), application.RenameWorkspaceInput{
			WorkspaceID: chi.URLParam(r, "workspaceID"),
			Name:        req.Name,
		})
		if err != nil {
			s.writeMappedError(w, r, err)
			return
		}

		WriteJSON(w, http.StatusOK, workspace)
	}
}

func (s Server) handleInviteMember() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req inviteMemberRequest
		if err := DecodeJSON(w, r, &req); err != nil {
			WriteError(w, r, http.StatusBadRequest, NewAPIError("invalid_json", "request body must be valid JSON"))
			return
		}

		invitation, err := s.workspaceService.InviteMember(r.Context(), requestContextUserID(r.Context()), application.InviteMemberInput{
			WorkspaceID: chi.URLParam(r, "workspaceID"),
			Email:       req.Email,
			Role:        domain.WorkspaceRole(req.Role),
		})
		if err != nil {
			s.writeMappedError(w, r, err)
			return
		}

		WriteJSON(w, http.StatusCreated, invitation)
	}
}

func (s Server) handleListWorkspaceInvitations() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		status := domain.WorkspaceInvitationStatusFilter(strings.TrimSpace(r.URL.Query().Get("status")))
		limit := -1
		if rawLimit := strings.TrimSpace(r.URL.Query().Get("limit")); rawLimit != "" {
			parsed, err := strconv.Atoi(rawLimit)
			if err != nil || parsed <= 0 {
				s.writeMappedError(w, r, fmt.Errorf("%w: invalid limit", domain.ErrValidation))
				return
			}
			limit = parsed
		}

		result, err := s.workspaceService.ListWorkspaceInvitations(r.Context(), requestContextUserID(r.Context()), application.ListWorkspaceInvitationsInput{
			WorkspaceID: chi.URLParam(r, "workspaceID"),
			Status:      status,
			Limit:       limit,
			Cursor:      strings.TrimSpace(r.URL.Query().Get("cursor")),
		})
		if err != nil {
			s.writeMappedError(w, r, err)
			return
		}

		WriteJSON(w, http.StatusOK, result)
	}
}

func (s Server) handleListMyInvitations() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		status := domain.WorkspaceInvitationStatusFilter(strings.TrimSpace(r.URL.Query().Get("status")))
		limit := -1
		if rawLimit := strings.TrimSpace(r.URL.Query().Get("limit")); rawLimit != "" {
			parsed, err := strconv.Atoi(rawLimit)
			if err != nil {
				s.writeMappedError(w, r, fmt.Errorf("%w: invalid limit", domain.ErrValidation))
				return
			}
			limit = parsed
		}

		result, err := s.workspaceService.ListMyInvitations(r.Context(), requestContextUserID(r.Context()), application.ListMyInvitationsInput{
			Status: status,
			Limit:  limit,
			Cursor: strings.TrimSpace(r.URL.Query().Get("cursor")),
		})
		if err != nil {
			s.writeMappedError(w, r, err)
			return
		}

		WriteJSON(w, http.StatusOK, result)
	}
}

func (s Server) handleAcceptInvitation() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req acceptInvitationRequest
		if err := DecodeJSON(w, r, &req); err != nil {
			WriteError(w, r, http.StatusBadRequest, NewAPIError("invalid_json", "request body must be valid JSON"))
			return
		}

		result, err := s.workspaceService.AcceptInvitation(r.Context(), requestContextUserID(r.Context()), application.AcceptInvitationInput{
			InvitationID: chi.URLParam(r, "invitationID"),
			Version:      req.Version,
		})
		if err != nil {
			s.writeMappedError(w, r, err)
			return
		}

		WriteJSON(w, http.StatusOK, result)
	}
}

func (s Server) handleUpdateInvitation() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req updateInvitationRequest
		if err := DecodeJSON(w, r, &req); err != nil {
			WriteError(w, r, http.StatusBadRequest, NewAPIError("invalid_json", "request body must be valid JSON"))
			return
		}

		invitation, err := s.workspaceService.UpdateInvitation(r.Context(), requestContextUserID(r.Context()), application.UpdateInvitationInput{
			InvitationID: chi.URLParam(r, "invitationID"),
			Role:         domain.WorkspaceRole(req.Role),
			Version:      req.Version,
		})
		if err != nil {
			s.writeMappedError(w, r, err)
			return
		}

		WriteJSON(w, http.StatusOK, invitation)
	}
}

func (s Server) handleRejectInvitation() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req rejectInvitationRequest
		if err := DecodeJSON(w, r, &req); err != nil {
			WriteError(w, r, http.StatusBadRequest, NewAPIError("invalid_json", "request body must be valid JSON"))
			return
		}

		invitation, err := s.workspaceService.RejectInvitation(r.Context(), requestContextUserID(r.Context()), application.RejectInvitationInput{
			InvitationID: chi.URLParam(r, "invitationID"),
			Version:      req.Version,
		})
		if err != nil {
			s.writeMappedError(w, r, err)
			return
		}

		WriteJSON(w, http.StatusOK, invitation)
	}
}

func (s Server) handleCancelInvitation() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req cancelInvitationRequest
		if err := DecodeJSON(w, r, &req); err != nil {
			WriteError(w, r, http.StatusBadRequest, NewAPIError("invalid_json", "request body must be valid JSON"))
			return
		}

		invitation, err := s.workspaceService.CancelInvitation(r.Context(), requestContextUserID(r.Context()), application.CancelInvitationInput{
			InvitationID: chi.URLParam(r, "invitationID"),
			Version:      req.Version,
		})
		if err != nil {
			s.writeMappedError(w, r, err)
			return
		}

		WriteJSON(w, http.StatusOK, invitation)
	}
}

func (s Server) handleListMembers() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		members, err := s.workspaceService.ListMembers(r.Context(), requestContextUserID(r.Context()), chi.URLParam(r, "workspaceID"))
		if err != nil {
			s.writeMappedError(w, r, err)
			return
		}

		WriteJSON(w, http.StatusOK, members)
	}
}

func (s Server) handleUpdateMemberRole() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req updateMemberRoleRequest
		if err := DecodeJSON(w, r, &req); err != nil {
			WriteError(w, r, http.StatusBadRequest, NewAPIError("invalid_json", "request body must be valid JSON"))
			return
		}

		member, err := s.workspaceService.UpdateMemberRole(r.Context(), requestContextUserID(r.Context()), application.UpdateMemberRoleInput{
			WorkspaceID: chi.URLParam(r, "workspaceID"),
			MemberID:    chi.URLParam(r, "memberID"),
			Role:        domain.WorkspaceRole(req.Role),
		})
		if err != nil {
			s.writeMappedError(w, r, err)
			return
		}

		WriteJSON(w, http.StatusOK, member)
	}
}

func (s Server) handleCreateFolder() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req createFolderRequest
		if err := DecodeJSON(w, r, &req); err != nil {
			WriteError(w, r, http.StatusBadRequest, NewAPIError("invalid_json", "request body must be valid JSON"))
			return
		}

		if req.ParentID != nil {
			trimmed := strings.TrimSpace(*req.ParentID)
			req.ParentID = &trimmed
			if trimmed == "" {
				req.ParentID = nil
			}
		}

		folder, err := s.folderService.CreateFolder(r.Context(), requestContextUserID(r.Context()), application.CreateFolderInput{
			WorkspaceID: chi.URLParam(r, "workspaceID"),
			Name:        req.Name,
			ParentID:    req.ParentID,
		})
		if err != nil {
			s.writeMappedError(w, r, err)
			return
		}

		WriteJSON(w, http.StatusCreated, folder)
	}
}

func (s Server) handleListFolders() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		folders, err := s.folderService.ListFolders(r.Context(), requestContextUserID(r.Context()), chi.URLParam(r, "workspaceID"))
		if err != nil {
			s.writeMappedError(w, r, err)
			return
		}

		WriteJSON(w, http.StatusOK, folders)
	}
}

func (s Server) handleRenameFolder() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req updateFolderRequest
		if err := DecodeJSON(w, r, &req); err != nil {
			WriteError(w, r, http.StatusBadRequest, NewAPIError("invalid_json", "request body must be valid JSON"))
			return
		}

		folder, err := s.folderService.RenameFolder(r.Context(), requestContextUserID(r.Context()), application.RenameFolderInput{
			FolderID: chi.URLParam(r, "folderID"),
			Name:     req.Name,
		})
		if err != nil {
			s.writeMappedError(w, r, err)
			return
		}

		WriteJSON(w, http.StatusOK, folder)
	}
}

func (s Server) handleCreatePage() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req createPageRequest
		if err := DecodeJSON(w, r, &req); err != nil {
			WriteError(w, r, http.StatusBadRequest, NewAPIError("invalid_json", "request body must be valid JSON"))
			return
		}

		if req.FolderID != nil {
			trimmed := strings.TrimSpace(*req.FolderID)
			req.FolderID = &trimmed
			if trimmed == "" {
				req.FolderID = nil
			}
		}

		page, draft, err := s.pageService.CreatePage(r.Context(), requestContextUserID(r.Context()), application.CreatePageInput{
			WorkspaceID: chi.URLParam(r, "workspaceID"),
			FolderID:    req.FolderID,
			Title:       req.Title,
		})
		if err != nil {
			s.writeMappedError(w, r, err)
			return
		}

		WriteJSON(w, http.StatusCreated, map[string]any{"page": page, "draft": draft})
	}
}

func (s Server) handleListPages() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var folderID *string
		if rawFolderID := strings.TrimSpace(r.URL.Query().Get("folder_id")); rawFolderID != "" {
			folderID = &rawFolderID
		}

		pages, err := s.pageService.ListPages(r.Context(), requestContextUserID(r.Context()), chi.URLParam(r, "workspaceID"), folderID)
		if err != nil {
			s.writeMappedError(w, r, err)
			return
		}

		WriteJSON(w, http.StatusOK, pages)
	}
}

func (s Server) handleGetPage() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		page, draft, err := s.pageService.GetPage(r.Context(), requestContextUserID(r.Context()), chi.URLParam(r, "pageID"))
		if err != nil {
			s.writeMappedError(w, r, err)
			return
		}

		WriteJSON(w, http.StatusOK, map[string]any{"page": page, "draft": draft})
	}
}

func (s Server) handleUpdatePage() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req updatePageRequest
		if err := DecodeJSON(w, r, &req); err != nil {
			WriteError(w, r, http.StatusBadRequest, NewAPIError("invalid_json", "request body must be valid JSON"))
			return
		}

		var folderID *string
		folderSet := req.FolderID != nil
		if folderSet {
			if string(req.FolderID) != "null" {
				if err := json.Unmarshal(req.FolderID, &folderID); err != nil {
					WriteError(w, r, http.StatusBadRequest, NewAPIError("invalid_json", "folder_id must be a string or null"))
					return
				}
				if folderID != nil {
					trimmed := strings.TrimSpace(*folderID)
					folderID = &trimmed
					if trimmed == "" {
						folderID = nil
					}
				}
			}
		}

		page, err := s.pageService.UpdatePage(r.Context(), requestContextUserID(r.Context()), application.UpdatePageInput{
			PageID:    chi.URLParam(r, "pageID"),
			Title:     req.Title,
			FolderID:  folderID,
			FolderSet: folderSet,
		})
		if err != nil {
			s.writeMappedError(w, r, err)
			return
		}

		WriteJSON(w, http.StatusOK, page)
	}
}

func (s Server) handleUpdateDraft() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req updateDraftRequest
		if err := DecodeJSONWithLimit(w, r, &req, documentJSONBodyLimitBytes); err != nil {
			WriteError(w, r, http.StatusBadRequest, NewAPIError("invalid_json", "request body must be valid JSON"))
			return
		}

		draft, err := s.pageService.UpdateDraft(r.Context(), requestContextUserID(r.Context()), application.UpdateDraftInput{
			PageID:  chi.URLParam(r, "pageID"),
			Content: req.Content,
		})
		if err != nil {
			s.writeMappedError(w, r, err)
			return
		}

		WriteJSON(w, http.StatusOK, draft)
	}
}

func (s Server) handleCreateRevision() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req createRevisionRequest
		if err := DecodeJSON(w, r, &req); err != nil {
			WriteError(w, r, http.StatusBadRequest, NewAPIError("invalid_json", "request body must be valid JSON"))
			return
		}

		revision, err := s.revisionService.CreateRevision(r.Context(), requestContextUserID(r.Context()), application.CreateRevisionInput{
			PageID: chi.URLParam(r, "pageID"),
			Label:  req.Label,
			Note:   req.Note,
		})
		if err != nil {
			s.writeMappedError(w, r, err)
			return
		}

		WriteJSON(w, http.StatusCreated, map[string]any{
			"id":         revision.ID,
			"page_id":    revision.PageID,
			"label":      revision.Label,
			"note":       revision.Note,
			"created_by": revision.CreatedBy,
			"created_at": revision.CreatedAt,
		})
	}
}

func (s Server) handleListRevisions() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		revisions, err := s.revisionService.ListRevisions(r.Context(), requestContextUserID(r.Context()), chi.URLParam(r, "pageID"))
		if err != nil {
			s.writeMappedError(w, r, err)
			return
		}

		WriteJSON(w, http.StatusOK, revisions)
	}
}

func (s Server) handleCompareRevisions() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		diff, err := s.revisionService.CompareRevisions(r.Context(), requestContextUserID(r.Context()), application.CompareRevisionsInput{
			PageID:         chi.URLParam(r, "pageID"),
			FromRevisionID: r.URL.Query().Get("from"),
			ToRevisionID:   r.URL.Query().Get("to"),
		})
		if err != nil {
			s.writeMappedError(w, r, err)
			return
		}

		WriteJSON(w, http.StatusOK, diff)
	}
}

func (s Server) handleRestoreRevision() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		result, err := s.revisionService.RestoreRevision(r.Context(), requestContextUserID(r.Context()), application.RestoreRevisionInput{
			PageID:     chi.URLParam(r, "pageID"),
			RevisionID: chi.URLParam(r, "revisionID"),
		})
		if err != nil {
			s.writeMappedError(w, r, err)
			return
		}

		WriteJSON(w, http.StatusOK, map[string]any{
			"draft": result.Draft,
			"revision": map[string]any{
				"id":         result.Revision.ID,
				"page_id":    result.Revision.PageID,
				"label":      result.Revision.Label,
				"note":       result.Revision.Note,
				"created_by": result.Revision.CreatedBy,
				"created_at": result.Revision.CreatedAt,
			},
		})
	}
}
func (s Server) handleCreateComment() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req createCommentRequest
		if err := DecodeJSON(w, r, &req); err != nil {
			WriteError(w, r, http.StatusBadRequest, NewAPIError("invalid_json", "request body must be valid JSON"))
			return
		}

		comment, err := s.commentService.CreateComment(r.Context(), requestContextUserID(r.Context()), application.CreateCommentInput{
			PageID: chi.URLParam(r, "pageID"),
			Body:   req.Body,
		})
		if err != nil {
			s.writeMappedError(w, r, err)
			return
		}

		WriteJSON(w, http.StatusCreated, comment)
	}
}

func (s Server) handleListComments() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		comments, err := s.commentService.ListComments(r.Context(), requestContextUserID(r.Context()), chi.URLParam(r, "pageID"))
		if err != nil {
			s.writeMappedError(w, r, err)
			return
		}

		WriteJSON(w, http.StatusOK, comments)
	}
}

func (s Server) handleResolveComment() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		comment, err := s.commentService.ResolveComment(r.Context(), requestContextUserID(r.Context()), application.ResolveCommentInput{
			CommentID: chi.URLParam(r, "commentID"),
		})
		if err != nil {
			s.writeMappedError(w, r, err)
			return
		}

		WriteJSON(w, http.StatusOK, comment)
	}
}

func (s Server) handleCreateThread() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req createThreadRequest
		if err := DecodeJSON(w, r, &req); err != nil {
			WriteError(w, r, http.StatusBadRequest, NewAPIError("invalid_json", "request body must be valid JSON"))
			return
		}

		if req.Anchor.QuotedText != nil {
			trimmed := strings.TrimSpace(*req.Anchor.QuotedText)
			req.Anchor.QuotedText = &trimmed
			if trimmed == "" {
				req.Anchor.QuotedText = nil
			}
		}

		detail, err := s.threadService.CreateThread(r.Context(), requestContextUserID(r.Context()), application.CreateThreadInput{
			PageID:   chi.URLParam(r, "pageID"),
			Body:     req.Body,
			Mentions: req.Mentions,
			Anchor: application.CreateThreadAnchorInput{
				Type:            domain.PageCommentThreadAnchorType(req.Anchor.Type),
				BlockID:         req.Anchor.BlockID,
				QuotedText:      req.Anchor.QuotedText,
				QuotedBlockText: req.Anchor.QuotedBlockText,
			},
		})
		if err != nil {
			s.writeMappedError(w, r, err)
			return
		}

		WriteJSON(w, http.StatusCreated, detail)
	}
}

func (s Server) handleGetThread() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		detail, err := s.threadService.GetThread(r.Context(), requestContextUserID(r.Context()), application.GetThreadInput{
			ThreadID: chi.URLParam(r, "threadID"),
		})
		if err != nil {
			s.writeMappedError(w, r, err)
			return
		}

		WriteJSON(w, http.StatusOK, detail)
	}
}

func (s Server) handleGetThreadNotificationPreference() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		preference, err := s.threadService.GetNotificationPreference(r.Context(), requestContextUserID(r.Context()), application.GetThreadNotificationPreferenceInput{
			ThreadID: chi.URLParam(r, "threadID"),
		})
		if err != nil {
			s.writeMappedError(w, r, err)
			return
		}

		WriteJSON(w, http.StatusOK, preference)
	}
}

func (s Server) handleUpdateThreadNotificationPreference() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req updateThreadNotificationPreferenceRequest
		if err := DecodeJSON(w, r, &req); err != nil {
			WriteError(w, r, http.StatusBadRequest, NewAPIError("invalid_json", "request body must be valid JSON"))
			return
		}

		preference, err := s.threadService.UpdateNotificationPreference(r.Context(), requestContextUserID(r.Context()), application.UpdateThreadNotificationPreferenceInput{
			ThreadID: chi.URLParam(r, "threadID"),
			Mode:     req.Mode,
		})
		if err != nil {
			s.writeMappedError(w, r, err)
			return
		}

		WriteJSON(w, http.StatusOK, preference)
	}
}

func (s Server) handleListThreads() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		threadFilters, ok := s.parseThreadListFilters(w, r)
		if !ok {
			return
		}

		threads, err := s.threadService.ListThreads(r.Context(), requestContextUserID(r.Context()), application.ListThreadsInput{
			PageID:            chi.URLParam(r, "pageID"),
			ThreadState:       threadFilters.threadState,
			AnchorState:       threadFilters.anchorState,
			CreatedByMe:       threadFilters.createdByMe,
			HasMissingAnchor:  threadFilters.hasMissingAnchor,
			HasOutdatedAnchor: threadFilters.hasOutdatedAnchor,
			Sort:              threadFilters.sortMode,
			Query:             r.URL.Query().Get("q"),
			Limit:             threadFilters.limit,
			Cursor:            threadFilters.cursor,
		})
		if err != nil {
			s.writeMappedError(w, r, err)
			return
		}

		WriteJSON(w, http.StatusOK, threads)
	}
}

func (s Server) handleListWorkspaceThreads() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		threadFilters, ok := s.parseThreadListFilters(w, r)
		if !ok {
			return
		}

		threads, err := s.threadService.ListWorkspaceThreads(r.Context(), requestContextUserID(r.Context()), application.ListWorkspaceThreadsInput{
			WorkspaceID:       chi.URLParam(r, "workspaceID"),
			ThreadState:       threadFilters.threadState,
			AnchorState:       threadFilters.anchorState,
			CreatedByMe:       threadFilters.createdByMe,
			HasMissingAnchor:  threadFilters.hasMissingAnchor,
			HasOutdatedAnchor: threadFilters.hasOutdatedAnchor,
			Sort:              threadFilters.sortMode,
			Query:             r.URL.Query().Get("q"),
			Limit:             threadFilters.limit,
			Cursor:            threadFilters.cursor,
		})
		if err != nil {
			s.writeMappedError(w, r, err)
			return
		}

		WriteJSON(w, http.StatusOK, threads)
	}
}

type threadListFilters struct {
	threadState       *domain.PageCommentThreadState
	anchorState       *domain.PageCommentThreadAnchorState
	createdByMe       bool
	hasMissingAnchor  *bool
	hasOutdatedAnchor *bool
	sortMode          string
	limit             int
	cursor            string
}

func (s Server) parseThreadListFilters(w http.ResponseWriter, r *http.Request) (threadListFilters, bool) {
	var filters threadListFilters

	if rawThreadState := strings.TrimSpace(r.URL.Query().Get("thread_state")); rawThreadState != "" && rawThreadState != "all" {
		value := domain.PageCommentThreadState(rawThreadState)
		filters.threadState = &value
	}

	if rawAnchorState := strings.TrimSpace(r.URL.Query().Get("anchor_state")); rawAnchorState != "" && rawAnchorState != "all" {
		value := domain.PageCommentThreadAnchorState(rawAnchorState)
		filters.anchorState = &value
	}

	if rawCreatedBy := strings.TrimSpace(r.URL.Query().Get("created_by")); rawCreatedBy != "" {
		if rawCreatedBy != "me" {
			s.writeMappedError(w, r, fmt.Errorf("%w: invalid created_by", domain.ErrValidation))
			return threadListFilters{}, false
		}
		filters.createdByMe = true
	}

	if rawHasMissingAnchor := strings.TrimSpace(r.URL.Query().Get("has_missing_anchor")); rawHasMissingAnchor != "" {
		switch rawHasMissingAnchor {
		case "true":
			value := true
			filters.hasMissingAnchor = &value
		case "false":
			value := false
			filters.hasMissingAnchor = &value
		default:
			s.writeMappedError(w, r, fmt.Errorf("%w: invalid has_missing_anchor", domain.ErrValidation))
			return threadListFilters{}, false
		}
	}

	if rawHasOutdatedAnchor := strings.TrimSpace(r.URL.Query().Get("has_outdated_anchor")); rawHasOutdatedAnchor != "" {
		switch rawHasOutdatedAnchor {
		case "true":
			value := true
			filters.hasOutdatedAnchor = &value
		case "false":
			value := false
			filters.hasOutdatedAnchor = &value
		default:
			s.writeMappedError(w, r, fmt.Errorf("%w: invalid has_outdated_anchor", domain.ErrValidation))
			return threadListFilters{}, false
		}
	}

	filters.sortMode = strings.TrimSpace(r.URL.Query().Get("sort"))
	filters.cursor = strings.TrimSpace(r.URL.Query().Get("cursor"))
	if rawLimit := strings.TrimSpace(r.URL.Query().Get("limit")); rawLimit != "" {
		limit, err := strconv.Atoi(rawLimit)
		if err != nil {
			s.writeMappedError(w, r, fmt.Errorf("%w: invalid limit", domain.ErrValidation))
			return threadListFilters{}, false
		}
		if limit <= 0 {
			s.writeMappedError(w, r, fmt.Errorf("%w: invalid limit", domain.ErrValidation))
			return threadListFilters{}, false
		}
		filters.limit = limit
	}
	return filters, true
}

func (s Server) handleCreateThreadReply() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req createThreadReplyRequest
		if err := DecodeJSON(w, r, &req); err != nil {
			WriteError(w, r, http.StatusBadRequest, NewAPIError("invalid_json", "request body must be valid JSON"))
			return
		}

		detail, err := s.threadService.CreateReply(r.Context(), requestContextUserID(r.Context()), application.CreateThreadReplyInput{
			ThreadID: chi.URLParam(r, "threadID"),
			Body:     req.Body,
			Mentions: req.Mentions,
		})
		if err != nil {
			s.writeMappedError(w, r, err)
			return
		}

		WriteJSON(w, http.StatusCreated, detail)
	}
}

func (s Server) handleResolveThread() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req resolveThreadRequest
		if r.Body != nil && r.Body != http.NoBody {
			if err := DecodeJSON(w, r, &req); err != nil {
				WriteError(w, r, http.StatusBadRequest, NewAPIError("invalid_json", "request body must be valid JSON"))
				return
			}
		}

		detail, err := s.threadService.ResolveThread(r.Context(), requestContextUserID(r.Context()), application.ResolveThreadInput{
			ThreadID:    chi.URLParam(r, "threadID"),
			ResolveNote: req.ResolveNote,
		})
		if err != nil {
			s.writeMappedError(w, r, err)
			return
		}

		WriteJSON(w, http.StatusOK, detail)
	}
}

func (s Server) handleReopenThread() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req reopenThreadRequest
		if r.Body != nil && r.Body != http.NoBody {
			if err := DecodeJSON(w, r, &req); err != nil {
				WriteError(w, r, http.StatusBadRequest, NewAPIError("invalid_json", "request body must be valid JSON"))
				return
			}
		}

		detail, err := s.threadService.ReopenThread(r.Context(), requestContextUserID(r.Context()), application.ReopenThreadInput{
			ThreadID:     chi.URLParam(r, "threadID"),
			ReopenReason: req.ReopenReason,
		})
		if err != nil {
			s.writeMappedError(w, r, err)
			return
		}

		WriteJSON(w, http.StatusOK, detail)
	}
}

func (s Server) handleSearchPages() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		results, err := s.searchService.SearchPages(r.Context(), requestContextUserID(r.Context()), application.SearchInput{
			WorkspaceID: chi.URLParam(r, "workspaceID"),
			Query:       r.URL.Query().Get("q"),
		})
		if err != nil {
			s.writeMappedError(w, r, err)
			return
		}

		WriteJSON(w, http.StatusOK, results)
	}
}

func (s Server) handleDeletePage() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		err := s.pageService.DeletePage(r.Context(), requestContextUserID(r.Context()), application.DeletePageInput{PageID: chi.URLParam(r, "pageID")})
		if err != nil {
			s.writeMappedError(w, r, err)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

func (s Server) handleListTrash() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		items, err := s.pageService.ListTrash(r.Context(), requestContextUserID(r.Context()), chi.URLParam(r, "workspaceID"))
		if err != nil {
			s.writeMappedError(w, r, err)
			return
		}

		WriteJSON(w, http.StatusOK, items)
	}
}

func (s Server) handleGetTrashPage() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		trashItem, page, draft, err := s.pageService.GetTrashPage(r.Context(), requestContextUserID(r.Context()), application.GetTrashPageInput{
			TrashItemID: chi.URLParam(r, "trashItemID"),
		})
		if err != nil {
			s.writeMappedError(w, r, err)
			return
		}

		WriteJSON(w, http.StatusOK, map[string]any{"trash_item": trashItem, "page": page, "draft": draft})
	}
}

func (s Server) handleRestoreTrashItem() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		page, err := s.pageService.RestoreTrashItem(r.Context(), requestContextUserID(r.Context()), application.RestoreTrashItemInput{TrashItemID: chi.URLParam(r, "trashItemID")})
		if err != nil {
			s.writeMappedError(w, r, err)
			return
		}

		WriteJSON(w, http.StatusOK, page)
	}
}

func (s Server) handleListNotifications() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit := 0
		if rawLimit := strings.TrimSpace(r.URL.Query().Get("limit")); rawLimit != "" {
			parsed, err := strconv.Atoi(rawLimit)
			if err != nil || parsed <= 0 {
				s.writeMappedError(w, r, fmt.Errorf("%w: invalid limit", domain.ErrValidation))
				return
			}
			limit = parsed
		}

		notifications, err := s.notificationService.ListNotifications(r.Context(), requestContextUserID(r.Context()), application.ListNotificationsInput{
			Status: strings.TrimSpace(r.URL.Query().Get("status")),
			Type:   strings.TrimSpace(r.URL.Query().Get("type")),
			Limit:  limit,
			Cursor: strings.TrimSpace(r.URL.Query().Get("cursor")),
		})
		if err != nil {
			s.writeMappedError(w, r, err)
			return
		}

		WriteJSON(w, http.StatusOK, notifications)
	}
}

func (s Server) handleMarkNotificationRead() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		notification, err := s.notificationService.MarkNotificationRead(r.Context(), requestContextUserID(r.Context()), chi.URLParam(r, "notificationID"))
		if err != nil {
			s.writeMappedError(w, r, err)
			return
		}

		WriteJSON(w, http.StatusOK, notification)
	}
}

func (s Server) handleBatchMarkNotificationsRead() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req batchMarkNotificationsReadRequest
		if err := DecodeJSON(w, r, &req); err != nil {
			WriteError(w, r, http.StatusBadRequest, NewAPIError("invalid_json", "request body must be valid JSON"))
			return
		}

		result, err := s.notificationService.MarkNotificationsRead(r.Context(), requestContextUserID(r.Context()), domain.BatchMarkNotificationsReadInput{
			NotificationIDs: req.NotificationIDs,
		})
		if err != nil {
			s.writeMappedError(w, r, err)
			return
		}

		WriteJSON(w, http.StatusOK, result)
	}
}

func (s Server) handleGetNotificationUnreadCount() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		unreadCount, err := s.notificationService.GetUnreadCount(r.Context(), requestContextUserID(r.Context()))
		if err != nil {
			s.writeMappedError(w, r, err)
			return
		}

		WriteJSON(w, http.StatusOK, unreadCount)
	}
}

func (s Server) handleNotificationsStream() http.HandlerFunc {
	type snapshotPayload struct {
		UnreadCount int64     `json:"unread_count"`
		SentAt      time.Time `json:"sent_at"`
	}
	type unreadCountPayload struct {
		UnreadCount int64     `json:"unread_count"`
		SentAt      time.Time `json:"sent_at"`
	}
	type invalidatedPayload struct {
		Reason string    `json:"reason"`
		SentAt time.Time `json:"sent_at"`
	}

	return func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			WriteError(w, r, http.StatusInternalServerError, NewAPIError("internal_error", "internal server error"))
			return
		}
		if s.notificationStreamService == nil {
			WriteError(w, r, http.StatusInternalServerError, NewAPIError("internal_error", "internal server error"))
			return
		}

		session, err := s.notificationStreamService.Open(r.Context(), requestContextUserID(r.Context()))
		if err != nil {
			s.writeMappedError(w, r, err)
			return
		}
		defer session.Close()

		setSSEHeaders(w)
		w.WriteHeader(http.StatusOK)

		if err := writeSSEEvent(w, "snapshot", snapshotPayload{
			UnreadCount: session.InitialUnreadCount(),
			SentAt:      time.Now().UTC(),
		}); err != nil {
			return
		}
		flusher.Flush()

		heartbeat := time.NewTicker(notificationStreamHeartbeatInterval)
		defer heartbeat.Stop()

		for {
			select {
			case <-r.Context().Done():
				return
			case event, ok := <-session.Events():
				if !ok {
					return
				}

				switch event.Type {
				case "unread_count":
					if event.UnreadCount == nil {
						return
					}
					if err := writeSSEEvent(w, "unread_count", unreadCountPayload{
						UnreadCount: *event.UnreadCount,
						SentAt:      event.SentAt,
					}); err != nil {
						return
					}
				case "inbox_invalidated":
					if err := writeSSEEvent(w, "inbox_invalidated", invalidatedPayload{
						Reason: event.Reason,
						SentAt: event.SentAt,
					}); err != nil {
						return
					}
				default:
					return
				}
				flusher.Flush()
			case <-heartbeat.C:
				if err := writeSSEComment(w, "keep-alive"); err != nil {
					return
				}
				flusher.Flush()
			}
		}
	}
}
