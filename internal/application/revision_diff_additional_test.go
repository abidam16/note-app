package application

import (
	"encoding/json"
	"testing"
)

func TestRevisionDiffHelpers(t *testing.T) {
	blocks, err := buildRevisionDiffBlocks(
		json.RawMessage(`[
			{"type":"paragraph","children":[{"type":"text","text":"hello world"}]},
			{"type":"code_block","text":"fmt.Println(1)"},
			{"type":"image","src":"/a.png"}
		]`),
		json.RawMessage(`[
			{"type":"paragraph","children":[{"type":"text","text":"hello brave world"}]},
			{"type":"code_block","text":"fmt.Println(1)"},
			{"type":"table","rows":[{"cells":[{"text":"a"},{"children":[{"type":"text","text":"b"}]}]}]}
		]`),
	)
	if err != nil {
		t.Fatalf("buildRevisionDiffBlocks error: %v", err)
	}
	if len(blocks) != 4 {
		t.Fatalf("expected 4 diff blocks, got %d", len(blocks))
	}
	if blocks[0].Status != "modified" || len(blocks[0].InlineDiff) == 0 {
		t.Fatalf("expected first block modified with inline diff, got %+v", blocks[0])
	}
	if blocks[1].Status != "unchanged" {
		t.Fatalf("expected second block unchanged, got %s", blocks[1].Status)
	}

	hasAdded := false
	hasRemoved := false
	for _, block := range blocks {
		if block.Status == "added" {
			hasAdded = true
		}
		if block.Status == "removed" {
			hasRemoved = true
		}
	}
	if !hasAdded || !hasRemoved {
		t.Fatalf("expected both added and removed blocks, got %+v", blocks)
	}

	if _, err := buildRevisionDiffBlocks(json.RawMessage(`not-json`), json.RawMessage(`[]`)); err == nil {
		t.Fatal("expected invalid from content to fail")
	}
	if _, err := buildRevisionDiffBlocks(json.RawMessage(`[]`), json.RawMessage(`not-json`)); err == nil {
		t.Fatal("expected invalid to content to fail")
	}

	if got := compactJSON(json.RawMessage(`{"a": 1}`)); got != `{"a":1}` {
		t.Fatalf("unexpected compact json: %s", got)
	}
	if got := compactJSON(json.RawMessage(`not-json`)); got != "not-json" {
		t.Fatalf("unexpected fallback compact string: %s", got)
	}

	if typ := blockType(json.RawMessage(`{"type":"paragraph"}`)); typ != "paragraph" {
		t.Fatalf("unexpected block type: %s", typ)
	}
	if typ := blockType(json.RawMessage(`bad`)); typ != "unknown" {
		t.Fatalf("expected unknown for invalid block, got %s", typ)
	}

	snap := blockSnapshot(json.RawMessage(`bad`))
	if snap.Type != "unknown" {
		t.Fatalf("expected unknown snapshot type, got %s", snap.Type)
	}

	obj, _ := decodeObject(json.RawMessage(`{"type":"image","src":"/img.png","alt":"cover"}`), "block")
	if text := extractBlockText(obj); text != "cover" {
		t.Fatalf("expected image alt to be used, got %q", text)
	}
	obj, _ = decodeObject(json.RawMessage(`{"type":"image","src":"/img.png"}`), "block")
	if text := extractBlockText(obj); text != "/img.png" {
		t.Fatalf("expected image src fallback, got %q", text)
	}

	obj, _ = decodeObject(json.RawMessage(`{"type":"table","rows":[{"cells":[{"text":"a"},{"children":[{"type":"text","text":"b"}]}]}]}`), "block")
	if text := extractBlockText(obj); text != "a\tb" {
		t.Fatalf("unexpected table text: %q", text)
	}

	if got := extractInlineNodesText(json.RawMessage(`[{"type":"text","text":"a"},{"type":"text","text":"b"}]`)); got != "ab" {
		t.Fatalf("unexpected inline text: %q", got)
	}
	if got := extractInlineNodesText(json.RawMessage(`not-array`)); got != "" {
		t.Fatalf("expected empty inline text for invalid payload, got %q", got)
	}

	if chunks := diffText("", ""); chunks != nil {
		t.Fatalf("expected nil diff for empty text, got %+v", chunks)
	}
	chunks := diffText("alpha beta", "alpha gamma beta")
	if len(chunks) < 2 {
		t.Fatalf("expected multiple diff chunks, got %+v", chunks)
	}

	merged := appendWordChunk(nil, "added", "one")
	merged = appendWordChunk(merged, "added", "two")
	if len(merged) != 1 || merged[0].Text != "one two" {
		t.Fatalf("expected chunk merge, got %+v", merged)
	}
}
