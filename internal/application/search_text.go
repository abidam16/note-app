package application

import (
	"encoding/json"
	"strings"
)

func ExtractSearchBody(content json.RawMessage) string {
	if len(content) == 0 {
		return ""
	}

	var root any
	if err := json.Unmarshal(content, &root); err != nil {
		return ""
	}

	parts := make([]string, 0)
	collectSearchText(root, &parts)
	return strings.Join(parts, " ")
}

func collectSearchText(node any, parts *[]string) {
	switch value := node.(type) {
	case map[string]any:
		for key, field := range value {
			switch key {
			case "text", "alt":
				if text, ok := field.(string); ok {
					trimmed := strings.TrimSpace(text)
					if trimmed != "" {
						*parts = append(*parts, trimmed)
					}
				}
			default:
				collectSearchText(field, parts)
			}
		}
	case []any:
		for _, item := range value {
			collectSearchText(item, parts)
		}
	}
}
