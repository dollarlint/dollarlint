package engine

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

func TestDiscoverFilesSkipsNonSourceAndMatchesGlobs(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.json"), `{}`)
	writeFile(t, filepath.Join(dir, "variants", "b.jsonc"), `{}`)
	writeFile(t, filepath.Join(dir, "variants", "c.json5"), `{}`)
	writeFile(t, filepath.Join(dir, "variants", "d.jsonl"), `{}`)
	writeFile(t, filepath.Join(dir, "variants", "e.ndjson"), `{}`)
	writeFile(t, filepath.Join(dir, "nested", "b.yaml"), `name: ok`)
	writeFile(t, filepath.Join(dir, "node_modules", "c.json"), `{}`)
	writeFile(t, filepath.Join(dir, "dist", "d.toml"), `name = "ok"`)
	writeFile(t, filepath.Join(dir, "generated", "e.json"), `{}`)
	cfg := DefaultConfig().Discovery
	cfg.Exclude = []string{"generated/**"}
	files, err := DiscoverFiles(dir, cfg)
	if err != nil {
		t.Fatalf("DiscoverFiles: %v", err)
	}
	var rels []string
	for _, file := range files {
		rels = append(rels, file.RelativePath)
	}
	sort.Strings(rels)
	got := strings.Join(rels, ",")
	if got != "a.json,nested/b.yaml,variants/b.jsonc,variants/c.json5,variants/d.jsonl,variants/e.ndjson" {
		t.Fatalf("discovered %s", got)
	}
	disabled := false
	cfg.UseDefaultExcludes = &disabled
	cfg.Exclude = nil
	files, err = DiscoverFiles(dir, cfg)
	if err != nil {
		t.Fatalf("DiscoverFiles without default excludes: %v", err)
	}
	if len(files) != 9 {
		t.Fatalf("without default excludes = %+v", files)
	}
	cfg = DefaultConfig().Discovery
	cfg.Include = []string{"*.yaml"}
	files, err = DiscoverFiles(dir, cfg)
	if err != nil {
		t.Fatalf("DiscoverFiles include yaml: %v", err)
	}
	if len(files) != 1 || files[0].RelativePath != "nested/b.yaml" {
		t.Fatalf("yaml include = %+v", files)
	}
}

func TestDiscoverFilesSkipsAdditionalGeneratedLocations(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "kept.json"), `{}`)
	writeFile(t, filepath.Join(dir, ".build", "swift.json"), `{}`)
	writeFile(t, filepath.Join(dir, "DerivedData", "derived.json"), `{}`)
	writeFile(t, filepath.Join(dir, "Intermediates.noindex", "intermediate.json"), `{}`)
	writeFile(t, filepath.Join(dir, "temp", "run.jsonl"), `{}`)
	writeFile(t, filepath.Join(dir, "Package.dSYM", "Contents", "Info.json"), `{}`)
	writeFile(t, filepath.Join(dir, "SourcePackages", "checkouts", "dependency.json"), `{}`)
	files, err := DiscoverFiles(dir, DefaultConfig().Discovery)
	if err != nil {
		t.Fatalf("DiscoverFiles: %v", err)
	}
	if len(files) != 1 || files[0].RelativePath != "kept.json" {
		t.Fatalf("generated-location discovery = %+v", files)
	}
}

func TestDiscoverSingleFileAndSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.json")
	link := filepath.Join(dir, "link.json")
	writeFile(t, target, `{}`)
	cfg := DefaultConfig().Discovery
	files, err := DiscoverFiles(target, cfg)
	if err != nil {
		t.Fatalf("DiscoverFiles single: %v", err)
	}
	if len(files) != 1 || files[0].RelativePath != "target.json" {
		t.Fatalf("single file = %+v", files)
	}
	if runtime.GOOS != "windows" {
		if err := os.Symlink(target, link); err != nil {
			t.Fatalf("symlink: %v", err)
		}
		files, err = DiscoverFiles(dir, cfg)
		if err != nil {
			t.Fatalf("DiscoverFiles symlink false: %v", err)
		}
		if len(files) != 1 {
			t.Fatalf("expected symlink skip, got %+v", files)
		}
		cfg.FollowSymlinks = true
		files, err = DiscoverFiles(dir, cfg)
		if err != nil {
			t.Fatalf("DiscoverFiles symlink true: %v", err)
		}
		if len(files) != 2 {
			t.Fatalf("expected symlink include, got %+v", files)
		}
	}
	cfg.Include = []string{"*.yaml"}
	files, err = DiscoverFiles(target, cfg)
	if err != nil {
		t.Fatalf("DiscoverFiles excluded single: %v", err)
	}
	if len(files) != 0 {
		t.Fatalf("expected excluded single file, got %+v", files)
	}
	cfg = DefaultConfig().Discovery
	cfg.Exclude = []string{"target.json"}
	files, err = DiscoverFiles(target, cfg)
	if err != nil {
		t.Fatalf("DiscoverFiles single force false: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected explicit file to bypass excludes, got %+v", files)
	}
	cfg.ForceExclude = true
	files, err = DiscoverFiles(target, cfg)
	if err != nil {
		t.Fatalf("DiscoverFiles single force true: %v", err)
	}
	if len(files) != 0 {
		t.Fatalf("expected force excluded single file, got %+v", files)
	}
}

