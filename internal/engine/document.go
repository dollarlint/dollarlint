package engine

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/pelletier/go-toml/v2"
	"github.com/tailscale/hujson"
	json5 "github.com/titanous/json5"
	"gopkg.in/yaml.v3"
)

const (
	DocumentFormatJSON      = "json"
	DocumentFormatJSONC     = "jsonc"
	DocumentFormatJSON5     = "json5"
	DocumentFormatJSONLines = "jsonl"
	DocumentFormatYAML      = "yaml"
	DocumentFormatTOML      = "toml"

	documentFormatJSONAuto = "json-auto"
)

type Document struct {
	Path          string
	RelativePath  string
	Format        string
	Data          any
	Schema        string
	SchemaSource  string
	SourceMap     SourceMap
	LineDocuments []LineDocument
	ParseErrors   []DocumentParseError

	azureRefs         []azureARMResourceRef
	azureRefsComputed bool
}

type LineDocument struct {
	Line      int
	Data      any
	SourceMap SourceMap
}

type DocumentParseError struct {
	Line    int
	Column  int
	Message string
}

func (d *Document) isLineDelimited() bool {
	return d != nil && d.Format == DocumentFormatJSONLines
}

func (d *Document) isEmptyLineDelimitedDocument() bool {
	return d.isLineDelimited() && len(d.LineDocuments) == 0
}

// azureResourceRefs returns the Azure ARM resource refs declared by this
// document, computing them lazily and caching the result. Safe to call
// repeatedly during a single Lint pass (Documents are not shared across
// goroutines).
func (d *Document) azureResourceRefs() []azureARMResourceRef {
	if d == nil {
		return nil
	}
	if !d.azureRefsComputed {
		d.azureRefs = collectAzureARMResourceRefs(d.Data)
		d.azureRefsComputed = true
	}
	return d.azureRefs
}

func ParseDocument(file DiscoveredFile) (*Document, error) {
	return parseDocument(file, DefaultConfig().Parsing, false)
}

func parseDocument(file DiscoveredFile, cfg ParsingConfig, sourceLocations bool) (*Document, error) {
	raw, err := os.ReadFile(file.Path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", file.Path, err)
	}
	raw = stripUTF8BOM(raw)
	format, err := formatForPath(file, cfg)
	if err != nil {
		return nil, err
	}
	effectiveFormat, data, lineDocuments, parseErrors, err := parseDocumentData(raw, format)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", file.Path, err)
	}
	schema, source := extractSchema(raw, data, effectiveFormat)
	document := &Document{
		Path:          file.Path,
		RelativePath:  file.RelativePath,
		Format:        effectiveFormat,
		Data:          data,
		Schema:        schema,
		SchemaSource:  source,
		LineDocuments: lineDocuments,
		ParseErrors:   parseErrors,
	}
	if sourceLocations {
		attachSourceMapFromRaw(document, raw)
	}
	return document, nil
}

func AttachSourceMap(document *Document) {
	if document == nil || document.SourceMap != nil {
		return
	}
	raw, err := os.ReadFile(document.Path)
	if err != nil {
		return
	}
	raw = stripUTF8BOM(raw)
	attachSourceMapFromRaw(document, raw)
}

func attachSourceMapFromRaw(document *Document, raw []byte) {
	if document == nil || document.SourceMap != nil {
		return
	}
	if document.isLineDelimited() {
		lineMaps := buildJSONLinesSourceMaps(raw)
		for i := range document.LineDocuments {
			document.LineDocuments[i].SourceMap = lineMaps[document.LineDocuments[i].Line]
		}
		document.SourceMap = buildJSONLinesSourceMap(raw)
		return
	}
	document.SourceMap = safeBuildSourceMap(raw, document.Format)
}

