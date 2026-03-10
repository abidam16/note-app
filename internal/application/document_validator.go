package application

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"note-app/internal/domain"
)

const (
	blockParagraph    = "paragraph"
	blockHeading      = "heading"
	blockBulletList   = "bullet_list"
	blockNumberedList = "numbered_list"
	blockTaskList     = "task_list"
	blockQuote        = "quote"
	blockCodeBlock    = "code_block"
	blockTable        = "table"
	blockImage        = "image"

	inlineText = "text"

	markBold       = "bold"
	markItalic     = "italic"
	markInlineCode = "inline_code"
	markLink       = "link"
)

func ValidateDocument(content json.RawMessage) error {
	if len(strings.TrimSpace(string(content))) == 0 {
		return fmt.Errorf("%w: content is required", domain.ErrValidation)
	}

	var blocks []json.RawMessage
	if err := json.Unmarshal(content, &blocks); err != nil {
		return fmt.Errorf("%w: content must be a JSON array of blocks", domain.ErrValidation)
	}

	for i, rawBlock := range blocks {
		if err := validateBlock(rawBlock, fmt.Sprintf("blocks[%d]", i)); err != nil {
			return err
		}
	}

	return nil
}

func validateBlock(raw json.RawMessage, path string) error {
	obj, err := decodeObject(raw, path)
	if err != nil {
		return err
	}

	blockType, err := requiredStringField(obj, "type", path)
	if err != nil {
		return err
	}

	switch blockType {
	case blockParagraph, blockQuote:
		return validateTextContainer(obj, path)
	case blockHeading:
		if _, err := requiredIntInRangeField(obj, "level", path, 1, 6); err != nil {
			return err
		}
		return validateTextContainer(obj, path)
	case blockBulletList:
		return validateListItems(obj, path, false)
	case blockNumberedList:
		if _, err := optionalIntMinField(obj, "start", path, 1); err != nil {
			return err
		}
		return validateListItems(obj, path, false)
	case blockTaskList:
		return validateListItems(obj, path, true)
	case blockCodeBlock:
		if _, err := requiredStringField(obj, "text", path); err != nil {
			return err
		}
		if _, _, err := optionalStringField(obj, "language", path); err != nil {
			return err
		}
		return nil
	case blockTable:
		return validateTable(obj, path)
	case blockImage:
		return validateImage(obj, path)
	default:
		return fmt.Errorf("%w: %s has unsupported block type %q", domain.ErrValidation, path, blockType)
	}
}

func validateTextContainer(obj map[string]json.RawMessage, path string) error {
	rawText, hasText := obj["text"]
	rawChildren, hasChildren := obj["children"]
	if hasText == hasChildren {
		return fmt.Errorf("%w: %s must define exactly one of text or children", domain.ErrValidation, path)
	}

	if hasText {
		if _, err := stringFromRaw(rawText, path+".text"); err != nil {
			return err
		}
		return nil
	}

	return validateInlineNodes(rawChildren, path+".children")
}

func validateListItems(obj map[string]json.RawMessage, path string, requireChecked bool) error {
	rawItems, ok := obj["items"]
	if !ok {
		return fmt.Errorf("%w: %s.items is required", domain.ErrValidation, path)
	}

	var items []json.RawMessage
	if err := json.Unmarshal(rawItems, &items); err != nil {
		return fmt.Errorf("%w: %s.items must be an array", domain.ErrValidation, path)
	}

	for i, rawItem := range items {
		itemPath := fmt.Sprintf("%s.items[%d]", path, i)
		item, err := decodeObject(rawItem, itemPath)
		if err != nil {
			return err
		}
		if requireChecked {
			if _, err := requiredBoolField(item, "checked", itemPath); err != nil {
				return err
			}
		}
		if err := validateTextContainer(item, itemPath); err != nil {
			return err
		}
	}

	return nil
}

func validateTable(obj map[string]json.RawMessage, path string) error {
	rawRows, ok := obj["rows"]
	if !ok {
		return fmt.Errorf("%w: %s.rows is required", domain.ErrValidation, path)
	}

	var rows []json.RawMessage
	if err := json.Unmarshal(rawRows, &rows); err != nil {
		return fmt.Errorf("%w: %s.rows must be an array", domain.ErrValidation, path)
	}

	for i, rawRow := range rows {
		rowPath := fmt.Sprintf("%s.rows[%d]", path, i)
		row, err := decodeObject(rawRow, rowPath)
		if err != nil {
			return err
		}

		rawCells, ok := row["cells"]
		if !ok {
			return fmt.Errorf("%w: %s.cells is required", domain.ErrValidation, rowPath)
		}

		var cells []json.RawMessage
		if err := json.Unmarshal(rawCells, &cells); err != nil {
			return fmt.Errorf("%w: %s.cells must be an array", domain.ErrValidation, rowPath)
		}

		for j, rawCell := range cells {
			cellPath := fmt.Sprintf("%s.cells[%d]", rowPath, j)
			cell, err := decodeObject(rawCell, cellPath)
			if err != nil {
				return err
			}
			if err := validateTextContainer(cell, cellPath); err != nil {
				return err
			}
		}
	}

	return nil
}

