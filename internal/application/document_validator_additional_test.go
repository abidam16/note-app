package application

import "testing"

func TestValidateDocumentAdditionalFailures(t *testing.T) {
	testCases := []struct {
		name    string
		content string
	}{
		{name: "empty content", content: ``},
		{name: "non-array root", content: `{"type":"paragraph"}`},
		{name: "heading level out of range", content: `[{"type":"heading","level":7,"text":"x"}]`},
		{name: "paragraph requires text xor children", content: `[{"type":"paragraph"}]`},
		{name: "bullet list items not array", content: `[{"type":"bullet_list","items":{}}]`},
		{name: "task list requires checked", content: `[{"type":"task_list","items":[{"text":"x"}]}]`},
		{name: "numbered list invalid start", content: `[{"type":"numbered_list","start":0,"items":[{"text":"x"}]}]`},
		{name: "code block text required", content: `[{"type":"code_block"}]`},
		{name: "table rows required", content: `[{"type":"table"}]`},
		{name: "table row cells required", content: `[{"type":"table","rows":[{}]}]`},
		{name: "image src required", content: `[{"type":"image","src":"   "}]`},
		{name: "inline type unsupported", content: `[{"type":"paragraph","children":[{"type":"emoji","text":":)"}]}]`},
		{name: "marks must be array", content: `[{"type":"paragraph","children":[{"type":"text","text":"x","marks":{}}]}]`},
		{name: "unsupported mark", content: `[{"type":"paragraph","children":[{"type":"text","text":"x","marks":[{"type":"underline"}]}]}]`},
		{name: "invalid link url", content: `[{"type":"paragraph","children":[{"type":"text","text":"x","marks":[{"type":"link","href":"ftp://bad"}]}]}]`},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if err := ValidateDocument([]byte(tc.content)); err == nil {
				t.Fatalf("expected validation error for %s", tc.name)
			}
		})
	}
}

func TestValidateDocumentAdditionalSuccessCases(t *testing.T) {
	valid := `[
		{"type":"heading","level":2,"children":[{"type":"text","text":"H"}]},
		{"type":"numbered_list","start":1,"items":[{"text":"item"}]},
		{"type":"task_list","items":[{"checked":true,"children":[{"type":"text","text":"done"}]}]},
		{"type":"table","rows":[{"cells":[{"text":"a"},{"children":[{"type":"text","text":"b"}]}]}]},
		{"type":"image","src":"https://example.com/image.png","alt":"img"}
	]`
	if err := ValidateDocument([]byte(valid)); err != nil {
		t.Fatalf("expected valid complex document, got %v", err)
	}
}

func TestValidateDocumentThreadAnchorsReady(t *testing.T) {
	valid := `[
		{"id":"block-1","type":"paragraph","children":[{"type":"text","text":"Hello"}]},
		{"id":"block-2","type":"heading","level":2,"children":[{"type":"text","text":"World"}]}
	]`
	if err := ValidateDocumentThreadAnchorsReady([]byte(valid)); err != nil {
		t.Fatalf("expected thread-anchor-ready document, got %v", err)
	}

	missingID := `[{"type":"paragraph","children":[{"type":"text","text":"Hello"}]}]`
	if err := ValidateDocumentThreadAnchorsReady([]byte(missingID)); err == nil {
		t.Fatal("expected missing block id to fail")
	}

	blankID := `[{"id":"   ","type":"paragraph","children":[{"type":"text","text":"Hello"}]}]`
	if err := ValidateDocumentThreadAnchorsReady([]byte(blankID)); err == nil {
		t.Fatal("expected blank block id to fail")
	}
}
