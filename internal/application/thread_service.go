package application

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"note-app/internal/domain"

	"github.com/google/uuid"
)

type ThreadPageRepository interface {
	GetByID(ctx context.Context, pageID string) (domain.Page, domain.PageDraft, error)
}

type ThreadRepository interface {
	CreateThread(ctx context.Context, thread domain.PageCommentThread, firstMessage domain.PageCommentThreadMessage, mentions []domain.PageCommentMessageMention, outboxEvent domain.OutboxEvent) (domain.PageCommentThreadDetail, error)
	GetThread(ctx context.Context, threadID string) (domain.PageCommentThreadDetail, error)
	ListThreads(ctx context.Context, pageID string, threadState *domain.PageCommentThreadState, anchorState *domain.PageCommentThreadAnchorState, createdBy *string, hasMissingAnchor *bool, hasOutdatedAnchor *bool, sort string, query string, limit int, cursor string) (domain.PageCommentThreadList, error)
	ListWorkspaceThreads(ctx context.Context, workspaceID string, threadState *domain.PageCommentThreadState, anchorState *domain.PageCommentThreadAnchorState, createdBy *string, hasMissingAnchor *bool, hasOutdatedAnchor *bool, sort string, query string, limit int, cursor string) (domain.WorkspaceCommentThreadList, error)
	AddReply(ctx context.Context, threadID string, message domain.PageCommentThreadMessage, mentions []domain.PageCommentMessageMention, updatedThread domain.PageCommentThread, outboxEvent domain.OutboxEvent) (domain.PageCommentThreadDetail, error)
	UpdateThreadState(ctx context.Context, threadID string, updatedThread domain.PageCommentThread, reevaluation *domain.ThreadAnchorReevaluationContext) (domain.PageCommentThreadDetail, error)
}

type ThreadNotificationPreferenceStore interface {
	GetThreadNotificationPreference(ctx context.Context, threadID, userID string) (*domain.ThreadNotificationPreference, error)
	SetThreadNotificationPreference(ctx context.Context, preference domain.ThreadNotificationPreference) error
}

type CreateThreadAnchorInput struct {
	Type            domain.PageCommentThreadAnchorType
	BlockID         string
	QuotedText      *string
	QuotedBlockText string
}

type CreateThreadInput struct {
	PageID   string
	Body     string
	Mentions []string
	Anchor   CreateThreadAnchorInput
}

type ListThreadsInput struct {
	PageID            string
	ThreadState       *domain.PageCommentThreadState
	AnchorState       *domain.PageCommentThreadAnchorState
	CreatedByMe       bool
	HasMissingAnchor  *bool
	HasOutdatedAnchor *bool
	Sort              string
	Query             string
	Limit             int
	Cursor            string
}

type ListWorkspaceThreadsInput struct {
	WorkspaceID       string
	ThreadState       *domain.PageCommentThreadState
	AnchorState       *domain.PageCommentThreadAnchorState
	CreatedByMe       bool
	HasMissingAnchor  *bool
	HasOutdatedAnchor *bool
	Sort              string
	Query             string
	Limit             int
	Cursor            string
}

type GetThreadInput struct {
	ThreadID string
}

type CreateThreadReplyInput struct {
	ThreadID string
	Body     string
	Mentions []string
}

type ResolveThreadInput struct {
	ThreadID    string
	ResolveNote string
}

type ReopenThreadInput struct {
	ThreadID     string
	ReopenReason string
}

type ThreadService struct {
	threads     ThreadRepository
	pages       ThreadPageRepository
	memberships WorkspaceMembershipReader
	preferences ThreadNotificationPreferenceStore
}

const (
	defaultThreadListLimit = 50
	maxThreadListLimit     = 100
)

func NewThreadService(threads ThreadRepository, pages ThreadPageRepository, memberships WorkspaceMembershipReader, preferences ...ThreadNotificationPreferenceStore) ThreadService {
	var preferenceReader ThreadNotificationPreferenceStore
	if len(preferences) > 0 {
		preferenceReader = preferences[0]
	}
	return ThreadService{
		threads:     threads,
		pages:       pages,
		memberships: memberships,
		preferences: preferenceReader,
	}
}

