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
dollarlint ./config --json
dollarlint . --include '**/*.yaml' --schema 'settings/*.toml=./schemas/settings.schema.json'
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
  showSkipped: false
```

By default, remote `http(s)` schema fetching is enabled. Known JSON Schema metaschemas are handled by the validator and are not pre-fetched as ordinary schema dependencies.

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
