package application

import (
	"encoding/json"
	"testing"
)

func TestRevisionDiffExtractionEdgeCases(t *testing.T) {
	if got := extractListText(map[string]json.RawMessage{}); got != "" {
		t.Fatalf("expected empty list text when items missing, got %q", got)
	}
	if got := extractListText(map[string]json.RawMessage{"items": json.RawMessage(`{"bad":true}`)}); got != "" {
		t.Fatalf("expected empty list text when items invalid, got %q", got)
	}

	if got := extractTableText(map[string]json.RawMessage{}); got != "" {
		t.Fatalf("expected empty table text when rows missing, got %q", got)
	}
	if got := extractTableText(map[string]json.RawMessage{"rows": json.RawMessage(`{"bad":true}`)}); got != "" {
		t.Fatalf("expected empty table text when rows invalid, got %q", got)
	}

	if got := extractTextContainer(map[string]json.RawMessage{"text": json.RawMessage(`"hello"`)}); got != "hello" {
		t.Fatalf("expected text container direct text, got %q", got)
	}
	if got := extractTextContainer(map[string]json.RawMessage{"children": json.RawMessage(`[{"type":"text","text":"a"}]`)}); got != "a" {
		t.Fatalf("expected text container children text, got %q", got)
	}

	if got := extractBlockText(map[string]json.RawMessage{"type": json.RawMessage(`"unknown"`)}); got != "\"unknown\"" {
		t.Fatalf("unexpected unknown block fallback: %q", got)
	}

	matrix := buildLCSMatrix([]string{"a", "b"}, []string{"a", "c", "b"})
	if len(matrix) != 3 || len(matrix[0]) != 4 {
		t.Fatalf("unexpected matrix shape: %d x %d", len(matrix), len(matrix[0]))
	}

	edits := buildMyersEdits([]string{"a", "b"}, []string{"a", "c", "b"})
	if len(edits) != 3 {
		t.Fatalf("unexpected myers edit count: %d", len(edits))
	}
	if edits[0].typ != sequenceEqual || edits[1].typ != sequenceAdd || edits[2].typ != sequenceEqual {
		t.Fatalf("unexpected myers edits: %+v", edits)
	}

	tokens := tokenizeDiffText("fmt.Println(value + 1)")
	if len(tokens) == 0 {
		t.Fatal("expected diff tokenizer to emit tokens")
	}
}
