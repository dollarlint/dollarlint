package engine

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"gopkg.in/yaml.v3"
)

type SourcePosition struct {
	Line   int
	Column int
}

type SourceMap map[string]SourcePosition

func (m SourceMap) Position(pointer string) (SourcePosition, bool) {
	if m == nil {
		return SourcePosition{}, false
	}
	pointer = normalizePointer(pointer)
	for {
		if pos, ok := m[pointer]; ok {
			return pos, true
		}
		if pointer == "/" || pointer == "" {
			break
		}
		pointer = path.Dir(pointer)
	}
	return SourcePosition{}, false
}

func buildSourceMap(raw []byte, format string) (SourceMap, error) {
	switch format {
	case DocumentFormatJSON:
		return buildJSONSourceMap(raw)
	case DocumentFormatYAML:
		return buildYAMLSourceMap(raw)
	case DocumentFormatTOML:
		return buildTOMLSourceMap(raw)
	default:
		return nil, fmt.Errorf("unsupported source map format %s", format)
	}
}

func safeBuildSourceMap(raw []byte, format string) (sourceMap SourceMap) {
	return safeSourceMap(func() (SourceMap, error) {
		return buildSourceMap(raw, format)
	})
}

func safeSourceMap(build func() (SourceMap, error)) (sourceMap SourceMap) {
	defer func() {
		if recover() != nil {
			sourceMap = nil
		}
	}()
	sourceMap, err := build()
	if err != nil {
		return nil
	}
	return sourceMap
}

func buildJSONSourceMap(raw []byte) (SourceMap, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	lineIndex := newLineIndex(raw)
	sourceMap := SourceMap{}
	if err := walkJSONValue(decoder, raw, lineIndex, sourceMap, "/"); err != nil {
		return nil, err
	}
	return sourceMap, nil
}

func walkJSONValue(decoder *json.Decoder, raw []byte, lineIndex []int, sourceMap SourceMap, pointer string) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	sourceMap[normalizePointer(pointer)] = positionAtOffset(lineIndex, tokenStartOffset(raw, int(decoder.InputOffset()), token))
	if delim, ok := token.(json.Delim); ok {
		switch delim {
		case '{':
			for decoder.More() {
				keyToken, _ := decoder.Token()
				key := keyToken.(string)
				if err := walkJSONValue(decoder, raw, lineIndex, sourceMap, joinPointer(pointer, key)); err != nil {
					return err
				}
			}
			_, err = decoder.Token()
			return err
		case '[':
			index := 0
			for decoder.More() {
				if err := walkJSONValue(decoder, raw, lineIndex, sourceMap, joinPointer(pointer, strconv.Itoa(index))); err != nil {
					return err
				}
				index++
			}
			_, err = decoder.Token()
			return err
		}
	}
	return nil
}

func tokenStartOffset(raw []byte, end int, token any) int {
	if end > len(raw) {
		end = len(raw)
	}
	switch token.(type) {
	case json.Delim:
		if end > 0 {
			return end - 1
		}
	case string:
		sawClosingQuote := false
		for i := end - 1; i >= 0; i-- {
			if raw[i] != '"' {
				continue
			}
			escapes := 0
			for j := i - 1; j >= 0 && raw[j] == '\\'; j-- {
				escapes++
			}
			if escapes%2 == 0 && !sawClosingQuote {
				sawClosingQuote = true
				continue
			}
			if escapes%2 == 0 {
				return i
			}
		}
	default:
		for i := end - 1; i >= 0; i-- {
			if isJSONTokenBoundary(raw[i]) {
				return i + 1
			}
		}
	}
	return 0
}

// isJSONTokenBoundary reports whether b is ASCII whitespace or a JSON
// structural character that can precede a primitive value.
func isJSONTokenBoundary(b byte) bool {
	switch b {
	case ' ', '\t', '\n', '\r', '\v', '\f', '{', '[', ',', ':':
		return true
	}
	return false
}

func buildYAMLSourceMap(raw []byte) (SourceMap, error) {
	var root yaml.Node
	if err := yaml.Unmarshal(raw, &root); err != nil {
		return nil, err
	}
	sourceMap := SourceMap{}
	node := &root
	if root.Kind == yaml.DocumentNode && len(root.Content) > 0 {
		node = root.Content[0]
	}
	walkYAMLNode(node, sourceMap, "/")
	return sourceMap, nil
}