func formatForPath(file DiscoveredFile, cfg ParsingConfig) (string, error) {
	switch strings.ToLower(filepath.Ext(file.Path)) {
	case ".json":
		return jsonFormatForPath(file, cfg)
	case ".jsonc":
		return DocumentFormatJSONC, nil
	case ".json5":
		return DocumentFormatJSON5, nil
	case ".jsonl", ".ndjson":
		return DocumentFormatJSONLines, nil
	case ".yaml", ".yml":
		return DocumentFormatYAML, nil
	case ".toml":
		return DocumentFormatTOML, nil
	default:
		return "", fmt.Errorf("unsupported file format %s", filepath.Ext(file.Path))
	}
}

func jsonFormatForPath(file DiscoveredFile, cfg ParsingConfig) (string, error) {
	mode, err := jsonParsingMode(cfg)
	if err != nil {
		return "", err
	}
	switch mode {
	case JSONParsingJSONC:
		return DocumentFormatJSONC, nil
	case JSONParsingAuto:
		return documentFormatJSONAuto, nil
	}
	return DocumentFormatJSON, nil
}

func parseDocumentData(raw []byte, format string) (string, any, []LineDocument, []DocumentParseError, error) {
	if format == DocumentFormatJSONLines {
		data, lineDocuments, parseErrors, err := decodeJSONLines(raw)
		return format, data, lineDocuments, parseErrors, err
	}
	if format == documentFormatJSONAuto {
		data, err := decodeDocument(raw, DocumentFormatJSON)
		if err == nil {
			return DocumentFormatJSON, data, nil, nil, nil
		}
		jsonErr := err
		data, err = decodeDocument(raw, DocumentFormatJSONC)
		if err == nil {
			return DocumentFormatJSONC, data, nil, nil, nil
		}
		if strings.Contains(jsonErr.Error(), "multiple JSON values") {
			return DocumentFormatJSON, nil, nil, nil, jsonErr
		}
		return DocumentFormatJSONC, nil, nil, nil, err
	}
	data, err := decodeDocument(raw, format)
	return format, data, nil, nil, err
}

func decodeDocument(raw []byte, format string) (any, error) {
	raw = stripUTF8BOM(raw)
	switch format {
	case DocumentFormatJSON:
		return decodeJSON(raw)
	case DocumentFormatJSONC:
		return decodeJSONC(raw)
	case DocumentFormatJSON5:
		return decodeJSON5(raw)
	case DocumentFormatYAML:
		var value any
		if err := yaml.Unmarshal(raw, &value); err != nil {
			return nil, err
		}
		return toJSONValue(normalizeYAML(value))
	case DocumentFormatTOML:
		var value map[string]any
		if err := toml.Unmarshal(raw, &value); err != nil {
			return nil, err
		}
		return toJSONValue(value)
	default:
		return nil, fmt.Errorf("unsupported document format %s", format)
	}
}

func stripUTF8BOM(raw []byte) []byte {
	if len(raw) >= 3 && raw[0] == 0xef && raw[1] == 0xbb && raw[2] == 0xbf {
		return raw[3:]
	}
	return raw
}

func decodeJSON(raw []byte) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	if err := ensureSingleJSONValue(decoder); err != nil {
		return nil, err
	}
	return value, nil
}

func ensureSingleJSONValue(decoder *json.Decoder) error {
	var extra any
	err := decoder.Decode(&extra)
	if err == io.EOF {
		return nil
	}
	if err != nil {
		return err
	}
	return fmt.Errorf("multiple JSON values")
}

func decodeJSONC(raw []byte) (any, error) {
	standardized, err := hujson.Standardize(raw)
	if err != nil {
		return nil, err
	}
	return decodeJSON(standardized)
}

func decodeJSON5(raw []byte) (any, error) {
	decoder := json5.NewDecoder(bytes.NewReader(raw))
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	var extra any
	err := decoder.Decode(&extra)
	if err != io.EOF {
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("multiple JSON values")
	}
	return toJSONValue(value)
}

