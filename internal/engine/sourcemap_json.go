package engine

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"
)

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

func buildJSONLinesSourceMap(raw []byte) SourceMap {
	lineMaps := buildJSONLinesSourceMaps(raw)
	sourceMap := SourceMap{}
	for lineNo, lineMap := range lineMaps {
		for pointer, position := range lineMap {
			linePointer := joinPointer("/", strconv.Itoa(lineNo))
			if pointer != "/" {
				linePointer += "/" + strings.TrimPrefix(pointer, "/")
			}
			sourceMap[linePointer] = position
		}
	}
	return sourceMap
}

func buildJSONLinesSourceMaps(raw []byte) map[int]SourceMap {
	lineMaps := map[int]SourceMap{}
	lineNo := 0
	for start := 0; start <= len(raw); {
		lineNo++
		end := bytes.IndexByte(raw[start:], '\n')
		var line []byte
		if end < 0 {
			line = raw[start:]
			start = len(raw) + 1
		} else {
			line = raw[start : start+end]
			start += end + 1
		}
		if n := len(line); n > 0 && line[n-1] == '\r' {
			line = line[:n-1]
		}
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		lineMap := safeBuildSourceMap(line, DocumentFormatJSON)
		if lineMap != nil {
			lineMaps[lineNo] = offsetSourceMap(lineMap, lineNo-1, 0)
		}
	}
	return lineMaps
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
