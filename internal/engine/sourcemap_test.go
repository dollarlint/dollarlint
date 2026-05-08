package engine

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
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
	assertPosition(t, sourceMap, "/server/inline/enabled", 4, 22)
	if sourceMap := safeBuildSourceMap([]byte("="), DocumentFormatTOML); sourceMap == nil {
		t.Fatalf("best-effort toml source map should still return root")
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
	if _, err := buildSourceMap([]byte(`{"ok": true}`), DocumentFormatJSON); err != nil {
		t.Fatalf("buildSourceMap json: %v", err)
	}
	if _, err := buildSourceMap([]byte(`ok: true`), DocumentFormatYAML); err != nil {
		t.Fatalf("buildSourceMap yaml: %v", err)
	}
	if sourceMap := safeBuildSourceMap(nil, "nope"); sourceMap != nil {
		t.Fatalf("unsupported safe source map should degrade to nil")
	}
	if sourceMap := safeSourceMap(func() (SourceMap, error) {
		panic("boom")
	}); sourceMap != nil {
		t.Fatalf("panic should degrade to nil source map")
	}
	setSourcePosition(sourceMap, "/zero", SourcePosition{})
	assertPosition(t, sourceMap, "/zero", 1, 1)
	walkYAMLNode(nil, sourceMap, "/nil")
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

func TestAttachSourceMap(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.json")
	writeFile(t, path, `{"name": "ok"}`)
	document := &Document{Path: path, Format: DocumentFormatJSON}
	AttachSourceMap(document)
	assertPosition(t, document.SourceMap, "/name", 1, 10)
	existing := document.SourceMap
	AttachSourceMap(document)
	if len(document.SourceMap) != len(existing) {
		t.Fatalf("existing source map should be left alone")
	}
	AttachSourceMap(nil)
	missing := &Document{Path: filepath.Join(dir, "missing.json"), Format: DocumentFormatJSON}
	AttachSourceMap(missing)
	if missing.SourceMap != nil {
		t.Fatalf("missing file should not attach source map")
	}
}

func TestTOMLScannerEdges(t *testing.T) {
	sourceMap, err := buildTOMLSourceMap([]byte("[[servers]] # comment\nempty = []\ninline = {}\nquoted = \"# not comment\"\narr = [ , \"x\" ]\ninvalid-inline = { nope }\n\"dotted.key\" = 1\n"))
	if err != nil {
		t.Fatalf("buildTOMLSourceMap edges: %v", err)
	}
	assertPosition(t, sourceMap, "/servers", 1, 3)
	assertPosition(t, sourceMap, "/servers/quoted", 4, 10)
	assertPosition(t, sourceMap, "/servers/arr/0", 5, 11)
	assertPosition(t, sourceMap, "/servers/dotted.key", 7, 16)
	if table, _, ok := parseTOMLTable("[missing", "[missing"); ok || table != nil {
		t.Fatalf("malformed table should not parse")
	}
	if table, _, ok := parseTOMLTable("[]", "[]"); ok || table != nil {
		t.Fatalf("empty table should not parse")
	}
	if table, col, ok := parseTOMLTable("[missing]", "[]"); !ok || table[0] != "missing" || col != 1 {
		t.Fatalf("table fallback column = %v %d %v", table, col, ok)
	}
	if _, _, _, ok := splitTOMLKeyValue("not a kv"); ok {
		t.Fatalf("line without equals should not parse as key-value")
	}
	if comment := trimTOMLComment(`name = "escaped \" # still string" # comment`); strings.Contains(comment, "comment") {
		t.Fatalf("escaped string comment trim = %q", comment)
	}
	if parts := splitTOMLTopLevel(`"a\".b".c`, '.'); len(parts) != 2 {
		t.Fatalf("escaped split parts = %+v", parts)
	}
	if entries := tomlInlineTableEntries("{}"); entries != nil {
		t.Fatalf("empty inline table entries = %+v", entries)
	}
	if offsets := tomlArrayElementOffsets("[]"); offsets != nil {
		t.Fatalf("empty array offsets = %+v", offsets)
	}
	if indexTOMLTopLevel("abc", '=') != -1 {
		t.Fatalf("missing top-level separator should be -1")
	}
}

func FuzzSafeBuildSourceMap(f *testing.F) {
	f.Add(DocumentFormatJSON, []byte(`{"name":"ok"}`))
	f.Add(DocumentFormatYAML, []byte("name: ok\n"))
	f.Add(DocumentFormatTOML, []byte("name = \"ok\"\n"))
	f.Fuzz(func(t *testing.T, format string, raw []byte) {
		switch format {
		case DocumentFormatJSON, DocumentFormatYAML, DocumentFormatTOML:
			_ = safeBuildSourceMap(raw, format)
		default:
			_ = safeBuildSourceMap(raw, DocumentFormatJSON)
		}
	})
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
