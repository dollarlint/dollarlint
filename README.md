<img src="docs/public/logo.svg" alt="DollarLint logo" height="58">

# DollarLint

DollarLint validates JSON (including JSONC, JSON5, JSON Lines), YAML, and TOML files against JSON Schemas.

## Install

With Homebrew:

```sh
brew install --cask dollarlint/tap/dollarlint
```

With npm:

```sh
npm install -g dollarlint
```

With Go:

```sh
go install github.com/dollarlint/dollarlint/cmd/dollarlint@latest
```

To build from a local checkout:

```sh
go build -o bin/dollarlint ./cmd/dollarlint
./bin/dollarlint validate .
```

## GitHub Action

Run DollarLint in GitHub Actions by pinning the action to the release tag you
want:

```yaml
name: DollarLint

on:
  pull_request:
  push:
    branches: [main]

permissions:
  contents: read

jobs:
  dollarlint:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v6
      - uses: dollarlint/dollarlint@vX.Y.Z
        with:
          path: .
```

The action installs the matching DollarLint release archive for the runner,
verifies it against `checksums.txt`, and runs `dollarlint validate`.

To upload SARIF results to GitHub code scanning:

```yaml
permissions:
  contents: read
  security-events: write

steps:
  - uses: actions/checkout@v6
  - uses: dollarlint/dollarlint@vX.Y.Z
    with:
      upload-sarif: true
```

Use `args` for additional validation flags, one argument per line:

```yaml
- uses: dollarlint/dollarlint@vX.Y.Z
  with:
    path: .
    args: |
      --schema-store
      --schema-store-failure=error
```

## Quick start

```sh
dollarlint init
dollarlint validate .
```

`dollarlint init` creates a starter `.dollarlint.toml` in the current directory. It is safe by default and will not overwrite an existing file unless you confirm or pass `--force`.

## Common commands

### Initialize configuration

```sh
dollarlint init
dollarlint init ./packages/api --schema-store
dollarlint init --output ./packages/api/.dollarlint.toml
dollarlint init --defaults --schema-store
```

### Validate files

```sh
dollarlint validate .
dollarlint validate ./config --locations
dollarlint validate ./config --verbose
dollarlint validate ./config --format json
dollarlint validate ./config --format sarif --output dollarlint.sarif
dollarlint validate . --include '**/*.yaml' --schema 'settings/*.toml=./schemas/settings.schema.json'
dollarlint validate . --schema-store
dollarlint validate . --schema-store --schema-store-failure error
```

Use `dollarlint validate <path>` for all validation runs. Bare paths are not accepted.

Exit codes:

- `0`: no non-ignored issues
- `1`: validation, schema loading, coverage, or parsing issues were found
- `2`: CLI/configuration error

## Schema declarations

Supported in-file conventions:

- JSON: root `$schema`, for example `{"$schema":"./schema.json"}`
- JSONC/JSON5: root `$schema`, using the same convention after comments and JSON5 syntax are normalized
- YAML: `# yaml-language-server: $schema=./schema.json`
- YAML: root `$schema`
- TOML: Taplo/Even Better TOML directive `#:schema ./schema.json`
- TOML: root `"$schema" = "./schema.json"`

JSON Lines files (`.jsonl` and `.ndjson`) are validated one non-empty line at a time. Because the file has no single root object, associate a schema through config, the `--schema` flag, or a catalog match.

Config-level schema associations can validate files that do not declare a schema themselves.

## Configuration

DollarLint configuration is TOML only. For each run, the CLI looks for one `.dollarlint.toml` in the target root, or beside an explicitly passed file. By default, that one config applies to the whole run. Set `configs.mode = "nearest"` in the root config to let nested `.dollarlint.toml` files apply to their own subtrees. A nested config inherits only when it declares `extends`.

Example:

