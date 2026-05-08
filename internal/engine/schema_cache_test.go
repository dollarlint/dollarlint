package engine

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestSchemaCacheLoadsLocalAndPrimesReferences(t *testing.T) {
	dir := t.TempDir()
	rootSchema := filepath.Join(dir, "root.json")
	childSchema := filepath.Join(dir, "child.json")
	writeFile(t, rootSchema, `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "type": "object",
  "properties": {"child": {"$ref": "./child.json"}}
}`)
	writeFile(t, childSchema, `{"type":"string"}`)
	rootURL, err := fileURL(rootSchema)
	if err != nil {
		t.Fatalf("fileURL: %v", err)
	}
	cfg := DefaultConfig()
	cache := NewSchemaCache(cfg)
	if err := cache.Prime(context.Background(), []string{rootURL.String()}); err != nil {
		t.Fatalf("Prime: %v", err)
	}
	childURL, err := fileURL(childSchema)
	if err != nil {
		t.Fatalf("fileURL child: %v", err)
	}
	if _, err := cache.Load(childURL.String()); err != nil {
		t.Fatalf("child should be cached/loadable: %v", err)
	}
	refs, err := discoverSchemaReferences(map[string]any{"$ref": "./child.json#/defs/x"}, rootURL.String())
	if err != nil {
		t.Fatalf("discover refs: %v", err)
	}
	if len(refs) != 1 || !strings.HasSuffix(refs[0], "/child.json") {
		t.Fatalf("refs = %+v", refs)
	}
}

func TestSchemaCacheRemoteFetch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/missing.json" {
			http.Error(w, "nope", http.StatusNotFound)
			return
		}
		if r.Header.Get("User-Agent") != "dollarlint" {
			t.Fatalf("missing user agent")
		}
		w.Write([]byte(`{"type":"object"}`))
	}))
	defer server.Close()
	cache := NewSchemaCache(DefaultConfig())
	if _, err := cache.Load(server.URL + "/schema.json"); err != nil {
		t.Fatalf("remote load: %v", err)
	}
	if _, err := cache.Load(server.URL + "/missing.json"); err == nil {
		t.Fatalf("expected non-2xx error")
	}
	cfg := DefaultConfig()
	disabled := false
	cfg.Schema.FetchRemote = &disabled
	cache = NewSchemaCache(cfg)
	if _, err := cache.Load(server.URL + "/schema.json"); err == nil {
		t.Fatalf("expected remote disabled error")
	}
}

func TestSchemaCacheRetriesTransientRemoteFetchFailures(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&attempts, 1)
		if count == 1 {
			http.Error(w, "try again", http.StatusServiceUnavailable)
			return
		}
		w.Write([]byte(`{"type":"object"}`))
	}))
	defer server.Close()

	cfg := DefaultConfig()
	cfg.Schema.Fetch.RetryMinWait = NewDuration(time.Millisecond)
	cfg.Schema.Fetch.RetryMaxWait = NewDuration(time.Millisecond)
	cache := NewSchemaCache(cfg)
	if _, err := cache.Load(server.URL + "/schema.json"); err != nil {
		t.Fatalf("remote load after retry: %v", err)
	}
	if attempts != 2 {
		t.Fatalf("attempts = %d", attempts)
	}
}

func TestSchemaCacheDoesNotRetryDeterministicHTTPFailures(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		http.Error(w, "missing", http.StatusNotFound)
	}))
	defer server.Close()

	cache := NewSchemaCache(DefaultConfig())
	if _, err := cache.Load(server.URL + "/missing.json"); err == nil {
		t.Fatalf("expected non-retryable 404 error")
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d", attempts)
	}
}