func (s ThreadService) CreateThread(ctx context.Context, actorID string, input CreateThreadInput) (domain.PageCommentThreadDetail, error) {
	page, draft, err := loadVisiblePageForActor(ctx, s.pages, s.memberships, actorID, input.PageID)
	if err != nil {
		return domain.PageCommentThreadDetail{}, err
	}

	body := strings.TrimSpace(input.Body)
	if body == "" {
		return domain.PageCommentThreadDetail{}, fmt.Errorf("%w: thread body is required", domain.ErrValidation)
	}

	if input.Anchor.Type != domain.PageCommentThreadAnchorTypeBlock {
		return domain.PageCommentThreadDetail{}, fmt.Errorf("%w: anchor type must be %q", domain.ErrValidation, domain.PageCommentThreadAnchorTypeBlock)
	}

	blockID := strings.TrimSpace(input.Anchor.BlockID)
	if blockID == "" {
		return domain.PageCommentThreadDetail{}, fmt.Errorf("%w: anchor.block_id is required", domain.ErrValidation)
	}

	blockText, err := findDraftBlockTextByID(draft.Content, blockID)
	if err != nil {
		return domain.PageCommentThreadDetail{}, err
	}

	var quotedText *string
	if input.Anchor.QuotedText != nil {
		trimmedQuotedText := strings.TrimSpace(*input.Anchor.QuotedText)
		if trimmedQuotedText != "" {
			if !strings.Contains(blockText, trimmedQuotedText) {
				return domain.PageCommentThreadDetail{}, fmt.Errorf("%w: anchor.quoted_text must match the anchored block", domain.ErrValidation)
			}
			quotedText = &trimmedQuotedText
		}
	}

	if trimmedQuotedBlockText := strings.TrimSpace(input.Anchor.QuotedBlockText); trimmedQuotedBlockText != "" && trimmedQuotedBlockText != blockText {
		return domain.PageCommentThreadDetail{}, fmt.Errorf("%w: anchor.quoted_block_text must match the anchored block", domain.ErrValidation)
	}

	now := time.Now().UTC()
	thread := domain.PageCommentThread{
		ID:     uuid.NewString(),
		PageID: page.ID,
		Anchor: domain.PageCommentThreadAnchor{
			Type:            domain.PageCommentThreadAnchorTypeBlock,
			BlockID:         &blockID,
			QuotedText:      quotedText,
			QuotedBlockText: blockText,
		},
		ThreadState:    domain.PageCommentThreadStateOpen,
		AnchorState:    domain.PageCommentThreadAnchorStateActive,
		CreatedBy:      actorID,
		CreatedAt:      now,
		LastActivityAt: now,
		ReplyCount:     1,
	}
	firstMessage := domain.PageCommentThreadMessage{
		ID:        uuid.NewString(),
		ThreadID:  thread.ID,
		Body:      body,
		CreatedBy: actorID,
		CreatedAt: now,
	}

	mentionUserIDs, err := normalizeThreadMentionUserIDs(input.Mentions)
	if err != nil {
		return domain.PageCommentThreadDetail{}, err
	}
	if err := validateThreadMentionUserIDs(ctx, s.memberships, page.WorkspaceID, mentionUserIDs); err != nil {
		return domain.PageCommentThreadDetail{}, err
	}

	mentionRows := buildThreadMessageMentions(firstMessage.ID, mentionUserIDs)

	outboxEvent, err := domain.NewThreadCreatedOutboxEvent(thread, firstMessage, page.WorkspaceID, mentionUserIDs)
	if err != nil {
		return domain.PageCommentThreadDetail{}, err
	}

	detail, err := s.threads.CreateThread(ctx, thread, firstMessage, mentionRows, outboxEvent)
	if err != nil {
		return domain.PageCommentThreadDetail{}, err
	}

	return detail, nil
}

