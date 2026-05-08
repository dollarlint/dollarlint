package engine

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/hashicorp/go-retryablehttp"
)

// maxSchemaResponseBytes caps the size of a single schema fetched over HTTP
// to avoid unbounded memory growth from misbehaving or hostile servers.
const maxSchemaResponseBytes = 64 * 1024 * 1024

type SchemaCache struct {
	cfg     Config
	client  *http.Client
	mu      sync.Mutex
	entries map[string]*schemaEntry
}

// schemaEntry serializes loads of a single URI and caches successful results.
// Errors are intentionally not cached: a transient network failure should not
// permanently poison the cache for the lifetime of the process.
type schemaEntry struct {
	mu     sync.Mutex
	loaded bool
	doc    any
}

func NewSchemaCache(cfg Config) *SchemaCache {
	cfg.ApplyDefaults()
	client := retryablehttp.NewClient()
	client.HTTPClient.Timeout = cfg.Schemas.Fetch.Timeout.Duration
	client.RetryMax = fetchRetries(cfg.Schemas.Fetch)
	client.RetryWaitMin = cfg.Schemas.Fetch.RetryMinWait.Duration
	client.RetryWaitMax = cfg.Schemas.Fetch.RetryMaxWait.Duration
	client.CheckRetry = retryableHTTPPolicy
	client.Logger = nil
	return &SchemaCache{
		cfg:     cfg,
		client:  client.StandardClient(),
		entries: map[string]*schemaEntry{},
	}
}

func retryableHTTPPolicy(ctx context.Context, resp *http.Response, err error) (bool, error) {
	retry, retryErr := retryablehttp.DefaultRetryPolicy(ctx, resp, err)
	if retry || retryErr != nil || resp == nil {
		return retry, retryErr
	}
	return resp.StatusCode == http.StatusRequestTimeout ||
		resp.StatusCode == http.StatusTooEarly, nil
}

func (c *SchemaCache) Load(raw string) (any, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.cfg.Schemas.Fetch.Timeout.Duration)
	defer cancel()
	return c.LoadContext(ctx, raw)
}

func (c *SchemaCache) LoadContext(ctx context.Context, raw string) (any, error) {
	key, err := withoutFragment(raw)
	if err != nil {
		return nil, fmt.Errorf("normalize schema URI %s: %w", raw, err)
	}
	entry := c.entry(key)
	entry.mu.Lock()
	defer entry.mu.Unlock()
	if entry.loaded {
		return entry.doc, nil
	}
	doc, err := c.loadUncached(ctx, key)
	if err != nil {
		return nil, err
	}
	entry.doc = doc
	entry.loaded = true
	return doc, nil
}

func (c *SchemaCache) Prime(ctx context.Context, roots []string) error {
	current := uniqueStrings(roots)
	visited := map[string]bool{}
	for depth := 0; len(current) > 0; depth++ {
		if depth > c.cfg.Schemas.MaxDepth {
			return fmt.Errorf("schema reference depth exceeds limit %d", c.cfg.Schemas.MaxDepth)
		}
		next, err := c.loadAndDiscover(ctx, current)
		if err != nil {
			return err
		}
		visitedNow := make([]string, 0, len(next))
		for _, ref := range next {
			key := withoutFragmentMust(ref)
			if visited[key] {
				continue
			}
			visited[key] = true
			visitedNow = append(visitedNow, key)
		}
		for _, ref := range current {
			visited[withoutFragmentMust(ref)] = true
		}
		current = visitedNow
	}
	return nil
}

func (c *SchemaCache) entry(key string) *schemaEntry {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry := c.entries[key]
	if entry == nil {
		entry = &schemaEntry{}
		c.entries[key] = entry
	}
	return entry
}

func (c *SchemaCache) loadAndDiscover(ctx context.Context, uris []string) ([]string, error) {
	type result struct {
		refs []string
		err  error
	}
	workers := c.cfg.Schemas.Concurrency
	if workers < 1 {
		workers = 1
	}
	if workers > len(uris) {
		workers = len(uris)
	}
	jobs := make(chan string)
	results := make(chan result, len(uris))
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for uri := range jobs {
				doc, err := c.LoadContext(ctx, uri)
				if err != nil {
					results <- result{err: err}
					continue
				}
				refs, err := discoverSchemaReferences(doc, uri)
				results <- result{refs: refs, err: err}
			}
		}()
	}
	for _, uri := range uris {
		select {
		case <-ctx.Done():
			close(jobs)
			wg.Wait()
			close(results)
			return nil, ctx.Err()
		case jobs <- uri:
		}
	}
	close(jobs)
	wg.Wait()
	close(results)
	var refs []string
	var errs []error
	for result := range results {
		if result.err != nil {
			errs = append(errs, result.err)
			continue
		}
		refs = append(refs, result.refs...)
	}
	return uniqueStrings(refs), errors.Join(errs...)
}

