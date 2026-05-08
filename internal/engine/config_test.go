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
	if path != "" || cfg.Version != 1 || !remoteFetchEnabled(cfg.Schemas) {
		t.Fatalf("unexpected defaults: path=%q cfg=%+v", path, cfg)
	}
	if configSearchRoot("") != "." {
		t.Fatalf("empty config search root should resolve to current directory")
	}
	writeFile(t, filepath.Join(dir, ".dollarlint.yaml"), "output:\n  showSkipped: true\n")
	cfg, path, err = LoadConfig(dir, "")
	if err != nil {
		t.Fatalf("LoadConfig ignored yaml: %v", err)
	}
	if path != "" || cfg.Output.ShowSkipped {
		t.Fatalf("non-toml config should be ignored: path=%q cfg=%+v", path, cfg)
	}
	writeFile(t, filepath.Join(dir, "dollarlint.toml"), "output = { showSkipped = true }\n")
	cfg, path, err = LoadConfig(dir, "")
	if err != nil {
		t.Fatalf("LoadConfig ignored no-dot toml: %v", err)
	}
	if path != "" || cfg.Output.ShowSkipped {
		t.Fatalf("no-dot toml config should be ignored: path=%q cfg=%+v", path, cfg)
	}
	writeFile(t, filepath.Join(dir, ".dollarlint.toml"), `
version = 1

[discovery]
include = ["*.json"]
extendExclude = ["generated/**"]
useDefaultExcludes = false
respectGitIgnore = false
forceExclude = true
followSymlinks = true

[schemas]
maxDepth = 3
requireCoverage = true

[schemas.optimizations]
enabled = true

[schemas.optimizations.azure]
pruneResources = false

[schemas.fetch]
enabled = false
cache = false
timeout = "2s"
retries = 4
retryMinWait = "100ms"
retryMaxWait = "1s"
allowedDomains = ["schemas.example.com"]
blockedDomains = ["bad.example.com"]

[schemas.catalogs]
enabled = true
failure = "skip"

[[schemas.catalogs.sources]]
name = "company"
format = "schemastore"
path = "./catalog.json"

[output]
showSkipped = true
locations = true
branchErrors = "all"

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
	fileRoot := filepath.Join(dir, "settings.json")
	writeFile(t, fileRoot, `{}`)
	fileCfg, filePath, err := LoadConfig(fileRoot, "")
	if err != nil {
		t.Fatalf("LoadConfig file root: %v", err)
	}
	if filePath != path || !fileCfg.Output.ShowSkipped {
		t.Fatalf("file-root config path=%q cfg=%+v", filePath, fileCfg)
	}
	fileCfg, filePath, err = LoadConfig(fileRoot, ".dollarlint.toml")
	if err != nil {
		t.Fatalf("LoadConfig file root explicit config: %v", err)
	}
	if filePath != path || !fileCfg.Output.ShowSkipped {
		t.Fatalf("file-root explicit config path=%q cfg=%+v", filePath, fileCfg)
	}
	if remoteFetchEnabled(cfg.Schemas) || remoteFetchCacheEnabled(cfg.Schemas.Fetch) || cfg.Schemas.MaxDepth != 3 || cfg.Schemas.Fetch.Timeout.Duration != 2*time.Second {
		t.Fatalf("toml cfg not decoded/defaulted: %+v", cfg)
	}
	if !cfg.Schemas.RequireCoverage {
		t.Fatalf("schema coverage requirement not decoded: %+v", cfg.Schemas)
	}
	if !cfg.Schemas.Catalogs.Enabled || cfg.Schemas.Catalogs.Failure != "skip" || len(cfg.Schemas.Catalogs.Sources) != 1 || cfg.Schemas.Catalogs.Sources[0].Path != "./catalog.json" {
		t.Fatalf("catalogs not decoded: %+v", cfg.Schemas.Catalogs)
	}
	if fetchRetries(cfg.Schemas.Fetch) != 4 || cfg.Schemas.Fetch.RetryMinWait.Duration != 100*time.Millisecond || cfg.Schemas.Fetch.RetryMaxWait.Duration != time.Second {
		t.Fatalf("fetch resilience config not decoded: %+v", cfg.Schemas.Fetch)
	}
	if len(cfg.Schemas.Fetch.AllowedDomains) != 1 || cfg.Schemas.Fetch.AllowedDomains[0] != "schemas.example.com" || len(cfg.Schemas.Fetch.BlockedDomains) != 1 {
		t.Fatalf("domain policy not decoded: %+v", cfg.Schemas)
	}
	if cfg.Schemas.Optimizations.Azure.PruneResources == nil || *cfg.Schemas.Optimizations.Azure.PruneResources {
		t.Fatalf("Azure resource pruning opt-out not decoded: %+v", cfg.Schemas)
	}
	if !cfg.Output.ShowSkipped || !cfg.Output.Locations || cfg.Output.BranchErrors != BranchErrorsAll {
		t.Fatalf("output preferences not decoded: %+v", cfg.Output)
	}
	if len(cfg.Discovery.Include) != 1 || cfg.Discovery.Include[0] != "*.json" || len(cfg.Discovery.ExtendExclude) != 1 || cfg.Discovery.ExtendExclude[0] != "generated/**" {
		t.Fatalf("discovery config not decoded: %+v", cfg.Discovery)
	}
	if discoveryUseDefaultExcludes(cfg.Discovery) || discoveryRespectGitIgnore(cfg.Discovery) || !cfg.Discovery.ForceExclude || !cfg.Discovery.FollowSymlinks {
		t.Fatalf("discovery booleans not decoded: %+v", cfg.Discovery)
	}
	if len(cfg.Ignore) != 1 {
		t.Fatalf("defaults or ignore missing: %+v", cfg)
	}
	customPath := filepath.Join(dir, "nested", ".dollarlint.toml")
	writeFile(t, customPath, `
version = 1
[schemas]
maxDepth = 4
[[schemas.associations]]
file = "*.toml"
schema = "./schema.json"
`)
	cfg, path, err = LoadConfig(dir, "nested/.dollarlint.toml")
	if err != nil {
		t.Fatalf("LoadConfig explicit toml: %v", err)
	}
	if path != customPath || cfg.Schemas.MaxDepth != 4 || len(cfg.Schemas.Associations) != 1 {
		t.Fatalf("toml cfg = %s %+v", path, cfg)
	}
	if cfg.Schemas.Catalogs.Sources[0].URL != defaultSchemaStoreCatalogURL {
		t.Fatalf("catalog default URL = %q", cfg.Schemas.Catalogs.Sources[0].URL)
	}
	if fetchRetries(cfg.Schemas.Fetch) != 2 {
		t.Fatalf("fetch retry default = %+v", cfg.Schemas.Fetch)
	}
	if !remoteFetchCacheEnabled(cfg.Schemas.Fetch) {
		t.Fatalf("fetch cache default = %+v", cfg.Schemas.Fetch)
	}
	if mode, err := catalogFailureMode(cfg.Schemas); err != nil || mode != CatalogFailureWarn {
		t.Fatalf("catalog failure default = %q, %v", mode, err)
	}
	if mode, err := catalogFailureMode(SchemaConfig{Catalogs: CatalogConfig{Failure: CatalogFailureSkip}}); err != nil || mode != CatalogFailureSkip {
		t.Fatalf("catalog failure skip = %q, %v", mode, err)
	}
	if mode, err := catalogFailureMode(SchemaConfig{Catalogs: CatalogConfig{Failure: CatalogFailureError}}); err != nil || mode != CatalogFailureError {
		t.Fatalf("catalog failure error = %q, %v", mode, err)
	}
	if _, err := catalogFailureMode(SchemaConfig{Catalogs: CatalogConfig{Failure: "explode"}}); err == nil {
		t.Fatalf("expected invalid catalog failure mode error")
	}
	if mode, err := branchErrorMode(OutputConfig{}); err != nil || mode != BranchErrorsBest {
		t.Fatalf("branch errors default = %q, %v", mode, err)
	}
	if mode, err := branchErrorMode(OutputConfig{BranchErrors: BranchErrorsAll}); err != nil || mode != BranchErrorsAll {
		t.Fatalf("branch errors all = %q, %v", mode, err)
	}
	if _, err := branchErrorMode(OutputConfig{BranchErrors: "explode"}); err == nil {
		t.Fatalf("expected invalid branch errors mode")
	}
	if fetchRetries(FetchConfig{}) != 0 {
		t.Fatalf("nil fetch retries should resolve to zero")
	}
	negativeRetries := -1
	if fetchRetries(FetchConfig{Retries: &negativeRetries}) != 0 {
		t.Fatalf("negative fetch retries should resolve to zero")
	}
}

func TestLoadConfigErrors(t *testing.T) {
	dir := t.TempDir()
	if _, _, err := LoadConfig(dir, ".dollarlint.toml"); err == nil {
		t.Fatalf("expected missing explicit config error")
	}
	dirConfigRoot := filepath.Join(t.TempDir(), "dir-config")
	if err := os.MkdirAll(filepath.Join(dirConfigRoot, ".dollarlint.toml"), 0o755); err != nil {
		t.Fatalf("mkdir dir config: %v", err)
	}
	if _, _, err := LoadConfig(dirConfigRoot, ""); err == nil || !strings.Contains(err.Error(), "read config") {
		t.Fatalf("expected directory config read error, got %v", err)
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
	writeFile(t, badTOML, `[schemas]
maxDepth = -1
`)
	if _, _, err := LoadConfig(dir, ""); err == nil || !strings.Contains(err.Error(), "schemas.maxDepth") {
		t.Fatalf("expected negative maxDepth error, got %v", err)
	}
	writeFile(t, badTOML, `[output]
branchErrors = "explode"
`)
	if _, _, err := LoadConfig(dir, ""); err == nil || !strings.Contains(err.Error(), "output.branchErrors") {
		t.Fatalf("expected invalid branchErrors error, got %v", err)
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
