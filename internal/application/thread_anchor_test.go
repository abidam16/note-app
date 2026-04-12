package application

import (
	"encoding/json"
	"testing"

	"note-app/internal/domain"
)

func TestEvaluateThreadAnchorState(t *testing.T) {
	blockID := "block-1"
	thread := domain.PageCommentThread{
		ID:     "thread-1",
		PageID: "page-1",
		Anchor: domain.PageCommentThreadAnchor{
			Type:            domain.PageCommentThreadAnchorTypeBlock,
			BlockID:         &blockID,
			QuotedBlockText: "hello world",
		},
	}

	state, err := evaluateThreadAnchorState(json.RawMessage(`[{"id":"block-1","type":"paragraph","children":[{"type":"text","text":"hello world"}]}]`), thread)
	if err != nil {
		t.Fatalf("evaluateThreadAnchorState(active) error = %v", err)
	}
	if state != domain.PageCommentThreadAnchorStateActive {
		t.Fatalf("expected active, got %s", state)
	}

	state, err = evaluateThreadAnchorState(json.RawMessage(`[{"id":"block-1","type":"paragraph","children":[{"type":"text","text":"hello brave world"}]}]`), thread)
	if err != nil {
		t.Fatalf("evaluateThreadAnchorState(outdated) error = %v", err)
	}
	if state != domain.PageCommentThreadAnchorStateOutdated {
		t.Fatalf("expected outdated, got %s", state)
	}

	state, err = evaluateThreadAnchorState(json.RawMessage(`[{"id":"block-2","type":"paragraph","children":[{"type":"text","text":"different text"}]}]`), thread)
	if err != nil {
		t.Fatalf("evaluateThreadAnchorState(missing) error = %v", err)
	}
	if state != domain.PageCommentThreadAnchorStateMissing {
		t.Fatalf("expected missing, got %s", state)
	}
}

func TestEvaluateThreadAnchorStateHandlesMissingAnchorAndInvalidDocument(t *testing.T) {
	thread := domain.PageCommentThread{
		ID:     "thread-1",
		PageID: "page-1",
		Anchor: domain.PageCommentThreadAnchor{
			Type:            domain.PageCommentThreadAnchorTypeBlock,
			QuotedBlockText: "hello world",
		},
	}

	state, err := evaluateThreadAnchorState(json.RawMessage(`[{"id":"block-1","type":"paragraph","children":[{"type":"text","text":"hello world"}]}]`), thread)
	if err != nil {
		t.Fatalf("evaluateThreadAnchorState(missing anchor) error = %v", err)
	}
	if state != domain.PageCommentThreadAnchorStateMissing {
		t.Fatalf("expected missing for empty block id, got %s", state)
	}

	blockID := "block-1"
	thread.Anchor.BlockID = &blockID
	if _, err := evaluateThreadAnchorState(json.RawMessage(`{"invalid":true}`), thread); err == nil {
		t.Fatal("expected invalid document error")
	}
}

func TestEvaluateThreadAnchorReanchorsWhenQuotedBlockTextMatchesUniquely(t *testing.T) {
	blockID := "block-1"
	thread := domain.PageCommentThread{
		ID:     "thread-1",
		PageID: "page-1",
		Anchor: domain.PageCommentThreadAnchor{
			Type:            domain.PageCommentThreadAnchorTypeBlock,
			BlockID:         &blockID,
			QuotedBlockText: "hello world",
		},
	}

	state, recoveredBlockID, err := evaluateThreadAnchor(json.RawMessage(`[{"id":"block-9","type":"paragraph","children":[{"type":"text","text":"hello world"}]}]`), thread)
	if err != nil {
		t.Fatalf("evaluateThreadAnchor(reanchor) error = %v", err)
	}
	if state != domain.PageCommentThreadAnchorStateActive {
		t.Fatalf("expected active after unique reanchor, got %s", state)
	}
	if recoveredBlockID == nil || *recoveredBlockID != "block-9" {
		t.Fatalf("expected recovered block id block-9, got %+v", recoveredBlockID)
	}

	state, recoveredBlockID, err = evaluateThreadAnchor(json.RawMessage(`[
		{"id":"block-8","type":"paragraph","children":[{"type":"text","text":"hello world"}]},
		{"id":"block-9","type":"paragraph","children":[{"type":"text","text":"hello world"}]}
	]`), thread)
	if err != nil {
		t.Fatalf("evaluateThreadAnchor(ambiguous) error = %v", err)
	}
	if state != domain.PageCommentThreadAnchorStateMissing {
		t.Fatalf("expected missing when reanchor is ambiguous, got %s", state)
	}
	if recoveredBlockID != nil {
		t.Fatalf("expected no recovered block id for ambiguous reanchor, got %+v", recoveredBlockID)
	}
}

func TestEvaluateThreadAnchorReanchorsWhenQuotedTextMatchesUniquely(t *testing.T) {
	blockID := "block-1"
	quotedText := "target sentence"
	thread := domain.PageCommentThread{
		ID:     "thread-1",
		PageID: "page-1",
		Anchor: domain.PageCommentThreadAnchor{
			Type:            domain.PageCommentThreadAnchorTypeBlock,
			BlockID:         &blockID,
			QuotedText:      &quotedText,
			QuotedBlockText: "full original block",
		},
	}

	state, recoveredBlockID, err := evaluateThreadAnchor(json.RawMessage(`[
		{"id":"block-9","type":"paragraph","children":[{"type":"text","text":"prefix target sentence suffix"}]}
	]`), thread)
	if err != nil {
		t.Fatalf("evaluateThreadAnchor(quoted text reanchor) error = %v", err)
	}
	if state != domain.PageCommentThreadAnchorStateOutdated {
		t.Fatalf("expected outdated after quoted_text recovery, got %s", state)
	}
	if recoveredBlockID == nil || *recoveredBlockID != "block-9" {
		t.Fatalf("expected recovered block id block-9, got %+v", recoveredBlockID)
	}

	state, recoveredBlockID, err = evaluateThreadAnchor(json.RawMessage(`[
		{"id":"block-8","type":"paragraph","children":[{"type":"text","text":"target sentence one"}]},
		{"id":"block-9","type":"paragraph","children":[{"type":"text","text":"target sentence two"}]}
	]`), thread)
	if err != nil {
		t.Fatalf("evaluateThreadAnchor(quoted text ambiguous) error = %v", err)
	}
	if state != domain.PageCommentThreadAnchorStateMissing {
		t.Fatalf("expected missing when quoted_text recovery is ambiguous, got %s", state)
	}
	if recoveredBlockID != nil {
		t.Fatalf("expected no recovered block id for ambiguous quoted_text recovery, got %+v", recoveredBlockID)
	}
}
