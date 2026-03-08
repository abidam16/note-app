package application

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"note-app/internal/domain"
)

type CompareRevisionsInput struct {
	PageID         string
	FromRevisionID string
	ToRevisionID   string
}

func (s RevisionService) CompareRevisions(ctx context.Context, actorID string, input CompareRevisionsInput) (domain.RevisionDiff, error) {
	if strings.TrimSpace(input.FromRevisionID) == "" || strings.TrimSpace(input.ToRevisionID) == "" {
		return domain.RevisionDiff{}, fmt.Errorf("%w: from and to revision ids are required", domain.ErrValidation)
	}

	page, _, err := s.pages.GetByID(ctx, input.PageID)
	if err != nil {
		return domain.RevisionDiff{}, err
	}
	if _, err := s.memberships.GetMembershipByUserID(ctx, page.WorkspaceID, actorID); err != nil {
		return domain.RevisionDiff{}, err
	}

	fromRevision, err := s.revisions.GetByID(ctx, input.FromRevisionID)
	if err != nil {
		return domain.RevisionDiff{}, err
	}
	toRevision, err := s.revisions.GetByID(ctx, input.ToRevisionID)
	if err != nil {
		return domain.RevisionDiff{}, err
	}
	if fromRevision.PageID != input.PageID || toRevision.PageID != input.PageID || fromRevision.PageID != toRevision.PageID {
		return domain.RevisionDiff{}, fmt.Errorf("%w: revisions must belong to the requested page", domain.ErrValidation)
	}
	if err := ValidateDocument(fromRevision.Content); err != nil {
		return domain.RevisionDiff{}, err
	}
	if err := ValidateDocument(toRevision.Content); err != nil {
		return domain.RevisionDiff{}, err
	}

	blocks, err := buildRevisionDiffBlocks(fromRevision.Content, toRevision.Content)
	if err != nil {
		return domain.RevisionDiff{}, err
	}

	return domain.RevisionDiff{
		PageID:         input.PageID,
		FromRevisionID: fromRevision.ID,
		ToRevisionID:   toRevision.ID,
		Blocks:         blocks,
	}, nil
}

func buildRevisionDiffBlocks(fromContent, toContent json.RawMessage) ([]domain.RevisionDiffBlock, error) {
	fromBlocks, err := decodeDocumentBlocks(fromContent)
	if err != nil {
		return nil, err
	}
	toBlocks, err := decodeDocumentBlocks(toContent)
	if err != nil {
		return nil, err
	}

	fromFingerprints := make([]string, len(fromBlocks))
	for i, block := range fromBlocks {
		fromFingerprints[i] = compactJSON(block)
	}
	toFingerprints := make([]string, len(toBlocks))
	for i, block := range toBlocks {
		toFingerprints[i] = compactJSON(block)
	}

	lcs := buildLCSMatrix(fromFingerprints, toFingerprints)
	result := make([]domain.RevisionDiffBlock, 0)
	index := 0
	for i, j := 0, 0; i < len(fromBlocks) || j < len(toBlocks); {
		switch {
		case i < len(fromBlocks) && j < len(toBlocks) && fromFingerprints[i] == toFingerprints[j]:
			result = append(result, domain.RevisionDiffBlock{
				Index:  index,
				Status: "unchanged",
				From:   blockSnapshot(fromBlocks[i]),
				To:     blockSnapshot(toBlocks[j]),
			})
			i++
			j++
		case i < len(fromBlocks) && j < len(toBlocks) && blockType(fromBlocks[i]) == blockType(toBlocks[j]):
			fromSnapshot := blockSnapshot(fromBlocks[i])
			toSnapshot := blockSnapshot(toBlocks[j])
			result = append(result, domain.RevisionDiffBlock{
				Index:      index,
				Status:     "modified",
				From:       fromSnapshot,
				To:         toSnapshot,
				InlineDiff: diffText(fromSnapshot.Text, toSnapshot.Text),
			})
			i++
			j++
		case j < len(toBlocks) && (i == len(fromBlocks) || lcs[i][j+1] >= lcs[i+1][j]):
			result = append(result, domain.RevisionDiffBlock{
				Index:  index,
				Status: "added",
				To:     blockSnapshot(toBlocks[j]),
			})
			j++
		default:
			result = append(result, domain.RevisionDiffBlock{
				Index:  index,
				Status: "removed",
				From:   blockSnapshot(fromBlocks[i]),
			})
			i++
		}
		index++
	}

	return result, nil
}

func decodeDocumentBlocks(content json.RawMessage) ([]json.RawMessage, error) {
	var blocks []json.RawMessage
	if err := json.Unmarshal(content, &blocks); err != nil {
		return nil, fmt.Errorf("%w: content must be a JSON array of blocks", domain.ErrValidation)
	}
	return blocks, nil
}

func compactJSON(raw json.RawMessage) string {
	var compacted bytes.Buffer
	if err := json.Compact(&compacted, raw); err != nil {
		return strings.TrimSpace(string(raw))
	}
	return compacted.String()
}

func buildLCSMatrix(a, b []string) [][]int {
	matrix := make([][]int, len(a)+1)
	for i := range matrix {
		matrix[i] = make([]int, len(b)+1)
	}
	for i := len(a) - 1; i >= 0; i-- {
		for j := len(b) - 1; j >= 0; j-- {
			if a[i] == b[j] {
				matrix[i][j] = matrix[i+1][j+1] + 1
				continue
			}
			if matrix[i+1][j] >= matrix[i][j+1] {
				matrix[i][j] = matrix[i+1][j]
			} else {
				matrix[i][j] = matrix[i][j+1]
			}
		}
	}
	return matrix
}

