---
title: Configuration
description: Configure discovery, schema loading, ignore rules, output, and timeouts.
---

`dollarlint` config files are TOML only. The CLI looks for `.dollarlint.toml` in the target root.

Create a starter config with:

```sh
dollarlint init
dollarlint init ./packages/api --schema-store
dollarlint init --output ./packages/api/.dollarlint.toml
dollarlint init --defaults --schema-store
```

`init` starts a short terminal interview by default. Use `--defaults` to skip prompts and accept defaults plus any flags you pass. It will not overwrite an existing config unless you confirm or pass `--force`.

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

Use `schema.schemaStore.failure` to decide what happens when the catalog cannot be loaded:

- `warn` records a warning, skips SchemaStore inference, and still validates explicit/configured schemas.
- `error` fails the run with exit code `2`.
- `skip` silently skips SchemaStore inference.

`schema.schemaStore.strict = true` remains supported as a legacy alias for `failure = "error"`.

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

Ignored issues remain visible in machine-readable output but do not count as failing issues, and the run can still exit `0`. Prefer ignore rules over excluding entire directories so that new issues in the same files are still surfaced.

## Timeouts

`timeouts.fetch` caps how long any single remote schema fetch can take. `timeouts.compile` caps how long the JSON Schema compiler may run for a single root schema. The defaults are conservative; raise them if you regularly load large schema bundles such as the Azure ARM catalog.

## Output defaults

The `[output]` table sets defaults for every run, so contributors get the same shape locally that CI does. Each key matches a CLI flag of the same name:

| Key           | Flag             | Effect |
| ------------- | ---------------- | ------ |
| `json`        | `--json`         | Emit a single JSON document instead of text. |
| `sarif`       | `--sarif`        | Emit SARIF 2.1.0 instead of text. |
| `verbose`     | `--verbose`      | Add schema URI and keyword details under each issue. |
| `quiet`       | `--quiet`        | Use terse text output. |
| `locations`   | `--locations`    | Resolve and show line/column positions in text and JSON. |
| `showSkipped` | `--show-skipped` | List files skipped because they declared no schema. |

CLI flags always win over the config file, so `--quiet` on a single run does not require editing `.dollarlint.toml`.
