package application

import (
	"encoding/json"
	"testing"
)

func TestRevisionDiffSwitchCoverage(t *testing.T) {
	paragraph, _ := decodeObject(json.RawMessage(`{"type":"paragraph","text":"hello"}`), "p")
	if got := extractBlockText(paragraph); got != "hello" {
		t.Fatalf("expected paragraph text, got %q", got)
	}

	list, _ := decodeObject(json.RawMessage(`{"type":"bullet_list","items":[{"text":"a"},{"children":[{"type":"text","text":"b"}]}]}`), "l")
	if got := extractBlockText(list); got != "a\nb" {
		t.Fatalf("expected list text, got %q", got)
	}

	code, _ := decodeObject(json.RawMessage(`{"type":"code_block","text":"fmt.Println(1)"}`), "c")
	if got := extractBlockText(code); got != "fmt.Println(1)" {
		t.Fatalf("expected code block text, got %q", got)
	}

	image, _ := decodeObject(json.RawMessage(`{"type":"image","src":"/x.png"}`), "i")
	if got := extractBlockText(image); got != "/x.png" {
		t.Fatalf("expected image src fallback, got %q", got)
	}

	snapshot := blockSnapshot(json.RawMessage(`{"type":"quote","children":[{"type":"text","text":"q"}]}`))
	if snapshot.Type != "quote" || snapshot.Text != "q" {
		t.Fatalf("unexpected snapshot: %+v", snapshot)
	}
}
