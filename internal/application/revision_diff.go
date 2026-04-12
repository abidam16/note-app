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

type revisionBlockInfo struct {
	fingerprint string
	snapshot    *domain.RevisionDiffSnapshot
}

type sequenceEditType uint8

const (
	sequenceEqual sequenceEditType = iota
	sequenceAdd
	sequenceRemove
)

type sequenceEdit struct {
	typ    sequenceEditType
	fromIx int
	toIx   int
}

func (s RevisionService) CompareRevisions(ctx context.Context, actorID string, input CompareRevisionsInput) (domain.RevisionDiff, error) {
	if strings.TrimSpace(input.FromRevisionID) == "" || strings.TrimSpace(input.ToRevisionID) == "" {
		return domain.RevisionDiff{}, fmt.Errorf("%w: from and to revision ids are required", domain.ErrValidation)
	}

	if _, _, err := loadVisiblePageForActor(ctx, s.pages, s.memberships, actorID, input.PageID); err != nil {
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

	fromInfos := buildRevisionBlockInfo(fromBlocks)
	toInfos := buildRevisionBlockInfo(toBlocks)

	fromFingerprints := make([]string, len(fromInfos))
	for i := range fromInfos {
		fromFingerprints[i] = fromInfos[i].fingerprint
	}
	toFingerprints := make([]string, len(toInfos))
	for i := range toInfos {
		toFingerprints[i] = toInfos[i].fingerprint
	}

	edits := buildMyersEdits(fromFingerprints, toFingerprints)
	result := make([]domain.RevisionDiffBlock, 0, len(fromInfos)+len(toInfos))
	index := 0
	for i := 0; i < len(edits); i++ {
		edit := edits[i]
		switch edit.typ {
		case sequenceEqual:
			result = append(result, domain.RevisionDiffBlock{
				Index:  index,
				Status: "unchanged",
				From:   fromInfos[edit.fromIx].snapshot,
				To:     toInfos[edit.toIx].snapshot,
				Lines:  buildBlockLines("unchanged", fromInfos[edit.fromIx].snapshot, toInfos[edit.toIx].snapshot),
			})
		case sequenceRemove:
			if i+1 < len(edits) && edits[i+1].typ == sequenceAdd {
				fromSnapshot := fromInfos[edit.fromIx].snapshot
				toSnapshot := toInfos[edits[i+1].toIx].snapshot
				if fromSnapshot.Type == toSnapshot.Type {
					result = append(result, domain.RevisionDiffBlock{
						Index:      index,
						Status:     "modified",
						From:       fromSnapshot,
						To:         toSnapshot,
						InlineDiff: diffText(fromSnapshot.Text, toSnapshot.Text),
						Lines:      buildBlockLines("modified", fromSnapshot, toSnapshot),
					})
					i++
					index++
					continue
				}
			}
			result = append(result, domain.RevisionDiffBlock{
				Index:  index,
				Status: "removed",
				From:   fromInfos[edit.fromIx].snapshot,
				Lines:  buildBlockLines("removed", fromInfos[edit.fromIx].snapshot, nil),
			})
		case sequenceAdd:
			result = append(result, domain.RevisionDiffBlock{
				Index:  index,
				Status: "added",
				To:     toInfos[edit.toIx].snapshot,
				Lines:  buildBlockLines("added", nil, toInfos[edit.toIx].snapshot),
			})
		}
		index++
	}

	return result, nil
}

func buildRevisionBlockInfo(blocks []json.RawMessage) []revisionBlockInfo {
	info := make([]revisionBlockInfo, len(blocks))
	for i := range blocks {
		info[i] = revisionBlockInfo{
			// Revision payloads may carry editor-generated block ids that change on
			// save even when the visible content did not. Strip those ids so Myers
			// can align semantically equal blocks instead of cascading false edits.
			fingerprint: normalizeBlockFingerprint(blocks[i]),
			snapshot:    blockSnapshot(blocks[i]),
		}
	}
	return info
}

func normalizeBlockFingerprint(raw json.RawMessage) string {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return compactJSON(raw)
	}

	normalized, err := json.Marshal(stripDiffMetadata(value))
	if err != nil {
		return compactJSON(raw)
	}
	return compactJSON(normalized)
}

func stripDiffMetadata(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		cleaned := make(map[string]any, len(typed))
		for key, child := range typed {
			if key == "id" {
				continue
			}
			cleaned[key] = stripDiffMetadata(child)
		}
		return cleaned
	case []any:
		for i := range typed {
			typed[i] = stripDiffMetadata(typed[i])
		}
		return typed
	default:
		return typed
	}
}

