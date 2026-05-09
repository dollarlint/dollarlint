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
[discovery]
include = []
`)
	cfg, path, err = LoadConfig(dir, "")
	if err != nil {
		t.Fatalf("LoadConfig empty include: %v", err)
	}
	if path == "" || cfg.Discovery.Include == nil || len(cfg.Discovery.Include) != 0 {
		t.Fatalf("explicit empty include should not get defaults: path=%q cfg=%+v", path, cfg)
	}
	writeFile(t, filepath.Join(dir, ".dollarlint.toml"), `
version = 1

[discovery]
include = ["*.json"]
exclude = ["generated/**"]
useDefaultExcludes = false
respectGitIgnore = false
forceExclude = true
followSymlinks = true

[parsing.json]
mode = "jsonc"

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
match = "all"

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
	if !cfg.Schemas.Catalogs.Enabled || cfg.Schemas.Catalogs.Failure != "skip" || cfg.Schemas.Catalogs.Match != CatalogMatchAll || len(cfg.Schemas.Catalogs.Sources) != 1 || cfg.Schemas.Catalogs.Sources[0].Path != filepath.Join(dir, "catalog.json") {
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
	if len(cfg.Discovery.Include) != 1 || cfg.Discovery.Include[0] != "*.json" || len(cfg.Discovery.Exclude) != 1 || cfg.Discovery.Exclude[0] != "generated/**" {
		t.Fatalf("discovery config not decoded: %+v", cfg.Discovery)
	}
	if discoveryUseDefaultExcludes(cfg.Discovery) || discoveryRespectGitIgnore(cfg.Discovery) || !cfg.Discovery.ForceExclude || !cfg.Discovery.FollowSymlinks {
		t.Fatalf("discovery booleans not decoded: %+v", cfg.Discovery)
	}
	if cfg.Parsing.JSON.Mode != JSONParsingJSONC {
		t.Fatalf("parsing config not decoded: %+v", cfg.Parsing)
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
	if cfg.Schemas.Associations[0].File != "nested/**/*.toml" {
		t.Fatalf("config-relative association file = %+v", cfg.Schemas.Associations[0])
	}
	if !strings.HasPrefix(cfg.Schemas.Associations[0].Schema, "file://") || !strings.HasSuffix(cfg.Schemas.Associations[0].Schema, "/nested/schema.json") {
		t.Fatalf("config-relative association schema = %+v", cfg.Schemas.Associations[0])
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
	if mode, err := jsonParsingMode(cfg.Parsing); err != nil || mode != JSONParsingAuto {
		t.Fatalf("json parsing default = %q, %v", mode, err)
	}
	if mode, err := jsonParsingMode(ParsingConfig{JSON: JSONParsingConfig{Mode: JSONParsingStrict}}); err != nil || mode != JSONParsingStrict {
		t.Fatalf("json parsing strict = %q, %v", mode, err)
	}
	if _, err := jsonParsingMode(ParsingConfig{JSON: JSONParsingConfig{Mode: "loose"}}); err == nil {
		t.Fatalf("expected invalid json parsing mode error")
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
	if mode, err := catalogMatchMode(cfg.Schemas); err != nil || mode != CatalogMatchAuto {
		t.Fatalf("catalog match default = %q, %v", mode, err)
	}
	if mode, err := catalogMatchMode(SchemaConfig{Catalogs: CatalogConfig{Match: CatalogMatchAll}}); err != nil || mode != CatalogMatchAll {
		t.Fatalf("catalog match all = %q, %v", mode, err)
	}
	if _, err := catalogMatchMode(SchemaConfig{Catalogs: CatalogConfig{Match: "explode"}}); err == nil {
		t.Fatalf("expected invalid catalog match mode error")
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

func TestLoadConfigExtendsMergesWithPresence(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".dollarlint.toml"), `
[discovery]
include = ["*.json"]
exclude = ["generated/**"]
useDefaultExcludes = true
forceExclude = true

[schemas]
requireCoverage = true
maxDepth = 4

[schemas.catalogs]
enabled = true
failure = "error"
match = "all"

[[schemas.catalogs.sources]]
name = "company"
format = "schemastore"
path = "./catalog.json"

[[schemas.associations]]
file = "*.json"
schema = "./root.schema"

[[ignore]]
file = "*.json"
reason = "parent"

[output]
locations = true
`)
	childPath := filepath.Join(dir, "packages", "api", ".dollarlint.toml")
	writeFile(t, childPath, `
extends = "../../.dollarlint.toml"

[discovery]
include = []
exclude = ["fixtures/**"]
useDefaultExcludes = false
forceExclude = false

[schemas]
requireCoverage = false

[schemas.catalogs]
enabled = false
match = "auto"

[[schemas.catalogs.sources]]
name = "company"
format = "schemastore"
path = "./child-catalog.json"
enabled = false

[[schemas.associations]]
file = "*.yaml"
schema = "./child.schema"

[[ignore]]
file = "*.yaml"
reason = "child"

[output]
locations = false
`)
	cfg, path, err := LoadConfig(dir, "packages/api/.dollarlint.toml")
	if err != nil {
		t.Fatalf("LoadConfig extends: %v", err)
	}
	if path != childPath {
		t.Fatalf("path = %s", path)
	}
	if cfg.Extends != "" {
		t.Fatalf("resolved config should clear extends, got %q", cfg.Extends)
	}
	if cfg.Discovery.Include == nil || len(cfg.Discovery.Include) != 0 {
		t.Fatalf("child empty include should replace parent: %+v", cfg.Discovery)
	}
	if len(cfg.Discovery.Exclude) != 2 || cfg.Discovery.Exclude[0] != "generated/**" || cfg.Discovery.Exclude[1] != "packages/api/fixtures/**" {
		t.Fatalf("exclude merge = %+v", cfg.Discovery.Exclude)
	}
	if discoveryUseDefaultExcludes(cfg.Discovery) || cfg.Discovery.ForceExclude {
		t.Fatalf("presence-aware boolean override failed: %+v", cfg.Discovery)
	}
	if cfg.Schemas.RequireCoverage || cfg.Schemas.MaxDepth != 4 {
		t.Fatalf("schema merge = %+v", cfg.Schemas)
	}
	if cfg.Schemas.Catalogs.Enabled || cfg.Schemas.Catalogs.Failure != "error" || cfg.Schemas.Catalogs.Match != CatalogMatchAuto || len(cfg.Schemas.Catalogs.Sources) != 1 {
		t.Fatalf("catalog merge = %+v", cfg.Schemas.Catalogs)
	}
	if cfg.Schemas.Catalogs.Sources[0].Path != filepath.Join(dir, "packages", "api", "child-catalog.json") || cfg.Schemas.Catalogs.Sources[0].Enabled == nil || *cfg.Schemas.Catalogs.Sources[0].Enabled {
		t.Fatalf("catalog source override = %+v", cfg.Schemas.Catalogs.Sources[0])
	}
	if len(cfg.Schemas.Associations) != 2 ||
		cfg.Schemas.Associations[0].File != "*.json" ||
		cfg.Schemas.Associations[1].File != "packages/api/**/*.yaml" ||
		!strings.HasSuffix(cfg.Schemas.Associations[1].Schema, "/packages/api/child.schema") {
		t.Fatalf("association merge = %+v", cfg.Schemas.Associations)
	}
	if len(cfg.Ignore) != 2 || cfg.Ignore[1].File != "packages/api/**/*.yaml" {
		t.Fatalf("ignore merge = %+v", cfg.Ignore)
	}
	if cfg.Output.Locations {
		t.Fatalf("output boolean override failed: %+v", cfg.Output)
	}
}

func TestLoadConfigExtendsCycle(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".dollarlint.toml"), `extends = "nested/.dollarlint.toml"`)
	writeFile(t, filepath.Join(dir, "nested", ".dollarlint.toml"), `extends = "../.dollarlint.toml"`)
	if _, _, err := LoadConfig(dir, ""); err == nil || !strings.Contains(err.Error(), "extends cycle") {
		t.Fatalf("expected extends cycle error, got %v", err)
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
