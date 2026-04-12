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
	if len(blocks[0].Lines) != 2 {
		t.Fatalf("expected first block to expose removed and added line entries, got %+v", blocks[0].Lines)
	}
	if blocks[0].Lines[0].Operation != "removed" || blocks[0].Lines[1].Operation != "added" {
		t.Fatalf("expected first block line ops to show modified pair, got %+v", blocks[0].Lines)
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

	merged := appendTextChunk(nil, "added", "one")
	merged = appendTextChunk(merged, "added", "two")
	if len(merged) != 1 || merged[0].Text != "onetwo" {
		t.Fatalf("expected chunk merge, got %+v", merged)
	}
}

func TestRevisionDiffLinesStayAlignedAfterDeletion(t *testing.T) {
	blocks, err := buildRevisionDiffBlocks(
		json.RawMessage(`[{"type":"code_block","text":"line 1\nline 2\nline 3\nline 4\nline 5"}]`),
		json.RawMessage(`[{"type":"code_block","text":"line 1\nline 2\nline 4\nline 5"}]`),
	)
	if err != nil {
		t.Fatalf("buildRevisionDiffBlocks error: %v", err)
	}
	if len(blocks) != 1 {
		t.Fatalf("expected one diff block, got %d", len(blocks))
	}
	if blocks[0].Status != "modified" {
		t.Fatalf("expected modified block, got %s", blocks[0].Status)
	}
	if len(blocks[0].Lines) != 5 {
		t.Fatalf("expected five aligned diff lines, got %+v", blocks[0].Lines)
	}

	assertLine := func(idx int, operation string, fromLine, toLine *int, text string) {
		t.Helper()
		line := blocks[0].Lines[idx]
		if line.Operation != operation || line.Text != text {
			t.Fatalf("unexpected line[%d]: %+v", idx, line)
		}
		if !sameOptionalInt(line.FromLineNumber, fromLine) || !sameOptionalInt(line.ToLineNumber, toLine) {
			t.Fatalf("unexpected line numbers at [%d]: %+v", idx, line)
		}
	}

	assertLine(0, "context", intPtr(1), intPtr(1), "line 1")
	assertLine(1, "context", intPtr(2), intPtr(2), "line 2")
	assertLine(2, "removed", intPtr(3), nil, "line 3")
	assertLine(3, "context", intPtr(4), intPtr(3), "line 4")
	assertLine(4, "context", intPtr(5), intPtr(4), "line 5")
}

func TestRevisionDiffIgnoresEditorBlockIDsWhenAligning(t *testing.T) {
	blocks, err := buildRevisionDiffBlocks(
		json.RawMessage(`[
			{"id":"old-1","type":"paragraph","children":[{"type":"text","text":"Empty"}]},
			{"id":"old-2","type":"paragraph","children":[{"type":"text","text":"dkkslsl"}]}
		]`),
		json.RawMessage(`[
			{"id":"new-1","type":"paragraph","children":[{"type":"text","text":"dkkslsl"}]},
			{"id":"new-2","type":"paragraph","children":[{"type":"text","text":"alsdjflaslkdfj"}]}
		]`),
	)
	if err != nil {
		t.Fatalf("buildRevisionDiffBlocks error: %v", err)
	}
	if len(blocks) != 3 {
		t.Fatalf("expected remove, unchanged, add; got %+v", blocks)
	}
	if blocks[0].Status != "removed" || blocks[0].From == nil || blocks[0].From.Text != "Empty" {
		t.Fatalf("expected first block removed old Empty, got %+v", blocks[0])
	}
	if blocks[1].Status != "unchanged" || blocks[1].From == nil || blocks[1].To == nil || blocks[1].From.Text != "dkkslsl" || blocks[1].To.Text != "dkkslsl" {
		t.Fatalf("expected second block unchanged dkkslsl, got %+v", blocks[1])
	}
	if blocks[2].Status != "added" || blocks[2].To == nil || blocks[2].To.Text != "alsdjflaslkdfj" {
		t.Fatalf("expected third block added new line, got %+v", blocks[2])
	}
}

func sameOptionalInt(got, want *int) bool {
	if got == nil || want == nil {
		return got == nil && want == nil
	}
	return *got == *want
}