func buildMyersEdits(from, to []string) []sequenceEdit {
	maxDistance := len(from) + len(to)
	if maxDistance == 0 {
		return nil
	}

	offset := maxDistance
	v := make([]int, 2*maxDistance+1)
	trace := make([][]int, 0, maxDistance+1)

	for d := 0; d <= maxDistance; d++ {
		trace = append(trace, append([]int(nil), v...))
		for k := -d; k <= d; k += 2 {
			kIndex := offset + k
			var x int
			if k == -d || (k != d && v[kIndex-1] < v[kIndex+1]) {
				x = v[kIndex+1]
			} else {
				x = v[kIndex-1] + 1
			}

			y := x - k
			for x < len(from) && y < len(to) && from[x] == to[y] {
				x++
				y++
			}
			v[kIndex] = x
			if x == len(from) && y == len(to) {
				return backtrackMyersEdits(from, to, trace, v, offset, d)
			}
		}
	}

	return nil
}

func backtrackMyersEdits(from, to []string, trace [][]int, finalV []int, offset, distance int) []sequenceEdit {
	trace = append(trace, append([]int(nil), finalV...))
	x := len(from)
	y := len(to)
	edits := make([]sequenceEdit, 0, len(from)+len(to))

	for d := distance; d >= 0; d-- {
		v := trace[d]
		k := x - y
		kIndex := offset + k

		var prevK int
		if d == 0 {
			prevK = 0
		} else if k == -d || (k != d && v[kIndex-1] < v[kIndex+1]) {
			prevK = k + 1
		} else {
			prevK = k - 1
		}

		prevX := v[offset+prevK]
		prevY := prevX - prevK

		for x > prevX && y > prevY {
			x--
			y--
			edits = append(edits, sequenceEdit{typ: sequenceEqual, fromIx: x, toIx: y})
		}

		if d == 0 {
			break
		}

		if x == prevX {
			y--
			edits = append(edits, sequenceEdit{typ: sequenceAdd, toIx: y})
		} else {
			x--
			edits = append(edits, sequenceEdit{typ: sequenceRemove, fromIx: x})
		}
	}

	reverseSequenceEdits(edits)
	return edits
}

func reverseSequenceEdits(edits []sequenceEdit) {
	for left, right := 0, len(edits)-1; left < right; left, right = left+1, right-1 {
		edits[left], edits[right] = edits[right], edits[left]
	}
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
	compacted.Grow(len(raw))
	if err := json.Compact(&compacted, raw); err != nil {
		return strings.TrimSpace(string(raw))
	}
	return compacted.String()
}

