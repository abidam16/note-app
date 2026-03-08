package http

import (
	"encoding/json"
	"net/http"
	"strings"

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

type inviteMemberRequest struct {
	Email string `json:"email"`
	Role  string `json:"role"`
}

type updateMemberRoleRequest struct {
	Role string `json:"role"`
}

type createFolderRequest struct {
	Name     string  `json:"name"`
	ParentID *string `json:"parent_id"`
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

func (s Server) handleRegister() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req registerRequest
		if err := DecodeJSON(r, &req); err != nil {
			WriteError(w, r, http.StatusBadRequest, NewAPIError("invalid_json", "request body must be valid JSON"))
			return
		}

		user, err := s.authService.Register(r.Context(), application.RegisterInput{
			Email:    req.Email,
			Password: req.Password,
			FullName: req.FullName,
		})
		if err != nil {
			status, apiErr := mapError(err)
			WriteError(w, r, status, apiErr)
			return
		}

		WriteJSON(w, http.StatusCreated, user)
	}
}

func (s Server) handleLogin() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req loginRequest
		if err := DecodeJSON(r, &req); err != nil {
			WriteError(w, r, http.StatusBadRequest, NewAPIError("invalid_json", "request body must be valid JSON"))
			return
		}

		result, err := s.authService.Login(r.Context(), application.LoginInput{Email: req.Email, Password: req.Password})
		if err != nil {
			status, apiErr := mapError(err)
			WriteError(w, r, status, apiErr)
			return
		}

		WriteJSON(w, http.StatusOK, result)
	}
}

func (s Server) handleRefresh() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req refreshRequest
		if err := DecodeJSON(r, &req); err != nil {
			WriteError(w, r, http.StatusBadRequest, NewAPIError("invalid_json", "request body must be valid JSON"))
			return
		}

		result, err := s.authService.Refresh(r.Context(), req.RefreshToken)
		if err != nil {
			status, apiErr := mapError(err)
			WriteError(w, r, status, apiErr)
			return
		}

		WriteJSON(w, http.StatusOK, result)
	}
}

func (s Server) handleLogout() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req refreshRequest
		if err := DecodeJSON(r, &req); err != nil {
			WriteError(w, r, http.StatusBadRequest, NewAPIError("invalid_json", "request body must be valid JSON"))
			return
		}

		if err := s.authService.Logout(r.Context(), req.RefreshToken); err != nil {
			status, apiErr := mapError(err)
			WriteError(w, r, status, apiErr)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

func (s Server) handleCurrentUser() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, err := s.authService.CurrentUser(r.Context(), requestContextUserID(r.Context()))
		if err != nil {
			status, apiErr := mapError(err)
			WriteError(w, r, status, apiErr)
			return
		}

		WriteJSON(w, http.StatusOK, user)
	}
}

func (s Server) handleCreateWorkspace() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req createWorkspaceRequest
		if err := DecodeJSON(r, &req); err != nil {
			WriteError(w, r, http.StatusBadRequest, NewAPIError("invalid_json", "request body must be valid JSON"))
			return
		}

		workspace, member, err := s.workspaceService.CreateWorkspace(r.Context(), requestContextUserID(r.Context()), application.CreateWorkspaceInput{Name: req.Name})
		if err != nil {
			status, apiErr := mapError(err)
			WriteError(w, r, status, apiErr)
			return
		}

		WriteJSON(w, http.StatusCreated, map[string]any{"workspace": workspace, "membership": member})
	}
}

func (s Server) handleInviteMember() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req inviteMemberRequest
		if err := DecodeJSON(r, &req); err != nil {
			WriteError(w, r, http.StatusBadRequest, NewAPIError("invalid_json", "request body must be valid JSON"))
			return
		}

		invitation, err := s.workspaceService.InviteMember(r.Context(), requestContextUserID(r.Context()), application.InviteMemberInput{
			WorkspaceID: chi.URLParam(r, "workspaceID"),
			Email:       req.Email,
			Role:        domain.WorkspaceRole(req.Role),
		})
		if err != nil {
			status, apiErr := mapError(err)
			WriteError(w, r, status, apiErr)
			return
		}

		WriteJSON(w, http.StatusCreated, invitation)
	}
}

func (s Server) handleAcceptInvitation() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		member, err := s.workspaceService.AcceptInvitation(r.Context(), requestContextUserID(r.Context()), chi.URLParam(r, "invitationID"))
		if err != nil {
			status, apiErr := mapError(err)
			WriteError(w, r, status, apiErr)
			return
		}

		WriteJSON(w, http.StatusOK, member)
	}
}

func (s Server) handleListMembers() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		members, err := s.workspaceService.ListMembers(r.Context(), requestContextUserID(r.Context()), chi.URLParam(r, "workspaceID"))
		if err != nil {
			status, apiErr := mapError(err)
			WriteError(w, r, status, apiErr)
			return
		}

		WriteJSON(w, http.StatusOK, members)
	}
}

func (s Server) handleUpdateMemberRole() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req updateMemberRoleRequest
		if err := DecodeJSON(r, &req); err != nil {
			WriteError(w, r, http.StatusBadRequest, NewAPIError("invalid_json", "request body must be valid JSON"))
			return
		}

		member, err := s.workspaceService.UpdateMemberRole(r.Context(), requestContextUserID(r.Context()), application.UpdateMemberRoleInput{
			WorkspaceID: chi.URLParam(r, "workspaceID"),
			MemberID:    chi.URLParam(r, "memberID"),
			Role:        domain.WorkspaceRole(req.Role),
		})
		if err != nil {
			status, apiErr := mapError(err)
			WriteError(w, r, status, apiErr)
			return
		}

		WriteJSON(w, http.StatusOK, member)
	}
}

