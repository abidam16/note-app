package application

import (
	"errors"
	"testing"

	"note-app/internal/domain"
)

func TestRevisionServiceHelpers(t *testing.T) {
	if normalizeOptionalText(nil) != nil {
		t.Fatal("expected nil optional text for nil input")
	}
	blank := "   "
	if normalizeOptionalText(&blank) != nil {
		t.Fatal("expected nil optional text for blank input")
	}
	value := "  hello  "
	normalized := normalizeOptionalText(&value)
	if normalized == nil || *normalized != "hello" {
		t.Fatalf("unexpected normalized value: %+v", normalized)
	}

	if cloneRawMessage(nil) != nil {
		t.Fatal("expected nil clone for nil value")
	}
	original := []byte(`{"a":1}`)
	cloned := cloneRawMessage(original)
	if string(cloned) != string(original) {
		t.Fatalf("unexpected clone value: %s", string(cloned))
	}
	cloned[0] = '['
	if string(original) == string(cloned) {
		t.Fatal("expected clone to be independent from original")
	}
}

func TestRevisionServiceCompareRequiresIDs(t *testing.T) {
	svc := RevisionService{}
	if _, err := svc.CompareRevisions(nil, "user", CompareRevisionsInput{}); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("expected validation error for missing compare ids, got %v", err)
	}
}