func (s ThreadService) GetThread(ctx context.Context, actorID string, input GetThreadInput) (domain.PageCommentThreadDetail, error) {
	detail, err := s.threads.GetThread(ctx, input.ThreadID)
	if err != nil {
		return domain.PageCommentThreadDetail{}, err
	}

	if _, _, err := loadVisiblePageForActor(ctx, s.pages, s.memberships, actorID, detail.Thread.PageID); err != nil {
		return domain.PageCommentThreadDetail{}, err
	}

	return detail, nil
}

func (s ThreadService) ListThreads(ctx context.Context, actorID string, input ListThreadsInput) (domain.PageCommentThreadList, error) {
	page, _, err := loadVisiblePageForActor(ctx, s.pages, s.memberships, actorID, input.PageID)
	if err != nil {
		return domain.PageCommentThreadList{}, err
	}

	createdBy, err := s.validateThreadListActorAndFilters(ctx, actorID, page.WorkspaceID, input.ThreadState, input.AnchorState, input.Sort, input.CreatedByMe)
	if err != nil {
		return domain.PageCommentThreadList{}, hideForeignResourceMembershipError(err)
	}

	limit, err := normalizeThreadListLimit(input.Limit)
	if err != nil {
		return domain.PageCommentThreadList{}, err
	}

	return s.threads.ListThreads(ctx, page.ID, input.ThreadState, input.AnchorState, createdBy, input.HasMissingAnchor, input.HasOutdatedAnchor, input.Sort, strings.TrimSpace(input.Query), limit, strings.TrimSpace(input.Cursor))
}

func (s ThreadService) ListWorkspaceThreads(ctx context.Context, actorID string, input ListWorkspaceThreadsInput) (domain.WorkspaceCommentThreadList, error) {
	createdBy, err := s.validateThreadListActorAndFilters(ctx, actorID, input.WorkspaceID, input.ThreadState, input.AnchorState, input.Sort, input.CreatedByMe)
	if err != nil {
		return domain.WorkspaceCommentThreadList{}, err
	}

	limit, err := normalizeThreadListLimit(input.Limit)
	if err != nil {
		return domain.WorkspaceCommentThreadList{}, err
	}

	return s.threads.ListWorkspaceThreads(ctx, input.WorkspaceID, input.ThreadState, input.AnchorState, createdBy, input.HasMissingAnchor, input.HasOutdatedAnchor, input.Sort, strings.TrimSpace(input.Query), limit, strings.TrimSpace(input.Cursor))
}

func (s ThreadService) CreateReply(ctx context.Context, actorID string, input CreateThreadReplyInput) (domain.PageCommentThreadDetail, error) {
	detail, err := s.threads.GetThread(ctx, input.ThreadID)
	if err != nil {
		return domain.PageCommentThreadDetail{}, err
	}

	page, _, err := loadVisiblePageForActor(ctx, s.pages, s.memberships, actorID, detail.Thread.PageID)
	if err != nil {
		return domain.PageCommentThreadDetail{}, err
	}

	body := strings.TrimSpace(input.Body)
	if body == "" {
		return domain.PageCommentThreadDetail{}, fmt.Errorf("%w: reply body is required", domain.ErrValidation)
	}

	mentionUserIDs, err := normalizeThreadMentionUserIDs(input.Mentions)
	if err != nil {
		return domain.PageCommentThreadDetail{}, err
	}
	if err := validateThreadMentionUserIDs(ctx, s.memberships, page.WorkspaceID, mentionUserIDs); err != nil {
		return domain.PageCommentThreadDetail{}, err
	}

	now := time.Now().UTC()
	message := domain.PageCommentThreadMessage{
		ID:        uuid.NewString(),
		ThreadID:  detail.Thread.ID,
		Body:      body,
		CreatedBy: actorID,
		CreatedAt: now,
	}

	mentionRows := buildThreadMessageMentions(message.ID, mentionUserIDs)

	replyOutboxEvent, err := domain.NewThreadReplyCreatedOutboxEvent(detail.Thread, message, page.WorkspaceID, mentionUserIDs)
	if err != nil {
		return domain.PageCommentThreadDetail{}, err
	}

	updatedThread := detail.Thread
	updatedThread.LastActivityAt = now
	updatedThread.ReplyCount = detail.Thread.ReplyCount + 1
	if updatedThread.ThreadState == domain.PageCommentThreadStateResolved {
		updatedThread.ThreadState = domain.PageCommentThreadStateOpen
		updatedThread.ReopenedBy = &actorID
		updatedThread.ReopenedAt = &now
		updatedThread.ResolvedBy = nil
		updatedThread.ResolvedAt = nil
		updatedThread.ResolveNote = nil
		updatedThread.ReopenReason = nil
	}

	updatedDetail, err := s.threads.AddReply(ctx, detail.Thread.ID, message, mentionRows, updatedThread, replyOutboxEvent)
	if err != nil {
		return domain.PageCommentThreadDetail{}, err
	}

	return updatedDetail, nil
}