func TestSchemaCacheRemoteDomainPolicy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"type":"object"}`))
	}))
	defer server.Close()
	host := mustURLHost(t, server.URL)
	cfg := DefaultConfig()
	cfg.Schema.AllowedDomains = []string{host}
	cache := NewSchemaCache(cfg)
	if _, err := cache.Load(server.URL + "/schema.json"); err != nil {
		t.Fatalf("allowed domain load: %v", err)
	}
	cfg.Schema.BlockedDomains = []string{host}
	cache = NewSchemaCache(cfg)
	if _, err := cache.Load(server.URL + "/schema.json"); err == nil || !strings.Contains(err.Error(), "blocked") {
		t.Fatalf("expected blocked domain error, got %v", err)
	}
	cfg = DefaultConfig()
	cfg.Schema.AllowedDomains = []string{"example.com"}
	cache = NewSchemaCache(cfg)
	if _, err := cache.Load(server.URL + "/schema.json"); err == nil || !strings.Contains(err.Error(), "not allowed") {
		t.Fatalf("expected disallowed domain error, got %v", err)
	}
	if matched, err := matchesAnyDomainPattern("api.example.com", []string{"*.example.com"}); err != nil || !matched {
		t.Fatalf("wildcard match = %v, %v", matched, err)
	}
	if matched, err := matchesAnyDomainPattern("example.com", []string{"*.example.com"}); err != nil || matched {
		t.Fatalf("wildcard root match = %v, %v", matched, err)
	}
	if _, err := matchesAnyDomainPattern("example.com", []string{"bad/domain"}); err == nil {
		t.Fatalf("expected invalid domain pattern")
	}
}

func TestSchemaCacheDepthAndLoadErrors(t *testing.T) {
	dir := t.TempDir()
	rootSchema := filepath.Join(dir, "root.json")
	childSchema := filepath.Join(dir, "child.json")
	writeFile(t, rootSchema, `{"$ref":"./child.json"}`)
	writeFile(t, childSchema, `{"$ref":"./grandchild.json"}`)
	rootURL, err := fileURL(rootSchema)
	if err != nil {
		t.Fatalf("fileURL: %v", err)
	}
	cfg := DefaultConfig()
	cfg.Schema.MaxDepth = 0
	cache := NewSchemaCache(cfg)
	if err := cache.Prime(context.Background(), []string{rootURL.String()}); err == nil {
		t.Fatalf("expected depth error")
	}
	if _, err := cache.Load("ftp://example.com/schema.json"); err == nil {
		t.Fatalf("expected unsupported scheme")
	}
	empty := "file://"
	if _, err := cache.Load(empty); err == nil {
		t.Fatalf("expected empty file URL path")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := cache.Prime(ctx, []string{rootURL.String()}); err == nil {
		t.Fatalf("expected canceled prime")
	}
}

func TestDecodeSchemaDocumentFallbacks(t *testing.T) {
	if value, err := decodeSchemaDocument([]byte("type: object"), "schema.yaml"); err != nil {
		t.Fatalf("yaml schema: %v", err)
	} else if value.(map[string]any)["type"] != "object" {
		t.Fatalf("yaml value = %+v", value)
	}
	if value, err := decodeSchemaDocument([]byte(`type = "object"`), "schema.toml"); err != nil {
		t.Fatalf("toml schema: %v", err)
	} else if value.(map[string]any)["type"] != "object" {
		t.Fatalf("toml value = %+v", value)
	}
	if _, err := decodeSchemaDocument([]byte(":"), "schema.json"); err == nil {
		t.Fatalf("expected invalid schema document")
	}
	if _, err := filePathFromURL(&url.URL{}); err == nil {
		t.Fatalf("expected empty path")
	}
}

func TestSchemaReferencesRespectIDAndSkipFragments(t *testing.T) {
	doc := map[string]any{
		"$id": "https://example.com/schemas/root.json",
		"$defs": map[string]any{
			"local": map[string]any{"$ref": "#/$defs/local"},
			"next":  map[string]any{"$dynamicRef": "next.json#/defs/root"},
		},
		"items": []any{map[string]any{"$recursiveRef": ""}},
	}
	refs, err := discoverSchemaReferences(doc, "file:///tmp/root.json")
	if err != nil {
		t.Fatalf("discover refs: %v", err)
	}
	if len(refs) != 1 || refs[0] != "https://example.com/schemas/next.json" {
		t.Fatalf("refs = %+v", refs)
	}
}

func TestSchemaCacheFetchTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond)
		w.Write([]byte(`{"type":"object"}`))
	}))
	defer server.Close()
	cfg := DefaultConfig()
	zeroRetries := 0
	cfg.Schema.Fetch.Retries = &zeroRetries
	cfg.Timeouts.Fetch = NewDuration(time.Nanosecond)
	cache := NewSchemaCache(cfg)
	if _, err := cache.Load(server.URL + "/slow.json"); err == nil {
		t.Fatalf("expected fetch timeout")
	}
}

func mustURLHost(t *testing.T, raw string) string {
	t.Helper()
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse %s: %v", raw, err)
	}
	host := parsed.Hostname()
	if ip := net.ParseIP(host); ip != nil {
		return ip.String()
	}
	return host
}