```toml
version = 1

[configs]
mode = "single"

[discovery]
exclude = ["generated/**"]
useDefaultExcludes = true
respectGitIgnore = true
forceExclude = false
followSymlinks = false

[parsing.json]
mode = "auto"

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
cache = true
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
match = "auto"

[[schemas.catalogs.sources]]
name = "schemastore"
format = "schemastore"
url = "https://www.schemastore.org/api/json/catalog.json"
enabled = true

[[schemas.catalogs.ignore]]
file = "data/tasks.json"
reason = "application data, not a tool config"

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

Output format and artifact location are run-time options, not persistent config. Use `--format text|json|sarif` and `--output <path>` on `dollarlint validate` when you need machine-readable output.

### Discovery defaults

If `discovery.include` is unset, DollarLint discovers JSON, JSONC, JSON5, JSON Lines (`.jsonl` and `.ndjson`), YAML, YML, and TOML files at any depth.

- Set `discovery.include` only when you want to replace the default file set.
- Set `discovery.include = []` only when you intentionally want to discover no files from config.
- A glob without a slash matches basenames at any depth (`*.json` matches `package.json` and `config/settings.json`).
- `useDefaultExcludes = true` skips common dependency, generated, cache, build, temp, and VCS directories (`node_modules`, `vendor`, `dist`, `build`, `.build`, `DerivedData`, `SourcePackages/checkouts`, `.git`, `.venv`, `.cache`).
- `discovery.exclude` adds project-specific excludes on top of defaults.
- `respectGitIgnore = true` applies `.gitignore` patterns during directory discovery, including nested `.gitignore` files as the walk descends.
- `forceExclude = true` also applies excludes to explicitly passed files.

Config-authored discovery, association, and ignore globs are relative to the directory containing `.dollarlint.toml`. CLI globs such as `--include`, `--exclude`, and `--schema` are relative to the validation root. Relative schema and local catalog paths in config are also resolved from the config file's directory.

### Nested configs and extends

Use `extends` when a config should inherit another config:

```toml
extends = "../../.dollarlint.toml"
```

Use nearest mode in the root config when a parent-directory run should apply nested configs:

```toml
[configs]
mode = "nearest"
```

In nearest mode, each file uses the closest `.dollarlint.toml` at or above its directory. A nested config does not implicitly inherit parent settings; add `extends` when you want shared defaults. Passing `--config` uses that explicit config for the whole run and suppresses nested config discovery.

### JSON parsing

`parsing.json.mode = "auto"` is the default. `.json` files are parsed as strict JSON first, then retried as JSONC when strict parsing fails, so comments and trailing commas work in projects whose existing tools accept them.

- Use `"strict"` to parse every `.json` file as standard JSON.
- Use `"jsonc"` to allow JSONC syntax in every `.json` file.
- Files ending in `.jsonc` always use JSONC parsing.

### Remote schema fetching

Remote `http(s)` schema fetching is enabled by default, and successful schemas/catalogs are cached on disk.

- Set `schemas.fetch.cache = false` or pass `--no-schema-cache` to disable caching.
- Transient network failures (`408`, `425`, `429`, retryable `5xx`) are retried with bounded backoff.
- `schemas.fetch.allowedDomains` restricts allowed hosts.
- `schemas.fetch.blockedDomains` denies hosts even if they otherwise match the allowlist.
- Leave `allowedDomains` empty to allow any remote schema host.
- For SchemaStore, prefer `*.schemastore.org` or include both `www.schemastore.org` and `json.schemastore.org`.

### Catalog matching and coverage

When `schemas.catalogs.enabled = true`, files without explicit schemas can match by filename using the built-in SchemaStore catalog, a local SchemaStore-shaped catalog, or additional sources. The default `schemas.catalogs.match = "auto"` skips low-confidence generic bare filename matches such as `tasks.json`; use `"all"` when you want every catalog filename match applied.

When the built-in SchemaStore source is enabled, DollarLint layers on a small set of curated filename associations for known catalog gaps and drift. Today that includes Rust's `rustfmt.toml` / `.rustfmt.toml` and `release-plz.toml` / `.release-plz.toml`.

Catalog matches are explainable in JSON output as `schemaMatch` and in text hints for catalog-sourced issues. DollarLint reports the catalog `fileMatch` pattern, confidence, why it matched or skipped, and a suggested config rule. If a catalog match is correct, add the suggested `[[schemas.associations]]` entry to make it explicit. If a file should never be inferred from catalogs, add:

```toml
[[schemas.catalogs.ignore]]
file = "data/tasks.json"
reason = "application data, not a tool config"
```

Precedence is:
1. in-file schema declaration
2. config association
3. DollarLint's built-in `.dollarlint.toml` association
4. catalog match
5. skipped

Set `schemas.requireCoverage = true` to fail the run when any discovered included file is not covered by one of those sources.

DollarLint also validates discovered `.dollarlint.toml` files against its embedded config schema. You can override that with an in-file schema declaration or a config association for `.dollarlint.toml`.

Catalog failures are separate from validation issues. With `schemas.catalogs.failure = "warn"` (default), DollarLint records a warning, skips catalog inference or catalog-inferred validation, still validates explicit/configured schemas, and exits `0` unless validation issues exist. Use `"error"` to fail with exit `2`, or `"skip"` for a silent fallback. Files with explicit in-file schemas still report schema load or compile failures as issues.

Known JSON Schema metaschemas are handled by the validator and are not pre-fetched as ordinary schema dependencies.

### Azure optimization

Azure Resource Manager deployment schemas from `schema.management.azure.com` are pruned to the resource provider schemas used by the template before compilation. This avoids compiling the full Azure provider catalog for typical ARM templates.

- Set `schemas.optimizations.azure.pruneResources = false` to disable Azure pruning.
- Set `schemas.optimizations.enabled = false` to disable all schema optimizations.

## Examples

The `examples/` directory includes a small local schema demo, a nested-config demo, a `examples/schemastore/` suite that validates common config files against remote schemas from `https://www.schemastore.org`, and Azure ARM deployment templates that exercise remote schema fetching plus ARM resource pruning.

