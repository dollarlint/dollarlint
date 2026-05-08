---
title: Configuration
description: Configure discovery, schema loading, ignore rules, output, and timeouts.
---

`dollarlint` config files are TOML only. The CLI looks for configuration files in the target root:

- `.dollarlint.toml`
- `dollarlint.toml`

Create a starter config with:

```sh
dollarlint init
dollarlint init ./packages/api --schema-store
dollarlint init --output dollarlint.toml
dollarlint init --defaults --schema-store
```

`init` starts a short terminal interview by default. Use `--defaults` to skip prompts and accept defaults plus any provided flags. It will not overwrite an existing config unless you confirm overwrite or pass `--force`.

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

## Discovery

Use `discovery.include` and `discovery.exclude` to decide which files are considered. The defaults include JSON, YAML, and TOML while skipping common generated or vendor locations such as `node_modules`, `dist`, and `.git`.

## Schema loading

Remote `http(s)` schema fetching is enabled by default. Set `schema.fetchRemote` to `false` when you need fully offline or hermetic validation.

Use `schema.fetch` to tune resilience for remote schemas. dollarlint retries transient network failures, `408`, `425`, `429`, and retryable `5xx` responses with bounded backoff.

Use `schema.allowedDomains` to restrict remote schema fetching to specific hosts. Leave it empty to allow any host. Use `schema.blockedDomains` to deny specific hosts; blocked domains win over allowed domains. Entries are exact hosts such as `www.schemastore.org` or wildcard hosts such as `*.example.com`.

The CLI accepts repeatable `--allow-domain` and `--block-domain` flags for one-off runs.

## SchemaStore catalog matching

Set `schema.schemaStore.enabled` to `true` to match conventional filenames using the SchemaStore catalog when a file does not declare its own schema.

The matching precedence is explicit in-file schema, then config-level schema associations, then SchemaStore filename matches, then skipped.

Use `schema.schemaStore.url` to point at a local or mirrored catalog for hermetic CI.

By default, SchemaStore catalog failures are non-fatal: dollarlint skips catalog matching and still validates files with explicit schemas or config associations. Set `schema.schemaStore.strict` to `true` when catalog availability should fail the run.

`schema.maxDepth` limits nested schema references, and recursion is detected so cyclical references do not spin forever.

## Ignore rules

Ignore rules match known issues by file pattern, JSON Schema keyword, and property:

```toml
[[ignore]]
file = "legacy/*.json"
keyword = "required"
property = "enabled"
reason = "legacy files are migrated gradually"
```

Ignored issues remain visible in machine-readable output but do not count as failing issues.