func (s ThreadService) ResolveThread(ctx context.Context, actorID string, input ResolveThreadInput) (domain.PageCommentThreadDetail, error) {
	detail, membership, err := s.threadDetailWithMembership(ctx, actorID, input.ThreadID)
	if err != nil {
		return domain.PageCommentThreadDetail{}, err
	}
	if membership.Role == domain.RoleViewer {
		return domain.PageCommentThreadDetail{}, domain.ErrForbidden
	}
	if detail.Thread.ThreadState == domain.PageCommentThreadStateResolved {
		return detail, nil
	}

	now := time.Now().UTC()
	var resolveNote *string
	if trimmed := strings.TrimSpace(input.ResolveNote); trimmed != "" {
		resolveNote = &trimmed
	}
	updatedThread := detail.Thread
	updatedThread.ThreadState = domain.PageCommentThreadStateResolved
	updatedThread.ResolvedBy = &actorID
	updatedThread.ResolvedAt = &now
	updatedThread.ResolveNote = resolveNote
	updatedThread.ReopenReason = nil
	updatedThread.LastActivityAt = now

	return s.threads.UpdateThreadState(ctx, detail.Thread.ID, updatedThread, nil)
}

func (s ThreadService) ReopenThread(ctx context.Context, actorID string, input ReopenThreadInput) (domain.PageCommentThreadDetail, error) {
	detail, _, err := s.threadDetailWithMembership(ctx, actorID, input.ThreadID)
	if err != nil {
		return domain.PageCommentThreadDetail{}, err
	}
	if detail.Thread.ThreadState == domain.PageCommentThreadStateOpen {
		return detail, nil
	}

	now := time.Now().UTC()
	var reopenReason *string
	if trimmed := strings.TrimSpace(input.ReopenReason); trimmed != "" {
		reopenReason = &trimmed
	}
	updatedThread := detail.Thread
	updatedThread.ThreadState = domain.PageCommentThreadStateOpen
	updatedThread.ResolvedBy = nil
	updatedThread.ResolvedAt = nil
	updatedThread.ResolveNote = nil
	updatedThread.ReopenedBy = &actorID
	updatedThread.ReopenedAt = &now
	updatedThread.ReopenReason = reopenReason
	updatedThread.LastActivityAt = now

	return s.threads.UpdateThreadState(ctx, detail.Thread.ID, updatedThread, nil)
}

