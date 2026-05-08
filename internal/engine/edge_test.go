package engine

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"golang.org/x/text/message"
)

func TestConfigErrorEdges(t *testing.T) {
	dir := t.TempDir()
	if _, _, err := LoadConfig(dir, dir); err == nil {
		t.Fatalf("expected read config directory error")
	}
	var cfg Config
	if err := decodeConfig("bad.json", []byte("{"), &cfg); err == nil {
		t.Fatalf("expected non-toml config rejection")
	}
	if err := decodeConfig("bad.toml", []byte("="), &cfg); err == nil {
		t.Fatalf("expected bad toml config")
	}
	inaccessible := filepath.Join(dir, "inaccessible")
	if err := os.Mkdir(inaccessible, 0o000); err != nil {
		t.Fatalf("mkdir inaccessible: %v", err)
	}
	defer os.Chmod(inaccessible, 0o755)
	_, _, _ = LoadConfig(inaccessible, "")
}

func TestDocumentEdgeBranches(t *testing.T) {
	if _, err := ParseDocument(DiscoveredFile{Path: "/definitely/missing.json", RelativePath: "missing.json"}); err == nil {
		t.Fatalf("expected read error")
	}
	if _, err := toJSONValue(map[string]any{"bad": func() {}}); err == nil {
		t.Fatalf("expected json marshal error")
	}
	if directive := yamlSchemaDirective([]byte("# other: value\n")); directive != "" {
		t.Fatalf("unexpected yaml directive %q", directive)
	}
	if directive := tomlSchemaDirective(nil); directive != "" {
		t.Fatalf("unexpected empty toml directive %q", directive)
	}
}

func TestDiscoveryDefaultRootAndWalkError(t *testing.T) {
	if _, err := DiscoverFiles("", DiscoveryConfig{Include: []string{"*.nothing"}}); err != nil {
		t.Fatalf("default-root discovery should not fail: %v", err)
	}
	dir := t.TempDir()
	blocked := filepath.Join(dir, "blocked")
	if err := os.Mkdir(blocked, 0o000); err != nil {
		t.Fatalf("mkdir blocked: %v", err)
	}
	defer os.Chmod(blocked, 0o755)
	_, _ = DiscoverFiles(dir, DefaultConfig().Discovery)
}

func TestSchemaCacheEdgeBranches(t *testing.T) {
	cfg := DefaultConfig()
	cache := NewSchemaCache(cfg)
	cache.cfg.Schemas.Concurrency = 0
	if _, err := cache.loadAndDiscover(context.Background(), nil); err != nil {
		t.Fatalf("empty loadAndDiscover: %v", err)
	}
	if _, err := cache.Load("%zz"); err == nil {
		t.Fatalf("expected bad URI normalization")
	}
	if _, err := cache.Load("file:///%zz"); err == nil {
		t.Fatalf("expected bad file URL escape")
	}
	if _, err := filePathFromURL(&url.URL{Path: "/%zz"}); err == nil {
		t.Fatalf("expected direct bad file URL escape")
	}
	missingURL, err := fileURL(filepath.Join(t.TempDir(), "missing.json"))
	if err != nil {
		t.Fatalf("fileURL missing: %v", err)
	}
	if _, err := cache.Load(missingURL.String()); err == nil {
		t.Fatalf("expected missing schema error")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "10")
		w.Write([]byte("x"))
	}))
	defer server.Close()
	if _, err := cache.Load(server.URL + "/short.json"); err == nil {
		t.Fatalf("expected response read error")
	}
	if _, err := cache.Load("http://exa mple.com/schema.json"); err == nil {
		t.Fatalf("expected request creation or fetch error")
	}
}

func TestPrimeDepthAndCycleEdges(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.json")
	b := filepath.Join(dir, "b.json")
	writeFile(t, a, `{"$ref":"./b.json"}`)
	writeFile(t, b, `{"$ref":"./a.json"}`)
	aURL, err := fileURL(a)
	if err != nil {
		t.Fatalf("fileURL a: %v", err)
	}
	cfg := DefaultConfig()
	cfg.Schemas.MaxDepth = -1
	cache := NewSchemaCache(cfg)
	if err := cache.Prime(context.Background(), []string{aURL.String()}); err == nil {
		t.Fatalf("expected depth error")
	}
	cfg = DefaultConfig()
	cache = NewSchemaCache(cfg)
	if err := cache.Prime(context.Background(), []string{aURL.String()}); err != nil {
		t.Fatalf("cycle should be caught without failing: %v", err)
	}
}

func TestReferenceAndURIEdges(t *testing.T) {
	if _, err := discoverSchemaReferences(map[string]any{}, "%zz"); err == nil {
		t.Fatalf("expected bad base URI")
	}
	var refs []string
	base, _ := url.Parse("file:///tmp/root.json")
	refs = appendResolvedReference(refs, "%zz", base)
	refs = appendResolvedReference(refs, "root.json#/defs/root", base)
	if len(refs) != 0 {
		t.Fatalf("self or invalid refs should be skipped: %+v", refs)
	}
	if withoutFragmentMust("%zz") != "%zz" {
		t.Fatalf("withoutFragmentMust should fall back to raw")
	}
	if _, err := withoutFragment("%zz"); err == nil {
		t.Fatalf("expected bad fragment URI")
	}
	if isRemoteURI("%zz") {
		t.Fatalf("bad URI is not remote")
	}
	if _, err := resolveSchemaURI("%zz", "/tmp/doc.json"); err == nil {
		t.Fatalf("expected bad schema URI")
	}
	winURL := fileURLFromAbs(`C:\repo\schema.json`, `C:\repo\schema.json`, "windows")
	if !strings.HasPrefix(winURL.Path, "/") {
		t.Fatalf("windows file URL path = %s", winURL.Path)
	}
	dirURL := fileURLFromAbs("/tmp/repo/", "/tmp/repo", "linux")
	if !strings.HasSuffix(dirURL.Path, "/") {
		t.Fatalf("directory file URL path = %s", dirURL.Path)
	}
}

func TestDecodeSchemaFallbackSuccess(t *testing.T) {
	value, err := decodeSchemaDocument([]byte(`{"type":"object"}`), "schema.unknown")
	if err != nil {
		t.Fatalf("fallback json decode: %v", err)
	}
	if value.(map[string]any)["type"] != "object" {
		t.Fatalf("fallback value = %+v", value)
	}
}

func TestValidateHelperEdges(t *testing.T) {
	err := &jsonschema.ValidationError{ErrorKind: emptyKeyword{}}
	if keywordName(err) != "" {
		t.Fatalf("expected empty keyword")
	}
	doc := &Document{Path: "/tmp/a.json", RelativePath: "a.json"}
	issues := issuesFromSchemaError(doc, errors.New("plain"), OutputConfig{})
	if len(issues) != 1 || issues[0].Message != "plain" {
		t.Fatalf("plain issues = %+v", issues)
	}
}

type emptyKeyword struct{}

func (emptyKeyword) KeywordPath() []string { return nil }

func (emptyKeyword) LocalizedString(_ *message.Printer) string { return "empty" }
