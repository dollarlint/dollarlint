---
title: Output formats
description: Choose text, JSON, SARIF, locations, verbosity, and quiet mode.
---

`dollarlint` produces one of three output formats per run. Pick the one that matches where the output is going to be read.

| Flag          | Format       | When to use it |
| ------------- | ------------ | -------------- |
| _(default)_   | Text         | Interactive runs and pull-request logs. |
| `--json`      | JSON         | Scripting, dashboards, and custom reporting. |
| `--sarif`     | SARIF 2.1.0  | GitHub code scanning and other SARIF consumers. |

Modifiers that apply across formats:

| Flag             | Effect |
| ---------------- | ------ |
| `--locations`    | Resolve line/column positions for text and JSON. |
| `--verbose`      | Add schema URI and keyword metadata under each text issue. |
| `--quiet`        | Use terse text output. |
| `--show-skipped` | List files skipped because they declared no schema. |

## Text

The default text output is optimized for scanning. Issues are grouped by file, aligned by location and keyword, and followed by a run summary.

```text
dollarlint found 2 issues in 1 file after 47ms

settings.json
  /name   type      got number, want string
  /count  minimum   minimum: got 0, want 1

Summary: 4 discovered, 3 validated, 1 skipped, 2 issues in 47ms
```

Use `--verbose` for schema URI, keyword location, and property details under each issue.

Warnings, such as an unavailable optional SchemaStore catalog, are reported separately from validation issues. They do not make the run fail unless the related policy is configured as `error`.

When the terminal supports color, text output uses subtle Lip Gloss styling for headings, file names, keywords, pointers, and summaries. JSON and SARIF output are never styled.

## Locations

Use `--locations` to include best-effort line and column positions:

```text
settings.json
  3:11  type      got number, want string  /name
  4:12  minimum   minimum: got 0, want 1   /count
```

Source mapping is built only when needed for `--locations` or SARIF output, keeping ordinary validation on the simpler path.

## JSON

Use `--json` for machine-readable results:

```sh
dollarlint . --json
```

The JSON summary includes discovery counts, issue counts, warning counts, ignored issue counts, elapsed duration, and nanosecond duration for precise automation.

## SARIF

Use `--sarif` to produce SARIF 2.1.0 for GitHub code scanning and similar tools:

```sh
dollarlint . --sarif > dollarlint.sarif
```

SARIF locations are best effort:

- JSON positions come from token walking.
- YAML positions come from parser node metadata.
- TOML positions come from a conservative source scanner for common tables and keys.

If source mapping cannot resolve a location, validation still succeeds and SARIF falls back to a file-level result. Run-level warnings are emitted as SARIF warning results without file locations.

SARIF and JSON output are written to standard output and are never styled, so it is safe to redirect either to a file or pipe it into another tool.
