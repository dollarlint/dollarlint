package dollarlint_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/dollarlint/dollarlint"
)

func TestLintWithPublicSchemaMapping(t *testing.T) {
	dir := t.TempDir()
	for name, content := range map[string]string{
		"schema.json": `{"type":"string"}`,
		"data.json":   `42`,
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	cfg := dollarlint.DefaultConfig()
	disabled := false
	cfg.Schemas.Fetch.Enabled = &disabled
	cfg.Discovery.Include = []string{"data.json"}
	cfg.Schemas.Associations = []dollarlint.SchemaAssociation{{File: "data.json", Schema: "https://SCHEMAS.example.test/schema.json"}}
	cfg.Schemas.Mappings = []dollarlint.SchemaMapping{{URL: "https://schemas.example.test/schema.json", Path: filepath.Join(dir, "schema.json")}}
	result, err := dollarlint.Lint(context.Background(), dollarlint.Options{Root: dir, Config: cfg})
	if err != nil {
		t.Fatal(err)
	}
	if result.Summary.Issues.Validation != 1 || result.Summary.Skipped != 0 || len(result.Warnings) != 0 {
		t.Fatalf("expected local mapped validation: summary=%+v warnings=%+v", result.Summary, result.Warnings)
	}
}
