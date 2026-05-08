package dollarlint

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDurationEncoding(t *testing.T) {
	duration := NewDuration(1500 * time.Millisecond)
	text, err := duration.MarshalText()
	if err != nil {
		t.Fatalf("MarshalText: %v", err)
	}
	if string(text) != "1.5s" {
		t.Fatalf("duration text = %q", text)
	}
	var fromText Duration
	if err := fromText.UnmarshalText([]byte("2s")); err != nil {
		t.Fatalf("UnmarshalText: %v", err)
	}
	if fromText.Duration != 2*time.Second {
		t.Fatalf("fromText = %v", fromText.Duration)
	}
	data, err := json.Marshal(duration)
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}
	if string(data) != `"1.5s"` {
		t.Fatalf("duration json = %s", data)
	}
	var fromString Duration
	if err := json.Unmarshal([]byte(`"3s"`), &fromString); err != nil {
		t.Fatalf("UnmarshalJSON string: %v", err)
	}
	var fromNumber Duration
	if err := json.Unmarshal([]byte(`1.25`), &fromNumber); err != nil {
		t.Fatalf("UnmarshalJSON number: %v", err)
	}
	if fromNumber.Duration != 1250*time.Millisecond {
		t.Fatalf("fromNumber = %v", fromNumber.Duration)
	}
	if err := fromText.UnmarshalText([]byte("nope")); err == nil {
		t.Fatalf("expected invalid text duration error")
	}
	if err := json.Unmarshal([]byte(`false`), &fromText); err == nil {
		t.Fatalf("expected invalid json duration error")
	}
	zero, err := NewDuration(0).MarshalText()
	if err != nil || string(zero) != "0s" {
		t.Fatalf("zero duration = %q, %v", zero, err)
	}
}

func TestLoadConfigDefaultsAndFormats(t *testing.T) {
	dir := t.TempDir()
	cfg, path, err := LoadConfig(dir, "")
	if err != nil {
		t.Fatalf("LoadConfig no file: %v", err)
	}
	if path != "" || cfg.Version != 1 || !remoteFetchEnabled(cfg.Schema) {
		t.Fatalf("unexpected defaults: path=%q cfg=%+v", path, cfg)
	}
	writeFile(t, filepath.Join(dir, ".dollarlint.yaml"), `
version: 1
schema:
  fetchRemote: false
  maxDepth: 3
timeouts:
  fetch: 2s
ignore:
  - file: "*.json"
    keyword: type
`)
	cfg, path, err = LoadConfig(dir, "")
	if err != nil {
		t.Fatalf("LoadConfig yaml: %v", err)
	}
	if filepath.Base(path) != ".dollarlint.yaml" {
		t.Fatalf("path = %s", path)
	}
	if remoteFetchEnabled(cfg.Schema) || cfg.Schema.MaxDepth != 3 || cfg.Timeouts.Fetch.Duration != 2*time.Second {
		t.Fatalf("yaml cfg not decoded/defaulted: %+v", cfg)
	}
	if len(cfg.Discovery.Include) == 0 || len(cfg.Ignore) != 1 {
		t.Fatalf("defaults or ignore missing: %+v", cfg)
	}
	tomlPath := filepath.Join(dir, "custom.toml")
	writeFile(t, tomlPath, `
version = 1
[schema]
maxDepth = 4
[[schema.associations]]
file = "*.toml"
schema = "./schema.json"
`)
	cfg, path, err = LoadConfig(dir, "custom.toml")
	if err != nil {
		t.Fatalf("LoadConfig toml: %v", err)
	}
	if path != tomlPath || cfg.Schema.MaxDepth != 4 || len(cfg.Schema.Associations) != 1 {
		t.Fatalf("toml cfg = %s %+v", path, cfg)
	}
	jsonPath := filepath.Join(dir, "custom.json")
	writeFile(t, jsonPath, `{"output":{"json":true}}`)
	cfg, _, err = LoadConfig(dir, jsonPath)
	if err != nil {
		t.Fatalf("LoadConfig json: %v", err)
	}
	if !cfg.Output.JSON {
		t.Fatalf("json output config not decoded")
	}
}

func TestLoadConfigErrors(t *testing.T) {
	dir := t.TempDir()
	if _, _, err := LoadConfig(dir, "missing.yaml"); err == nil {
		t.Fatalf("expected missing explicit config error")
	}
	badYAML := filepath.Join(dir, ".dollarlint.yaml")
	writeFile(t, badYAML, "schema: [")
	if _, _, err := LoadConfig(dir, ""); err == nil {
		t.Fatalf("expected bad yaml error")
	}
	unsupported := filepath.Join(dir, "config.ini")
	writeFile(t, unsupported, "")
	var cfg Config
	if err := decodeConfig(unsupported, nil, &cfg); err == nil {
		t.Fatalf("expected unsupported config format")
	}
	blockingDir := filepath.Join(dir, "block")
	if err := os.Mkdir(blockingDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if _, _, err := LoadConfig(blockingDir, ""); err != nil && !strings.Contains(err.Error(), "config") {
		t.Fatalf("unexpected config search error: %v", err)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(strings.TrimLeft(content, "\n")), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
