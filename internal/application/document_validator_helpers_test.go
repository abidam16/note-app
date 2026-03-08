package application

import (
	"encoding/json"
	"errors"
	"testing"

	"note-app/internal/domain"
)

func TestDocumentValidatorHelperFunctions(t *testing.T) {
	if _, err := decodeObject(json.RawMessage(`[]`), "x"); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("expected decodeObject validation error, got %v", err)
	}

	obj := map[string]json.RawMessage{"name": json.RawMessage(`"john"`), "num": json.RawMessage(`1`), "flag": json.RawMessage(`true`)}
	if v, err := requiredStringField(obj, "name", "x"); err != nil || v != "john" {
		t.Fatalf("requiredStringField unexpected result v=%q err=%v", v, err)
	}
	if _, err := requiredStringField(obj, "missing", "x"); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("expected requiredStringField missing error, got %v", err)
	}

	if _, ok, err := optionalStringField(obj, "name", "x"); err != nil || !ok {
		t.Fatalf("expected optional string success, err=%v ok=%t", err, ok)
	}
	if _, ok, err := optionalStringField(obj, "missing", "x"); err != nil || ok {
		t.Fatalf("expected optional missing success, err=%v ok=%t", err, ok)
	}

	if _, err := stringFromRaw(json.RawMessage(`123`), "x"); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("expected stringFromRaw validation error, got %v", err)
	}

	if _, err := requiredIntInRangeField(obj, "num", "x", 1, 2); err != nil {
		t.Fatalf("expected int in range success, got %v", err)
	}
	if _, err := requiredIntInRangeField(obj, "num", "x", 2, 3); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("expected int range error, got %v", err)
	}

	if _, err := optionalIntMinField(obj, "num", "x", 1); err != nil {
		t.Fatalf("expected optional int min success, got %v", err)
	}
	if _, err := optionalIntMinField(map[string]json.RawMessage{"num": json.RawMessage(`0`)}, "num", "x", 1); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("expected optional int min validation, got %v", err)
	}

	if _, err := requiredBoolField(obj, "flag", "x"); err != nil {
		t.Fatalf("expected required bool success, got %v", err)
	}
	if _, err := requiredBoolField(map[string]json.RawMessage{"flag": json.RawMessage(`"yes"`)}, "flag", "x"); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("expected required bool validation, got %v", err)
	}
}