func (s Server) handleCreateFolder() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req createFolderRequest
		if err := DecodeJSON(r, &req); err != nil {
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
			status, apiErr := mapError(err)
			WriteError(w, r, status, apiErr)
			return
		}

		WriteJSON(w, http.StatusCreated, folder)
	}
}

func (s Server) handleListFolders() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		folders, err := s.folderService.ListFolders(r.Context(), requestContextUserID(r.Context()), chi.URLParam(r, "workspaceID"))
		if err != nil {
			status, apiErr := mapError(err)
			WriteError(w, r, status, apiErr)
			return
		}

		WriteJSON(w, http.StatusOK, folders)
	}
}

func (s Server) handleCreatePage() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req createPageRequest
		if err := DecodeJSON(r, &req); err != nil {
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
			status, apiErr := mapError(err)
			WriteError(w, r, status, apiErr)
			return
		}

		WriteJSON(w, http.StatusCreated, map[string]any{"page": page, "draft": draft})
	}
}

func (s Server) handleGetPage() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		page, draft, err := s.pageService.GetPage(r.Context(), requestContextUserID(r.Context()), chi.URLParam(r, "pageID"))
		if err != nil {
			status, apiErr := mapError(err)
			WriteError(w, r, status, apiErr)
			return
		}

		WriteJSON(w, http.StatusOK, map[string]any{"page": page, "draft": draft})
	}
}

func (s Server) handleUpdatePage() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req updatePageRequest
		if err := DecodeJSON(r, &req); err != nil {
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
			status, apiErr := mapError(err)
			WriteError(w, r, status, apiErr)
			return
		}

		WriteJSON(w, http.StatusOK, page)
	}
}

func (s Server) handleUpdateDraft() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req updateDraftRequest
		if err := DecodeJSON(r, &req); err != nil {
			WriteError(w, r, http.StatusBadRequest, NewAPIError("invalid_json", "request body must be valid JSON"))
			return
		}

		draft, err := s.pageService.UpdateDraft(r.Context(), requestContextUserID(r.Context()), application.UpdateDraftInput{
			PageID:  chi.URLParam(r, "pageID"),
			Content: req.Content,
		})
		if err != nil {
			status, apiErr := mapError(err)
			WriteError(w, r, status, apiErr)
			return
		}

		WriteJSON(w, http.StatusOK, draft)
	}
}

func (s Server) handleCreateRevision() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req createRevisionRequest
		if err := DecodeJSON(r, &req); err != nil {
			WriteError(w, r, http.StatusBadRequest, NewAPIError("invalid_json", "request body must be valid JSON"))
			return
		}

		revision, err := s.revisionService.CreateRevision(r.Context(), requestContextUserID(r.Context()), application.CreateRevisionInput{
			PageID: chi.URLParam(r, "pageID"),
			Label:  req.Label,
			Note:   req.Note,
		})
		if err != nil {
			status, apiErr := mapError(err)
			WriteError(w, r, status, apiErr)
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
			status, apiErr := mapError(err)
			WriteError(w, r, status, apiErr)
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
			status, apiErr := mapError(err)
			WriteError(w, r, status, apiErr)
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
			status, apiErr := mapError(err)
			WriteError(w, r, status, apiErr)
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
		if err := DecodeJSON(r, &req); err != nil {
			WriteError(w, r, http.StatusBadRequest, NewAPIError("invalid_json", "request body must be valid JSON"))
			return
		}

		comment, err := s.commentService.CreateComment(r.Context(), requestContextUserID(r.Context()), application.CreateCommentInput{
			PageID: chi.URLParam(r, "pageID"),
			Body:   req.Body,
		})
		if err != nil {
			status, apiErr := mapError(err)
			WriteError(w, r, status, apiErr)
			return
		}

		WriteJSON(w, http.StatusCreated, comment)
	}
}

func (s Server) handleListComments() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		comments, err := s.commentService.ListComments(r.Context(), requestContextUserID(r.Context()), chi.URLParam(r, "pageID"))
		if err != nil {
			status, apiErr := mapError(err)
			WriteError(w, r, status, apiErr)
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
			status, apiErr := mapError(err)
			WriteError(w, r, status, apiErr)
			return
		}

		WriteJSON(w, http.StatusOK, comment)
	}
}
func (s Server) handleSearchPages() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		results, err := s.searchService.SearchPages(r.Context(), requestContextUserID(r.Context()), application.SearchInput{
			WorkspaceID: chi.URLParam(r, "workspaceID"),
			Query:       r.URL.Query().Get("q"),
		})
		if err != nil {
			status, apiErr := mapError(err)
			WriteError(w, r, status, apiErr)
			return
		}

		WriteJSON(w, http.StatusOK, results)
	}
}

func (s Server) handleDeletePage() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		err := s.pageService.DeletePage(r.Context(), requestContextUserID(r.Context()), application.DeletePageInput{PageID: chi.URLParam(r, "pageID")})
		if err != nil {
			status, apiErr := mapError(err)
			WriteError(w, r, status, apiErr)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

func (s Server) handleListTrash() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		items, err := s.pageService.ListTrash(r.Context(), requestContextUserID(r.Context()), chi.URLParam(r, "workspaceID"))
		if err != nil {
			status, apiErr := mapError(err)
			WriteError(w, r, status, apiErr)
			return
		}

		WriteJSON(w, http.StatusOK, items)
	}
}

func (s Server) handleRestoreTrashItem() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		page, err := s.pageService.RestoreTrashItem(r.Context(), requestContextUserID(r.Context()), application.RestoreTrashItemInput{TrashItemID: chi.URLParam(r, "trashItemID")})
		if err != nil {
			status, apiErr := mapError(err)
			WriteError(w, r, status, apiErr)
			return
		}

		WriteJSON(w, http.StatusOK, page)
	}
}
