package engine

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func BenchmarkLintLocalSharedSchema(b *testing.B) {
	dir := b.TempDir()
	benchWriteFile(b, filepath.Join(dir, "schema.schema"), benchmarkObjectSchema(32))
	for i := 0; i < 180; i++ {
		benchWriteFile(b, filepath.Join(dir, fmt.Sprintf("local-%03d.json", i)), benchmarkObjectDocument("./schema.schema", 32))
	}

	cfg := benchmarkConfig()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result, err := Lint(context.Background(), Options{Root: dir, Config: cfg})
		if err != nil {
			b.Fatalf("Lint: %v", err)
		}
		if result.Summary.Validated != 180 || result.Summary.Issues != 0 {
			b.Fatalf("unexpected result: %+v", result.Summary)
		}
	}
}

func BenchmarkLintMixedSlowRemoteSchema(b *testing.B) {
	dir := b.TempDir()
	benchWriteFile(b, filepath.Join(dir, "local.schema"), benchmarkObjectSchema(40))
	for i := 0; i < 160; i++ {
		benchWriteFile(b, filepath.Join(dir, fmt.Sprintf("local-%03d.json", i)), benchmarkObjectDocument("./local.schema", 40))
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(45 * time.Millisecond)
		_, _ = w.Write([]byte(benchmarkObjectSchema(8)))
	}))
	defer server.Close()
	benchWriteFile(b, filepath.Join(dir, "remote.json"), benchmarkObjectDocument(server.URL+"/schema.json", 8))

	cfg := benchmarkConfig()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result, err := Lint(context.Background(), Options{Root: dir, Config: cfg})
		if err != nil {
			b.Fatalf("Lint: %v", err)
		}
		if result.Summary.Validated != 161 || result.Summary.Issues != 0 {
			b.Fatalf("unexpected result: %+v", result.Summary)
		}
	}
}

func BenchmarkLintLargeCatalog(b *testing.B) {
	dir := b.TempDir()
	benchWriteFile(b, filepath.Join(dir, "matched.schema"), benchmarkObjectSchema(12))
	var catalog strings.Builder
	catalog.WriteString(`{"schemas":[`)
	for i := 0; i < 900; i++ {
		if i > 0 {
			catalog.WriteByte(',')
		}
		fmt.Fprintf(&catalog, `{"name":"Unused %d","fileMatch":["unused-%03d.json"],"url":"./unused.schema"}`, i, i)
	}
	catalog.WriteString(`,{"name":"Matched","fileMatch":["target-*.json"],"url":"./matched.schema"}]}`)
	benchWriteFile(b, filepath.Join(dir, "catalog.catalog"), catalog.String())
	for i := 0; i < 220; i++ {
		benchWriteFile(b, filepath.Join(dir, fmt.Sprintf("target-%03d.json", i)), benchmarkObjectDocument("", 12))
	}

	cfg := benchmarkConfig()
	cfg.Schemas.Catalogs.Enabled = true
	cfg.Schemas.Catalogs.Sources = []CatalogSource{{Name: "bench", Format: "schemastore", Path: filepath.Join(dir, "catalog.catalog")}}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result, err := Lint(context.Background(), Options{Root: dir, Config: cfg})
		if err != nil {
			b.Fatalf("Lint: %v", err)
		}
		if result.Summary.Validated != 220 || result.Summary.Issues != 0 {
			b.Fatalf("unexpected result: %+v", result.Summary)
		}
	}
}

func benchmarkConfig() Config {
	cfg := configWithoutSchemaStore()
	cfg.Discovery.Include = []string{"*.json"}
	cfg.Schemas.Concurrency = 8
	return cfg
}

func benchmarkObjectSchema(properties int) string {
	var builder strings.Builder
	builder.WriteString(`{"type":"object","additionalProperties":false,"required":[`)
	for i := 0; i < properties; i++ {
		if i > 0 {
			builder.WriteByte(',')
		}
		fmt.Fprintf(&builder, "%q", fmt.Sprintf("field%d", i))
	}
	builder.WriteString(`],"properties":{"$schema":{"type":"string"}`)
	for i := 0; i < properties; i++ {
		fmt.Fprintf(&builder, `,"field%d":{"type":"string","minLength":1}`, i)
	}
	builder.WriteString(`}}`)
	return builder.String()
}

func benchmarkObjectDocument(schema string, properties int) string {
	var builder strings.Builder
	builder.WriteByte('{')
	wrote := false
	if schema != "" {
		fmt.Fprintf(&builder, `"$schema":%q`, schema)
		wrote = true
	}
	for i := 0; i < properties; i++ {
		if wrote {
			builder.WriteByte(',')
		}
		fmt.Fprintf(&builder, `"field%d":"value-%d"`, i, i)
		wrote = true
	}
	builder.WriteByte('}')
	return builder.String()
}

func benchWriteFile(tb testing.TB, path, content string) {
	tb.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		tb.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		tb.Fatalf("write %s: %v", path, err)
	}
}
