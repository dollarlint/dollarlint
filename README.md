# dollarlint

`dollarlint` validates source JSON, YAML, and TOML files against the JSON Schema each file declares.

It is built as both a CLI and a Go SDK. Files without a schema declaration are skipped, but still counted in the run summary so CI output makes discovery behavior clear.

## Install

```sh
go install github.com/agorischek/dollarlint/cmd/dollarlint@latest
```

## CLI

```sh
dollarlint .
dollarlint ./config --locations
dollarlint ./config --verbose
dollarlint ./config --json
dollarlint ./config --sarif > dollarlint.sarif
dollarlint . --include '**/*.yaml' --schema 'settings/*.toml=./schemas/settings.schema.json'
dollarlint ./examples/schemastore --locations
```

Exit codes:

- `0`: no non-ignored issues
- `1`: validation, schema loading, or parsing issues were found
- `2`: CLI/configuration error

## Schema Declarations

Supported in-file conventions:

- JSON: root `$schema`, for example `{"$schema":"./schema.json"}`
- YAML: `# yaml-language-server: $schema=./schema.json`
- YAML: root `$schema`
- TOML: Taplo/Even Better TOML directive `#:schema ./schema.json`
- TOML: root `"$schema" = "./schema.json"`

Config-level schema associations can validate files that do not declare a schema themselves.

## Configuration

`dollarlint` searches the target root for:

- `.dollarlint.yaml`
- `.dollarlint.yml`
- `.dollarlint.toml`
- `.dollarlint.json`
- `dollarlint.yaml`
- `dollarlint.yml`
- `dollarlint.toml`
- `dollarlint.json`

Example:

```yaml
version: 1

discovery:
  include:
    - "*.json"
    - "**/*.json"
    - "*.yaml"
    - "**/*.yaml"
    - "*.toml"
    - "**/*.toml"
  exclude:
    - node_modules
    - "**/node_modules/**"
    - dist
    - "**/dist/**"

schema:
  fetchRemote: true
  maxDepth: 8
  concurrency: 8
  associations:
    - file: "settings/*.toml"
      schema: "./schemas/settings.schema.json"

timeouts:
  fetch: 10s
  compile: 30s

ignore:
  - file: "fixtures/*.json"
    keyword: "required"
    property: "legacyName"
    reason: "legacy fixture kept for compatibility"

output:
  json: false
  sarif: false
  showSkipped: false
  verbose: false
  quiet: false
  locations: false
```

By default, remote `http(s)` schema fetching is enabled. Known JSON Schema metaschemas are handled by the validator and are not pre-fetched as ordinary schema dependencies.

## Examples

The `examples/` directory includes a small local schema demo and a `examples/schemastore/` suite that validates common config files against remote schemas from `https://www.schemastore.org`.

```sh
dollarlint ./examples/schemastore --locations
```

## Text Output

Default text output is grouped by file:

```text
dollarlint found 2 issues in 1 file after 47ms

settings.json
  /name   type      got number, want string
  /count  minimum   minimum: got 0, want 1

Summary: 4 discovered, 3 validated, 1 skipped, 2 issues in 47ms
```

Use `--locations` to opt into line/column source mapping for text and JSON output:

```text
settings.json
  3:11  type      got number, want string  /name
  4:12  minimum   minimum: got 0, want 1   /count
```

Use `--verbose` to show schema URI and keyword metadata under each issue. Use `--quiet` for terse success output.

## SARIF

Use `--sarif` to emit SARIF 2.1.0 for GitHub code scanning and similar tools.

`dollarlint` builds source-location maps only for SARIF runs or when `--locations` is requested, keeping ordinary text and JSON validation on the simpler validation path. SARIF locations are best-effort:

- JSON positions are derived from a token walk over the source.
- YAML positions come from `yaml.Node` line/column metadata.
- TOML positions come from a conservative line scanner for common keys, tables, arrays, and inline tables.

When a validation issue points to something missing, such as a `required` property, SARIF falls back to the nearest parent object location. If source mapping fails for any reason, validation still succeeds and SARIF falls back to file-level results.

## Go SDK

```go
package main

import (
	"context"
	"log"

	"github.com/agorischek/dollarlint"
)

func main() {
	cfg := dollarlint.DefaultConfig()
	result, err := dollarlint.Lint(context.Background(), dollarlint.Options{
		Root:   ".",
		Config: cfg,
	})
	if err != nil {
		log.Fatal(err)
	}
	if result.HasIssues() {
		log.Fatalf("found %d issues", result.Summary.Issues)
	}
}
```

## Development

```sh
go test ./...
go test -coverprofile=coverage.out .
go tool cover -func=coverage.out
```