```sh
dollarlint validate ./examples/nested-configs --locations
dollarlint validate ./examples/schemastore --locations
dollarlint validate ./examples/azure --locations
```

## Text output

Default text output is grouped by file:

```text
dollarlint found 2 validation issues in 1 file after 47ms

settings.json
  /name   type      expected string, received number
  /count  minimum   must be >= 1

Summary: 4 discovered, 3 validated, 1 skipped, 2 validation issues in 47ms
```

Use `--locations` to opt into line/column source mapping for text output. JSON and SARIF request source mapping automatically.

```text
settings.json
  3:11  type      expected string, received number  /name
  4:12  minimum   must be >= 1   /count
```

Use `--verbose` to show schema URI and keyword metadata under each issue. Use `--quiet` for terse success output.
Set `output.branchErrors = "all"` when you need every failed `oneOf`/`anyOf` branch leaf for schema debugging; the default `"best"` reports the closest matching branch.

JSON output (`--format json`) includes a top-level `$schema` declaration, `formatVersion`, relative `path` fields, root-relative local schemas, active findings in `issues`, ignored findings in `ignoredIssues`, always-present arrays, per-issue `category`, structured warnings, skip classification fields on skipped files, and numeric `summary.durationNanos`. The output schema is published in `schemas/dollarlint-result.schema.json`.

Use `--format bundle` when you need one artifact that captures every renderer from the same validation run. Bundle output is JSON with `json`, `sarif`, and `styled` keys; `styled` includes the normal terminal text as both ANSI and plain strings, plus the text-output options used to render it.

Text output uses subtle terminal styling when color is available and stays plain for machine-readable formats such as `--format json`, `--format sarif`, and `--format bundle`.

## SARIF

Use `--format sarif` to emit SARIF 2.1.0 for GitHub code scanning and similar tools. Use `--output` to write the SARIF artifact directly:

```sh
dollarlint validate . --format sarif --output dollarlint.sarif
```

DollarLint builds source-location maps for JSON and SARIF runs, and for text output when `--locations` is requested. Source locations are best-effort:

- JSON-family positions are derived from a token walk over the source.
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
