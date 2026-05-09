package engine

import (
	"bytes"
	"strconv"
	"strings"
	"unicode"
)

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