func blockSnapshot(raw json.RawMessage) *domain.RevisionDiffSnapshot {
	obj, err := decodeObject(raw, "block")
	if err != nil {
		return &domain.RevisionDiffSnapshot{Type: "unknown", Text: strings.TrimSpace(string(raw))}
	}
	blockType, _ := requiredStringField(obj, "type", "block")
	return &domain.RevisionDiffSnapshot{Type: blockType, Text: extractBlockText(obj)}
}

func blockType(raw json.RawMessage) string {
	obj, err := decodeObject(raw, "block")
	if err != nil {
		return "unknown"
	}
	value, err := requiredStringField(obj, "type", "block")
	if err != nil {
		return "unknown"
	}
	return value
}

func extractBlockText(obj map[string]json.RawMessage) string {
	blockType, err := requiredStringField(obj, "type", "block")
	if err != nil {
		return ""
	}

	switch blockType {
	case blockParagraph, blockHeading, blockQuote:
		return extractTextContainer(obj)
	case blockBulletList, blockNumberedList, blockTaskList:
		return extractListText(obj)
	case blockCodeBlock:
		text, _ := requiredStringField(obj, "text", "block")
		return text
	case blockTable:
		return extractTableText(obj)
	case blockImage:
		if alt, ok, _ := optionalStringField(obj, "alt", "block"); ok && strings.TrimSpace(alt) != "" {
			return alt
		}
		src, _ := requiredStringField(obj, "src", "block")
		return src
	default:
		return strings.TrimSpace(string(obj["type"]))
	}
}

func extractTextContainer(obj map[string]json.RawMessage) string {
	if rawText, ok := obj["text"]; ok {
		text, _ := stringFromRaw(rawText, "block.text")
		return text
	}
	if rawChildren, ok := obj["children"]; ok {
		return extractInlineNodesText(rawChildren)
	}
	return ""
}

func extractListText(obj map[string]json.RawMessage) string {
	rawItems, ok := obj["items"]
	if !ok {
		return ""
	}
	var items []json.RawMessage
	if err := json.Unmarshal(rawItems, &items); err != nil {
		return ""
	}
	parts := make([]string, 0, len(items))
	for _, rawItem := range items {
		item, err := decodeObject(rawItem, "item")
		if err != nil {
			continue
		}
		parts = append(parts, extractTextContainer(item))
	}
	return strings.Join(parts, "\n")
}

func extractTableText(obj map[string]json.RawMessage) string {
	rawRows, ok := obj["rows"]
	if !ok {
		return ""
	}
	var rows []json.RawMessage
	if err := json.Unmarshal(rawRows, &rows); err != nil {
		return ""
	}
	rowTexts := make([]string, 0, len(rows))
	for _, rawRow := range rows {
		row, err := decodeObject(rawRow, "row")
		if err != nil {
			continue
		}
		rawCells, ok := row["cells"]
		if !ok {
			continue
		}
		var cells []json.RawMessage
		if err := json.Unmarshal(rawCells, &cells); err != nil {
			continue
		}
		cellTexts := make([]string, 0, len(cells))
		for _, rawCell := range cells {
			cell, err := decodeObject(rawCell, "cell")
			if err != nil {
				continue
			}
			cellTexts = append(cellTexts, extractTextContainer(cell))
		}
		rowTexts = append(rowTexts, strings.Join(cellTexts, "\t"))
	}
	return strings.Join(rowTexts, "\n")
}

func extractInlineNodesText(raw json.RawMessage) string {
	var nodes []json.RawMessage
	if err := json.Unmarshal(raw, &nodes); err != nil {
		return ""
	}
	parts := make([]string, 0, len(nodes))
	for _, rawNode := range nodes {
		node, err := decodeObject(rawNode, "inline")
		if err != nil {
			continue
		}
		text, err := requiredStringField(node, "text", "inline")
		if err != nil {
			continue
		}
		parts = append(parts, text)
	}
	return strings.Join(parts, "")
}

func diffText(fromText, toText string) []domain.RevisionDiffTextChunk {
	fromWords := strings.Fields(fromText)
	toWords := strings.Fields(toText)
	if len(fromWords) == 0 && len(toWords) == 0 {
		return nil
	}

	lcs := buildLCSMatrix(fromWords, toWords)
	chunks := make([]domain.RevisionDiffTextChunk, 0)
	for i, j := 0, 0; i < len(fromWords) || j < len(toWords); {
		switch {
		case i < len(fromWords) && j < len(toWords) && fromWords[i] == toWords[j]:
			chunks = appendWordChunk(chunks, "equal", fromWords[i])
			i++
			j++
		case j < len(toWords) && (i == len(fromWords) || lcs[i][j+1] >= lcs[i+1][j]):
			chunks = appendWordChunk(chunks, "added", toWords[j])
			j++
		default:
			chunks = appendWordChunk(chunks, "removed", fromWords[i])
			i++
		}
	}
	return chunks
}

func appendWordChunk(chunks []domain.RevisionDiffTextChunk, operation, word string) []domain.RevisionDiffTextChunk {
	if len(chunks) == 0 || chunks[len(chunks)-1].Operation != operation {
		return append(chunks, domain.RevisionDiffTextChunk{Operation: operation, Text: word})
	}
	chunks[len(chunks)-1].Text += " " + word
	return chunks
}
