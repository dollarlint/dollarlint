package engine

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

func TestLoadConfigDefaultsAndTOML(t *testing.T) {
	dir := t.TempDir()
	cfg, path, err := LoadConfig(dir, "")
	if err != nil {
		t.Fatalf("LoadConfig no file: %v", err)
	}
	if path != "" || cfg.Version != 1 || !remoteFetchEnabled(cfg.Schema) {
		t.Fatalf("unexpected defaults: path=%q cfg=%+v", path, cfg)
	}
	writeFile(t, filepath.Join(dir, ".dollarlint.yaml"), "output:\n  json: true\n")
	cfg, path, err = LoadConfig(dir, "")
	if err != nil {
		t.Fatalf("LoadConfig ignored yaml: %v", err)
	}
	if path != "" || cfg.Output.JSON {
		t.Fatalf("non-toml config should be ignored: path=%q cfg=%+v", path, cfg)
	}
	writeFile(t, filepath.Join(dir, "dollarlint.toml"), "output = { json = true }\n")
	cfg, path, err = LoadConfig(dir, "")
	if err != nil {
		t.Fatalf("LoadConfig ignored no-dot toml: %v", err)
	}
	if path != "" || cfg.Output.JSON {
		t.Fatalf("no-dot toml config should be ignored: path=%q cfg=%+v", path, cfg)
	}
	writeFile(t, filepath.Join(dir, ".dollarlint.toml"), `
version = 1

[schema]
fetchRemote = false
fetchSchemaStore = false
allowedDomains = ["schemas.example.com"]
blockedDomains = ["bad.example.com"]
azureResourcePruning = false
maxDepth = 3

[schema.fetch]
retries = 4
retryMinWait = "100ms"
retryMaxWait = "1s"

[schema.schemaStore]
enabled = true
url = "./catalog.json"
strict = true

[timeouts]
fetch = "2s"

[[ignore]]
file = "*.json"
keyword = "type"
`)
	cfg, path, err = LoadConfig(dir, "")
	if err != nil {
		t.Fatalf("LoadConfig toml: %v", err)
	}
	if filepath.Base(path) != ".dollarlint.toml" {
		t.Fatalf("path = %s", path)
	}
	if remoteFetchEnabled(cfg.Schema) || cfg.Schema.MaxDepth != 3 || cfg.Timeouts.Fetch.Duration != 2*time.Second {
		t.Fatalf("toml cfg not decoded/defaulted: %+v", cfg)
	}
	if !cfg.Schema.SchemaStore.Enabled || cfg.Schema.SchemaStore.URL != "./catalog.json" || !cfg.Schema.SchemaStore.Strict {
		t.Fatalf("schemaStore not decoded: %+v", cfg.Schema.SchemaStore)
	}
	if fetchRetries(cfg.Schema.Fetch) != 4 || cfg.Schema.Fetch.RetryMinWait.Duration != 100*time.Millisecond || cfg.Schema.Fetch.RetryMaxWait.Duration != time.Second {
		t.Fatalf("fetch resilience config not decoded: %+v", cfg.Schema.Fetch)
	}
	if len(cfg.Schema.AllowedDomains) != 1 || cfg.Schema.AllowedDomains[0] != "schemas.example.com" || len(cfg.Schema.BlockedDomains) != 1 {
		t.Fatalf("domain policy not decoded: %+v", cfg.Schema)
	}
	if cfg.Schema.AzureResourcePruning == nil || *cfg.Schema.AzureResourcePruning {
		t.Fatalf("Azure resource pruning opt-out not decoded: %+v", cfg.Schema)
	}
	if len(cfg.Discovery.Include) == 0 || len(cfg.Ignore) != 1 {
		t.Fatalf("defaults or ignore missing: %+v", cfg)
	}
	customPath := filepath.Join(dir, "nested", ".dollarlint.toml")
	writeFile(t, customPath, `
version = 1
[schema]
maxDepth = 4
[[schema.associations]]
file = "*.toml"
schema = "./schema.json"
`)
	cfg, path, err = LoadConfig(dir, "nested/.dollarlint.toml")
	if err != nil {
		t.Fatalf("LoadConfig explicit toml: %v", err)
	}
	if path != customPath || cfg.Schema.MaxDepth != 4 || len(cfg.Schema.Associations) != 1 {
		t.Fatalf("toml cfg = %s %+v", path, cfg)
	}
	if cfg.Schema.SchemaStore.URL != defaultSchemaStoreCatalogURL {
		t.Fatalf("schemaStore default URL = %q", cfg.Schema.SchemaStore.URL)
	}
	if fetchRetries(cfg.Schema.Fetch) != 2 {
		t.Fatalf("fetch retry default = %+v", cfg.Schema.Fetch)
	}
	if fetchRetries(FetchConfig{}) != 0 {
		t.Fatalf("nil fetch retries should resolve to zero")
	}
	negativeRetries := -1
	if fetchRetries(FetchConfig{Retries: &negativeRetries}) != 0 {
		t.Fatalf("negative fetch retries should resolve to zero")
	}
	legacy := Config{Schema: SchemaConfig{SchemaStoreCatalogURL: "./legacy-catalog.json"}}
	legacyEnabled := true
	legacy.Schema.FetchSchemaStore = &legacyEnabled
	legacy.ApplyDefaults()
	if legacy.Schema.SchemaStore.URL != "./legacy-catalog.json" || !legacy.Schema.SchemaStore.Enabled {
		t.Fatalf("legacy schemastore config not defaulted: %+v", legacy.Schema)
	}
}

func TestLoadConfigErrors(t *testing.T) {
	dir := t.TempDir()
	if _, _, err := LoadConfig(dir, ".dollarlint.toml"); err == nil {
		t.Fatalf("expected missing explicit config error")
	}
	badTOML := filepath.Join(dir, ".dollarlint.toml")
	writeFile(t, badTOML, "[schema")
	if _, _, err := LoadConfig(dir, ""); err == nil {
		t.Fatalf("expected bad toml error")
	}
	jsonConfig := filepath.Join(dir, "custom.json")
	writeFile(t, jsonConfig, `{"output":{"json":true}}`)
	if _, _, err := LoadConfig(dir, jsonConfig); err == nil {
		t.Fatalf("expected explicit json config rejection")
	}
	noDotConfig := filepath.Join(dir, "dollarlint.toml")
	writeFile(t, noDotConfig, "version = 1\n")
	if _, _, err := LoadConfig(dir, noDotConfig); err == nil {
		t.Fatalf("expected explicit no-dot toml config rejection")
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
