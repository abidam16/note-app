package application

import (
	"encoding/json"
	"testing"
)

var (
	benchmarkDocument = json.RawMessage(`[
		{"type":"heading","level":2,"children":[{"type":"text","text":"Architecture Overview"}]},
		{"type":"paragraph","children":[
			{"type":"text","text":"This document explains the current platform design."},
			{"type":"text","text":"It includes search, revisions, and validation.","marks":[{"type":"italic"}]}
		]},
		{"type":"bullet_list","items":[
			{"children":[{"type":"text","text":"Capture requirements"}]},
			{"children":[{"type":"text","text":"Validate blocks"}]},
			{"children":[{"type":"text","text":"Store searchable body"}]}
		]},
		{"type":"task_list","items":[
			{"checked":true,"children":[{"type":"text","text":"Draft schema"}]},
			{"checked":false,"children":[{"type":"text","text":"Measure performance"}]}
		]},
		{"type":"code_block","language":"go","text":"func main() { println(\"hello\") }"},
		{"type":"table","rows":[
			{"cells":[
				{"text":"Owner"},
				{"children":[{"type":"text","text":"Platform Team"}]}
			]},
			{"cells":[
				{"text":"Status"},
				{"children":[{"type":"text","text":"Active"}]}
			]}
		]},
		{"type":"image","src":"https://example.com/diagram.png","alt":"System diagram"}
	]`)
	benchmarkUpdatedDocument = json.RawMessage(`[
		{"type":"heading","level":2,"children":[{"type":"text","text":"Architecture Overview"}]},
		{"type":"paragraph","children":[
			{"type":"text","text":"This document explains the optimized platform design."},
			{"type":"text","text":"It now includes faster search and leaner diffs.","marks":[{"type":"italic"}]}
		]},
		{"type":"bullet_list","items":[
			{"children":[{"type":"text","text":"Capture requirements"}]},
			{"children":[{"type":"text","text":"Validate blocks"}]},
			{"children":[{"type":"text","text":"Store searchable body"}]},
			{"children":[{"type":"text","text":"Benchmark the hottest paths"}]}
		]},
		{"type":"task_list","items":[
			{"checked":true,"children":[{"type":"text","text":"Draft schema"}]},
			{"checked":true,"children":[{"type":"text","text":"Measure performance"}]}
		]},
		{"type":"code_block","language":"go","text":"func main() { println(\"optimized\") }"},
		{"type":"table","rows":[
			{"cells":[
				{"text":"Owner"},
				{"children":[{"type":"text","text":"Platform Team"}]}
			]},
			{"cells":[
				{"text":"Status"},
				{"children":[{"type":"text","text":"Optimized"}]}
			]}
		]},
		{"type":"image","src":"https://example.com/diagram-v2.png","alt":"Updated system diagram"}
	]`)
)

func BenchmarkValidateDocument(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		if err := ValidateDocument(benchmarkDocument); err != nil {
			b.Fatalf("ValidateDocument() error = %v", err)
		}
	}
}

func BenchmarkExtractSearchBody(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		if got := ExtractSearchBody(benchmarkDocument); got == "" {
			b.Fatal("expected non-empty search body")
		}
	}
}

func BenchmarkBuildRevisionDiffBlocks(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		blocks, err := buildRevisionDiffBlocks(benchmarkDocument, benchmarkUpdatedDocument)
		if err != nil {
			b.Fatalf("buildRevisionDiffBlocks() error = %v", err)
		}
		if len(blocks) == 0 {
			b.Fatal("expected non-empty diff")
		}
	}
}
