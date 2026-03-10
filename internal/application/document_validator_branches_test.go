package application

import (
	"encoding/json"
	"errors"
	"testing"

	"note-app/internal/domain"
)

func TestDocumentValidatorBranchHelpers(t *testing.T) {
	listObj := map[string]json.RawMessage{
		"items": json.RawMessage(`[{"text":"a"}]`),
	}
	if err := validateListItems(listObj, "b", false); err != nil {
		t.Fatalf("expected bullet list validation success, got %v", err)
	}

	taskObj := map[string]json.RawMessage{
		"items": json.RawMessage(`[{"checked":true,"children":[{"type":"text","text":"done"}]}]`),
	}
	if err := validateListItems(taskObj, "t", true); err != nil {
		t.Fatalf("expected task list validation success, got %v", err)
	}

	tableObj := map[string]json.RawMessage{
		"rows": json.RawMessage(`[{"cells":[{"text":"x"},{"children":[{"type":"text","text":"y"}]}]}]`),
	}
	if err := validateTable(tableObj, "table"); err != nil {
		t.Fatalf("expected table validation success, got %v", err)
	}

	imageObj := map[string]json.RawMessage{"src": json.RawMessage(`"https://example.com/x.png"`), "alt": json.RawMessage(`"x"`)}
	if err := validateImage(imageObj, "img"); err != nil {
		t.Fatalf("expected image validation success, got %v", err)
	}

	if err := validateInlineNodes(json.RawMessage(`[{"type":"text","text":"ok","marks":[{"type":"italic"},{"type":"inline_code"},{"type":"link","href":"https://example.com"}]}]`), "inline"); err != nil {
		t.Fatalf("expected inline validation success, got %v", err)
	}

	if err := validateMarks(json.RawMessage(`[{"type":"bold"},{"type":"italic"},{"type":"inline_code"}]`), "marks"); err != nil {
		t.Fatalf("expected marks validation success, got %v", err)
	}

	if err := validateMarks(json.RawMessage(`[{"type":"link","href":"https://example.com/path"}]`), "marks"); err != nil {
		t.Fatalf("expected link mark validation success, got %v", err)
	}

	if _, err := requiredIntInRangeField(map[string]json.RawMessage{"level": json.RawMessage(`"x"`)}, "level", "p", 1, 6); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("expected int parse validation error, got %v", err)
	}
	if _, err := requiredBoolField(map[string]json.RawMessage{}, "checked", "p"); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("expected missing bool validation error, got %v", err)
	}
}