func TestDiscoverFilesRespectsGitIgnore(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".gitignore"), "ignored.json\nignored-dir/\n*.tmp.json\n!keep.tmp.json\n")
	writeFile(t, filepath.Join(dir, "kept.json"), `{}`)
	writeFile(t, filepath.Join(dir, "ignored.json"), `{}`)
	writeFile(t, filepath.Join(dir, "ignored-dir", "nested.json"), `{}`)
	writeFile(t, filepath.Join(dir, "drop.tmp.json"), `{}`)
	writeFile(t, filepath.Join(dir, "keep.tmp.json"), `{}`)
	cfg := DefaultConfig().Discovery
	files, err := DiscoverFiles(dir, cfg)
	if err != nil {
		t.Fatalf("DiscoverFiles gitignore: %v", err)
	}
	var rels []string
	for _, file := range files {
		rels = append(rels, file.RelativePath)
	}
	sort.Strings(rels)
	if got := strings.Join(rels, ","); got != "keep.tmp.json,kept.json" {
		t.Fatalf("gitignore discovered %s", got)
	}
	disabled := false
	cfg.RespectGitIgnore = &disabled
	files, err = DiscoverFiles(dir, cfg)
	if err != nil {
		t.Fatalf("DiscoverFiles no gitignore: %v", err)
	}
	if len(files) != 5 {
		t.Fatalf("expected gitignore disabled to include all JSON files, got %+v", files)
	}
}

func TestDiscoverFilesRespectsNestedGitIgnore(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".gitignore"), "root-ignored.json\n")
	writeFile(t, filepath.Join(dir, "kept.json"), `{}`)
	writeFile(t, filepath.Join(dir, "root-ignored.json"), `{}`)
	writeFile(t, filepath.Join(dir, "nested", ".gitignore"), "ignored.json\n!keep.json\n")
	writeFile(t, filepath.Join(dir, "nested", "ignored.json"), `{}`)
	writeFile(t, filepath.Join(dir, "nested", "keep.json"), `{}`)
	writeFile(t, filepath.Join(dir, "nested", "deeper", "ignored.json"), `{}`)
	files, err := DiscoverFiles(dir, DefaultConfig().Discovery)
	if err != nil {
		t.Fatalf("DiscoverFiles nested gitignore: %v", err)
	}
	var rels []string
	for _, file := range files {
		rels = append(rels, file.RelativePath)
	}
	sort.Strings(rels)
	if got := strings.Join(rels, ","); got != "kept.json,nested/keep.json" {
		t.Fatalf("nested gitignore discovered %s", got)
	}
}