func buildLCSMatrix(a, b []string) [][]int {
	rows := len(a) + 1
	cols := len(b) + 1
	// Keep the DP matrix in one backing slice to cut per-row allocations and
	// improve cache locality during the reverse fill pass.
	backing := make([]int, rows*cols)
	matrix := make([][]int, rows)
	for i := range matrix {
		offset := i * cols
		matrix[i] = backing[offset : offset+cols]
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

func buildBlockLines(status string, fromSnapshot, toSnapshot *domain.RevisionDiffSnapshot) []domain.RevisionDiffLine {
	switch status {
	case "unchanged":
		return buildUnchangedBlockLines(splitSnapshotLines(fromSnapshot))
	case "removed":
		return buildRemovedBlockLines(splitSnapshotLines(fromSnapshot))
	case "added":
		return buildAddedBlockLines(splitSnapshotLines(toSnapshot))
	case "modified":
		return buildModifiedBlockLines(splitSnapshotLines(fromSnapshot), splitSnapshotLines(toSnapshot))
	default:
		return nil
	}
}

func splitSnapshotLines(snapshot *domain.RevisionDiffSnapshot) []string {
	if snapshot == nil {
		return nil
	}
	text := strings.ReplaceAll(snapshot.Text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	text = strings.TrimSuffix(text, "\n")
	if text == "" {
		return nil
	}
	return strings.Split(text, "\n")
}

func buildUnchangedBlockLines(lines []string) []domain.RevisionDiffLine {
	if len(lines) == 0 {
		return nil
	}
	result := make([]domain.RevisionDiffLine, 0, len(lines))
	for i, line := range lines {
		fromLineNumber := i + 1
		toLineNumber := i + 1
		result = append(result, domain.RevisionDiffLine{
			Operation:      "context",
			FromLineNumber: intPtr(fromLineNumber),
			ToLineNumber:   intPtr(toLineNumber),
			Text:           line,
		})
	}
	return result
}

func buildRemovedBlockLines(lines []string) []domain.RevisionDiffLine {
	if len(lines) == 0 {
		return nil
	}
	result := make([]domain.RevisionDiffLine, 0, len(lines))
	for i, line := range lines {
		fromLineNumber := i + 1
		result = append(result, domain.RevisionDiffLine{
			Operation:      "removed",
			FromLineNumber: intPtr(fromLineNumber),
			Text:           line,
			Chunks: []domain.RevisionDiffTextChunk{
				{Operation: "removed", Text: line},
			},
		})
	}
	return result
}

func buildAddedBlockLines(lines []string) []domain.RevisionDiffLine {
	if len(lines) == 0 {
		return nil
	}
	result := make([]domain.RevisionDiffLine, 0, len(lines))
	for i, line := range lines {
		toLineNumber := i + 1
		result = append(result, domain.RevisionDiffLine{
			Operation:    "added",
			ToLineNumber: intPtr(toLineNumber),
			Text:         line,
			Chunks: []domain.RevisionDiffTextChunk{
				{Operation: "added", Text: line},
			},
		})
	}
	return result
}

func buildModifiedBlockLines(fromLines, toLines []string) []domain.RevisionDiffLine {
	if len(fromLines) == 0 {
		return buildAddedBlockLines(toLines)
	}
	if len(toLines) == 0 {
		return buildRemovedBlockLines(fromLines)
	}

	edits := buildMyersEdits(fromLines, toLines)
	result := make([]domain.RevisionDiffLine, 0, len(fromLines)+len(toLines))
	for i := 0; i < len(edits); i++ {
		edit := edits[i]
		if edit.typ == sequenceEqual {
			fromLineNumber := edit.fromIx + 1
			toLineNumber := edit.toIx + 1
			result = append(result, domain.RevisionDiffLine{
				Operation:      "context",
				FromLineNumber: intPtr(fromLineNumber),
				ToLineNumber:   intPtr(toLineNumber),
				Text:           fromLines[edit.fromIx],
			})
			continue
		}

		removed := make([]int, 0, 2)
		added := make([]int, 0, 2)
		for ; i < len(edits) && edits[i].typ != sequenceEqual; i++ {
			switch edits[i].typ {
			case sequenceRemove:
				removed = append(removed, edits[i].fromIx)
			case sequenceAdd:
				added = append(added, edits[i].toIx)
			}
		}
		i--

		pairCount := len(removed)
		if len(added) < pairCount {
			pairCount = len(added)
		}

		for j := 0; j < pairCount; j++ {
			removedChunks, addedChunks := buildPairedLineChunks(fromLines[removed[j]], toLines[added[j]])
			fromLineNumber := removed[j] + 1
			toLineNumber := added[j] + 1
			result = append(result,
				domain.RevisionDiffLine{
					Operation:      "removed",
					FromLineNumber: intPtr(fromLineNumber),
					Text:           fromLines[removed[j]],
					Chunks:         removedChunks,
				},
				domain.RevisionDiffLine{
					Operation:    "added",
					ToLineNumber: intPtr(toLineNumber),
					Text:         toLines[added[j]],
					Chunks:       addedChunks,
				},
			)
		}

		for _, fromIx := range removed[pairCount:] {
			fromLineNumber := fromIx + 1
			result = append(result, domain.RevisionDiffLine{
				Operation:      "removed",
				FromLineNumber: intPtr(fromLineNumber),
				Text:           fromLines[fromIx],
				Chunks: []domain.RevisionDiffTextChunk{
					{Operation: "removed", Text: fromLines[fromIx]},
				},
			})
		}
		for _, toIx := range added[pairCount:] {
			toLineNumber := toIx + 1
			result = append(result, domain.RevisionDiffLine{
				Operation:    "added",
				ToLineNumber: intPtr(toLineNumber),
				Text:         toLines[toIx],
				Chunks: []domain.RevisionDiffTextChunk{
					{Operation: "added", Text: toLines[toIx]},
				},
			})
		}
	}

	return result
}

func buildPairedLineChunks(fromLine, toLine string) ([]domain.RevisionDiffTextChunk, []domain.RevisionDiffTextChunk) {
	combined := diffText(fromLine, toLine)
	return projectLineChunks(combined, "removed", fromLine), projectLineChunks(combined, "added", toLine)
}

func projectLineChunks(chunks []domain.RevisionDiffTextChunk, wantedOperation, fallback string) []domain.RevisionDiffTextChunk {
	projected := make([]domain.RevisionDiffTextChunk, 0, len(chunks))
	for _, chunk := range chunks {
		if chunk.Operation != "equal" && chunk.Operation != wantedOperation {
			continue
		}
		projected = appendTextChunk(projected, chunk.Operation, chunk.Text)
	}
	if len(projected) > 0 {
		return projected
	}
	if fallback == "" {
		return nil
	}
	return []domain.RevisionDiffTextChunk{{Operation: wantedOperation, Text: fallback}}
}

func intPtr(value int) *int {
	return &value
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
	var builder strings.Builder
	validItems := 0
	for _, rawItem := range items {
		item, err := decodeObject(rawItem, "item")
		if err != nil {
			continue
		}
		if validItems > 0 {
			builder.WriteByte('\n')
		}
		builder.WriteString(extractTextContainer(item))
		validItems++
	}
	return builder.String()
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
	var rowBuilder strings.Builder
	validRows := 0
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
		var cellBuilder strings.Builder
		validCells := 0
		for _, rawCell := range cells {
			cell, err := decodeObject(rawCell, "cell")
			if err != nil {
				continue
			}
			if validCells > 0 {
				cellBuilder.WriteByte('\t')
			}
			cellBuilder.WriteString(extractTextContainer(cell))
			validCells++
		}
		if validRows > 0 {
			rowBuilder.WriteByte('\n')
		}
		rowBuilder.WriteString(cellBuilder.String())
		validRows++
	}
	return rowBuilder.String()
}

func extractInlineNodesText(raw json.RawMessage) string {
	var nodes []json.RawMessage
	if err := json.Unmarshal(raw, &nodes); err != nil {
		return ""
	}
	var builder strings.Builder
	for _, rawNode := range nodes {
		node, err := decodeObject(rawNode, "inline")
		if err != nil {
			continue
		}
		text, err := requiredStringField(node, "text", "inline")
		if err != nil {
			continue
		}
		builder.WriteString(text)
	}
	return builder.String()
}

func diffText(fromText, toText string) []domain.RevisionDiffTextChunk {
	fromTokens := tokenizeDiffText(fromText)
	toTokens := tokenizeDiffText(toText)
	if len(fromTokens) == 0 && len(toTokens) == 0 {
		return nil
	}

	edits := buildMyersEdits(fromTokens, toTokens)
	chunks := make([]domain.RevisionDiffTextChunk, 0, len(fromTokens)+len(toTokens))
	for _, edit := range edits {
		switch edit.typ {
		case sequenceEqual:
			chunks = appendTextChunk(chunks, "equal", fromTokens[edit.fromIx])
		case sequenceAdd:
			chunks = appendTextChunk(chunks, "added", toTokens[edit.toIx])
		case sequenceRemove:
			chunks = appendTextChunk(chunks, "removed", fromTokens[edit.fromIx])
		}
	}
	return chunks
}

func tokenizeDiffText(text string) []string {
	if text == "" {
		return nil
	}

	tokens := make([]string, 0, len(text)/2+1)
	var builder strings.Builder
	currentClass := diffTokenClassUnknown

	flush := func() {
		if builder.Len() == 0 {
			return
		}
		tokens = append(tokens, builder.String())
		builder.Reset()
	}

	for _, r := range text {
		class := classifyDiffRune(r)
		if class == diffTokenClassPunctuation {
			flush()
			tokens = append(tokens, string(r))
			currentClass = diffTokenClassUnknown
			continue
		}
		if builder.Len() > 0 && class != currentClass {
			flush()
		}
		builder.WriteRune(r)
		currentClass = class
	}
	flush()
	return tokens
}

type diffTokenClass uint8

const (
	diffTokenClassUnknown diffTokenClass = iota
	diffTokenClassWord
	diffTokenClassWhitespace
	diffTokenClassPunctuation
)

func classifyDiffRune(r rune) diffTokenClass {
	switch {
	case r == '_' || (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z'):
		return diffTokenClassWord
	case r == ' ' || r == '\t':
		return diffTokenClassWhitespace
	default:
		return diffTokenClassPunctuation
	}
}

func appendTextChunk(chunks []domain.RevisionDiffTextChunk, operation, text string) []domain.RevisionDiffTextChunk {
	if text == "" {
		return chunks
	}
	if len(chunks) == 0 || chunks[len(chunks)-1].Operation != operation {
		return append(chunks, domain.RevisionDiffTextChunk{Operation: operation, Text: text})
	}
	chunks[len(chunks)-1].Text += text
	return chunks
}
