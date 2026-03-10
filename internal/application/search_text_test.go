package application

import (
	"encoding/json"
	"testing"
)

func TestExtractSearchBody(t *testing.T) {
	content := json.RawMessage(`[
		{"type":"heading","text":"Project"},
		{"type":"paragraph","children":[{"type":"text","text":"hello"},{"type":"text","text":"world"}]},
		{"type":"table","rows":[[{"type":"text","text":"a"},{"type":"text","text":"b"}]]}
	]`)

	body := ExtractSearchBody(content)
	if body == "" {
		t.Fatal("expected non-empty search body")
	}
	if body != "Project hello world a b" {
		t.Fatalf("unexpected search body: %q", body)
	}
}

func TestExtractSearchBodyInvalidJSON(t *testing.T) {
	if got := ExtractSearchBody(json.RawMessage(`not json`)); got != "" {
		t.Fatalf("expected empty search body for invalid json, got %q", got)
	}
}
