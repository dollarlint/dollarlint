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
	writeFile(t, filepath.Join(dir, "nested", "b.yaml"), `name: ok`)
	writeFile(t, filepath.Join(dir, "node_modules", "c.json"), `{}`)
	writeFile(t, filepath.Join(dir, "dist", "d.toml"), `name = "ok"`)
	cfg := DefaultConfig().Discovery
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
	if got != "a.json,nested/b.yaml" {
		t.Fatalf("discovered %s", got)
	}
	cfg.Include = []string{"*.yaml"}
	files, err = DiscoverFiles(dir, cfg)
	if err != nil {
		t.Fatalf("DiscoverFiles include yaml: %v", err)
	}
	if len(files) != 1 || files[0].RelativePath != "nested/b.yaml" {
		t.Fatalf("yaml include = %+v", files)
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
}

func TestDiscoverAndGlobErrors(t *testing.T) {
	dir := t.TempDir()
	if _, err := DiscoverFiles(filepath.Join(dir, "missing"), DefaultConfig().Discovery); err == nil {
		t.Fatalf("expected missing root error")
	}
	if matchPattern("", "a.json") {
		t.Fatalf("empty pattern matched")
	}
	if !matchPattern(`.\*.json`, "*.json") {
		t.Fatalf("clean glob did not normalize backslashes")
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