func (c *SchemaCache) loadUncached(ctx context.Context, raw string) (any, error) {
	parsed, _ := url.Parse(raw)
	switch parsed.Scheme {
	case "file":
		path, err := filePathFromURL(parsed)
		if err != nil {
			return nil, err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read schema %s: %w", path, err)
		}
		return decodeSchemaDocument(data, path)
	case "http", "https":
		if err := checkRemoteDomainPolicy(raw, c.cfg.Schemas); err != nil {
			return nil, err
		}
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
		req.Header.Set("User-Agent", "dollarlint")
		resp, err := c.client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("fetch schema %s: %w", raw, err)
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode > 299 {
			return nil, fmt.Errorf("fetch schema %s: HTTP %d", raw, resp.StatusCode)
		}
		data, err := io.ReadAll(io.LimitReader(resp.Body, maxSchemaResponseBytes+1))
		if err != nil {
			return nil, fmt.Errorf("read schema response %s: %w", raw, err)
		}
		if len(data) > maxSchemaResponseBytes {
			return nil, fmt.Errorf("fetch schema %s: response exceeds %d bytes", raw, maxSchemaResponseBytes)
		}
		return decodeSchemaDocument(data, parsed.Path)
	default:
		return nil, fmt.Errorf("unsupported schema URI scheme %q", parsed.Scheme)
	}
}

func filePathFromURL(parsed *url.URL) (string, error) {
	path, err := url.PathUnescape(parsed.Path)
	if err != nil {
		return "", err
	}
	if path == "" {
		return "", fmt.Errorf("empty file URL path")
	}
	return filepath.FromSlash(path), nil
}

func decodeSchemaDocument(data []byte, hint string) (any, error) {
	switch strings.ToLower(filepath.Ext(hint)) {
	case ".json", "":
		if value, err := decodeDocument(data, DocumentFormatJSON); err == nil {
			return value, nil
		}
	case ".yaml", ".yml":
		if value, err := decodeDocument(data, DocumentFormatYAML); err == nil {
			return value, nil
		}
	case ".toml":
		if value, err := decodeDocument(data, DocumentFormatTOML); err == nil {
			return value, nil
		}
	}
	for _, format := range []string{DocumentFormatJSON, DocumentFormatYAML, DocumentFormatTOML} {
		value, err := decodeDocument(data, format)
		if err == nil {
			return value, nil
		}
	}
	return nil, fmt.Errorf("schema document is not valid JSON, YAML, or TOML")
}

func discoverSchemaReferences(doc any, baseURI string) ([]string, error) {
	base, err := url.Parse(baseURI)
	if err != nil {
		return nil, err
	}
	return uniqueStrings(walkSchemaReferences(doc, base, nil)), nil
}

func walkSchemaReferences(value any, base *url.URL, refs []string) []string {
	switch typed := value.(type) {
	case map[string]any:
		nextBase := base
		if id, ok := typed["$id"].(string); ok && strings.TrimSpace(id) != "" {
			if parsed, err := url.Parse(strings.TrimSpace(id)); err == nil {
				nextBase = base.ResolveReference(parsed)
			}
		}
		for _, key := range []string{"$ref", "$dynamicRef", "$recursiveRef"} {
			if ref, ok := typed[key].(string); ok {
				refs = appendResolvedReference(refs, ref, nextBase)
			}
		}
		for _, child := range typed {
			refs = walkSchemaReferences(child, nextBase, refs)
		}
	case []any:
		for _, child := range typed {
			refs = walkSchemaReferences(child, base, refs)
		}
	}
	return refs
}

func appendResolvedReference(refs []string, ref string, base *url.URL) []string {
	ref = strings.TrimSpace(ref)
	if ref == "" || strings.HasPrefix(ref, "#") {
		return refs
	}
	parsed, err := url.Parse(ref)
	if err != nil {
		return refs
	}
	resolved := base.ResolveReference(parsed)
	resolved.Fragment = ""
	if resolved.String() == withoutFragmentMust(base.String()) {
		return refs
	}
	return append(refs, resolved.String())
}

func withoutFragmentMust(raw string) string {
	value, err := withoutFragment(raw)
	if err != nil {
		return raw
	}
	return value
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
