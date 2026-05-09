package engine

import (
	"fmt"
	"path"
	"sort"
	"strings"
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
	case DocumentFormatJSONC:
		return buildJSONLikeSourceMap(raw, false)
	case DocumentFormatJSON5:
		return buildJSONLikeSourceMap(raw, true)
	case DocumentFormatJSONLines:
		return buildJSONLinesSourceMap(raw), nil
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

func offsetSourceMap(sourceMap SourceMap, lineOffset, columnOffset int) SourceMap {
	if sourceMap == nil {
		return nil
	}
	shifted := SourceMap{}
	for pointer, position := range sourceMap {
		if position.Line <= 0 {
			continue
		}
		if position.Line == 1 {
			position.Column += columnOffset
		}
		position.Line += lineOffset
		shifted[pointer] = position
	}
	return shifted
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