func (s ThreadService) ReevaluatePageAnchors(ctx context.Context, pageID string, content json.RawMessage, reevaluation domain.ThreadAnchorReevaluationContext) error {
	list, err := s.threads.ListThreads(ctx, pageID, nil, nil, nil, nil, nil, "", "", 0, "")
	if err != nil {
		return err
	}

	for _, thread := range list.Threads {
		nextState, recoveredBlockID, err := evaluateThreadAnchor(content, thread)
		if err != nil {
			return err
		}
		blockIDChanged := false
		if recoveredBlockID != nil {
			if thread.Anchor.BlockID == nil || *thread.Anchor.BlockID != *recoveredBlockID {
				blockIDChanged = true
			}
		}
		if nextState == thread.AnchorState && !blockIDChanged {
			continue
		}

		updatedThread := thread
		updatedThread.AnchorState = nextState
		if blockIDChanged {
			updatedThread.Anchor.BlockID = recoveredBlockID
		}
		if _, err := s.threads.UpdateThreadState(ctx, thread.ID, updatedThread, &reevaluation); err != nil {
			return err
		}
	}

	return nil
}

func (s ThreadService) threadDetailWithMembership(ctx context.Context, actorID, threadID string) (domain.PageCommentThreadDetail, domain.WorkspaceMember, error) {
	detail, err := s.threads.GetThread(ctx, threadID)
	if err != nil {
		return domain.PageCommentThreadDetail{}, domain.WorkspaceMember{}, err
	}

	page, _, err := loadVisiblePageForActor(ctx, s.pages, s.memberships, actorID, detail.Thread.PageID)
	if err != nil {
		return domain.PageCommentThreadDetail{}, domain.WorkspaceMember{}, err
	}

	membership, err := s.memberships.GetMembershipByUserID(ctx, page.WorkspaceID, actorID)
	if err != nil {
		return domain.PageCommentThreadDetail{}, domain.WorkspaceMember{}, hideForeignResourceMembershipError(err)
	}

	return detail, membership, nil
}

func (s ThreadService) validateThreadListActorAndFilters(ctx context.Context, actorID, workspaceID string, threadState *domain.PageCommentThreadState, anchorState *domain.PageCommentThreadAnchorState, sort string, createdByMe bool) (*string, error) {
	if _, err := s.memberships.GetMembershipByUserID(ctx, workspaceID, actorID); err != nil {
		return nil, err
	}

	if threadState != nil {
		switch *threadState {
		case domain.PageCommentThreadStateOpen, domain.PageCommentThreadStateResolved:
		default:
			return nil, fmt.Errorf("%w: invalid thread_state", domain.ErrValidation)
		}
	}

	if anchorState != nil {
		switch *anchorState {
		case domain.PageCommentThreadAnchorStateActive, domain.PageCommentThreadAnchorStateOutdated, domain.PageCommentThreadAnchorStateMissing:
		default:
			return nil, fmt.Errorf("%w: invalid anchor_state", domain.ErrValidation)
		}
	}

	if sort != "" {
		switch sort {
		case "recent_activity", "newest", "oldest":
		default:
			return nil, fmt.Errorf("%w: invalid sort", domain.ErrValidation)
		}
	}

	if createdByMe {
		return &actorID, nil
	}

	return nil, nil
}

func normalizeThreadListLimit(limit int) (int, error) {
	switch {
	case limit == 0:
		return defaultThreadListLimit, nil
	case limit < 0:
		return 0, fmt.Errorf("%w: invalid limit", domain.ErrValidation)
	case limit > maxThreadListLimit:
		return 0, fmt.Errorf("%w: invalid limit", domain.ErrValidation)
	default:
		return limit, nil
	}
}

func findDraftBlockTextByID(content json.RawMessage, blockID string) (string, error) {
	blockText, found, err := lookupDraftBlockTextByID(content, blockID)
	if err != nil {
		return "", err
	}
	if !found {
		return "", fmt.Errorf("%w: anchor.block_id does not exist in the current draft", domain.ErrValidation)
	}
	return blockText, nil
}

func evaluateThreadAnchorState(content json.RawMessage, thread domain.PageCommentThread) (domain.PageCommentThreadAnchorState, error) {
	state, _, err := evaluateThreadAnchor(content, thread)
	return state, err
}