func TestDiscoverAndGlobErrors(t *testing.T) {
	dir := t.TempDir()
	if _, err := DiscoverFiles(filepath.Join(dir, "missing"), DefaultConfig().Discovery); err == nil {
		t.Fatalf("expected missing root error")
	}
	badGitIgnoreDir := filepath.Join(dir, "bad-gitignore")
	if err := os.MkdirAll(filepath.Join(badGitIgnoreDir, ".gitignore"), 0o755); err != nil {
		t.Fatalf("mkdir bad gitignore: %v", err)
	}
	if _, err := DiscoverFiles(badGitIgnoreDir, DefaultConfig().Discovery); err == nil {
		t.Fatalf("expected unreadable gitignore error")
	}
	rootFile := filepath.Join(dir, "root-file")
	writeFile(t, rootFile, "")
	if _, err := loadGitIgnoreRules(rootFile); err == nil {
		t.Fatalf("expected non-directory gitignore root error")
	}
	if matchPattern("", "a.json") {
		t.Fatalf("empty pattern matched")
	}
	if !matchPattern(`.\*.json`, "*.json") {
		t.Fatalf("clean glob did not normalize backslashes")
	}
	cfg := normalizedDiscoveryConfig(DiscoveryConfig{})
	if len(cfg.Include) == 0 || !discoveryUseDefaultExcludes(cfg) || !discoveryRespectGitIgnore(cfg) {
		t.Fatalf("normalized discovery config = %+v", cfg)
	}
	cfg = normalizedDiscoveryConfig(DiscoveryConfig{Include: []string{}})
	if cfg.Include == nil || len(cfg.Include) != 0 {
		t.Fatalf("explicit empty include should not get defaults: %+v", cfg)
	}
	if rule, ok := parseGitIgnoreRule("", "# comment"); ok || rule.Pattern != "" {
		t.Fatalf("comment rule parsed: %+v", rule)
	}
	if rule, ok := parseGitIgnoreRule("", "!"); ok || rule.Pattern != "" {
		t.Fatalf("empty negation rule parsed: %+v", rule)
	}
	if rule, ok := parseGitIgnoreRule("", `\#literal.json`); !ok || !rule.matches("#literal.json", false) {
		t.Fatalf("escaped comment rule = %+v %v", rule, ok)
	}
	if rule, ok := parseGitIgnoreRule("", `\!literal.json`); !ok || !rule.matches("!literal.json", false) {
		t.Fatalf("escaped negation rule = %+v %v", rule, ok)
	}
	if rule, ok := parseGitIgnoreRule("", "/anchored.json"); !ok || !rule.Anchored || !rule.matches("anchored.json", false) || rule.matches("nested/anchored.json", false) {
		t.Fatalf("anchored rule = %+v %v", rule, ok)
	}
	if rule, ok := parseGitIgnoreRule("", "/cache/"); !ok || !rule.Anchored || !rule.DirOnly || !rule.matches("cache/file.json", false) || rule.matches("nested/cache/file.json", false) {
		t.Fatalf("anchored directory rule = %+v %v", rule, ok)
	}
	if rule, ok := parseGitIgnoreRule("nested", "skip.json"); !ok || !rule.matches("nested/skip.json", false) || rule.matches("other/skip.json", false) || rule.matches("nested", true) {
		t.Fatalf("based rule = %+v %v", rule, ok)
	}
	if rule, ok := parseGitIgnoreRule("", "logs/cache/"); !ok || !rule.matches("logs/cache", true) || !rule.matches("logs/cache/file.json", false) || !rule.matches("nested/logs/cache/file.json", false) {
		t.Fatalf("directory rule = %+v %v", rule, ok)
	}
	if rule, ok := parseGitIgnoreRule("", "docs/*.json"); !ok || !rule.matches("docs/a.json", false) || !rule.matches("nested/docs/a.json", false) || rule.matches("docs/a.yaml", false) {
		t.Fatalf("slash pattern rule = %+v %v", rule, ok)
	}
}

func TestResolveSchemaURI(t *testing.T) {
	dir := t.TempDir()
	doc := filepath.Join(dir, "nested", "doc.json")
	resolved, err := resolveSchemaURI("../schema.json#/defs/root", doc)
	if err != nil {
		t.Fatalf("resolve local schema: %v", err)
	}
	if !strings.HasPrefix(resolved, "file://") || !strings.HasSuffix(resolved, "/schema.json#/defs/root") {
		t.Fatalf("resolved local = %s", resolved)
	}
	remote, err := resolveSchemaURI("https://example.com/schema.json", doc)
	if err != nil {
		t.Fatalf("resolve remote: %v", err)
	}
	if remote != "https://example.com/schema.json" {
		t.Fatalf("remote = %s", remote)
	}
	if _, err := resolveSchemaURI("", doc); err == nil {
		t.Fatalf("expected empty schema error")
	}
	if _, err := withoutFragment("http://example.com/schema.json#/x"); err != nil {
		t.Fatalf("withoutFragment: %v", err)
	}
	if isRemoteURI("https://example.com") != true || isRemoteURI("file:///tmp/a") != false {
		t.Fatalf("isRemoteURI mismatch")
	}
}
