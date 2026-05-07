package dollarlint

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"gopkg.in/yaml.v3"
)

const (
	DocumentFormatJSON = "json"
	DocumentFormatYAML = "yaml"
	DocumentFormatTOML = "toml"
)

type Document struct {
	Path         string
	RelativePath string
	Format       string
	Data         any
	Schema       string
	SchemaSource string
}

func ParseDocument(file DiscoveredFile) (*Document, error) {
	raw, err := os.ReadFile(file.Path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", file.Path, err)
	}
	format, err := formatForPath(file.Path)
	if err != nil {
		return nil, err
	}
	data, err := decodeDocument(raw, format)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", file.Path, err)
	}
	schema, source := extractSchema(raw, data, format)
	return &Document{
		Path:         file.Path,
		RelativePath: file.RelativePath,
		Format:       format,
		Data:         data,
		Schema:       schema,
		SchemaSource: source,
	}, nil
}

func formatForPath(path string) (string, error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".json":
		return DocumentFormatJSON, nil
	case ".yaml", ".yml":
		return DocumentFormatYAML, nil
	case ".toml":
		return DocumentFormatTOML, nil
	default:
		return "", fmt.Errorf("unsupported file format %s", filepath.Ext(path))
	}
}

func decodeDocument(raw []byte, format string) (any, error) {
	switch format {
	case DocumentFormatJSON:
		return decodeJSON(raw)
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

func decodeJSON(raw []byte) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	return value, nil
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
			return strings.TrimSpace(schema)
		}
		return ""
	}
	return ""
}

func firstLines(raw []byte, limit int) []string {
	lines := strings.Split(string(raw), "\n")
	if len(lines) > limit {
		return lines[:limit]
	}
	return lines
}