func evaluateThreadAnchor(content json.RawMessage, thread domain.PageCommentThread) (domain.PageCommentThreadAnchorState, *string, error) {
	if thread.Anchor.Type != domain.PageCommentThreadAnchorTypeBlock || thread.Anchor.BlockID == nil || strings.TrimSpace(*thread.Anchor.BlockID) == "" {
		return domain.PageCommentThreadAnchorStateMissing, nil, nil
	}

	blockText, found, err := lookupDraftBlockTextByID(content, *thread.Anchor.BlockID)
	if err != nil {
		return "", nil, err
	}
	if !found {
		recoveredBlockID, recovered, err := findUniqueBlockIDByText(content, thread.Anchor.QuotedBlockText)
		if err != nil {
			return "", nil, err
		}
		if recovered {
			return domain.PageCommentThreadAnchorStateActive, &recoveredBlockID, nil
		}
		recoveredBlockID, recovered, err = findUniqueBlockIDByQuotedText(content, thread.Anchor.QuotedText)
		if err != nil {
			return "", nil, err
		}
		if recovered {
			return domain.PageCommentThreadAnchorStateOutdated, &recoveredBlockID, nil
		}
		return domain.PageCommentThreadAnchorStateMissing, nil, nil
	}
	if blockText == thread.Anchor.QuotedBlockText {
		return domain.PageCommentThreadAnchorStateActive, nil, nil
	}
	return domain.PageCommentThreadAnchorStateOutdated, nil, nil
}

func lookupDraftBlockTextByID(content json.RawMessage, blockID string) (string, bool, error) {
	blocks, err := decodeDocumentBlocks(content)
	if err != nil {
		return "", false, err
	}

	for _, rawBlock := range blocks {
		block, err := decodeObject(rawBlock, "block")
		if err != nil {
			continue
		}
		rawID, ok := block["id"]
		if !ok {
			continue
		}
		currentID, err := stringFromRaw(rawID, "block.id")
		if err != nil {
			continue
		}
		if currentID != blockID {
			continue
		}
		return extractBlockText(block), true, nil
	}

	return "", false, nil
}

func findUniqueBlockIDByText(content json.RawMessage, quotedBlockText string) (string, bool, error) {
	if strings.TrimSpace(quotedBlockText) == "" {
		return "", false, nil
	}

	blocks, err := decodeDocumentBlocks(content)
	if err != nil {
		return "", false, err
	}

	matchID := ""
	matchCount := 0
	for _, rawBlock := range blocks {
		block, err := decodeObject(rawBlock, "block")
		if err != nil {
			continue
		}
		rawID, ok := block["id"]
		if !ok {
			continue
		}
		currentID, err := stringFromRaw(rawID, "block.id")
		if err != nil || strings.TrimSpace(currentID) == "" {
			continue
		}
		currentText := extractBlockText(block)
		if currentText != quotedBlockText {
			continue
		}
		matchID = currentID
		matchCount++
		if matchCount > 1 {
			return "", false, nil
		}
	}

	if matchCount == 1 {
		return matchID, true, nil
	}
	return "", false, nil
}

func findUniqueBlockIDByQuotedText(content json.RawMessage, quotedText *string) (string, bool, error) {
	if quotedText == nil || strings.TrimSpace(*quotedText) == "" {
		return "", false, nil
	}

	blocks, err := decodeDocumentBlocks(content)
	if err != nil {
		return "", false, err
	}

	matchID := ""
	matchCount := 0
	target := strings.TrimSpace(*quotedText)
	for _, rawBlock := range blocks {
		block, err := decodeObject(rawBlock, "block")
		if err != nil {
			continue
		}
		rawID, ok := block["id"]
		if !ok {
			continue
		}
		currentID, err := stringFromRaw(rawID, "block.id")
		if err != nil || strings.TrimSpace(currentID) == "" {
			continue
		}
		currentText := extractBlockText(block)
		if !strings.Contains(currentText, target) {
			continue
		}
		matchID = currentID
		matchCount++
		if matchCount > 1 {
			return "", false, nil
		}
	}

	if matchCount == 1 {
		return matchID, true, nil
	}
	return "", false, nil
}
