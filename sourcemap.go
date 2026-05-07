package dollarlint

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path"
	"strconv"
	"strings"
	"unicode"

	"github.com/pelletier/go-toml/v2/unstable"
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
			if unicode.IsSpace(rune(raw[i])) || strings.ContainsRune("{[,:", rune(raw[i])) {
				return i + 1
			}
		}
	}
	return 0
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
	var parser unstable.Parser
	parser.Reset(raw)
	sourceMap := SourceMap{"/": {Line: 1, Column: 1}}
	var currentTable []string
	for parser.NextExpression() {
		expr := parser.Expression()
		switch expr.Kind {
		case unstable.Table, unstable.ArrayTable:
			currentTable = tomlKeyParts(expr)
			setSourcePosition(sourceMap, pointerFromParts(currentTable), tomlKeyPosition(&parser, expr))
		case unstable.KeyValue:
			walkTOMLKeyValue(&parser, expr, currentTable, sourceMap)
		}
	}
	if err := parser.Error(); err != nil {
		return nil, err
	}
	return sourceMap, nil
}

func walkTOMLKeyValue(parser *unstable.Parser, node *unstable.Node, prefix []string, sourceMap SourceMap) {
	keyParts := append(append([]string(nil), prefix...), tomlKeyParts(node)...)
	pointer := pointerFromParts(keyParts)
	value := node.Value()
	setSourcePosition(sourceMap, pointer, tomlBestPosition(parser, value, node))
	walkTOMLValue(parser, value, keyParts, sourceMap)
}

func walkTOMLValue(parser *unstable.Parser, node *unstable.Node, parts []string, sourceMap SourceMap) {
	if node == nil {
		return
	}
	switch node.Kind {
	case unstable.Array:
		it := node.Children()
		index := 0
		for it.Next() {
			child := it.Node()
			childParts := append(append([]string(nil), parts...), strconv.Itoa(index))
			setSourcePosition(sourceMap, pointerFromParts(childParts), tomlNodePosition(parser, child))
			walkTOMLValue(parser, child, childParts, sourceMap)
			index++
		}
	case unstable.InlineTable:
		it := node.Children()
		for it.Next() {
			walkTOMLKeyValue(parser, it.Node(), parts, sourceMap)
		}
	}
}

func tomlKeyParts(node *unstable.Node) []string {
	var parts []string
	it := node.Key()
	for it.Next() {
		parts = append(parts, string(it.Node().Data))
	}
	return parts
}

func tomlKeyPosition(parser *unstable.Parser, node *unstable.Node) SourcePosition {
	it := node.Key()
	it.Next()
	return tomlNodePosition(parser, it.Node())
}

func tomlBestPosition(parser *unstable.Parser, preferred, fallback *unstable.Node) SourcePosition {
	pos := tomlNodePosition(parser, preferred)
	if pos.Line != 1 || pos.Column != 1 || preferred == nil || preferred.Raw.Length > 0 {
		return pos
	}
	return tomlNodePosition(parser, fallback)
}

func tomlNodePosition(parser *unstable.Parser, node *unstable.Node) SourcePosition {
	if node == nil {
		return SourcePosition{Line: 1, Column: 1}
	}
	shape := parser.Shape(node.Raw)
	return SourcePosition{Line: shape.Start.Line, Column: shape.Start.Column}
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
	if offset < 0 {
		offset = 0
	}
	line := 0
	for i := len(lineStarts) - 1; i >= 0; i-- {
		if lineStarts[i] <= offset {
			line = i
			break
		}
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
