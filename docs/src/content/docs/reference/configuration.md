---
title: Configuration
description: The .dollarlint.toml configuration file.
---

`dollarlint` config files are TOML only. The CLI searches the target root
for `.dollarlint.toml`.

## Full example

```toml
version = 1

[discovery]
include = ["*.json", "**/*.json", "*.yaml", "**/*.yaml", "*.toml", "**/*.toml"]
exclude = ["node_modules", "**/node_modules/**", "dist", "**/dist/**"]

[schema]
fetchRemote = true
allowedDomains = ["www.schemastore.org", "raw.githubusercontent.com"]
blockedDomains = ["untrusted.example.com"]
azureResourcePruning = true
maxDepth = 8
concurrency = 8

[schema.fetch]
retries = 2
retryMinWait = "250ms"
retryMaxWait = "2s"

[schema.schemaStore]
enabled = false
url = "https://www.schemastore.org/api/json/catalog.json"
failure = "warn"
strict = false

[[schema.associations]]
file = "settings/*.toml"
schema = "./schemas/settings.schema.json"

[timeouts]
fetch = "10s"
compile = "30s"

[[ignore]]
file = "fixtures/*.json"
keyword = "required"
property = "legacyName"
reason = "legacy fixture kept for compatibility"

[output]
json = false
sarif = false
showSkipped = false
verbose = false
quiet = false
locations = false
```

## Sections

### `[discovery]`

Globs that control which files are walked.

- `include` — globs of files to consider.
- `exclude` — globs of files and directories to ignore.

### `[schema]`

Controls schema resolution and fetching.

- `fetchRemote` — allow `http(s)` schemas. Default `true`.
- `allowedDomains` — empty allows any host. Otherwise restricts remote
  schemas to these hosts. Supports `host.example.com` and `*.example.com`
  patterns.
- `blockedDomains` — denies these hosts even if otherwise allowed.
- `azureResourcePruning` — prune ARM deployment schemas to the resource
  providers a template actually uses. Default `true`.
- `maxDepth` — schema reference traversal depth.
- `concurrency` — parallelism for schema work.

### `[schema.fetch]`

Bounded retries for transient failures, including `408`, `425`, `429`, and
retryable `5xx` responses.

### `[schema.schemaStore]`

See [SchemaStore inference](/guides/schemastore/).

### `[[schema.associations]]`

Map files to schemas without modifying the files themselves.

```toml
[[schema.associations]]
file = "settings/*.toml"
schema = "./schemas/settings.schema.json"
```

### `[timeouts]`

- `fetch` — per-request timeout for remote schema fetches.
- `compile` — per-schema compile timeout.

### `[[ignore]]`

Suppress specific findings without disabling whole files. Each entry
matches a file glob and may further narrow by `keyword` and `property`.

```toml
[[ignore]]
file = "fixtures/*.json"
keyword = "required"
property = "legacyName"
reason = "legacy fixture kept for compatibility"
```

### `[output]`

Defaults for the corresponding CLI flags. CLI flags always win.

## Precedence

For each file, schemas are resolved in this order:

1. Explicit in-file `$schema` declaration.
2. `[[schema.associations]]` config entries.
3. SchemaStore catalog matches (if enabled).
4. Otherwise the file is skipped.