func decodeJSONLines(raw []byte) (any, []LineDocument, []DocumentParseError, error) {
	var values []any
	var documents []LineDocument
	var parseErrors []DocumentParseError
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
		value, err := decodeJSON(line)
		if err != nil {
			parseErrors = append(parseErrors, DocumentParseError{
				Line:    lineNo,
				Column:  jsonErrorColumn(line, err),
				Message: fmt.Sprintf("parse line %d: %v", lineNo, err),
			})
			continue
		}
		values = append(values, value)
		documents = append(documents, LineDocument{Line: lineNo, Data: value})
	}
	return values, documents, parseErrors, nil
}

func jsonErrorColumn(raw []byte, err error) int {
	var syntaxErr *json.SyntaxError
	if !errors.As(err, &syntaxErr) || syntaxErr.Offset <= 0 {
		return 1
	}
	pos := positionAtOffset(newLineIndex(raw), int(syntaxErr.Offset)-1)
	return pos.Column
}

func toJSONValue(value any) (any, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return decodeJSON(data)
}

func normalizeYAML(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, value := range typed {
			out[key] = normalizeYAML(value)
		}
		return out
	case map[any]any:
		out := make(map[string]any, len(typed))
		for key, value := range typed {
			out[fmt.Sprint(key)] = normalizeYAML(value)
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for i, value := range typed {
			out[i] = normalizeYAML(value)
		}
		return out
	default:
		return value
	}
}

func extractSchema(raw []byte, data any, format string) (string, string) {
	switch format {
	case DocumentFormatYAML:
		if schema := yamlSchemaDirective(raw); schema != "" {
			return schema, "yaml-language-server"
		}
	case DocumentFormatTOML:
		if schema := tomlSchemaDirective(raw); schema != "" {
			return schema, "taplo-directive"
		}
	}
	if schema := rootSchema(data); schema != "" {
		return schema, "$schema"
	}
	return "", ""
}

func rootSchema(data any) string {
	root, ok := data.(map[string]any)
	if !ok {
		return ""
	}
	schema, ok := root["$schema"].(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(schema)
}

func yamlSchemaDirective(raw []byte) string {
	for _, line := range firstLines(raw, 25) {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "#") {
			continue
		}
		body := strings.TrimSpace(strings.TrimPrefix(trimmed, "#"))
		if !strings.HasPrefix(body, "yaml-language-server:") {
			continue
		}
		fields := strings.Fields(strings.TrimSpace(strings.TrimPrefix(body, "yaml-language-server:")))
		for _, field := range fields {
			if schema, ok := strings.CutPrefix(field, "$schema="); ok {
				return strings.TrimSpace(schema)
			}
		}
	}
	return ""
}

func tomlSchemaDirective(raw []byte) string {
	for _, line := range firstLines(raw, 25) {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || (strings.HasPrefix(trimmed, "#") && !strings.HasPrefix(trimmed, "#:schema")) {
			continue
		}
		if schema, ok := strings.CutPrefix(trimmed, "#:schema"); ok {
			if schema != "" && strings.TrimLeftFunc(schema, unicode.IsSpace) == schema {
				continue
			}
			return strings.TrimSpace(schema)
		}
		return ""
	}
	return ""
}

// firstLines returns up to limit leading lines from raw without materializing
// the full slice of lines for large inputs.
func firstLines(raw []byte, limit int) []string {
	if limit <= 0 {
		return nil
	}
	lines := make([]string, 0, limit)
	start := 0
	for len(lines) < limit && start <= len(raw) {
		end := bytes.IndexByte(raw[start:], '\n')
		if end < 0 {
			lines = append(lines, string(raw[start:]))
			break
		}
		line := raw[start : start+end]
		if n := len(line); n > 0 && line[n-1] == '\r' {
			line = line[:n-1]
		}
		lines = append(lines, string(line))
		start += end + 1
	}
	return lines
}