func validateImage(obj map[string]json.RawMessage, path string) error {
	src, err := requiredStringField(obj, "src", path)
	if err != nil {
		return err
	}
	if strings.TrimSpace(src) == "" {
		return fmt.Errorf("%w: %s.src is required", domain.ErrValidation, path)
	}
	if _, _, err := optionalStringField(obj, "alt", path); err != nil {
		return err
	}
	return nil
}

func validateInlineNodes(raw json.RawMessage, path string) error {
	var nodes []json.RawMessage
	if err := json.Unmarshal(raw, &nodes); err != nil {
		return fmt.Errorf("%w: %s must be an array", domain.ErrValidation, path)
	}

	for i, rawNode := range nodes {
		nodePath := fmt.Sprintf("%s[%d]", path, i)
		node, err := decodeObject(rawNode, nodePath)
		if err != nil {
			return err
		}
		nodeType, err := requiredStringField(node, "type", nodePath)
		if err != nil {
			return err
		}
		if nodeType != inlineText {
			return fmt.Errorf("%w: %s has unsupported inline type %q", domain.ErrValidation, nodePath, nodeType)
		}
		if _, err := requiredStringField(node, "text", nodePath); err != nil {
			return err
		}
		if rawMarks, ok := node["marks"]; ok {
			if err := validateMarks(rawMarks, nodePath+".marks"); err != nil {
				return err
			}
		}
	}

	return nil
}

func validateMarks(raw json.RawMessage, path string) error {
	var marks []json.RawMessage
	if err := json.Unmarshal(raw, &marks); err != nil {
		return fmt.Errorf("%w: %s must be an array", domain.ErrValidation, path)
	}

	for i, rawMark := range marks {
		markPath := fmt.Sprintf("%s[%d]", path, i)
		mark, err := decodeObject(rawMark, markPath)
		if err != nil {
			return err
		}
		markType, err := requiredStringField(mark, "type", markPath)
		if err != nil {
			return err
		}

		switch markType {
		case markBold, markItalic, markInlineCode:
		case markLink:
			href, err := requiredStringField(mark, "href", markPath)
			if err != nil {
				return err
			}
			parsed, err := url.Parse(strings.TrimSpace(href))
			if err != nil || parsed == nil || parsed.Scheme == "" || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
				return fmt.Errorf("%w: %s.href must be a valid http or https URL", domain.ErrValidation, markPath)
			}
		default:
			return fmt.Errorf("%w: %s has unsupported mark type %q", domain.ErrValidation, markPath, markType)
		}
	}

	return nil
}

func decodeObject(raw json.RawMessage, path string) (map[string]json.RawMessage, error) {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, fmt.Errorf("%w: %s must be an object", domain.ErrValidation, path)
	}
	return obj, nil
}

func requiredStringField(obj map[string]json.RawMessage, field, path string) (string, error) {
	raw, ok := obj[field]
	if !ok {
		return "", fmt.Errorf("%w: %s.%s is required", domain.ErrValidation, path, field)
	}
	return stringFromRaw(raw, path+"."+field)
}

func optionalStringField(obj map[string]json.RawMessage, field, path string) (string, bool, error) {
	raw, ok := obj[field]
	if !ok {
		return "", false, nil
	}
	value, err := stringFromRaw(raw, path+"."+field)
	if err != nil {
		return "", true, err
	}
	return value, true, nil
}

func stringFromRaw(raw json.RawMessage, path string) (string, error) {
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", fmt.Errorf("%w: %s must be a string", domain.ErrValidation, path)
	}
	return value, nil
}

func requiredIntInRangeField(obj map[string]json.RawMessage, field, path string, minValue, maxValue int) (int, error) {
	raw, ok := obj[field]
	if !ok {
		return 0, fmt.Errorf("%w: %s.%s is required", domain.ErrValidation, path, field)
	}

	var value int
	if err := json.Unmarshal(raw, &value); err != nil {
		return 0, fmt.Errorf("%w: %s.%s must be an integer", domain.ErrValidation, path, field)
	}
	if value < minValue || value > maxValue {
		return 0, fmt.Errorf("%w: %s.%s must be between %d and %d", domain.ErrValidation, path, field, minValue, maxValue)
	}
	return value, nil
}

func optionalIntMinField(obj map[string]json.RawMessage, field, path string, minValue int) (int, error) {
	raw, ok := obj[field]
	if !ok {
		return 0, nil
	}

	var value int
	if err := json.Unmarshal(raw, &value); err != nil {
		return 0, fmt.Errorf("%w: %s.%s must be an integer", domain.ErrValidation, path, field)
	}
	if value < minValue {
		return 0, fmt.Errorf("%w: %s.%s must be greater than or equal to %d", domain.ErrValidation, path, field, minValue)
	}
	return value, nil
}

func requiredBoolField(obj map[string]json.RawMessage, field, path string) (bool, error) {
	raw, ok := obj[field]
	if !ok {
		return false, fmt.Errorf("%w: %s.%s is required", domain.ErrValidation, path, field)
	}

	var value bool
	if err := json.Unmarshal(raw, &value); err != nil {
		return false, fmt.Errorf("%w: %s.%s must be a boolean", domain.ErrValidation, path, field)
	}
	return value, nil
}
