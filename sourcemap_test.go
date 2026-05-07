package dollarlint

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestSourceMapJSONPositions(t *testing.T) {
	sourceMap, err := buildJSONSourceMap([]byte("{\n  \"name\": \"ok\",\n  \"arr\": [true, null, 3],\n  \"a/b~c\": {\"x\": false}\n}"))
	if err != nil {
		t.Fatalf("buildJSONSourceMap: %v", err)
	}
	assertPosition(t, sourceMap, "/", 1, 1)
	assertPosition(t, sourceMap, "/name", 2, 11)
	assertPosition(t, sourceMap, "/arr", 3, 10)
	assertPosition(t, sourceMap, "/arr/0", 3, 11)
	assertPosition(t, sourceMap, "/arr/1", 3, 17)
	assertPosition(t, sourceMap, "/arr/2", 3, 23)
	assertPosition(t, sourceMap, "/a~1b~0c/x", 4, 18)
	if _, err := buildJSONSourceMap([]byte("{")); err == nil {
		t.Fatalf("expected bad json source map error")
	}
}

func TestSourceMapYAMLPositions(t *testing.T) {
	sourceMap, err := buildYAMLSourceMap([]byte("name: ok\nitems:\n  - one\n  - two\nnested:\n  child: true\n"))
	if err != nil {
		t.Fatalf("buildYAMLSourceMap: %v", err)
	}
	assertPosition(t, sourceMap, "/", 1, 1)
	assertPosition(t, sourceMap, "/name", 1, 7)
	assertPosition(t, sourceMap, "/items/1", 4, 5)
	assertPosition(t, sourceMap, "/nested/child", 6, 10)
	if _, err := buildYAMLSourceMap([]byte(":\n")); err == nil {
		t.Fatalf("expected bad yaml source map error")
	}
}

func TestSourceMapTOMLPositions(t *testing.T) {
	sourceMap, err := buildTOMLSourceMap([]byte("[server]\nname = \"ok\"\nports = [1, 2]\ninline = { enabled = true }\n"))
	if err != nil {
		t.Fatalf("buildTOMLSourceMap: %v", err)
	}
	assertPosition(t, sourceMap, "/", 1, 1)
	assertPosition(t, sourceMap, "/server", 1, 2)
	assertPosition(t, sourceMap, "/server/name", 2, 8)
	assertPosition(t, sourceMap, "/server/ports/1", 3, 13)
	assertPosition(t, sourceMap, "/server/inline/enabled", 4, 12)
	if _, err := buildTOMLSourceMap([]byte("=")); err == nil {
		t.Fatalf("expected bad toml source map error")
	}
}

func TestSourceMapHelpers(t *testing.T) {
	sourceMap := SourceMap{"/parent": {Line: 5, Column: 3}}
	pos, ok := sourceMap.Position("/parent/missing")
	if !ok || pos.Line != 5 || pos.Column != 3 {
		t.Fatalf("parent fallback = %+v ok=%v", pos, ok)
	}
	if _, ok := SourceMap(nil).Position("/anything"); ok {
		t.Fatalf("nil source map should not resolve")
	}
	if _, ok := (SourceMap{"/other": {Line: 1, Column: 1}}).Position("/missing/path"); ok {
		t.Fatalf("missing position should not resolve")
	}
	if normalizePointer("") != "/" || normalizePointer("x") != "/x" || normalizePointer("/x") != "/x" {
		t.Fatalf("normalizePointer mismatch")
	}
	if got := pointerFromParts([]string{"a/b", "c~d"}); got != "/a~1b/c~0d" {
		t.Fatalf("pointerFromParts = %s", got)
	}
	if pos := positionAtOffset([]int{0}, -10); pos.Line != 1 || pos.Column != 1 {
		t.Fatalf("negative offset = %+v", pos)
	}
	if _, err := buildSourceMap(nil, "nope"); err == nil {
		t.Fatalf("expected unsupported source map format")
	}
	setSourcePosition(sourceMap, "/zero", SourcePosition{})
	assertPosition(t, sourceMap, "/zero", 1, 1)
	if pos := tomlNodePosition(nil, nil); pos.Line != 1 || pos.Column != 1 {
		t.Fatalf("nil toml node position = %+v", pos)
	}
	walkYAMLNode(nil, sourceMap, "/nil")
	walkTOMLValue(nil, nil, nil, sourceMap)
	if tokenStartOffset([]byte("x"), 5, true) != 0 {
		t.Fatalf("tokenStartOffset should clamp end")
	}
	if tokenStartOffset([]byte(`"a\"b"`), 6, "") != 0 {
		t.Fatalf("escaped string token should start at zero")
	}
	if tokenStartOffset(nil, 0, json.Delim('{')) != 0 {
		t.Fatalf("empty delimiter start should be zero")
	}
}

func TestJSONWalkerErrorBranches(t *testing.T) {
	decoder := json.NewDecoder(bytes.NewReader(nil))
	if err := walkJSONValue(decoder, nil, []int{0}, SourceMap{}, "/"); err == nil {
		t.Fatalf("expected empty decoder error")
	}
	decoder = json.NewDecoder(bytes.NewReader([]byte(`{"a": }`)))
	if err := walkJSONValue(decoder, []byte(`{"a": }`), []int{0}, SourceMap{}, "/"); err == nil {
		t.Fatalf("expected recursive value error")
	}
	decoder = json.NewDecoder(bytes.NewReader([]byte(`[1, ]`)))
	if err := walkJSONValue(decoder, []byte(`[1, ]`), []int{0}, SourceMap{}, "/"); err == nil {
		t.Fatalf("expected array value error")
	}
}

func assertPosition(t *testing.T, sourceMap SourceMap, pointer string, line, column int) {
	t.Helper()
	pos, ok := sourceMap.Position(pointer)
	if !ok {
		t.Fatalf("missing position for %s", pointer)
	}
	if pos.Line != line || pos.Column != column {
		t.Fatalf("%s position = %+v, want %d:%d", pointer, pos, line, column)
	}
}