func walkYAMLNode(node *yaml.Node, sourceMap SourceMap, pointer string) {
	if node == nil {
		return
	}
	setSourcePosition(sourceMap, pointer, SourcePosition{Line: node.Line, Column: node.Column})
	switch node.Kind {
	case yaml.MappingNode:
		for i := 0; i+1 < len(node.Content); i += 2 {
			key := node.Content[i]
			value := node.Content[i+1]
			walkYAMLNode(value, sourceMap, joinPointer(pointer, key.Value))
		}
	case yaml.SequenceNode:
		for i, child := range node.Content {
			walkYAMLNode(child, sourceMap, joinPointer(pointer, strconv.Itoa(i)))
		}
	}
}

func buildTOMLSourceMap(raw []byte) (SourceMap, error) {
	sourceMap := SourceMap{"/": {Line: 1, Column: 1}}
	var currentTable []string
	lineNo := 0
	for start := 0; start <= len(raw); {
		lineNo++
		end := bytes.IndexByte(raw[start:], '\n')
		var lineBytes []byte
		if end < 0 {
			lineBytes = raw[start:]
			start = len(raw) + 1
		} else {
			lineBytes = raw[start : start+end]
			start += end + 1
		}
		// Strip an optional trailing carriage return for CRLF inputs.
		if n := len(lineBytes); n > 0 && lineBytes[n-1] == '\r' {
			lineBytes = lineBytes[:n-1]
		}
		line := string(lineBytes)
		body := trimTOMLComment(line)
		trimmed := strings.TrimSpace(body)
		if trimmed == "" {
			continue
		}
		if table, col, ok := parseTOMLTable(trimmed, line); ok {
			currentTable = table
			setSourcePosition(sourceMap, pointerFromParts(table), SourcePosition{Line: lineNo, Column: col})
			continue
		}
		key, value, valueCol, ok := splitTOMLKeyValue(body)
		if !ok {
			continue
		}
		keyParts := append(append([]string(nil), currentTable...), splitTOMLKey(key)...)
		pointer := pointerFromParts(keyParts)
		setSourcePosition(sourceMap, pointer, SourcePosition{Line: lineNo, Column: valueCol})
		scanTOMLValuePositions(sourceMap, keyParts, value, lineNo, valueCol)
	}
	return sourceMap, nil
}

func parseTOMLTable(trimmed, original string) ([]string, int, bool) {
	arrayTable := strings.HasPrefix(trimmed, "[[")
	if !strings.HasPrefix(trimmed, "[") {
		return nil, 0, false
	}
	prefix := "["
	suffix := "]"
	if arrayTable {
		prefix = "[["
		suffix = "]]"
	}
	if !strings.HasSuffix(trimmed, suffix) {
		return nil, 0, false
	}
	name := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(trimmed, prefix), suffix))
	if name == "" {
		return nil, 0, false
	}
	col := strings.Index(original, name) + 1
	if col <= 0 {
		col = 1
	}
	return splitTOMLKey(name), col, true
}

func splitTOMLKeyValue(line string) (string, string, int, bool) {
	eq := indexTOMLTopLevel(line, '=')
	if eq < 0 {
		return "", "", 0, false
	}
	key := strings.TrimSpace(line[:eq])
	valueStart := eq + 1
	for valueStart < len(line) && unicode.IsSpace(rune(line[valueStart])) {
		valueStart++
	}
	if key == "" || valueStart >= len(line) {
		return "", "", 0, false
	}
	return key, line[valueStart:], valueStart + 1, true
}

func scanTOMLValuePositions(sourceMap SourceMap, parts []string, value string, line, column int) {
	value = strings.TrimSpace(value)
	switch {
	case strings.HasPrefix(value, "["):
		for i, offset := range tomlArrayElementOffsets(value) {
			childParts := append(append([]string(nil), parts...), strconv.Itoa(i))
			setSourcePosition(sourceMap, pointerFromParts(childParts), SourcePosition{Line: line, Column: column + offset})
		}
	case strings.HasPrefix(value, "{"):
		for _, kv := range tomlInlineTableEntries(value) {
			childParts := append(append([]string(nil), parts...), splitTOMLKey(kv.key)...)
			setSourcePosition(sourceMap, pointerFromParts(childParts), SourcePosition{Line: line, Column: column + kv.valueOffset})
		}
	}
}

func trimTOMLComment(line string) string {
	inString := false
	var quote byte
	escaped := false
	for i := 0; i < len(line); i++ {
		ch := line[i]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' && quote == '"' {
				escaped = true
				continue
			}
			if ch == quote {
				inString = false
			}
			continue
		}
		if ch == '"' || ch == '\'' {
			inString = true
			quote = ch
			continue
		}
		if ch == '#' {
			return line[:i]
		}
	}
	return line
}

