---
title: Configuration
description: Configure discovery, schema loading, ignore rules, output, and timeouts.
---

`dollarlint` looks for configuration files in the target root:

- `.dollarlint.yaml`
- `.dollarlint.yml`
- `.dollarlint.toml`
- `.dollarlint.json`
- `dollarlint.yaml`
- `dollarlint.yml`
- `dollarlint.toml`
- `dollarlint.json`

## Full example

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

## Discovery

Use `discovery.include` and `discovery.exclude` to decide which files are considered. The defaults include JSON, YAML, and TOML while skipping common generated or vendor locations such as `node_modules`, `dist`, and `.git`.

## Schema loading

Remote `http(s)` schema fetching is enabled by default. Set `schema.fetchRemote` to `false` when you need fully offline or hermetic validation.

`schema.maxDepth` limits nested schema references, and recursion is detected so cyclical references do not spin forever.

The `examples/schemastore/` directory demonstrates remote fetching with SchemaStore URLs for common package, CI, formatting, dependency, protobuf, and Python tooling configs.

## Ignore rules

Ignore rules match known issues by file pattern, JSON Schema keyword, and property:

```yaml
ignore:
  - file: "legacy/*.json"
    keyword: "required"
    property: "enabled"
    reason: "legacy files are migrated gradually"
```

Ignored issues remain visible in machine-readable output but do not count as failing issues.
