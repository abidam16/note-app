package application

import (
	"encoding/json"
	"errors"
	"testing"

	"note-app/internal/domain"
)

func TestValidateDocumentAcceptsSupportedBlocks(t *testing.T) {
	content := json.RawMessage(`[
		{"type":"heading","level":2,"children":[{"type":"text","text":"Architecture"}]},
		{"type":"paragraph","children":[{"type":"text","text":"Open the ","marks":[{"type":"bold"}]},{"type":"text","text":"spec","marks":[{"type":"link","href":"https://example.com/spec"}]}]},
		{"type":"bullet_list","items":[{"children":[{"type":"text","text":"Capture requirements"}]}]},
		{"type":"numbered_list","start":1,"items":[{"text":"Design API"}]},
		{"type":"task_list","items":[{"checked":true,"children":[{"type":"text","text":"Review schema","marks":[{"type":"italic"}]}]}]},
		{"type":"quote","text":"Ship the smallest valuable slice first."},
		{"type":"code_block","language":"go","text":"fmt.Println(\"hello\")"},
		{"type":"table","rows":[{"cells":[{"text":"Name"},{"children":[{"type":"text","text":"Value","marks":[{"type":"inline_code"}]}]}]}]},
		{"type":"image","src":"/uploads/system-design.png","alt":"System design"}
	]`)

	if err := ValidateDocument(content); err != nil {
		t.Fatalf("ValidateDocument() error = %v", err)
	}
}

func TestValidateDocumentRejectsUnsupportedBlockType(t *testing.T) {
	err := ValidateDocument(json.RawMessage(`[{"type":"embed","url":"https://example.com"}]`))
	if !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("expected validation error, got %v", err)
	}
}

func TestValidateDocumentRejectsMalformedMetadata(t *testing.T) {
	tests := []struct {
		name    string
		content json.RawMessage
	}{
		{
			name:    "invalid link mark",
			content: json.RawMessage(`[{"type":"paragraph","children":[{"type":"text","text":"Broken link","marks":[{"type":"link","href":"notaurl"}]}]}]`),
		},
		{
			name:    "missing image src",
			content: json.RawMessage(`[{"type":"image","alt":"Missing source"}]`),
		},
		{
			name:    "invalid nested block in paragraph children",
			content: json.RawMessage(`[{"type":"paragraph","children":[{"type":"paragraph","text":"nested"}]}]`),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateDocument(tt.content)
			if !errors.Is(err, domain.ErrValidation) {
				t.Fatalf("expected validation error, got %v", err)
			}
		})
	}
}