func splitTOMLKey(key string) []string {
	var parts []string
	for _, part := range splitTOMLTopLevel(key, '.') {
		part = strings.TrimSpace(part)
		part = strings.Trim(part, `"'`)
		if part != "" {
			parts = append(parts, part)
		}
	}
	return parts
}

func tomlArrayElementOffsets(value string) []int {
	inner := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(value, "["), "]"))
	if inner == "" {
		return nil
	}
	var offsets []int
	base := strings.Index(value, inner)
	for _, part := range splitTOMLTopLevel(inner, ',') {
		trimmed := strings.TrimLeftFunc(part, unicode.IsSpace)
		if trimmed == "" {
			base += len(part) + 1
			continue
		}
		offsets = append(offsets, base+len(part)-len(trimmed))
		base += len(part) + 1
	}
	return offsets
}

type tomlInlineEntry struct {
	key         string
	valueOffset int
}

func tomlInlineTableEntries(value string) []tomlInlineEntry {
	inner := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(value, "{"), "}"))
	if inner == "" {
		return nil
	}
	base := strings.Index(value, inner)
	var entries []tomlInlineEntry
	for _, part := range splitTOMLTopLevel(inner, ',') {
		eq := indexTOMLTopLevel(part, '=')
		if eq < 0 {
			base += len(part) + 1
			continue
		}
		key := strings.TrimSpace(part[:eq])
		valueStart := eq + 1
		for valueStart < len(part) && unicode.IsSpace(rune(part[valueStart])) {
			valueStart++
		}
		if key != "" && valueStart < len(part) {
			entries = append(entries, tomlInlineEntry{key: key, valueOffset: base + valueStart})
		}
		base += len(part) + 1
	}
	return entries
}

func indexTOMLTopLevel(value string, needle rune) int {
	parts := splitTOMLTopLevel(value, needle)
	if len(parts) <= 1 {
		return -1
	}
	return len(parts[0])
}

func splitTOMLTopLevel(value string, sep rune) []string {
	var parts []string
	start := 0
	depth := 0
	inString := false
	var quote rune
	escaped := false
	for i, ch := range value {
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' && quote == '"' {
				escaped = true
				continue
			}
			if ch == quote {
				inString = false
			}
			continue
		}
		switch ch {
		case '"', '\'':
			inString = true
			quote = ch
		case '[', '{':
			depth++
		case ']', '}':
			if depth > 0 {
				depth--
			}
		default:
			if ch == sep && depth == 0 {
				parts = append(parts, value[start:i])
				start = i + len(string(ch))
			}
		}
	}
	return append(parts, value[start:])
}

func setSourcePosition(sourceMap SourceMap, pointer string, position SourcePosition) {
	if position.Line <= 0 {
		position.Line = 1
	}
	if position.Column <= 0 {
		position.Column = 1
	}
	sourceMap[normalizePointer(pointer)] = position
}

func newLineIndex(raw []byte) []int {
	starts := []int{0}
	for i, b := range raw {
		if b == '\n' {
			starts = append(starts, i+1)
		}
	}
	return starts
}

func positionAtOffset(lineStarts []int, offset int) SourcePosition {
	if len(lineStarts) == 0 {
		return SourcePosition{Line: 1, Column: 1}
	}
	if offset < 0 {
		offset = 0
	}
	// lineStarts is sorted ascending; find the greatest index whose start <= offset.
	line := sort.SearchInts(lineStarts, offset+1) - 1
	if line < 0 {
		return SourcePosition{Line: 1, Column: 1}
	}
	return SourcePosition{Line: line + 1, Column: offset - lineStarts[line] + 1}
}

func normalizePointer(pointer string) string {
	if pointer == "" {
		return "/"
	}
	if !strings.HasPrefix(pointer, "/") {
		return "/" + pointer
	}
	return pointer
}

func joinPointer(base, token string) string {
	base = normalizePointer(base)
	escaped := escapePointerToken(token)
	if base == "/" {
		return "/" + escaped
	}
	return base + "/" + escaped
}

func pointerFromParts(parts []string) string {
	pointer := "/"
	for _, part := range parts {
		pointer = joinPointer(pointer, part)
	}
	return pointer
}

func escapePointerToken(token string) string {
	token = strings.ReplaceAll(token, "~", "~0")
	return strings.ReplaceAll(token, "/", "~1")
}
