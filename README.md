<img src="docs/public/logo.svg" alt="dollarlint logo" height="58">

# dollarlint

`dollarlint` validates source JSON, YAML, and TOML files against the JSON Schema each file declares.

Files without a schema declaration, config association, built-in association, or catalog match are skipped by default, but still counted in the run summary so CI output makes discovery behavior clear. Set `schemas.requireCoverage = true` when every included file must be covered.

## Install

```sh
go install github.com/agorischek/dollarlint/cmd/dollarlint@latest
```

To build from a local checkout:

```sh
go build -o bin/dollarlint ./cmd/dollarlint
./bin/dollarlint validate .
```

## CLI

```sh
dollarlint init
dollarlint validate .
dollarlint validate ./config --locations
dollarlint validate ./config --verbose
dollarlint validate ./config --format json
dollarlint validate ./config --format sarif --output dollarlint.sarif
dollarlint validate . --include '**/*.yaml' --schema 'settings/*.toml=./schemas/settings.schema.json'
dollarlint validate . --schema-store
dollarlint validate . --schema-store --schema-store-failure error
dollarlint validate ./examples/schemastore --locations
dollarlint validate ./examples/azure --locations
```

`validate` is the canonical validation command. Bare paths are intentionally not accepted; use `dollarlint validate [path]`.

Use `dollarlint init` to interview you and create a starter `.dollarlint.toml` in the current directory. It is safe by default and will not overwrite an existing config unless you confirm overwrite or pass `--force`.

```sh
dollarlint init
dollarlint init ./packages/api --schema-store
dollarlint init --output ./packages/api/.dollarlint.toml
dollarlint init --defaults --schema-store
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

`dollarlint` config files are TOML only. The CLI searches the target root for:

- `.dollarlint.toml`

Example:

```toml
version = 1

[discovery]
extendExclude = ["generated/**"]
useDefaultExcludes = true
respectGitIgnore = true
forceExclude = false
followSymlinks = false

[schemas]
maxDepth = 8
concurrency = 8
requireCoverage = false

[schemas.optimizations]
enabled = true

[schemas.optimizations.azure]
pruneResources = true

[schemas.fetch]
enabled = true
timeout = "10s"
retries = 2
retryMinWait = "250ms"
retryMaxWait = "2s"
allowedDomains = ["*.schemastore.org", "raw.githubusercontent.com"]
blockedDomains = ["untrusted.example.com"]

[schemas.compile]
timeout = "30s"

[schemas.catalogs]
enabled = false
failure = "warn"

[[schemas.catalogs.sources]]
name = "schemastore"
format = "schemastore"
url = "https://www.schemastore.org/api/json/catalog.json"
enabled = true

[[schemas.associations]]
file = "settings/*.toml"
schema = "./schemas/settings.schema.json"

[[ignore]]
file = "fixtures/*.json"
keyword = "required"
property = "legacyName"
reason = "legacy fixture kept for compatibility"

