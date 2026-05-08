---
title: Output formats
description: Text, JSON, and SARIF output produced by dollarlint.
---

## Text (default)

Default text output is grouped by file:

```text
dollarlint found 2 issues in 1 file after 47ms

settings.json
  /name   type      got number, want string
  /count  minimum   minimum: got 0, want 1

Summary: 4 discovered, 3 validated, 1 skipped, 2 issues in 47ms
```

Use `--locations` to opt into line/column source mapping for text and JSON
output:

```text
settings.json
  3:11  type      got number, want string  /name
  4:12  minimum   minimum: got 0, want 1   /count
```

- `--verbose` adds schema URI and keyword metadata under each issue.
- `--quiet` produces terse success output.

Text output uses subtle terminal styling when color is available and stays
plain for machine-readable modes such as `--json` and `--sarif`.

## JSON

```sh
dollarlint validate ./config --json
```

Emits a stable JSON document with one entry per file and one entry per
issue, plus the run summary.

## SARIF

```sh
dollarlint validate ./config --sarif > dollarlint.sarif
```

Emits SARIF 2.1.0 suitable for GitHub code scanning and similar tools.

`dollarlint` builds source-location maps only for SARIF runs or when
`--locations` is requested, keeping ordinary text and JSON validation on
the simpler validation path. SARIF locations are best-effort:

- JSON positions come from a token walk over the source.
- YAML positions come from `yaml.Node` line/column metadata.
- TOML positions come from a conservative line scanner for common keys,
  tables, arrays, and inline tables.

When a validation issue points to something missing — for example a
`required` property — SARIF falls back to the nearest parent object
location. If source mapping fails for any reason, validation still
succeeds and SARIF falls back to file-level results.