[output]
showSkipped = false
verbose = false
quiet = false
locations = false
branchErrors = "best"
```

Output format and output file are invocation choices, not persistent config. Use `--format text|json|sarif` and `--output <path>` on `dollarlint validate` when a run needs a machine-readable artifact.

Discovery uses safe defaults. Leave `discovery.include` unset to discover JSON, YAML, YML, and TOML files at any depth. Set `include` only when you want to replace that default set with custom discovery globs. A glob without a slash matches basenames at any depth, so `*.json` matches both `package.json` and `config/settings.json`; use slashes when you want to anchor a pattern to part of the relative path. `useDefaultExcludes = true` skips common dependency, generated, cache, and VCS directories like `node_modules`, `vendor`, `dist`, `build`, `.git`, `.venv`, and `.cache`. Add project-specific exclusions with `discovery.extendExclude` rather than copying the default list. `respectGitIgnore = true` applies root `.gitignore` patterns during directory discovery, while `forceExclude = true` also applies excludes to explicitly passed files.

By default, remote `http(s)` schema fetching is enabled. Transient network failures, `408`, `425`, `429`, and retryable `5xx` responses are retried with bounded backoff. `schemas.fetch.allowedDomains` can restrict remote schemas to specific hosts, and `schemas.fetch.blockedDomains` can deny hosts even when they otherwise match the allowlist. Leave `allowedDomains` empty to allow any remote schema host, and use entries such as `schemas.example.com` or `*.example.com` for exact or wildcard host matches. If you allowlist SchemaStore, prefer `*.schemastore.org` or include both `www.schemastore.org` and `json.schemastore.org`.

Catalog matching is configurable with `schemas.catalogs`. When enabled, files without an explicit schema can be matched by conventional filename using the built-in SchemaStore catalog, a local SchemaStore-shaped catalog, or additional catalog sources. Precedence is explicit in-file schema, then config associations, then dollarlint's built-in `.dollarlint.toml` association, then catalog matches, then skipped. Set `schemas.requireCoverage = true` to fail the run when any discovered included file is not covered by one of those schema sources.

dollarlint automatically validates discovered `.dollarlint.toml` files against its embedded config schema. Users can override that by adding an in-file schema declaration or a config association for `.dollarlint.toml`.

Catalog failures are modeled separately from validation issues. By default `schemas.catalogs.failure = "warn"` records a warning, skips catalog inference, still validates explicit/configured schemas, and exits `0` unless validation issues are found. Use `"error"` when catalog availability should fail the run with exit `2`, or `"skip"` for a silent fallback. Known JSON Schema metaschemas are handled by the validator and are not pre-fetched as ordinary schema dependencies.

Azure Resource Manager deployment schemas from `schema.management.azure.com` are pruned to the resource provider schemas used by the template before compilation. This avoids compiling the full Azure provider catalog for ordinary ARM templates. Set `schemas.optimizations.azure.pruneResources = false` to disable this Azure-specific optimization, or `schemas.optimizations.enabled = false` to disable all schema optimizations.

## Examples

The `examples/` directory includes a small local schema demo, a `examples/schemastore/` suite that validates common config files against remote schemas from `https://www.schemastore.org`, and Azure ARM deployment templates that exercise remote schema fetching plus ARM resource pruning.

```sh
dollarlint validate ./examples/schemastore --locations
dollarlint validate ./examples/azure --locations
```

## Text Output

Default text output is grouped by file:

```text
dollarlint found 2 issues in 1 file after 47ms

settings.json
  /name   type      expected string, received number
  /count  minimum   must be >= 1

Summary: 4 discovered, 3 validated, 1 skipped, 2 issues in 47ms
```

Use `--locations` to opt into line/column source mapping for text and JSON output:

```text
settings.json
  3:11  type      expected string, received number  /name
  4:12  minimum   must be >= 1   /count
```

Use `--verbose` to show schema URI and keyword metadata under each issue. Use `--quiet` for terse success output.
Set `output.branchErrors = "all"` when you need every failed `oneOf`/`anyOf` branch leaf for schema debugging; the default `"best"` reports the closest matching branch.

Text output uses subtle terminal styling when color is available and stays plain for machine-readable formats such as `--format json` and `--format sarif`.

## SARIF

Use `--format sarif` to emit SARIF 2.1.0 for GitHub code scanning and similar tools. Use `--output` to write the SARIF artifact directly:

```sh
dollarlint validate . --format sarif --output dollarlint.sarif
```

`dollarlint` builds source-location maps only for SARIF runs or when `--locations` is requested, keeping ordinary text and JSON validation on the simpler validation path. SARIF locations are best-effort:

- JSON positions are derived from a token walk over the source.
- YAML positions come from `yaml.Node` line/column metadata.
- TOML positions come from a conservative line scanner for common keys, tables, arrays, and inline tables.

When a validation issue points to something missing, such as a `required` property, SARIF falls back to the nearest parent object location. If source mapping fails for any reason, validation still succeeds and SARIF falls back to file-level results.

## Development

```sh
go test ./...
go test -coverprofile=coverage.out ./internal/engine
go tool cover -func=coverage.out
```

Most implementation lives in `internal/engine`, with CLI wiring in `internal/cli`, so future integrations such as `serve`, LSP, and MCP can share the same validation engine without expanding the public Go API accidentally.
